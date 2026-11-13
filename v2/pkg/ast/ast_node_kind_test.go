package ast

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeKindTypeCategories(t *testing.T) {
	for _, tc := range []struct {
		kind      NodeKind
		abstract  bool
		composite bool
	}{
		{NodeKindInterfaceTypeDefinition, true, true},
		{NodeKindUnionTypeDefinition, true, true},
		{NodeKindObjectTypeDefinition, false, true},
		{NodeKindScalarTypeDefinition, false, false},
		{NodeKindEnumTypeDefinition, false, false},
		{NodeKindInputObjectTypeDefinition, false, false},
		{NodeKindUnknown, false, false},
	} {
		assert.Equal(t, tc.abstract, tc.kind.IsAbstractType(), "%s.IsAbstractType", tc.kind)
		assert.Equal(t, tc.composite, tc.kind.IsCompositeType(), "%s.IsCompositeType", tc.kind)
	}
}
