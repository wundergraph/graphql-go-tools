package testdata

import (
	context "context"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata/productv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MockService struct {
	productv1.UnimplementedProductServiceServer
}

func (s *MockService) LookupProductById(ctx context.Context, in *productv1.LookupProductByIdRequest) (*productv1.LookupProductByIdResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method LookupProductById not implemented")
}
func (s *MockService) LookupProductByName(ctx context.Context, in *productv1.LookupProductByNameRequest) (*productv1.LookupProductByNameResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method LookupProductByName not implemented")
}
func (s *MockService) LookupStorageById(ctx context.Context, in *productv1.LookupStorageByIdRequest) (*productv1.LookupStorageByIdResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method LookupStorageById not implemented")
}
func (s *MockService) QueryUsers(ctx context.Context, in *productv1.QueryUsersRequest) (*productv1.QueryUsersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryUsers not implemented")
}
func (s *MockService) QueryUser(ctx context.Context, in *productv1.QueryUserRequest) (*productv1.QueryUserResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryUser not implemented")
}
func (s *MockService) QueryNestedType(ctx context.Context, in *productv1.QueryNestedTypeRequest) (*productv1.QueryNestedTypeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryNestedType not implemented")
}
func (s *MockService) QueryRecursiveType(ctx context.Context, in *productv1.QueryRecursiveTypeRequest) (*productv1.QueryRecursiveTypeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryRecursiveType not implemented")
}
func (s *MockService) QueryTypeFilterWithArguments(ctx context.Context, in *productv1.QueryTypeFilterWithArgumentsRequest) (*productv1.QueryTypeFilterWithArgumentsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryTypeFilterWithArguments not implemented")
}
func (s *MockService) QueryTypeWithMultipleFilterFields(ctx context.Context, in *productv1.QueryTypeWithMultipleFilterFieldsRequest) (*productv1.QueryTypeWithMultipleFilterFieldsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryTypeWithMultipleFilterFields not implemented")
}
func (s *MockService) QueryComplexFilterType(ctx context.Context, in *productv1.QueryComplexFilterTypeRequest) (*productv1.QueryComplexFilterTypeResponse, error) {
	// Return test data
	return &productv1.QueryComplexFilterTypeResponse{
		TypeWithComplexFilterInput: []*productv1.TypeWithComplexFilterInput{
			{
				Id:   "test-id-123",
				Name: in.GetFilter().Filter.GetName(),
			},
		},
	}, nil
}
