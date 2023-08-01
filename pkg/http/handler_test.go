package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/starwars"
	"github.com/wundergraph/graphql-go-tools/pkg/subscription"
)

func TestGraphQLHTTPRequestHandler_ServeHTTP(t *testing.T) {
	starwars.SetRelativePathToStarWarsPackage("../starwars")

	handler := NewGraphqlHTTPHandlerFunc(starwars.NewExecutionHandler(t), abstractlogger.NoopLogger, &ws.DefaultHTTPUpgrader)
	server := httptest.NewServer(handler)
	defer server.Close()

	addr := server.Listener.Addr().String()
	httpAddr := fmt.Sprintf("http://%s", addr)
	wsAddr := fmt.Sprintf("ws://%s", addr)

	t.Run("http", func(t *testing.T) {
		t.Run("should return 400 Bad Request when query does not fit to schema", func(t *testing.T) {
			requestBodyBytes := starwars.InvalidQueryRequestBody(t)
			req, err := http.NewRequest(http.MethodPost, httpAddr, bytes.NewBuffer(requestBodyBytes))
			require.NoError(t, err)

			client := http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("should successfully handle query and return 200 OK", func(t *testing.T) {
			starWarsCases := []starwars.TestCase{
				{
					Name:        "simple hero query",
					RequestBody: starwars.LoadQuery(t, starwars.FileSimpleHeroQuery, nil),
				},
				{
					Name:        "droid query with argument and variable",
					RequestBody: starwars.LoadQuery(t, starwars.FileDroidWithArgAndVarQuery, starwars.QueryVariables{"droidID": "2000"}),
				},
				{
					Name:        "hero with aliases query",
					RequestBody: starwars.LoadQuery(t, starwars.FileHeroWithAliasesQuery, nil),
				},
				{
					Name:        "fragments query",
					RequestBody: starwars.LoadQuery(t, starwars.FileFragmentsQuery, starwars.QueryVariables{"droidID": "2000"}),
				},
				{
					Name:        "hero with operation name query",
					RequestBody: starwars.LoadQuery(t, starwars.FileHeroWithOperationNameQuery, nil),
				},
				{
					Name:        "directives include query",
					RequestBody: starwars.LoadQuery(t, starwars.FileDirectivesIncludeQuery, starwars.QueryVariables{"withFriends": true}),
				},
				{
					Name:        "directives skip query",
					RequestBody: starwars.LoadQuery(t, starwars.FileDirectivesSkipQuery, starwars.QueryVariables{"skipFriends": true}),
				},
				{
					Name:        "create review mutation",
					RequestBody: starwars.LoadQuery(t, starwars.FileCreateReviewMutation, starwars.QueryVariables{"ep": "JEDI", "review": starwars.ReviewInput()}),
				},
				{
					Name:        "inline fragments query",
					RequestBody: starwars.LoadQuery(t, starwars.FileInlineFragmentsQuery, nil),
				},
				{
					Name:        "union query",
					RequestBody: starwars.LoadQuery(t, starwars.FileUnionQuery, starwars.QueryVariables{"name": "Han Solo"}),
				},
			}

			for _, testCase := range starWarsCases {
				testCase := testCase

				t.Run(testCase.Name, func(t *testing.T) {
					requestBodyBytes := testCase.RequestBody
					req, err := http.NewRequest(http.MethodPost, httpAddr, bytes.NewBuffer(requestBodyBytes))
					require.NoError(t, err)

					client := http.Client{}
					resp, err := client.Do(req)
					require.NoError(t, err)

					responseBodyBytes, err := ioutil.ReadAll(resp.Body)
					require.NoError(t, err)

					assert.Equal(t, http.StatusOK, resp.StatusCode)
					assert.Contains(t, resp.Header.Get(httpHeaderContentType), httpContentTypeApplicationJson)
					assert.Equal(t, `{"data":null}`, string(responseBodyBytes))
				})
			}

		})
	})

	t.Run("websockets", func(t *testing.T) {
		var clientConn net.Conn
		defer func() {
			err := clientConn.Close()
			require.NoError(t, err)
		}()

		ctx, cancelFunc := context.WithCancel(context.Background())

		t.Run("should upgrade to websocket and establish connection successfully", func(t *testing.T) {
			var err error
			clientConn, _, _, err = ws.Dial(ctx, wsAddr)
			assert.NoError(t, err)

			initialClientMessage := subscription.Message{
				Id:      "",
				Type:    subscription.MessageTypeConnectionInit,
				Payload: nil,
			}
			sendMessageToServer(t, clientConn, initialClientMessage)

			serverMessage := readMessageFromServer(t, clientConn)
			assert.Equal(t, `{"id":"","type":"connection_ack","payload":null}`, string(serverMessage))
		})

		t.Run("should successfully start a subscription", func(t *testing.T) {
			startSubscriptionMessage := subscription.Message{
				Id:      "1",
				Type:    subscription.MessageTypeStart,
				Payload: starwars.LoadQuery(t, starwars.FileRemainingJedisSubscription, nil),
			}
			sendMessageToServer(t, clientConn, startSubscriptionMessage)

			serverMessage := readMessageFromServer(t, clientConn)
			assert.Equal(t, `{"id":"1","type":"data","payload":{"data":null}}`, string(serverMessage))
		})

		cancelFunc()
	})

}

func TestGraphQLHTTPRequestHandler_IsWebsocketUpgrade(t *testing.T) {
	handler := NewGraphqlHTTPHandlerFunc(nil, nil, nil).(*GraphQLHTTPRequestHandler)

	t.Run("should return false if upgrade header does not exist", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		isWebsocketUpgrade := handler.isWebsocketUpgrade(req)
		assert.False(t, isWebsocketUpgrade)
	})

	t.Run("should return false if upgrade header does not contain websocket", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		req.Header = map[string][]string{
			httpHeaderUpgrade: {"any"},
		}

		isWebsocketUpgrade := handler.isWebsocketUpgrade(req)
		assert.False(t, isWebsocketUpgrade)
	})

	t.Run("should return true if upgrade header contains websocket", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		req.Header = map[string][]string{
			httpHeaderUpgrade: {"any", "websocket"},
		}

		isWebsocketUpgrade := handler.isWebsocketUpgrade(req)
		assert.True(t, isWebsocketUpgrade)
	})
}

