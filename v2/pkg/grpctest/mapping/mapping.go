package mapping

import (
	"testing"

	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
)

// DefaultGRPCMapping returns a hardcoded default mapping between GraphQL and Protobuf
func DefaultGRPCMapping() *grpcdatasource.GRPCMapping {
	return &grpcdatasource.GRPCMapping{
		Service: "ProductService",
		QueryRPCs: map[string]grpcdatasource.RPCConfig{
			"users": {
				RPC:      "QueryUsers",
				Request:  "QueryUsersRequest",
				Response: "QueryUsersResponse",
			},
			"user": {
				RPC:      "QueryUser",
				Request:  "QueryUserRequest",
				Response: "QueryUserResponse",
			},
			"nestedType": {
				RPC:      "QueryNestedType",
				Request:  "QueryNestedTypeRequest",
				Response: "QueryNestedTypeResponse",
			},
			"recursiveType": {
				RPC:      "QueryRecursiveType",
				Request:  "QueryRecursiveTypeRequest",
				Response: "QueryRecursiveTypeResponse",
			},
			"typeFilterWithArguments": {
				RPC:      "QueryTypeFilterWithArguments",
				Request:  "QueryTypeFilterWithArgumentsRequest",
				Response: "QueryTypeFilterWithArgumentsResponse",
			},
			"typeWithMultipleFilterFields": {
				RPC:      "QueryTypeWithMultipleFilterFields",
				Request:  "QueryTypeWithMultipleFilterFieldsRequest",
				Response: "QueryTypeWithMultipleFilterFieldsResponse",
			},
			"complexFilterType": {
				RPC:      "QueryComplexFilterType",
				Request:  "QueryComplexFilterTypeRequest",
				Response: "QueryComplexFilterTypeResponse",
			},
			"randomPet": {
				RPC:      "QueryRandomPet",
				Request:  "QueryRandomPetRequest",
				Response: "QueryRandomPetResponse",
			},
			"allPets": {
				RPC:      "QueryAllPets",
				Request:  "QueryAllPetsRequest",
				Response: "QueryAllPetsResponse",
			},
			"categories": {
				RPC:      "QueryCategories",
				Request:  "QueryCategoriesRequest",
				Response: "QueryCategoriesResponse",
			},
			"categoriesByKind": {
				RPC:      "QueryCategoriesByKind",
				Request:  "QueryCategoriesByKindRequest",
				Response: "QueryCategoriesByKindResponse",
			},
			"filterCategories": {
				RPC:      "QueryFilterCategories",
				Request:  "QueryFilterCategoriesRequest",
				Response: "QueryFilterCategoriesResponse",
			},
		},
		MutationRPCs: grpcdatasource.RPCConfigMap{
			"createProduct": {
				RPC:      "CreateProduct",
				Request:  "CreateProductRequest",
				Response: "CreateProductResponse",
			},
		},
		SubscriptionRPCs: grpcdatasource.RPCConfigMap{},
		EntityRPCs: map[string]grpcdatasource.EntityRPCConfig{
			"Product": {
				Key: "id",
				RPCConfig: grpcdatasource.RPCConfig{
					RPC:      "LookupProductById",
					Request:  "LookupProductByIdRequest",
					Response: "LookupProductByIdResponse",
				},
			},
			"Storage": {
				Key: "id",
				RPCConfig: grpcdatasource.RPCConfig{
					RPC:      "LookupStorageById",
					Request:  "LookupStorageByIdRequest",
					Response: "LookupStorageByIdResponse",
				},
			},
		},
		EnumValues: map[string][]grpcdatasource.EnumValueMapping{
			"CategoryKind": {
				{Value: "BOOK", TargetValue: "CATEGORY_KIND_BOOK"},
				{Value: "ELECTRONICS", TargetValue: "CATEGORY_KIND_ELECTRONICS"},
				{Value: "FURNITURE", TargetValue: "CATEGORY_KIND_FURNITURE"},
				{Value: "OTHER", TargetValue: "CATEGORY_KIND_OTHER"},
			},
		},
		Fields: map[string]grpcdatasource.FieldMap{
			"Query": {
				"user": {
					TargetName: "user",
					ArgumentMappings: map[string]string{
						"id": "id",
					},
				},
				"nestedType": {
					TargetName: "nested_type",
				},
				"recursiveType": {
					TargetName: "recursive_type",
				},
				"randomPet": {
					TargetName: "random_pet",
				},
				"allPets": {
					TargetName: "all_pets",
				},
				"categories": {
					TargetName: "categories",
				},
				"categoriesByKind": {
					TargetName: "categories_by_kind",
					ArgumentMappings: map[string]string{
						"kind": "kind",
					},
				},
				"filterCategories": {
					TargetName: "filter_categories",
					ArgumentMappings: map[string]string{
						"filter": "filter",
					},
				},
				"typeFilterWithArguments": {
					TargetName: "type_filter_with_arguments",
					ArgumentMappings: map[string]string{
						"filterField1": "filter_field_1",
						"filterField2": "filter_field_2",
					},
				},
				"typeWithMultipleFilterFields": {
					TargetName: "type_with_multiple_filter_fields",
					ArgumentMappings: map[string]string{
						"filter": "filter",
					},
				},
				"complexFilterType": {
					TargetName: "complex_filter_type",
					ArgumentMappings: map[string]string{
						"filter": "filter",
					},
				},
			},
			"Mutation": {
				"createProduct": {
					TargetName: "create_product",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
			},
			"ProductInput": {
				"name": {
					TargetName: "name",
				},
				"price": {
					TargetName: "price",
				},
			},
			"Product": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"price": {
					TargetName: "price",
				},
			},
			"Storage": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"location": {
					TargetName: "location",
				},
			},
			"User": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
			},
			"NestedTypeA": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"b": {
					TargetName: "b",
				},
			},
			"NestedTypeB": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"c": {
					TargetName: "c",
				},
			},
			"NestedTypeC": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
			},
			"RecursiveType": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"recursiveType": {
					TargetName: "recursive_type",
				},
			},
			"TypeWithMultipleFilterFields": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"filterField1": {
					TargetName: "filter_field_1",
				},
				"filterField2": {
					TargetName: "filter_field_2",
				},
			},
			"TypeWithComplexFilterInput": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
			},
			"Cat": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"kind": {
					TargetName: "kind",
				},
				"meowVolume": {
					TargetName: "meow_volume",
				},
			},
			"Dog": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"kind": {
					TargetName: "kind",
				},
				"barkVolume": {
					TargetName: "bark_volume",
				},
			},
			"Animal": {
				"cat": {
					TargetName: "cat",
				},
				"dog": {
					TargetName: "dog",
				},
			},
			"FilterType": {
				"name": {
					TargetName: "name",
				},
				"filterField1": {
					TargetName: "filter_field_1",
				},
				"filterField2": {
					TargetName: "filter_field_2",
				},
				"pagination": {
					TargetName: "pagination",
				},
			},
			"Pagination": {
				"page": {
					TargetName: "page",
				},
				"perPage": {
					TargetName: "per_page",
				},
			},
			"ComplexFilterTypeInput": {
				"filter": {
					TargetName: "filter",
				},
			},
			"Category": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"kind": {
					TargetName: "kind",
				},
			},
			"CategoryFilter": {
				"category": {
					TargetName: "category",
				},
				"pagination": {
					TargetName: "pagination",
				},
			},
		},
	}
}

func MustDefaultGRPCMapping(t *testing.T) *grpcdatasource.GRPCMapping {
	mapping := DefaultGRPCMapping()
	return mapping
}
