package execution

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
	"sync"
	"time"
)

type MQTTDataSourcePlanner struct {
	BaseDataSourcePlanner
}

func NewMQTTDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *MQTTDataSourcePlanner {
	return &MQTTDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

func (n *MQTTDataSourcePlanner) DirectiveName() []byte {
	return []byte("MQTTDataSource")
}

func (n *MQTTDataSourcePlanner) DirectiveDefinition() []byte {
	data, _ := n.graphqlDefinitions.Find("directives/mqtt_datasource.graphql")
	return data
}

func (n *MQTTDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &MQTTDataSource{
		log: n.log,
	}, n.args
}

func (n *MQTTDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	n.walker, n.operation, n.definition, n.args = walker, operation, definition, args
}

func (n *MQTTDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (n *MQTTDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (n *MQTTDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (n *MQTTDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (n *MQTTDataSourcePlanner) EnterField(ref int) {
	n.rootField.setIfNotDefined(ref)
}

func (n *MQTTDataSourcePlanner) LeaveField(ref int) {
	if !n.rootField.isDefinedAndEquals(ref) {
		return
	}
	definition, exists := n.walker.FieldDefinition(ref)
	if !exists {
		return
	}
	directive, exists := n.definition.FieldDefinitionDirectiveByName(definition, n.DirectiveName())
	if !exists {
		return
	}
	value, exists := n.definition.DirectiveArgumentValueByName(directive, literal.BROKERADDR)
	if !exists {
		return
	}
	variableValue := n.definition.StringValueContentBytes(value.Ref)
	arg := &StaticVariableArgument{
		Name:  literal.BROKERADDR,
		Value: make([]byte, len(variableValue)),
	}
	copy(arg.Value, variableValue)
	n.args = append(n.args, arg)

	value, exists = n.definition.DirectiveArgumentValueByName(directive, literal.CLIENTID)
	if !exists {
		return
	}
	variableValue = n.definition.StringValueContentBytes(value.Ref)
	arg = &StaticVariableArgument{
		Name:  literal.CLIENTID,
		Value: make([]byte, len(variableValue)),
	}
	copy(arg.Value, variableValue)
	n.args = append(n.args, arg)

	value, exists = n.definition.DirectiveArgumentValueByName(directive, literal.TOPIC)
	if !exists {
		return
	}
	variableValue = n.definition.StringValueContentBytes(value.Ref)
	arg = &StaticVariableArgument{
		Name:  literal.TOPIC,
		Value: make([]byte, len(variableValue)),
	}
	copy(arg.Value, variableValue)
	n.args = append(n.args, arg)
}

type MQTTDataSource struct {
	log    log.Logger
	once   sync.Once
	ch     chan mqtt.Message
	client mqtt.Client
}

func (m *MQTTDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) (ins Instruction) {

	defer func() {
		if ins != CloseConnection {
			return
		}
		m.log.Debug("MQTTDataSource.Resolve.client.Disconnect")
		m.client.Disconnect(250)
		m.log.Debug("MQTTDataSource.Resolve.client.Disconnect.disconnected")
	}()

	m.once.Do(func() {

		brokerArg := args.ByKey(literal.BROKERADDR)
		clientIDArg := args.ByKey(literal.CLIENTID)
		topicArg := args.ByKey(literal.TOPIC)

		m.log.Debug("MQTTDataSource.Resolve.init",
			log.String("broker", string(brokerArg)),
			log.String("clientID", string(clientIDArg)),
			log.String("topic", string(topicArg)),
		)

		m.ch = make(chan mqtt.Message)
		m.start(string(brokerArg), string(clientIDArg), string(topicArg))
	})

	select {
	case <-ctx.Done():
		return CloseConnection
	case msg, ok := <-m.ch:
		if !ok {
			return CloseConnection
		}
		_, err := out.Write(msg.Payload())
		if err != nil {
			return CloseConnection
		}
		return KeepStreamAlive
	}
}

func (m *MQTTDataSource) start(brokerAddr, clientID, topic string) {
	mqtt.ERROR = m.log.LevelLogger(log.ErrorLevel)
	mqtt.DEBUG = m.log.LevelLogger(log.DebugLevel)
	opts := mqtt.NewClientOptions().AddBroker(brokerAddr).SetClientID(clientID)
	opts.SetKeepAlive(5 * time.Second)
	opts.SetResumeSubs(true)
	opts.SetAutoReconnect(true)
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		m.ch <- msg
		msg.Ack()
	})
	opts.SetPingTimeout(5 * time.Second)

	m.client = mqtt.NewClient(opts)
	if token := m.client.Connect(); token.Wait() && token.Error() != nil {
		m.log.Error("MQTTDataSource.start.Connect",
			log.Error(token.Error()),
		)
		close(m.ch)
		return
	}

	if token := m.client.Subscribe(topic, 0, nil); token.Wait() && token.Error() != nil {
		m.log.Error("MQTTDataSource.start.Subscribe",
			log.Error(token.Error()),
		)
		close(m.ch)
		return
	}
}