func TestGraphQLHTTPRequestHandler_ExtraVariables(t *testing.T) {
	handler := NewGraphqlHTTPHandlerFunc(nil, nil, nil).(*GraphQLHTTPRequestHandler)

	t.Run("should create extra variables successfully", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "http://localhost:8080/path", nil)
		require.NoError(t, err)

		req.Header = map[string][]string{
			httpHeaderUpgrade: {"websocket"},
		}

		req.AddCookie(&http.Cookie{})

		extraVariablesBytes := &bytes.Buffer{}
		err = handler.extraVariables(req, extraVariablesBytes)

		assert.NoError(t, err)

		expectedJson := fmt.Sprintf("%s\n", `{"request":{"cookies":{},"headers":{"Cookie":"=","Upgrade":"websocket"},"host":"localhost:8080","method":"GET","uri":""}}`)
		assert.Equal(t, expectedJson, extraVariablesBytes.String())
	})
}

func sendMessageToServer(t *testing.T, clientConn net.Conn, message subscription.Message) {
	messageBytes, err := json.Marshal(message)
	require.NoError(t, err)

	err = wsutil.WriteClientText(clientConn, messageBytes)
	require.NoError(t, err)
}

func readMessageFromServer(t *testing.T, clientConn net.Conn) []byte {
	msgBytes, _, err := wsutil.ReadServerData(clientConn)
	require.NoError(t, err)

	return msgBytes
}
