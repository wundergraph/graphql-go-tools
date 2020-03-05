package subscription

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/jensneuse/graphql-go-tools/pkg/execution"
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

	DefaultKeepAliveInterval = "15s"
)

type Message struct {
	Id      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Client interface {
	ReadFromClient() (Message, error)
	WriteToClient(Message) error
	IsConnected() bool
	Disconnect() error
}

type Handler struct {
	logger            abstractlogger.Logger
	client            Client
	keepAliveInterval time.Duration
	subCancellations  subscriptionCancellations
	executionHandler  *execution.Handler
}

func NewHandler(logger abstractlogger.Logger, client Client, executionHandler *execution.Handler) (*Handler, error) {
	keepAliveInterval, err := time.ParseDuration(DefaultKeepAliveInterval)
	if err != nil {
		return nil, err
	}

	return &Handler{
		logger:            logger,
		client:            client,
		keepAliveInterval: keepAliveInterval,
		subCancellations:  subscriptionCancellations{},
		executionHandler:  executionHandler,
	}, nil
}

func (h *Handler) Handle(ctx context.Context) {
	defer func() {
		h.subCancellations.CancelAll()
	}()

	for {
		if !h.client.IsConnected() {
			h.logger.Debug("subscription.Handler.Handle()",
				abstractlogger.String("message", "client has disconnected"),
			)

			return
		}

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
			case MessageTypeStart:
				h.handleStart(message.Id, message.Payload)
			case MessageTypeStop:
				h.handleStop(message.Id)
			case MessageTypeConnectionTerminate:
				h.handleConnectionTerminate()
				return
			}
		}

		select {
		case <-ctx.Done():
			return
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

func (h *Handler) handleStart(id string, payload []byte) {
	ctx := h.subCancellations.Add(id)
	go h.startSubscription(ctx, id, payload)
}

func (h *Handler) startSubscription(ctx context.Context, id string, data []byte) {
	executor, node, executionContext, err := h.executionHandler.Handle(data, []byte(""))
	if err != nil {
		h.logger.Error("subscription.Handler.startSubscription()",
			abstractlogger.Error(err),
		)

		h.handleError(id, "error on subscription execution")
	}

	executionContext.Context = ctx
	buf := bytes.NewBuffer(make([]byte, 0, 1024))

	for {
		buf.Reset()
		select {
		case <-ctx.Done():
			return
		default:
			h.executeSubscription(buf, id, executor, node, executionContext)
		}
	}

}

func (h *Handler) executeSubscription(buf *bytes.Buffer, id string, executor *execution.Executor, node execution.RootNode, ctx execution.Context) {
	_, err := executor.Execute(ctx, node, buf)
	if err != nil {
		h.logger.Error("subscription.Handle.executeSubscription()",
			abstractlogger.Error(err),
		)

		h.handleError(id, "error on subscription execution")
		return
	}

	h.logger.Debug("subscription.Handle.executeSubscription()",
		abstractlogger.ByteString("execution_result", buf.Bytes()),
	)

	h.sendData(id, buf.Bytes())

	// TODO: send complete?
}

func (h *Handler) handleStop(id string) {
	h.subCancellations.Cancel(id)
}

func (h *Handler) sendData(id string, responseData []byte) {
	dataMessage := Message{
		Id:      id,
		Type:    MessageTypeData,
		Payload: responseData,
	}

	err := h.client.WriteToClient(dataMessage)
	if err != nil {
		h.logger.Error("subscription.Handler.sendData()",
			abstractlogger.Error(err),
		)
	}
}

func (h *Handler) sendComplete(id string) {
	completeMessage := Message{
		Id:      id,
		Type:    MessageTypeComplete,
		Payload: nil,
	}

	err := h.client.WriteToClient(completeMessage)
	if err != nil {
		h.logger.Error("subscription.Handler.sendComplete()",
			abstractlogger.Error(err),
		)
	}
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
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(h.keepAliveInterval):
			h.sendKeepAlive()
		}
	}
}

func (h *Handler) sendKeepAlive() {
	keepAliveMessage := Message{
		Type: MessageTypeConnectionKeepAlive,
	}

	err := h.client.WriteToClient(keepAliveMessage)
	if err != nil {
		h.logger.Error("subscription.Handler.sendKeepAlive()",
			abstractlogger.Error(err),
		)
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
		Id:      id,
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

func (h *Handler) activeSubscriptions() int {
	return len(h.subCancellations)
}
