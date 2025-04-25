package grpctest

import (
	"embed"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

//go:embed testdata product.proto
var grpcTestData embed.FS

func getSchemaBytes() ([]byte, error) {
	graphqlBytes, err := grpcTestData.ReadFile("testdata/products.graphqls")
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}
	return graphqlBytes, nil
}

func getProtoBytes() ([]byte, error) {
	protoBytes, err := grpcTestData.ReadFile("product.proto")
	if err != nil {
		return nil, fmt.Errorf("failed to read proto file: %w", err)
	}
	return protoBytes, nil
}

func GraphQLSchema(t *testing.T) *graphql.Schema {
	schemaBytes, err := getSchemaBytes()
	require.NoError(t, err)
	require.NotEmpty(t, schemaBytes, "graphql schema is empty")

	schema, err := graphql.NewSchemaFromBytes(schemaBytes)
	require.NoError(t, err)

	return schema
}

func ProtoSchema(t *testing.T) string {
	protoBytes, err := getProtoBytes()
	require.NoError(t, err)

	return string(protoBytes)
}

var FieldConfigurations plan.FieldConfigurations = plan.FieldConfigurations{
	{
		TypeName:  "Query",
		FieldName: "user",
		Arguments: []plan.ArgumentConfiguration{
			{
				Name:       "id",
				SourceType: plan.FieldArgumentSource,
			},
		},
	},
	{
		TypeName:  "Query",
		FieldName: "typeFilterWithArguments",
		Arguments: []plan.ArgumentConfiguration{
			{
				Name:       "filterField1",
				SourceType: plan.FieldArgumentSource,
			},
			{
				Name:       "filterField2",
				SourceType: plan.FieldArgumentSource,
			},
		},
	},
	{
		TypeName:  "Query",
		FieldName: "typeWithMultipleFilterFields",
		Arguments: []plan.ArgumentConfiguration{
			{
				Name:       "filter",
				SourceType: plan.FieldArgumentSource,
			},
		},
	},
	{
		TypeName:  "Query",
		FieldName: "complexFilterType",
		Arguments: []plan.ArgumentConfiguration{
			{
				Name:       "filter",
				SourceType: plan.FieldArgumentSource,
			},
		},
	},
}

var DataSourceMetadata = &plan.DataSourceMetadata{
	RootNodes: plan.TypeFields{
		{
			TypeName: "Product",
			FieldNames: []string{
				"id",
				"name",
				"price",
			},
		},
		{
			TypeName: "Storage",
			FieldNames: []string{
				"id",
				"name",
				"location",
			},
		},
		{
			TypeName: "Query",
			FieldNames: []string{
				"users",
				"user",
				"nestedType",
				"recursiveType",
				"typeFilterWithArguments",
				"typeWithMultipleFilterFields",
				"complexFilterType",
			},
		},
	},
	ChildNodes: plan.TypeFields{
		{
			TypeName: "User",
			FieldNames: []string{
				"id",
				"name",
			},
		},
		{
			TypeName: "NestedTypeA",
			FieldNames: []string{
				"id",
				"name",
				"b",
			},
		},
		{
			TypeName: "NestedTypeB",
			FieldNames: []string{
				"id",
				"name",
				"c",
			},
		},
		{
			TypeName: "NestedTypeC",
			FieldNames: []string{
				"id",
				"name",
			},
		},
		{
			TypeName: "RecursiveType",
			FieldNames: []string{
				"id",
				"name",
				"recursiveType",
			},
		},
		{
			TypeName: "TypeWithMultipleFilterFields",
			FieldNames: []string{
				"id",
				"name",
				"filterField1",
				"filterField2",
			},
		},
		{
			TypeName: "TypeWithComplexFilterInput",
			FieldNames: []string{
				"id",
				"name",
			},
		},
	},
}
