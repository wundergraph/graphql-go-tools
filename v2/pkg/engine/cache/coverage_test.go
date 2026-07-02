package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestCoversTypeConditionedFields pins that fields guarded by OnTypeNames are
// required only when the cached value's __typename matches — a concrete-type
// response must not miss coverage because of a SIBLING type's fields.
func TestCoversTypeConditionedFields(t *testing.T) {
	polymorphic := &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name:        []byte("name"),
				Value:       &resolve.Scalar{Nullable: false, Path: []string{"name"}},
				OnTypeNames: [][]byte{[]byte("Book"), []byte("Movie")},
			},
			{
				Name:        []byte("pages"),
				Value:       &resolve.Scalar{Nullable: false, Path: []string{"pages"}},
				OnTypeNames: [][]byte{[]byte("Book")},
			},
			{
				Name:        []byte("runtime"),
				Value:       &resolve.Scalar{Nullable: false, Path: []string{"runtime"}},
				OnTypeNames: [][]byte{[]byte("Movie")},
			},
		},
	}

	t.Run("a Book value covers without the Movie-only field", func(t *testing.T) {
		value := astjson.MustParseBytes([]byte(`{"__typename":"Book","name":"Dune","pages":412}`))
		assert.True(t, covers(nil, value, polymorphic))
	})

	t.Run("a Book value missing its OWN conditioned field does not cover", func(t *testing.T) {
		value := astjson.MustParseBytes([]byte(`{"__typename":"Book","name":"Dune"}`))
		assert.False(t, covers(nil, value, polymorphic))
	})

	t.Run("without __typename every conditioned field stays required (conservative)", func(t *testing.T) {
		value := astjson.MustParseBytes([]byte(`{"name":"Dune","pages":412}`))
		assert.False(t, covers(nil, value, polymorphic))
	})

	t.Run("unconditioned fields are unaffected", func(t *testing.T) {
		plain := &resolve.Object{Fields: []*resolve.Field{
			{Name: []byte("name"), Value: &resolve.Scalar{Nullable: false, Path: []string{"name"}}},
		}}
		assert.True(t, covers(nil, astjson.MustParseBytes([]byte(`{"__typename":"Book","name":"Dune"}`)), plain))
		assert.False(t, covers(nil, astjson.MustParseBytes([]byte(`{"__typename":"Book"}`)), plain))
	})
}
