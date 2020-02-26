package subscriptionhandler

import (
	"encoding/json"
)

const (
	CONNECTION_INIT       = "connection_init"
	CONNECTION_ACK        = "connection_ack"
	CONNECTION_ERROR      = "connection_error"
	CONNECTION_KEEP_ALIVE = "ka"
	START                 = "start"
	STOP                  = "stop"
	CONNECTION_TERMINATE  = "connection_terminate"
	DATA                  = "data"
	ERROR                 = "error"
	COMPLETE              = "complete"
)

type SubscriptionMessage struct {
	Id      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ClientSubscription interface {
	ReadFromClient() (SubscriptionMessage, error)
	WriteToClient(message SubscriptionMessage) error
}

type SubscriptionHandler struct {
}

func (s *SubscriptionHandler) Handle(subscription ClientSubscription) {
	for {
		message, err := subscription.ReadFromClient()
		if err != nil {
			panic("should not panic")
		}
		switch message.Type {
		case CONNECTION_INIT:
			subscription.WriteToClient(SubscriptionMessage{})
		case START:
		case STOP:
		}
	}
}