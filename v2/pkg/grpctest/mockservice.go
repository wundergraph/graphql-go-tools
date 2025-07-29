package grpctest

import (
	context "context"
	"fmt"
	"math"
	"math/rand"
	"strconv"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var _ productv1.ProductServiceServer = &MockService{}

type MockService struct {
	productv1.UnimplementedProductServiceServer
}

// Helper functions to convert input types to output types
func convertCategoryInputsToCategories(inputs []*productv1.CategoryInput) []*productv1.Category {
	if inputs == nil {
		return nil
	}
	results := make([]*productv1.Category, len(inputs))
	for i, input := range inputs {
		results[i] = &productv1.Category{
			Id:   fmt.Sprintf("cat-input-%d", i),
			Name: input.GetName(),
			Kind: input.GetKind(),
		}
	}
	return results
}

func convertCategoryInputListToCategories(inputs *productv1.ListOfCategoryInput) []*productv1.Category {
	if inputs == nil || inputs.List == nil || inputs.List.Items == nil {
		return nil
	}
	results := make([]*productv1.Category, len(inputs.List.Items))
	for i, input := range inputs.List.Items {
		results[i] = &productv1.Category{
			Id:   fmt.Sprintf("cat-list-input-%d", i),
			Name: input.GetName(),
			Kind: input.GetKind(),
		}
	}
	return results
}

func convertUserInputsToUsers(inputs *productv1.ListOfUserInput) []*productv1.User {
	if inputs == nil || inputs.List == nil || inputs.List.Items == nil {
		return nil
	}
	results := make([]*productv1.User, len(inputs.List.Items))
	for i, input := range inputs.List.Items {
		results[i] = &productv1.User{
			Id:   fmt.Sprintf("user-input-%d", i),
			Name: input.GetName(),
		}
	}
	return results
}

func convertNestedUserInputsToUsers(nestedInputs *productv1.ListOfListOfUserInput) *productv1.ListOfListOfUser {
	if nestedInputs == nil || nestedInputs.List == nil {
		return &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{},
			},
		}
	}

	results := make([]*productv1.ListOfUser, len(nestedInputs.List.Items))
	for i, userList := range nestedInputs.List.Items {
		users := make([]*productv1.User, len(userList.List.Items))
		for j, userInput := range userList.List.Items {
			users[j] = &productv1.User{
				Id:   fmt.Sprintf("nested-user-%d-%d", i, j),
				Name: userInput.GetName(),
			}
		}
		results[i] = &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: users,
			},
		}
	}

	return &productv1.ListOfListOfUser{
		List: &productv1.ListOfListOfUser_List{
			Items: results,
		},
	}
}

func convertNestedCategoryInputsToCategories(nestedInputs *productv1.ListOfListOfCategoryInput) *productv1.ListOfListOfCategory {
	if nestedInputs == nil || nestedInputs.List == nil {
		return &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{},
			},
		}
	}

	results := make([]*productv1.ListOfCategory, len(nestedInputs.List.Items))
	for i, categoryList := range nestedInputs.List.Items {
		categories := make([]*productv1.Category, len(categoryList.List.Items))
		for j, categoryInput := range categoryList.List.Items {
			categories[j] = &productv1.Category{
				Id:   fmt.Sprintf("nested-cat-%d-%d", i, j),
				Name: categoryInput.GetName(),
				Kind: categoryInput.GetKind(),
			}
		}
		results[i] = &productv1.ListOfCategory{
			List: &productv1.ListOfCategory_List{
				Items: categories,
			},
		}
	}

	return &productv1.ListOfListOfCategory{
		List: &productv1.ListOfListOfCategory_List{
			Items: results,
		},
	}
}

// MutationCreateNullableFieldsType implements productv1.ProductServiceServer.
func (s *MockService) MutationCreateNullableFieldsType(ctx context.Context, in *productv1.MutationCreateNullableFieldsTypeRequest) (*productv1.MutationCreateNullableFieldsTypeResponse, error) {
	input := in.GetInput()

	// Create a new NullableFieldsType from the input
	result := &productv1.NullableFieldsType{
		Id:             fmt.Sprintf("nullable-%d", rand.Intn(1000)),
		Name:           input.GetName(),
		RequiredString: input.GetRequiredString(),
		RequiredInt:    input.GetRequiredInt(),
	}

	// Handle optional fields - copy from input if they exist
	if input.OptionalString != nil {
		result.OptionalString = &wrapperspb.StringValue{Value: input.OptionalString.GetValue()}
	}
	if input.OptionalInt != nil {
		result.OptionalInt = &wrapperspb.Int32Value{Value: input.OptionalInt.GetValue()}
	}
	if input.OptionalFloat != nil {
		result.OptionalFloat = &wrapperspb.DoubleValue{Value: input.OptionalFloat.GetValue()}
	}
	if input.OptionalBoolean != nil {
		result.OptionalBoolean = &wrapperspb.BoolValue{Value: input.OptionalBoolean.GetValue()}
	}

	return &productv1.MutationCreateNullableFieldsTypeResponse{
		CreateNullableFieldsType: result,
	}, nil
}

// MutationUpdateNullableFieldsType implements productv1.ProductServiceServer.
func (s *MockService) MutationUpdateNullableFieldsType(ctx context.Context, in *productv1.MutationUpdateNullableFieldsTypeRequest) (*productv1.MutationUpdateNullableFieldsTypeResponse, error) {
	id := in.GetId()
	input := in.GetInput()

	// Return nil if trying to update a non-existent ID
	if id == "non-existent" {
		return &productv1.MutationUpdateNullableFieldsTypeResponse{
			UpdateNullableFieldsType: nil,
		}, nil
	}

	// Create updated NullableFieldsType
	result := &productv1.NullableFieldsType{
		Id:             id,
		Name:           input.GetName(),
		RequiredString: input.GetRequiredString(),
		RequiredInt:    input.GetRequiredInt(),
	}

	// Handle optional fields - copy from input if they exist
	if input.OptionalString != nil {
		result.OptionalString = &wrapperspb.StringValue{Value: input.OptionalString.GetValue()}
	}
	if input.OptionalInt != nil {
		result.OptionalInt = &wrapperspb.Int32Value{Value: input.OptionalInt.GetValue()}
	}
	if input.OptionalFloat != nil {
		result.OptionalFloat = &wrapperspb.DoubleValue{Value: input.OptionalFloat.GetValue()}
	}
	if input.OptionalBoolean != nil {
		result.OptionalBoolean = &wrapperspb.BoolValue{Value: input.OptionalBoolean.GetValue()}
	}

	return &productv1.MutationUpdateNullableFieldsTypeResponse{
		UpdateNullableFieldsType: result,
	}, nil
}

// QueryAllNullableFieldsTypes implements productv1.ProductServiceServer.
func (s *MockService) QueryAllNullableFieldsTypes(ctx context.Context, in *productv1.QueryAllNullableFieldsTypesRequest) (*productv1.QueryAllNullableFieldsTypesResponse, error) {
	var results []*productv1.NullableFieldsType

	// Create a variety of test data with different nullable field combinations

	// Entry 1: All fields populated
	results = append(results, &productv1.NullableFieldsType{
		Id:              "nullable-1",
		Name:            "Full Data Entry",
		OptionalString:  &wrapperspb.StringValue{Value: "Optional String Value"},
		OptionalInt:     &wrapperspb.Int32Value{Value: 42},
		OptionalFloat:   &wrapperspb.DoubleValue{Value: math.MaxFloat64},
		OptionalBoolean: &wrapperspb.BoolValue{Value: true},
		RequiredString:  "Required String 1",
		RequiredInt:     100,
	})

	// Entry 2: Some nullable fields are null
	results = append(results, &productv1.NullableFieldsType{
		Id:              "nullable-2",
		Name:            "Partial Data Entry",
		OptionalString:  &wrapperspb.StringValue{Value: "Only string is set"},
		OptionalInt:     nil, // null
		OptionalFloat:   nil, // null
		OptionalBoolean: &wrapperspb.BoolValue{Value: false},
		RequiredString:  "Required String 2",
		RequiredInt:     200,
	})

	// Entry 3: All nullable fields are null
	results = append(results, &productv1.NullableFieldsType{
		Id:              "nullable-3",
		Name:            "Minimal Data Entry",
		OptionalString:  nil, // null
		OptionalInt:     nil, // null
		OptionalFloat:   nil, // null
		OptionalBoolean: nil, // null
		RequiredString:  "Required String 3",
		RequiredInt:     300,
	})

	return &productv1.QueryAllNullableFieldsTypesResponse{
		AllNullableFieldsTypes: results,
	}, nil
}

