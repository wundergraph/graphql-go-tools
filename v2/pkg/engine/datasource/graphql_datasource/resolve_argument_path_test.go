package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestResolveArgumentPath(t *testing.T) {
	args := []resolve.FieldArgument{
		{
			Name:     "id",
			Variable: &resolve.ContextVariable{Path: []string{"a"}},
		},
		{
			Name:     "key",
			Variable: &resolve.ContextVariable{Path: []string{"b"}},
		},
	}

	t.Run("empty path returns unchanged", func(t *testing.T) {
		result := resolveArgumentPath(nil, args)
		assert.Nil(t, result)
	})

	t.Run("single element resolves to variable path", func(t *testing.T) {
		result := resolveArgumentPath([]string{"id"}, args)
		assert.Equal(t, []string{"a"}, result)
	})

	t.Run("unknown argument returns unchanged", func(t *testing.T) {
		result := resolveArgumentPath([]string{"unknown"}, args)
		assert.Equal(t, []string{"unknown"}, result)
	})

	t.Run("nested path resolves root and appends rest", func(t *testing.T) {
		result := resolveArgumentPath([]string{"key", "sellerId"}, args)
		assert.Equal(t, []string{"b", "sellerId"}, result)
	})

	t.Run("deeply nested path resolves root and appends rest", func(t *testing.T) {
		result := resolveArgumentPath([]string{"key", "address", "id"}, args)
		assert.Equal(t, []string{"b", "address", "id"}, result)
	})

	t.Run("nested path with unknown root returns unchanged", func(t *testing.T) {
		result := resolveArgumentPath([]string{"missing", "field"}, args)
		assert.Equal(t, []string{"missing", "field"}, result)
	})

	t.Run("non-context-variable returns original path", func(t *testing.T) {
		argsWithObjectVar := []resolve.FieldArgument{
			{
				Name:     "obj",
				Variable: &resolve.ObjectVariable{Path: []string{"x"}},
			},
		}
		result := resolveArgumentPath([]string{"obj", "field"}, argsWithObjectVar)
		assert.Equal(t, []string{"obj", "field"}, result)
	})
}
