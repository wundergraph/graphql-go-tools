package grpcdatasource

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// mockServiceSpy wraps MockService and counts calls to the methods
type mockServiceSpy struct {
	grpctest.MockService
	categoriesCalls      atomic.Int64
	normalizedScoreCalls atomic.Int64
	relatedCategoryCalls atomic.Int64
	productCountCalls    atomic.Int64
}

func (s *mockServiceSpy) QueryCategories(ctx context.Context, req *productv1.QueryCategoriesRequest) (*productv1.QueryCategoriesResponse, error) {
	s.categoriesCalls.Add(1)
	return s.MockService.QueryCategories(ctx, req)
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
