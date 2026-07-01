package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestAddFieldArgumentsImportsVariableDefinitionsForRawCopiedArguments(t *testing.T) {
	operation := unsafeparser.ParseGraphqlDocumentString(`query RawArgs($id: ID!, $term: String = "all") {
		product(id: $id, filter: { term: $term }) {
			name
		}
	}`)
	operationDefinitionRef := operation.RootNodes[0].Ref
	downstreamFieldRef := graphqlDatasourceTestFieldRef(t, &operation, "product")

	upstreamOperation := ast.NewDocument()
	upstreamSelectionSet := upstreamOperation.AddSelectionSet()
	upstreamOperationNode := upstreamOperation.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
		OperationType: ast.OperationTypeQuery,
		SelectionSet:  upstreamSelectionSet.Ref,
		HasSelections: true,
	})
	upstreamField := upstreamOperation.AddField(ast.Field{
		Name: upstreamOperation.Input.AppendInputString("product"),
	})
	upstreamOperation.AddSelection(upstreamSelectionSet.Ref, ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  upstreamField.Ref,
	})

	walker := astvisitor.NewWalker(4)
	walker.Ancestors = append(walker.Ancestors, ast.Node{
		Kind: ast.NodeKindOperationDefinition,
		Ref:  operationDefinitionRef,
	})

	planner := &Planner[Configuration]{
		visitor: &plan.Visitor{
			Operation: &operation,
			Walker:    &walker,
		},
		upstreamOperation:                  upstreamOperation,
		nodes:                              []ast.Node{upstreamOperationNode},
		addDirectivesToVariableDefinitions: map[int][]int{},
	}

	planner.addFieldArguments(upstreamField.Ref, downstreamFieldRef, nil)

	got, err := astprinter.PrintString(upstreamOperation)
	require.NoError(t, err)
	assert.Equal(t, `query($id: ID!, $term: String = "all"){product(id: $id, filter: {term: $term})}`, got)
	assert.Equal(t, `{"term":$$1$$,"id":$$0$$}`, string(planner.upstreamVariables))
	assert.Len(t, planner.variables, 2)
}

func graphqlDatasourceTestFieldRef(t *testing.T, operation *ast.Document, fieldName string) int {
	t.Helper()
	for i := range operation.Fields {
		if operation.FieldNameString(i) == fieldName {
			return i
		}
	}
	t.Fatalf("field %q not found", fieldName)
	return ast.InvalidRef
}
