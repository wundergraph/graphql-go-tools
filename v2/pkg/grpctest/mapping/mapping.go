package mapping

import (
	"testing"

	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
)

// DefaultGRPCMapping returns a hardcoded default mapping between GraphQL and Protobuf
func DefaultGRPCMapping() *grpcdatasource.GRPCMapping {
	return &grpcdatasource.GRPCMapping{
		Service: "Products",
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
		},
		MutationRPCs: grpcdatasource.RPCConfigMap{
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
		SubscriptionRPCs: grpcdatasource.RPCConfigMap{},
		EntityRPCs: map[string][]grpcdatasource.EntityRPCConfig{
			"Product": {
				{
					Key: "id",
					RPCConfig: grpcdatasource.RPCConfig{
						RPC:      "LookupProductById",
						Request:  "LookupProductByIdRequest",
						Response: "LookupProductByIdResponse",
					},
				},
			},
			"Storage": {
				{
					Key: "id",
					RPCConfig: grpcdatasource.RPCConfig{
						RPC:      "LookupStorageById",
						Request:  "LookupStorageByIdRequest",
						Response: "LookupStorageByIdResponse",
					},
				},
			},
			"Warehouse": {
				{
					Key: "id",
					RPCConfig: grpcdatasource.RPCConfig{
						RPC:      "LookupWarehouseById",
						Request:  "LookupWarehouseByIdRequest",
						Response: "LookupWarehouseByIdResponse",
					},
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
				"nullableFieldsType": {
					TargetName: "nullable_fields_type",
				},
				"nullableFieldsTypeById": {
					TargetName: "nullable_fields_type_by_id",
					ArgumentMappings: map[string]string{
						"id": "id",
					},
				},
				"nullableFieldsTypeWithFilter": {
					TargetName: "nullable_fields_type_with_filter",
					ArgumentMappings: map[string]string{
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
					ArgumentMappings: map[string]string{
						"id": "id",
					},
				},
				"blogPostsWithFilter": {
					TargetName: "blog_posts_with_filter",
					ArgumentMappings: map[string]string{
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
					ArgumentMappings: map[string]string{
						"id": "id",
					},
				},
				"authorsWithFilter": {
					TargetName: "authors_with_filter",
					ArgumentMappings: map[string]string{
						"filter": "filter",
					},
				},
				"allAuthors": {
					TargetName: "all_authors",
				},
				"bulkSearchAuthors": {
					TargetName: "bulk_search_authors",
					ArgumentMappings: map[string]string{
						"filters": "filters",
					},
				},
				"bulkSearchBlogPosts": {
					TargetName: "bulk_search_blog_posts",
					ArgumentMappings: map[string]string{
						"filters": "filters",
					},
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
				"createNullableFieldsType": {
					TargetName: "create_nullable_fields_type",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
				"updateNullableFieldsType": {
					TargetName: "update_nullable_fields_type",
					ArgumentMappings: map[string]string{
						"id":    "id",
						"input": "input",
					},
				},
				"createBlogPost": {
					TargetName: "create_blog_post",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
				"updateBlogPost": {
					TargetName: "update_blog_post",
					ArgumentMappings: map[string]string{
						"id":    "id",
						"input": "input",
					},
				},
				"createAuthor": {
					TargetName: "create_author",
					ArgumentMappings: map[string]string{
						"input": "input",
					},
				},
				"updateAuthor": {
					TargetName: "update_author",
					ArgumentMappings: map[string]string{
						"id":    "id",
						"input": "input",
					},
				},
				"bulkCreateAuthors": {
					TargetName: "bulk_create_authors",
					ArgumentMappings: map[string]string{
						"authors": "authors",
					},
				},
				"bulkUpdateAuthors": {
					TargetName: "bulk_update_authors",
					ArgumentMappings: map[string]string{
						"authors": "authors",
					},
				},
				"bulkCreateBlogPosts": {
					TargetName: "bulk_create_blog_posts",
					ArgumentMappings: map[string]string{
						"blogPosts": "blog_posts",
					},
				},
				"bulkUpdateBlogPosts": {
					TargetName: "bulk_update_blog_posts",
					ArgumentMappings: map[string]string{
						"blogPosts": "blog_posts",
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
				"productCount": {
					TargetName: "product_count",
					ArgumentMappings: map[string]string{
						"filters": "filters",
					},
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
			},
			"ActionResult": {
				"actionSuccess": {
					TargetName: "action_success",
				},
				"actionError": {
					TargetName: "action_error",
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
		},
	}
}

func MustDefaultGRPCMapping(t *testing.T) *grpcdatasource.GRPCMapping {
	mapping := DefaultGRPCMapping()
	return mapping
}