// QueryNullableFieldsType implements productv1.ProductServiceServer.
func (s *MockService) QueryNullableFieldsType(ctx context.Context, in *productv1.QueryNullableFieldsTypeRequest) (*productv1.QueryNullableFieldsTypeResponse, error) {
	// Return a single NullableFieldsType with mixed null/non-null values
	result := &productv1.NullableFieldsType{
		Id:              "nullable-default",
		Name:            "Default Nullable Fields Type",
		OptionalString:  &wrapperspb.StringValue{Value: "Default optional string"},
		OptionalInt:     &wrapperspb.Int32Value{Value: 777},
		OptionalFloat:   nil, // null
		OptionalBoolean: &wrapperspb.BoolValue{Value: true},
		RequiredString:  "Default required string",
		RequiredInt:     999,
	}

	return &productv1.QueryNullableFieldsTypeResponse{
		NullableFieldsType: result,
	}, nil
}

// QueryNullableFieldsTypeById implements productv1.ProductServiceServer.
func (s *MockService) QueryNullableFieldsTypeById(ctx context.Context, in *productv1.QueryNullableFieldsTypeByIdRequest) (*productv1.QueryNullableFieldsTypeByIdResponse, error) {
	id := in.GetId()

	// Return null for specific test IDs
	if id == "not-found" || id == "null-test" {
		return &productv1.QueryNullableFieldsTypeByIdResponse{
			NullableFieldsTypeById: nil,
		}, nil
	}

	// Create different test data based on ID
	var result *productv1.NullableFieldsType

	switch id {
	case "full-data":
		result = &productv1.NullableFieldsType{
			Id:              id,
			Name:            "Full Data by ID",
			OptionalString:  &wrapperspb.StringValue{Value: "All fields populated"},
			OptionalInt:     &wrapperspb.Int32Value{Value: 123},
			OptionalFloat:   &wrapperspb.DoubleValue{Value: 12.34},
			OptionalBoolean: &wrapperspb.BoolValue{Value: false},
			RequiredString:  "Required by ID",
			RequiredInt:     456,
		}
	case "partial-data":
		result = &productv1.NullableFieldsType{
			Id:              id,
			Name:            "Partial Data by ID",
			OptionalString:  nil, // null
			OptionalInt:     &wrapperspb.Int32Value{Value: 789},
			OptionalFloat:   nil, // null
			OptionalBoolean: &wrapperspb.BoolValue{Value: true},
			RequiredString:  "Partial required by ID",
			RequiredInt:     321,
		}
	case "minimal-data":
		result = &productv1.NullableFieldsType{
			Id:              id,
			Name:            "Minimal Data by ID",
			OptionalString:  nil, // null
			OptionalInt:     nil, // null
			OptionalFloat:   nil, // null
			OptionalBoolean: nil, // null
			RequiredString:  "Only required fields",
			RequiredInt:     111,
		}
	default:
		// Generic response for any other ID
		result = &productv1.NullableFieldsType{
			Id:              id,
			Name:            fmt.Sprintf("Nullable Type %s", id),
			OptionalString:  &wrapperspb.StringValue{Value: fmt.Sprintf("Optional for %s", id)},
			OptionalInt:     &wrapperspb.Int32Value{Value: int32(len(id) * 10)},
			OptionalFloat:   &wrapperspb.DoubleValue{Value: float64(len(id)) * 1.5},
			OptionalBoolean: &wrapperspb.BoolValue{Value: len(id)%2 == 0},
			RequiredString:  fmt.Sprintf("Required for %s", id),
			RequiredInt:     int32(len(id) * 100),
		}
	}

	return &productv1.QueryNullableFieldsTypeByIdResponse{
		NullableFieldsTypeById: result,
	}, nil
}

// QueryNullableFieldsTypeWithFilter implements productv1.ProductServiceServer.
func (s *MockService) QueryNullableFieldsTypeWithFilter(ctx context.Context, in *productv1.QueryNullableFieldsTypeWithFilterRequest) (*productv1.QueryNullableFieldsTypeWithFilterResponse, error) {
	filter := in.GetFilter()
	var results []*productv1.NullableFieldsType

	// If no filter provided, return empty results
	if filter == nil {
		return &productv1.QueryNullableFieldsTypeWithFilterResponse{
			NullableFieldsTypeWithFilter: results,
		}, nil
	}

	// Create test data based on filter criteria
	nameFilter := ""
	if filter.Name != nil {
		nameFilter = filter.Name.GetValue()
	}

	optionalStringFilter := ""
	if filter.OptionalString != nil {
		optionalStringFilter = filter.OptionalString.GetValue()
	}

	includeNulls := false
	if filter.IncludeNulls != nil {
		includeNulls = filter.IncludeNulls.GetValue()
	}

	// Generate filtered results
	for i := 1; i <= 3; i++ {
		var optionalString *wrapperspb.StringValue
		var optionalInt *wrapperspb.Int32Value
		var optionalFloat *wrapperspb.DoubleValue
		var optionalBoolean *wrapperspb.BoolValue

		// Vary the nullable fields based on includeNulls and index
		if includeNulls || i%2 == 1 {
			if optionalStringFilter != "" {
				optionalString = &wrapperspb.StringValue{Value: optionalStringFilter}
			} else {
				optionalString = &wrapperspb.StringValue{Value: fmt.Sprintf("Filtered string %d", i)}
			}
		}

		if includeNulls || i%3 != 0 {
			optionalInt = &wrapperspb.Int32Value{Value: int32(i * 100)}
		}

		if includeNulls || i%2 == 0 {
			optionalFloat = &wrapperspb.DoubleValue{Value: float64(i) * 10.5}
		}

		if includeNulls || i%4 != 0 {
			optionalBoolean = &wrapperspb.BoolValue{Value: i%2 == 0}
		}

		name := fmt.Sprintf("Filtered Item %d", i)
		if nameFilter != "" {
			name = fmt.Sprintf("%s - %d", nameFilter, i)
		}

		results = append(results, &productv1.NullableFieldsType{
			Id:              fmt.Sprintf("filtered-%d", i),
			Name:            name,
			OptionalString:  optionalString,
			OptionalInt:     optionalInt,
			OptionalFloat:   optionalFloat,
			OptionalBoolean: optionalBoolean,
			RequiredString:  fmt.Sprintf("Required filtered %d", i),
			RequiredInt:     int32(i * 1000),
		})
	}

	return &productv1.QueryNullableFieldsTypeWithFilterResponse{
		NullableFieldsTypeWithFilter: results,
	}, nil
}

// MutationPerformAction implements productv1.ProductServiceServer.
func (s *MockService) MutationPerformAction(ctx context.Context, in *productv1.MutationPerformActionRequest) (*productv1.MutationPerformActionResponse, error) {
	input := in.GetInput()
	actionType := input.GetType()

	// Simulate different action results based on the action type
	var result *productv1.ActionResult

	switch actionType {
	case "error_action":
		// Return an error result
		result = &productv1.ActionResult{
			Value: &productv1.ActionResult_ActionError{
				ActionError: &productv1.ActionError{
					Message: "Action failed due to validation error",
					Code:    "VALIDATION_ERROR",
				},
			},
		}
	case "invalid_action":
		// Return a different error result
		result = &productv1.ActionResult{
			Value: &productv1.ActionResult_ActionError{
				ActionError: &productv1.ActionError{
					Message: "Invalid action type provided",
					Code:    "INVALID_ACTION",
				},
			},
		}
	default:
		// Return a success result
		result = &productv1.ActionResult{
			Value: &productv1.ActionResult_ActionSuccess{
				ActionSuccess: &productv1.ActionSuccess{
					Message:   fmt.Sprintf("Action '%s' completed successfully", actionType),
					Timestamp: "2024-01-01T00:00:00Z",
				},
			},
		}
	}

	return &productv1.MutationPerformActionResponse{
		PerformAction: result,
	}, nil
}

