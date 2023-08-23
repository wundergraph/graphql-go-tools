package websocket

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
	"github.com/TykTechnologies/graphql-go-tools/pkg/subscription"
	"github.com/TykTechnologies/graphql-go-tools/pkg/testing/subscriptiontesting"
)

func TestHandleWithOptions(t *testing.T) {
	chatServer := httptest.NewServer(subscriptiontesting.ChatGraphQLEndpointHandler())
	defer chatServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executorPoolV2 := setupExecutorPoolV2(t, ctx, chatServer.URL)
	serverConn, _ := net.Pipe()
	testClient := NewTestClient(false)

	done := make(chan bool)
	errChan := make(chan error)
	go Handle(
		done,
		errChan,
		serverConn,
		executorPoolV2,
		WithCustomClient(testClient),
		WithCustomSubscriptionUpdateInterval(50*time.Millisecond),
		WithCustomKeepAliveInterval(3600*time.Second), // keep_alive should not intervene with our tests, so make it high
	)

	require.Eventually(t, func() bool {
		<-done
		return true
	}, 1*time.Second, 2*time.Millisecond)

	testClient.writeMessageFromClient([]byte(`{"type":"connection_init"}`))
	assert.Eventually(t, func() bool {
		expectedMessage := []byte(`{"type":"connection_ack"}`)
		actualMessage := testClient.readMessageToClient()
		assert.Equal(t, expectedMessage, actualMessage)
		return true
	}, 1*time.Second, 2*time.Millisecond, "never satisfied on connection_init")

	testClient.writeMessageFromClient([]byte(`{"id":"1","type":"start","payload":{"query":"{ room(name:\"#my_room\") { name } }"}}`))
	assert.Eventually(t, func() bool {
		expectedMessage := []byte(`{"id":"1","type":"data","payload":{"data":{"room":{"name":"#my_room"}}}}`)
		actualMessage := testClient.readMessageToClient()
		assert.Equal(t, expectedMessage, actualMessage)
		expectedMessage = []byte(`{"id":"1","type":"complete"}`)
		actualMessage = testClient.readMessageToClient()
		assert.Equal(t, expectedMessage, actualMessage)
		return true
	}, 2*time.Second, 2*time.Millisecond, "never satisfied on start non-subscription")

	testClient.writeMessageFromClient([]byte(`{"id":"2","type":"start","payload":{"query":"subscription { messageAdded(roomName:\"#my_room\") { text } }"}}`))
	<-time.After(15 * time.Millisecond)
	testClient.writeMessageFromClient([]byte(`{"id":"3","type":"start","payload":{"query":"mutation { post(text: \"hello\", username: \"me\", roomName: \"#my_room\") { text } }"}}`))
	assert.Eventually(t, func() bool {
		expectedMessages := []string{
			`{"id":"3","type":"data","payload":{"data":{"post":{"text":"hello"}}}}`,
			`{"id":"3","type":"complete"}`,
			`{"id":"2","type":"data","payload":{"data":{"messageAdded":{"text":"hello"}}}}`,
		}
		actualMessage := testClient.readMessageToClient()
		assert.Contains(t, expectedMessages, string(actualMessage))
		actualMessage = testClient.readMessageToClient()
		assert.Contains(t, expectedMessages, string(actualMessage))
		actualMessage = testClient.readMessageToClient()
		assert.Contains(t, expectedMessages, string(actualMessage))
		return true
	}, 2*time.Second, 2*time.Millisecond, "never satisfied on start subscription")

	testClient.writeMessageFromClient([]byte(`{"id":"2","type":"stop"}`))
	assert.Eventually(t, func() bool {
		expectedMessage := []byte(`{"id":"2","type":"complete"}`)
		actualMessage := testClient.readMessageToClient()
		assert.Equal(t, expectedMessage, actualMessage)
		return true
	}, 2*time.Second, 2*time.Millisecond, "never satisfied on stop subscription")

}

func setupExecutorPoolV2(t *testing.T, ctx context.Context, chatServerURL string) *subscription.ExecutorV2Pool {
	chatSchemaBytes, err := subscriptiontesting.LoadSchemaFromExamplesDirectoryWithinPkg()
	require.NoError(t, err)

	chatSchema, err := graphql.NewSchemaFromReader(bytes.NewBuffer(chatSchemaBytes))
	require.NoError(t, err)

	engineConf := graphql.NewEngineV2Configuration(chatSchema)
	engineConf.SetDataSources([]plan.DataSourceConfiguration{
		{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"room"}},
				{TypeName: "Mutation", FieldNames: []string{"post"}},
				{TypeName: "Subscription", FieldNames: []string{"messageAdded"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Chatroom", FieldNames: []string{"name", "messages"}},
				{TypeName: "Message", FieldNames: []string{"text", "createdBy"}},
			},
			Factory: &graphql_datasource.Factory{
				HTTPClient: httpclient.DefaultNetHttpClient,
			},
			Custom: graphql_datasource.ConfigJson(graphql_datasource.Configuration{
				Fetch: graphql_datasource.FetchConfiguration{
					URL:    chatServerURL,
					Method: http.MethodPost,
					Header: nil,
				},
				Subscription: graphql_datasource.SubscriptionConfiguration{
					URL: chatServerURL,
				},
			}),
		},
	})
	engineConf.SetFieldConfigurations([]plan.FieldConfiguration{
		{
			TypeName:  "Query",
			FieldName: "room",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "name",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Mutation",
			FieldName: "post",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "roomName",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "username",
					SourceType: plan.FieldArgumentSource,
				},
				{
					Name:       "text",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
		{
			TypeName:  "Subscription",
			FieldName: "messageAdded",
			Arguments: []plan.ArgumentConfiguration{
				{
					Name:       "roomName",
					SourceType: plan.FieldArgumentSource,
				},
			},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost:8080", nil)
	require.NoError(t, err)

	req.Header.Set("X-Other-Key", "x-other-value")

	initCtx := subscription.NewInitialHttpRequestContext(req)

	engine, err := graphql.NewExecutionEngineV2(initCtx, abstractlogger.NoopLogger, engineConf)
	require.NoError(t, err)

	executorPool := subscription.NewExecutorV2Pool(engine, ctx)
	return executorPool
}
