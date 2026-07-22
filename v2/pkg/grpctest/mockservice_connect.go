// Manually maintained adapter that wraps the gRPC MockService onto the
// ConnectRPC handler interface emitted by protoc-gen-connect-go. Update by
// hand when MockService method signatures change; there is intentionally no
// code generator because the adapter is a pure passthrough.

package grpctest

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/grpc/metadata"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1/productv1connect"
)

// connectCtxToGRPC bridges Connect HTTP headers onto a gRPC-style
// incoming metadata context so the underlying MockService (which reads
// via metadata.FromIncomingContext) can recover header values regardless
// of whether the request arrived over native gRPC or ConnectRPC. Keys
// are lowercased because grpc/metadata.MD normalises that way and the
// MockService looks them up via the lowercase form.
func connectCtxToGRPC(ctx context.Context, headers http.Header) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	md := metadata.MD{}
	for k, vs := range headers {
		md[strings.ToLower(k)] = append([]string(nil), vs...)
	}
	return metadata.NewIncomingContext(ctx, md)
}

// MockServiceConnect adapts the gRPC MockService onto the ConnectRPC
// handler interface emitted by protoc-gen-connect-go. The same backing
// implementation can therefore serve Connect, gRPC, and gRPC-Web from a
// single HTTP handler, which lets the grpc_datasource tests exercise the
// data source against either transport without duplicating fixtures.
type MockServiceConnect struct {
	productv1connect.UnimplementedProductServiceHandler

	inner *MockService
}

// NewMockServiceConnect wraps the supplied MockService.
func NewMockServiceConnect(inner *MockService) *MockServiceConnect {
	return &MockServiceConnect{inner: inner}
}

var _ productv1connect.ProductServiceHandler = (*MockServiceConnect)(nil)

