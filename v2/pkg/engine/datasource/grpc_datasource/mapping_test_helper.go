package grpcdatasource

func testMapping() *GRPCMapping {
	return &GRPCMapping{
		Service: "Products",
		QueryRPCs: map[string]RPCConfig{
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
			"calculateTotals": {
				RPC:      "QueryCalculateTotals",
				Request:  "QueryCalculateTotalsRequest",
				Response: "QueryCalculateTotalsResponse",
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
			"categoriesByKinds": {
				RPC:      "QueryCategoriesByKinds",
				Request:  "QueryCategoriesByKindsRequest",
				Response: "QueryCategoriesByKindsResponse",
			},
			"filterCategories": {
				RPC:      "QueryFilterCategories",
				Request:  "QueryFilterCategoriesRequest",
				Response: "QueryFilterCategoriesResponse",
			},
			"randomSearchResult": {
				RPC:      "QueryRandomSearchResult",
				Request:  "QueryRandomSearchResultRequest",
				Response: "QueryRandomSearchResultResponse",
			},
			"search": {
				RPC:      "QuerySearch",
				Request:  "QuerySearchRequest",
				Response: "QuerySearchResponse",
			},
		},
		MutationRPCs: RPCConfigMap{
			"createUser": {
				RPC:      "MutationCreateUser",
				Request:  "MutationCreateUserRequest",
				Response: "MutationCreateUserResponse",
			},
			"performAction": {
				RPC:      "MutationPerformAction",
				Request:  "MutationPerformActionRequest",
				Response: "MutationPerformActionResponse",
			},
		},
		SubscriptionRPCs: RPCConfigMap{},
		EntityRPCs: map[string]EntityRPCConfig{
			"Product": {
				Key: "id",
				RPCConfig: RPCConfig{
					RPC:      "LookupProductById",
					Request:  "LookupProductByIdRequest",
					Response: "LookupProductByIdResponse",
				},
			},
			"Storage": {
				Key: "id",
				RPCConfig: RPCConfig{
					RPC:      "LookupStorageById",
					Request:  "LookupStorageByIdRequest",
					Response: "LookupStorageByIdResponse",
				},
			},
		},
		EnumValues: map[string][]EnumValueMapping{
			"CategoryKind": {
				{Value: "BOOK", TargetValue: "CATEGORY_KIND_BOOK"},
				{Value: "ELECTRONICS", TargetValue: "CATEGORY_KIND_ELECTRONICS"},
				{Value: "FURNITURE", TargetValue: "CATEGORY_KIND_FURNITURE"},
				{Value: "OTHER", TargetValue: "CATEGORY_KIND_OTHER"},
			},
		},
		Fields: map[string]FieldMap{
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
				"categoriesByKinds": {
					TargetName: "categories_by_kinds",
					ArgumentMappings: map[string]string{
						"kinds": "kinds",
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
				"calculateTotals": {
					TargetName: "calculate_totals",
					ArgumentMappings: map[string]string{
						"orders": "orders",
					},
				},
				"search": {
					TargetName: "search",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
				"randomSearchResult": {
					TargetName: "random_search_result",
				},
			},
			"Mutation": {
				"createUser": {
					TargetName: "create_user",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
				"performAction": {
					TargetName: "perform_action",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
			},
			"UserInput": {
				"name": {
					TargetName: "name",
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
			"Order": {
				"orderId": {
					TargetName: "order_id",
				},
				"customerName": {
					TargetName: "customer_name",
				},
				"totalItems": {
					TargetName: "total_items",
				},
				"orderLines": {
					TargetName: "order_lines",
				},
			},
			"OrderLine": {
				"productId": {
					TargetName: "product_id",
				},
				"quantity": {
					TargetName: "quantity",
				},
				"modifiers": {
					TargetName: "modifiers",
				},
			},
			"OrderInput": {
				"orderId": {
					TargetName: "order_id",
				},
				"customerName": {
					TargetName: "customer_name",
				},
				"lines": {
					TargetName: "lines",
				},
			},
			"OrderLineInput": {
				"productId": {
					TargetName: "product_id",
				},
				"quantity": {
					TargetName: "quantity",
				},
				"modifiers": {
					TargetName: "modifiers",
				},
			},
			"ActionSuccess": {
				"message": {
					TargetName: "message",
				},
				"timestamp": {
					TargetName: "timestamp",
				},
			},
			"ActionError": {
				"message": {
					TargetName: "message",
				},
				"code": {
					TargetName: "code",
				},
			},
			"SearchInput": {
				"query": {
					TargetName: "query",
				},
				"limit": {
					TargetName: "limit",
				},
			},
			"ActionInput": {
				"type": {
					TargetName: "type",
				},
				"payload": {
					TargetName: "payload",
				},
			},
			"SearchResult": {
				"product": {
					TargetName: "product",
				},
				"user": {
					TargetName: "user",
				},
				"category": {
					TargetName: "category",
				},
			},
			"ActionResult": {
				"actionSuccess": {
					TargetName: "action_success",
				},
				"actionError": {
					TargetName: "action_error",
				},
			},
		},
	}
}
