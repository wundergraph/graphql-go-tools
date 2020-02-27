package execution

import (
	"encoding/json"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/nats-io/nats.go"
	"io"
	"sync"
	"time"
)

type NatsDataSourceConfig struct {
	Addr  string
	Topic string
}

type NatsDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig NatsDataSourceConfig
}

func NewNatsDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *NatsDataSourcePlanner {
	return &NatsDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

func (n *NatsDataSourcePlanner) DataSourceName() string {
	return "NatsDataSource"
}

func (n *NatsDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &NatsDataSource{
		log: n.log,
	}, n.args
}

func (n *NatsDataSourcePlanner) Initialize(config DataSourcePlannerConfiguration) (err error) {
	n.walker, n.operation, n.definition = config.walker, config.operation, config.definition
	return json.NewDecoder(config.dataSourceConfiguration).Decode(&n.dataSourceConfig)
}

func (n *NatsDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (n *NatsDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (n *NatsDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (n *NatsDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (n *NatsDataSourcePlanner) EnterField(ref int) {
	n.rootField.setIfNotDefined(ref)
}

func (n *NatsDataSourcePlanner) LeaveField(ref int) {
	if !n.rootField.isDefinedAndEquals(ref) {
		return
	}
	n.args = append(n.args, &StaticVariableArgument{
		Name:  literal.ADDR,
		Value: []byte(n.dataSourceConfig.Addr),
	})
	n.args = append(n.args, &StaticVariableArgument{
		Name:  literal.TOPIC,
		Value: []byte(n.dataSourceConfig.Topic),
	})
}

type NatsDataSource struct {
	log  log.Logger
	conn *nats.Conn
	sub  *nats.Subscription
	once sync.Once
}

func (n *NatsDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	var err error
	n.once.Do(func() {

		addrArg := args.ByKey(literal.ADDR)
		topicArg := args.ByKey(literal.TOPIC)

		addr := nats.DefaultURL
		topic := string(topicArg)

		if len(addrArg) != 0 {
			addr = string(addrArg)
		}

		go func() {
			<-ctx.Done()
			if n.sub != nil {
				n.log.Debug("NatsDataSource.unsubscribing",
					log.String("addr", addr),
					log.String("topic", topic),
				)
				err := n.sub.Unsubscribe()
				if err != nil {
					n.log.Error("Unsubscribe", log.Error(err))
				}
			}
			if n.conn != nil {
				n.log.Debug("NatsDataSource.closing",
					log.String("addr", addr),
					log.String("topic", topic),
				)
				n.conn.Close()
			}
		}()

		n.log.Debug("NatsDataSource.connecting",
			log.String("addr", addr),
			log.String("topic", topic),
		)

		n.conn, err = nats.Connect(addr)
		if err != nil {
			panic(err)
		}

		n.log.Debug("NatsDataSource.subscribing",
			log.String("addr", addr),
			log.String("topic", topic),
		)

		n.sub, err = n.conn.SubscribeSync(topic)
		if err != nil {
			panic(err)
		}
	})

	if err != nil {
		return CloseConnection
	}

	message, err := n.sub.NextMsg(time.Minute)
	if err != nil {
		return CloseConnection
	}

	_, err = out.Write(message.Data)
	if err != nil {
		return CloseConnection
	}

	return KeepStreamAlive
}
