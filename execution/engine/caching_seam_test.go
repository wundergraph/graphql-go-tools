package engine_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engine "github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache/cachetesting"
)

func TestCaching_StageNoop_Golden(t *testing.T) {
	engine.Plan(t, cachetesting.StageNoop, "{ me { id username } }", map[string]string{
		"*": `{"data":{"me":{"id":"1","username":"ada"}}}`,
	})
}

func TestCaching_LoaderSeam_ConfigGates(t *testing.T) {
	responses := map[string]string{
		"*": `{"data":{"topProducts":[{"upc":"1","name":"Table"},{"upc":"2","name":"Chair"}]}}`,
	}
	wantBody := `{"data":{"topProducts":[{"upc":"1","name":"Table"},{"upc":"2","name":"Chair"}]}}`

	t.Run("[A1] controller nil fetches normally", func(t *testing.T) {
		pr := engine.Plan(t, cachetesting.StageNoop, "{ topProducts { upc name } }", responses)

		got := resolvePlan(t, pr.Response, nil)

		assert.Equal(t, cachetesting.Compact(t, wantBody), got)
	})

	t.Run("[A2] controller set and all Cache nil does not begin", func(t *testing.T) {
		pr := engine.Plan(t, cachetesting.StageNoop, "{ topProducts { upc name } }", responses)
		fake := cachetesting.NewRecordingCache(nil)

		got := resolvePlan(t, pr.Response, fake)

		assert.Equal(t, []cachetesting.Call(nil), fake.Calls())
		assert.Equal(t, int64(0), fake.Begins())
		assert.Equal(t, cachetesting.Compact(t, wantBody), got)
	})
}

func TestCaching_Decision_Dispatch(t *testing.T) {
	// C3 full-hit skip is deferred to A2, where the real controller can splice
	// cached bytes before the loader skips the network. The synthetic config
	// below is only a loader-seam driver for dispatch once a fetch is eligible.
	tests := []struct {
		name      string
		query     string
		responses map[string]string
		script    map[string]cachetesting.ScriptedDecision
		wantCalls []cachetesting.Call
		wantBody  string
	}{
		{
			name:  "[C1] miss fetches and records result",
			query: "{ topProducts { upc name } }",
			responses: map[string]string{
				"*": `{"data":{"topProducts":[{"upc":"1","name":"Table"}]}}`,
			},
			script: map[string]cachetesting.ScriptedDecision{
				"": {
					Decision: resolve.DecisionFetch,
					Handle:   &resolve.FetchCacheHandle{Decision: resolve.DecisionFetch},
				},
			},
			wantCalls: []cachetesting.Call{
				{
					Op:         "Prepare",
					FetchPath:  "",
					Items:      1,
					InputBytes: `{"method":"POST","url":"http://product.service","header":{},"body":{"query":"{topProducts {upc name}}"}}`,
					Decision:   resolve.DecisionFetch,
				},
				{
					Op:           "Result",
					FetchPath:    "",
					Items:        1,
					ResponseData: `{"topProducts":[{"upc":"1","name":"Table"}]}`,
				},
				{Op: "End"},
			},
			wantBody: `{"data":{"topProducts":[{"upc":"1","name":"Table"}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := engine.Plan(t, cachetesting.StageNoop, tt.query, tt.responses)
			injectCache(t, pr.Response, "", &resolve.FetchCacheConfig{L2: true})
			fake := cachetesting.NewRecordingCache(tt.script)

			got := resolvePlan(t, pr.Response, fake)

			assert.Equal(t, tt.wantCalls, fake.Calls())
			assert.Equal(t, cachetesting.Compact(t, tt.wantBody), got)
		})
	}
}

func resolvePlan(t *testing.T, resp *resolve.GraphQLResponse, controller resolve.CacheController) string {
	t.Helper()

	ctx := resolve.NewContext(t.Context())
	if controller != nil {
		ctx.SetCacheController(controller)
	}
	var buf bytes.Buffer
	r := resolve.New(t.Context(), resolve.ResolverOptions{})
	_, err := r.ResolveGraphQLResponse(ctx, resp, nil, &buf)
	require.NoError(t, err)
	return buf.String()
}

// injectCache is a loader-seam driver: it tests the loader's decision dispatch
// once a fetch has a config, independent of the still-inert S4b stamper.
func injectCache(t *testing.T, resp *resolve.GraphQLResponse, responsePath string, cfg *resolve.FetchCacheConfig) {
	t.Helper()

	require.True(t, setCache(resp.Fetches, responsePath, cfg), "fetch path %q not found", responsePath)
}

func setCache(node *resolve.FetchTreeNode, responsePath string, cfg *resolve.FetchCacheConfig) bool {
	if node == nil {
		return false
	}
	if node.Item != nil && node.Item.ResponsePath == responsePath {
		switch fetch := node.Item.Fetch.(type) {
		case *resolve.SingleFetch:
			fetch.Cache = cfg
		case *resolve.EntityFetch:
			fetch.Cache = cfg
		case *resolve.BatchEntityFetch:
			fetch.Cache = cfg
		default:
			return false
		}
		return true
	}
	for _, child := range node.ChildNodes {
		if setCache(child, responsePath, cfg) {
			return true
		}
	}
	return false
}
