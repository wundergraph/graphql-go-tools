package grpcdatasource

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

func newTestCompiler(t *testing.T) *RPCCompiler {
	t.Helper()
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)
	return compiler
}

func findMessageDesc(t *testing.T, compiler *RPCCompiler, fullName string) protoref.MessageDescriptor {
	t.Helper()
	for _, m := range compiler.doc.Messages {
		if string(m.Desc.FullName()) == fullName {
			return m.Desc
		}
	}
	t.Fatalf("message %q not found in proto document", fullName)
	return nil
}

func TestConnectTransport_Invoke_Protobuf(t *testing.T) {
	compiler := newTestCompiler(t)

	reqDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryResponse")
	categoryDesc := findMessageDesc(t, compiler, "productv1.Category")

	// Build a response message.
	respMsg := dynamicpb.NewMessage(respDesc)
	categoryMsg := dynamicpb.NewMessage(categoryDesc)
	categoryMsg.Set(categoryDesc.Fields().ByName("id"), protoref.ValueOfString("cat-123"))
	categoryMsg.Set(categoryDesc.Fields().ByName("name"), protoref.ValueOfString("Electronics"))
	respMsg.Set(respDesc.Fields().ByName("category"), protoref.ValueOfMessage(categoryMsg))

	respBytes, err := proto.Marshal(respMsg)
	require.NoError(t, err)

	var receivedContentType string
	var receivedProtocolVersion string
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedProtocolVersion = r.Header.Get("Connect-Protocol-Version")
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	}))
	defer server.Close()

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  server.URL,
		Encoding: ConnectEncodingProtobuf,
	})

	inputMsg := dynamicpb.NewMessage(reqDesc)
	inputMsg.Set(reqDesc.Fields().ByName("id"), protoref.ValueOfString("cat-123"))

	outputMsg := dynamicpb.NewMessage(respDesc)

	err = transport.Invoke(context.Background(), "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.NoError(t, err)

	require.Equal(t, "application/proto", receivedContentType)
	require.Equal(t, "1", receivedProtocolVersion)
	require.NotEmpty(t, receivedBody)

	outputJSON, err := protojson.Marshal(outputMsg)
	require.NoError(t, err)
	require.Contains(t, string(outputJSON), "cat-123")
	require.Contains(t, string(outputJSON), "Electronics")
}

func TestConnectTransport_Invoke_JSON(t *testing.T) {
	compiler := newTestCompiler(t)

	reqDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryResponse")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, _ := io.ReadAll(r.Body)
		require.True(t, json.Valid(body), "request body should be valid JSON: %s", string(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"category":{"id":"cat-456","name":"Books"}}`))
	}))
	defer server.Close()

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  server.URL,
		Encoding: ConnectEncodingJSON,
	})

	inputMsg := dynamicpb.NewMessage(reqDesc)
	inputMsg.Set(reqDesc.Fields().ByName("id"), protoref.ValueOfString("cat-456"))

	outputMsg := dynamicpb.NewMessage(respDesc)

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.NoError(t, err)

	outputJSON, err := protojson.Marshal(outputMsg)
	require.NoError(t, err)
	require.Contains(t, string(outputJSON), "cat-456")
	require.Contains(t, string(outputJSON), "Books")
}

func TestConnectTransport_Invoke_ConnectError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"not_found","message":"category not found"}`))
	}))
	defer server.Close()

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  server.URL,
		Encoding: ConnectEncodingProtobuf,
	})

	compiler := newTestCompiler(t)
	reqDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryResponse")

	inputMsg := dynamicpb.NewMessage(reqDesc)
	outputMsg := dynamicpb.NewMessage(respDesc)

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not_found")
	require.Contains(t, err.Error(), "category not found")
}

func TestConnectTransport_Invoke_NonJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>Bad Gateway</html>"))
	}))
	defer server.Close()

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  server.URL,
		Encoding: ConnectEncodingProtobuf,
	})

	compiler := newTestCompiler(t)
	reqDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryResponse")

	inputMsg := dynamicpb.NewMessage(reqDesc)
	outputMsg := dynamicpb.NewMessage(respDesc)

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 502")
}

func TestConnectTransport_Invoke_HeaderForwarding(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{})
	}))
	defer server.Close()

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  server.URL,
		Encoding: ConnectEncodingProtobuf,
	})

	compiler := newTestCompiler(t)
	reqDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryResponse")

	inputMsg := dynamicpb.NewMessage(reqDesc)
	outputMsg := dynamicpb.NewMessage(respDesc)

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"authorization", "Bearer test-token",
		"x-custom-header", "custom-value",
	)

	err := transport.Invoke(ctx, "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.NoError(t, err)

	require.Equal(t, "Bearer test-token", receivedHeaders.Get("Authorization"))
	require.Equal(t, "custom-value", receivedHeaders.Get("X-Custom-Header"))
}

func TestGRPCTransport_Invoke(t *testing.T) {
	// Use the existing mockInterface from grpc_datasource_test.go.
	mi := mockInterface{}
	transport := NewGRPCTransport(mi)

	compiler := newTestCompiler(t)
	reqDesc := findMessageDesc(t, compiler, "productv1.QueryComplexFilterTypeRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryComplexFilterTypeResponse")

	inputMsg := dynamicpb.NewMessage(reqDesc)
	outputMsg := dynamicpb.NewMessage(respDesc)

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryComplexFilterType", inputMsg, outputMsg)
	require.NoError(t, err)
}
