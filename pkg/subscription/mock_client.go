package subscription

import (
	"errors"
)

type mockClient struct {
	messageFromServer Message
	messageToServer   Message
	err               error
	connected         bool
}

func newMockClient() *mockClient {
	return &mockClient{
		connected: true,
	}
}

func (c *mockClient) ReadFromClient() (Message, error) {
	returnMessage, returnErr := c.messageToServer, c.err
	c.messageToServer = Message{}
	c.err = nil
	return returnMessage, returnErr
}

func (c *mockClient) WriteToClient(message Message) error {
	c.messageFromServer = message
	return c.err
}

func (c *mockClient) Disconnect() error {
	c.connected = false
	return nil
}

func (c *mockClient) readFromServer() Message {
	return c.messageFromServer
}

func (c *mockClient) prepareConnectionInitMessage() *mockClient {
	c.messageToServer = Message{
		Type: MessageTypeConnectionInit,
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
