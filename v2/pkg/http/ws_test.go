package http

import (
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/subscription"
)

func TestWebsocketSubscriptionClient_WriteToClient(t *testing.T) {
	connToServer, connToClient := net.Pipe()

	websocketClient := NewWebsocketSubscriptionClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should write successfully to client", func(t *testing.T) {
		messageToClient := subscription.Message{
			Id:      "1",
			Type:    subscription.MessageTypeData,
			Payload: []byte(`{"data":null}`),
		}

		go func() {
			err := websocketClient.WriteToClient(messageToClient)
			assert.NoError(t, err)
		}()

		data, opCode, err := wsutil.ReadServerData(connToServer)
		require.NoError(t, err)
		require.Equal(t, ws.OpText, opCode)

		time.Sleep(10 * time.Millisecond)

		var messageFromServer subscription.Message
		err = json.Unmarshal(data, &messageFromServer)
		require.NoError(t, err)

		assert.Equal(t, messageToClient, messageFromServer)
	})

	t.Run("should not write to client when connection is closed", func(t *testing.T) {
		err := connToServer.Close()
		require.NoError(t, err)

		websocketClient.isClosedConnection = true

		err = websocketClient.WriteToClient(subscription.Message{})
		assert.NoError(t, err)
	})
}

func TestWebsocketSubscriptionClient_ReadFromClient(t *testing.T) {
	t.Run("should successfully read from client", func(t *testing.T) {
		connToServer, connToClient := net.Pipe()
		websocketClient := NewWebsocketSubscriptionClient(abstractlogger.NoopLogger, connToClient)

		messageToServer := &subscription.Message{
			Id:      "1",
			Type:    subscription.MessageTypeData,
			Payload: []byte(`{"data":null}`),
		}

		go func() {
			data, err := json.Marshal(messageToServer)
			require.NoError(t, err)

			err = wsutil.WriteClientText(connToServer, data)
			require.NoError(t, err)
		}()

		time.Sleep(10 * time.Millisecond)

		messageFromClient, err := websocketClient.ReadFromClient()
		assert.NoError(t, err)
		assert.Equal(t, messageToServer, messageFromClient)
	})
}

func TestWebsocketSubscriptionClient_IsConnected(t *testing.T) {
	_, connToClient := net.Pipe()
	websocketClient := NewWebsocketSubscriptionClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should return true when a connection is established", func(t *testing.T) {
		isConnected := websocketClient.IsConnected()
		assert.True(t, isConnected)
	})

	t.Run("should return false when a connection is closed", func(t *testing.T) {
		err := connToClient.Close()
		require.NoError(t, err)

		websocketClient.isClosedConnection = true

		isConnected := websocketClient.IsConnected()
		assert.False(t, isConnected)
	})
}

func TestWebsocketSubscriptionClient_Disconnect(t *testing.T) {
	_, connToClient := net.Pipe()
	websocketClient := NewWebsocketSubscriptionClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should disconnect and indicate a closed connection", func(t *testing.T) {
		err := websocketClient.Disconnect()
		assert.NoError(t, err)
		assert.Equal(t, true, websocketClient.isClosedConnection)
	})
}

func TestWebsocketSubscriptionClient_isClosedConnectionError(t *testing.T) {
	_, connToClient := net.Pipe()
	websocketClient := NewWebsocketSubscriptionClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should not close connection when it is not a closed connection error", func(t *testing.T) {
		isClosedConnectionError := websocketClient.isClosedConnectionError(errors.New("no closed connection err"))
		assert.False(t, isClosedConnectionError)
	})

	t.Run("should close connection when it is a closed connection error", func(t *testing.T) {
		isClosedConnectionError := websocketClient.isClosedConnectionError(wsutil.ClosedError{})
		assert.True(t, isClosedConnectionError)
	})
}
