package grpctest

import (
	context "context"
	"fmt"
	"math"
	"math/rand"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

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
