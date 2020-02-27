package execution

import (
	"encoding/json"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
	"sync"
	"time"
)

type MQTTDataSourceConfig struct {
	BrokerAddr string
	ClientID   string
	Topic      string
}

type MQTTDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig MQTTDataSourceConfig
}

func NewMQTTDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *MQTTDataSourcePlanner {
	return &MQTTDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

func (n *MQTTDataSourcePlanner) DataSourceName() string {
	return "MQTTDataSource"
}

func (n *MQTTDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &MQTTDataSource{
		log: n.log,
	}, n.args
}

func (n *MQTTDataSourcePlanner) Initialize(config DataSourcePlannerConfiguration) (err error) {
	n.walker, n.operation, n.definition = config.walker, config.operation, config.definition
	return json.NewDecoder(config.dataSourceConfiguration).Decode(&n.dataSourceConfig)
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
	n.args = append(n.args, &StaticVariableArgument{
		Name:  literal.BROKERADDR,
		Value: []byte(n.dataSourceConfig.BrokerAddr),
	})
	n.args = append(n.args, &StaticVariableArgument{
		Name:  literal.CLIENTID,
		Value: []byte(n.dataSourceConfig.ClientID),
	})
	n.args = append(n.args, &StaticVariableArgument{
		Name:  literal.TOPIC,
		Value: []byte(n.dataSourceConfig.Topic),
	})
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
