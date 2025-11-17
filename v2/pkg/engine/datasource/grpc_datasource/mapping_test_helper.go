package grpcdatasource

func testMapping() *GRPCMapping {
	return &GRPCMapping{
		Service: "Products",
		QueryRPCs: RPCConfigMap[RPCConfig]{
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
			"search": {
				RPC:      "QuerySearch",
				Request:  "QuerySearchRequest",
				Response: "QuerySearchResponse",
			},
			"randomSearchResult": {
				RPC:      "QueryRandomSearchResult",
				Request:  "QueryRandomSearchResultRequest",
				Response: "QueryRandomSearchResultResponse",
			},
			"nullableFieldsType": {
				RPC:      "QueryNullableFieldsType",
				Request:  "QueryNullableFieldsTypeRequest",
				Response: "QueryNullableFieldsTypeResponse",
			},
			"nullableFieldsTypeById": {
				RPC:      "QueryNullableFieldsTypeById",
				Request:  "QueryNullableFieldsTypeByIdRequest",
				Response: "QueryNullableFieldsTypeByIdResponse",
			},
			"nullableFieldsTypeWithFilter": {
				RPC:      "QueryNullableFieldsTypeWithFilter",
				Request:  "QueryNullableFieldsTypeWithFilterRequest",
				Response: "QueryNullableFieldsTypeWithFilterResponse",
			},
			"allNullableFieldsTypes": {
				RPC:      "QueryAllNullableFieldsTypes",
				Request:  "QueryAllNullableFieldsTypesRequest",
				Response: "QueryAllNullableFieldsTypesResponse",
			},
			"blogPost": {
				RPC:      "QueryBlogPost",
				Request:  "QueryBlogPostRequest",
				Response: "QueryBlogPostResponse",
			},
			"blogPostById": {
				RPC:      "QueryBlogPostById",
				Request:  "QueryBlogPostByIdRequest",
				Response: "QueryBlogPostByIdResponse",
			},
			"blogPostsWithFilter": {
				RPC:      "QueryBlogPostsWithFilter",
				Request:  "QueryBlogPostsWithFilterRequest",
				Response: "QueryBlogPostsWithFilterResponse",
			},
			"allBlogPosts": {
				RPC:      "QueryAllBlogPosts",
				Request:  "QueryAllBlogPostsRequest",
				Response: "QueryAllBlogPostsResponse",
			},
			"author": {
				RPC:      "QueryAuthor",
				Request:  "QueryAuthorRequest",
				Response: "QueryAuthorResponse",
			},
			"authorById": {
				RPC:      "QueryAuthorById",
				Request:  "QueryAuthorByIdRequest",
				Response: "QueryAuthorByIdResponse",
			},
			"authorsWithFilter": {
				RPC:      "QueryAuthorsWithFilter",
				Request:  "QueryAuthorsWithFilterRequest",
				Response: "QueryAuthorsWithFilterResponse",
			},
			"allAuthors": {
				RPC:      "QueryAllAuthors",
				Request:  "QueryAllAuthorsRequest",
				Response: "QueryAllAuthorsResponse",
			},
			"bulkSearchAuthors": {
				RPC:      "QueryBulkSearchAuthors",
				Request:  "QueryBulkSearchAuthorsRequest",
				Response: "QueryBulkSearchAuthorsResponse",
			},
			"bulkSearchBlogPosts": {
				RPC:      "QueryBulkSearchBlogPosts",
				Request:  "QueryBulkSearchBlogPostsRequest",
				Response: "QueryBulkSearchBlogPostsResponse",
			},
			"testContainer": {
				RPC:      "QueryTestContainer",
				Request:  "QueryTestContainerRequest",
				Response: "QueryTestContainerResponse",
			},
			"testContainers": {
				RPC:      "QueryTestContainers",
				Request:  "QueryTestContainersRequest",
				Response: "QueryTestContainersResponse",
			},
		},
		MutationRPCs: RPCConfigMap[RPCConfig]{
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
			"createNullableFieldsType": {
				RPC:      "MutationCreateNullableFieldsType",
				Request:  "MutationCreateNullableFieldsTypeRequest",
				Response: "MutationCreateNullableFieldsTypeResponse",
			},
			"updateNullableFieldsType": {
				RPC:      "MutationUpdateNullableFieldsType",
				Request:  "MutationUpdateNullableFieldsTypeRequest",
				Response: "MutationUpdateNullableFieldsTypeResponse",
			},
			"createBlogPost": {
				RPC:      "MutationCreateBlogPost",
				Request:  "MutationCreateBlogPostRequest",
				Response: "MutationCreateBlogPostResponse",
			},
			"updateBlogPost": {
				RPC:      "MutationUpdateBlogPost",
				Request:  "MutationUpdateBlogPostRequest",
				Response: "MutationUpdateBlogPostResponse",
			},
			"createAuthor": {
				RPC:      "MutationCreateAuthor",
				Request:  "MutationCreateAuthorRequest",
				Response: "MutationCreateAuthorResponse",
			},
			"updateAuthor": {
				RPC:      "MutationUpdateAuthor",
				Request:  "MutationUpdateAuthorRequest",
				Response: "MutationUpdateAuthorResponse",
			},
			"bulkCreateAuthors": {
				RPC:      "MutationBulkCreateAuthors",
				Request:  "MutationBulkCreateAuthorsRequest",
				Response: "MutationBulkCreateAuthorsResponse",
			},
			"bulkUpdateAuthors": {
				RPC:      "MutationBulkUpdateAuthors",
				Request:  "MutationBulkUpdateAuthorsRequest",
				Response: "MutationBulkUpdateAuthorsResponse",
			},
			"bulkCreateBlogPosts": {
				RPC:      "MutationBulkCreateBlogPosts",
				Request:  "MutationBulkCreateBlogPostsRequest",
				Response: "MutationBulkCreateBlogPostsResponse",
			},
			"bulkUpdateBlogPosts": {
				RPC:      "MutationBulkUpdateBlogPosts",
				Request:  "MutationBulkUpdateBlogPostsRequest",
				Response: "MutationBulkUpdateBlogPostsResponse",
			},
		},
		SubscriptionRPCs: RPCConfigMap[RPCConfig]{},
		ResolveRPCs: RPCConfigMap[ResolveRPCMapping]{
			"Category": {
				"productCount": {
					FieldMappingData: FieldMapData{
						TargetName: "product_count",
						ArgumentMappings: FieldArgumentMap{
							"filters": "filters",
						},
					},
					RPC:      "ResolveCategoryProductCount",
					Request:  "ResolveCategoryProductCountRequest",
					Response: "ResolveCategoryProductCountResponse",
				},
				"popularityScore": {
					FieldMappingData: FieldMapData{
						TargetName: "popularity_score",
						ArgumentMappings: FieldArgumentMap{
							"threshold": "threshold",
						},
					},
					RPC:      "ResolveCategoryPopularityScore",
					Request:  "ResolveCategoryPopularityScoreRequest",
					Response: "ResolveCategoryPopularityScoreResponse",
				},
				"categoryMetrics": {
					FieldMappingData: FieldMapData{
						TargetName: "category_metrics",
						ArgumentMappings: FieldArgumentMap{
							"metricType": "metric_type",
						},
					},
					RPC:      "ResolveCategoryCategoryMetrics",
					Request:  "ResolveCategoryCategoryMetricsRequest",
					Response: "ResolveCategoryCategoryMetricsResponse",
				},
				"mascot": {
					FieldMappingData: FieldMapData{
						TargetName: "mascot",
						ArgumentMappings: FieldArgumentMap{
							"includeVolume": "include_volume",
						},
					},
					RPC:      "ResolveCategoryMascot",
					Request:  "ResolveCategoryMascotRequest",
					Response: "ResolveCategoryMascotResponse",
				},
				"categoryStatus": {
					FieldMappingData: FieldMapData{
						TargetName: "category_status",
						ArgumentMappings: FieldArgumentMap{
							"checkHealth": "check_health",
						},
					},
					RPC:      "ResolveCategoryCategoryStatus",
					Request:  "ResolveCategoryCategoryStatusRequest",
					Response: "ResolveCategoryCategoryStatusResponse",
				},
			},
			"CategoryMetrics": {
				"normalizedScore": {
					FieldMappingData: FieldMapData{
						TargetName: "normalized_score",
						ArgumentMappings: FieldArgumentMap{
							"baseline": "baseline",
						},
					},
					RPC:      "ResolveCategoryMetricsNormalizedScore",
					Request:  "ResolveCategoryMetricsNormalizedScoreRequest",
					Response: "ResolveCategoryMetricsNormalizedScoreResponse",
				},
			},
			"Product": {
				"shippingEstimate": {
					FieldMappingData: FieldMapData{
						TargetName: "shipping_estimate",
						ArgumentMappings: FieldArgumentMap{
							"input": "input",
						},
					},
					RPC:      "ResolveProductShippingEstimate",
					Request:  "ResolveProductShippingEstimateRequest",
					Response: "ResolveProductShippingEstimateResponse",
				},
				"recommendedCategory": {
					FieldMappingData: FieldMapData{
						TargetName: "recommended_category",
						ArgumentMappings: FieldArgumentMap{
							"maxPrice": "max_price",
						},
					},
					RPC:      "ResolveProductRecommendedCategory",
					Request:  "ResolveProductRecommendedCategoryRequest",
					Response: "ResolveProductRecommendedCategoryResponse",
				},
				"mascotRecommendation": {
					FieldMappingData: FieldMapData{
						TargetName: "mascot_recommendation",
						ArgumentMappings: FieldArgumentMap{
							"includeDetails": "include_details",
						},
					},
					RPC:      "ResolveProductMascotRecommendation",
					Request:  "ResolveProductMascotRecommendationRequest",
					Response: "ResolveProductMascotRecommendationResponse",
				},
				"stockStatus": {
					FieldMappingData: FieldMapData{
						TargetName: "stock_status",
						ArgumentMappings: FieldArgumentMap{
							"checkAvailability": "check_availability",
						},
					},
					RPC:      "ResolveProductStockStatus",
					Request:  "ResolveProductStockStatusRequest",
					Response: "ResolveProductStockStatusResponse",
				},
				"productDetails": {
					FieldMappingData: FieldMapData{
						TargetName: "product_details",
						ArgumentMappings: FieldArgumentMap{
							"includeExtended": "include_extended",
						},
					},
					RPC:      "ResolveProductProductDetails",
					Request:  "ResolveProductProductDetailsRequest",
					Response: "ResolveProductProductDetailsResponse",
				},
			},
			"Subcategory": {
				"itemCount": {
					FieldMappingData: FieldMapData{
						TargetName: "item_count",
						ArgumentMappings: FieldArgumentMap{
							"filters": "filters",
						},
					},
					RPC:      "ResolveSubcategoryItemCount",
					Request:  "ResolveSubcategoryItemCountRequest",
					Response: "ResolveSubcategoryItemCountResponse",
				},
			},
			"TestContainer": {
				"details": {
					FieldMappingData: FieldMapData{
						TargetName: "details",
						ArgumentMappings: FieldArgumentMap{
							"includeExtended": "include_extended",
						},
					},
					RPC:      "ResolveTestContainerDetails",
					Request:  "ResolveTestContainerDetailsRequest",
					Response: "ResolveTestContainerDetailsResponse",
				},
			},
		},
		EntityRPCs: map[string][]EntityRPCConfig{
			"Product": {
				{
					Key: "id",
					RPCConfig: RPCConfig{
						RPC:      "LookupProductById",
						Request:  "LookupProductByIdRequest",
						Response: "LookupProductByIdResponse",
					},
				},
			},
			"Storage": {
				{
					Key: "id",
					RPCConfig: RPCConfig{
						RPC:      "LookupStorageById",
						Request:  "LookupStorageByIdRequest",
						Response: "LookupStorageByIdResponse",
					},
				},
			},
			"Warehouse": {
				{
					Key: "id",
					RPCConfig: RPCConfig{
						RPC:      "LookupWarehouseById",
						Request:  "LookupWarehouseByIdRequest",
						Response: "LookupWarehouseByIdResponse",
					},
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
			"ShippingDestination": {
				{Value: "DOMESTIC", TargetValue: "SHIPPING_DESTINATION_DOMESTIC"},
				{Value: "EXPRESS", TargetValue: "SHIPPING_DESTINATION_EXPRESS"},
				{Value: "INTERNATIONAL", TargetValue: "SHIPPING_DESTINATION_INTERNATIONAL"},
			},
		},
		Fields: map[string]FieldMap{
			"Query": {
				"users": {
					TargetName: "users",
				},
				"user": {
					TargetName: "user",
					ArgumentMappings: FieldArgumentMap{
						"id": "id",
					},
				},
				"nestedType": {
					TargetName: "nested_type",
				},
				"recursiveType": {
					TargetName: "recursive_type",
				},
				"typeFilterWithArguments": {
					TargetName: "type_filter_with_arguments",
					ArgumentMappings: FieldArgumentMap{
						"filterField1": "filter_field_1",
						"filterField2": "filter_field_2",
					},
				},
				"typeWithMultipleFilterFields": {
					TargetName: "type_with_multiple_filter_fields",
					ArgumentMappings: FieldArgumentMap{
						"filter": "filter",
					},
				},
				"complexFilterType": {
					TargetName: "complex_filter_type",
					ArgumentMappings: FieldArgumentMap{
						"filter": "filter",
					},
				},
				"calculateTotals": {
					TargetName: "calculate_totals",
					ArgumentMappings: FieldArgumentMap{
						"orders": "orders",
					},
				},
				"categories": {
					TargetName: "categories",
				},
				"categoriesByKind": {
					TargetName: "categories_by_kind",
					ArgumentMappings: FieldArgumentMap{
						"kind": "kind",
					},
				},
				"categoriesByKinds": {
					TargetName: "categories_by_kinds",
					ArgumentMappings: FieldArgumentMap{
						"kinds": "kinds",
					},
				},
				"filterCategories": {
					TargetName: "filter_categories",
					ArgumentMappings: FieldArgumentMap{
						"filter": "filter",
					},
				},
				"randomPet": {
					TargetName: "random_pet",
				},
				"allPets": {
					TargetName: "all_pets",
				},
				"search": {
					TargetName: "search",
					ArgumentMappings: FieldArgumentMap{
						"input": "input",
					},
				},
				"randomSearchResult": {
					TargetName: "random_search_result",
				},
				"nullableFieldsType": {
					TargetName: "nullable_fields_type",
				},
				"nullableFieldsTypeById": {
					TargetName: "nullable_fields_type_by_id",
					ArgumentMappings: FieldArgumentMap{
						"id": "id",
					},
				},
				"nullableFieldsTypeWithFilter": {
					TargetName: "nullable_fields_type_with_filter",
					ArgumentMappings: FieldArgumentMap{
						"filter": "filter",
					},
				},
				"allNullableFieldsTypes": {
					TargetName: "all_nullable_fields_types",
				},
				"blogPost": {
					TargetName: "blog_post",
				},
				"blogPostById": {
					TargetName: "blog_post_by_id",
					ArgumentMappings: FieldArgumentMap{
						"id": "id",
					},
				},
				"blogPostsWithFilter": {
					TargetName: "blog_posts_with_filter",
					ArgumentMappings: FieldArgumentMap{
						"filter": "filter",
					},
				},
				"allBlogPosts": {
					TargetName: "all_blog_posts",
				},
				"author": {
					TargetName: "author",
				},
				"authorById": {
					TargetName: "author_by_id",
					ArgumentMappings: FieldArgumentMap{
						"id": "id",
					},
				},
				"authorsWithFilter": {
					TargetName: "authors_with_filter",
					ArgumentMappings: FieldArgumentMap{
						"filter": "filter",
					},
				},
				"allAuthors": {
					TargetName: "all_authors",
				},
				"bulkSearchAuthors": {
					TargetName: "bulk_search_authors",
					ArgumentMappings: FieldArgumentMap{
						"filters": "filters",
					},
				},
				"bulkSearchBlogPosts": {
					TargetName: "bulk_search_blog_posts",
					ArgumentMappings: FieldArgumentMap{
						"filters": "filters",
					},
				},
				"testContainer": {
					TargetName: "test_container",
					ArgumentMappings: FieldArgumentMap{
						"id": "id",
					},
				},
				"testContainers": {
					TargetName: "test_containers",
				},
			},
			"Mutation": {
				"createUser": {
					TargetName: "create_user",
					ArgumentMappings: FieldArgumentMap{
						"input": "input",
					},
				},
				"performAction": {
					TargetName: "perform_action",
					ArgumentMappings: FieldArgumentMap{
						"input": "input",
					},
				},
				"createNullableFieldsType": {
					TargetName: "create_nullable_fields_type",
					ArgumentMappings: FieldArgumentMap{
						"input": "input",
					},
				},
				"updateNullableFieldsType": {
					TargetName: "update_nullable_fields_type",
					ArgumentMappings: FieldArgumentMap{
						"id":    "id",
						"input": "input",
					},
				},
				"createBlogPost": {
					TargetName: "create_blog_post",
					ArgumentMappings: FieldArgumentMap{
						"input": "input",
					},
				},
				"updateBlogPost": {
					TargetName: "update_blog_post",
					ArgumentMappings: FieldArgumentMap{
						"id":    "id",
						"input": "input",
					},
				},
				"createAuthor": {
					TargetName: "create_author",
					ArgumentMappings: FieldArgumentMap{
						"input": "input",
					},
				},
				"updateAuthor": {
					TargetName: "update_author",
					ArgumentMappings: FieldArgumentMap{
						"id":    "id",
						"input": "input",
					},
				},
				"bulkCreateAuthors": {
					TargetName: "bulk_create_authors",
					ArgumentMappings: FieldArgumentMap{
						"authors": "authors",
					},
				},
				"bulkUpdateAuthors": {
					TargetName: "bulk_update_authors",
					ArgumentMappings: FieldArgumentMap{
						"authors": "authors",
					},
				},
				"bulkCreateBlogPosts": {
					TargetName: "bulk_create_blog_posts",
					ArgumentMappings: FieldArgumentMap{
						"blogPosts": "blog_posts",
					},
				},
				"bulkUpdateBlogPosts": {
					TargetName: "bulk_update_blog_posts",
					ArgumentMappings: FieldArgumentMap{
						"blogPosts": "blog_posts",
					},
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
				"shippingEstimate": {
					TargetName: "shipping_estimate",
					ArgumentMappings: FieldArgumentMap{
						"input": "input",
					},
				},
				"recommendedCategory": {
					TargetName: "recommended_category",
					ArgumentMappings: FieldArgumentMap{
						"maxPrice": "max_price",
					},
				},
				"mascotRecommendation": {
					TargetName: "mascot_recommendation",
					ArgumentMappings: FieldArgumentMap{
						"includeDetails": "include_details",
					},
				},
				"stockStatus": {
					TargetName: "stock_status",
					ArgumentMappings: FieldArgumentMap{
						"checkAvailability": "check_availability",
					},
				},
				"productDetails": {
					TargetName: "product_details",
					ArgumentMappings: FieldArgumentMap{
						"includeExtended": "include_extended",
					},
				},
			},
			"ProductDetails": {
				"id": {
					TargetName: "id",
				},
				"description": {
					TargetName: "description",
				},
				"reviewSummary": {
					TargetName: "review_summary",
				},
				"recommendedPet": {
					TargetName: "recommended_pet",
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
			"Warehouse": {
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
			"FilterTypeInput": {
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
			"CategoryFilter": {
				"category": {
					TargetName: "category",
				},
				"pagination": {
					TargetName: "pagination",
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
				"productCount": {
					TargetName: "product_count",
					ArgumentMappings: FieldArgumentMap{
						"filters": "filters",
					},
				},
				"subcategories": {
					TargetName: "subcategories",
				},
				"popularityScore": {
					TargetName: "popularity_score",
					ArgumentMappings: FieldArgumentMap{
						"threshold": "threshold",
					},
				},
				"categoryMetrics": {
					TargetName: "category_metrics",
					ArgumentMappings: FieldArgumentMap{
						"metricType": "metric_type",
					},
				},
				"mascot": {
					TargetName: "mascot",
					ArgumentMappings: FieldArgumentMap{
						"includeVolume": "include_volume",
					},
				},
				"categoryStatus": {
					TargetName: "category_status",
					ArgumentMappings: FieldArgumentMap{
						"checkHealth": "check_health",
					},
				},
			},
			"Subcategory": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"description": {
					TargetName: "description",
				},
				"isActive": {
					TargetName: "is_active",
				},
				"itemCount": {
					TargetName: "item_count",
					ArgumentMappings: FieldArgumentMap{
						"filters": "filters",
					},
				},
			},
			"CategoryMetrics": {
				"id": {
					TargetName: "id",
				},
				"metricType": {
					TargetName: "metric_type",
				},
				"value": {
					TargetName: "value",
				},
				"timestamp": {
					TargetName: "timestamp",
				},
				"categoryId": {
					TargetName: "category_id",
				},
				"normalizedScore": {
					TargetName: "normalized_score",
					ArgumentMappings: FieldArgumentMap{
						"baseline": "baseline",
					},
				},
				"relatedCategory": {
					TargetName: "related_category",
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
				"owner": {
					TargetName: "owner",
				},
				"breed": {
					TargetName: "breed",
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
				"owner": {
					TargetName: "owner",
				},
				"breed": {
					TargetName: "breed",
				},
			},
			"Owner": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"contact": {
					TargetName: "contact",
				},
			},
			"ContactInfo": {
				"email": {
					TargetName: "email",
				},
				"phone": {
					TargetName: "phone",
				},
				"address": {
					TargetName: "address",
				},
			},
			"Address": {
				"street": {
					TargetName: "street",
				},
				"city": {
					TargetName: "city",
				},
				"country": {
					TargetName: "country",
				},
				"zipCode": {
					TargetName: "zip_code",
				},
			},
			"CatBreed": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"origin": {
					TargetName: "origin",
				},
				"characteristics": {
					TargetName: "characteristics",
				},
			},
			"DogBreed": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"origin": {
					TargetName: "origin",
				},
				"characteristics": {
					TargetName: "characteristics",
				},
			},
			"BreedCharacteristics": {
				"size": {
					TargetName: "size",
				},
				"temperament": {
					TargetName: "temperament",
				},
				"lifespan": {
					TargetName: "lifespan",
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
			"TestContainer": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"description": {
					TargetName: "description",
				},
				"details": {
					TargetName: "details",
					ArgumentMappings: FieldArgumentMap{
						"includeExtended": "include_extended",
					},
				},
			},
			"TestDetails": {
				"id": {
					TargetName: "id",
				},
				"summary": {
					TargetName: "summary",
				},
				"pet": {
					TargetName: "pet",
				},
				"status": {
					TargetName: "status",
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
			"NullableFieldsType": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"optionalString": {
					TargetName: "optional_string",
				},
				"optionalInt": {
					TargetName: "optional_int",
				},
				"optionalFloat": {
					TargetName: "optional_float",
				},
				"optionalBoolean": {
					TargetName: "optional_boolean",
				},
				"requiredString": {
					TargetName: "required_string",
				},
				"requiredInt": {
					TargetName: "required_int",
				},
			},
			"BlogPost": {
				"id": {
					TargetName: "id",
				},
				"title": {
					TargetName: "title",
				},
				"content": {
					TargetName: "content",
				},
				"tags": {
					TargetName: "tags",
				},
				"optionalTags": {
					TargetName: "optional_tags",
				},
				"categories": {
					TargetName: "categories",
				},
				"keywords": {
					TargetName: "keywords",
				},
				"viewCounts": {
					TargetName: "view_counts",
				},
				"ratings": {
					TargetName: "ratings",
				},
				"isPublished": {
					TargetName: "is_published",
				},
				"tagGroups": {
					TargetName: "tag_groups",
				},
				"relatedTopics": {
					TargetName: "related_topics",
				},
				"commentThreads": {
					TargetName: "comment_threads",
				},
				"suggestions": {
					TargetName: "suggestions",
				},
				"relatedCategories": {
					TargetName: "related_categories",
				},
				"contributors": {
					TargetName: "contributors",
				},
				"mentionedProducts": {
					TargetName: "mentioned_products",
				},
				"mentionedUsers": {
					TargetName: "mentioned_users",
				},
				"categoryGroups": {
					TargetName: "category_groups",
				},
				"contributorTeams": {
					TargetName: "contributor_teams",
				},
			},
			"Author": {
				"id": {
					TargetName: "id",
				},
				"name": {
					TargetName: "name",
				},
				"email": {
					TargetName: "email",
				},
				"skills": {
					TargetName: "skills",
				},
				"languages": {
					TargetName: "languages",
				},
				"socialLinks": {
					TargetName: "social_links",
				},
				"teamsByProject": {
					TargetName: "teams_by_project",
				},
				"collaborations": {
					TargetName: "collaborations",
				},
				"writtenPosts": {
					TargetName: "written_posts",
				},
				"favoriteCategories": {
					TargetName: "favorite_categories",
				},
				"relatedAuthors": {
					TargetName: "related_authors",
				},
				"productReviews": {
					TargetName: "product_reviews",
				},
				"authorGroups": {
					TargetName: "author_groups",
				},
				"categoryPreferences": {
					TargetName: "category_preferences",
				},
				"projectTeams": {
					TargetName: "project_teams",
				},
			},
			"BlogPostInput": {
				"title": {
					TargetName: "title",
				},
				"content": {
					TargetName: "content",
				},
				"tags": {
					TargetName: "tags",
				},
				"optionalTags": {
					TargetName: "optional_tags",
				},
				"categories": {
					TargetName: "categories",
				},
				"keywords": {
					TargetName: "keywords",
				},
				"viewCounts": {
					TargetName: "view_counts",
				},
				"ratings": {
					TargetName: "ratings",
				},
				"isPublished": {
					TargetName: "is_published",
				},
				"tagGroups": {
					TargetName: "tag_groups",
				},
				"relatedTopics": {
					TargetName: "related_topics",
				},
				"commentThreads": {
					TargetName: "comment_threads",
				},
				"suggestions": {
					TargetName: "suggestions",
				},
				"relatedCategories": {
					TargetName: "related_categories",
				},
				"contributors": {
					TargetName: "contributors",
				},
				"categoryGroups": {
					TargetName: "category_groups",
				},
			},
			"AuthorInput": {
				"name": {
					TargetName: "name",
				},
				"email": {
					TargetName: "email",
				},
				"skills": {
					TargetName: "skills",
				},
				"languages": {
					TargetName: "languages",
				},
				"socialLinks": {
					TargetName: "social_links",
				},
				"teamsByProject": {
					TargetName: "teams_by_project",
				},
				"collaborations": {
					TargetName: "collaborations",
				},
				"favoriteCategories": {
					TargetName: "favorite_categories",
				},
				"authorGroups": {
					TargetName: "author_groups",
				},
				"projectTeams": {
					TargetName: "project_teams",
				},
			},
			"BlogPostFilter": {
				"title": {
					TargetName: "title",
				},
				"hasCategories": {
					TargetName: "has_categories",
				},
				"minTags": {
					TargetName: "min_tags",
				},
			},
			"AuthorFilter": {
				"name": {
					TargetName: "name",
				},
				"hasTeams": {
					TargetName: "has_teams",
				},
				"skillCount": {
					TargetName: "skill_count",
				},
			},
			"NullableFieldsInput": {
				"name": {
					TargetName: "name",
				},
				"optionalString": {
					TargetName: "optional_string",
				},
				"optionalInt": {
					TargetName: "optional_int",
				},
				"optionalFloat": {
					TargetName: "optional_float",
				},
				"optionalBoolean": {
					TargetName: "optional_boolean",
				},
				"requiredString": {
					TargetName: "required_string",
				},
				"requiredInt": {
					TargetName: "required_int",
				},
			},
			"NullableFieldsFilter": {
				"name": {
					TargetName: "name",
				},
				"optionalString": {
					TargetName: "optional_string",
				},
				"includeNulls": {
					TargetName: "include_nulls",
				},
			},
			"CategoryInput": {
				"name": {
					TargetName: "name",
				},
				"kind": {
					TargetName: "kind",
				},
			},
			"ProductCountFilter": {
				"minPrice": {
					TargetName: "min_price",
				},
				"maxPrice": {
					TargetName: "max_price",
				},
				"inStock": {
					TargetName: "in_stock",
				},
				"searchTerm": {
					TargetName: "search_term",
				},
			},
			"SubcategoryItemFilter": {
				"minPrice": {
					TargetName: "min_price",
				},
				"maxPrice": {
					TargetName: "max_price",
				},
				"inStock": {
					TargetName: "in_stock",
				},
				"isActive": {
					TargetName: "is_active",
				},
				"searchTerm": {
					TargetName: "search_term",
				},
			},
			"ShippingEstimateInput": {
				"destination": {
					TargetName: "destination",
				},
				"weight": {
					TargetName: "weight",
				},
				"expedited": {
					TargetName: "expedited",
				},
			},
			"UserInput": {
				"name": {
					TargetName: "name",
				},
			},
		},
	}

}