// QueryRandomSearchResult implements productv1.ProductServiceServer.
func (s *MockService) QueryRandomSearchResult(ctx context.Context, in *productv1.QueryRandomSearchResultRequest) (*productv1.QueryRandomSearchResultResponse, error) {
	// Randomly return one of the three union types
	var result *productv1.SearchResult

	switch rand.Intn(3) {
	case 0:
		// Return a Product
		result = &productv1.SearchResult{
			Value: &productv1.SearchResult_Product{
				Product: &productv1.Product{
					Id:    "product-random-1",
					Name:  "Random Product",
					Price: 29.99,
				},
			},
		}
	case 1:
		// Return a User
		result = &productv1.SearchResult{
			Value: &productv1.SearchResult_User{
				User: &productv1.User{
					Id:   "user-random-1",
					Name: "Random User",
				},
			},
		}
	default:
		// Return a Category
		result = &productv1.SearchResult{
			Value: &productv1.SearchResult_Category{
				Category: &productv1.Category{
					Id:   "category-random-1",
					Name: "Random Category",
					Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS,
				},
			},
		}
	}

	return &productv1.QueryRandomSearchResultResponse{
		RandomSearchResult: result,
	}, nil
}

// QuerySearch implements productv1.ProductServiceServer.
func (s *MockService) QuerySearch(ctx context.Context, in *productv1.QuerySearchRequest) (*productv1.QuerySearchResponse, error) {
	input := in.GetInput()
	query := input.GetQuery()
	limit := input.GetLimit()

	// Default limit if not specified
	if limit.GetValue() <= 0 {
		limit = &wrapperspb.Int32Value{Value: 10}
	}

	var results []*productv1.SearchResult

	// Generate a mix of different union types based on the query
	for i := int32(0); i < limit.GetValue() && i < 6; i++ { // Cap at 6 results for testing
		switch i % 3 {
		case 0:
			// Add a Product
			results = append(results, &productv1.SearchResult{
				Value: &productv1.SearchResult_Product{
					Product: &productv1.Product{
						Id:    fmt.Sprintf("product-search-%d", i+1),
						Name:  fmt.Sprintf("Product matching '%s' #%d", query, i+1),
						Price: float64(10 + i*5),
					},
				},
			})
		case 1:
			// Add a User
			results = append(results, &productv1.SearchResult{
				Value: &productv1.SearchResult_User{
					User: &productv1.User{
						Id:   fmt.Sprintf("user-search-%d", i+1),
						Name: fmt.Sprintf("User matching '%s' #%d", query, i+1),
					},
				},
			})
		case 2:
			// Add a Category
			kinds := []productv1.CategoryKind{
				productv1.CategoryKind_CATEGORY_KIND_BOOK,
				productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS,
				productv1.CategoryKind_CATEGORY_KIND_FURNITURE,
			}
			results = append(results, &productv1.SearchResult{
				Value: &productv1.SearchResult_Category{
					Category: &productv1.Category{
						Id:   fmt.Sprintf("category-search-%d", i+1),
						Name: fmt.Sprintf("Category matching '%s' #%d", query, i+1),
						Kind: kinds[i%int32(len(kinds))],
					},
				},
			})
		}
	}

	return &productv1.QuerySearchResponse{
		Search: results,
	}, nil
}

func (s *MockService) LookupProductById(ctx context.Context, in *productv1.LookupProductByIdRequest) (*productv1.LookupProductByIdResponse, error) {
	var results []*productv1.Product

	for _, input := range in.GetKeys() {
		productId := input.GetId()
		results = append(results, &productv1.Product{
			Id:    productId,
			Name:  fmt.Sprintf("Product %s", productId),
			Price: 99.99,
		})
	}

	return &productv1.LookupProductByIdResponse{
		Result: results,
	}, nil
}

func (s *MockService) LookupStorageById(ctx context.Context, in *productv1.LookupStorageByIdRequest) (*productv1.LookupStorageByIdResponse, error) {
	var results []*productv1.Storage

	for _, input := range in.GetKeys() {
		storageId := input.GetId()
		results = append(results, &productv1.Storage{
			Id:       storageId,
			Name:     fmt.Sprintf("Storage %s", storageId),
			Location: fmt.Sprintf("Location %d", rand.Intn(100)),
		})
	}

	return &productv1.LookupStorageByIdResponse{
		Result: results,
	}, nil
}

func (s *MockService) QueryUsers(ctx context.Context, in *productv1.QueryUsersRequest) (*productv1.QueryUsersResponse, error) {
	var results []*productv1.User

	// Generate 3 random users
	for i := 1; i <= 3; i++ {
		results = append(results, &productv1.User{
			Id:   fmt.Sprintf("user-%d", i),
			Name: fmt.Sprintf("User %d", i),
		})
	}

	return &productv1.QueryUsersResponse{
		Users: results,
	}, nil
}

func (s *MockService) QueryUser(ctx context.Context, in *productv1.QueryUserRequest) (*productv1.QueryUserResponse, error) {
	userId := in.GetId()

	// Return a gRPC status error for a specific test case
	if userId == "error-user" {
		return nil, status.Errorf(codes.NotFound, "user not found: %s", userId)
	}

	return &productv1.QueryUserResponse{
		User: &productv1.User{
			Id:   userId,
			Name: fmt.Sprintf("User %s", userId),
		},
	}, nil
}

func (s *MockService) QueryNestedType(ctx context.Context, in *productv1.QueryNestedTypeRequest) (*productv1.QueryNestedTypeResponse, error) {
	var nestedTypes []*productv1.NestedTypeA

	// Generate 2 nested types
	for i := 1; i <= 2; i++ {
		nestedTypes = append(nestedTypes, &productv1.NestedTypeA{
			Id:   fmt.Sprintf("nested-a-%d", i),
			Name: fmt.Sprintf("Nested A %d", i),
			B: &productv1.NestedTypeB{
				Id:   fmt.Sprintf("nested-b-%d", i),
				Name: fmt.Sprintf("Nested B %d", i),
				C: &productv1.NestedTypeC{
					Id:   fmt.Sprintf("nested-c-%d", i),
					Name: fmt.Sprintf("Nested C %d", i),
				},
			},
		})
	}

	return &productv1.QueryNestedTypeResponse{
		NestedType: nestedTypes,
	}, nil
}

func (s *MockService) QueryRecursiveType(ctx context.Context, in *productv1.QueryRecursiveTypeRequest) (*productv1.QueryRecursiveTypeResponse, error) {
	// Create a recursive structure 3 levels deep
	recursiveType := &productv1.RecursiveType{
		Id:   "recursive-1",
		Name: "Level 1",
		RecursiveType: &productv1.RecursiveType{
			Id:   "recursive-2",
			Name: "Level 2",
			RecursiveType: &productv1.RecursiveType{
				Id:   "recursive-3",
				Name: "Level 3",
			},
		},
	}

	return &productv1.QueryRecursiveTypeResponse{
		RecursiveType: recursiveType,
	}, nil
}

func (s *MockService) QueryTypeFilterWithArguments(ctx context.Context, in *productv1.QueryTypeFilterWithArgumentsRequest) (*productv1.QueryTypeFilterWithArgumentsResponse, error) {
	filterField1 := in.GetFilterField_1()
	filterField2 := in.GetFilterField_2()

	var fields []*productv1.TypeWithMultipleFilterFields

	// Create results that echo the filter values
	for i := 1; i <= 2; i++ {
		fields = append(fields, &productv1.TypeWithMultipleFilterFields{
			Id:            fmt.Sprintf("multi-filter-%d", i),
			Name:          fmt.Sprintf("MultiFilter %d", i),
			FilterField_1: filterField1,
			FilterField_2: filterField2,
		})
	}

	return &productv1.QueryTypeFilterWithArgumentsResponse{
		TypeFilterWithArguments: fields,
	}, nil
}

