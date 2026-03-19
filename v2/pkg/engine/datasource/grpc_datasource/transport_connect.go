package grpcdatasource

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
)

// ConnectEncoding represents the encoding format for Connect protocol requests.
type ConnectEncoding int

const (
	// ConnectEncodingProtobuf uses binary protobuf encoding.
	ConnectEncodingProtobuf ConnectEncoding = iota
	// ConnectEncodingJSON uses JSON encoding via protojson.
	ConnectEncodingJSON
)

// maxConnectResponseSize limits the response body read from a Connect service to 10 MB
// to prevent memory exhaustion from unexpectedly large or malicious responses.
const maxConnectResponseSize = 10 * 1024 * 1024

// ConnectTransportConfig holds the configuration for creating a Connect transport.
type ConnectTransportConfig struct {
	// BaseURL is the base URL of the Connect service (e.g., "http://localhost:8080").
	BaseURL string
	// HTTPClient is the HTTP client to use. If nil, http.DefaultClient is used.
	HTTPClient *http.Client
	// Encoding specifies the serialization format (Protobuf or JSON).
	Encoding ConnectEncoding
}

// connectTransport implements RPCTransport using the Connect protocol over HTTP.
type connectTransport struct {
	baseURL    string
	httpClient *http.Client
	encoding   ConnectEncoding
}

// NewConnectTransport creates an RPCTransport that uses the Connect protocol.
func NewConnectTransport(config ConnectTransportConfig) RPCTransport {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &connectTransport{
		baseURL:    strings.TrimRight(config.BaseURL, "/"),
		httpClient: httpClient,
		encoding:   config.Encoding,
	}
}

func (t *connectTransport) Invoke(ctx context.Context, methodFullName string, input, output protoref.Message) error {
	url := t.baseURL + methodFullName

	// protoref.Message (protoreflect.Message) is the reflection interface.
	// The underlying runtime type (*dynamicpb.Message) implements proto.Message,
	// which is required by proto.Marshal / protojson.Marshal.
	// We need to get the proto.Message interface via the ProtoReflect().Interface() path,
	// or directly type-assert since dynamicpb.Message implements proto.Message.
	inputMsg, ok := input.Interface().(proto.Message)
	if !ok {
		return fmt.Errorf("connect: input does not implement proto.Message")
	}

	var reqBody []byte
	var contentType string
	var err error

	switch t.encoding {
	case ConnectEncodingProtobuf:
		contentType = "application/proto"
		reqBody, err = proto.Marshal(inputMsg)
	case ConnectEncodingJSON:
		contentType = "application/json"
		reqBody, err = protojson.Marshal(inputMsg)
	default:
		return fmt.Errorf("connect: unsupported encoding: %d", t.encoding)
	}
	if err != nil {
		return fmt.Errorf("connect: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("connect: create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Connect-Protocol-Version", "1")

	// Forward gRPC metadata as HTTP headers.
	// Keys ending in "-bin" carry binary values; the Connect protocol requires
	// these to be base64-encoded before placing them in an HTTP header.
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		for k, vals := range md {
			isBin := strings.HasSuffix(k, "-bin")
			for _, v := range vals {
				if isBin {
					req.Header.Add(k, base64.StdEncoding.EncodeToString([]byte(v)))
				} else {
					req.Header.Add(k, v)
				}
			}
		}
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxConnectResponseSize))
	if err != nil {
		return fmt.Errorf("connect: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return parseConnectError(resp.StatusCode, respBody)
	}

	// Unmarshal response into the output message.
	outputMsg, ok := output.Interface().(proto.Message)
	if !ok {
		return fmt.Errorf("connect: output does not implement proto.Message")
	}

	switch t.encoding {
	case ConnectEncodingProtobuf:
		err = proto.Unmarshal(respBody, outputMsg)
	case ConnectEncodingJSON:
		err = protojson.Unmarshal(respBody, outputMsg)
	}
	if err != nil {
		return fmt.Errorf("connect: unmarshal response: %w", err)
	}

	return nil
}

// connectError represents an error response from a Connect protocol service.
type connectError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// parseConnectError parses an error response body from a Connect service.
// Connect errors are always JSON-encoded regardless of the request encoding.
// If the body is not valid JSON (e.g., a 502 from a reverse proxy), falls back to raw status code.
func parseConnectError(statusCode int, body []byte) error {
	var ce connectError
	if err := json.Unmarshal(body, &ce); err != nil {
		return fmt.Errorf("connect: HTTP %d: %s", statusCode, string(body))
	}
	return fmt.Errorf("connect: %s: %s", ce.Code, ce.Message)
}
