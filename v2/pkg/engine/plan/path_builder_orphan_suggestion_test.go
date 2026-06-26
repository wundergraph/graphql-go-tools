package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestShouldRemoveDuplicateLeafAbstractFieldPathIgnoresOrphanedProvidedSuggestion(t *testing.T) {
	definition := unsafeparser.ParseGraphqlDocumentString(`
		type Query { user: User }
		type User { related: SearchResult }
		union SearchResult = Book | Movie
		type Book { title: String }
		type Movie { title: String }
	`)
	operation := unsafeparser.ParseGraphqlDocumentString(`query {
		user {
			... on User {
				related {
					... on Book { title }
				}
			}
		}
	}`)
	relatedFieldRef := pathBuilderTestFieldRef(t, &operation, "related")
	userNode, ok := definition.NodeByNameStr("User")
	assert.True(t, ok)

	currentPlanner := newPlannerPathsConfiguration("query", PlannerPathObject, []pathConfiguration{
		{
			parentPath:    "query.user.$0User",
			path:          "query.user.$0User.related",
			fieldRef:      relatedFieldRef,
			enclosingNode: userNode,
			pathType:      PathTypeField,
		},
	})
	otherPlanner := newPlannerPathsConfiguration("query", PlannerPathObject, []pathConfiguration{
		{
			parentPath:    "query.user.$0User",
			path:          "query.user.$0User.related",
			fieldRef:      relatedFieldRef,
			enclosingNode: userNode,
			pathType:      PathTypeField,
		},
		{
			parentPath:    "query.user.$0User.related.$0Book",
			path:          "query.user.$0User.related.$0Book.title",
			fieldRef:      pathBuilderTestFieldRef(t, &operation, "title"),
			enclosingNode: userNode,
			pathType:      PathTypeField,
		},
	})

	builder := &PathBuilder{
		visitor: &pathBuilderVisitor{
			operation:  &operation,
			definition: &definition,
			nodeSuggestions: newNodeSuggestions([]NodeSuggestion{
				{
					Path:       "query.user.$0User.related",
					FieldRef:   relatedFieldRef,
					IsProvided: true,
					IsOrphan:   true,
				},
			}),
			planners: []PlannerConfiguration{
				&plannerConfiguration[any]{plannerPathsConfiguration: currentPlanner},
				&plannerConfiguration[any]{plannerPathsConfiguration: otherPlanner},
			},
		},
	}

	shouldRemove := builder.shouldRemoveDuplicateLeafAbstractFieldPath(0, currentPlanner, &pathConfiguration{
		parentPath:    "query.user.$0User",
		path:          "query.user.$0User.related",
		fieldRef:      relatedFieldRef,
		enclosingNode: userNode,
		pathType:      PathTypeField,
	})

	assert.False(t, shouldRemove)
}

func pathBuilderTestFieldRef(t *testing.T, operation *ast.Document, fieldName string) int {
	t.Helper()
	for i := range operation.Fields {
		if operation.FieldNameString(i) == fieldName {
			return i
		}
	}
	t.Fatalf("field %q not found", fieldName)
	return ast.InvalidRef
}