func (s *MockService) QueryTypeWithMultipleFilterFields(ctx context.Context, in *productv1.QueryTypeWithMultipleFilterFieldsRequest) (*productv1.QueryTypeWithMultipleFilterFieldsResponse, error) {
	filter := in.GetFilter()

	var fields []*productv1.TypeWithMultipleFilterFields

	// Echo the filter values in the results
	for i := 1; i <= 2; i++ {
		fields = append(fields, &productv1.TypeWithMultipleFilterFields{
			Id:            fmt.Sprintf("filtered-%d", i),
			Name:          "Filter: " + strconv.Itoa(i),
			FilterField_1: filter.FilterField_1,
			FilterField_2: filter.FilterField_2,
		})
	}

	return &productv1.QueryTypeWithMultipleFilterFieldsResponse{
		TypeWithMultipleFilterFields: fields,
	}, nil
}

func (s *MockService) QueryComplexFilterType(ctx context.Context, in *productv1.QueryComplexFilterTypeRequest) (*productv1.QueryComplexFilterTypeResponse, error) {
	filter := in.GetFilter()

	var name string
	if filter != nil && filter.GetFilter() != nil {
		name = filter.GetFilter().GetName()
	} else {
		name = "Default Product"
	}

	return &productv1.QueryComplexFilterTypeResponse{
		ComplexFilterType: []*productv1.TypeWithComplexFilterInput{
			{
				Id:   "test-id-123",
				Name: name,
			},
		},
	}, nil
}

func (s *MockService) QueryRandomPet(ctx context.Context, in *productv1.QueryRandomPetRequest) (*productv1.QueryRandomPetResponse, error) {
	// Create either a cat or dog randomly
	var pet *productv1.Animal

	// Random choice between cat and dog
	if rand.Intn(2) == 0 {
		// Create a cat
		pet = &productv1.Animal{
			Instance: &productv1.Animal_Cat{
				Cat: &productv1.Cat{
					Id:         "cat-1",
					Name:       "Whiskers",
					Kind:       "Siamese",
					MeowVolume: int32(rand.Intn(10) + 1), // Random volume between 1-10
				},
			},
		}
	} else {
		// Create a dog
		pet = &productv1.Animal{
			Instance: &productv1.Animal_Dog{
				Dog: &productv1.Dog{
					Id:         "dog-1",
					Name:       "Spot",
					Kind:       "Dalmatian",
					BarkVolume: int32(rand.Intn(10) + 1), // Random volume between 1-10
				},
			},
		}
	}

	return &productv1.QueryRandomPetResponse{
		RandomPet: pet,
	}, nil
}

func (s *MockService) QueryAllPets(ctx context.Context, in *productv1.QueryAllPetsRequest) (*productv1.QueryAllPetsResponse, error) {
	// Create a mix of cats and dogs
	var pets []*productv1.Animal

	// Add 2 cats
	for i := 1; i <= 2; i++ {
		pets = append(pets, &productv1.Animal{
			Instance: &productv1.Animal_Cat{
				Cat: &productv1.Cat{
					Id:         fmt.Sprintf("cat-%d", i),
					Name:       fmt.Sprintf("Cat %d", i),
					Kind:       fmt.Sprintf("Breed %d", i),
					MeowVolume: int32(i + 3), // Different volumes
				},
			},
		})
	}

	// Add 2 dogs
	for i := 1; i <= 2; i++ {
		pets = append(pets, &productv1.Animal{
			Instance: &productv1.Animal_Dog{
				Dog: &productv1.Dog{
					Id:         fmt.Sprintf("dog-%d", i),
					Name:       fmt.Sprintf("Dog %d", i),
					Kind:       fmt.Sprintf("Breed %d", i),
					BarkVolume: int32(i + 5), // Different volumes
				},
			},
		})
	}

	return &productv1.QueryAllPetsResponse{
		AllPets: pets,
	}, nil
}

// Implementation for QueryCategories
func (s *MockService) QueryCategories(ctx context.Context, in *productv1.QueryCategoriesRequest) (*productv1.QueryCategoriesResponse, error) {
	// Generate a list of categories
	var categories []*productv1.Category

	// Create sample categories for each CategoryKind
	categoryKinds := []productv1.CategoryKind{
		productv1.CategoryKind_CATEGORY_KIND_BOOK,
		productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS,
		productv1.CategoryKind_CATEGORY_KIND_FURNITURE,
		productv1.CategoryKind_CATEGORY_KIND_OTHER,
	}

	for i, kind := range categoryKinds {
		categories = append(categories, &productv1.Category{
			Id:   fmt.Sprintf("category-%d", i+1),
			Name: fmt.Sprintf("%s Category", kind.String()),
			Kind: kind,
		})
	}

	return &productv1.QueryCategoriesResponse{
		Categories: categories,
	}, nil
}

// Implementation for QueryCategoriesByKind
func (s *MockService) QueryCategoriesByKind(ctx context.Context, in *productv1.QueryCategoriesByKindRequest) (*productv1.QueryCategoriesByKindResponse, error) {
	kind := in.GetKind()

	// Generate categories for the specified kind
	var categories []*productv1.Category

	// Create 3 categories of the requested kind
	for i := 1; i <= 3; i++ {
		categories = append(categories, &productv1.Category{
			Id:   fmt.Sprintf("%s-category-%d", kind.String(), i),
			Name: fmt.Sprintf("%s Category %d", kind.String(), i),
			Kind: kind,
		})
	}

	return &productv1.QueryCategoriesByKindResponse{
		CategoriesByKind: categories,
	}, nil
}

func (s *MockService) QueryCategoriesByKinds(ctx context.Context, in *productv1.QueryCategoriesByKindsRequest) (*productv1.QueryCategoriesByKindsResponse, error) {
	kinds := in.GetKinds()

	var categories []*productv1.Category

	for i, kind := range kinds {
		categories = append(categories, &productv1.Category{
			Id:   fmt.Sprintf("%s-category-%d", kind.String(), i),
			Name: fmt.Sprintf("%s Category %d", kind.String(), i),
			Kind: kind,
		})
	}

	return &productv1.QueryCategoriesByKindsResponse{
		CategoriesByKinds: categories,
	}, nil
}

// Implementation for QueryFilterCategories
func (s *MockService) QueryFilterCategories(ctx context.Context, in *productv1.QueryFilterCategoriesRequest) (*productv1.QueryFilterCategoriesResponse, error) {
	filter := in.GetFilter()

	if filter == nil {
		return &productv1.QueryFilterCategoriesResponse{
			FilterCategories: []*productv1.Category{},
		}, nil
	}

	kind := filter.GetCategory()

	// Generate filtered categories
	var categories []*productv1.Category

	// Create categories that match the filter
	for i := 1; i <= 5; i++ {
		categories = append(categories, &productv1.Category{
			Id:   fmt.Sprintf("filtered-%s-category-%d", kind.String(), i),
			Name: fmt.Sprintf("Filtered %s Category %d", kind.String(), i),
			Kind: kind,
		})
	}

	// Apply pagination if provided
	pagination := filter.GetPagination()
	if pagination != nil {
		page := int(pagination.GetPage())
		perPage := int(pagination.GetPerPage())

		if page > 0 && perPage > 0 && len(categories) > perPage {
			startIdx := (page - 1) * perPage
			endIdx := startIdx + perPage

			if startIdx < len(categories) {
				if endIdx > len(categories) {
					endIdx = len(categories)
				}
				categories = categories[startIdx:endIdx]
			} else {
				categories = []*productv1.Category{}
			}
		}
	}

	return &productv1.QueryFilterCategoriesResponse{
		FilterCategories: categories,
	}, nil
}

