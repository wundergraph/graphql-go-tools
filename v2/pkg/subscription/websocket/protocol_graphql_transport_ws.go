package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/subscription"
)

// GraphQLTransportWSMessageType is a type that defines graphql-transport-ws message type names.
type GraphQLTransportWSMessageType string

const (
	GraphQLTransportWSMessageTypeConnectionInit GraphQLTransportWSMessageType = "connection_init"
	GraphQLTransportWSMessageTypeConnectionAck  GraphQLTransportWSMessageType = "connection_ack"
	GraphQLTransportWSMessageTypePing           GraphQLTransportWSMessageType = "ping"
	GraphQLTransportWSMessageTypePong           GraphQLTransportWSMessageType = "pong"
	GraphQLTransportWSMessageTypeSubscribe      GraphQLTransportWSMessageType = "subscribe"
	GraphQLTransportWSMessageTypeNext           GraphQLTransportWSMessageType = "next"
	GraphQLTransportWSMessageTypeError          GraphQLTransportWSMessageType = "error"
	GraphQLTransportWSMessageTypeComplete       GraphQLTransportWSMessageType = "complete"
)

const (
	GraphQLTransportWSHeartbeatPayload = `{"type":"heartbeat"}`
)

// GraphQLTransportWSMessage is a struct that can be (de)serialized to graphql-transport-ws message format.
type GraphQLTransportWSMessage struct {
	Id      string                        `json:"id,omitempty"`
	Type    GraphQLTransportWSMessageType `json:"type"`
	Payload json.RawMessage               `json:"payload,omitempty"`
}

