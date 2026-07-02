package cachingtesting

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// The engine-based half of the harness: real ExecutionEngine over real HTTP
// subgraph upstreams (the federation_integration_static_test.go pattern),
// with caching wired through Configuration.SetCaching and the runtime
// controller attached per request via engine.WithCacheController. Plan()
// remains for PLAN-shape assertions (pretty-printed plans, ART trace
// internals, defer frames, gated in-process ordering, benchmarks); request
// execution goes through the engine.

// SubgraphRule routes one canned response: the first rule whose Match
// substring appears in the incoming request body wins (empty Match matches
// everything). Count records how many requests the rule served.
type SubgraphRule struct {
	Match    string
	Response string
	Count    atomic.Int64
}

// Subgraph is one httptest upstream with routing rules and a request counter.
type Subgraph struct {
	Rules []*SubgraphRule

	mu       sync.Mutex
	requests []string
	server   *httptest.Server
	count    atomic.Int64
}

// Requests returns how many requests the subgraph received in total.
func (s *Subgraph) Requests() int64 {
	return s.count.Load()
}

// Bodies returns the exact request bodies received, in order.
func (s *Subgraph) Bodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// Subgraphs maps subgraph NAME to its upstream double.
type Subgraphs map[string]*Subgraph

// Rules builds a Subgraph from ordered routing rules.
func Rules(rules ...*SubgraphRule) *Subgraph {
	return &Subgraph{Rules: rules}
}

// Respond builds a single-response Subgraph (every request gets response).
func Respond(response string) *Subgraph {
	return Rules(&SubgraphRule{Response: response})
}

// Rule builds one routing rule.
func Rule(match, response string) *SubgraphRule {
	return &SubgraphRule{Match: match, Response: response}
}

func (s *Subgraph) start(tb testing.TB, name string) {
	tb.Helper()
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			tb.Errorf("subgraph %q: read body: %v", name, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.count.Add(1)
		s.mu.Lock()
		s.requests = append(s.requests, string(body))
		s.mu.Unlock()
		for _, rule := range s.Rules {
			if rule.Match == "" || strings.Contains(string(body), rule.Match) {
				rule.Count.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(rule.Response))
				return
			}
		}
		tb.Errorf("subgraph %q: no rule matched request body: %s", name, body)
		http.Error(w, "no canned response", http.StatusInternalServerError)
	}))
	tb.Cleanup(s.server.Close)
}

// NewEngine builds a real ExecutionEngine over the committed caching fixture
// federation, with every datasource pointed at its subgraph double and the
// caching policies (keyed by SUBGRAPH NAME) wired through SetCaching. Every
// configured subgraph must have a double; unconfigured doubles fail the test
// on first use via their own rule mismatch.
func NewEngine(tb testing.TB, caching map[string]cacheconfig.CachingConfiguration, subgraphs Subgraphs) *engine.ExecutionEngine {
	tb.Helper()

	rc := routerConfig(tb)
	nameToID := subgraphNameToDatasourceID(rc)
	idToName := make(map[string]string, len(nameToID))
	for name, id := range nameToID {
		idToName[id] = name
	}

	// Point each datasource at its double BEFORE the engine configuration is
	// built; a subgraph without a double keeps its unreachable fixture URL, so
	// touching it fails loudly.
	for _, ds := range rc.EngineConfig.DatasourceConfigurations {
		name := idToName[ds.Id]
		subgraph, ok := subgraphs[name]
		if !ok {
			continue
		}
		subgraph.start(tb, name)
		require.NotNil(tb, ds.CustomGraphql, "datasource %q has no graphql config", name)
		ds.CustomGraphql.Fetch.Url.StaticVariableContent = subgraph.server.URL
	}
	for name := range subgraphs {
		_, ok := nameToID[name]
		require.True(tb, ok, "unknown subgraph name %q", name)
	}

	factory := engine.NewFederationEngineConfigFactory(tb.Context())
	conf, err := factory.BuildEngineConfiguration(rc)
	require.NoError(tb, err)

	if len(caching) > 0 {
		byID := make(map[string]cacheconfig.CachingConfiguration, len(caching))
		for name, cfg := range caching {
			id, ok := nameToID[name]
			require.True(tb, ok, "caching configured for unknown subgraph %q", name)
			byID[id] = cfg
		}
		conf.SetCaching(byID)
	}

	executionEngine, err := engine.NewExecutionEngine(tb.Context(), abstractlogger.Noop{}, conf, resolve.ResolverOptions{MaxConcurrency: 16})
	require.NoError(tb, err)
	return executionEngine
}

// Execute runs one operation through the engine with the given runtime cache
// controller (nil = caching runtime off) and returns the full response body.
func Execute(tb testing.TB, executionEngine *engine.ExecutionEngine, query string, controller resolve.CacheController, options ...engine.ExecutionOptions) string {
	return ExecuteWithVariables(tb, executionEngine, query, "", controller, options...)
}

// ExecuteWithVariables is Execute with request variables (JSON, "" for none).
func ExecuteWithVariables(tb testing.TB, executionEngine *engine.ExecutionEngine, query, variables string, controller resolve.CacheController, options ...engine.ExecutionOptions) string {
	tb.Helper()
	request := &graphql.Request{Query: query}
	if variables != "" {
		request.Variables = []byte(variables)
	}
	if controller != nil {
		options = append(options, engine.WithCacheController(controller))
	}
	writer := graphql.NewEngineResultWriter()
	require.NoError(tb, executionEngine.Execute(context.Background(), request, &writer, options...))
	return writer.String()
}
