package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// harness is a federated engine over three stub subgraphs. users answers me,
// products answers topProducts, and reviews answers the entity fetch behind both
// me.reviews and topProducts.reviews, which is the fetch an response cache stores.
type harness struct {
	engine   *ExecutionEngine
	users    *stub
	products *stub
	reviews  *stub
}

func newHarness(t *testing.T, configure ...func(*Configuration)) *harness {
	t.Helper()

	h := &harness{users: newStub(t), products: newStub(t), reviews: newStub(t)}

	// The federation configuration is the one checked in for these tests, with
	// the three service placeholders pointed at the stubs, so no schema or
	// datasource configuration has to be written out here.
	cfgData, err := os.ReadFile("testdata/config_factory_federation/config.json")
	require.NoError(t, err)
	cfgData = bytes.ReplaceAll(cfgData, []byte("http://user.service"), []byte(h.users.server.URL))
	cfgData = bytes.ReplaceAll(cfgData, []byte("http://product.service"), []byte(h.products.server.URL))
	cfgData = bytes.ReplaceAll(cfgData, []byte("http://review.service"), []byte(h.reviews.server.URL))

	var routerConfig nodev1.RouterConfig
	require.NoError(t, protojson.Unmarshal(cfgData, &routerConfig))

	ctx := t.Context()
	engineConfig, err := NewFederationEngineConfigFactory(ctx).BuildEngineConfiguration(&routerConfig)
	require.NoError(t, err)
	for _, configure := range configure {
		configure(&engineConfig)
	}

	h.engine, err = NewExecutionEngine(ctx, abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	require.NoError(t, err)

	return h
}

func (h *harness) execute(t *testing.T, query string, options ...ExecutionOptions) string {
	t.Helper()

	writer := graphql.NewEngineResultWriter()
	err := h.engine.Execute(t.Context(), &graphql.Request{Query: query}, &writer, options...)
	require.NoError(t, err)

	return writer.String()
}

// withResponseCache hands the execution its cache the way a router does, by setting
// it on the resolve context. Reaching into the execution context is what keeps
// this out of the engine's exported surface: the cache is the caller's to supply,
// and no option needs to exist for a test to supply one.
func withResponseCache(t *testing.T, cache caching.Cache) ExecutionOptions {
	t.Helper()

	return func(execCtx *internalExecutionContext) {
		execCtx.resolveContext.SetResponseCache(cache, time.Minute, func(err error) {
			t.Errorf("response cache reported an error: %v", err)
		})
	}
}

func withRateLimiter(limiter resolve.RateLimiter) ExecutionOptions {
	return func(execCtx *internalExecutionContext) {
		execCtx.resolveContext.RateLimitOptions = resolve.RateLimitOptions{Enable: true}
		execCtx.resolveContext.SetRateLimiter(limiter)
	}
}

func withLoaderHooks(hooks resolve.LoaderHooks) ExecutionOptions {
	return func(execCtx *internalExecutionContext) {
		execCtx.resolveContext.LoaderHooks = hooks
	}
}

// stub is a subgraph that answers with whatever the test last gave it, so a case
// states what a subgraph returns at the point that matters rather than through a
// handler that has to work it out from the request it was sent.
type stub struct {
	server *httptest.Server
	count  atomic.Int64
	last   atomic.Value // string
	state  atomic.Value // stubState
}

type stubState struct {
	body         string
	status       int
	cacheControl string
}

func newStub(t *testing.T) *stub {
	t.Helper()

	s := &stub{}
	s.last.Store("")
	s.state.Store(stubState{status: http.StatusOK, cacheControl: cacheableForAMinute})

	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.count.Add(1)
		request, _ := io.ReadAll(r.Body)
		s.last.Store(string(request))

		state := s.state.Load().(stubState)
		w.Header().Set("Content-Type", "application/json")
		if state.cacheControl != "" {
			w.Header().Set("Cache-Control", state.cacheControl)
		}
		if state.status != http.StatusOK {
			w.WriteHeader(state.status)
		}
		_, _ = w.Write([]byte(state.body))
	}))
	t.Cleanup(s.server.Close)

	return s
}

