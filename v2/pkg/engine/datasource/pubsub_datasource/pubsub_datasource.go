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

var eventSubjectRegex = regexp.MustCompile(`{{ args.([a-zA-Z0-9_]+) }}`)

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
	Subjects   []string  `json:"subjects"`
	Type       EventType `json:"type"`
	TypeName   string    `json:"typeName"`
}

type Configuration struct {
	Events []EventConfiguration `json:"events"`
}

type PublishAndRequestEventConfiguration struct {
	subject string
	data    []byte
	config  *EventConfiguration
}

func (p *PublishAndRequestEventConfiguration) reset() {
	p.config = nil
	p.data = nil
	p.subject = ""
}

type SubscriptionEventConfiguration struct {
	subjects []string
	config   *EventConfiguration
}

func (s *SubscriptionEventConfiguration) reset() {
	s.config = nil
	s.subjects = nil
}

type Planner[T Configuration] struct {
	config                              Configuration
	publishAndRequestEventConfiguration PublishAndRequestEventConfiguration
	subscriptionEventConfiguration      SubscriptionEventConfiguration
	pubSubBySourceName                  map[string]PubSub
	rootFieldRef                        int
	variables                           resolve.Variables
	visitor                             *plan.Visitor
}

func (p *Planner[T]) EnterField(ref int) {
	if p.rootFieldRef != -1 {
		// This is a nested field; nothing needs to be done
		return
	}
	p.rootFieldRef = ref

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

	switch eventConfig.Type {
	case EventTypePublish:
		fallthrough
	case EventTypeRequest:
		p.handlePublishAndRequestEvent(ref, eventConfig)
	case EventTypeSubscribe:
		p.handleSubscriptionEvent(ref, eventConfig)
	default:
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("invalid EventType \"%s\"", eventConfig.Type))
	}
}

func (p *Planner[T]) EnterDocument(_, _ *ast.Document) {
	p.rootFieldRef = -1
	p.publishAndRequestEventConfiguration.reset()
	p.subscriptionEventConfiguration.reset()
}

func (p *Planner[T]) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration[T], dataSourcePlannerConfiguration plan.DataSourcePlannerConfiguration) error {
	p.visitor = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	visitor.Walker.RegisterEnterDocumentVisitor(p)
	p.config = Configuration(configuration.CustomConfiguration())
	return nil
}

func (p *Planner[T]) ConfigureFetch() resolve.FetchConfiguration {
	if p.publishAndRequestEventConfiguration.config == nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: event config is nil"))
		return resolve.FetchConfiguration{}
	}
	var dataSource resolve.DataSource
	pubsub, ok := p.pubSubBySourceName[p.publishAndRequestEventConfiguration.config.SourceName]
	if !ok {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("no pubsub connection exists with source name \"%s\"", p.publishAndRequestEventConfiguration.config.SourceName))
		return resolve.FetchConfiguration{}
	}
	switch p.publishAndRequestEventConfiguration.config.Type {
	case EventTypePublish:
		dataSource = &PublishDataSource{
			pubSub: pubsub,
		}
	case EventTypeRequest:
		dataSource = &RequestDataSource{
			pubSub: pubsub,
		}
	default:
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: invalid event type \"%s\"", p.publishAndRequestEventConfiguration.config.Type))
		return resolve.FetchConfiguration{}
	}
	return resolve.FetchConfiguration{
		Input:      fmt.Sprintf(`{"subject":"%s", "data": %s, "sourceName":"%s"}`, p.publishAndRequestEventConfiguration.subject, p.publishAndRequestEventConfiguration.data, p.publishAndRequestEventConfiguration.config.SourceName),
		Variables:  p.variables,
		DataSource: dataSource,
		PostProcessing: resolve.PostProcessingConfiguration{
			MergePath: []string{p.publishAndRequestEventConfiguration.config.FieldName},
		},
	}
}

