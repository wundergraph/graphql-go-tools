package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"io"
	"sync"
	"time"
)

type NatsDataSourcePlanner struct {
	BaseDataSourcePlanner
}

func NewNatsDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *NatsDataSourcePlanner {
	return &NatsDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

func (n *NatsDataSourcePlanner) DirectiveName() []byte {
	return []byte("NatsDataSource")
}

func (n *NatsDataSourcePlanner) DirectiveDefinition() []byte {
	data, _ := n.graphqlDefinitions.Find("directives/nats_datasource.graphql")
	return data
}

func (n *NatsDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &NatsDataSource{
		log: n.log,
	}, n.args
}

func (n *NatsDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	n.walker, n.operation, n.definition, n.args = walker, operation, definition, args
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
	definition, exists := n.walker.FieldDefinition(ref)
	if !exists {
		return
	}
	directive, exists := n.definition.FieldDefinitionDirectiveByName(definition, n.DirectiveName())
	if !exists {
		return
	}
	value, exists := n.definition.DirectiveArgumentValueByName(directive, literal.ADDR)
	if !exists {
		return
	}
	variableValue := n.definition.StringValueContentBytes(value.Ref)
	arg := &StaticVariableArgument{
		Name:  literal.ADDR,
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

type NatsDataSource struct {
	log  *zap.Logger
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
					zap.String("addr", addr),
					zap.String("topic", topic),
				)
				err := n.sub.Unsubscribe()
				if err != nil {
					n.log.Error("Unsubscribe", zap.Error(err))
				}
			}
			if n.conn != nil {
				n.log.Debug("NatsDataSource.closing",
					zap.String("addr", addr),
					zap.String("topic", topic),
				)
				n.conn.Close()
			}
		}()

		n.log.Debug("NatsDataSource.connecting",
			zap.String("addr", addr),
			zap.String("topic", topic),
		)

		n.conn, err = nats.Connect(addr)
		if err != nil {
			panic(err)
		}

		n.log.Debug("NatsDataSource.subscribing",
			zap.String("addr", addr),
			zap.String("topic", topic),
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
