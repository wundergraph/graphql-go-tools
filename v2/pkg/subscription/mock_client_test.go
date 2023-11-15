package subscription

import (
	"errors"
	"sync"
)

type mockClient struct {
	mu                 sync.Mutex
	messagesFromServer []Message
	messageToServer    *Message
	err                error
	messagePipe        chan *Message
	connected          bool
	serverHasRead      bool
}

func newMockClient() *mockClient {
	return &mockClient{
		connected:   true,
		messagePipe: make(chan *Message, 1),
	}
}

func (c *mockClient) ReadFromClient() (*Message, error) {
	c.mu.Lock()
	returnErr := c.err
	c.mu.Unlock()
	returnMessage := <-c.messagePipe
	if returnErr != nil {
		return nil, returnErr
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.serverHasRead = true
	c.err = nil
	return returnMessage, returnErr
}

func (c *mockClient) WriteToClient(message Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messagesFromServer = append(c.messagesFromServer, message)
	return c.err
}

func (c *mockClient) IsConnected() bool {
	return c.connected
}

func (c *mockClient) Disconnect() error {
	c.connected = false
	return nil
}

func (c *mockClient) hasMoreMessagesThan(num int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messagesFromServer) > num
}

func (c *mockClient) readFromServer() []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.messagesFromServer[0:len(c.messagesFromServer):len(c.messagesFromServer)]
}

func (c *mockClient) prepareConnectionInitMessage() *mockClient {
	c.messageToServer = &Message{
		Type: MessageTypeConnectionInit,
	}

	return c
}

func (c *mockClient) prepareConnectionInitMessageWithPayload(payload []byte) *mockClient {
	c.messageToServer = &Message{
		Type:    MessageTypeConnectionInit,
		Payload: payload,
	}

	return c
}

func (c *mockClient) prepareStartMessage(id string, payload []byte) *mockClient {
	c.messageToServer = &Message{
		Id:      id,
		Type:    MessageTypeStart,
		Payload: payload,
	}

	return c
}

func (c *mockClient) prepareStopMessage(id string) *mockClient {
	c.messageToServer = &Message{
		Id:      id,
		Type:    MessageTypeStop,
		Payload: nil,
	}

	return c
}

func (c *mockClient) prepareConnectionTerminateMessage() *mockClient {
	c.messageToServer = &Message{
		Type: MessageTypeConnectionTerminate,
	}

	return c
}

func (c *mockClient) send() bool {
	c.messagePipe <- c.messageToServer
	c.messageToServer = nil
	return true
}

func (c *mockClient) withoutError() *mockClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = nil
	return c
}

func (c *mockClient) withError() *mockClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = errors.New("error")
	return c
}

func (c *mockClient) and() *mockClient {
	return c
}

func (c *mockClient) reset() *mockClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messagesFromServer = []Message{}
	c.err = nil
	return c
}

func (c *mockClient) reconnect() *mockClient {
	c.reset()
	c.connected = true
	return c
}
