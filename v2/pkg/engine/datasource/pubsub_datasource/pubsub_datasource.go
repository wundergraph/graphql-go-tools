package pubsub_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	natsserver "github.com/nats-io/nats-server/v2/server"
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

// argument template has form {{args.path}} with flexible whitespace surrounding args.path
var argumentTemplateRegex = regexp.MustCompile(`{{\s*args((?:\.[a-zA-Z0-9_]+)+)\s*}}`)

// variable template has form $$number$$ where the number can range from one to multiple digits
var variableTemplateRegex = regexp.MustCompile(`\$\$\d+\$\$`)

func EventTypeFromString(s string) (EventType, error) {
	et := EventType(strings.ToLower(s))
	switch et {
	case EventTypePublish, EventTypeRequest, EventTypeSubscribe:
		return et, nil
	default:
		return "", fmt.Errorf("invalid event type: %q", s)
	}
}

type StreamConfiguration struct {
	Consumer   string
	StreamName string
}

type EventConfiguration struct {
	FieldName           string               `json:"fieldName"`
	SourceName          string               `json:"sourceName"`
	StreamConfiguration *StreamConfiguration `json:"streamConfiguration"`
	Subjects            []string             `json:"subjects"`
	Type                EventType            `json:"type"`
	TypeName            string               `json:"typeName"`
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
	case EventTypePublish, EventTypeRequest:
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
	var streamConfiguration string
	if p.subscriptionEventConfiguration.config.StreamConfiguration != nil {
		object, err := json.Marshal(p.subscriptionEventConfiguration.config.StreamConfiguration)
		if err != nil {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to marshal event subscription streamConfiguration"))
			return plan.SubscriptionConfiguration{}
		}
		streamConfiguration = fmt.Sprintf(", \"streamConfiguration\":%s", object)
	}
	return plan.SubscriptionConfiguration{
		Input:     fmt.Sprintf(`{"subjects":%s, "sourceName":"%s"%s}`, jsonArray, p.subscriptionEventConfiguration.config.SourceName, streamConfiguration),
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
	Subscribe(ctx context.Context, subjects []string, updater resolve.SubscriptionUpdater, streamConfiguration *StreamConfiguration) error
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

type SubscriptionSourceInput struct {
	Subjects            []string             `json:"subjects"`
	SourceName          string               `json:"sourceName"`
	StreamConfiguration *StreamConfiguration `json:"streamConfiguration"`
}

func (s *SubscriptionSource) Start(ctx *resolve.Context, input []byte, updater resolve.SubscriptionUpdater) error {
	var subscriptionSourceInput SubscriptionSourceInput
	if err := json.Unmarshal(input, &subscriptionSourceInput); err != nil {
		return err
	}

	return s.pubSub.Subscribe(ctx.Context(), subscriptionSourceInput.Subjects, updater, subscriptionSourceInput.StreamConfiguration)
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

func (p *Planner[T]) inputObjectDefinitionRefByFieldDefinitionRef(fieldDefinitionRef int, argumentNameBytes []byte) (inputObjectDefinitionRef int, inputValueTypeRef int) {
	inputValueTypeRef = -1
	for _, inputValueDefinitionRef := range p.visitor.Definition.FieldDefinitions[fieldDefinitionRef].ArgumentsDefinition.Refs {
		if !p.visitor.Definition.InputValueDefinitionNameBytes(inputValueDefinitionRef).Equals(argumentNameBytes) {
			continue
		}
		inputObjectDefinitionRef, inputValueTypeRef = p.inputObjectDefinitionRefByInputValueDefinitionRef(inputValueDefinitionRef)
		if inputObjectDefinitionRef != -1 {
			return inputObjectDefinitionRef, inputValueTypeRef
		}
	}
	return -1, inputValueTypeRef
}
func (p *Planner[T]) inputObjectDefinitionRefByInputValueDefinitionRef(inputValueDefinitionRef int) (inputObjectDefinitionRef int, inputValueTypeRef int) {
	inputValueTypeRef = p.visitor.Definition.InputValueDefinitions[inputValueDefinitionRef].Type
	typeNameBytes := p.visitor.Definition.ResolveTypeNameBytes(inputValueTypeRef)
	node, ok := p.visitor.Definition.Index.FirstNodeByNameBytes(typeNameBytes)
	if !ok {
		return -1, inputValueTypeRef
	}
	if node.Kind != ast.NodeKindInputObjectTypeDefinition {
		return -1, inputValueTypeRef
	}
	return node.Ref, inputValueTypeRef
}

func (p *Planner[T]) extractEventSubject(fieldRef int, subject string) (string, error) {
	allMatches := argumentTemplateRegex.FindAllStringSubmatch(subject, -1)
	// if no argument templates are defined, there are only static values
	if len(allMatches) < 1 {
		if natsserver.IsValidSubject(subject) {
			return subject, nil
		}
		return "", fmt.Errorf(`subject "%s" is not a valid NATS subject`, subject)
	}
	substitutedSubject := subject
	for templateNumber, matches := range allMatches {
		if len(matches) != 2 {
			return "", fmt.Errorf("expected a single matching group for argument template")
		}
		// the path begins with ".", so ignore the first empty string element
		argumentPath := strings.Split(matches[1][1:], ".")
		fieldNameBytes := p.visitor.Operation.FieldNameBytes(fieldRef)
		fieldDefinitionRef, ok := p.visitor.Definition.ObjectTypeDefinitionFieldWithName(p.visitor.Walker.EnclosingTypeDefinition.Ref, fieldNameBytes)
		if !ok {
			return "", fmt.Errorf(`expected field definition to exist for field "%s"`, fieldNameBytes)
		}
		inputObjectDefinitionRef, lastInputValueTypeRef := p.inputObjectDefinitionRefByFieldDefinitionRef(fieldDefinitionRef, []byte(argumentPath[0]))
		for _, path := range argumentPath[1:] {
			inputValueNameBytes := []byte(path)
			if inputObjectDefinitionRef < 0 {
				return "", fmt.Errorf(`the event subject "%s" defines nested input field "%s" in argument template #%d, but no parent input object was found`, subject, inputValueNameBytes, templateNumber+1)
			}
			inputObjectDefinition := p.visitor.Definition.InputObjectTypeDefinitions[inputObjectDefinitionRef]
			inputObjectDefinitionRef = -1
			lastInputValueTypeRef = -1
			for _, inputValueDefinitionRef := range inputObjectDefinition.InputFieldsDefinition.Refs {
				if !p.visitor.Definition.InputValueDefinitionNameBytes(inputValueDefinitionRef).Equals(inputValueNameBytes) {
					continue
				}
				inputObjectDefinitionRef, lastInputValueTypeRef = p.inputObjectDefinitionRefByInputValueDefinitionRef(inputValueDefinitionRef)
				break
			}
		}
		if inputObjectDefinitionRef != -1 {
			return "", fmt.Errorf(`the event subject "%s" defines the final nested input field "%s" in argument template #%d, but it is not a leaf type`, subject, argumentPath[len(argumentPath)-1], templateNumber+1)
		}
		if lastInputValueTypeRef == -1 {
			return "", fmt.Errorf(`the event subject "%s" defines the final nested input field "%s" in argument template #%d, but it does not exist`, subject, argumentPath[len(argumentPath)-1], templateNumber+1)
		}
		argumentName := argumentPath[0]
		argumentRef, ok := p.visitor.Operation.FieldArgument(fieldRef, []byte(argumentName))
		if !ok {
			return "", fmt.Errorf(`argument "%s" is not defined`, argumentName)
		}
		p.visitor.Definition.ObjectTypeDefinitionFieldWithName(p.visitor.Walker.EnclosingTypeDefinition.Ref, p.visitor.Operation.FieldNameBytes(fieldRef))
		argumentValue := p.visitor.Operation.ArgumentValue(argumentRef)
		if argumentValue.Kind != ast.ValueKindVariable {
			return "", fmt.Errorf(`expected argument "%s" kind to be "ValueKindVariable" but received "%s"`, argumentName, argumentValue.Kind)
		}
		variableName := p.visitor.Operation.VariableValueNameBytes(argumentValue.Ref)
		_, ok = p.visitor.Operation.VariableDefinitionByNameAndOperation(p.visitor.Walker.Ancestors[0].Ref, variableName)
		if !ok {
			return "", fmt.Errorf(`expected definition to exist for variable "%s"`, variableName)
		}
		// the variable path should be the variable name, e.g., "a", and then the 2nd element from the path onwards
		variablePath := append([]string{string(variableName)}, argumentPath[1:]...)

		// the definition is passed as operation because getJSONRootType resolves the type from the first argument,
		// but lastInputValueTypeRef comes from the definition
		renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(p.visitor.Definition, p.visitor.Definition, lastInputValueTypeRef, variablePath...)
		if err != nil {
			return "", err
		}
		contextVariable := &resolve.ContextVariable{
			Path:     variablePath,
			Renderer: renderer,
		}
		// We need to replace the template literal with the variable placeholder (and reuse if it already exists)
		variablePlaceHolder, _ := p.variables.AddVariable(contextVariable) // $$0$$
		substitutedSubject = strings.ReplaceAll(substitutedSubject, matches[0], variablePlaceHolder)
	}
	// substitute the variable templates for dummy values to check that the string is a valid NATS subject
	if natsserver.IsValidSubject(variableTemplateRegex.ReplaceAllLiteralString(substitutedSubject, "a")) {
		return substitutedSubject, nil
	}
	return "", fmt.Errorf(`subject "%s" is not a valid NATS subject`, subject)
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
		variableName := p.visitor.Operation.VariableValueNameBytes(argValue.Ref)
		variableDefinition, ok := p.visitor.Operation.VariableDefinitionByNameAndOperation(p.visitor.Walker.Ancestors[0].Ref, variableName)
		if !ok {
			return nil, fmt.Errorf("expected definition to exist for variable \"%s\"", variableName)
		}
		variableTypeRef := p.visitor.Operation.VariableDefinitions[variableDefinition].Type
		renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(p.visitor.Operation, p.visitor.Definition, variableTypeRef, string(variableName))
		if err != nil {
			return nil, err
		}
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
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("could not extract subscription event subjects: %w", err))
			return
		}
		extractedSubjects = append(extractedSubjects, extractedSubject)
	}
	p.subscriptionEventConfiguration.subjects = extractedSubjects
	p.subscriptionEventConfiguration.config = eventConfiguration
}
