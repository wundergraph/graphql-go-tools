package pubsub_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type EventType string

const (
	EventTypePublish   EventType = "publish"
	EventTypeRequest   EventType = "request"
	EventTypeSubscribe EventType = "subscribe"
)

func EventTypeFromString(s string) (EventType, error) {
	et := EventType(strings.ToLower(s))
	switch et {
	case EventTypePublish, EventTypeRequest, EventTypeSubscribe:
		return et, nil
	default:
		return "", fmt.Errorf("invalid event type: %q", s)
	}
}

type EventConfiguration struct {
	FieldName  string    `json:"fieldName"`
	SourceName string    `json:"sourceName"`
	Topic      string    `json:"topic"`
	Type       EventType `json:"type"`
	TypeName   string    `json:"typeName"`
}

type Configuration struct {
	Events []EventConfiguration `json:"events"`
}

type Planner[T Configuration] struct {
	config  Configuration
	current struct {
		topic  string
		data   []byte
		config *EventConfiguration
	}
	pubSubBySourceName map[string]PubSub
	rootFieldRef       int
	variables          resolve.Variables
	visitor            *plan.Visitor
}

func (p *Planner[T]) EnterField(ref int) {
	if p.rootFieldRef == -1 {
		p.rootFieldRef = ref
	} else {
		// This is a nested field, we don't need to do anything here
		return
	}
	fieldName := p.visitor.Operation.FieldNameString(ref)
	typeName := p.visitor.Walker.EnclosingTypeDefinition.NameString(p.visitor.Definition)
	var eventConfig *EventConfiguration
	for _, cfg := range p.config.Events {
		if cfg.TypeName == typeName && cfg.FieldName == fieldName {
			eventConfig = &cfg
			break
		}
	}
	if eventConfig == nil {
		return
	}

	topic := eventConfig.Topic
	rex, err := regexp.Compile(`{{ args.([a-zA-Z0-9_]+) }}`)
	if err != nil {
		return
	}
	matches := rex.FindAllStringSubmatch(topic, -1)
	if len(matches) != 1 || len(matches[0]) != 2 {
		return
	}
	argName := matches[0][1]
	// We need to find the argument in the operation
	arg, ok := p.visitor.Operation.FieldArgument(ref, []byte(argName))
	if !ok {
		return
	}
	argValue := p.visitor.Operation.ArgumentValue(arg)
	if argValue.Kind != ast.ValueKindVariable {
		return
	}
	variableName := p.visitor.Operation.VariableValueNameBytes(argValue.Ref)
	variableDefinition, ok := p.visitor.Operation.VariableDefinitionByNameAndOperation(p.visitor.Walker.Ancestors[0].Ref, variableName)
	if !ok {
		return
	}
	variableTypeRef := p.visitor.Operation.VariableDefinitions[variableDefinition].Type
	renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(p.visitor.Operation, p.visitor.Operation, variableTypeRef, string(variableName))
	if err != nil {
		return
	}
	contextVariable := &resolve.ContextVariable{
		Path:     []string{string(variableName)},
		Renderer: renderer,
	}
	variablePlaceHolder, exists := p.variables.AddVariable(contextVariable) // $$0$$
	if exists {
		return
	}
	// We need to replace the template literal with the variable placeholder
	p.current.topic = rex.ReplaceAllLiteralString(topic, variablePlaceHolder)

	// Collect the field arguments for fetch based operations
	fieldArgs := p.visitor.Operation.FieldArguments(ref)
	var dataBuffer bytes.Buffer
	dataBuffer.WriteByte('{')
	for ii, arg := range fieldArgs {
		if ii > 0 {
			dataBuffer.WriteByte(',')
		}
		argValue := p.visitor.Operation.ArgumentValue(arg)
		renderer := resolve.NewJSONVariableRenderer()
		variableName := p.visitor.Operation.VariableValueNameBytes(argValue.Ref)
		contextVariable := &resolve.ContextVariable{
			Path:     []string{string(variableName)},
			Renderer: renderer,
		}
		variablePlaceHolder, _ := p.variables.AddVariable(contextVariable)
		argumentName := p.visitor.Operation.ArgumentNameString(arg)
		escapedKey, err := json.Marshal(argumentName)
		if err != nil {
			return
		}
		dataBuffer.Write(escapedKey)
		dataBuffer.WriteByte(':')
		dataBuffer.WriteString(variablePlaceHolder)
	}

	dataBuffer.WriteByte('}')
	p.current.config = eventConfig
	p.current.data = dataBuffer.Bytes()
}

func (p *Planner[T]) EnterDocument(operation, definition *ast.Document) {
	p.rootFieldRef = -1
	p.current.topic = ""
	p.current.config = nil
}

func (p *Planner[T]) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration[T], dataSourcePlannerConfiguration plan.DataSourcePlannerConfiguration) error {
	p.visitor = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	visitor.Walker.RegisterEnterDocumentVisitor(p)
	p.config = Configuration(configuration.CustomConfiguration())
	return nil
}

