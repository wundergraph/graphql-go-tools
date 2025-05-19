package grpctest

import (
	"embed"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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

func GraphQLSchema() (ast.Document, error) {
	schemaBytes, err := getSchemaBytes()
	if err != nil {
		return ast.Document{}, fmt.Errorf("failed to get schema bytes: %w", err)
	}

	doc, report := astparser.ParseGraphqlDocumentBytes(schemaBytes)
	if report.HasErrors() {
		return ast.Document{}, fmt.Errorf("failed to parse schema: %w", report)
	}

	if err := asttransform.MergeDefinitionWithBaseSchema(&doc); err != nil {
		return ast.Document{}, fmt.Errorf("failed to merge schema: %w", err)
	}

	astvalidation.DefaultDefinitionValidator().Validate(&doc, &report)
	if report.HasErrors() {
		return ast.Document{}, fmt.Errorf("failed to validate schema: %w", report)
	}

	return doc, nil
}

func GraphQLSchemaWithoutBaseDefinitions() (ast.Document, error) {
	schemaBytes, err := getSchemaBytes()
	if err != nil {
		return ast.Document{}, fmt.Errorf("failed to get schema bytes: %w", err)
	}

	doc, report := astparser.ParseGraphqlDocumentBytes(schemaBytes)
	if report.HasErrors() {
		return ast.Document{}, fmt.Errorf("failed to parse schema: %w", report)
	}

	return doc, nil
}

func MustGraphQLSchema(t *testing.T) ast.Document {
	schemaBytes, err := getSchemaBytes()
	require.NoError(t, err)
	doc := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(string(schemaBytes))

	report := &operationreport.Report{}
	astvalidation.DefaultDefinitionValidator().Validate(&doc, report)
	if report.HasErrors() {
		t.Fatalf("failed to validate schema: %s", report.Error())
	}

	return doc
}

func ProtoSchema() (string, error) {
	protoBytes, err := getProtoBytes()
	if err != nil {
		return "", fmt.Errorf("failed to read proto file: %w", err)
	}

	return string(protoBytes), nil
}

func MustProtoSchema(t *testing.T) string {
	schema, err := ProtoSchema()
	require.NoError(t, err)
	return schema
}

func GetFieldConfigurations() plan.FieldConfigurations {
	return plan.FieldConfigurations{
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
		{
			TypeName:  "Query",
			FieldName: "categoriesByKind",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "kind",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "categoriesByKinds",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "kinds",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "filterCategories",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "filter",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "calculateTotals",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "orders",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "createUser",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
	}
}

func GetDataSourceMetadata() *plan.DataSourceMetadata {
	return &plan.DataSourceMetadata{
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
					"categories",
					"categoriesByKind",
					"filterCategories",
					"randomPet",
					"allPets",
				},
			},
			{
				TypeName: "Mutation",
				FieldNames: []string{
					"createUser",
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
			{
				TypeName: "Category",
				FieldNames: []string{
					"id",
					"name",
					"kind",
				},
			},
			{
				TypeName: "CategoryFilter",
				FieldNames: []string{
					"category",
					"pagination",
				},
			},
			{
				TypeName: "CategoryKind",
				FieldNames: []string{
					"BOOK",
					"ELECTRONICS",
					"FURNITURE",
					"OTHER",
				},
			},
			{
				TypeName: "Animal",
				FieldNames: []string{
					"id",
					"name",
					"kind",
				},
			},
			{
				TypeName: "Cat",
				FieldNames: []string{
					"id",
					"name",
					"kind",
					"meowVolume",
				},
			},
			{
				TypeName: "Dog",
				FieldNames: []string{
					"id",
					"name",
					"kind",
					"barkVolume",
				},
			},
			{
				TypeName: "UserInput",
				FieldNames: []string{
					"name",
				},
			},
		},
	}
}
