package websocket

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/subscription"
)

type testServerWebsocketResponse struct {
	data        []byte
	opCode      ws.OpCode
	statusCode  ws.StatusCode
	closeReason string
	err         error
}

func TestClient_WriteToClient(t *testing.T) {
	t.Run("should write successfully to client", func(t *testing.T) {
		connToServer, connToClient := net.Pipe()
		websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
		messageToClient := []byte(`{
			"id": "1",
			"type": "data",
			"payload": {"data":null}
		}`)

		go func() {
			err := websocketClient.WriteBytesToClient(messageToClient)
			assert.NoError(t, err)
		}()

		data, opCode, err := wsutil.ReadServerData(connToServer)
		require.NoError(t, err)
		require.Equal(t, ws.OpText, opCode)

		time.Sleep(10 * time.Millisecond)
		assert.Equal(t, messageToClient, data)
	})

	t.Run("should not write to client when connection is closed", func(t *testing.T) {
		t.Run("when not wrapped", func(t *testing.T) {
			t.Run("io: read/write on closed pipe", func(t *testing.T) {
				connToServer, connToClient := net.Pipe()
				websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
				err := connToServer.Close()
				require.NoError(t, err)

				err = websocketClient.WriteBytesToClient([]byte(""))
				assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
				assert.True(t, websocketClient.isClosedConnection)
			})
		})

		t.Run("when wrapped", func(t *testing.T) {
			t.Run("io: read/write on closed pipe", func(t *testing.T) {
				connToClient := FakeConn{}
				wrappedErr := fmt.Errorf("outside wrapper: %w",
					fmt.Errorf("inner wrapper: %w",
						io.ErrClosedPipe,
					),
				)
				connToClient.setWriteReturns(0, wrappedErr)
				websocketClient := NewClient(abstractlogger.NoopLogger, &connToClient)

				err := websocketClient.WriteBytesToClient([]byte("message"))
				assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
				assert.True(t, websocketClient.isClosedConnection)
			})
		})
	})
}

func TestClient_ReadFromClient(t *testing.T) {
	t.Run("should successfully read from client", func(t *testing.T) {
		connToServer, connToClient := net.Pipe()
		websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

		messageToServer := []byte(`{
			"id": "1",
			"type": "data",
			"payload": {"data":null}
		}`)

		go func() {
			err := wsutil.WriteClientText(connToServer, messageToServer)
			require.NoError(t, err)
		}()

		time.Sleep(10 * time.Millisecond)

		messageFromClient, err := websocketClient.ReadBytesFromClient()
		assert.NoError(t, err)
		assert.Equal(t, messageToServer, messageFromClient)
	})
	t.Run("should detect a closed connection", func(t *testing.T) {
		t.Run("before read", func(t *testing.T) {
			_, connToClient := net.Pipe()
			websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
			defer connToClient.Close()
			websocketClient.isClosedConnection = true

			assert.Eventually(t, func() bool {
				_, err := websocketClient.ReadBytesFromClient()
				return assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
			}, 1*time.Second, 2*time.Millisecond)
		})
		t.Run("when not wrapped", func(t *testing.T) {
			t.Run("io.EOF", func(t *testing.T) {
				connToServer, connToClient := net.Pipe()
				websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
				err := connToServer.Close()
				require.NoError(t, err)

				_, err = websocketClient.ReadBytesFromClient()
				assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
				assert.True(t, websocketClient.isClosedConnection)
			})
			t.Run("io: read/write on closed pipe", func(t *testing.T) {
				connToClient := &FakeConn{}
				connToClient.setReadReturns(0, io.ErrClosedPipe)
				websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

				_, err := websocketClient.ReadBytesFromClient()
				assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
				assert.True(t, websocketClient.isClosedConnection)
			})
			t.Run("unexpected EOF", func(t *testing.T) {
				connToClient := &FakeConn{}
				connToClient.setReadReturns(0, io.ErrUnexpectedEOF)
				websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

				_, err := websocketClient.ReadBytesFromClient()
				assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
				assert.True(t, websocketClient.isClosedConnection)
			})
		})

		t.Run("when wrapped", func(t *testing.T) {
			t.Run("io.EOF", func(t *testing.T) {
				connToClient := &FakeConn{}
				wrappedErr := fmt.Errorf("outside wrapper: %w",
					fmt.Errorf("inner wrapper: %w",
						io.EOF,
					),
				)
				connToClient.setReadReturns(0, wrappedErr)
				websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

				_, err := websocketClient.ReadBytesFromClient()
				assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
				assert.True(t, websocketClient.isClosedConnection)
			})
			t.Run("io: read/write on closed pipe", func(t *testing.T) {
				connToClient := &FakeConn{}
				wrappedErr := fmt.Errorf("outside wrapper: %w",
					fmt.Errorf("inner wrapper: %w",
						io.ErrClosedPipe,
					),
				)
				connToClient.setReadReturns(0, wrappedErr)
				websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

				_, err := websocketClient.ReadBytesFromClient()
				assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
				assert.True(t, websocketClient.isClosedConnection)
			})
			t.Run("unexpected EOF", func(t *testing.T) {
				connToClient := &FakeConn{}
				wrappedErr := fmt.Errorf("outside wrapper: %w",
					fmt.Errorf("inner wrapper: %w",
						io.ErrUnexpectedEOF,
					),
				)
				connToClient.setReadReturns(0, wrappedErr)
				websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

				_, err := websocketClient.ReadBytesFromClient()
				assert.Equal(t, subscription.ErrTransportClientClosedConnection, err)
				assert.True(t, websocketClient.isClosedConnection)
			})
		})

	})
}