// Implementation for CreateUser mutation
func (s *MockService) MutationCreateUser(ctx context.Context, in *productv1.MutationCreateUserRequest) (*productv1.MutationCreateUserResponse, error) {
	input := in.GetInput()

	// Create a new user with the input name and a random ID
	user := &productv1.User{
		Id:   fmt.Sprintf("user-%d", rand.Intn(1000)),
		Name: input.GetName(),
	}

	return &productv1.MutationCreateUserResponse{
		CreateUser: user,
	}, nil
}

// Implementation for QueryCalculateTotals
func (s *MockService) QueryCalculateTotals(ctx context.Context, in *productv1.QueryCalculateTotalsRequest) (*productv1.QueryCalculateTotalsResponse, error) {
	orders := in.GetOrders()
	var calculatedOrders []*productv1.Order

	for _, orderInput := range orders {
		// Calculate total items by summing up quantities from all order lines
		var totalItems int32
		for _, line := range orderInput.GetLines() {
			totalItems += line.GetQuantity()
		}

		orderLines := []*productv1.OrderLine{}
		for _, line := range orderInput.GetLines() {
			orderLines = append(orderLines, &productv1.OrderLine{
				ProductId: line.GetProductId(),
				Quantity:  line.GetQuantity(),
				Modifiers: line.GetModifiers(),
			})
		}

		calculatedOrders = append(calculatedOrders, &productv1.Order{
			OrderId:      orderInput.GetOrderId(),
			CustomerName: orderInput.GetCustomerName(),
			TotalItems:   totalItems,
			OrderLines: &productv1.ListOfOrderLine{
				List: &productv1.ListOfOrderLine_List{
					Items: orderLines,
				},
			},
		})
	}

	return &productv1.QueryCalculateTotalsResponse{
		CalculateTotals: calculatedOrders,
	}, nil
}

// BlogPost query implementations
func (s *MockService) QueryBlogPost(ctx context.Context, in *productv1.QueryBlogPostRequest) (*productv1.QueryBlogPostResponse, error) {
	// Return a default blog post with comprehensive list examples
	result := &productv1.BlogPost{
		Id:      "blog-default",
		Title:   "Default Blog Post",
		Content: "This is a sample blog post content for testing nested lists.",
		Tags:    []string{"tech", "programming", "go"},
		OptionalTags: &productv1.ListOfString{
			List: &productv1.ListOfString_List{
				Items: []string{"optional1", "optional2"},
			},
		},
		Categories: []string{"Technology", "", "Programming"}, // includes null/empty
		Keywords: &productv1.ListOfString{
			List: &productv1.ListOfString_List{
				Items: []string{"keyword1", "keyword2"},
			},
		},
		ViewCounts: []int32{100, 150, 200, 250},
		Ratings: &productv1.ListOfFloat{
			List: &productv1.ListOfFloat_List{
				Items: []float64{4.5, 3.8, 5.0},
			},
		},
		IsPublished: &productv1.ListOfBoolean{
			List: &productv1.ListOfBoolean_List{
				Items: []bool{false, true, true},
			},
		},
		TagGroups: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{
						Items: []string{"tech", "programming"},
					}},
					{List: &productv1.ListOfString_List{
						Items: []string{"golang", "backend"},
					}},
				},
			},
		},
		RelatedTopics: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{Items: []string{"microservices", "api"}}},
					{List: &productv1.ListOfString_List{Items: []string{"databases", "performance"}}},
				},
			},
		},
		CommentThreads: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{Items: []string{"Great post!", "Very helpful"}}},
					{List: &productv1.ListOfString_List{Items: []string{"Could use more examples", "Thanks for sharing"}}},
				},
			},
		},
		Suggestions: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{Items: []string{"Add code examples", "Include diagrams"}}},
				},
			},
		},
		RelatedCategories: []*productv1.Category{
			{Id: "cat-1", Name: "Technology", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
			{Id: "cat-2", Name: "Programming", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
		},
		Contributors: []*productv1.User{
			{Id: "user-1", Name: "John Doe"},
			{Id: "user-2", Name: "Jane Smith"},
		},
		MentionedProducts: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "prod-1", Name: "Sample Product", Price: 99.99},
				},
			},
		},
		MentionedUsers: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "user-3", Name: "Bob Johnson"},
				},
			},
		},
		CategoryGroups: &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{
					{List: &productv1.ListOfCategory_List{
						Items: []*productv1.Category{
							{Id: "cat-3", Name: "Web Development", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
							{Id: "cat-4", Name: "Backend", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
						},
					}},
				},
			},
		},
		ContributorTeams: &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "user-4", Name: "Alice Brown"},
							{Id: "user-5", Name: "Charlie Wilson"},
						},
					}},
				},
			},
		},
	}

	return &productv1.QueryBlogPostResponse{
		BlogPost: result,
	}, nil
}

func (s *MockService) QueryBlogPostById(ctx context.Context, in *productv1.QueryBlogPostByIdRequest) (*productv1.QueryBlogPostByIdResponse, error) {
	id := in.GetId()

	// Return null for specific test IDs
	if id == "not-found" {
		return &productv1.QueryBlogPostByIdResponse{
			BlogPostById: nil,
		}, nil
	}

	// Create different test data based on ID
	var result *productv1.BlogPost

	switch id {
	case "simple":
		result = &productv1.BlogPost{
			Id:         id,
			Title:      "Simple Post",
			Content:    "Simple content",
			Tags:       []string{"simple"},
			Categories: []string{"Basic"},
			ViewCounts: []int32{10},
			// Required nested lists must have data
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"simple"}}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"basic"}}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"Nice post"}}},
					},
				},
			},
			// Required complex lists must have data
			RelatedCategories: []*productv1.Category{
				{Id: "cat-simple", Name: "Basic", Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			Contributors: []*productv1.User{
				{Id: "user-simple", Name: "Simple Author"},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-group-simple", Name: "Simple Category", Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
		}
	case "complex":
		result = &productv1.BlogPost{
			Id:      id,
			Title:   "Complex Blog Post",
			Content: "Complex content with comprehensive lists",
			Tags:    []string{"complex", "advanced", "detailed"},
			OptionalTags: &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{"deep-dive", "tutorial"},
				},
			},
			Categories: []string{"Advanced", "Tutorial", "Guide"},
			Keywords: &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{"advanced", "complex", "comprehensive"},
				},
			},
			ViewCounts: []int32{500, 600, 750, 800, 950},
			Ratings: &productv1.ListOfFloat{
				List: &productv1.ListOfFloat_List{
					Items: []float64{4.8, 4.9, 4.7, 5.0},
				},
			},
			IsPublished: &productv1.ListOfBoolean{
				List: &productv1.ListOfBoolean_List{
					Items: []bool{false, false, true, true},
				},
			},
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"advanced", "expert"}}},
						{List: &productv1.ListOfString_List{Items: []string{"tutorial", "guide", "comprehensive"}}},
						{List: &productv1.ListOfString_List{Items: []string{"deep-dive", "detailed"}}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"architecture", "patterns", "design"}}},
						{List: &productv1.ListOfString_List{Items: []string{"optimization", "performance", "scaling"}}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"Excellent deep dive!", "Very thorough"}}},
						{List: &productv1.ListOfString_List{Items: []string{"Could be longer", "More examples please"}}},
						{List: &productv1.ListOfString_List{Items: []string{"Best tutorial I've read", "Thank you!"}}},
					},
				},
			},
			Suggestions: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"Add video content", "Include interactive examples"}}},
						{List: &productv1.ListOfString_List{Items: []string{"Create follow-up posts", "Add Q&A section"}}},
					},
				},
			},
			// Complex example includes all new complex list fields
			RelatedCategories: []*productv1.Category{
				{Id: "cat-complex-1", Name: "Advanced Programming", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
				{Id: "cat-complex-2", Name: "Software Architecture", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
			},
			Contributors: []*productv1.User{
				{Id: "user-complex-1", Name: "Expert Author"},
				{Id: "user-complex-2", Name: "Technical Reviewer"},
			},
			MentionedProducts: &productv1.ListOfProduct{
				List: &productv1.ListOfProduct_List{
					Items: []*productv1.Product{
						{Id: "prod-complex-1", Name: "Advanced IDE", Price: 299.99},
						{Id: "prod-complex-2", Name: "Profiling Tool", Price: 149.99},
					},
				},
			},
			MentionedUsers: &productv1.ListOfUser{
				List: &productv1.ListOfUser_List{
					Items: []*productv1.User{
						{Id: "user-complex-3", Name: "Referenced Expert"},
					},
				},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-group-1", Name: "System Design", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
								{Id: "cat-group-2", Name: "Architecture Patterns", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
							},
						}},
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-group-3", Name: "Performance", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
							},
						}},
					},
				},
			},
			ContributorTeams: &productv1.ListOfListOfUser{
				List: &productv1.ListOfListOfUser_List{
					Items: []*productv1.ListOfUser{
						{List: &productv1.ListOfUser_List{
							Items: []*productv1.User{
								{Id: "team-complex-1", Name: "Senior Engineer A"},
								{Id: "team-complex-2", Name: "Senior Engineer B"},
							},
						}},
						{List: &productv1.ListOfUser_List{
							Items: []*productv1.User{
								{Id: "team-complex-3", Name: "QA Lead"},
							},
						}},
					},
				},
			},
		}
	default:
		// Generic response for any other ID
		result = &productv1.BlogPost{
			Id:         id,
			Title:      fmt.Sprintf("Blog Post %s", id),
			Content:    fmt.Sprintf("Content for blog post %s", id),
			Tags:       []string{fmt.Sprintf("tag-%s", id), "general"},
			Categories: []string{"General", fmt.Sprintf("Category-%s", id)},
			ViewCounts: []int32{int32(len(id) * 10), int32(len(id) * 20)},
			// Required nested lists must have data
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("tag-%s", id), "group"},
						}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("topic-%s", id)},
						}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Comment on %s", id)},
						}},
					},
				},
			},
			// Required complex lists must have data
			RelatedCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-%s", id), Name: fmt.Sprintf("Category %s", id), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			Contributors: []*productv1.User{
				{Id: fmt.Sprintf("user-%s", id), Name: fmt.Sprintf("Author %s", id)},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-group-%s", id), Name: fmt.Sprintf("Group Category %s", id), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
		}
	}

	return &productv1.QueryBlogPostByIdResponse{
		BlogPostById: result,
	}, nil
}

