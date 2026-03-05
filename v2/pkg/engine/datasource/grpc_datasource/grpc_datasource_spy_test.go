package grpcdatasource

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
)

// mockServiceSpy wraps MockService and counts calls to the methods
type mockServiceSpy struct {
	grpctest.MockService

	categoriesCalls      atomic.Int64
	categoryCalls        atomic.Int64
	normalizedScoreCalls atomic.Int64
	relatedCategoryCalls atomic.Int64
	productCountCalls    atomic.Int64
	popularityScoreCalls atomic.Int64
	categoryMetricsCalls atomic.Int64
	mascotCalls          atomic.Int64

	queryCategoryFunc func(ctx context.Context, req *productv1.QueryCategoryRequest) (*productv1.QueryCategoryResponse, error)
}

func (s *mockServiceSpy) QueryCategories(ctx context.Context, req *productv1.QueryCategoriesRequest) (*productv1.QueryCategoriesResponse, error) {
	s.categoriesCalls.Add(1)
	return s.MockService.QueryCategories(ctx, req)
}

func (s *mockServiceSpy) QueryCategory(ctx context.Context, req *productv1.QueryCategoryRequest) (*productv1.QueryCategoryResponse, error) {
	s.categoryCalls.Add(1)
	if s.queryCategoryFunc != nil {
		return s.queryCategoryFunc(ctx, req)
	}
	return s.MockService.QueryCategory(ctx, req)
}

func (s *mockServiceSpy) ResolveCategoryMetricsNormalizedScore(ctx context.Context, req *productv1.ResolveCategoryMetricsNormalizedScoreRequest) (*productv1.ResolveCategoryMetricsNormalizedScoreResponse, error) {
	s.normalizedScoreCalls.Add(1)
	return s.MockService.ResolveCategoryMetricsNormalizedScore(ctx, req)
}

func (s *mockServiceSpy) ResolveCategoryMetricsRelatedCategory(ctx context.Context, req *productv1.ResolveCategoryMetricsRelatedCategoryRequest) (*productv1.ResolveCategoryMetricsRelatedCategoryResponse, error) {
	s.relatedCategoryCalls.Add(1)
	return s.MockService.ResolveCategoryMetricsRelatedCategory(ctx, req)
}

func (s *mockServiceSpy) ResolveCategoryProductCount(ctx context.Context, req *productv1.ResolveCategoryProductCountRequest) (*productv1.ResolveCategoryProductCountResponse, error) {
	s.productCountCalls.Add(1)
	return s.MockService.ResolveCategoryProductCount(ctx, req)
}

func (s *mockServiceSpy) ResolveCategoryPopularityScore(ctx context.Context, req *productv1.ResolveCategoryPopularityScoreRequest) (*productv1.ResolveCategoryPopularityScoreResponse, error) {
	s.popularityScoreCalls.Add(1)
	return s.MockService.ResolveCategoryPopularityScore(ctx, req)
}

func (s *mockServiceSpy) ResolveCategoryCategoryMetrics(ctx context.Context, req *productv1.ResolveCategoryCategoryMetricsRequest) (*productv1.ResolveCategoryCategoryMetricsResponse, error) {
	s.categoryMetricsCalls.Add(1)
	return s.MockService.ResolveCategoryCategoryMetrics(ctx, req)
}

func (s *mockServiceSpy) ResolveCategoryMascot(ctx context.Context, req *productv1.ResolveCategoryMascotRequest) (*productv1.ResolveCategoryMascotResponse, error) {
	s.mascotCalls.Add(1)
	return s.MockService.ResolveCategoryMascot(ctx, req)
}

func setupSpyServer(t *testing.T) (*mockServiceSpy, *grpc.ClientConn, func()) {
	spy := &mockServiceSpy{}
	lis := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	productv1.RegisterProductServiceServer(server, spy)
	go func() {
		if err := server.Serve(lis); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithLocalDNSResolution(),
	)
	require.NoError(t, err)

	return spy, conn, func() {
		conn.Close()
		server.Stop()
		lis.Close()
	}
}

// Test_DataSource_Load_NullMetrics_NestedResolversNotInvoked verifies that when nullMetrics
// is always null, the nested field resolver RPCs (normalizedScore, relatedCategory, productCount)
// are never invoked by the engine.
func Test_DataSource_Load_NullMetrics_NestedResolversNotInvoked(t *testing.T) {
	spy, conn, cleanup := setupSpyServer(t)
	t.Cleanup(cleanup)

	query := "query CategoriesWithNullMetrics($baseline: Float!, $include: Boolean) { categories { id name nullMetrics { id normalizedScore(baseline: $baseline) relatedCategory(include: $include) { id name productCount } } } }"
	vars := `{"variables":{"baseline":100,"include":true}}`

	schemaDoc := grpctest.MustGraphQLSchema(t)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors(), "failed to parse query: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Mapping:      testMapping(),
		Compiler:     compiler,
	})
	require.NoError(t, err)

	input := fmt.Sprintf(`{"query":%q,"body":%s}`, query, vars)
	_, err = ds.Load(context.Background(), nil, []byte(input))
	require.NoError(t, err)

	require.Equal(t, int64(1), spy.categoriesCalls.Load(), "QueryCategories must be called")
	require.Zero(t, spy.normalizedScoreCalls.Load(), "ResolveCategoryMetricsNormalizedScore must not be called when parent nullMetrics is null")
	require.Zero(t, spy.relatedCategoryCalls.Load(), "ResolveCategoryMetricsRelatedCategory must not be called when parent nullMetrics is null")
	require.Zero(t, spy.productCountCalls.Load(), "ResolveCategoryProductCount must not be called when parent nullMetrics is null")
}

// Test_DataSource_Load_NullCategory_FieldResolversNotInvoked verifies that when the top-level
// category query returns null, no nested field resolver RPCs are invoked by the engine.
func Test_DataSource_Load_NullCategory_FieldResolversNotInvoked(t *testing.T) {
	spy, conn, cleanup := setupSpyServer(t)
	t.Cleanup(cleanup)

	spy.queryCategoryFunc = func(_ context.Context, _ *productv1.QueryCategoryRequest) (*productv1.QueryCategoryResponse, error) {
		return &productv1.QueryCategoryResponse{
			Category: nil,
		}, nil
	}

	query := `query CategoryQuery($id: ID!, $threshold: Int, $metricType: String!, $includeVolume: Boolean!) { category(id: $id) { id name popularityScore(threshold: $threshold) categoryMetrics(metricType: $metricType) { id } mascot(includeVolume: $includeVolume) { ... on Cat { id } } } }`
	vars := `{"variables":{"id":"cat-1","threshold":10,"metricType":"views","includeVolume":true}}`

	schemaDoc := grpctest.MustGraphQLSchema(t)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors(), "failed to parse query: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Mapping:      testMapping(),
		Compiler:     compiler,
	})
	require.NoError(t, err)

	input := fmt.Sprintf(`{"query":%q,"body":%s}`, query, vars)
	_, err = ds.Load(context.Background(), nil, []byte(input))
	require.NoError(t, err)

	require.Equal(t, int64(1), spy.categoryCalls.Load(), "QueryCategory must be called once")
	require.Zero(t, spy.popularityScoreCalls.Load(), "ResolveCategoryPopularityScore must not be called when category is null")
	require.Zero(t, spy.categoryMetricsCalls.Load(), "ResolveCategoryCategoryMetrics must not be called when category is null")
	require.Zero(t, spy.mascotCalls.Load(), "ResolveCategoryMascot must not be called when category is null")
}
