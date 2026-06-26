package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
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

func TestFullRequestedUnionMemberTypeNamesMergesRepeatedPathMembers(t *testing.T) {
	definition := unsafeparser.ParseGraphqlDocumentString(`
		type Query { search: SearchResult }
		union SearchResult = Book | Movie | Album
		type Book { title: String }
		type Movie { title: String }
		type Album { title: String }
	`)
	operation := unsafeparser.ParseGraphqlDocumentString(`query {
		book: search { ... on Book { title } }
		movie: search { ... on Movie { title } }
	}`)
	unionNode, ok := definition.NodeByNameStr("SearchResult")
	assert.True(t, ok)
	bookFieldRef := planTestFieldRef(t, &operation, "book")
	movieFieldRef := planTestFieldRef(t, &operation, "movie")

	filter := &DataSourceFilter{
		operation:  &operation,
		definition: &definition,
		nodes: newNodeSuggestions([]NodeSuggestion{
			{Path: "query.search", FieldRef: bookFieldRef},
			{Path: "query.search", FieldRef: movieFieldRef},
		}),
	}

	first := filter.fullRequestedUnionMemberTypeNames(0, unionNode.Ref)
	second := filter.fullRequestedUnionMemberTypeNames(1, unionNode.Ref)

	assert.Equal(t, []string{"Book"}, first)
	assert.Equal(t, []string{"Book", "Movie"}, second)
	assert.Equal(t, map[string][]string{
		"query.search": {"Book", "Movie"},
	}, filter.abstractFieldRequestedMembers)
}

func planTestFieldRef(t *testing.T, operation *ast.Document, aliasOrName string) int {
	t.Helper()
	for i := range operation.Fields {
		if operation.FieldAliasOrNameString(i) == aliasOrName {
			return i
		}
	}
	t.Fatalf("field %q not found", aliasOrName)
	return ast.InvalidRef
}