func (s *MockService) QueryBlogPostsWithFilter(ctx context.Context, in *productv1.QueryBlogPostsWithFilterRequest) (*productv1.QueryBlogPostsWithFilterResponse, error) {
	filter := in.GetFilter()
	var results []*productv1.BlogPost

	// If no filter provided, return empty results
	if filter == nil {
		return &productv1.QueryBlogPostsWithFilterResponse{
			BlogPostsWithFilter: results,
		}, nil
	}

	titleFilter := ""
	if filter.Title != nil {
		titleFilter = filter.Title.GetValue()
	}

	hasCategories := false
	if filter.HasCategories != nil {
		hasCategories = filter.HasCategories.GetValue()
	}

	minTags := int32(0)
	if filter.MinTags != nil {
		minTags = filter.MinTags.GetValue()
	}

	// Generate filtered results
	for i := 1; i <= 3; i++ {
		title := fmt.Sprintf("Filtered Post %d", i)
		if titleFilter != "" {
			title = fmt.Sprintf("%s - Post %d", titleFilter, i)
		}

		var tags []string
		tagsCount := minTags + int32(i)
		for j := int32(0); j < tagsCount; j++ {
			tags = append(tags, fmt.Sprintf("tag%d", j+1))
		}

		var categories []string
		if hasCategories {
			categories = []string{fmt.Sprintf("Category%d", i), "Filtered"}
		}

		results = append(results, &productv1.BlogPost{
			Id:         fmt.Sprintf("filtered-blog-%d", i),
			Title:      title,
			Content:    fmt.Sprintf("Filtered content %d", i),
			Tags:       tags,
			Categories: categories,
			ViewCounts: []int32{int32(i * 100)},
			// Required nested lists must have data
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("filtered-tag-%d", i)},
						}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("filtered-topic-%d", i)},
						}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Filtered comment %d", i)},
						}},
					},
				},
			},
			// Required complex lists must have data
			RelatedCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-filtered-%d", i), Name: fmt.Sprintf("Filtered Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			Contributors: []*productv1.User{
				{Id: fmt.Sprintf("user-filtered-%d", i), Name: fmt.Sprintf("Filtered Author %d", i)},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-group-filtered-%d", i), Name: fmt.Sprintf("Filtered Group %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
		})
	}

	return &productv1.QueryBlogPostsWithFilterResponse{
		BlogPostsWithFilter: results,
	}, nil
}

func (s *MockService) QueryAllBlogPosts(ctx context.Context, in *productv1.QueryAllBlogPostsRequest) (*productv1.QueryAllBlogPostsResponse, error) {
	var results []*productv1.BlogPost

	// Create a variety of blog posts
	for i := 1; i <= 4; i++ {
		var optionalTags *productv1.ListOfString
		var keywords *productv1.ListOfString
		var ratings *productv1.ListOfFloat

		// Vary the optional fields
		if i%2 == 1 {
			optionalTags = &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{fmt.Sprintf("optional%d", i), "common"},
				},
			}
		}

		if i%3 == 0 {
			keywords = &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{fmt.Sprintf("keyword%d", i)},
				},
			}
		}

		if i%2 == 0 {
			ratings = &productv1.ListOfFloat{
				List: &productv1.ListOfFloat_List{
					Items: []float64{float64(i) + 0.5, float64(i) + 1.0},
				},
			}
		}

		results = append(results, &productv1.BlogPost{
			Id:           fmt.Sprintf("blog-%d", i),
			Title:        fmt.Sprintf("Blog Post %d", i),
			Content:      fmt.Sprintf("Content for blog post %d", i),
			Tags:         []string{fmt.Sprintf("tag%d", i), "common"},
			OptionalTags: optionalTags,
			Categories:   []string{fmt.Sprintf("Category%d", i)},
			Keywords:     keywords,
			ViewCounts:   []int32{int32(i * 100), int32(i * 150)},
			Ratings:      ratings,
			IsPublished: &productv1.ListOfBoolean{
				List: &productv1.ListOfBoolean_List{
					Items: []bool{i%2 == 0, true},
				},
			},
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("group%d", i), "shared"},
						}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("topic%d", i)},
						}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Comment for post %d", i)},
						}},
					},
				},
			},
			// Required complex lists must have data
			RelatedCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-all-%d", i), Name: fmt.Sprintf("Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			Contributors: []*productv1.User{
				{Id: fmt.Sprintf("user-all-%d", i), Name: fmt.Sprintf("Author %d", i)},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-group-all-%d", i), Name: fmt.Sprintf("Group Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty
			Suggestions: &productv1.ListOfListOfString{},
		})
	}

	return &productv1.QueryAllBlogPostsResponse{
		AllBlogPosts: results,
	}, nil
}

