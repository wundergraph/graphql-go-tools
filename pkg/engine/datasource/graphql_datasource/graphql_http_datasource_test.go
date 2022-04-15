package graphql_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jensneuse/graphql-go-tools/examples/chat"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnNullVariables(t *testing.T) {

	t.Run("variables with whitespace", func(t *testing.T) {
		s := &FetchSource{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"email":null,"firstName": "FirstTest",		"lastName":"LastTest","phone":123456,"preferences":{ "notifications":{}},"password":"password"}}}`))
		expected := `{"body":{"variables":{"firstName":"FirstTest","lastName":"LastTest","phone":123456,"password":"password"}}}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("empty variables", func(t *testing.T) {
		s := &FetchSource{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{}}}`))
		expected := `{"body":{"variables":{}}}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("two variables, one null", func(t *testing.T) {
		s := &FetchSource{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"a":null,"b":true}}}`))
		expected := `{"body":{"variables":{"b":true}}}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("two variables, one null reverse", func(t *testing.T) {
		s := &FetchSource{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"a":true,"b":null}}}`))
		expected := `{"body":{"variables":{"a":true}}}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("null variables", func(t *testing.T) {
		s := &FetchSource{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":null}}`))
		expected := `{"body":{"variables":null}}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("ignore null inside non variables", func(t *testing.T) {
		s := &FetchSource{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"foo":null},"body":"query {foo(bar: null){baz}}"}}`))
		expected := `{"body":{"variables":{},"body":"query {foo(bar: null){baz}}"}}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("variables missing", func(t *testing.T) {
		s := &FetchSource{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"query":"{foo}"}}`))
		expected := `{"body":{"query":"{foo}"}}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("variables null", func(t *testing.T) {
		s := &FetchSource{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"query":"{foo}","variables":null}}`))
		expected := `{"body":{"query":"{foo}","variables":null}}`
		assert.Equal(t, expected, string(out))
	})
}

var errSubscriptionClientFail = errors.New("subscription client fail error")

type FailingSubscriptionClient struct{}

func (f FailingSubscriptionClient) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	return errSubscriptionClientFail
}

func TestSubscriptionSource_Start(t *testing.T) {
	chatServer := httptest.NewServer(chat.GraphQLEndpointHandler())
	defer chatServer.Close()

	sendChatMessage := func(t *testing.T, username, message string) {
		time.Sleep(200 * time.Millisecond)
		httpClient := http.Client{}
		req, err := http.NewRequest(
			http.MethodPost,
			chatServer.URL,
			bytes.NewBufferString(fmt.Sprintf(`{"variables": {}, "operationName": "SendMessage", "query": "mutation SendMessage { post(roomName: \"#test\", username: \"%s\", text: \"%s\") { id } }"}`, username, message)),
		)
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	chatServerSubscriptionOptions := func(t *testing.T, body string) []byte {
		var gqlBody GraphQLBody
		_ = json.Unmarshal([]byte(body), &gqlBody)
		options := GraphQLSubscriptionOptions{
			URL:    chatServer.URL,
			Body:   gqlBody,
			Header: nil,
		}

		optionsBytes, err := json.Marshal(options)
		require.NoError(t, err)

		return optionsBytes
	}

	newSubscriptionSource := func(ctx context.Context) SubscriptionSource {
		httpClient := http.Client{}
		subscriptionSource := SubscriptionSource{client: NewWebSocketGraphQLSubscriptionClient(&httpClient, ctx)}
		return subscriptionSource
	}

	t.Run("should return error when input is invalid", func(t *testing.T) {
		source := SubscriptionSource{client: FailingSubscriptionClient{}}
		err := source.Start(context.Background(), []byte(`{"url": "", "body": "", "header": null}`), nil)
		assert.Error(t, err)
	})

	t.Run("should return error when subscription client returns an error", func(t *testing.T) {
		source := SubscriptionSource{client: FailingSubscriptionClient{}}
		err := source.Start(context.Background(), []byte(`{"url": "", "body": {}, "header": null}`), nil)
		assert.Error(t, err)
		assert.Equal(t, resolve.ErrUnableToResolve, err)
	})

	t.Run("invalid json: should stop before sending to upstream", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: "#test") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.ErrorIs(t, err, resolve.ErrUnableToResolve)
	})

	t.Run("invalid syntax (roomNam)", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomNam: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.NoError(t, err)

		msg, ok := <-next
		assert.True(t, ok)
		assert.Equal(t, `{"errors":[{"message":"Unknown argument \"roomNam\" on field \"messageAdded\" of type \"Subscription\". Did you mean \"roomName\"?","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"Field \"messageAdded\" argument \"roomName\" of type \"String!\" is required but not provided.","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`, string(msg))
		_, ok = <-next
		assert.False(t, ok)
	})

	t.Run("should close connection on stop message", func(t *testing.T) {
		next := make(chan []byte)
		subscriptionLifecycle, cancelSubscription := context.WithCancel(context.Background())
		resolverLifecycle, cancelResolver := context.WithCancel(context.Background())
		defer cancelResolver()

		source := newSubscriptionSource(resolverLifecycle)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(subscriptionLifecycle, chatSubscriptionOptions, next)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		nextBytes := <-next
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, string(nextBytes))
		cancelSubscription()
		_, ok := <-next
		assert.False(t, ok)
	})

	t.Run("should successfully subscribe with chat example", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		nextBytes := <-next
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, string(nextBytes))
	})
}
