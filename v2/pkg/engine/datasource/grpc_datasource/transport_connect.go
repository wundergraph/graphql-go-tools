package grpcdatasource

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ConnectEncoding selects the wire format for Connect protocol requests.
// It is declared as a string so that router/config layers can pass values
// through unchanged from YAML/JSON without intermediate translation. The
// zero value ("") is treated as ConnectEncodingProtobuf for safety.
type ConnectEncoding string

const (
	// ConnectEncodingProtobuf uses binary protobuf encoding.
	ConnectEncodingProtobuf ConnectEncoding = "proto"
	// ConnectEncodingJSON uses JSON encoding via protojson.
	ConnectEncodingJSON ConnectEncoding = "json"
)

// maxConnectResponseSize limits the response body read from a Connect service
// to 10 MB to prevent memory exhaustion from unexpectedly large or malicious
// responses. The limit is enforced by connect-go (connect.WithReadMaxBytes).
const maxConnectResponseSize = 10 * 1024 * 1024

// ConnectTransportConfig holds the configuration for creating a Connect transport.
type ConnectTransportConfig struct {
	// BaseURL is the base URL of the Connect service (e.g., "http://localhost:8080").
	BaseURL string
	// HTTPClient is the HTTP client to use. If nil, http.DefaultClient is used.
	HTTPClient connect.HTTPClient
	// Encoding specifies the serialization format (Protobuf or JSON).
	Encoding ConnectEncoding
	// Interceptors are applied, in order, to every Connect client this
	// transport constructs. Use them for cross-cutting concerns such as
	// tracing, logging, retries, or auth header injection. The slice is
	// captured at construction time; mutating it after NewConnectTransport
	// returns has no effect.
	Interceptors []connect.Interceptor
}

// connectTransport implements RPCTransport using the Connect protocol over HTTP.
//
// Internally it delegates to connect-go's typed Client, parameterised over
// dynamicpb.Message so the same transport instance can serve every procedure
// the data source resolves at runtime. connect-go would normally allocate
// response messages with `new(Resp)`, which produces an empty
// *dynamicpb.Message with no descriptor; the custom codecs below replace
// that empty message with `dynamicpb.NewMessage(respDesc)` before delegating
// to proto.Unmarshal/protojson.Unmarshal, which is the missing piece the
// default proto codec cannot supply.
type connectTransport struct {
	baseURL      string
	httpClient   connect.HTTPClient
	encoding     ConnectEncoding
	interceptors []connect.Interceptor

	// clients caches one connect.Client per procedure. The cache is
	// intentionally unbounded: the set of procedures is bounded by the
	// schema this transport is wired into, so it stabilises after warmup.
	mu      sync.RWMutex
	clients map[string]*connect.Client[dynamicpb.Message, dynamicpb.Message]
}

// NewConnectTransport creates an RPCTransport that uses the Connect protocol.
func NewConnectTransport(config ConnectTransportConfig) RPCTransport {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &connectTransport{
		baseURL:      strings.TrimRight(config.BaseURL, "/"),
		httpClient:   httpClient,
		encoding:     config.Encoding,
		interceptors: config.Interceptors,
		clients:      make(map[string]*connect.Client[dynamicpb.Message, dynamicpb.Message]),
	}
}

// dynamicProtoCodec is a connect.Codec that knows the expected response
// descriptor for a single procedure. The Unmarshal path is the only point
// where the descriptor is needed; Marshal works for any proto.Message.
type dynamicProtoCodec struct {
	responseDesc protoreflect.MessageDescriptor
}

func (c *dynamicProtoCodec) Name() string { return "proto" }

func (c *dynamicProtoCodec) Marshal(v any) ([]byte, error) {
	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("connect: marshal value is %T, want proto.Message", v)
	}
	return proto.Marshal(msg)
}

func (c *dynamicProtoCodec) Unmarshal(data []byte, v any) error {
	msg, ok := v.(*dynamicpb.Message)
	if !ok {
		return fmt.Errorf("connect: unmarshal value is %T, want *dynamicpb.Message", v)
	}
	*msg = *dynamicpb.NewMessage(c.responseDesc)
	return proto.Unmarshal(data, msg)
}

