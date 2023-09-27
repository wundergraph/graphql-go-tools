package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/pkg/graphql"
	"github.com/wundergraph/graphql-go-tools/pkg/subscription"
)

// GraphQLWSMessageType is a type that defines graphql-ws message type names.
type GraphQLWSMessageType string

const (
	GraphQLWSMessageTypeConnectionInit      GraphQLWSMessageType = "connection_init"
	GraphQLWSMessageTypeConnectionAck       GraphQLWSMessageType = "connection_ack"
	GraphQLWSMessageTypeConnectionError     GraphQLWSMessageType = "connection_error"
	GraphQLWSMessageTypeConnectionTerminate GraphQLWSMessageType = "connection_terminate"
	GraphQLWSMessageTypeConnectionKeepAlive GraphQLWSMessageType = "ka"
	GraphQLWSMessageTypeStart               GraphQLWSMessageType = "start"
	GraphQLWSMessageTypeStop                GraphQLWSMessageType = "stop"
	GraphQLWSMessageTypeData                GraphQLWSMessageType = "data"
	GraphQLWSMessageTypeError               GraphQLWSMessageType = "error"
	GraphQLWSMessageTypeComplete            GraphQLWSMessageType = "complete"
)

var ErrGraphQLWSUnexpectedMessageType = errors.New("unexpected message type")

// GraphQLWSMessage is a struct that can be (de)serialized to graphql-ws message format.
type GraphQLWSMessage struct {
	Id      string               `json:"id,omitempty"`
	Type    GraphQLWSMessageType `json:"type"`
	Payload json.RawMessage      `json:"payload,omitempty"`
}

// GraphQLWSMessageReader can be used to read graphql-ws messages.
type GraphQLWSMessageReader struct {
	logger abstractlogger.Logger
}

// Read deserializes a byte slice to the GraphQLWSMessage struct.
func (g *GraphQLWSMessageReader) Read(data []byte) (*GraphQLWSMessage, error) {
	var message GraphQLWSMessage
	err := json.Unmarshal(data, &message)
	if err != nil {
		g.logger.Error("websocket.GraphQLWSMessageReader.Read: on json unmarshal",
			abstractlogger.Error(err),
			abstractlogger.ByteString("data", data),
		)

		return nil, err
	}
	return &message, nil
}

// GraphQLWSMessageWriter can be used to write graphql-ws messages to a transport client.
type GraphQLWSMessageWriter struct {
	logger abstractlogger.Logger
	mu     *sync.Mutex
	Client subscription.TransportClient
}

// WriteData writes a message of type 'data' to the transport client.
func (g *GraphQLWSMessageWriter) WriteData(id string, responseData []byte) error {
	message := &GraphQLWSMessage{
		Id:      id,
		Type:    GraphQLWSMessageTypeData,
		Payload: responseData,
	}
	return g.write(message)
}

// WriteComplete writes a message of type 'complete' to the transport client.
func (g *GraphQLWSMessageWriter) WriteComplete(id string) error {
	message := &GraphQLWSMessage{
		Id:      id,
		Type:    GraphQLWSMessageTypeComplete,
		Payload: nil,
	}
	return g.write(message)
}

// WriteKeepAlive writes a message of type 'ka' to the transport client.
func (g *GraphQLWSMessageWriter) WriteKeepAlive() error {
	message := &GraphQLWSMessage{
		Type:    GraphQLWSMessageTypeConnectionKeepAlive,
		Payload: nil,
	}
	return g.write(message)
}

// WriteTerminate writes a message of type 'connection_terminate' to the transport client.
func (g *GraphQLWSMessageWriter) WriteTerminate(reason string) error {
	payloadBytes, err := json.Marshal(reason)
	if err != nil {
		return err
	}
	message := &GraphQLWSMessage{
		Type:    GraphQLWSMessageTypeConnectionTerminate,
		Payload: payloadBytes,
	}
	return g.write(message)
}

// WriteConnectionError writes a message of type 'connection_error' to the transport client.
func (g *GraphQLWSMessageWriter) WriteConnectionError(reason string) error {
	payloadBytes, err := json.Marshal(reason)
	if err != nil {
		return err
	}
	message := &GraphQLWSMessage{
		Type:    GraphQLWSMessageTypeConnectionError,
		Payload: payloadBytes,
	}
	return g.write(message)
}

// WriteError writes a message of type 'error' to the transport client.
func (g *GraphQLWSMessageWriter) WriteError(id string, errors graphql.RequestErrors) error {
	payloadBytes, err := json.Marshal(errors)
	if err != nil {
		return err
	}
	message := &GraphQLWSMessage{
		Id:      id,
		Type:    GraphQLWSMessageTypeError,
		Payload: payloadBytes,
	}
	return g.write(message)
}

