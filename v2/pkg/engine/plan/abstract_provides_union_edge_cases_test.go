package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestIsProvidedFieldUsesInterfaceFragmentStrippedPath(t *testing.T) {
	visitor := &collectNodesDSVisitor{
		providesEntries: map[string]struct{}{
			providedFieldKey("SomeInterface", "providedField", "query.node.providedField"): {},
		},
	}

	got := visitor.isProvidedField(fieldInfo{
		typeName:                    "SomeInterface",
		fieldName:                   "providedField",
		currentPath:                 "query.node.$0SomeInterface.providedField",
		currentPathWithoutFragments: "query.node.providedField",
		onFragment:                  true,
		onInterfaceFragment:         true,
		enclosingTypeDefinition: ast.Node{
			Kind: ast.NodeKindInterfaceTypeDefinition,
			Ref:  0,
		},
	})

	assert.True(t, got)
}