func (p *Planner[T]) ConfigureFetch() resolve.FetchConfiguration {
	if p.current.config == nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: event config is nil"))
		return resolve.FetchConfiguration{}
	}
	var dataSource resolve.DataSource
	pubsub, ok := p.pubSubBySourceName[p.current.config.SourceName]
	if !ok {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("no pubsub connection exists with source name \"%s\"", p.current.config.SourceName))
		return resolve.FetchConfiguration{}
	}
	switch p.current.config.Type {
	case EventTypePublish:
		dataSource = &PublishDataSource{
			pubSub: pubsub,
		}
	case EventTypeRequest:
		dataSource = &RequestDataSource{
			pubSub: pubsub,
		}
	default:
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: invalid event type \"%s\"", p.current.config.Type))
		return resolve.FetchConfiguration{}
	}
	return resolve.FetchConfiguration{
		Input:      fmt.Sprintf(`{"topic":"%s", "data": %s}`, p.current.topic, p.current.data),
		Variables:  p.variables,
		DataSource: dataSource,
		PostProcessing: resolve.PostProcessingConfiguration{
			MergePath: []string{p.current.config.FieldName},
		},
	}
}

func (p *Planner[T]) ConfigureSubscription() plan.SubscriptionConfiguration {
	if p.current.config == nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: event config is nil"))
		return plan.SubscriptionConfiguration{}
	}
	if p.current.config.Type != EventTypeSubscribe {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: invalid event type \"%s\"", p.current.config.Type))
		return plan.SubscriptionConfiguration{}
	}
	pubsub, ok := p.pubSubBySourceName[p.current.config.SourceName]
	if !ok {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: no pubsub connection exists with source name \"%s\"", p.current.config.SourceName))
		return plan.SubscriptionConfiguration{}
	}
	return plan.SubscriptionConfiguration{
		Input:     fmt.Sprintf(`{"topic":"%s"}`, p.current.topic),
		Variables: p.variables,
		DataSource: &SubscriptionSource{
			pubSub: pubsub,
		},
		PostProcessing: resolve.PostProcessingConfiguration{
			MergePath: []string{p.current.config.FieldName},
		},
	}
}

func (p *Planner[T]) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
		IncludeTypeNameFields:      true,
	}
}

func (p *Planner[T]) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	return "", false
}

func (p *Planner[T]) UpstreamSchema(dataSourceConfig plan.DataSourceConfiguration[T]) (*ast.Document, bool) {
	return nil, false
}

type Connector interface {
	New(ctx context.Context) PubSub
}

func NewFactory[T Configuration](executionContext context.Context, pubSubBySourceName map[string]PubSub) *Factory[T] {
	return &Factory[T]{
		executionContext:   executionContext,
		PubSubBySourceName: pubSubBySourceName,
	}
}

type Factory[T Configuration] struct {
	executionContext   context.Context
	PubSubBySourceName map[string]PubSub
}

func (f *Factory[T]) Planner(logger abstractlogger.Logger) plan.DataSourcePlanner[T] {
	return &Planner[T]{
		pubSubBySourceName: f.PubSubBySourceName,
	}
}

func (f *Factory[T]) Context() context.Context {
	return f.executionContext
}

// PubSub describe the interface that implements the primitive operations for pubsub
type PubSub interface {
	// ID is the unique identifier of the pubsub implementation (e.g. NATS)
	// This is used to uniquely identify a subscription
	ID() string
	// Subscribe starts listening on the given topic and sends the received messages to the given next channel
	Subscribe(ctx context.Context, topic string, updater resolve.SubscriptionUpdater) error
	// Publish sends the given data to the given topic
	Publish(ctx context.Context, topic string, data []byte) error
	// Request sends a request on the given topic and writes the response to the given writer
	Request(ctx context.Context, topic string, data []byte, w io.Writer) error
}

type SubscriptionSource struct {
	pubSub PubSub
}

func (s *SubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) error {
	topic, err := jsonparser.GetString(input, "topic")
	if err != nil {
		return err
	}
	_, err = xxh.WriteString(topic)
	if err != nil {
		return err
	}
	_, err = xxh.WriteString(s.pubSub.ID())
	return err
}

func (s *SubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	topic, err := jsonparser.GetString(input, "topic")
	if err != nil {
		return err
	}

	return s.pubSub.Subscribe(ctx.Context(), topic, updater)
}

type PublishDataSource struct {
	pubSub PubSub
}

func (s *PublishDataSource) Load(ctx context.Context, input []byte, w io.Writer) error {
	topic, err := jsonparser.GetString(input, "topic")
	if err != nil {
		return fmt.Errorf("error getting topic from input: %w", err)
	}

	data, _, _, err := jsonparser.Get(input, "data")
	if err != nil {
		return fmt.Errorf("error getting data from input: %w", err)
	}

	if err := s.pubSub.Publish(ctx, topic, data); err != nil {
		return err
	}
	_, err = io.WriteString(w, `{"success": true}`)
	return err
}

type RequestDataSource struct {
	pubSub PubSub
}

func (s *RequestDataSource) Load(ctx context.Context, input []byte, w io.Writer) error {
	topic, err := jsonparser.GetString(input, "topic")
	if err != nil {
		return err
	}

	return s.pubSub.Request(ctx, topic, nil, w)
}
