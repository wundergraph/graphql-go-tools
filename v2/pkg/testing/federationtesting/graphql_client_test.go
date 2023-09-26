package federationtesting

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/subscription"
)

type queryVariables map[string]interface{}

func requestBody(t *testing.T, query string, variables queryVariables) []byte {
	var variableJsonBytes []byte
	if len(variables) > 0 {
		var err error
		variableJsonBytes, err = json.Marshal(variables)
		require.NoError(t, err)
	}

	body := graphql.Request{
		OperationName: "",
		Variables:     variableJsonBytes,
		Query:         query,
	}

	jsonBytes, err := json.Marshal(body)
	require.NoError(t, err)

	return jsonBytes
}

func loadQuery(t *testing.T, filePath string, variables queryVariables) []byte {
	query, err := os.ReadFile(filePath)
	require.NoError(t, err)

	return requestBody(t, string(query), variables)
}

func NewGraphqlClient(httpClient *http.Client) *GraphqlClient {
	return &GraphqlClient{
		httpClient: httpClient,
	}
}

type GraphqlClient struct {
	httpClient *http.Client
}

func (g *GraphqlClient) Query(ctx context.Context, addr, queryFilePath string, variables queryVariables, t *testing.T) []byte {
	reqBody := loadQuery(t, queryFilePath, variables)
	req, err := http.NewRequest(http.MethodPost, addr, bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	req = req.WithContext(ctx)
	resp, err := g.httpClient.Do(req)
	require.NoError(t, err)
	responseBodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	return responseBodyBytes
}

func (g *GraphqlClient) Subscription(ctx context.Context, addr, queryFilePath string, variables queryVariables, t *testing.T) chan []byte {
	messageCh := make(chan []byte)

	conn, _, _, err := ws.Dial(ctx, addr)
	require.NoError(t, err)
	// 1. send connection init
	initialClientMessage := subscription.Message{
		Id:      "",
		Type:    subscription.MessageTypeConnectionInit,
		Payload: nil,
	}

	err = g.sendMessageToServer(conn, initialClientMessage)
	require.NoError(t, err)
	// 2. receive connection ack
	serverMessage := g.readMessageFromServer(t, conn)
	assert.Equal(t, `{"id":"","type":"connection_ack","payload":null}`, string(serverMessage))
	// 3. send `start` message with subscription operation
	startSubscriptionMessage := subscription.Message{
		Id:      "1",
		Type:    subscription.MessageTypeStart,
		Payload: loadQuery(t, queryFilePath, variables),
	}

	err = g.sendMessageToServer(conn, startSubscriptionMessage)
	require.NoError(t, err)

	// 4. start receiving messages from subscription

	go func() {
		defer conn.Close()
		defer close(messageCh)

		for {
			msgBytes, _, err := wsutil.ReadServerData(conn)
			require.NoError(t, err)

			messageCh <- msgBytes
		}
	}()

	return messageCh
}

func (g *GraphqlClient) sendMessageToServer(clientConn net.Conn, message subscription.Message) error {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	if err = wsutil.WriteClientText(clientConn, messageBytes); err != nil {
		return err
	}

	return nil
}

func (g *GraphqlClient) readMessageFromServer(t *testing.T, clientConn net.Conn) []byte {
	msgBytes, _, err := wsutil.ReadServerData(clientConn)
	require.NoError(t, err)

	return msgBytes
}
