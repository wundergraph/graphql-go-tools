package graphql_datasource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"nhooyr.io/websocket"
)

func TestWebsocketSubscriptionClient(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		msgType, data, err := conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomName: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"first"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"second"}}}}`))
		assert.NoError(t, err)
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"data","id":"1","payload":{"data":{"messageAdded":{"text":"third"}}}}`))
		assert.NoError(t, err)
		close(serverDone)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, ctx)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomName: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	first := <-next
	second := <-next
	third := <-next
	assert.Equal(t, `{"data":{"messageAdded":{"text":"first"}}}`, string(first))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"second"}}}`, string(second))
	assert.Equal(t, `{"data":{"messageAdded":{"text":"third"}}}`, string(third))
	<-serverDone
}

func TestWebsocketSubscriptionClientErrorArray(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		msgType, data, err := conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomNam: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"error","id":"1","payload":[{"message":"error"},{"message":"error"}]}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))
		_, _, err = conn.Read(r.Context())
		assert.NotNil(t, err)
		close(serverDone)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, ctx)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomNam: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	message := <-next
	assert.Equal(t, `{"errors":[{"message":"error"},{"message":"error"}]}`, string(message))
	_, ok := <-next
	assert.False(t, ok)
	assert.Eventuallyf(t, func() bool {
		<-serverDone
		return true
	}, time.Second, time.Millisecond*10, "server did not close")
}

func TestWebsocketSubscriptionClientErrorObject(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		assert.NoError(t, err)
		msgType, data, err := conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"connection_init"}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"connection_ack"}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"start","id":"1","payload":{"query":"subscription {messageAdded(roomNam: \"room\"){text}}"}}`, string(data))
		err = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"error","id":"1","payload":{"message":"error"}}`))
		assert.NoError(t, err)
		msgType, data, err = conn.Read(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, websocket.MessageText, msgType)
		assert.Equal(t, `{"type":"stop","id":"1"}`, string(data))
		_, _, err = conn.Read(r.Context())
		assert.NotNil(t, err)
		close(serverDone)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, ctx)
	next := make(chan []byte)
	err := client.Subscribe(ctx, GraphQLSubscriptionOptions{
		URL: strings.Replace(server.URL, "http", "ws", -1),
		Body: GraphQLBody{
			Query: `subscription {messageAdded(roomNam: "room"){text}}`,
		},
	}, next)
	assert.NoError(t, err)
	message := <-next
	assert.Equal(t, `{"errors":[{"message":"error"}]}`, string(message))
	_, ok := <-next
	assert.False(t, ok)
	<-serverDone
}
