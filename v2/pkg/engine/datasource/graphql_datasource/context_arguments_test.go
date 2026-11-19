package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestContextVariableReusedByProjectionAndInputType(t *testing.T) {
	schema := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema("type Wallet { one(a: [String]): String two(b: [String]): String ids(c: [ID]): String }")
	op := unsafeparser.ParseGraphqlDocumentString("{ one two ids }")
	binding := func(field, argument, inputType string) plan.PlannedContextArgument {
		return plan.PlannedContextArgument{ContextArgument: plan.ContextArgument{TypeName: "Wallet", FieldName: field, ArgumentName: argument, ArgumentType: inputType}, Sources: []resolve.ContextSource{{Depth: 1, Selection: []resolve.ContextSelection{{Field: "locales"}}}}}
	}
	planner := Planner[Configuration]{visitor: &plan.Visitor{}, config: Configuration{schemaConfiguration: SchemaConfiguration{upstreamSchemaAst: &schema}}, upstreamOperation: &op, nodes: []ast.Node{{Kind: ast.NodeKindOperationDefinition, Ref: 0}},
		dataSourcePlannerConfig: plan.DataSourcePlannerConfiguration{ContextArguments: map[int][]plan.PlannedContextArgument{
			0: {binding("one", "a", "[String]")}, 1: {binding("two", "b", "[String]")}, 2: {binding("ids", "c", "[ID]")},
		}},
	}
	for ref := range op.Fields {
		planner.addFieldArguments(ref, ref, nil)
	}
	require.Len(t, planner.variables, 2)
	require.Len(t, op.VariableDefinitions, 2)
	argumentVariable := func(ref int) string {
		arg := op.FieldArguments(ref)[0]
		return op.VariableValueNameString(op.ArgumentValue(arg).Ref)
	}
	require.Equal(t, argumentVariable(0), argumentVariable(1))
	require.NotEqual(t, argumentVariable(0), argumentVariable(2))
}
