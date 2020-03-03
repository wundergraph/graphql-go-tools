package subscription

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jensneuse/abstractlogger"
)

const (
	MessageTypeConnectionInit      = "connection_init"
	MessageTypeConnectionAck       = "connection_ack"
	MessageTypeConnectionError     = "connection_error"
	MessageTypeConnectionTerminate = "connection_terminate"
	MessageTypeConnectionKeepAlive = "connection_keep_alive"
	MessageTypeStart               = "start"
	MessageTypeStop                = "stop"
	MessageTypeData                = "data"
	MessageTypeError               = "error"
	MessageTypeComplete            = "complete"

	DefaultKeepAliveInterval = "30s"
)

type Message struct {
	Id      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Client interface {
	ReadFromClient() (Message, error)
	WriteToClient(Message) error
	Disconnect() error
}

type Handler struct {
	logger            abstractlogger.Logger
	client            Client
	keepAliveInterval time.Duration
	subCancellations  subscriptionCancellations
}

func NewHandler(logger abstractlogger.Logger, client Client) (*Handler, error) {
	keepAliveInterval, err := time.ParseDuration(DefaultKeepAliveInterval)
	if err != nil {
		return nil, err
	}

	return &Handler{
		logger:            logger,
		client:            client,
		keepAliveInterval: keepAliveInterval,
	}, nil
}

func (h *Handler) Handle(ctx context.Context) {
	runHandleLoop := true

	for runHandleLoop {
		message, err := h.client.ReadFromClient()
		if err != nil {
			h.logger.Error("subscription.Handler.Handle()",
				abstractlogger.Error(err),
				abstractlogger.Any("message", message),
			)

			h.handleConnectionError("could not read message from client")
		} else {
			switch message.Type {
			case MessageTypeConnectionInit:
				h.handleInit()
				go h.handleKeepAlive(ctx)
			case MessageTypeConnectionTerminate:
				h.handleConnectionTerminate()
				runHandleLoop = false
			}
		}

		select {
		case <-ctx.Done():
			runHandleLoop = false
		default:
			continue
		}
	}
}

func (h *Handler) ChangeKeepAliveInterval(d time.Duration) {
	h.keepAliveInterval = d
}

func (h *Handler) handleInit() {
	ackMessage := Message{
		Type: MessageTypeConnectionAck,
	}

	err := h.client.WriteToClient(ackMessage)
	if err != nil {
		h.logger.Error("subscription.Handler.handleInit()",
			abstractlogger.Error(err),
		)
	}
}

func (h *Handler) handleStart() {

}

func (h *Handler) handleStop() {

}

func (h *Handler) handleConnectionTerminate() {
	err := h.client.Disconnect()
	if err != nil {
		h.logger.Error("subscription.Handler.handleConnectionTerminate()",
			abstractlogger.Error(err),
		)
	}
}

func (h *Handler) handleKeepAlive(ctx context.Context) {
	runKeepAliveLoop := true
	for runKeepAliveLoop {
		time.Sleep(h.keepAliveInterval)

		keepAliveMessage := Message{
			Type: MessageTypeConnectionKeepAlive,
		}

		err := h.client.WriteToClient(keepAliveMessage)
		if err != nil {
			h.logger.Error("subscription.Handler.handleKeepAlive()",
				abstractlogger.Error(err),
			)
		}

		select {
		case <-ctx.Done():
			runKeepAliveLoop = false
		default:
			continue
		}
	}
}

func (h *Handler) handleConnectionError(errorPayload interface{}) {
	payloadBytes, err := json.Marshal(errorPayload)
	if err != nil {
		h.logger.Error("subscription.Handler.handleConnectionError()",
			abstractlogger.Error(err),
			abstractlogger.Any("errorPayload", errorPayload),
		)
	}

	connectionErrorMessage := Message{
		Type:    MessageTypeConnectionError,
		Payload: payloadBytes,
	}

	err = h.client.WriteToClient(connectionErrorMessage)
	if err != nil {
		h.logger.Error("subscription.Handler.handleConnectionError()",
			abstractlogger.Error(err),
		)
	}
}

func (h *Handler) handleError(id string, errorPayload interface{}) {
	payloadBytes, err := json.Marshal(errorPayload)
	if err != nil {
		h.logger.Error("subscription.Handler.handleError()",
			abstractlogger.Error(err),
			abstractlogger.Any("errorPayload", errorPayload),
		)
	}

	errorMessage := Message{
		Id:      "",
		Type:    MessageTypeError,
		Payload: payloadBytes,
	}

	err = h.client.WriteToClient(errorMessage)
	if err != nil {
		h.logger.Error("subscription.Handler.handleError()",
			abstractlogger.Error(err),
		)
	}
}
