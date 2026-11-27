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
	schema := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema("type Wallet { one(a: [String]): String two(b: [String]): String ids(c: [ID]): String strict(d: [String!]): String single(e: String): String }")
	op := unsafeparser.ParseGraphqlDocumentString("{ one two ids strict single }")
	config := Configuration{schemaConfiguration: SchemaConfiguration{upstreamSchemaAst: &schema}}
	metadata := &plan.DataSourceMetadata{}
	for _, target := range []struct{ field, argument string }{{"one", "a"}, {"two", "b"}, {"ids", "c"}, {"strict", "d"}, {"single", "e"}} {
		metadata.ContextArguments = append(metadata.ContextArguments, plan.ContextArgument{
			TypeName: "Wallet", FieldName: target.field, ArgumentName: target.argument,
		})
	}
	ds, err := plan.NewDataSourceConfiguration("wallet", &Factory[Configuration]{}, metadata, config)
	require.NoError(t, err)
	bindings := make(map[int][]plan.PlannedContextArgument)
	for i, argument := range ds.FederationConfiguration().ContextArguments {
		bindings[i] = []plan.PlannedContextArgument{{
			ContextArgument: argument,
			Sources:         []resolve.ContextSource{{Depth: 1, Selection: []resolve.ContextSelection{{Field: "locales"}}}},
		}}
	}
	planner := Planner[Configuration]{visitor: &plan.Visitor{}, config: config, upstreamOperation: &op, nodes: []ast.Node{{Kind: ast.NodeKindOperationDefinition, Ref: 0}},
		dataSourcePlannerConfig: plan.DataSourcePlannerConfiguration{ContextArguments: bindings},
	}
	for ref := range op.Fields {
		planner.addFieldArguments(ref, ref, nil)
	}
	require.Len(t, planner.variables, 4)
	require.Len(t, op.VariableDefinitions, 4)
	argumentVariable := func(ref int) string {
		arg := op.FieldArguments(ref)[0]
		return op.VariableValueNameString(op.ArgumentValue(arg).Ref)
	}
	require.Equal(t, argumentVariable(0), argumentVariable(1))
	for i, want := range []string{"[String]", "[ID]", "[String!]", "String"} {
		got, err := op.PrintTypeBytes(op.VariableDefinitions[i].Type, nil)
		require.NoError(t, err)
		require.Equal(t, want, string(got))
	}
	require.NotEqual(t, argumentVariable(0), argumentVariable(2))
	require.NotEqual(t, argumentVariable(0), argumentVariable(3))
	require.NotEqual(t, argumentVariable(0), argumentVariable(4))
}