func TestClient_IsConnected(t *testing.T) {
	_, connToClient := net.Pipe()
	websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

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

func TestClient_Disconnect(t *testing.T) {
	_, connToClient := net.Pipe()
	websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)

	t.Run("should disconnect and indicate a closed connection", func(t *testing.T) {
		err := websocketClient.Disconnect()
		assert.NoError(t, err)
		assert.Equal(t, true, websocketClient.isClosedConnection)
	})
}

func TestClient_DisconnectWithReason(t *testing.T) {
	t.Run("disconnect with invalid reason", func(t *testing.T) {
		connToServer, connToClient := net.Pipe()
		websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
		serverResponseChan := make(chan testServerWebsocketResponse)

		go readServerResponse(serverResponseChan, connToServer)

		go func() {
			err := websocketClient.DisconnectWithReason(
				"invalid reason",
			)
			assert.NoError(t, err)
		}()

		assert.Eventually(t, func() bool {
			actualServerResult := <-serverResponseChan
			assert.NoError(t, actualServerResult.err)
			assert.Equal(t, ws.OpClose, actualServerResult.opCode)
			assert.Equal(t, ws.StatusCode(4400), actualServerResult.statusCode)
			assert.Equal(t, "unknown reason", actualServerResult.closeReason)
			assert.Equal(t, false, websocketClient.IsConnected())
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})

	t.Run("disconnect with reason", func(t *testing.T) {
		connToServer, connToClient := net.Pipe()
		websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
		serverResponseChan := make(chan testServerWebsocketResponse)

		go readServerResponse(serverResponseChan, connToServer)

		go func() {
			err := websocketClient.DisconnectWithReason(
				NewCloseReason(4400, "error occurred"),
			)
			assert.NoError(t, err)
		}()

		assert.Eventually(t, func() bool {
			actualServerResult := <-serverResponseChan
			assert.NoError(t, actualServerResult.err)
			assert.Equal(t, ws.OpClose, actualServerResult.opCode)
			assert.Equal(t, ws.StatusCode(4400), actualServerResult.statusCode)
			assert.Equal(t, "error occurred", actualServerResult.closeReason)
			assert.Equal(t, false, websocketClient.IsConnected())
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})

	t.Run("disconnect with compiled reason", func(t *testing.T) {
		connToServer, connToClient := net.Pipe()
		websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
		serverResponseChan := make(chan testServerWebsocketResponse)

		go readServerResponse(serverResponseChan, connToServer)

		go func() {
			err := websocketClient.DisconnectWithReason(
				CompiledCloseReasonNormal,
			)
			assert.NoError(t, err)
		}()

		assert.Eventually(t, func() bool {
			actualServerResult := <-serverResponseChan
			assert.NoError(t, actualServerResult.err)
			assert.Equal(t, ws.OpClose, actualServerResult.opCode)
			assert.Equal(t, ws.StatusCode(1000), actualServerResult.statusCode)
			assert.Equal(t, "Normal Closure", actualServerResult.closeReason)
			assert.Equal(t, false, websocketClient.IsConnected())
			return true
		}, 1*time.Second, 2*time.Millisecond)
	})
}

func TestClient_isClosedConnectionError(t *testing.T) {
	_, connToClient := net.Pipe()

	t.Run("should not close connection when it is not a closed connection error", func(t *testing.T) {
		websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
		require.False(t, websocketClient.isClosedConnection)

		isClosedConnectionError := websocketClient.isClosedConnectionError(errors.New("no closed connection err"))
		assert.False(t, isClosedConnectionError)
	})

	t.Run("should close connection when it is a closed connection error", func(t *testing.T) {
		websocketClient := NewClient(abstractlogger.NoopLogger, connToClient)
		require.False(t, websocketClient.isClosedConnection)

		isClosedConnectionError := websocketClient.isClosedConnectionError(wsutil.ClosedError{})
		assert.True(t, isClosedConnectionError)
		websocketClient.isClosedConnection = false

		require.False(t, websocketClient.isClosedConnection)
		isClosedConnectionError = websocketClient.isClosedConnectionError(wsutil.ClosedError{
			Code:   ws.StatusNormalClosure,
			Reason: "Normal Closure",
		})
		assert.True(t, isClosedConnectionError)
	})
}

type TestClient struct {
	connectionMutex   *sync.RWMutex
	messageFromClient chan []byte
	messageToClient   chan []byte
	isConnected       bool
	shouldFail        bool
}

func NewTestClient(shouldFail bool) *TestClient {
	return &TestClient{
		connectionMutex:   &sync.RWMutex{},
		messageFromClient: make(chan []byte, 1),
		messageToClient:   make(chan []byte, 1),
		isConnected:       true,
		shouldFail:        shouldFail,
	}
}

func (t *TestClient) ReadBytesFromClient() ([]byte, error) {
	if t.shouldFail {
		return nil, errors.New("shouldFail is true")
	}
	return <-t.messageFromClient, nil
}

func (t *TestClient) WriteBytesToClient(message []byte) error {
	if t.shouldFail {
		return errors.New("shouldFail is true")
	}
	t.messageToClient <- message
	return nil
}

func (t *TestClient) IsConnected() bool {
	t.connectionMutex.RLock()
	defer t.connectionMutex.RUnlock()
	return t.isConnected
}

func (t *TestClient) Disconnect() error {
	t.connectionMutex.Lock()
	defer t.connectionMutex.Unlock()
	t.isConnected = false
	return nil
}

func (t *TestClient) DisconnectWithReason(reason interface{}) error {
	t.connectionMutex.Lock()
	defer t.connectionMutex.Unlock()
	t.isConnected = false
	return nil
}

func (t *TestClient) readMessageToClient() []byte {
	return <-t.messageToClient
}

func (t *TestClient) writeMessageFromClient(message []byte) {
	t.messageFromClient <- message
}

type FakeConn struct {
	readReturnN    int
	readReturnErr  error
	writeReturnN   int
	writeReturnErr error
}

func (f *FakeConn) setReadReturns(n int, err error) {
	f.readReturnN = n
	f.readReturnErr = err
}

func (f *FakeConn) Read(b []byte) (n int, err error) {
	return f.readReturnN, f.readReturnErr
}

func (f *FakeConn) setWriteReturns(n int, err error) {
	f.writeReturnN = n
	f.writeReturnErr = err
}

func (f *FakeConn) Write(b []byte) (n int, err error) {
	return f.writeReturnN, f.writeReturnErr
}

func (f *FakeConn) Close() error {
	panic("implement me")
}

func (f *FakeConn) LocalAddr() net.Addr {
	panic("implement me")
}

func (f *FakeConn) RemoteAddr() net.Addr {
	panic("implement me")
}

func (f *FakeConn) SetDeadline(t time.Time) error {
	panic("implement me")
}

func (f *FakeConn) SetReadDeadline(t time.Time) error {
	panic("implement me")
}

func (f *FakeConn) SetWriteDeadline(t time.Time) error {
	panic("implement me")
}

func readServerResponse(responseChan chan testServerWebsocketResponse, connToServer net.Conn) {
	var statusCode ws.StatusCode
	var closeReason string
	frame, err := ws.ReadFrame(connToServer)
	if err == nil {
		statusCode, closeReason = ws.ParseCloseFrameData(frame.Payload)
	}

	response := testServerWebsocketResponse{
		data:        frame.Payload,
		opCode:      frame.Header.OpCode,
		statusCode:  statusCode,
		closeReason: closeReason,
		err:         err,
	}

	responseChan <- response
}
