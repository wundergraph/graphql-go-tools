package ast

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNodeKindIsAbstractType(t *testing.T) {
	t.Run("Interface type returns true", func(t *testing.T) {
		assert.Equal(t, NodeKindInterfaceTypeDefinition.IsAbstractType(), true)
	})

	t.Run("Union type returns true", func(t *testing.T) {
		assert.Equal(t, NodeKindUnionTypeDefinition.IsAbstractType(), true)
	})

	t.Run("Enum type returns false", func(t *testing.T) {
		assert.Equal(t, NodeKindEnumTypeDefinition.IsAbstractType(), false)
	})

	t.Run("Interface type returns false", func(t *testing.T) {
		assert.Equal(t, NodeKindObjectTypeDefinition.IsAbstractType(), false)
	})

	t.Run("Scalar type returns false", func(t *testing.T) {
		assert.Equal(t, NodeKindScalarTypeDefinition.IsAbstractType(), false)
	})
}