func (p *Planner[T]) ConfigureSubscription() plan.SubscriptionConfiguration {
	if p.subscriptionEventConfiguration.config == nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: event config is nil"))
		return plan.SubscriptionConfiguration{}
	}
	if p.subscriptionEventConfiguration.config.Type != EventTypeSubscribe {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: invalid event type \"%s\"", p.subscriptionEventConfiguration.config.Type))
		return plan.SubscriptionConfiguration{}
	}
	pubsub, ok := p.pubSubBySourceName[p.subscriptionEventConfiguration.config.SourceName]
	if !ok {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: no pubsub connection exists with source name \"%s\"", p.subscriptionEventConfiguration.config.SourceName))
		return plan.SubscriptionConfiguration{}
	}
	jsonArray, err := json.Marshal(p.subscriptionEventConfiguration.subjects)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to marshal event subscription subjects"))
		return plan.SubscriptionConfiguration{}
	}
	return plan.SubscriptionConfiguration{
		Input:     fmt.Sprintf(`{"subjects":%s, "sourceName":"%s"}`, jsonArray, p.subscriptionEventConfiguration.config.SourceName),
		Variables: p.variables,
		DataSource: &SubscriptionSource{
			pubSub: pubsub,
		},
		PostProcessing: resolve.PostProcessingConfiguration{
			MergePath: []string{p.subscriptionEventConfiguration.config.FieldName},
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

func (p *Planner[T]) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	return "", false
}

func (p *Planner[T]) UpstreamSchema(_ plan.DataSourceConfiguration[T]) (*ast.Document, bool) {
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

func (f *Factory[T]) Planner(_ abstractlogger.Logger) plan.DataSourcePlanner[T] {
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
	// Subscribe starts listening on the given subjects and sends the received messages to the given next channel
	Subscribe(ctx context.Context, subjects []string, updater resolve.SubscriptionUpdater) error
	// Publish sends the given data to the given subject
	Publish(ctx context.Context, subject string, data []byte) error
	// Request sends a request on the given subject and writes the response to the given writer
	Request(ctx context.Context, subject string, data []byte, w io.Writer) error
}

type SubscriptionSource struct {
	pubSub PubSub
}

func (s *SubscriptionSource) UniqueRequestID(ctx *resolve.Context, input []byte, xxh *xxhash.Digest) error {
	// input must be unique across datasources
	_, err := xxh.Write(input)
	return err
}

func (s *SubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	var subjects []string
	_, err := jsonparser.ArrayEach(input, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		subjects = append(subjects, string(value))
	}, "subjects")
	if err != nil {
		return err
	}

	return s.pubSub.Subscribe(ctx.Context(), subjects, updater)
}

type PublishDataSource struct {
	pubSub PubSub
}

func (s *PublishDataSource) Load(ctx context.Context, input []byte, w io.Writer) error {
	subject, err := jsonparser.GetString(input, "subject")
	if err != nil {
		return fmt.Errorf("error getting subject from input: %w", err)
	}

	data, _, _, err := jsonparser.Get(input, "data")
	if err != nil {
		return fmt.Errorf("error getting data from input: %w", err)
	}

	if err := s.pubSub.Publish(ctx, subject, data); err != nil {
		return err
	}
	_, err = io.WriteString(w, `{"success": true}`)
	return err
}

type RequestDataSource struct {
	pubSub PubSub
}

func (s *RequestDataSource) Load(ctx context.Context, input []byte, w io.Writer) error {
	subject, err := jsonparser.GetString(input, "subject")
	if err != nil {
		return err
	}

	return s.pubSub.Request(ctx, subject, nil, w)
}

func (p *Planner[T]) extractEventSubject(ref int, subject string) (string, error) {
	matches := eventSubjectRegex.FindAllStringSubmatch(subject, -1)
	if len(matches) != 1 || len(matches[0]) != 2 {
		return "", fmt.Errorf("expected subject to match regex")
	}
	argumentName := matches[0][1]
	// We need to find the argument in the operation
	argumentRef, ok := p.visitor.Operation.FieldArgument(ref, []byte(argumentName))
	if !ok {
		return "", fmt.Errorf("argument \"%s\" is not defined", argumentName)
	}
	argumentValue := p.visitor.Operation.ArgumentValue(argumentRef)
	if argumentValue.Kind != ast.ValueKindVariable {
		return "", fmt.Errorf("expected argument \"%s\" kind to be \"ValueKindVariable\" but received \"%s\"", argumentName, argumentValue.Kind)
	}
	variableName := p.visitor.Operation.VariableValueNameBytes(argumentValue.Ref)
	variableDefinition, ok := p.visitor.Operation.VariableDefinitionByNameAndOperation(p.visitor.Walker.Ancestors[0].Ref, variableName)
	if !ok {
		return "", fmt.Errorf("expected definition to exist for variable \"%s\"", variableName)
	}
	variableTypeRef := p.visitor.Operation.VariableDefinitions[variableDefinition].Type
	renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(p.visitor.Operation, p.visitor.Operation, variableTypeRef, string(variableName))
	if err != nil {
		return "", err
	}
	contextVariable := &resolve.ContextVariable{
		Path:     []string{string(variableName)},
		Renderer: renderer,
	}
	variablePlaceHolder, exists := p.variables.AddVariable(contextVariable) // $$0$$
	if exists {
		return "", fmt.Errorf("context variable \"%s\" already exists", variableName)
	}
	// We need to replace the template literal with the variable placeholder
	return eventSubjectRegex.ReplaceAllLiteralString(subject, variablePlaceHolder), nil
}

func (p *Planner[T]) eventDataBytes(ref int) ([]byte, error) {
	// Collect the field arguments for fetch based operations
	fieldArgs := p.visitor.Operation.FieldArguments(ref)
	var dataBuffer bytes.Buffer
	dataBuffer.WriteByte('{')
	for i, arg := range fieldArgs {
		if i > 0 {
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
			return nil, err
		}
		dataBuffer.Write(escapedKey)
		dataBuffer.WriteByte(':')
		dataBuffer.WriteString(variablePlaceHolder)
	}
	dataBuffer.WriteByte('}')
	return dataBuffer.Bytes(), nil
}

func (p *Planner[T]) handlePublishAndRequestEvent(ref int, eventConfiguration *EventConfiguration) {
	if len(eventConfiguration.Subjects) != 1 {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("publish and request events should define one subject but received %d", len(eventConfiguration.Subjects)))
		return
	}
	rawSubject := eventConfiguration.Subjects[0]
	extractedSubject, err := p.extractEventSubject(ref, rawSubject)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("could not extract event subject: %w", err))
		return
	}
	dataBytes, err := p.eventDataBytes(ref)
	if err != nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to write event data bytes: %w", err))
		return
	}
	p.publishAndRequestEventConfiguration.config = eventConfiguration
	p.publishAndRequestEventConfiguration.data = dataBytes
	p.publishAndRequestEventConfiguration.subject = extractedSubject
}

func (p *Planner[T]) handleSubscriptionEvent(ref int, eventConfiguration *EventConfiguration) {
	subjectsLength := len(eventConfiguration.Subjects)
	if subjectsLength < 1 {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("expected at least one subscription subject but received %d", subjectsLength))
		return
	}
	extractedSubjects := make([]string, 0, subjectsLength)
	for _, rawSubject := range eventConfiguration.Subjects {
		extractedSubject, err := p.extractEventSubject(ref, rawSubject)
		if err != nil {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("could not extract subscriptionevent subjects: %w", err))
			return
		}
		extractedSubjects = append(extractedSubjects, extractedSubject)
	}
	p.subscriptionEventConfiguration.subjects = extractedSubjects
	p.subscriptionEventConfiguration.config = eventConfiguration
}
