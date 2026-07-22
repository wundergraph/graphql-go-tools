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

func MustGraphQLSchema(t testing.TB) ast.Document {
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

func MustProtoSchema(t testing.TB) string {
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
			FieldName: "calculateTotals",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "orders",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "category",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "id",
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
			FieldName: "search",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "nullableFieldsTypeById",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "id",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "nullableFieldsTypeWithFilter",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "filter",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "blogPostById",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "id",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "blogPostsWithFilter",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "filter",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "authorById",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "id",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "authorsWithFilter",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "filter",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "bulkSearchAuthors",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "filters",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "bulkSearchBlogPosts",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "filters",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "testContainer",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "id",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Query",
			FieldName: "conditionalSearch",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "conditions",
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
		{
			TypeName:  "Mutation",
			FieldName: "performAction",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "createNullableFieldsType",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "updateNullableFieldsType",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "id",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "createBlogPost",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "updateBlogPost",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "id",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "createAuthor",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "updateAuthor",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "id",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "bulkCreateAuthors",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "authors",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "bulkUpdateAuthors",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "authors",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "bulkCreateBlogPosts",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "blogPosts",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "bulkUpdateBlogPosts",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "blogPosts",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Product",
			FieldName: "shippingEstimate",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "input",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Product",
			FieldName: "recommendedCategory",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "maxPrice",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Product",
			FieldName: "mascotRecommendation",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "includeDetails",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Product",
			FieldName: "stockStatus",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "checkAvailability",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Product",
			FieldName: "productDetails",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "includeExtended",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Storage",
			FieldName: "storageStatus",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "checkHealth",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Storage",
			FieldName: "linkedStorages",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "depth",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Storage",
			FieldName: "nearbyStorages",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "radius",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Storage",
			FieldName: "filteredTagSummary",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "prefix",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Storage",
			FieldName: "multiFilteredTagSummary",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "prefixes",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "maxResults",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Storage",
			FieldName: "nullableFilteredTagSummary",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "prefix",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Category",
			FieldName: "productCount",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "filters",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Category",
			FieldName: "popularityScore",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "threshold",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Category",
			FieldName: "categoryMetrics",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "metricType",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Category",
			FieldName: "mascot",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "includeVolume",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Category",
			FieldName: "categoryStatus",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "checkHealth",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Category",
			FieldName: "childCategories",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "include",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Category",
			FieldName: "optionalCategories",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "include",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Subcategory",
			FieldName: "itemCount",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "filters",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Subcategory",
			FieldName: "featuredCategory",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "includeChildren",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "CategoryMetrics",
			FieldName: "normalizedScore",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "baseline",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "CategoryMetrics",
			FieldName: "relatedCategory",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "include",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "TestContainer",
			FieldName: "details",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "includeExtended",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
	}
}

func GetDataSourceMetadata() *plan.DataSourceMetadata {
	return &plan.DataSourceMetadata{
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Warehouse",
					SelectionSet: "id",
				},
			},
			Requires: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					FieldName:    "stockHealthScore",
					SelectionSet: "itemCount restockData { lastRestockDate }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalTagSummary",
					SelectionSet: "optionalTags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedMetadata",
					SelectionSet: "metadata { capacity zone priority }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalProcessedMetadata",
					SelectionSet: "metadata { capacity zone }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedTags",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalProcessedTags",
					SelectionSet: "optionalTags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedMetadataHistory",
					SelectionSet: "metadataHistory { capacity zone }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "kindSummary",
					SelectionSet: "storageKind",
				},
				{
					TypeName:     "Storage",
					FieldName:    "categoryInfoSummary",
					SelectionSet: "categoryInfo { kind name }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "itemInfo",
					SelectionSet: "primaryItem { ... on PalletItem { __typename name palletCount } ... on ContainerItem { __typename name containerSize } }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "operationReport",
					SelectionSet: "lastStorageOperation { ... on StorageSuccess { __typename message completedAt } ... on StorageFailure { __typename message errorCode } }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "securitySummary",
					SelectionSet: "securitySetup { securityLevel primaryItem { ... on PalletItem { __typename name palletCount } ... on ContainerItem { __typename name containerSize } } }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "itemHandlerInfo",
					SelectionSet: "primaryItem { ... on PalletItem { __typename handler { name } } ... on ContainerItem { __typename handler { name } } }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "itemSpecsInfo",
					SelectionSet: "primaryItem { ... on PalletItem { __typename specs { name dimensions { length width } } } ... on ContainerItem { __typename specs { name dimensions { length width } } } }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "deepItemInfo",
					SelectionSet: "primaryItem { ... on PalletItem { __typename handler { assignedItem { ... on ContainerItem { __typename name containerSize } ... on PalletItem { __typename name palletCount } } } } ... on ContainerItem { __typename handler { name } } }",
				},
				{
					TypeName:     "Storage",
					FieldName:    "filteredTagSummary",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "multiFilteredTagSummary",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "nullableFilteredTagSummary",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Warehouse",
					FieldName:    "stockHealthScore",
					SelectionSet: "inventoryCount restockData { lastRestockDate }",
				},
			},
		},
		RootNodes: plan.TypeFields{
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
					"calculateTotals",
					"categories",
					"category",
					"categoriesByKind",
					"categoriesByKinds",
					"filterCategories",
					"randomPet",
					"allPets",
					"search",
					"randomSearchResult",
					"nullableFieldsType",
					"nullableFieldsTypeById",
					"nullableFieldsTypeWithFilter",
					"allNullableFieldsTypes",
					"blogPost",
					"blogPostById",
					"blogPostsWithFilter",
					"allBlogPosts",
					"author",
					"authorById",
					"authorsWithFilter",
					"allAuthors",
					"bulkSearchAuthors",
					"bulkSearchBlogPosts",
					"testContainer",
					"testContainers",
					"conditionalSearch",
				},
			},
			{
				TypeName: "Mutation",
				FieldNames: []string{
					"createUser",
					"performAction",
					"createNullableFieldsType",
					"updateNullableFieldsType",
					"createBlogPost",
					"updateBlogPost",
					"createAuthor",
					"updateAuthor",
					"bulkCreateAuthors",
					"bulkUpdateAuthors",
					"bulkCreateBlogPosts",
					"bulkUpdateBlogPosts",
				},
			},
			{
				TypeName: "Product",
				FieldNames: []string{
					"id",
					"name",
					"price",
					"shippingEstimate",
					"recommendedCategory",
					"mascotRecommendation",
					"stockStatus",
					"productDetails",
				},
			},
			{
				TypeName: "Storage",
				FieldNames: []string{
					"id",
					"name",
					"location",
					"stockHealthScore",
					"tagSummary",
					"optionalTagSummary",
					"metadataScore",
					"processedMetadata",
					"optionalProcessedMetadata",
					"processedTags",
					"optionalProcessedTags",
					"processedMetadataHistory",
					"kindSummary",
					"categoryInfoSummary",
					"itemInfo",
					"operationReport",
					"securitySummary",
					"itemHandlerInfo",
					"itemSpecsInfo",
					"deepItemInfo",
					"storageStatus",
					"linkedStorages",
					"nearbyStorages",
					"filteredTagSummary",
					"multiFilteredTagSummary",
					"nullableFilteredTagSummary",
				},
				ExternalFieldNames: []string{
					"itemCount",
					"restockData",
					"tags",
					"optionalTags",
					"metadata",
					"metadataHistory",
					"storageKind",
					"categoryInfo",
					"primaryItem",
					"lastStorageOperation",
					"securitySetup",
				},
			},
			{
				TypeName: "Warehouse",
				FieldNames: []string{
					"id",
					"name",
					"location",
					"stockHealthScore",
				},
				ExternalFieldNames: []string{
					"inventoryCount",
					"restockData",
				},
			},
		},
		ChildNodes: plan.TypeFields{
			{
				TypeName: "Product",
				FieldNames: []string{
					"id",
					"name",
					"price",
					"shippingEstimate",
					"recommendedCategory",
					"mascotRecommendation",
					"stockStatus",
					"productDetails",
				},
			},
			{
				TypeName: "ProductDetails",
				FieldNames: []string{
					"id",
					"description",
					"reviewSummary",
					"recommendedPet",
				},
			},
			{
				TypeName: "Storage",
				FieldNames: []string{
					"id",
					"name",
					"location",
					"stockHealthScore",
					"tagSummary",
					"optionalTagSummary",
					"metadataScore",
					"processedMetadata",
					"optionalProcessedMetadata",
					"processedTags",
					"optionalProcessedTags",
					"processedMetadataHistory",
					"kindSummary",
					"categoryInfoSummary",
					"itemInfo",
					"operationReport",
					"securitySummary",
					"itemHandlerInfo",
					"itemSpecsInfo",
					"deepItemInfo",
					"storageStatus",
					"linkedStorages",
					"nearbyStorages",
					"filteredTagSummary",
					"multiFilteredTagSummary",
					"nullableFilteredTagSummary",
				},
				ExternalFieldNames: []string{
					"itemCount",
					"restockData",
					"tags",
					"optionalTags",
					"metadata",
					"metadataHistory",
					"storageKind",
					"categoryInfo",
					"primaryItem",
					"lastStorageOperation",
					"securitySetup",
				},
			},
			{
				TypeName: "Warehouse",
				FieldNames: []string{
					"id",
					"name",
					"location",
					"stockHealthScore",
				},
				ExternalFieldNames: []string{
					"inventoryCount",
					"restockData",
				},
			},
			{
				TypeName: "RestockData",
				FieldNames: []string{
					"lastRestockDate",
				},
			},
			{
				TypeName: "StorageMetadata",
				FieldNames: []string{
					"capacity",
					"zone",
					"priority",
				},
			},
			{
				TypeName: "StorageCategoryInfo",
				FieldNames: []string{
					"kind",
					"name",
				},
			},
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
				TypeName: "FilterTypeInput",
				FieldNames: []string{
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
				TypeName: "FilterType",
				FieldNames: []string{
					"name",
					"filterField1",
					"filterField2",
					"pagination",
				},
			},
			{
				TypeName: "Pagination",
				FieldNames: []string{
					"page",
					"perPage",
				},
			},
			{
				TypeName: "ComplexFilterTypeInput",
				FieldNames: []string{
					"filter",
				},
			},
			{
				TypeName: "OrderLineInput",
				FieldNames: []string{
					"productId",
					"quantity",
					"modifiers",
				},
			},
			{
				TypeName: "OrderInput",
				FieldNames: []string{
					"orderId",
					"customerName",
					"lines",
				},
			},
			{
				TypeName: "Order",
				FieldNames: []string{
					"orderId",
					"customerName",
					"totalItems",
					"orderLines",
				},
			},
			{
				TypeName: "OrderLine",
				FieldNames: []string{
					"productId",
					"quantity",
					"modifiers",
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
				TypeName: "Category",
				FieldNames: []string{
					"id",
					"name",
					"kind",
					"productCount",
					"subcategories",
					"popularityScore",
					"categoryMetrics",
					"mascot",
					"categoryStatus",
					"childCategories",
					"optionalCategories",
					"nullMetrics",
					"totalProducts",
					"topSubcategory",
					"activeSubcategories",
				},
			},
			{
				TypeName: "Subcategory",
				FieldNames: []string{
					"id",
					"name",
					"description",
					"isActive",
					"itemCount",
					"featuredCategory",
					"parentCategory",
				},
			},
			{
				TypeName: "CategoryMetrics",
				FieldNames: []string{
					"id",
					"metricType",
					"value",
					"timestamp",
					"categoryId",
					"normalizedScore",
					"relatedCategory",
					"averageScore",
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
					"owner",
					"breed",
				},
			},
			{
				TypeName: "Dog",
				FieldNames: []string{
					"id",
					"name",
					"kind",
					"barkVolume",
					"owner",
					"breed",
				},
			},
			{
				TypeName: "Owner",
				FieldNames: []string{
					"id",
					"name",
					"contact",
					"pet",
				},
			},
			{
				TypeName: "ContactInfo",
				FieldNames: []string{
					"email",
					"phone",
					"address",
				},
			},
			{
				TypeName: "Address",
				FieldNames: []string{
					"street",
					"city",
					"country",
					"zipCode",
				},
			},
			{
				TypeName: "CatBreed",
				FieldNames: []string{
					"id",
					"name",
					"origin",
					"characteristics",
				},
			},
			{
				TypeName: "DogBreed",
				FieldNames: []string{
					"id",
					"name",
					"origin",
					"characteristics",
				},
			},
			{
				TypeName: "BreedCharacteristics",
				FieldNames: []string{
					"size",
					"temperament",
					"lifespan",
				},
			},
			{
				TypeName: "StorageItem",
				FieldNames: []string{
					"id",
					"name",
					"weight",
				},
			},
			{
				TypeName: "PalletItem",
				FieldNames: []string{
					"id",
					"name",
					"weight",
					"palletCount",
					"handler",
					"specs",
				},
			},
			{
				TypeName: "ContainerItem",
				FieldNames: []string{
					"id",
					"name",
					"weight",
					"containerSize",
					"handler",
					"specs",
				},
			},
			{
				TypeName: "ItemHandler",
				FieldNames: []string{
					"id",
					"name",
					"assignedItem",
				},
			},
			{
				TypeName: "PalletSpecs",
				FieldNames: []string{
					"name",
					"maxWeight",
					"dimensions",
				},
			},
			{
				TypeName: "ContainerSpecs",
				FieldNames: []string{
					"name",
					"volume",
					"dimensions",
				},
			},
			{
				TypeName: "Dimensions",
				FieldNames: []string{
					"length",
					"width",
					"height",
				},
			},
			{
				TypeName: "StorageSuccess",
				FieldNames: []string{
					"message",
					"completedAt",
				},
			},
			{
				TypeName: "StorageFailure",
				FieldNames: []string{
					"message",
					"errorCode",
				},
			},
			{
				TypeName: "SecuritySetup",
				FieldNames: []string{
					"securityLevel",
					"primaryItem",
				},
			},
			{
				TypeName: "ActionSuccess",
				FieldNames: []string{
					"message",
					"timestamp",
				},
			},
			{
				TypeName: "ActionError",
				FieldNames: []string{
					"message",
					"code",
				},
			},
			{
				TypeName: "TestContainer",
				FieldNames: []string{
					"id",
					"name",
					"description",
					"details",
				},
			},
			{
				TypeName: "TestDetails",
				FieldNames: []string{
					"id",
					"summary",
					"pet",
					"status",
				},
			},
			{
				TypeName: "SearchInput",
				FieldNames: []string{
					"query",
					"limit",
				},
			},
			{
				TypeName: "ActionInput",
				FieldNames: []string{
					"type",
					"payload",
				},
			},
			{
				TypeName: "NullableFieldsType",
				FieldNames: []string{
					"id",
					"name",
					"optionalString",
					"optionalInt",
					"optionalFloat",
					"optionalBoolean",
					"requiredString",
					"requiredInt",
				},
			},
			{
				TypeName: "BlogPost",
				FieldNames: []string{
					"id",
					"title",
					"content",
					"tags",
					"optionalTags",
					"categories",
					"keywords",
					"viewCounts",
					"ratings",
					"isPublished",
					"tagGroups",
					"relatedTopics",
					"commentThreads",
					"suggestions",
					"relatedCategories",
					"contributors",
					"mentionedProducts",
					"mentionedUsers",
					"categoryGroups",
					"contributorTeams",
				},
			},
			{
				TypeName: "Author",
				FieldNames: []string{
					"id",
					"name",
					"email",
					"skills",
					"languages",
					"socialLinks",
					"teamsByProject",
					"collaborations",
					"writtenPosts",
					"favoriteCategories",
					"relatedAuthors",
					"productReviews",
					"authorGroups",
					"categoryPreferences",
					"projectTeams",
				},
			},
			{
				TypeName: "BlogPostInput",
				FieldNames: []string{
					"title",
					"content",
					"tags",
					"optionalTags",
					"categories",
					"keywords",
					"viewCounts",
					"ratings",
					"isPublished",
					"tagGroups",
					"relatedTopics",
					"commentThreads",
					"suggestions",
					"relatedCategories",
					"contributors",
					"categoryGroups",
				},
			},
			{
				TypeName: "AuthorInput",
				FieldNames: []string{
					"name",
					"email",
					"skills",
					"languages",
					"socialLinks",
					"teamsByProject",
					"collaborations",
					"favoriteCategories",
					"authorGroups",
					"projectTeams",
				},
			},
			{
				TypeName: "BlogPostFilter",
				FieldNames: []string{
					"title",
					"hasCategories",
					"minTags",
				},
			},
			{
				TypeName: "AuthorFilter",
				FieldNames: []string{
					"name",
					"hasTeams",
					"skillCount",
				},
			},
			{
				TypeName: "NullableFieldsInput",
				FieldNames: []string{
					"name",
					"optionalString",
					"optionalInt",
					"optionalFloat",
					"optionalBoolean",
					"requiredString",
					"requiredInt",
				},
			},
			{
				TypeName: "NullableFieldsFilter",
				FieldNames: []string{
					"name",
					"optionalString",
					"includeNulls",
				},
			},
			{
				TypeName: "CategoryInput",
				FieldNames: []string{
					"name",
					"kind",
				},
			},
			{
				TypeName: "ProductCountFilter",
				FieldNames: []string{
					"minPrice",
					"maxPrice",
					"inStock",
					"searchTerm",
				},
			},
			{
				TypeName: "SubcategoryItemFilter",
				FieldNames: []string{
					"minPrice",
					"maxPrice",
					"inStock",
					"isActive",
					"searchTerm",
				},
			},
			{
				TypeName: "ShippingDestination",
				FieldNames: []string{
					"DOMESTIC",
					"EXPRESS",
					"INTERNATIONAL",
				},
			},
			{
				TypeName: "ShippingEstimateInput",
				FieldNames: []string{
					"destination",
					"weight",
					"expedited",
				},
			},
			{
				TypeName: "UserInput",
				FieldNames: []string{
					"name",
				},
			},
			{
				TypeName: "ConditionsInput",
				FieldNames: []string{
					"and",
					"or",
					"key",
					"value",
				},
			},
			{
				TypeName: "ConditionalSearchResult",
				FieldNames: []string{
					"id",
					"name",
					"matchedConditions",
				},
			},
		},
	}
}