// dynamicJSONCodec is the JSON twin of dynamicProtoCodec.
type dynamicJSONCodec struct {
	responseDesc protoreflect.MessageDescriptor
}

func (c *dynamicJSONCodec) Name() string { return "json" }

func (c *dynamicJSONCodec) Marshal(v any) ([]byte, error) {
	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("connect: marshal value is %T, want proto.Message", v)
	}
	return protojson.Marshal(msg)
}

func (c *dynamicJSONCodec) Unmarshal(data []byte, v any) error {
	msg, ok := v.(*dynamicpb.Message)
	if !ok {
		return fmt.Errorf("connect: unmarshal value is %T, want *dynamicpb.Message", v)
	}
	*msg = *dynamicpb.NewMessage(c.responseDesc)
	return protojson.Unmarshal(data, msg)
}

// clientFor returns the cached connect-go client for a procedure, building
// one on first use. The client is keyed by procedure because the codec
// carries the response descriptor; encoding is shared across all procedures
// served by this transport.
func (t *connectTransport) clientFor(procedure string, respDesc protoreflect.MessageDescriptor) *connect.Client[dynamicpb.Message, dynamicpb.Message] {
	t.mu.RLock()
	if cli, ok := t.clients[procedure]; ok {
		t.mu.RUnlock()
		return cli
	}
	t.mu.RUnlock()

	t.mu.Lock()
	defer t.mu.Unlock()
	if cli, ok := t.clients[procedure]; ok {
		return cli
	}

	var codec connect.Codec
	switch t.encoding {
	case ConnectEncodingJSON:
		codec = &dynamicJSONCodec{responseDesc: respDesc}
	default:
		codec = &dynamicProtoCodec{responseDesc: respDesc}
	}

	opts := []connect.ClientOption{
		connect.WithCodec(codec),
		connect.WithReadMaxBytes(maxConnectResponseSize),
	}
	if len(t.interceptors) > 0 {
		opts = append(opts, connect.WithInterceptors(t.interceptors...))
	}
	cli := connect.NewClient[dynamicpb.Message, dynamicpb.Message](
		t.httpClient,
		t.baseURL+procedure,
		opts...,
	)
	t.clients[procedure] = cli
	return cli
}

// Invoke sends a Connect unary call to the configured base URL.
//
// input is expected to be a *dynamicpb.Message produced by the calling
// data source; output is a *dynamicpb.Message pre-built with the response
// descriptor that this transport will populate on success.
func (t *connectTransport) Invoke(ctx context.Context, methodFullName string, input, output protoreflect.Message) error {
	inDyn, ok := input.Interface().(*dynamicpb.Message)
	if !ok {
		return fmt.Errorf("connect: input is %T, want *dynamicpb.Message", input.Interface())
	}
	outDyn, ok := output.Interface().(*dynamicpb.Message)
	if !ok {
		return fmt.Errorf("connect: output is %T, want *dynamicpb.Message", output.Interface())
	}

	cli := t.clientFor(methodFullName, output.Descriptor())

	req := connect.NewRequest(inDyn)
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		for k, vs := range md {
			// Headers ending in "-bin" carry binary values. The Go HTTP
			// client rejects raw binary in headers, and the Connect protocol
			// (over HTTP) does not auto-encode them, so we base64 the value
			// before placing it on the wire to match gRPC metadata semantics.
			isBin := strings.HasSuffix(k, "-bin")
			for _, v := range vs {
				if isBin {
					req.Header().Add(k, base64.StdEncoding.EncodeToString([]byte(v)))
				} else {
					req.Header().Add(k, v)
				}
			}
		}
	}

	resp, err := cli.CallUnary(ctx, req)
	if err != nil {
		// Wrap with %w so callers can errors.As the original *connect.Error
		// to inspect Code, Message, Details, and Metadata. The default
		// formatting of *connect.Error is "<code>: <message>", so the error
		// string remains human-readable.
		return fmt.Errorf("connect: %w", err)
	}

	// resp.Msg was populated by the codec's Unmarshal with a fresh
	// dynamicpb.Message bound to the response descriptor. Reset the caller's
	// output before merging so a reused output message does not accumulate
	// repeated-field values across invocations.
	proto.Reset(outDyn)
	proto.Merge(outDyn, resp.Msg)
	return nil
}
