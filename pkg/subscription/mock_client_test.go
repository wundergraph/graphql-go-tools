package subscription

import (
	"errors"
)

type mockClient struct {
	messagesFromServer []Message
	messageToServer    Message
	err                error
	connected          bool
	serverHasRead      bool
}

func newMockClient() *mockClient {
	return &mockClient{
		connected: true,
	}
}

func (c *mockClient) ReadFromClient() (Message, error) {
	returnMessage, returnErr := c.messageToServer, c.err
	c.serverHasRead = true
	c.messageToServer = Message{}
	c.err = nil
	return returnMessage, returnErr
}

func (c *mockClient) WriteToClient(message Message) error {
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

func (c *mockClient) readFromServer() []Message {
	return c.messagesFromServer
}

func (c *mockClient) prepareConnectionInitMessage() *mockClient {
	c.messageToServer = Message{
		Type: MessageTypeConnectionInit,
	}

	return c
}

func (c *mockClient) prepareStartMessage(id string, payload []byte) *mockClient {
	c.messageToServer = Message{
		Id:      id,
		Type:    MessageTypeStart,
		Payload: payload,
	}

	return c
}

func (c *mockClient) prepareStopMessage(id string) *mockClient {
	c.messageToServer = Message{
		Id:      id,
		Type:    MessageTypeStop,
		Payload: nil,
	}

	return c
}

func (c *mockClient) prepareConnectionTerminateMessage() *mockClient {
	c.messageToServer = Message{
		Type: MessageTypeConnectionTerminate,
	}

	return c
}

func (c *mockClient) withoutError() *mockClient {
	c.err = nil
	return c
}

func (c *mockClient) withError() *mockClient {
	c.err = errors.New("error")
	return c
}

func (c *mockClient) and() *mockClient {
	return c
}

func (c *mockClient) resetReceivedMessages() *mockClient {
	c.messagesFromServer = []Message{}
	return c
}
