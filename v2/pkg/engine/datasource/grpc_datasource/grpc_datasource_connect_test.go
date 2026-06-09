package grpcdatasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1/productv1connect"
)

// setupTestConnectServer starts an httptest server backed by the
// MockServiceConnect adapter (the gRPC MockService wrapped onto the
// ConnectRPC handler interface). The server speaks Connect, gRPC, and
// gRPC-Web on the same H2C endpoint, but for these tests we drive it via
// the Connect transport.
//
// Returns a base URL that can be passed to NewConnectTransport.
func setupTestConnectServer(t testing.TB) (baseURL string, cleanup func()) {
	t.Helper()

	mock := &grpctest.MockService{}
	connectImpl := grpctest.NewMockServiceConnect(mock)

	mux := http.NewServeMux()
	mux.Handle(productv1connect.NewProductServiceHandler(connectImpl))

	srv := httptest.NewUnstartedServer(h2c.NewHandler(mux, &http2.Server{}))
	srv.EnableHTTP2 = true
	srv.Start()

	cleanup = srv.Close
	return srv.URL, cleanup
}

// connectE2E bundles the per-test inputs for the table-driven Connect e2e
// tests below. The zero value of Ctx falls back to context.Background(),
// of Encoding to ConnectEncodingProtobuf, and Headers/FederationConfigs
// to nil; callers only set the knobs that matter for the case at hand.
type connectE2E struct {
	BaseURL           string
	Query             string
	Vars              string
	Ctx               context.Context
	Headers           http.Header
	Encoding          ConnectEncoding
	FederationConfigs plan.FederationFieldConfigurations
}

// loadConnectQuery runs a GraphQL query through a DataSource that dials a
// ConnectRPC server backed by the MockService. The helper exists so the
// table-driven Connect tests below can focus on query/mapping/validation
// without re-stating the planner/transport scaffolding.
func loadConnectQuery(t *testing.T, opts connectE2E) []byte {
	t.Helper()

	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.Encoding == "" {
		opts.Encoding = ConnectEncodingProtobuf
	}

	schemaDoc := grpctest.MustGraphQLSchema(t)
	queryDoc, report := astparser.ParseGraphqlDocumentString(opts.Query)
	require.False(t, report.HasErrors(), "failed to parse query: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  opts.BaseURL,
		Encoding: opts.Encoding,
	})

	ds, err := NewDataSource(transport, DataSourceConfig{
		Operation:         &queryDoc,
		Definition:        &schemaDoc,
		SubgraphName:      "Products",
		Mapping:           testMapping(),
		Compiler:          compiler,
		FederationConfigs: opts.FederationConfigs,
	})
	require.NoError(t, err)

	input := fmt.Sprintf(`{"query":%q,"body":%s}`, opts.Query, opts.Vars)
	output, err := ds.Load(opts.Ctx, opts.Headers, []byte(input))
	require.NoError(t, err)
	return output
}

// Test_DataSource_Load_WithMockServiceConnect mirrors the gRPC end-to-end
// happy path (Test_DataSource_Load_WithMockService) but routes the call
// through the Connect transport instead of the gRPC client connection.
// It proves that the data source pipeline (compiler -> JSON builder ->
// transport -> response unmarshal) works for the Connect protocol against
// the same MockService implementation. Runs the same query under both
// the protobuf and JSON wire formats so the two encoders are exercised
// from the very first happy path.
func Test_DataSource_Load_WithMockServiceConnect(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	vars := `{"variables":{"filter":{"filter":{"name":"Test Product","filterField1":"filterField1","filterField2":"filterField2"}}}}`

	type response struct {
		Data struct {
			ComplexFilterType []struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"complexFilterType"`
		} `json:"data"`
	}

	for _, encoding := range []ConnectEncoding{ConnectEncodingProtobuf, ConnectEncodingJSON} {
		t.Run(string(encoding), func(t *testing.T) {
			output := loadConnectQuery(t, connectE2E{
				BaseURL:  baseURL,
				Encoding: encoding,
				Query:    query,
				Vars:     vars,
			})

			var resp response
			require.NoError(t, json.Unmarshal(output, &resp))
			require.NotEmpty(t, resp.Data.ComplexFilterType, "response should contain at least one item; empty slice would otherwise panic on index below")
			require.Equal(t, "test-id-123", resp.Data.ComplexFilterType[0].Id)
			require.Equal(t, "Test Product", resp.Data.ComplexFilterType[0].Name)
		})
	}
}