// Author query implementations
func (s *MockService) QueryAuthor(ctx context.Context, in *productv1.QueryAuthorRequest) (*productv1.QueryAuthorResponse, error) {
	result := &productv1.Author{
		Id:   "author-default",
		Name: "Default Author",
		Email: &wrapperspb.StringValue{
			Value: "author@example.com",
		},
		Skills:    []string{"Go", "GraphQL", "Protocol Buffers"},
		Languages: []string{"English", "Spanish", ""},
		SocialLinks: &productv1.ListOfString{
			List: &productv1.ListOfString_List{
				Items: []string{"https://twitter.com/author", "https://linkedin.com/in/author"},
			},
		},
		TeamsByProject: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{
						Items: []string{"Alice", "Bob", "Charlie"},
					}},
					{List: &productv1.ListOfString_List{
						Items: []string{"David", "Eve"},
					}},
				},
			},
		},
		Collaborations: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{
						Items: []string{"Open Source Project A", "Research Paper B"},
					}},
					{List: &productv1.ListOfString_List{
						Items: []string{"Conference Talk C"},
					}},
				},
			},
		},
		WrittenPosts: &productv1.ListOfBlogPost{
			List: &productv1.ListOfBlogPost_List{
				Items: []*productv1.BlogPost{
					{Id: "blog-1", Title: "GraphQL Best Practices", Content: "Content here..."},
					{Id: "blog-2", Title: "gRPC vs REST", Content: "Comparison content..."},
				},
			},
		},
		FavoriteCategories: []*productv1.Category{
			{Id: "cat-fav-1", Name: "Software Engineering", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
			{Id: "cat-fav-2", Name: "Technical Writing", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
		},
		RelatedAuthors: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "author-rel-1", Name: "Related Author One"},
					{Id: "author-rel-2", Name: "Related Author Two"},
				},
			},
		},
		ProductReviews: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "prod-rev-1", Name: "Code Editor Pro", Price: 199.99},
				},
			},
		},
		AuthorGroups: &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "group-auth-1", Name: "Team Lead Alpha"},
							{Id: "group-auth-2", Name: "Senior Dev Beta"},
						},
					}},
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "group-auth-3", Name: "Junior Dev Gamma"},
						},
					}},
					// empty list
					{List: &productv1.ListOfUser_List{}},
					// null item
					nil,
				},
			},
		},
		CategoryPreferences: &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{
					{List: &productv1.ListOfCategory_List{
						Items: []*productv1.Category{
							{Id: "pref-cat-1", Name: "Microservices", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
							{Id: "pref-cat-2", Name: "Cloud Computing", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
						},
					}},
				},
			},
		},
	}

	return &productv1.QueryAuthorResponse{
		Author: result,
	}, nil
}

func (s *MockService) QueryAuthorById(ctx context.Context, in *productv1.QueryAuthorByIdRequest) (*productv1.QueryAuthorByIdResponse, error) {
	id := in.GetId()

	// Return null for specific test IDs
	if id == "not-found" {
		return &productv1.QueryAuthorByIdResponse{
			AuthorById: nil,
		}, nil
	}

	var result *productv1.Author

	switch id {
	case "minimal":
		result = &productv1.Author{
			Id:        id,
			Name:      "Minimal Author",
			Skills:    []string{"Basic"},
			Languages: []string{"English"},
			TeamsByProject: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{"Solo"},
						}},
					},
				},
			},
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: "cat-minimal", Name: "Basic Category", Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-pref-minimal", Name: "Minimal Preference", Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty
			Collaborations: &productv1.ListOfListOfString{},
		}
	case "experienced":
		result = &productv1.Author{
			Id:   id,
			Name: "Experienced Author",
			Email: &wrapperspb.StringValue{
				Value: "experienced@example.com",
			},
			Skills:    []string{"Go", "GraphQL", "gRPC", "Microservices", "Kubernetes"},
			Languages: []string{"English", "French", "German"},
			SocialLinks: &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{
						"https://github.com/experienced",
						"https://twitter.com/experienced",
						"https://medium.com/@experienced",
					},
				},
			},
			TeamsByProject: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{"Senior Dev 1", "Senior Dev 2", "Tech Lead"},
						}},
						{List: &productv1.ListOfString_List{
							Items: []string{"Architect", "Principal Engineer"},
						}},
						{List: &productv1.ListOfString_List{
							Items: []string{"PM", "Designer", "QA Lead"},
						}},
					},
				},
			},
			Collaborations: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{"Major OSS Project", "Industry Standard", "Research Initiative"},
						}},
						{List: &productv1.ListOfString_List{
							Items: []string{"Conference Keynote", "Workshop Series"},
						}},
					},
				},
			},
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: "cat-experienced-1", Name: "Advanced Programming", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
				{Id: "cat-experienced-2", Name: "Technical Leadership", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-pref-experienced-1", Name: "System Architecture", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
								{Id: "cat-pref-experienced-2", Name: "Team Management", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
							},
						}},
					},
				},
			},
		}
	default:
		result = &productv1.Author{
			Id:   id,
			Name: fmt.Sprintf("Author %s", id),
			Email: &wrapperspb.StringValue{
				Value: fmt.Sprintf("%s@example.com", id),
			},
			Skills:    []string{fmt.Sprintf("Skill-%s", id), "General"},
			Languages: []string{"English", fmt.Sprintf("Language-%s", id)},
			TeamsByProject: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Team-%s", id)},
						}},
					},
				},
			},
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-%s", id), Name: fmt.Sprintf("Favorite Category %s", id), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-pref-%s", id), Name: fmt.Sprintf("Preference %s", id), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty
			Collaborations: &productv1.ListOfListOfString{},
		}
	}

	return &productv1.QueryAuthorByIdResponse{
		AuthorById: result,
	}, nil
}

func (s *MockService) QueryAuthorsWithFilter(ctx context.Context, in *productv1.QueryAuthorsWithFilterRequest) (*productv1.QueryAuthorsWithFilterResponse, error) {
	filter := in.GetFilter()
	var results []*productv1.Author

	if filter == nil {
		return &productv1.QueryAuthorsWithFilterResponse{
			AuthorsWithFilter: results,
		}, nil
	}

	nameFilter := ""
	if filter.Name != nil {
		nameFilter = filter.Name.GetValue()
	}

	hasTeams := false
	if filter.HasTeams != nil {
		hasTeams = filter.HasTeams.GetValue()
	}

	skillCount := int32(0)
	if filter.SkillCount != nil {
		skillCount = filter.SkillCount.GetValue()
	}

	// Generate filtered results
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("Filtered Author %d", i)
		if nameFilter != "" {
			name = fmt.Sprintf("%s - Author %d", nameFilter, i)
		}

		var skills []string
		skillsNeeded := skillCount + int32(i)
		for j := int32(0); j < skillsNeeded; j++ {
			skills = append(skills, fmt.Sprintf("Skill%d", j+1))
		}

		var teamsByProject *productv1.ListOfListOfString
		if hasTeams {
			teamsByProject = &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Team%d", i), "SharedTeam"},
						}},
					},
				},
			}
		} else {
			teamsByProject = &productv1.ListOfListOfString{}
		}

		results = append(results, &productv1.Author{
			Id:             fmt.Sprintf("filtered-author-%d", i),
			Name:           name,
			Skills:         skills,
			Languages:      []string{"English", fmt.Sprintf("Lang%d", i)},
			TeamsByProject: teamsByProject,
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-filtered-%d", i), Name: fmt.Sprintf("Filtered Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-pref-filtered-%d", i), Name: fmt.Sprintf("Filtered Preference %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty
			Collaborations: &productv1.ListOfListOfString{},
		})
	}

	return &productv1.QueryAuthorsWithFilterResponse{
		AuthorsWithFilter: results,
	}, nil
}

func (s *MockService) QueryAllAuthors(ctx context.Context, in *productv1.QueryAllAuthorsRequest) (*productv1.QueryAllAuthorsResponse, error) {
	var results []*productv1.Author

	for i := 1; i <= 3; i++ {
		var email *wrapperspb.StringValue
		var socialLinks *productv1.ListOfString
		var collaborations *productv1.ListOfListOfString

		if i%2 == 1 {
			email = &wrapperspb.StringValue{
				Value: fmt.Sprintf("author%d@example.com", i),
			}
		}

		if i%3 == 0 {
			socialLinks = &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{fmt.Sprintf("https://github.com/author%d", i)},
				},
			}
		}

		if i == 2 {
			collaborations = &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{"Collaboration A", "Collaboration B"},
						}},
					},
				},
			}
		} else {
			collaborations = &productv1.ListOfListOfString{}
		}

		results = append(results, &productv1.Author{
			Id:          fmt.Sprintf("author-%d", i),
			Name:        fmt.Sprintf("Author %d", i),
			Email:       email,
			Skills:      []string{fmt.Sprintf("Skill%d", i), "Common"},
			Languages:   []string{"English", fmt.Sprintf("Language%d", i)},
			SocialLinks: socialLinks,
			TeamsByProject: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Team%d", i)},
						}},
					},
				},
			},
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-all-%d", i), Name: fmt.Sprintf("All Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-pref-all-%d", i), Name: fmt.Sprintf("All Preference %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty/variable
			Collaborations: collaborations,
		})
	}

	return &productv1.QueryAllAuthorsResponse{
		AllAuthors: results,
	}, nil
}

