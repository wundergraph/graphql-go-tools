package grpcdatasource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type recordingHTTPClient struct {
	calls  atomic.Int32
	client *http.Client
}

func (c *recordingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return c.client.Do(req)
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
	// connect-go translates HTTP 502 to the "unavailable" Connect code and
	// surfaces the upstream status text in the message. We check for both
	// pieces so the assertion still tells us what went wrong if the format
	// shifts in a future connect-go release.
	require.Contains(t, err.Error(), "unavailable")
	require.Contains(t, err.Error(), "502")
}

func TestConnectTransport_Invoke_HeaderForwarding(t *testing.T) {
	var receivedHeaders http.Header
	// connect-go's client validates the response Content-Type, so the mock
	// must echo a valid Connect response (here: an empty proto body) rather
	// than a bare 200.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/proto")
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

func TestConnectTransport_Invoke_BinaryHeaderForwarding(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/proto")
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

	binaryValue := "\x00\x01\x02\x03"
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"x-trace-id-bin", binaryValue,
		"authorization", "Bearer token",
	)

	err := transport.Invoke(ctx, "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.NoError(t, err)

	// Binary header must be base64-encoded per the Connect protocol spec.
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte(binaryValue)), receivedHeaders.Get("X-Trace-Id-Bin"))
	// String header must be forwarded as-is.
	require.Equal(t, "Bearer token", receivedHeaders.Get("Authorization"))
}

// TestConnectTransport_BaseURL_TrailingSlash exercises strings.TrimRight on
// BaseURL so that procedure paths (which start with "/") do not collide
// with a trailing slash and produce a "//" segment in the request URL.
func TestConnectTransport_BaseURL_TrailingSlash(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{})
	}))
	defer server.Close()

	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:  server.URL + "/",
		Encoding: ConnectEncodingProtobuf,
	})

	compiler := newTestCompiler(t)
	reqDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryResponse")

	inputMsg := dynamicpb.NewMessage(reqDesc)
	outputMsg := dynamicpb.NewMessage(respDesc)

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.NoError(t, err)
	require.Equal(t, "/productv1.ProductService/QueryCategory", receivedPath)
}

// TestConnectTransport_Invoke_ContextCancellation verifies that callers
// can abort an in-flight call via the supplied context and that the
// resulting error chain still satisfies errors.Is(err, context.Canceled).
func TestConnectTransport_Invoke_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := transport.Invoke(ctx, "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "expected context.Canceled in error chain, got %v", err)
}

// TestConnectTransport_DefaultHTTPClient verifies that omitting HTTPClient
// from ConnectTransportConfig falls back to http.DefaultClient and still
// produces a working transport.
func TestConnectTransport_DefaultHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
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

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.NoError(t, err)
}

// TestConnectTransport_CustomHTTPClient verifies that a configured HTTPClient
// is used for outgoing Connect requests.
func TestConnectTransport_CustomHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{})
	}))
	defer server.Close()

	httpClient := &recordingHTTPClient{client: server.Client()}
	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:    server.URL,
		Encoding:   ConnectEncodingProtobuf,
		HTTPClient: httpClient,
	})

	compiler := newTestCompiler(t)
	reqDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryResponse")

	inputMsg := dynamicpb.NewMessage(reqDesc)
	outputMsg := dynamicpb.NewMessage(respDesc)

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.NoError(t, err)
	require.Equal(t, int32(1), httpClient.calls.Load())
}

// TestConnectTransport_Interceptors verifies that configured Connect
// interceptors are applied to outgoing unary calls.
func TestConnectTransport_Interceptors(t *testing.T) {
	var interceptorCalls atomic.Int32
	receivedHeader := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader <- r.Header.Get("X-Interceptor")
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{})
	}))
	defer server.Close()

	interceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			interceptorCalls.Add(1)
			req.Header().Set("X-Interceptor", "applied")
			return next(ctx, req)
		}
	})
	transport := NewConnectTransport(ConnectTransportConfig{
		BaseURL:      server.URL,
		Encoding:     ConnectEncodingProtobuf,
		Interceptors: []connect.Interceptor{interceptor},
	})

	compiler := newTestCompiler(t)
	reqDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryCategoryResponse")

	inputMsg := dynamicpb.NewMessage(reqDesc)
	outputMsg := dynamicpb.NewMessage(respDesc)

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryCategory", inputMsg, outputMsg)
	require.NoError(t, err)
	require.Equal(t, int32(1), interceptorCalls.Load())
	require.Equal(t, "applied", <-receivedHeader)
}
