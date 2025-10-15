package introspection_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func TestSource_Load(t *testing.T) {
	run := func(schema string, input string, fixtureName string) func(t *testing.T) {
		t.Helper()
		return func(t *testing.T) {
			def, report := astparser.ParseGraphqlDocumentString(schema)
			require.False(t, report.HasErrors())
			require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))

			var data introspection.Data
			gen := introspection.NewGenerator()
			gen.Generate(&def, &report, &data)
			require.False(t, report.HasErrors())

			source := &Source{introspectionData: &data}
			responseData, err := source.Load(context.Background(), []byte(input))
			require.NoError(t, err)

			actualResponse := &bytes.Buffer{}
			require.NoError(t, json.Indent(actualResponse, responseData, "", "  "))
			// Trim the trailing newline that json.Indent adds
			responseBytes := actualResponse.Bytes()
			if len(responseBytes) > 0 && responseBytes[len(responseBytes)-1] == '\n' {
				responseBytes = responseBytes[:len(responseBytes)-1]
			}
			goldie.Assert(t, fixtureName, responseBytes)
		}
	}

	t.Run("schema introspection", run(testSchema, `{"request_type":1}`, `schema_introspection`))
	t.Run("schema introspection with custom root operation types", run(testSchemaWithCustomRootOperationTypes, `{"request_type":1}`, `schema_introspection_with_custom_root_operation_types`))
	t.Run("type introspection", run(testSchema, `{"request_type":2,"type_name":"Query"}`, `type_introspection`))
	t.Run("type introspection of not existing type", run(testSchema, `{"request_type":2,"type_name":"NotExisting"}`, `not_existing_type`))
}

const testSchema = `
schema {
    query: Query
}

type Query {
    me: Droid @deprecated
    droid(id: ID!): Droid
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI @deprecated
}

type Droid {
    name: String!
}
`

const testSchemaWithCustomRootOperationTypes = `
schema {
    query: CustomQuery
	mutation: CustomMutation
	subscription: CustomSubscription
}

type CustomQuery {
    me: Droid @deprecated
    droid(id: ID!): Droid
}

type CustomMutation {
	destroyDroid(id: ID!): Boolean!
}

type CustomSubscription {
	destroyedDroid: Droid
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI @deprecated
}

type Droid {
    name: String!
}`
