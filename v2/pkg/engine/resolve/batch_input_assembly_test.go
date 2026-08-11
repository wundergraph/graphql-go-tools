package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	arena "github.com/wundergraph/go-arena"
)

// TestBatchInputAssembly pins the exact final input bytes for every keep
// shape: the header/footer bracket, the separator-joining of kept segments,
// and the undefined-variables post-processing on the final bytes.
func TestBatchInputAssembly(t *testing.T) {
	newAssembly := func() *batchInputAssembly {
		return &batchInputAssembly{
			header:    []byte(`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[`),
			separator: []byte(`,`),
			footer:    []byte(`]}}}`),
			segments: [][]byte{
				[]byte(`{"__typename":"Product","upc":"1"}`),
				[]byte(`{"__typename":"Product","upc":"2"}`),
				[]byte(`{"__typename":"Product","upc":"3"}`),
			},
		}
	}
	assemble := func(t *testing.T, b *batchInputAssembly, keep []bool) string {
		t.Helper()
		out, err := b.assemble(arena.NewMonotonicArena(), keep)
		require.NoError(t, err)
		return string(out)
	}

	t.Run("nil keep sends every segment", func(t *testing.T) {
		assert.Equal(t,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"},{"__typename":"Product","upc":"3"}]}}}`,
			assemble(t, newAssembly(), nil))
	})

	t.Run("all-true keep equals nil keep", func(t *testing.T) {
		assert.Equal(t,
			assemble(t, newAssembly(), nil),
			assemble(t, newAssembly(), []bool{true, true, true}))
	})

	t.Run("dropping the middle segment keeps exactly one separator", func(t *testing.T) {
		assert.Equal(t,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"3"}]}}}`,
			assemble(t, newAssembly(), []bool{true, false, true}))
	})

	t.Run("keeping only the first segment writes no separator", func(t *testing.T) {
		assert.Equal(t,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[{"__typename":"Product","upc":"1"}]}}}`,
			assemble(t, newAssembly(), []bool{true, false, false}))
	})

	t.Run("keeping only the last segment writes no separator", func(t *testing.T) {
		assert.Equal(t,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[{"__typename":"Product","upc":"3"}]}}}`,
			assemble(t, newAssembly(), []bool{false, false, true}))
	})

	t.Run("all-false keep leaves an empty representations list", func(t *testing.T) {
		assert.Equal(t,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[]}}}`,
			assemble(t, newAssembly(), []bool{false, false, false}))
	})

	t.Run("a keep shorter than the segments drops the unmarked tail", func(t *testing.T) {
		assert.Equal(t,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[{"__typename":"Product","upc":"1"}]}}}`,
			assemble(t, newAssembly(), []bool{true}))
	})

	t.Run("undefined variables are set on the FINAL bytes", func(t *testing.T) {
		b := newAssembly()
		b.undefinedVariables = []string{"first", "second"}
		assert.Equal(t,
			`{"undefined":["first","second"],"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[{"__typename":"Product","upc":"2"}]}}}`,
			assemble(t, b, []bool{false, true, false}))
	})

	t.Run("no segments renders the bare bracket", func(t *testing.T) {
		b := newAssembly()
		b.segments = nil
		assert.Equal(t,
			`{"method":"POST","url":"http://reviews","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations)}","variables":{"representations":[]}}}`,
			assemble(t, b, nil))
	})
}