// LookupProductById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) LookupProductById(ctx context.Context, req *connect.Request[productv1.LookupProductByIdRequest]) (*connect.Response[productv1.LookupProductByIdResponse], error) {
	resp, err := s.inner.LookupProductById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// LookupStorageById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) LookupStorageById(ctx context.Context, req *connect.Request[productv1.LookupStorageByIdRequest]) (*connect.Response[productv1.LookupStorageByIdResponse], error) {
	resp, err := s.inner.LookupStorageById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// LookupWarehouseById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) LookupWarehouseById(ctx context.Context, req *connect.Request[productv1.LookupWarehouseByIdRequest]) (*connect.Response[productv1.LookupWarehouseByIdResponse], error) {
	resp, err := s.inner.LookupWarehouseById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationBulkCreateAuthors forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationBulkCreateAuthors(ctx context.Context, req *connect.Request[productv1.MutationBulkCreateAuthorsRequest]) (*connect.Response[productv1.MutationBulkCreateAuthorsResponse], error) {
	resp, err := s.inner.MutationBulkCreateAuthors(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationBulkCreateBlogPosts forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationBulkCreateBlogPosts(ctx context.Context, req *connect.Request[productv1.MutationBulkCreateBlogPostsRequest]) (*connect.Response[productv1.MutationBulkCreateBlogPostsResponse], error) {
	resp, err := s.inner.MutationBulkCreateBlogPosts(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationBulkUpdateAuthors forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationBulkUpdateAuthors(ctx context.Context, req *connect.Request[productv1.MutationBulkUpdateAuthorsRequest]) (*connect.Response[productv1.MutationBulkUpdateAuthorsResponse], error) {
	resp, err := s.inner.MutationBulkUpdateAuthors(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationBulkUpdateBlogPosts forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationBulkUpdateBlogPosts(ctx context.Context, req *connect.Request[productv1.MutationBulkUpdateBlogPostsRequest]) (*connect.Response[productv1.MutationBulkUpdateBlogPostsResponse], error) {
	resp, err := s.inner.MutationBulkUpdateBlogPosts(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationCreateAuthor forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationCreateAuthor(ctx context.Context, req *connect.Request[productv1.MutationCreateAuthorRequest]) (*connect.Response[productv1.MutationCreateAuthorResponse], error) {
	resp, err := s.inner.MutationCreateAuthor(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationCreateBlogPost forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationCreateBlogPost(ctx context.Context, req *connect.Request[productv1.MutationCreateBlogPostRequest]) (*connect.Response[productv1.MutationCreateBlogPostResponse], error) {
	resp, err := s.inner.MutationCreateBlogPost(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationCreateNullableFieldsType forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationCreateNullableFieldsType(ctx context.Context, req *connect.Request[productv1.MutationCreateNullableFieldsTypeRequest]) (*connect.Response[productv1.MutationCreateNullableFieldsTypeResponse], error) {
	resp, err := s.inner.MutationCreateNullableFieldsType(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationCreateUser forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationCreateUser(ctx context.Context, req *connect.Request[productv1.MutationCreateUserRequest]) (*connect.Response[productv1.MutationCreateUserResponse], error) {
	resp, err := s.inner.MutationCreateUser(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationPerformAction forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationPerformAction(ctx context.Context, req *connect.Request[productv1.MutationPerformActionRequest]) (*connect.Response[productv1.MutationPerformActionResponse], error) {
	resp, err := s.inner.MutationPerformAction(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationUpdateAuthor forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationUpdateAuthor(ctx context.Context, req *connect.Request[productv1.MutationUpdateAuthorRequest]) (*connect.Response[productv1.MutationUpdateAuthorResponse], error) {
	resp, err := s.inner.MutationUpdateAuthor(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationUpdateBlogPost forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationUpdateBlogPost(ctx context.Context, req *connect.Request[productv1.MutationUpdateBlogPostRequest]) (*connect.Response[productv1.MutationUpdateBlogPostResponse], error) {
	resp, err := s.inner.MutationUpdateBlogPost(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// MutationUpdateNullableFieldsType forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) MutationUpdateNullableFieldsType(ctx context.Context, req *connect.Request[productv1.MutationUpdateNullableFieldsTypeRequest]) (*connect.Response[productv1.MutationUpdateNullableFieldsTypeResponse], error) {
	resp, err := s.inner.MutationUpdateNullableFieldsType(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryAllAuthors forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryAllAuthors(ctx context.Context, req *connect.Request[productv1.QueryAllAuthorsRequest]) (*connect.Response[productv1.QueryAllAuthorsResponse], error) {
	resp, err := s.inner.QueryAllAuthors(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryAllBlogPosts forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryAllBlogPosts(ctx context.Context, req *connect.Request[productv1.QueryAllBlogPostsRequest]) (*connect.Response[productv1.QueryAllBlogPostsResponse], error) {
	resp, err := s.inner.QueryAllBlogPosts(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryAllNullableFieldsTypes forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryAllNullableFieldsTypes(ctx context.Context, req *connect.Request[productv1.QueryAllNullableFieldsTypesRequest]) (*connect.Response[productv1.QueryAllNullableFieldsTypesResponse], error) {
	resp, err := s.inner.QueryAllNullableFieldsTypes(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryAllPets forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryAllPets(ctx context.Context, req *connect.Request[productv1.QueryAllPetsRequest]) (*connect.Response[productv1.QueryAllPetsResponse], error) {
	resp, err := s.inner.QueryAllPets(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryAuthor forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryAuthor(ctx context.Context, req *connect.Request[productv1.QueryAuthorRequest]) (*connect.Response[productv1.QueryAuthorResponse], error) {
	resp, err := s.inner.QueryAuthor(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryAuthorById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryAuthorById(ctx context.Context, req *connect.Request[productv1.QueryAuthorByIdRequest]) (*connect.Response[productv1.QueryAuthorByIdResponse], error) {
	resp, err := s.inner.QueryAuthorById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryAuthorsWithFilter forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryAuthorsWithFilter(ctx context.Context, req *connect.Request[productv1.QueryAuthorsWithFilterRequest]) (*connect.Response[productv1.QueryAuthorsWithFilterResponse], error) {
	resp, err := s.inner.QueryAuthorsWithFilter(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryBlogPost forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryBlogPost(ctx context.Context, req *connect.Request[productv1.QueryBlogPostRequest]) (*connect.Response[productv1.QueryBlogPostResponse], error) {
	resp, err := s.inner.QueryBlogPost(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryBlogPostById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryBlogPostById(ctx context.Context, req *connect.Request[productv1.QueryBlogPostByIdRequest]) (*connect.Response[productv1.QueryBlogPostByIdResponse], error) {
	resp, err := s.inner.QueryBlogPostById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryBlogPostsWithFilter forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryBlogPostsWithFilter(ctx context.Context, req *connect.Request[productv1.QueryBlogPostsWithFilterRequest]) (*connect.Response[productv1.QueryBlogPostsWithFilterResponse], error) {
	resp, err := s.inner.QueryBlogPostsWithFilter(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryBulkSearchAuthors forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryBulkSearchAuthors(ctx context.Context, req *connect.Request[productv1.QueryBulkSearchAuthorsRequest]) (*connect.Response[productv1.QueryBulkSearchAuthorsResponse], error) {
	resp, err := s.inner.QueryBulkSearchAuthors(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryBulkSearchBlogPosts forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryBulkSearchBlogPosts(ctx context.Context, req *connect.Request[productv1.QueryBulkSearchBlogPostsRequest]) (*connect.Response[productv1.QueryBulkSearchBlogPostsResponse], error) {
	resp, err := s.inner.QueryBulkSearchBlogPosts(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryCalculateTotals forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryCalculateTotals(ctx context.Context, req *connect.Request[productv1.QueryCalculateTotalsRequest]) (*connect.Response[productv1.QueryCalculateTotalsResponse], error) {
	resp, err := s.inner.QueryCalculateTotals(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryCategories forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryCategories(ctx context.Context, req *connect.Request[productv1.QueryCategoriesRequest]) (*connect.Response[productv1.QueryCategoriesResponse], error) {
	resp, err := s.inner.QueryCategories(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryCategoriesByKind forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryCategoriesByKind(ctx context.Context, req *connect.Request[productv1.QueryCategoriesByKindRequest]) (*connect.Response[productv1.QueryCategoriesByKindResponse], error) {
	resp, err := s.inner.QueryCategoriesByKind(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryCategoriesByKinds forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryCategoriesByKinds(ctx context.Context, req *connect.Request[productv1.QueryCategoriesByKindsRequest]) (*connect.Response[productv1.QueryCategoriesByKindsResponse], error) {
	resp, err := s.inner.QueryCategoriesByKinds(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryCategory forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryCategory(ctx context.Context, req *connect.Request[productv1.QueryCategoryRequest]) (*connect.Response[productv1.QueryCategoryResponse], error) {
	resp, err := s.inner.QueryCategory(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryComplexFilterType forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryComplexFilterType(ctx context.Context, req *connect.Request[productv1.QueryComplexFilterTypeRequest]) (*connect.Response[productv1.QueryComplexFilterTypeResponse], error) {
	resp, err := s.inner.QueryComplexFilterType(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryConditionalSearch forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryConditionalSearch(ctx context.Context, req *connect.Request[productv1.QueryConditionalSearchRequest]) (*connect.Response[productv1.QueryConditionalSearchResponse], error) {
	resp, err := s.inner.QueryConditionalSearch(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryFilterCategories forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryFilterCategories(ctx context.Context, req *connect.Request[productv1.QueryFilterCategoriesRequest]) (*connect.Response[productv1.QueryFilterCategoriesResponse], error) {
	resp, err := s.inner.QueryFilterCategories(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryNestedType forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryNestedType(ctx context.Context, req *connect.Request[productv1.QueryNestedTypeRequest]) (*connect.Response[productv1.QueryNestedTypeResponse], error) {
	resp, err := s.inner.QueryNestedType(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryNullableFieldsType forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryNullableFieldsType(ctx context.Context, req *connect.Request[productv1.QueryNullableFieldsTypeRequest]) (*connect.Response[productv1.QueryNullableFieldsTypeResponse], error) {
	resp, err := s.inner.QueryNullableFieldsType(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryNullableFieldsTypeById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryNullableFieldsTypeById(ctx context.Context, req *connect.Request[productv1.QueryNullableFieldsTypeByIdRequest]) (*connect.Response[productv1.QueryNullableFieldsTypeByIdResponse], error) {
	resp, err := s.inner.QueryNullableFieldsTypeById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryNullableFieldsTypeWithFilter forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryNullableFieldsTypeWithFilter(ctx context.Context, req *connect.Request[productv1.QueryNullableFieldsTypeWithFilterRequest]) (*connect.Response[productv1.QueryNullableFieldsTypeWithFilterResponse], error) {
	resp, err := s.inner.QueryNullableFieldsTypeWithFilter(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryRandomPet forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryRandomPet(ctx context.Context, req *connect.Request[productv1.QueryRandomPetRequest]) (*connect.Response[productv1.QueryRandomPetResponse], error) {
	resp, err := s.inner.QueryRandomPet(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryRandomSearchResult forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryRandomSearchResult(ctx context.Context, req *connect.Request[productv1.QueryRandomSearchResultRequest]) (*connect.Response[productv1.QueryRandomSearchResultResponse], error) {
	resp, err := s.inner.QueryRandomSearchResult(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryRecursiveType forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryRecursiveType(ctx context.Context, req *connect.Request[productv1.QueryRecursiveTypeRequest]) (*connect.Response[productv1.QueryRecursiveTypeResponse], error) {
	resp, err := s.inner.QueryRecursiveType(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QuerySearch forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QuerySearch(ctx context.Context, req *connect.Request[productv1.QuerySearchRequest]) (*connect.Response[productv1.QuerySearchResponse], error) {
	resp, err := s.inner.QuerySearch(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryTestContainer forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryTestContainer(ctx context.Context, req *connect.Request[productv1.QueryTestContainerRequest]) (*connect.Response[productv1.QueryTestContainerResponse], error) {
	resp, err := s.inner.QueryTestContainer(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryTestContainers forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryTestContainers(ctx context.Context, req *connect.Request[productv1.QueryTestContainersRequest]) (*connect.Response[productv1.QueryTestContainersResponse], error) {
	resp, err := s.inner.QueryTestContainers(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryTypeFilterWithArguments forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryTypeFilterWithArguments(ctx context.Context, req *connect.Request[productv1.QueryTypeFilterWithArgumentsRequest]) (*connect.Response[productv1.QueryTypeFilterWithArgumentsResponse], error) {
	resp, err := s.inner.QueryTypeFilterWithArguments(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryTypeWithMultipleFilterFields forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryTypeWithMultipleFilterFields(ctx context.Context, req *connect.Request[productv1.QueryTypeWithMultipleFilterFieldsRequest]) (*connect.Response[productv1.QueryTypeWithMultipleFilterFieldsResponse], error) {
	resp, err := s.inner.QueryTypeWithMultipleFilterFields(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryUser forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryUser(ctx context.Context, req *connect.Request[productv1.QueryUserRequest]) (*connect.Response[productv1.QueryUserResponse], error) {
	resp, err := s.inner.QueryUser(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// QueryUsers forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) QueryUsers(ctx context.Context, req *connect.Request[productv1.QueryUsersRequest]) (*connect.Response[productv1.QueryUsersResponse], error) {
	resp, err := s.inner.QueryUsers(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageCategoryInfoSummaryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageCategoryInfoSummaryById(ctx context.Context, req *connect.Request[productv1.RequireStorageCategoryInfoSummaryByIdRequest]) (*connect.Response[productv1.RequireStorageCategoryInfoSummaryByIdResponse], error) {
	resp, err := s.inner.RequireStorageCategoryInfoSummaryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageDeepItemInfoById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageDeepItemInfoById(ctx context.Context, req *connect.Request[productv1.RequireStorageDeepItemInfoByIdRequest]) (*connect.Response[productv1.RequireStorageDeepItemInfoByIdResponse], error) {
	resp, err := s.inner.RequireStorageDeepItemInfoById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageFilteredTagSummaryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageFilteredTagSummaryById(ctx context.Context, req *connect.Request[productv1.RequireStorageFilteredTagSummaryByIdRequest]) (*connect.Response[productv1.RequireStorageFilteredTagSummaryByIdResponse], error) {
	resp, err := s.inner.RequireStorageFilteredTagSummaryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageItemHandlerInfoById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageItemHandlerInfoById(ctx context.Context, req *connect.Request[productv1.RequireStorageItemHandlerInfoByIdRequest]) (*connect.Response[productv1.RequireStorageItemHandlerInfoByIdResponse], error) {
	resp, err := s.inner.RequireStorageItemHandlerInfoById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageItemInfoById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageItemInfoById(ctx context.Context, req *connect.Request[productv1.RequireStorageItemInfoByIdRequest]) (*connect.Response[productv1.RequireStorageItemInfoByIdResponse], error) {
	resp, err := s.inner.RequireStorageItemInfoById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageItemSpecsInfoById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageItemSpecsInfoById(ctx context.Context, req *connect.Request[productv1.RequireStorageItemSpecsInfoByIdRequest]) (*connect.Response[productv1.RequireStorageItemSpecsInfoByIdResponse], error) {
	resp, err := s.inner.RequireStorageItemSpecsInfoById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageKindSummaryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageKindSummaryById(ctx context.Context, req *connect.Request[productv1.RequireStorageKindSummaryByIdRequest]) (*connect.Response[productv1.RequireStorageKindSummaryByIdResponse], error) {
	resp, err := s.inner.RequireStorageKindSummaryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageMetadataScoreById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageMetadataScoreById(ctx context.Context, req *connect.Request[productv1.RequireStorageMetadataScoreByIdRequest]) (*connect.Response[productv1.RequireStorageMetadataScoreByIdResponse], error) {
	resp, err := s.inner.RequireStorageMetadataScoreById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageMultiFilteredTagSummaryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageMultiFilteredTagSummaryById(ctx context.Context, req *connect.Request[productv1.RequireStorageMultiFilteredTagSummaryByIdRequest]) (*connect.Response[productv1.RequireStorageMultiFilteredTagSummaryByIdResponse], error) {
	resp, err := s.inner.RequireStorageMultiFilteredTagSummaryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageNullableFilteredTagSummaryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageNullableFilteredTagSummaryById(ctx context.Context, req *connect.Request[productv1.RequireStorageNullableFilteredTagSummaryByIdRequest]) (*connect.Response[productv1.RequireStorageNullableFilteredTagSummaryByIdResponse], error) {
	resp, err := s.inner.RequireStorageNullableFilteredTagSummaryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageOperationReportById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageOperationReportById(ctx context.Context, req *connect.Request[productv1.RequireStorageOperationReportByIdRequest]) (*connect.Response[productv1.RequireStorageOperationReportByIdResponse], error) {
	resp, err := s.inner.RequireStorageOperationReportById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageOptionalProcessedMetadataById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageOptionalProcessedMetadataById(ctx context.Context, req *connect.Request[productv1.RequireStorageOptionalProcessedMetadataByIdRequest]) (*connect.Response[productv1.RequireStorageOptionalProcessedMetadataByIdResponse], error) {
	resp, err := s.inner.RequireStorageOptionalProcessedMetadataById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageOptionalProcessedTagsById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageOptionalProcessedTagsById(ctx context.Context, req *connect.Request[productv1.RequireStorageOptionalProcessedTagsByIdRequest]) (*connect.Response[productv1.RequireStorageOptionalProcessedTagsByIdResponse], error) {
	resp, err := s.inner.RequireStorageOptionalProcessedTagsById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageOptionalTagSummaryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageOptionalTagSummaryById(ctx context.Context, req *connect.Request[productv1.RequireStorageOptionalTagSummaryByIdRequest]) (*connect.Response[productv1.RequireStorageOptionalTagSummaryByIdResponse], error) {
	resp, err := s.inner.RequireStorageOptionalTagSummaryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageProcessedMetadataById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageProcessedMetadataById(ctx context.Context, req *connect.Request[productv1.RequireStorageProcessedMetadataByIdRequest]) (*connect.Response[productv1.RequireStorageProcessedMetadataByIdResponse], error) {
	resp, err := s.inner.RequireStorageProcessedMetadataById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageProcessedMetadataHistoryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageProcessedMetadataHistoryById(ctx context.Context, req *connect.Request[productv1.RequireStorageProcessedMetadataHistoryByIdRequest]) (*connect.Response[productv1.RequireStorageProcessedMetadataHistoryByIdResponse], error) {
	resp, err := s.inner.RequireStorageProcessedMetadataHistoryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageProcessedTagsById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageProcessedTagsById(ctx context.Context, req *connect.Request[productv1.RequireStorageProcessedTagsByIdRequest]) (*connect.Response[productv1.RequireStorageProcessedTagsByIdResponse], error) {
	resp, err := s.inner.RequireStorageProcessedTagsById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageSecuritySummaryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageSecuritySummaryById(ctx context.Context, req *connect.Request[productv1.RequireStorageSecuritySummaryByIdRequest]) (*connect.Response[productv1.RequireStorageSecuritySummaryByIdResponse], error) {
	resp, err := s.inner.RequireStorageSecuritySummaryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageStockHealthScoreById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageStockHealthScoreById(ctx context.Context, req *connect.Request[productv1.RequireStorageStockHealthScoreByIdRequest]) (*connect.Response[productv1.RequireStorageStockHealthScoreByIdResponse], error) {
	resp, err := s.inner.RequireStorageStockHealthScoreById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireStorageTagSummaryById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireStorageTagSummaryById(ctx context.Context, req *connect.Request[productv1.RequireStorageTagSummaryByIdRequest]) (*connect.Response[productv1.RequireStorageTagSummaryByIdResponse], error) {
	resp, err := s.inner.RequireStorageTagSummaryById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// RequireWarehouseStockHealthScoreById forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) RequireWarehouseStockHealthScoreById(ctx context.Context, req *connect.Request[productv1.RequireWarehouseStockHealthScoreByIdRequest]) (*connect.Response[productv1.RequireWarehouseStockHealthScoreByIdResponse], error) {
	resp, err := s.inner.RequireWarehouseStockHealthScoreById(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryActiveSubcategories forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryActiveSubcategories(ctx context.Context, req *connect.Request[productv1.ResolveCategoryActiveSubcategoriesRequest]) (*connect.Response[productv1.ResolveCategoryActiveSubcategoriesResponse], error) {
	resp, err := s.inner.ResolveCategoryActiveSubcategories(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryCategoryMetrics forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryCategoryMetrics(ctx context.Context, req *connect.Request[productv1.ResolveCategoryCategoryMetricsRequest]) (*connect.Response[productv1.ResolveCategoryCategoryMetricsResponse], error) {
	resp, err := s.inner.ResolveCategoryCategoryMetrics(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryCategoryStatus forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryCategoryStatus(ctx context.Context, req *connect.Request[productv1.ResolveCategoryCategoryStatusRequest]) (*connect.Response[productv1.ResolveCategoryCategoryStatusResponse], error) {
	resp, err := s.inner.ResolveCategoryCategoryStatus(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryChildCategories forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryChildCategories(ctx context.Context, req *connect.Request[productv1.ResolveCategoryChildCategoriesRequest]) (*connect.Response[productv1.ResolveCategoryChildCategoriesResponse], error) {
	resp, err := s.inner.ResolveCategoryChildCategories(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryMascot forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryMascot(ctx context.Context, req *connect.Request[productv1.ResolveCategoryMascotRequest]) (*connect.Response[productv1.ResolveCategoryMascotResponse], error) {
	resp, err := s.inner.ResolveCategoryMascot(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryMetricsAverageScore forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryMetricsAverageScore(ctx context.Context, req *connect.Request[productv1.ResolveCategoryMetricsAverageScoreRequest]) (*connect.Response[productv1.ResolveCategoryMetricsAverageScoreResponse], error) {
	resp, err := s.inner.ResolveCategoryMetricsAverageScore(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryMetricsNormalizedScore forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryMetricsNormalizedScore(ctx context.Context, req *connect.Request[productv1.ResolveCategoryMetricsNormalizedScoreRequest]) (*connect.Response[productv1.ResolveCategoryMetricsNormalizedScoreResponse], error) {
	resp, err := s.inner.ResolveCategoryMetricsNormalizedScore(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryMetricsRelatedCategory forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryMetricsRelatedCategory(ctx context.Context, req *connect.Request[productv1.ResolveCategoryMetricsRelatedCategoryRequest]) (*connect.Response[productv1.ResolveCategoryMetricsRelatedCategoryResponse], error) {
	resp, err := s.inner.ResolveCategoryMetricsRelatedCategory(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryOptionalCategories forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryOptionalCategories(ctx context.Context, req *connect.Request[productv1.ResolveCategoryOptionalCategoriesRequest]) (*connect.Response[productv1.ResolveCategoryOptionalCategoriesResponse], error) {
	resp, err := s.inner.ResolveCategoryOptionalCategories(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryPopularityScore forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryPopularityScore(ctx context.Context, req *connect.Request[productv1.ResolveCategoryPopularityScoreRequest]) (*connect.Response[productv1.ResolveCategoryPopularityScoreResponse], error) {
	resp, err := s.inner.ResolveCategoryPopularityScore(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryProductCount forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryProductCount(ctx context.Context, req *connect.Request[productv1.ResolveCategoryProductCountRequest]) (*connect.Response[productv1.ResolveCategoryProductCountResponse], error) {
	resp, err := s.inner.ResolveCategoryProductCount(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryTopSubcategory forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryTopSubcategory(ctx context.Context, req *connect.Request[productv1.ResolveCategoryTopSubcategoryRequest]) (*connect.Response[productv1.ResolveCategoryTopSubcategoryResponse], error) {
	resp, err := s.inner.ResolveCategoryTopSubcategory(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveCategoryTotalProducts forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveCategoryTotalProducts(ctx context.Context, req *connect.Request[productv1.ResolveCategoryTotalProductsRequest]) (*connect.Response[productv1.ResolveCategoryTotalProductsResponse], error) {
	resp, err := s.inner.ResolveCategoryTotalProducts(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveProductMascotRecommendation forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveProductMascotRecommendation(ctx context.Context, req *connect.Request[productv1.ResolveProductMascotRecommendationRequest]) (*connect.Response[productv1.ResolveProductMascotRecommendationResponse], error) {
	resp, err := s.inner.ResolveProductMascotRecommendation(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveProductProductDetails forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveProductProductDetails(ctx context.Context, req *connect.Request[productv1.ResolveProductProductDetailsRequest]) (*connect.Response[productv1.ResolveProductProductDetailsResponse], error) {
	resp, err := s.inner.ResolveProductProductDetails(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveProductRecommendedCategory forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveProductRecommendedCategory(ctx context.Context, req *connect.Request[productv1.ResolveProductRecommendedCategoryRequest]) (*connect.Response[productv1.ResolveProductRecommendedCategoryResponse], error) {
	resp, err := s.inner.ResolveProductRecommendedCategory(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveProductShippingEstimate forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveProductShippingEstimate(ctx context.Context, req *connect.Request[productv1.ResolveProductShippingEstimateRequest]) (*connect.Response[productv1.ResolveProductShippingEstimateResponse], error) {
	resp, err := s.inner.ResolveProductShippingEstimate(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveProductStockStatus forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveProductStockStatus(ctx context.Context, req *connect.Request[productv1.ResolveProductStockStatusRequest]) (*connect.Response[productv1.ResolveProductStockStatusResponse], error) {
	resp, err := s.inner.ResolveProductStockStatus(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveStorageLinkedStorages forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveStorageLinkedStorages(ctx context.Context, req *connect.Request[productv1.ResolveStorageLinkedStoragesRequest]) (*connect.Response[productv1.ResolveStorageLinkedStoragesResponse], error) {
	resp, err := s.inner.ResolveStorageLinkedStorages(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveStorageNearbyStorages forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveStorageNearbyStorages(ctx context.Context, req *connect.Request[productv1.ResolveStorageNearbyStoragesRequest]) (*connect.Response[productv1.ResolveStorageNearbyStoragesResponse], error) {
	resp, err := s.inner.ResolveStorageNearbyStorages(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveStorageStorageStatus forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveStorageStorageStatus(ctx context.Context, req *connect.Request[productv1.ResolveStorageStorageStatusRequest]) (*connect.Response[productv1.ResolveStorageStorageStatusResponse], error) {
	resp, err := s.inner.ResolveStorageStorageStatus(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveSubcategoryItemCount forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveSubcategoryItemCount(ctx context.Context, req *connect.Request[productv1.ResolveSubcategoryItemCountRequest]) (*connect.Response[productv1.ResolveSubcategoryItemCountResponse], error) {
	resp, err := s.inner.ResolveSubcategoryItemCount(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveSubcategoryParentCategory forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveSubcategoryParentCategory(ctx context.Context, req *connect.Request[productv1.ResolveSubcategoryParentCategoryRequest]) (*connect.Response[productv1.ResolveSubcategoryParentCategoryResponse], error) {
	resp, err := s.inner.ResolveSubcategoryParentCategory(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ResolveTestContainerDetails forwards the Connect call to the gRPC implementation.
func (s *MockServiceConnect) ResolveTestContainerDetails(ctx context.Context, req *connect.Request[productv1.ResolveTestContainerDetailsRequest]) (*connect.Response[productv1.ResolveTestContainerDetailsResponse], error) {
	resp, err := s.inner.ResolveTestContainerDetails(connectCtxToGRPC(ctx, req.Header()), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}
