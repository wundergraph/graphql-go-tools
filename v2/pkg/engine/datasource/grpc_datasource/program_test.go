package grpcdatasource

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

func TestCompileProgram(t *testing.T) {
	t.Parallel()

	type expected struct {
		stageCount int
	}

	tests := []struct {
		name      string
		operation string
		expected  expected
		err       error
	}{
		// {
		// 	name:      "simple program",
		// 	operation: `query UsersWithTypename { users { __typename id __typename name } }`,
		// },
		{
			name:      "query with field resolver",
			operation: `query CategoriesWithFieldResolvers($whoop: ProductCountFilter) { categories { id productCount(filters: $whoop) } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)
			// Parse the GraphQL query
			queryDoc, report := astparser.ParseGraphqlDocumentString(tt.operation)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}

			rpcPlanVisitor := newRPCPlanVisitor(rpcPlanVisitorConfig{
				subgraphName: "Products",
				mapping:      testMapping(),
			})

			plan, err := rpcPlanVisitor.PlanOperation(&queryDoc, &schemaDoc)
			require.NoError(t, err)

			compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
			require.NoError(t, err)

			runtime, err := newSchemaRuntime(compiler.doc)
			require.NoError(t, err)

			program, err := compileProgram(plan, runtime)
			require.NoError(t, err)

			fmt.Println("program", program)
		})
	}
}
