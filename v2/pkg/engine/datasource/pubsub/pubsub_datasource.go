package pubsub

import (
	"context"
	"fmt"
	"regexp"

	"github.com/buger/jsonparser"
	"github.com/nats-io/nats.go"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type Planner struct {
	visitor      *plan.Visitor
	variables    resolve.Variables
	rootFieldRef int
	pubSub       PubSub
	topic        string
}

/*
Example Schema:

type Subscription {
	channelUpdates(id: ID!): ChannelUpdate! @pubsub(topic: "channels.{{ args.id }}")
}

type ChannelUpdate @key(fields: "id") {
	id: ID!
	name: String!
	newMessages: [Message!]!
}

type Message {
	id: ID!
	text: String!
}

Example Subscription:

subscription {
	channelUpdates(id: "123") {
		id
		name
		newMessages {
			id
			text
		}
	}
}

Example PubSub Message:

{
	"id": "123",
	"name": "My Channel",
	"newMessages": [
		{
			"id": "456",
			"text": "Hello World"
		}
	]
}
*/

var (
	pubSubDirectiveName              = []byte("pubsub")
	pubSubDirectiveTopicArgumentName = []byte("topic")
)

func (p *Planner) EnterField(ref int) {
	if p.rootFieldRef == -1 {
		p.rootFieldRef = ref
	} else {
		// This is a nested field, we don't need to do anything here
		return
	}
	// This is the root field, we need to check if it has a @pubsub directive
	definition, ok := p.visitor.Walker.FieldDefinition(ref)
	if !ok {
		return
	}
	directive, ok := p.visitor.Definition.FieldDefinitionDirectiveByName(definition, pubSubDirectiveName)
	if !ok {
		return
	}
	// We found a @pubsub directive, extract the topic from it
	topicValue, ok := p.visitor.Definition.DirectiveArgumentValueByName(directive, pubSubDirectiveTopicArgumentName)
	if !ok {
		return
	}
	if topicValue.Kind != ast.ValueKindString {
		return
	}
	topic := p.visitor.Definition.StringValueContentString(topicValue.Ref) // "channels.{{ args.id }}"
	// We need to replace the {{ args.id }} with the actual value
	// so we need to parse the template literal,
	// extract the argument name and add a variable and placeholder to the subscription config

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
	variableName := p.visitor.Operation.ValueContentBytes(argValue)
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
	p.topic = rex.ReplaceAllString(topic, variablePlaceHolder)
}

func (p *Planner) EnterDocument(operation, definition *ast.Document) {
	p.rootFieldRef = -1
	p.topic = ""
}

func (p *Planner) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration, dataSourcePlannerConfiguration plan.DataSourcePlannerConfiguration) error {
	p.visitor = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	visitor.Walker.RegisterEnterDocumentVisitor(p)
	return nil
}

func (p *Planner) ConfigureFetch() resolve.FetchConfiguration {
	return resolve.FetchConfiguration{}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{
		Input:     fmt.Sprintf(`{"topic":"%s"}`, p.topic),
		Variables: p.variables,
		DataSource: &SubscriptionSource{
			pubSub: p.pubSub,
		},
		PostProcessing: resolve.PostProcessingConfiguration{},
	}
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
		IncludeTypeNameFields:      false,
	}
}

func (p *Planner) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	return "", false
}

func (p *Planner) UpstreamSchema(dataSourceConfig plan.DataSourceConfiguration) *ast.Document {
	return nil
}

type Connector interface {
	New(ctx context.Context) PubSub
}

type Factory struct {
	Connector Connector
}

func (f *Factory) Planner(ctx context.Context) plan.DataSourcePlanner {
	return &Planner{
		pubSub: f.Connector.New(ctx),
	}
}

type PubSub interface {
	Subscribe(ctx context.Context, topic string, next chan<- []byte) error
}

type SubscriptionSource struct {
	pubSub PubSub
}

func (s *SubscriptionSource) Start(ctx *resolve.Context, input []byte, next chan<- []byte) error {
	topic, err := jsonparser.GetString(input, "topic")
	if err != nil {
		return err
	}
	return s.pubSub.Subscribe(ctx.Context(), topic, next)
}

type NatsPubSub struct {
	nc *nats.Conn
}

func (n *NatsPubSub) Subscribe(ctx context.Context, topic string, next chan<- []byte) error {
	ch := make(chan *nats.Msg)
	sub, err := n.nc.ChanSubscribe(topic, ch)
	if err != nil {
		return err
	}
	done := ctx.Done()
	go func() {
		for {
			select {
			case <-done:
				_ = sub.Unsubscribe()
				return
			case msg := <-ch:
				next <- msg.Data
			}
		}
	}()
	return nil
}
