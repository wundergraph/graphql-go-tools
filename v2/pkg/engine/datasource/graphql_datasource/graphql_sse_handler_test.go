package graphql_datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphQLSubscriptionClientSubscribe_SSE(t *testing.T) {
	t.Skip("FIXME")

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlQuery := r.URL.Query()
		assert.Equal(t, "subscription {messageAdded(roomName: \"room\"){text}}", urlQuery.Get("query"))

		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"messageAdded":{"text":"first"}}}`)
		flusher.Flush()

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"messageAdded":{"text":"second"}}}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next
	second := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_SSE_RequestAbort(t *testing.T) {
	t.Skip("FIXME")

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	ctx, clientCancel := context.WithCancel(context.Background())
	// cancel after start the request
	clientCancel()

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: "http://dummy",
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	assert.Eventuallyf(t, func() bool {
		<-next
		return true
	}, time.Millisecond*100, time.Millisecond*10, "subscription did not close")
}

func TestGraphQLSubscriptionClientSubscribe_SSE_POST(t *testing.T) {
	t.Skip("FIXME")

	postReqBody := GraphQLBody{
		Query: `subscription {messageAdded(roomName: "room"){text}}`,
	}
	expectedReqBody, err := json.Marshal(postReqBody)
	assert.NoError(t, err)

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		actualReqBody, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, expectedReqBody, actualReqBody)

		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"messageAdded":{"text":"first"}}}`)
		flusher.Flush()

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"messageAdded":{"text":"second"}}}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err = client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL:           server.URL,
		Body:          postReqBody,
		UseSSE:        true,
		SSEMethodPost: true,
	}, next)
	assert.NoError(t, err)

	first := <-next
	second := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_SSE_WithEvents(t *testing.T) {
	t.Skip("FIXME")

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "event: next\ndata: %s\n\n", `{"data":{"messageAdded":{"text":"first"}}}`)
		flusher.Flush()

		_, _ = fmt.Fprintf(w, "event: next\ndata: %s\n\n", `{"data":{"messageAdded":{"text":"second"}}}`)
		flusher.Flush()

		_, _ = fmt.Fprintf(w, "event: complete\n\n")
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next
	second := <-next

	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_SSE_Error(t *testing.T) {
	t.Skip("FIXME")

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"errors":[{"message":"Unexpected error.","locations":[{"line":2,"column":3}],"path":["countdown"]}]}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next

	assert.Equal(t, `{"errors":[{"message":"Unexpected error.","locations":[{"line":2,"column":3}],"path":["countdown"]}]}`, string(first))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_SSE_Error_Without_Header(t *testing.T) {
	t.Skip("FIXME")

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "%s\n\n", `{"errors":[{"message":"Unexpected error.","locations":[{"line":2,"column":3}],"path":["countdown"]}],"data":null}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next

	assert.Equal(t, `{"errors":[{"message":"Unexpected error.","locations":[{"line":2,"column":3}],"path":["countdown"]}]}`, string(first))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestGraphQLSubscriptionClientSubscribe_QueryParams(t *testing.T) {
	t.Skip("FIXME")

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlQuery := r.URL.Query()
		assert.Equal(t, "subscription($a: Int!){countdown(from: $a)}", urlQuery.Get("query"))
		assert.Equal(t, "CountDown", urlQuery.Get("operationName"))
		assert.Equal(t, `{"a":5}`, urlQuery.Get("variables"))
		assert.Equal(t, `{"persistedQuery":{"version":1,"sha256Hash":"d41d8cd98f00b204e9800998ecf8427e"}}`, urlQuery.Get("extensions"))

		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"countdown":5}}`)
		flusher.Flush()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query:         `subscription($a: Int!){countdown(from: $a)}`,
			OperationName: "CountDown",
			Variables:     []byte(`{"a":5}`),
			Extensions:    []byte(`{"persistedQuery":{"version":1,"sha256Hash":"d41d8cd98f00b204e9800998ecf8427e"}}`),
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next
	assert.Equal(t, `{"data":{"countdown":5}}`, string(first))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}

func TestBuildPOSTRequestSSE(t *testing.T) {
	t.Skip("FIXME")

	subscriptionOptions := GraphQLSubscriptionOptions{
		URL: "test",
		Body: GraphQLBody{
			Query:         `subscription($a: Int!){countdown(from: $a)}`,
			OperationName: "CountDown",
			Variables:     []byte(`{"a":5}`),
			Extensions:    []byte(`{"persistedQuery":{"version":1,"sha256Hash":"d41d8cd98f00b204e9800998ecf8427e"}}`),
		},
	}

	h := gqlSSEConnectionHandler{
		options: subscriptionOptions,
	}

	req, err := h.buildPOSTRequest(context.Background())
	assert.NoError(t, err)

	expectedReqBody, err := json.Marshal(subscriptionOptions.Body)
	assert.NoError(t, err)

	assert.Equal(t, http.MethodPost, req.Method)

	actualReqBody, err := io.ReadAll(req.Body)
	assert.NoError(t, err)
	assert.Equal(t, expectedReqBody, actualReqBody)
}

func TestBuildGETRequestSSE(t *testing.T) {
	t.Skip("FIXME")

	subscriptionOptions := GraphQLSubscriptionOptions{
		URL: "test",
		Body: GraphQLBody{
			Query:         `subscription($a: Int!){countdown(from: $a)}`,
			OperationName: "CountDown",
			Variables:     []byte(`{"a":5}`),
			Extensions:    []byte(`{"persistedQuery":{"version":1,"sha256Hash":"d41d8cd98f00b204e9800998ecf8427e"}}`),
		},
	}

	h := gqlSSEConnectionHandler{
		options: subscriptionOptions,
	}

	req, err := h.buildGETRequest(context.Background())
	assert.NoError(t, err)

	assert.Equal(t, http.MethodGet, req.Method)

	urlQuery := req.URL.Query()
	assert.Equal(t, subscriptionOptions.Body.Query, urlQuery.Get("query"))
	assert.Equal(t, subscriptionOptions.Body.OperationName, urlQuery.Get("operationName"))

	assert.Equal(t, string(subscriptionOptions.Body.Variables), urlQuery.Get("variables"))
	assert.Equal(t, string(subscriptionOptions.Body.Extensions), urlQuery.Get("extensions"))

}

func TestGraphQLSubscriptionClientSubscribe_SSE_Upstream_Dies(t *testing.T) {
	t.Skip("FIXME")

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlQuery := r.URL.Query()
		assert.Equal(t, "subscription {messageAdded(roomName: \"room\"){text}}", urlQuery.Get("query"))

		// Make sure that the writer supports flushing.
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"data":{"messageAdded":{"text":"first"}}}`)
		flusher.Flush()

		// Kill the upstream server. We should catch this event as an "unexpected EOF"
		// error and return an error message to the subscriber.
		h, ok := w.(http.Hijacker)
		require.True(t, ok)
		rawConn, _, err := h.Hijack()
		require.NoError(t, err)
		_ = rawConn.Close()

		close(serverDone)
	}))
	defer server.Close()

	serverCtx, serverCancel := context.WithCancel(context.Background())

	ctx, clientCancel := context.WithCancel(context.Background())

	client := NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, serverCtx,
		WithReadTimeout(time.Millisecond),
		WithLogger(logger()),
	)

	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: server.URL,
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
		UseSSE: true,
	}, next)
	assert.NoError(t, err)

	first := <-next
	second := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	// Upstream died
	assert.Equal(t, `{"errors":[{"message":"internal error"}]}`, string(second))

	clientCancel()
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
	serverCancel()
}
