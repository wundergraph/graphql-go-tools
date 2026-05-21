package grpcdatasource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
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

// Test_DataSource_Load_WithMockServiceConnect mirrors the gRPC end-to-end
// happy path (Test_DataSource_Load_WithMockService) but routes the call
// through the Connect transport instead of the gRPC client connection.
// It proves that the data source pipeline (compiler -> JSON builder ->
// transport -> response unmarshal) works for the Connect protocol against
// the same MockService implementation.
func Test_DataSource_Load_WithMockServiceConnect(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"Test Product","filterField1":"filterField1","filterField2":"filterField2"}}}}`

	schemaDoc := grpctest.MustGraphQLSchema(t)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	require.NoError(t, err)

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  baseURL,
		Encoding: ConnectEncodingProtobuf,
	})

	ds, err := NewDataSource(transport, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping: &GRPCMapping{
			Service: "Products",
			QueryRPCs: RPCConfigMap[RPCConfig]{
				"complexFilterType": {
					RPC:      "QueryComplexFilterType",
					Request:  "QueryComplexFilterTypeRequest",
					Response: "QueryComplexFilterTypeResponse",
				},
			},
			Fields: map[string]FieldMap{
				"Query": {
					"complexFilterType": {
						TargetName: "complex_filter_type",
						ArgumentMappings: map[string]string{
							"filter": "filter",
						},
					},
				},
				"FilterType": {
					"name":         {TargetName: "name"},
					"filterField1": {TargetName: "filter_field_1"},
					"filterField2": {TargetName: "filter_field_2"},
				},
			},
		},
	})
	require.NoError(t, err)

	output, err := ds.Load(context.Background(), nil, []byte(`{"query":"`+query+`","body":`+variables+`}`))
	require.NoError(t, err)

	type response struct {
		Data struct {
			ComplexFilterType []struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"complexFilterType"`
		} `json:"data"`
	}
	var resp response
	require.NoError(t, json.Unmarshal(output, &resp))
	require.Equal(t, "test-id-123", resp.Data.ComplexFilterType[0].Id)
	require.Equal(t, "Test Product", resp.Data.ComplexFilterType[0].Name)
}

// Test_DataSource_Load_WithMockServiceConnect_JSON re-runs the same
// happy-path query with JSON encoding instead of Protobuf. Both wire
// formats must yield identical decoded responses.
func Test_DataSource_Load_WithMockServiceConnect_JSON(t *testing.T) {
	baseURL, cleanup := setupTestConnectServer(t)
	t.Cleanup(cleanup)

	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"Test Product","filterField1":"a","filterField2":"b"}}}}`

	schemaDoc := grpctest.MustGraphQLSchema(t)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	require.NoError(t, err)

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  baseURL,
		Encoding: ConnectEncodingJSON,
	})

	ds, err := NewDataSource(transport, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping: &GRPCMapping{
			Service: "Products",
			QueryRPCs: RPCConfigMap[RPCConfig]{
				"complexFilterType": {
					RPC:      "QueryComplexFilterType",
					Request:  "QueryComplexFilterTypeRequest",
					Response: "QueryComplexFilterTypeResponse",
				},
			},
			Fields: map[string]FieldMap{
				"Query": {
					"complexFilterType": {
						TargetName: "complex_filter_type",
						ArgumentMappings: map[string]string{
							"filter": "filter",
						},
					},
				},
				"FilterType": {
					"name":         {TargetName: "name"},
					"filterField1": {TargetName: "filter_field_1"},
					"filterField2": {TargetName: "filter_field_2"},
				},
			},
		},
	})
	require.NoError(t, err)

	output, err := ds.Load(context.Background(), nil, []byte(`{"query":"`+query+`","body":`+variables+`}`))
	require.NoError(t, err)

	require.Contains(t, string(output), `"id":"test-id-123"`)
	require.Contains(t, string(output), `"name":"Test Product"`)
}
