package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

const deduplicationTestSchema = `type Query { hello: String }`

// deduplicationTestSubgraph echoes the Authorization header it received back as the
// value of "hello", so a response reveals which caller's headers the fetch was made
// with. The delay keeps the single flight window open long enough for concurrent
// executions to meet inside it.
type deduplicationTestSubgraph struct {
	server *httptest.Server
	calls  atomic.Int64
}

func newDeduplicationTestSubgraph(t *testing.T, delay time.Duration) *deduplicationTestSubgraph {
	t.Helper()

	subgraph := &deduplicationTestSubgraph{}
	subgraph.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subgraph.calls.Add(1)
		authorization := r.Header.Get("Authorization")

		time.Sleep(delay)

		w.Header().Set("Content-Type", "application/json")
		response, err := json.Marshal(map[string]any{
			"data": map[string]any{"hello": authorization},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(response)
	}))
	t.Cleanup(subgraph.server.Close)

	return subgraph
}

func newDeduplicationTestEngine(t *testing.T, subgraphURL string) *ExecutionEngine {
	t.Helper()

	schema, err := graphql.NewSchemaFromString(deduplicationTestSchema)
	require.NoError(t, err)

	engineConf := NewConfiguration(schema)
	engineConf.SetDataSources([]plan.DataSource{
		mustGraphqlDataSourceConfiguration(t,
			"id",
			mustFactory(t, http.DefaultClient),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"hello"}},
				},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    subgraphURL,
					Method: http.MethodPost,
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, deduplicationTestSchema),
			}),
		),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	require.NoError(t, err)

	return engine
}

// testSubgraphHeaders is a minimal resolve.SubgraphHeadersBuilder that forwards a
// single Authorization header.
type testSubgraphHeaders struct {
	header http.Header
	hash   uint64
}

func newTestSubgraphHeaders(authorization string) *testSubgraphHeaders {
	header := http.Header{}
	header.Set("Authorization", authorization)

	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	digest := xxhash.New()
	for _, key := range keys {
		_, _ = digest.WriteString(key)
		_, _ = digest.WriteString(":")
		_, _ = digest.WriteString(strings.Join(header[key], ","))
		_, _ = digest.WriteString(";")
	}

	return &testSubgraphHeaders{header: header, hash: digest.Sum64()}
}

func (h *testSubgraphHeaders) HeadersForSubgraph(_ string) (http.Header, uint64) {
	return h.header.Clone(), h.hash
}

func (h *testSubgraphHeaders) HashAll() uint64 {
	return h.hash
}

// executeConcurrently runs one execution per session, all released at the same moment,
// and returns the value of "hello" each of them saw.
func executeConcurrently(t *testing.T, engine *ExecutionEngine, sessions []string, options func(session string) []ExecutionOptions) []string {
	t.Helper()

	results := make([]string, len(sessions))
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i, session := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()

			operation := graphql.Request{Query: `query Op { hello }`, OperationName: "Op"}
			resultWriter := graphql.NewEngineResultWriter()

			<-start
			err := engine.Execute(context.Background(), &operation, &resultWriter, options(session)...)
			assert.NoError(t, err)

			var parsed struct {
				Data struct {
					Hello string `json:"hello"`
				} `json:"data"`
			}
			assert.NoError(t, json.Unmarshal([]byte(resultWriter.String()), &parsed))
			results[i] = parsed.Data.Hello
		}()
	}

	close(start)
	wg.Wait()

	return results
}

func TestWithSubgraphHeadersBuilder(t *testing.T) {
	t.Parallel()

	t.Run("sets the builder on the resolve context", func(t *testing.T) {
		t.Parallel()

		builder := newTestSubgraphHeaders("session-1")
		executionCtx := &internalExecutionContext{
			resolveContext: resolve.NewContext(context.Background()),
		}

		WithSubgraphHeadersBuilder(builder)(executionCtx)

		assert.Same(t, builder, executionCtx.resolveContext.SubgraphHeadersBuilder)
	})

	t.Run("concurrent identical operations resolve against their own headers", func(t *testing.T) {
		t.Parallel()

		subgraph := newDeduplicationTestSubgraph(t, 50*time.Millisecond)
		engine := newDeduplicationTestEngine(t, subgraph.server.URL)

		sessions := make([]string, 8)
		for i := range sessions {
			sessions[i] = fmt.Sprintf("session-%d", i)
		}

		results := executeConcurrently(t, engine, sessions, func(session string) []ExecutionOptions {
			return []ExecutionOptions{WithSubgraphHeadersBuilder(newTestSubgraphHeaders(session))}
		})

		assert.Equal(t, sessions, results)
		assert.Equal(t, int64(len(sessions)), subgraph.calls.Load(),
			"each distinct header set must produce its own fetch")
	})

	t.Run("identical headers are still deduplicated", func(t *testing.T) {
		t.Parallel()

		subgraph := newDeduplicationTestSubgraph(t, 50*time.Millisecond)
		engine := newDeduplicationTestEngine(t, subgraph.server.URL)

		sessions := make([]string, 8)
		for i := range sessions {
			sessions[i] = "session-shared"
		}

		results := executeConcurrently(t, engine, sessions, func(session string) []ExecutionOptions {
			return []ExecutionOptions{WithSubgraphHeadersBuilder(newTestSubgraphHeaders(session))}
		})

		assert.Equal(t, sessions, results)
		assert.Less(t, subgraph.calls.Load(), int64(len(sessions)),
			"a shared header set must still collapse into fewer fetches")
	})
}

func TestWithExecutionOptions(t *testing.T) {
	t.Parallel()

	t.Run("sets the execution options on the resolve context", func(t *testing.T) {
		t.Parallel()

		executionCtx := &internalExecutionContext{
			resolveContext: resolve.NewContext(context.Background()),
		}

		WithExecutionOptions(resolve.ExecutionOptions{
			DisableSubgraphRequestDeduplication: true,
			DisableInboundRequestDeduplication:  true,
		})(executionCtx)

		assert.True(t, executionCtx.resolveContext.ExecutionOptions.DisableSubgraphRequestDeduplication)
		assert.True(t, executionCtx.resolveContext.ExecutionOptions.DisableInboundRequestDeduplication)
	})

	t.Run("disabling subgraph deduplication gives every execution its own fetch", func(t *testing.T) {
		t.Parallel()

		subgraph := newDeduplicationTestSubgraph(t, 50*time.Millisecond)
		engine := newDeduplicationTestEngine(t, subgraph.server.URL)

		sessions := make([]string, 8)
		for i := range sessions {
			sessions[i] = "session-shared"
		}

		results := executeConcurrently(t, engine, sessions, func(session string) []ExecutionOptions {
			return []ExecutionOptions{
				WithSubgraphHeadersBuilder(newTestSubgraphHeaders(session)),
				WithExecutionOptions(resolve.ExecutionOptions{DisableSubgraphRequestDeduplication: true}),
			}
		})

		assert.Equal(t, sessions, results)
		assert.Equal(t, int64(len(sessions)), subgraph.calls.Load(),
			"deduplication is disabled, so no fetch may be shared")
	})
}
