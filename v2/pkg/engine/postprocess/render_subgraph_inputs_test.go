package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// renderInputFetch builds a SingleFetch carrying a deferred SubgraphOperation
// artifact (empty Input) that the renderSubgraphInputs stage must render.
func renderInputFetch(t *testing.T, fetchID int, operationName, source string, envelope resolve.SubgraphRequestEnvelope, fragments []resolve.SubgraphVariable, queryPlan *resolve.QueryPlan) *resolve.FetchTreeNode {
	t.Helper()
	return resolve.Single(&resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{FetchID: fetchID},
		FetchConfiguration: resolve.FetchConfiguration{
			OperationName: operationName,
			QueryPlan:     queryPlan,
			SubgraphOperation: &resolve.SubgraphOperation{
				Document:  parseUpstreamDocument(t, source),
				Variables: fragments,
				Envelope:  envelope,
			},
		},
	})
}

func renderedInput(t *testing.T, node *resolve.FetchTreeNode) string {
	t.Helper()
	f, ok := node.Item.Fetch.(*resolve.SingleFetch)
	require.True(t, ok)
	return f.Input
}

func TestRenderSubgraphInputs_Render(t *testing.T) {
	const source = `query($representations: [_Any!]!){_entities(representations: $representations){__typename}}`
	fragments := []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}}

	t.Run("without header", func(t *testing.T) {
		node := renderInputFetch(t, 1, "", source, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x"}, fragments, nil)
		(&renderSubgraphInputs{}).ProcessFetchTree(resolve.Sequence(node))
		require.Equal(t,
			`{"method":"POST","url":"http://x","body":{"query":"`+source+`","variables":{"representations":[$$0$$]}}}`,
			renderedInput(t, node),
		)
		require.Nil(t, node.Item.Fetch.(*resolve.SingleFetch).SubgraphOperation)
	})

	t.Run("with header", func(t *testing.T) {
		node := renderInputFetch(t, 1, "", source, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x", Header: []byte(`{"Auth":["secret"]}`)}, fragments, nil)
		(&renderSubgraphInputs{}).ProcessFetchTree(resolve.Sequence(node))
		require.Equal(t,
			`{"method":"POST","url":"http://x","header":{"Auth":["secret"]},"body":{"query":"`+source+`","variables":{"representations":[$$0$$]}}}`,
			renderedInput(t, node),
		)
	})

	t.Run("multiple variables replay sjson write order", func(t *testing.T) {
		// The variables list is replayed through sjson.SetRawBytes in the exact
		// write order the planner recorded it — reproducing the planner's bytes
		// byte-for-byte, including sjson's prepend-new-key behaviour (the second
		// written key "first" lands before the first written key).
		frags := []resolve.SubgraphVariable{
			{Name: "representations", Value: []byte("[$$0$$]")},
			{Name: "first", Value: []byte("$$1$$")},
		}
		node := renderInputFetch(t, 1, "", source, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x"}, frags, nil)
		(&renderSubgraphInputs{}).ProcessFetchTree(resolve.Sequence(node))
		require.Equal(t,
			`{"method":"POST","url":"http://x","body":{"query":"`+source+`","variables":{"first":$$1$$,"representations":[$$0$$]}}}`,
			renderedInput(t, node),
		)
	})

	t.Run("with operation-name suffix compensation", func(t *testing.T) {
		const namedSource = `query MyOp($representations: [_Any!]!){_entities(representations: $representations){__typename}}`
		node := renderInputFetch(t, 3, "MyOp", namedSource, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x"}, fragments, nil)
		(&renderSubgraphInputs{}).ProcessFetchTree(resolve.Sequence(node))
		require.Equal(t,
			`{"method":"POST","url":"http://x","body":{"query":"query MyOp__3($representations: [_Any!]!){_entities(representations: $representations){__typename}}","variables":{"representations":[$$0$$]}}}`,
			renderedInput(t, node),
		)
	})

	t.Run("deferred query plan is rendered and suffixed for survivors", func(t *testing.T) {
		const namedSource = `query MyOp($representations: [_Any!]!){_entities(representations: $representations){__typename}}`
		qp := &resolve.QueryPlan{DependsOnFields: []resolve.Representation{{Kind: resolve.RepresentationKindKey, TypeName: "Employee"}}}
		node := renderInputFetch(t, 3, "MyOp", namedSource, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x"}, fragments, qp)
		(&renderSubgraphInputs{}).ProcessFetchTree(resolve.Sequence(node))
		f := node.Item.Fetch.(*resolve.SingleFetch)
		require.Equal(t,
			`query MyOp__3($representations: [_Any!]!){
    _entities(representations: $representations){
        __typename
    }
}`, f.QueryPlan.Query)
		// DependsOnFields untouched.
		require.Len(t, f.QueryPlan.DependsOnFields, 1)
	})
}

func TestRenderSubgraphInputs_ClearsArtifactWithoutRenderingWhenInputPresent(t *testing.T) {
	// When the planner already printed the input eagerly (non-empty Input), the
	// stage keeps the bytes verbatim and only clears the artifact.
	node := resolve.Single(&resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{FetchID: 1},
		FetchConfiguration: resolve.FetchConfiguration{
			Input:             `{"q":"0"}`,
			SubgraphOperation: &resolve.SubgraphOperation{},
		},
	})
	(&renderSubgraphInputs{}).ProcessFetchTree(resolve.Sequence(node))
	f := node.Item.Fetch.(*resolve.SingleFetch)
	require.Equal(t, `{"q":"0"}`, f.Input)
	require.Nil(t, f.SubgraphOperation)
}
