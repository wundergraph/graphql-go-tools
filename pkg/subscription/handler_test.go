package subscription

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Handle(t *testing.T) {
	client := newMockClient()

	subscriptionHandler, err := NewHandler(abstractlogger.NoopLogger, client)
	require.NoError(t, err)

	handlerRoutine := func(ctx context.Context) func() bool {
		return func() bool {
			subscriptionHandler.Handle(ctx)
			return true
		}
	}

	t.Run("connection_init", func(t *testing.T) {
		t.Run("should send connection error message when error on read occurrs", func(t *testing.T) {
			client.prepareConnectionInitMessage().withError()

			ctx, cancelFunc := context.WithCancel(context.Background())

			cancelFunc()
			require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

			expectedMessage := Message{
				Type:    MessageTypeConnectionError,
				Payload: jsonizePayload(t, "could not read message from client"),
			}

			messageFromServer := client.readFromServer()
			assert.Equal(t, expectedMessage, messageFromServer)
		})

		t.Run("should successfully init connection and respond with ack", func(t *testing.T) {
			client.prepareConnectionInitMessage().withoutError()

			ctx, cancelFunc := context.WithCancel(context.Background())

			cancelFunc()
			require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

			expectedMessage := Message{
				Type: MessageTypeConnectionAck,
			}

			messageFromServer := client.readFromServer()
			assert.Equal(t, expectedMessage, messageFromServer)
		})
	})

	t.Run("connection_keep_alive", func(t *testing.T) {
		t.Run("should successfully send keep alive messages after connection_init", func(t *testing.T) {
			keepAliveInterval, err := time.ParseDuration("5ms")
			require.NoError(t, err)

			subscriptionHandler.ChangeKeepAliveInterval(keepAliveInterval)

			client.prepareConnectionInitMessage().withoutError()
			ctx, cancelFunc := context.WithCancel(context.Background())

			handlerRoutineFunc := handlerRoutine(ctx)
			go handlerRoutineFunc()

			time.Sleep(2 * keepAliveInterval)
			cancelFunc()

			expectedMessage := Message{
				Type: MessageTypeConnectionKeepAlive,
			}

			messageFromServer := client.readFromServer()
			assert.Equal(t, expectedMessage, messageFromServer)
		})
	})

	t.Run("connection_terminate", func(t *testing.T) {
		t.Run("should successfully disconnect from client", func(t *testing.T) {
			client.prepareConnectionTerminateMessage().withoutError()
			require.True(t, client.connected)

			ctx, cancelFunc := context.WithCancel(context.Background())

			cancelFunc()
			require.Eventually(t, handlerRoutine(ctx), 1*time.Second, 5*time.Millisecond)

			assert.False(t, client.connected)
		})
	})

}

func jsonizePayload(t *testing.T, payload interface{}) json.RawMessage {
	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	return jsonBytes
}
