package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/jensneuse/abstractlogger"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

// SSETransport implements the Transport interface using Server-Sent Events.
// Unlike WebSocket, each subscription creates a separate HTTP request.
// TCP connection reuse is handled by http.Client's connection pool.
//
// Supports both POST (graphql-sse spec) and GET (traditional SSE) methods.
type SSETransport struct {
	ctx    context.Context
	client *http.Client
	log    abstractlogger.Logger

	mu    sync.Mutex
	conns map[*SSEConnection]struct{}
}

// NewSSETransport creates a new SSETransport with the provided http.Client.
// The transport will automatically close all connections when ctx is cancelled.
func NewSSETransport(ctx context.Context, client *http.Client, log abstractlogger.Logger) *SSETransport {
	if log == nil {
		log = abstractlogger.NoopLogger
	}

	t := &SSETransport{
		ctx:    ctx,
		client: client,
		log:    log,
		conns:  make(map[*SSEConnection]struct{}),
	}

	context.AfterFunc(ctx, t.closeAll)

	return t
}

// Subscribe initiates a GraphQL subscription over SSE.
// Each call creates a new HTTP request (no multiplexing).
//
// The HTTP method is determined by opts.SSEMethod:
//   - SSEMethodAuto or SSEMethodPOST: POST with JSON body (graphql-sse spec)
//   - SSEMethodGET: GET with query parameters (traditional SSE)
func (t *SSETransport) Subscribe(ctx context.Context, req *common.Request, opts common.Options) (<-chan *common.Message, func(), error) {
	var httpReq *http.Request
	var err error

	method := opts.SSEMethod
	if method == common.SSEMethodAuto {
		method = common.SSEMethodPOST // Default to POST (graphql-sse spec)
	}

	t.log.Debug("sseTransport.Subscribe",
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("method", string(method)),
	)

	switch method {
	case common.SSEMethodPOST:
		httpReq, err = buildPOSTRequest(t.ctx, req, opts)
	case common.SSEMethodGET:
		httpReq, err = buildGETRequest(t.ctx, req, opts)
	default:
		return nil, nil, fmt.Errorf("unsupported SSE method: %s", method)
	}

	if err != nil {
		return nil, nil, err
	}

	// Execute request
	resp, err := t.client.Do(httpReq)
	if err != nil {
		t.log.Error("sseTransport.Subscribe",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.Error(err),
		)
		return nil, nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.log.Error("sseTransport.Subscribe",
			abstractlogger.String("endpoint", opts.Endpoint),
			abstractlogger.Int("status", resp.StatusCode),
		)
		if len(body) > 0 {
			return nil, nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}
		return nil, nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Verify content type (should be text/event-stream)
	if err := t.validateContentType(resp); err != nil {
		resp.Body.Close()
		return nil, nil, err
	}

	t.log.Debug("sseTransport.Subscribe",
		abstractlogger.String("endpoint", opts.Endpoint),
		abstractlogger.String("status", "connected"),
	)

	// Create connection
	conn := newSSEConnection(resp)

	t.mu.Lock()
	t.conns[conn] = struct{}{}
	t.mu.Unlock()

	go conn.ReadLoop()

	cancel := func() {
		conn.Close()
		t.removeConn(conn)
	}

	return conn.ch, cancel, nil
}

// buildPOSTRequest creates a POST request with JSON body (graphql-sse spec).
func buildPOSTRequest(ctx context.Context, req *common.Request, opts common.Options) (*http.Request, error) {
	body, err := json.Marshal(map[string]any{
		"query":         req.Query,
		"variables":     req.Variables,
		"operationName": req.OperationName,
		"extensions":    req.Extensions,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")

	// Add custom headers
	maps.Copy(httpReq.Header, opts.Headers)

	return httpReq, nil
}

// buildGETRequest creates a GET request with query parameters (traditional SSE).
func buildGETRequest(ctx context.Context, req *common.Request, opts common.Options) (*http.Request, error) {
	// Parse the endpoint URL
	u, err := url.Parse(opts.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	// Build query parameters
	q := u.Query()
	q.Set("query", req.Query)

	if len(req.Variables) > 0 {
		varsJSON, err := json.Marshal(req.Variables)
		if err != nil {
			return nil, fmt.Errorf("marshal variables: %w", err)
		}
		q.Set("variables", string(varsJSON))
	}

	if req.OperationName != "" {
		q.Set("operationName", req.OperationName)
	}

	if len(req.Extensions) > 0 {
		extJSON, err := json.Marshal(req.Extensions)
		if err != nil {
			return nil, fmt.Errorf("marshal extensions: %w", err)
		}
		q.Set("extensions", string(extJSON))
	}

	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")

	// Add custom headers
	maps.Copy(httpReq.Header, opts.Headers)

	return httpReq, nil
}

// validateContentType checks that the response has the correct content type.
func (t *SSETransport) validateContentType(resp *http.Response) error {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		return nil // Allow missing content-type
	}

	// Check if it starts with text/event-stream (may include charset)
	if strings.HasPrefix(contentType, "text/event-stream") {
		return nil
	}

	return fmt.Errorf("unexpected content-type: %s", contentType)
}

func (t *SSETransport) removeConn(conn *SSEConnection) {
	t.mu.Lock()
	delete(t.conns, conn)
	t.mu.Unlock()
}

// closeAll terminates all active SSE connections. Called automatically when context is cancelled.
func (t *SSETransport) closeAll() {
	t.mu.Lock()
	conns := make([]*SSEConnection, 0, len(t.conns))
	for conn := range t.conns {
		conns = append(conns, conn)
	}
	t.conns = make(map[*SSEConnection]struct{})
	t.mu.Unlock()

	t.log.Debug("sseTransport.closeAll",
		abstractlogger.Int("connections", len(conns)),
	)

	for _, conn := range conns {
		conn.Close()
	}
}

// ConnCount returns the number of active SSE connections.
func (t *SSETransport) ConnCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.conns)
}
