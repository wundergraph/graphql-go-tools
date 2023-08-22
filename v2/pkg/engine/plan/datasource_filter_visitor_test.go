// BEGIN: 8f7e6d5a7b3c
package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestFilterDataSources(t *testing.T) {
	definition := unsafeparser.ParseGraphqlDocumentString("type Query {user: User} type User {age: Int  name: String}")
	_ = asttransform.MergeDefinitionWithBaseSchema(&definition)

	operation := unsafeparser.ParseGraphqlDocumentString("{ user {age} }")

	dataSources := []DataSourceConfiguration{
		{
			RootNodes: []TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"user"},
				},
			},
			ChildNodes: []TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"age"},
				},
			},
		},
		{
			RootNodes: []TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"name"},
				},
			},
		},
	}

	expectedDataSources := []DataSourceConfiguration{
		{
			RootNodes: []TypeField{
				{
					TypeName:   "Query",
					FieldNames: []string{"user"},
				},
			},
			ChildNodes: []TypeField{
				{
					TypeName:   "User",
					FieldNames: []string{"age"},
				},
			},
		},
	}

	report := operationreport.Report{}
	result := FilterDataSources(&operation, &definition, &report, dataSources)
	assert.False(t, report.HasErrors())
	assert.Equal(t, expectedDataSources, result)
}
