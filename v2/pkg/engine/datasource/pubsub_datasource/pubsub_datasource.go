package pubsub_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

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

type EventMetadata struct {
	ProviderID string    `json:"providerId"`
	Type       EventType `json:"type"`
	TypeName   string    `json:"typeName"`
	FieldName  string    `json:"fieldName"`
}

type EventConfiguration struct {
	Metadata      *EventMetadata `json:"metadata"`
	Configuration any            `json:"configuration"`
}

type Configuration struct {
	Events []EventConfiguration `json:"events"`
}

type Planner[T Configuration] struct {
	config                  Configuration
	natsPubSubByProviderID  map[string]NatsPubSub
	kafkaPubSubByProviderID map[string]KafkaPubSub
	eventManager            any
	rootFieldRef            int
	variables               resolve.Variables
	visitor                 *plan.Visitor
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
		if cfg.Metadata.TypeName == typeName && cfg.Metadata.FieldName == fieldName {
			eventConfig = &cfg
			break
		}
	}
	if eventConfig == nil {
		return
	}

	switch v := eventConfig.Configuration.(type) {
	case *NatsEventConfiguration:
		em := &NatsEventManager{
			visitor:            p.visitor,
			variables:          &p.variables,
			eventMetadata:      *eventConfig.Metadata,
			eventConfiguration: v,
		}
		p.eventManager = em

		switch eventConfig.Metadata.Type {
		case EventTypePublish, EventTypeRequest:
			em.handlePublishAndRequestEvent(ref)
		case EventTypeSubscribe:
			em.handleSubscriptionEvent(ref)
		default:
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("invalid EventType \"%s\"", eventConfig.Metadata.Type))
		}
	case *KafkaEventConfiguration:
		em := &KafkaEventManager{
			visitor:            p.visitor,
			variables:          &p.variables,
			eventMetadata:      *eventConfig.Metadata,
			eventConfiguration: v,
		}
		p.eventManager = em

		switch eventConfig.Metadata.Type {
		case EventTypePublish:
			em.handlePublishEvent(ref)
		case EventTypeSubscribe:
			em.handleSubscriptionEvent(ref)
		default:
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("invalid EventType \"%s\"", eventConfig.Metadata.Type))
		}
	default:
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("invalid event configuration type"))
	}
}

func (p *Planner[T]) EnterDocument(_, _ *ast.Document) {
	p.rootFieldRef = -1
	p.eventManager = nil
}

func (p *Planner[T]) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration[T], dataSourcePlannerConfiguration plan.DataSourcePlannerConfiguration) error {
	p.visitor = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	visitor.Walker.RegisterEnterDocumentVisitor(p)
	p.config = Configuration(configuration.CustomConfiguration())
	return nil
}

func (p *Planner[T]) ConfigureFetch() resolve.FetchConfiguration {
	if p.eventManager == nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: event manager is nil"))
		return resolve.FetchConfiguration{}
	}

	var dataSource resolve.DataSource

	switch v := p.eventManager.(type) {
	case *NatsEventManager:
		pubsub, ok := p.natsPubSubByProviderID[v.eventMetadata.ProviderID]
		if !ok {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("no pubsub connection exists with source id \"%s\"", v.eventMetadata.ProviderID))
			return resolve.FetchConfiguration{}
		}

		switch v.eventMetadata.Type {
		case EventTypePublish:
			dataSource = &NatsPublishDataSource{
				pubSub: pubsub,
			}
		case EventTypeRequest:
			dataSource = &NatsRequestDataSource{
				pubSub: pubsub,
			}
		default:
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: invalid event type \"%s\" for Nats", v.eventMetadata.Type))
			return resolve.FetchConfiguration{}
		}

		return resolve.FetchConfiguration{
			Input:      v.publishAndRequestEventConfiguration.MarshalJSONTemplate(),
			Variables:  p.variables,
			DataSource: dataSource,
			PostProcessing: resolve.PostProcessingConfiguration{
				MergePath: []string{v.eventMetadata.FieldName},
			},
		}

	case *KafkaEventManager:
		pubsub, ok := p.natsPubSubByProviderID[v.eventMetadata.ProviderID]
		if !ok {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("no pubsub connection exists with source id \"%s\"", v.eventMetadata.ProviderID))
			return resolve.FetchConfiguration{}
		}

		switch v.eventMetadata.Type {
		case EventTypePublish:
			dataSource = &NatsPublishDataSource{
				pubSub: pubsub,
			}
		case EventTypeRequest:
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("event type \"%s\" is not supported for Kafka", v.eventMetadata.Type))
			return resolve.FetchConfiguration{}
		default:
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: invalid event type \"%s\" for Kafka", v.eventMetadata.Type))
			return resolve.FetchConfiguration{}
		}

		return resolve.FetchConfiguration{
			Input:      v.publishEventConfiguration.MarshalJSONTemplate(),
			Variables:  p.variables,
			DataSource: dataSource,
			PostProcessing: resolve.PostProcessingConfiguration{
				MergePath: []string{v.eventMetadata.FieldName},
			},
		}

	default:
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure fetch: invalid event manager type: %T", p.eventManager))
	}

	return resolve.FetchConfiguration{}
}

