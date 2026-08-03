package resolve

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
)

func parseOp(t *testing.T, input string) *ast.Document {
	t.Helper()
	doc, report := astparser.ParseGraphqlDocumentString(input)
	require.False(t, report.HasErrors())
	return &doc
}

func artifactFetchConfig(doc *ast.Document, envelope SubgraphRequestEnvelope, vars []SubgraphVariable) *FetchConfiguration {
	return &FetchConfiguration{
		RequiresEntityBatchFetch: true,
		SubgraphOperation: &SubgraphOperation{
			Document:  doc,
			Variables: vars,
			Envelope:  envelope,
		},
	}
}

func TestFetchConfiguration_Equals_Artifact(t *testing.T) {
	const src = `query($representations: [_Any!]!){_entities(representations: $representations){__typename}}`
	env := SubgraphRequestEnvelope{Method: "POST", URL: "http://x", Header: []byte(`{"Auth":["s"]}`)}
	vars := []SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}}

	t.Run("identical documents/envelopes/variables are equal", func(t *testing.T) {
		a := artifactFetchConfig(parseOp(t, src), env, vars)
		b := artifactFetchConfig(parseOp(t, src), env, vars)
		require.True(t, a.Equals(b))
		require.True(t, b.Equals(a))
	})

	t.Run("differing query is not equal", func(t *testing.T) {
		a := artifactFetchConfig(parseOp(t, src), env, vars)
		b := artifactFetchConfig(parseOp(t, `query($representations: [_Any!]!){_entities(representations: $representations){id}}`), env, vars)
		require.False(t, a.Equals(b))
		require.False(t, b.Equals(a))
	})

	t.Run("differing envelope url is not equal", func(t *testing.T) {
		a := artifactFetchConfig(parseOp(t, src), env, vars)
		b := artifactFetchConfig(parseOp(t, src), SubgraphRequestEnvelope{Method: "POST", URL: "http://y", Header: env.Header}, vars)
		require.False(t, a.Equals(b))
	})

	t.Run("differing header presence is not equal", func(t *testing.T) {
		a := artifactFetchConfig(parseOp(t, src), env, vars)
		b := artifactFetchConfig(parseOp(t, src), SubgraphRequestEnvelope{Method: "POST", URL: "http://x"}, vars)
		require.False(t, a.Equals(b))
	})

	t.Run("differing variable order is not equal", func(t *testing.T) {
		twoA := []SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}}
		twoB := []SubgraphVariable{{Name: "first", Value: []byte("$$1$$")}, {Name: "representations", Value: []byte("[$$0$$]")}}
		a := artifactFetchConfig(parseOp(t, src), env, twoA)
		b := artifactFetchConfig(parseOp(t, src), env, twoB)
		require.False(t, a.Equals(b))
	})

	t.Run("differing variable name is not equal", func(t *testing.T) {
		a := artifactFetchConfig(parseOp(t, src), env, []SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}})
		b := artifactFetchConfig(parseOp(t, src), env, []SubgraphVariable{{Name: "reps", Value: []byte("[$$0$$]")}})
		require.False(t, a.Equals(b))
	})

	t.Run("differing variable value is not equal", func(t *testing.T) {
		a := artifactFetchConfig(parseOp(t, src), env, []SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}})
		b := artifactFetchConfig(parseOp(t, src), env, []SubgraphVariable{{Name: "representations", Value: []byte("[$$1$$]")}})
		require.False(t, a.Equals(b))
	})

	t.Run("artifact versus printed input is not equal", func(t *testing.T) {
		a := artifactFetchConfig(parseOp(t, src), env, vars)
		b := &FetchConfiguration{RequiresEntityBatchFetch: true, Input: `{"method":"POST"}`}
		require.False(t, a.Equals(b))
		require.False(t, b.Equals(a))
	})

	t.Run("cached print is reused (both sides cached afterwards)", func(t *testing.T) {
		a := artifactFetchConfig(parseOp(t, src), env, vars)
		b := artifactFetchConfig(parseOp(t, src), env, vars)
		require.True(t, a.Equals(b))
		require.True(t, a.SubgraphOperation.printedQueryCached)
		require.True(t, b.SubgraphOperation.printedQueryCached)
	})
}

func TestFetchConfiguration_Equals_PrintedInputUnchanged(t *testing.T) {
	// Both sides have printed Input (today's path): byte comparison, unaffected
	// by the presence of a SubgraphOperation artifact.
	a := &FetchConfiguration{Input: `{"a":1}`, RequiresEntityFetch: true}
	b := &FetchConfiguration{Input: `{"a":1}`, RequiresEntityFetch: true}
	require.True(t, a.Equals(b))

	c := &FetchConfiguration{Input: `{"a":2}`, RequiresEntityFetch: true}
	require.False(t, a.Equals(c))
}

func TestSubgraphOperation_PrintedQuery(t *testing.T) {
	const src = `query($representations: [_Any!]!){_entities(representations: $representations){__typename}}`

	t.Run("unseeded prints the document compactly and caches", func(t *testing.T) {
		op := &SubgraphOperation{Document: parseOp(t, src)}
		printed, err := op.PrintedQuery()
		require.NoError(t, err)
		require.Equal(t, src, string(printed))
		require.True(t, op.printedQueryCached)
	})

	t.Run("seeded bytes are returned verbatim", func(t *testing.T) {
		op := &SubgraphOperation{Document: parseOp(t, src)}
		op.SetPrintedQuery([]byte(`{seeded}`))
		printed, err := op.PrintedQuery()
		require.NoError(t, err)
		require.Equal(t, `{seeded}`, string(printed))
	})

	t.Run("a seeded empty print is cached, not re-printed", func(t *testing.T) {
		op := &SubgraphOperation{Document: parseOp(t, src)}
		op.SetPrintedQuery(nil)
		printed, err := op.PrintedQuery()
		require.NoError(t, err)
		require.Nil(t, printed)
	})
}