// BlogPost mutation implementations
func (s *MockService) MutationCreateBlogPost(ctx context.Context, in *productv1.MutationCreateBlogPostRequest) (*productv1.MutationCreateBlogPostResponse, error) {
	input := in.GetInput()

	result := &productv1.BlogPost{
		Id:             fmt.Sprintf("blog-%d", rand.Intn(1000)),
		Title:          input.GetTitle(),
		Content:        input.GetContent(),
		Tags:           input.GetTags(),
		OptionalTags:   input.GetOptionalTags(),
		Categories:     input.GetCategories(),
		Keywords:       input.GetKeywords(),
		ViewCounts:     input.GetViewCounts(),
		Ratings:        input.GetRatings(),
		IsPublished:    input.GetIsPublished(),
		TagGroups:      input.GetTagGroups(),
		RelatedTopics:  input.GetRelatedTopics(),
		CommentThreads: input.GetCommentThreads(),
		Suggestions:    input.GetSuggestions(),
		// Convert input types to output types
		RelatedCategories: convertCategoryInputListToCategories(input.GetRelatedCategories()),
		Contributors:      convertUserInputsToUsers(input.GetContributors()),
		CategoryGroups:    convertNestedCategoryInputsToCategories(input.GetCategoryGroups()),
		MentionedProducts: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "prod-1", Name: "Sample Product", Price: 99.99},
				},
			},
		},
		MentionedUsers: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "user-3", Name: "Bob Johnson"},
				},
			},
		},
		ContributorTeams: &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "user-4", Name: "Alice Brown"},
						},
					}},
				},
			},
		},
	}

	return &productv1.MutationCreateBlogPostResponse{
		CreateBlogPost: result,
	}, nil
}

func (s *MockService) MutationUpdateBlogPost(ctx context.Context, in *productv1.MutationUpdateBlogPostRequest) (*productv1.MutationUpdateBlogPostResponse, error) {
	id := in.GetId()
	input := in.GetInput()

	if id == "non-existent" {
		return &productv1.MutationUpdateBlogPostResponse{
			UpdateBlogPost: nil,
		}, nil
	}

	result := &productv1.BlogPost{
		Id:             id,
		Title:          input.GetTitle(),
		Content:        input.GetContent(),
		Tags:           input.GetTags(),
		OptionalTags:   input.GetOptionalTags(),
		Categories:     input.GetCategories(),
		Keywords:       input.GetKeywords(),
		ViewCounts:     input.GetViewCounts(),
		Ratings:        input.GetRatings(),
		IsPublished:    input.GetIsPublished(),
		TagGroups:      input.GetTagGroups(),
		RelatedTopics:  input.GetRelatedTopics(),
		CommentThreads: input.GetCommentThreads(),
		Suggestions:    input.GetSuggestions(),
		// Convert input types to output types
		RelatedCategories: convertCategoryInputListToCategories(input.GetRelatedCategories()),
		Contributors:      convertUserInputsToUsers(input.GetContributors()),
		CategoryGroups:    convertNestedCategoryInputsToCategories(input.GetCategoryGroups()),
		MentionedProducts: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "prod-updated", Name: "Updated Product", Price: 149.99},
				},
			},
		},
		MentionedUsers: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "user-updated", Name: "Updated User"},
				},
			},
		},
		ContributorTeams: &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "user-team-updated", Name: "Updated Team Member"},
						},
					}},
				},
			},
		},
	}

	return &productv1.MutationUpdateBlogPostResponse{
		UpdateBlogPost: result,
	}, nil
}

// Author mutation implementations
func (s *MockService) MutationCreateAuthor(ctx context.Context, in *productv1.MutationCreateAuthorRequest) (*productv1.MutationCreateAuthorResponse, error) {
	input := in.GetInput()

	result := &productv1.Author{
		Id:             fmt.Sprintf("author-%d", rand.Intn(1000)),
		Name:           input.GetName(),
		Email:          input.GetEmail(),
		Skills:         input.GetSkills(),
		Languages:      input.GetLanguages(),
		SocialLinks:    input.GetSocialLinks(),
		TeamsByProject: input.GetTeamsByProject(),
		Collaborations: input.GetCollaborations(),
		// Convert input types to output types for complex fields
		FavoriteCategories: convertCategoryInputsToCategories(input.GetFavoriteCategories()),
		AuthorGroups:       convertNestedUserInputsToUsers(input.GetAuthorGroups()),
		ProjectTeams:       convertNestedUserInputsToUsers(input.GetProjectTeams()),
		// Keep other complex fields with mock data since they're not in the simplified input
		WrittenPosts: &productv1.ListOfBlogPost{
			List: &productv1.ListOfBlogPost_List{
				Items: []*productv1.BlogPost{
					{Id: "blog-created", Title: "Created Post", Content: "Content..."},
				},
			},
		},
		RelatedAuthors: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "related-author", Name: "Related Author"},
				},
			},
		},
		ProductReviews: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "reviewed-product", Name: "Code Editor", Price: 199.99},
				},
			},
		},
		CategoryPreferences: &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{
					{List: &productv1.ListOfCategory_List{
						Items: []*productv1.Category{
							{Id: "pref-cat", Name: "Backend Development", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
						},
					}},
				},
			},
		},
	}

	return &productv1.MutationCreateAuthorResponse{
		CreateAuthor: result,
	}, nil
}

func (s *MockService) MutationUpdateAuthor(ctx context.Context, in *productv1.MutationUpdateAuthorRequest) (*productv1.MutationUpdateAuthorResponse, error) {
	id := in.GetId()
	input := in.GetInput()

	if id == "non-existent" {
		return &productv1.MutationUpdateAuthorResponse{
			UpdateAuthor: nil,
		}, nil
	}

	result := &productv1.Author{
		Id:             id,
		Name:           input.GetName(),
		Email:          input.GetEmail(),
		Skills:         input.GetSkills(),
		Languages:      input.GetLanguages(),
		SocialLinks:    input.GetSocialLinks(),
		TeamsByProject: input.GetTeamsByProject(),
		Collaborations: input.GetCollaborations(),
		// Convert input types to output types for complex fields
		FavoriteCategories: convertCategoryInputsToCategories(input.GetFavoriteCategories()),
		AuthorGroups:       convertNestedUserInputsToUsers(input.GetAuthorGroups()),
		ProjectTeams:       convertNestedUserInputsToUsers(input.GetProjectTeams()),
		// Keep other complex fields with mock data since they're not in the simplified input
		WrittenPosts: &productv1.ListOfBlogPost{
			List: &productv1.ListOfBlogPost_List{
				Items: []*productv1.BlogPost{
					{Id: "blog-updated", Title: "Updated Post", Content: "Updated content..."},
				},
			},
		},
		RelatedAuthors: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "related-author-updated", Name: "Updated Related Author"},
				},
			},
		},
		ProductReviews: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "reviewed-product-updated", Name: "Updated Code Editor", Price: 249.99},
				},
			},
		},
		CategoryPreferences: &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{
					{List: &productv1.ListOfCategory_List{
						Items: []*productv1.Category{
							{Id: "pref-cat-updated", Name: "Updated Backend Development", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
						},
					}},
				},
			},
		},
	}

	return &productv1.MutationUpdateAuthorResponse{
		UpdateAuthor: result,
	}, nil
}
