package grpctest

import (
	context "context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MockService struct {
	productv1.UnimplementedProductServiceServer
}

func (s *MockService) LookupProductById(ctx context.Context, in *productv1.LookupProductByIdRequest) (*productv1.LookupProductByIdResponse, error) {
	var results []*productv1.LookupProductByIdResult

	for _, input := range in.GetInputs() {
		productId := input.GetKey().GetId()
		results = append(results, &productv1.LookupProductByIdResult{
			Product: &productv1.Product{
				Id:    productId,
				Name:  fmt.Sprintf("Product %s", productId),
				Price: 99.99,
			},
		})
	}

	return &productv1.LookupProductByIdResponse{
		Results: results,
	}, nil
}

func (s *MockService) LookupProductByName(ctx context.Context, in *productv1.LookupProductByNameRequest) (*productv1.LookupProductByNameResponse, error) {
	var results []*productv1.LookupProductByNameResult

	for _, input := range in.GetInputs() {
		productName := input.GetName()
		results = append(results, &productv1.LookupProductByNameResult{
			Product: &productv1.Product{
				Id:    fmt.Sprintf("id-for-%s", strings.ReplaceAll(productName, " ", "-")),
				Name:  productName,
				Price: 49.99,
			},
		})
	}

	return &productv1.LookupProductByNameResponse{
		Results: results,
	}, nil
}

func (s *MockService) LookupStorageById(ctx context.Context, in *productv1.LookupStorageByIdRequest) (*productv1.LookupStorageByIdResponse, error) {
	var results []*productv1.LookupStorageByIdResult

	for _, input := range in.GetInputs() {
		storageId := input.GetKey().GetId()
		results = append(results, &productv1.LookupStorageByIdResult{
			Storage: &productv1.Storage{
				Id:       storageId,
				Name:     fmt.Sprintf("Storage %s", storageId),
				Location: fmt.Sprintf("Location %d", rand.Intn(100)),
			},
		})
	}

	return &productv1.LookupStorageByIdResponse{
		Results: results,
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
	filterField1 := in.GetFilterField1()
	filterField2 := in.GetFilterField2()

	var fields []*productv1.TypeWithMultipleFilterFields

	// Create results that echo the filter values
	for i := 1; i <= 2; i++ {
		fields = append(fields, &productv1.TypeWithMultipleFilterFields{
			Id:           fmt.Sprintf("multi-filter-%d", i),
			Name:         fmt.Sprintf("MultiFilter %d", i),
			FilterField1: filterField1,
			FilterField2: filterField2,
		})
	}

	return &productv1.QueryTypeFilterWithArgumentsResponse{
		TypeWithMultipleFilterFields: fields,
	}, nil
}

func (s *MockService) QueryTypeWithMultipleFilterFields(ctx context.Context, in *productv1.QueryTypeWithMultipleFilterFieldsRequest) (*productv1.QueryTypeWithMultipleFilterFieldsResponse, error) {
	filter := in.GetFilter()

	var fields []*productv1.TypeWithMultipleFilterFields

	// Echo the filter values in the results
	for i := 1; i <= 2; i++ {
		fields = append(fields, &productv1.TypeWithMultipleFilterFields{
			Id:           fmt.Sprintf("filtered-%d", i),
			Name:         filter.GetName() + " " + strconv.Itoa(i),
			FilterField1: filter.GetFilterField1(),
			FilterField2: filter.GetFilterField2(),
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