func (s *stub) update(mutate func(*stubState)) {
	state := s.state.Load().(stubState)
	mutate(&state)
	s.state.Store(state)
}

func (s *stub) answers(body string)       { s.update(func(st *stubState) { st.body = body }) }
func (s *stub) status(code int)           { s.update(func(st *stubState) { st.status = code }) }
func (s *stub) cacheControl(value string) { s.update(func(st *stubState) { st.cacheControl = value }) }
func (s *stub) calls() int64              { return s.count.Load() }

// representationCount is how many entities the last entity fetch asked for, which
// is the only way to tell a batch that was trimmed from one that was not.
func (s *stub) representationCount(t *testing.T) int {
	t.Helper()

	var request struct {
		Variables struct {
			Representations []json.RawMessage `json:"representations"`
		} `json:"variables"`
	}
	require.NoError(t, json.Unmarshal([]byte(s.last.Load().(string)), &request))

	return len(request.Variables.Representations)
}

// productsAnswer builds the products subgraph's answer to topProducts, one entry
// per upc in the order given.
func productsAnswer(upcs ...string) string {
	entries := make([]string, len(upcs))
	for i, upc := range upcs {
		entries[i] = fmt.Sprintf(`{"upc":%q,"name":"Product %s","__typename":"Product"}`, upc, upc)
	}

	return `{"data":{"topProducts":[` + strings.Join(entries, ",") + `]}}`
}

// reviewsAnswer builds the reviews subgraph's answer to an entity fetch
func reviewsAnswer(bodies ...string) string {
	entries := make([]string, len(bodies))
	for i, body := range bodies {
		if body == "" {
			entries[i] = "null"
			continue
		}
		entries[i] = fmt.Sprintf(`{"reviews":[{"body":%q}]}`, body)
	}

	return `{"data":{"_entities":[` + strings.Join(entries, ",") + `]}}`
}

// --- recorders ---

type countingRateLimiter struct {
	preFetch atomic.Int64
}

func (l *countingRateLimiter) RateLimitPreFetch(*resolve.Context, *resolve.FetchInfo, json.RawMessage) (*resolve.RateLimitDeny, error) {
	l.preFetch.Add(1)
	return nil, nil
}

func (l *countingRateLimiter) RenderResponseExtension(*resolve.Context, io.Writer) error { return nil }

type countingLoaderHooks struct {
	onLoad     atomic.Int64
	onFinished atomic.Int64
}

func (h *countingLoaderHooks) OnLoad(ctx context.Context, _ resolve.DataSourceInfo) context.Context {
	h.onLoad.Add(1)
	return ctx
}

func (h *countingLoaderHooks) OnFinished(context.Context, resolve.DataSourceInfo, *resolve.ResponseInfo) {
	h.onFinished.Add(1)
}

// mapCache is an response cache held in a map, which is all the engine asks of one.
// Expiry is left out because nothing here waits for it: a TTL is only checked for
// being positive, the way a real adapter refuses an item it cannot expire.
type mapCache struct {
	mu    sync.Mutex
	items map[string]caching.Item
}

func newMapCache() *mapCache {
	return &mapCache{items: make(map[string]caching.Item)}
}

func (c *mapCache) GetMany(_ context.Context, keys []string) (map[string]caching.Item, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	found := make(map[string]caching.Item, len(keys))
	for _, key := range keys {
		if item, ok := c.items[key]; ok {
			found[key] = item
		}
	}

	return found, nil
}

func (c *mapCache) SetMany(_ context.Context, items []caching.Item) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, item := range items {
		if item.TTL <= 0 {
			return fmt.Errorf("%w: key %q", caching.ErrMissingTTL, item.Key)
		}
		c.items[item.Key] = item
	}

	return nil
}

func (c *mapCache) keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]string, 0, len(c.items))
	for key := range c.items {
		keys = append(keys, key)
	}

	return keys
}