// WriteAck writes a message of type 'connection_ack' to the transport client.
func (g *GraphQLWSMessageWriter) WriteAck() error {
	message := &GraphQLWSMessage{
		Type: GraphQLWSMessageTypeConnectionAck,
	}
	return g.write(message)
}

func (g *GraphQLWSMessageWriter) write(message *GraphQLWSMessage) error {
	jsonData, err := json.Marshal(message)
	if err != nil {
		g.logger.Error("websocket.GraphQLWSMessageWriter.write: on json marshal",
			abstractlogger.Error(err),
			abstractlogger.String("id", message.Id),
			abstractlogger.String("type", string(message.Type)),
			abstractlogger.ByteString("payload", message.Payload),
		)
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.Client.WriteBytesToClient(jsonData)
}

// GraphQLWSWriteEventHandler can be used to handle subscription events and forward them to a GraphQLWSMessageWriter.
type GraphQLWSWriteEventHandler struct {
	logger abstractlogger.Logger
	Writer GraphQLWSMessageWriter
}

// Emit is an implementation of subscription.EventHandler. It forwards events to the HandleWriteEvent.
func (g *GraphQLWSWriteEventHandler) Emit(eventType subscription.EventType, id string, data []byte, err error) {
	messageType := GraphQLWSMessageType("")
	switch eventType {
	case subscription.EventTypeOnSubscriptionCompleted:
		messageType = GraphQLWSMessageTypeComplete
	case subscription.EventTypeOnSubscriptionData:
		messageType = GraphQLWSMessageTypeData
	case subscription.EventTypeOnNonSubscriptionExecutionResult:
		g.HandleWriteEvent(GraphQLWSMessageTypeData, id, data, err)
		g.HandleWriteEvent(GraphQLWSMessageTypeComplete, id, data, err)
		return
	case subscription.EventTypeOnError:
		messageType = GraphQLWSMessageTypeError
	case subscription.EventTypeOnDuplicatedSubscriberID:
		messageType = GraphQLWSMessageTypeError
	case subscription.EventTypeOnConnectionError:
		messageType = GraphQLWSMessageTypeConnectionError
	default:
		return
	}

	g.HandleWriteEvent(messageType, id, data, err)
}

// HandleWriteEvent forwards messages to the underlying writer.
func (g *GraphQLWSWriteEventHandler) HandleWriteEvent(messageType GraphQLWSMessageType, id string, data []byte, providedErr error) {
	var err error
	switch messageType {
	case GraphQLWSMessageTypeComplete:
		err = g.Writer.WriteComplete(id)
	case GraphQLWSMessageTypeData:
		err = g.Writer.WriteData(id, data)
	case GraphQLWSMessageTypeError:
		err = g.Writer.WriteError(id, graphql.RequestErrorsFromError(providedErr))
	case GraphQLWSMessageTypeConnectionError:
		err = g.Writer.WriteConnectionError(providedErr.Error())
	case GraphQLWSMessageTypeConnectionKeepAlive:
		err = g.Writer.WriteKeepAlive()
	case GraphQLWSMessageTypeConnectionAck:
		err = g.Writer.WriteAck()
	default:
		g.logger.Warn("websocket.GraphQLWSWriteEventHandler.HandleWriteEvent: on write event handling with unexpected message type",
			abstractlogger.Error(err),
			abstractlogger.String("id", id),
			abstractlogger.String("type", string(messageType)),
			abstractlogger.ByteString("payload", data),
			abstractlogger.Error(providedErr),
		)
		return
	}
	if err != nil {
		g.logger.Error("websocket.GraphQLWSWriteEventHandler.HandleWriteEvent: on write event handling",
			abstractlogger.Error(err),
			abstractlogger.String("id", id),
			abstractlogger.String("type", string(messageType)),
			abstractlogger.ByteString("payload", data),
			abstractlogger.Error(providedErr),
		)
	}
}

// ProtocolGraphQLWSHandlerOptions can be used to provide options to the graphql-ws protocol handler.
type ProtocolGraphQLWSHandlerOptions struct {
	Logger                  abstractlogger.Logger
	WebSocketInitFunc       InitFunc
	CustomKeepAliveInterval time.Duration
}

// ProtocolGraphQLWSHandler is able to handle the graphql-ws protocol.
type ProtocolGraphQLWSHandler struct {
	logger            abstractlogger.Logger
	reader            GraphQLWSMessageReader
	writeEventHandler GraphQLWSWriteEventHandler
	keepAliveInterval time.Duration
	initFunc          InitFunc
}

// NewProtocolGraphQLWSHandler creates a new ProtocolGraphQLWSHandler with default options.
func NewProtocolGraphQLWSHandler(client subscription.TransportClient) (*ProtocolGraphQLWSHandler, error) {
	return NewProtocolGraphQLWSHandlerWithOptions(client, ProtocolGraphQLWSHandlerOptions{})
}

// NewProtocolGraphQLWSHandlerWithOptions creates a new ProtocolGraphQLWSHandler. It requires an option struct.
func NewProtocolGraphQLWSHandlerWithOptions(client subscription.TransportClient, opts ProtocolGraphQLWSHandlerOptions) (*ProtocolGraphQLWSHandler, error) {
	protocolHandler := &ProtocolGraphQLWSHandler{
		logger: abstractlogger.Noop{},
		reader: GraphQLWSMessageReader{
			logger: abstractlogger.Noop{},
		},
		writeEventHandler: GraphQLWSWriteEventHandler{
			logger: abstractlogger.Noop{},
			Writer: GraphQLWSMessageWriter{
				logger: abstractlogger.Noop{},
				Client: client,
				mu:     &sync.Mutex{},
			},
		},
		initFunc: opts.WebSocketInitFunc,
	}

	if opts.Logger != nil {
		protocolHandler.logger = opts.Logger
		protocolHandler.reader.logger = opts.Logger
		protocolHandler.writeEventHandler.logger = opts.Logger
		protocolHandler.writeEventHandler.Writer.logger = opts.Logger
	}

	if opts.CustomKeepAliveInterval != 0 {
		protocolHandler.keepAliveInterval = opts.CustomKeepAliveInterval
	} else {
		parsedKeepAliveInterval, err := time.ParseDuration(subscription.DefaultKeepAliveInterval)
		if err != nil {
			return nil, err
		}
		protocolHandler.keepAliveInterval = parsedKeepAliveInterval
	}

	return protocolHandler, nil
}

// Handle will handle the actual graphql-ws protocol messages. It's an implementation of subscription.Protocol.
func (p *ProtocolGraphQLWSHandler) Handle(ctx context.Context, engine subscription.Engine, data []byte) error {
	message, err := p.reader.Read(data)
	if err != nil {
		var jsonSyntaxError *json.SyntaxError
		if errors.As(err, &jsonSyntaxError) {
			p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeError, "", nil, errors.New("json syntax error"))
			return nil
		}
		p.logger.Error("websocket.ProtocolGraphQLWSHandler.Handle: on message reading",
			abstractlogger.Error(err),
			abstractlogger.ByteString("payload", data),
		)
		return err
	}

	switch message.Type {
	case GraphQLWSMessageTypeConnectionInit:
		ctx, err = p.handleInit(ctx, message.Payload)
		if err != nil {
			p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionError, "", nil, errors.New("failed to accept the websocket connection"))
			return engine.TerminateAllSubscriptions(&p.writeEventHandler)
		}

		go p.handleKeepAlive(ctx)
	case GraphQLWSMessageTypeStart:
		return engine.StartOperation(ctx, message.Id, message.Payload, &p.writeEventHandler)
	case GraphQLWSMessageTypeStop:
		return engine.StopSubscription(message.Id, &p.writeEventHandler)
	case GraphQLWSMessageTypeConnectionTerminate:
		return engine.TerminateAllSubscriptions(&p.writeEventHandler)
	default:
		p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionError, message.Id, nil, fmt.Errorf("%s: %s", ErrGraphQLWSUnexpectedMessageType.Error(), message.Type))
	}

	return nil
}

// EventHandler returns the underlying graphql-ws event handler. It's an implementation of subscription.Protocol.
func (p *ProtocolGraphQLWSHandler) EventHandler() subscription.EventHandler {
	return &p.writeEventHandler
}

func (p *ProtocolGraphQLWSHandler) handleInit(ctx context.Context, payload []byte) (context.Context, error) {
	initCtx := ctx
	if p.initFunc != nil && len(payload) > 0 {
		// check initial payload to see whether to accept the websocket connection
		var err error
		if initCtx, err = p.initFunc(ctx, payload); err != nil {
			return initCtx, err
		}
	}

	p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionAck, "", nil, nil)
	return initCtx, nil
}

func (p *ProtocolGraphQLWSHandler) handleKeepAlive(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.keepAliveInterval):
			p.writeEventHandler.HandleWriteEvent(GraphQLWSMessageTypeConnectionKeepAlive, "", nil, nil)
		}
	}
}

// Interface guards
var _ subscription.EventHandler = (*GraphQLWSWriteEventHandler)(nil)
var _ subscription.Protocol = (*ProtocolGraphQLWSHandler)(nil)