// GraphQLTransportWSMessageSubscribePayload is a struct that can be (de)serialized to graphql-transport-ws message payload format.
type GraphQLTransportWSMessageSubscribePayload struct {
	OperationName string          `json:"operationName,omitempty"`
	Query         string          `json:"query"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Extensions    json.RawMessage `json:"extensions,omitempty"`
}

// GraphQLTransportWSMessageReader can be used to read graphql-transport-ws messages.
type GraphQLTransportWSMessageReader struct {
	logger abstractlogger.Logger
}

// Read deserializes a byte slice to the GraphQLTransportWSMessage struct.
func (g *GraphQLTransportWSMessageReader) Read(data []byte) (*GraphQLTransportWSMessage, error) {
	var message GraphQLTransportWSMessage
	err := json.Unmarshal(data, &message)
	if err != nil {
		g.logger.Error("websocket.GraphQLTransportWSMessageReader.Read: on json unmarshal",
			abstractlogger.Error(err),
			abstractlogger.ByteString("data", data),
		)

		return nil, err
	}
	return &message, nil
}

// DeserializeSubscribePayload deserialized the subscribe payload from a graphql-transport-ws message.
func (g *GraphQLTransportWSMessageReader) DeserializeSubscribePayload(message *GraphQLTransportWSMessage) (*GraphQLTransportWSMessageSubscribePayload, error) {
	var deserializedPayload GraphQLTransportWSMessageSubscribePayload
	err := json.Unmarshal(message.Payload, &deserializedPayload)
	if err != nil {
		g.logger.Error("websocket.GraphQLTransportWSMessageReader.DeserializeSubscribePayload: on subscribe payload deserialization",
			abstractlogger.Error(err),
			abstractlogger.ByteString("payload", message.Payload),
		)
		return nil, err
	}

	return &deserializedPayload, nil
}

// GraphQLTransportWSMessageWriter can be used to write graphql-transport-ws messages to a transport client.
type GraphQLTransportWSMessageWriter struct {
	logger abstractlogger.Logger
	mu     *sync.Mutex
	Client subscription.TransportClient
}

// WriteConnectionAck writes a message of type 'connection_ack' to the transport client.
func (g *GraphQLTransportWSMessageWriter) WriteConnectionAck() error {
	message := &GraphQLTransportWSMessage{
		Type: GraphQLTransportWSMessageTypeConnectionAck,
	}
	return g.write(message)
}

// WritePing writes a message of type 'ping' to the transport client. Payload is optional.
func (g *GraphQLTransportWSMessageWriter) WritePing(payload []byte) error {
	message := &GraphQLTransportWSMessage{
		Type:    GraphQLTransportWSMessageTypePing,
		Payload: payload,
	}
	return g.write(message)
}

// WritePong writes a message of type 'pong' to the transport client. Payload is optional.
func (g *GraphQLTransportWSMessageWriter) WritePong(payload []byte) error {
	message := &GraphQLTransportWSMessage{
		Type:    GraphQLTransportWSMessageTypePong,
		Payload: payload,
	}
	return g.write(message)
}

// WriteNext writes a message of type 'next' to the transport client including the execution result as payload.
func (g *GraphQLTransportWSMessageWriter) WriteNext(id string, executionResult []byte) error {
	message := &GraphQLTransportWSMessage{
		Id:      id,
		Type:    GraphQLTransportWSMessageTypeNext,
		Payload: executionResult,
	}
	return g.write(message)
}

// WriteError writes a message of type 'error' to the transport client including the graphql errors as payload.
func (g *GraphQLTransportWSMessageWriter) WriteError(id string, graphqlErrors graphql.RequestErrors) error {
	payloadBytes, err := json.Marshal(graphqlErrors)
	if err != nil {
		return err
	}
	message := &GraphQLTransportWSMessage{
		Id:      id,
		Type:    GraphQLTransportWSMessageTypeError,
		Payload: payloadBytes,
	}
	return g.write(message)
}

// WriteComplete writes a message of type 'complete' to the transport client.
func (g *GraphQLTransportWSMessageWriter) WriteComplete(id string) error {
	message := &GraphQLTransportWSMessage{
		Id:   id,
		Type: GraphQLTransportWSMessageTypeComplete,
	}
	return g.write(message)
}

func (g *GraphQLTransportWSMessageWriter) write(message *GraphQLTransportWSMessage) error {
	jsonData, err := json.Marshal(message)
	if err != nil {
		g.logger.Error("websocket.GraphQLTransportWSMessageWriter.write: on json marshal",
			abstractlogger.Error(err),
			abstractlogger.String("id", message.Id),
			abstractlogger.String("type", string(message.Type)),
			abstractlogger.Any("payload", message.Payload),
		)
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.Client.WriteBytesToClient(jsonData)
}

// GraphQLTransportWSEventHandler can be used to handle subscription events and forward them to a GraphQLTransportWSMessageWriter.
type GraphQLTransportWSEventHandler struct {
	logger             abstractlogger.Logger
	Writer             GraphQLTransportWSMessageWriter
	OnConnectionOpened func()
}

// Emit is an implementation of subscription.EventHandler. It forwards some events to the HandleWriteEvent.
func (g *GraphQLTransportWSEventHandler) Emit(eventType subscription.EventType, id string, data []byte, err error) {
	messageType := GraphQLTransportWSMessageType("")
	switch eventType {
	case subscription.EventTypeOnSubscriptionCompleted:
		messageType = GraphQLTransportWSMessageTypeComplete
	case subscription.EventTypeOnSubscriptionData:
		messageType = GraphQLTransportWSMessageTypeNext
	case subscription.EventTypeOnNonSubscriptionExecutionResult:
		g.HandleWriteEvent(GraphQLTransportWSMessageTypeNext, id, data, err)
		g.HandleWriteEvent(GraphQLTransportWSMessageTypeComplete, id, data, err)
		return
	case subscription.EventTypeOnError:
		messageType = GraphQLTransportWSMessageTypeError
	case subscription.EventTypeOnConnectionOpened:
		if g.OnConnectionOpened != nil {
			g.OnConnectionOpened()
		}
		return
	case subscription.EventTypeOnDuplicatedSubscriberID:
		err = g.Writer.Client.DisconnectWithReason(
			NewCloseReason(4409, fmt.Sprintf("Subscriber for %s already exists", id)),
		)

		if err != nil {
			g.logger.Error("websocket.GraphQLTransportWSEventHandler.Emit: on duplicate subscriber id handling",
				abstractlogger.Error(err),
				abstractlogger.String("id", id),
				abstractlogger.String("type", string(messageType)),
				abstractlogger.ByteString("payload", data),
			)
		}
		return
	default:
		return
	}
	g.HandleWriteEvent(messageType, id, data, err)
}

// HandleWriteEvent forwards messages to the underlying writer.
func (g *GraphQLTransportWSEventHandler) HandleWriteEvent(messageType GraphQLTransportWSMessageType, id string, data []byte, providedErr error) {
	var err error
	switch messageType {
	case GraphQLTransportWSMessageTypeComplete:
		err = g.Writer.WriteComplete(id)
	case GraphQLTransportWSMessageTypeNext:
		err = g.Writer.WriteNext(id, data)
	case GraphQLTransportWSMessageTypeError:
		err = g.Writer.WriteError(id, graphql.RequestErrorsFromError(providedErr))
	case GraphQLTransportWSMessageTypeConnectionAck:
		err = g.Writer.WriteConnectionAck()
	case GraphQLTransportWSMessageTypePing:
		err = g.Writer.WritePing(data)
	case GraphQLTransportWSMessageTypePong:
		err = g.Writer.WritePong(data)
	default:
		g.logger.Warn("websocket.GraphQLTransportWSEventHandler.HandleWriteEvent: on write event handling with unexpected message type",
			abstractlogger.Error(err),
			abstractlogger.String("id", id),
			abstractlogger.String("type", string(messageType)),
			abstractlogger.ByteString("payload", data),
			abstractlogger.Error(providedErr),
		)
		err = g.Writer.Client.DisconnectWithReason(
			NewCloseReason(
				4400,
				fmt.Sprintf("invalid type '%s'", string(messageType)),
			),
		)
		if err != nil {
			g.logger.Error("websocket.GraphQLTransportWSEventHandler.HandleWriteEvent: after disconnecting on write event handling with unexpected message type",
				abstractlogger.Error(err),
				abstractlogger.String("id", id),
				abstractlogger.String("type", string(messageType)),
				abstractlogger.ByteString("payload", data),
			)
		}
		return
	}
	if err != nil {
		g.logger.Error("websocket.GraphQLTransportWSEventHandler.HandleWriteEvent: on write event handling",
			abstractlogger.Error(err),
			abstractlogger.String("id", id),
			abstractlogger.String("type", string(messageType)),
			abstractlogger.ByteString("payload", data),
			abstractlogger.Error(providedErr),
		)
	}
}

// ProtocolGraphQLTransportWSHandlerOptions can be used to provide options to the graphql-transport-ws protocol handler.
type ProtocolGraphQLTransportWSHandlerOptions struct {
	Logger                    abstractlogger.Logger
	WebSocketInitFunc         InitFunc
	CustomKeepAliveInterval   time.Duration
	CustomInitTimeOutDuration time.Duration
}

// ProtocolGraphQLTransportWSHandler is able to handle the graphql-transport-ws protocol.
type ProtocolGraphQLTransportWSHandler struct {
	logger                        abstractlogger.Logger
	reader                        GraphQLTransportWSMessageReader
	eventHandler                  GraphQLTransportWSEventHandler
	connectionInitialized         bool
	heartbeatInterval             time.Duration
	heartbeatStarted              bool
	initFunc                      InitFunc
	connectionAcknowledged        bool
	connectionInitTimerStarted    bool
	connectionInitTimeOutCancel   context.CancelFunc
	connectionInitTimeOutDuration time.Duration
}

// NewProtocolGraphQLTransportWSHandler creates a new ProtocolGraphQLTransportWSHandler with default options.
func NewProtocolGraphQLTransportWSHandler(client subscription.TransportClient) (*ProtocolGraphQLTransportWSHandler, error) {
	return NewProtocolGraphQLTransportWSHandlerWithOptions(client, ProtocolGraphQLTransportWSHandlerOptions{})
}

// NewProtocolGraphQLTransportWSHandlerWithOptions creates a new ProtocolGraphQLTransportWSHandler. It requires an option struct.
func NewProtocolGraphQLTransportWSHandlerWithOptions(client subscription.TransportClient, opts ProtocolGraphQLTransportWSHandlerOptions) (*ProtocolGraphQLTransportWSHandler, error) {
	protocolHandler := &ProtocolGraphQLTransportWSHandler{
		logger: abstractlogger.Noop{},
		reader: GraphQLTransportWSMessageReader{
			logger: abstractlogger.Noop{},
		},
		eventHandler: GraphQLTransportWSEventHandler{
			logger: abstractlogger.Noop{},
			Writer: GraphQLTransportWSMessageWriter{
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
		protocolHandler.eventHandler.logger = opts.Logger
		protocolHandler.eventHandler.Writer.logger = opts.Logger
	}

	if opts.CustomKeepAliveInterval != 0 {
		protocolHandler.heartbeatInterval = opts.CustomKeepAliveInterval
	} else {
		parsedKeepAliveInterval, err := time.ParseDuration(subscription.DefaultKeepAliveInterval)
		if err != nil {
			return nil, err
		}
		protocolHandler.heartbeatInterval = parsedKeepAliveInterval
	}

	if opts.CustomInitTimeOutDuration != 0 {
		protocolHandler.connectionInitTimeOutDuration = opts.CustomInitTimeOutDuration
	} else {
		timeOutDuration, err := time.ParseDuration(DefaultConnectionInitTimeOut)
		if err != nil {
			return nil, err
		}
		protocolHandler.connectionInitTimeOutDuration = timeOutDuration
	}

	// Pass event functions
	protocolHandler.eventHandler.OnConnectionOpened = protocolHandler.startConnectionInitTimer

	return protocolHandler, nil
}

// Handle will handle the actual graphql-transport-ws protocol messages. It's an implementation of subscription.Protocol.
func (p *ProtocolGraphQLTransportWSHandler) Handle(ctx context.Context, engine subscription.Engine, data []byte) error {
	if !p.connectionAcknowledged && !p.connectionInitTimerStarted {
		p.startConnectionInitTimer()
	}

	message, err := p.reader.Read(data)
	if err != nil {
		var jsonSyntaxError *json.SyntaxError
		if errors.As(err, &jsonSyntaxError) {
			p.closeConnectionWithReason(NewCloseReason(4400, "JSON syntax error"))
			return nil
		}
		p.logger.Error("websocket.ProtocolGraphQLTransportWSHandler.Handle: on message reading",
			abstractlogger.Error(err),
			abstractlogger.ByteString("payload", data),
		)
		return err
	}
	switch message.Type {
	case GraphQLTransportWSMessageTypeConnectionInit:
		ctx, err = p.handleInit(ctx, message.Payload)
		if err != nil {
			p.logger.Error("websocket.ProtocolGraphQLTransportWSHandler.Handle: on handling init",
				abstractlogger.Error(err),
			)
			p.closeConnectionWithReason(
				CompiledCloseReasonInternalServerError,
			)
		}
		p.startHeartbeat(ctx)
	case GraphQLTransportWSMessageTypePing:
		p.handlePing(message.Payload)
	case GraphQLTransportWSMessageTypePong:
		return nil // no need to act on pong currently (this may change in future for heartbeat checks)
	case GraphQLTransportWSMessageTypeSubscribe:
		return p.handleSubscribe(ctx, engine, message)
	case GraphQLTransportWSMessageTypeComplete:
		return p.handleComplete(engine, message.Id)
	default:
		p.closeConnectionWithReason(
			NewCloseReason(4400, fmt.Sprintf("Invalid type '%s'", string(message.Type))),
		)
	}

	return nil
}

// EventHandler returns the underlying graphql-transport-ws event handler. It's an implementation of subscription.Protocol.
func (p *ProtocolGraphQLTransportWSHandler) EventHandler() subscription.EventHandler {
	return &p.eventHandler
}

func (p *ProtocolGraphQLTransportWSHandler) startConnectionInitTimer() {
	if p.connectionInitTimerStarted {
		return
	}

	timeOutContext, timeOutContextCancel := context.WithCancel(context.Background())
	p.connectionInitTimeOutCancel = timeOutContextCancel
	p.connectionInitTimerStarted = true
	timeOutParams := subscription.TimeOutParams{
		Name:           "connection init time out",
		Logger:         p.logger,
		TimeOutContext: timeOutContext,
		TimeOutAction: func() {
			p.closeConnectionWithReason(
				NewCloseReason(4408, "Connection initialisation timeout"),
			)
		},
		TimeOutDuration: p.connectionInitTimeOutDuration,
	}
	go subscription.TimeOutChecker(timeOutParams)
}

func (p *ProtocolGraphQLTransportWSHandler) stopConnectionInitTimer() bool {
	if p.connectionInitTimeOutCancel == nil {
		return false
	}

	p.connectionInitTimeOutCancel()
	p.connectionInitTimeOutCancel = nil
	return true
}

func (p *ProtocolGraphQLTransportWSHandler) startHeartbeat(ctx context.Context) {
	if p.heartbeatStarted {
		return
	}

	p.heartbeatStarted = true
	go p.heartbeat(ctx)
}

func (p *ProtocolGraphQLTransportWSHandler) heartbeat(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.heartbeatInterval):
			p.eventHandler.HandleWriteEvent(GraphQLTransportWSMessageTypePong, "", []byte(GraphQLTransportWSHeartbeatPayload), nil)
		}
	}
}

func (p *ProtocolGraphQLTransportWSHandler) handleInit(ctx context.Context, payload []byte) (context.Context, error) {
	if p.connectionInitialized {
		p.closeConnectionWithReason(
			NewCloseReason(4429, "Too many initialisation requests"),
		)
		return ctx, nil
	}

	initCtx := ctx
	if p.initFunc != nil && len(payload) > 0 {
		// check initial payload to see whether to accept the websocket connection
		var err error
		if initCtx, err = p.initFunc(ctx, payload); err != nil {
			return initCtx, err
		}
	}

	if p.stopConnectionInitTimer() {
		p.eventHandler.HandleWriteEvent(GraphQLTransportWSMessageTypeConnectionAck, "", nil, nil)
	} else {
		p.closeConnectionWithReason(CompiledCloseReasonInternalServerError)
	}
	p.connectionInitialized = true
	return initCtx, nil
}

func (p *ProtocolGraphQLTransportWSHandler) handlePing(payload []byte) {
	// Pong should return the same payload as ping.
	// https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API/Writing_WebSocket_servers#pings_and_pongs_the_heartbeat_of_websockets
	p.eventHandler.HandleWriteEvent(GraphQLTransportWSMessageTypePong, "", payload, nil)
}

func (p *ProtocolGraphQLTransportWSHandler) handleSubscribe(ctx context.Context, engine subscription.Engine, message *GraphQLTransportWSMessage) error {
	if !p.connectionInitialized {
		p.closeConnectionWithReason(
			NewCloseReason(4401, "Unauthorized"),
		)
		return nil
	}

	subscribePayload, err := p.reader.DeserializeSubscribePayload(message)
	if err != nil {
		return err
	}

	enginePayload := graphql.Request{
		OperationName: subscribePayload.OperationName,
		Query:         subscribePayload.Query,
		Variables:     subscribePayload.Variables,
	}

	enginePayloadBytes, err := json.Marshal(enginePayload)
	if err != nil {
		return err
	}

	return engine.StartOperation(ctx, message.Id, enginePayloadBytes, &p.eventHandler)
}

func (p *ProtocolGraphQLTransportWSHandler) handleComplete(engine subscription.Engine, id string) error {
	return engine.StopSubscription(id, &p.eventHandler)
}

func (p *ProtocolGraphQLTransportWSHandler) closeConnectionWithReason(reason interface{}) {
	err := p.eventHandler.Writer.Client.DisconnectWithReason(
		reason,
	)
	if err != nil {
		p.logger.Error("websocket.ProtocolGraphQLTransportWSHandler.closeConnectionWithReason: after trying to disconnect with reason",
			abstractlogger.Error(err),
		)
	}
}

// Interface guards
var _ subscription.EventHandler = (*GraphQLTransportWSEventHandler)(nil)
var _ subscription.Protocol = (*ProtocolGraphQLTransportWSHandler)(nil)