func (p *Planner[T]) ConfigureSubscription() plan.SubscriptionConfiguration {
	if p.eventManager == nil {
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure subscription: event manager is nil"))
		return plan.SubscriptionConfiguration{}
	}

	switch v := p.eventManager.(type) {
	case *NatsEventManager:
		pubsub, ok := p.natsPubSubByProviderID[v.eventMetadata.ProviderID]
		if !ok {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("no pubsub connection exists with source id \"%s\"", v.eventMetadata.ProviderID))
			return plan.SubscriptionConfiguration{}
		}
		object, err := json.Marshal(v.subscriptionEventConfiguration)
		if err != nil {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to marshal event subscription streamConfiguration"))
			return plan.SubscriptionConfiguration{}
		}
		return plan.SubscriptionConfiguration{
			Input:     string(object),
			Variables: p.variables,
			DataSource: &NatsSubscriptionSource{
				pubSub: pubsub,
			},
			PostProcessing: resolve.PostProcessingConfiguration{
				MergePath: []string{v.eventMetadata.FieldName},
			},
		}
	case *KafkaEventManager:
		pubsub, ok := p.natsPubSubByProviderID[v.eventMetadata.ProviderID]
		if !ok {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("no pubsub connection exists with source id \"%s\"", v.eventMetadata.ProviderID))
			return plan.SubscriptionConfiguration{}
		}
		object, err := json.Marshal(v.subscriptionEventConfiguration)
		if err != nil {
			p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to marshal event subscription streamConfiguration"))
			return plan.SubscriptionConfiguration{}
		}
		return plan.SubscriptionConfiguration{
			Input:     string(object),
			Variables: p.variables,
			DataSource: &NatsSubscriptionSource{
				pubSub: pubsub,
			},
			PostProcessing: resolve.PostProcessingConfiguration{
				MergePath: []string{v.eventMetadata.FieldName},
			},
		}
	default:
		p.visitor.Walker.StopWithInternalErr(fmt.Errorf("failed to configure subscription: invalid event manager type: %T", p.eventManager))
	}

	return plan.SubscriptionConfiguration{}
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

func NewFactory[T Configuration](executionContext context.Context, natsPubSubBySourceName map[string]NatsPubSub, kafkaPubSubBySourceName map[string]KafkaPubSub) *Factory[T] {
	return &Factory[T]{
		executionContext:        executionContext,
		natsPubSubBySourceName:  natsPubSubBySourceName,
		kafkaPubSubBySourceName: kafkaPubSubBySourceName,
	}
}

type Factory[T Configuration] struct {
	executionContext        context.Context
	natsPubSubBySourceName  map[string]NatsPubSub
	kafkaPubSubBySourceName map[string]KafkaPubSub
}

func (f *Factory[T]) Planner(_ abstractlogger.Logger) plan.DataSourcePlanner[T] {
	return &Planner[T]{
		natsPubSubByProviderID:  f.natsPubSubBySourceName,
		kafkaPubSubByProviderID: f.kafkaPubSubBySourceName,
	}
}

func (f *Factory[T]) Context() context.Context {
	return f.executionContext
}

func buildEventDataBytes(ref int, visitor *plan.Visitor, variables *resolve.Variables) ([]byte, error) {
	// Collect the field arguments for fetch based operations
	fieldArgs := visitor.Operation.FieldArguments(ref)
	var dataBuffer bytes.Buffer
	dataBuffer.WriteByte('{')
	for i, arg := range fieldArgs {
		if i > 0 {
			dataBuffer.WriteByte(',')
		}
		argValue := visitor.Operation.ArgumentValue(arg)
		variableName := visitor.Operation.VariableValueNameBytes(argValue.Ref)
		variableDefinition, ok := visitor.Operation.VariableDefinitionByNameAndOperation(visitor.Walker.Ancestors[0].Ref, variableName)
		if !ok {
			return nil, fmt.Errorf("expected definition to exist for variable \"%s\"", variableName)
		}
		variableTypeRef := visitor.Operation.VariableDefinitions[variableDefinition].Type
		renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(visitor.Operation, visitor.Definition, variableTypeRef, string(variableName))
		if err != nil {
			return nil, err
		}
		contextVariable := &resolve.ContextVariable{
			Path:     []string{string(variableName)},
			Renderer: renderer,
		}
		variablePlaceHolder, _ := variables.AddVariable(contextVariable)
		argumentName := visitor.Operation.ArgumentNameString(arg)
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
