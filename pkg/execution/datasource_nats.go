package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/nats-io/nats.go"
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
	return &NatsDataSource{}, []Argument{}
}

func (n *NatsDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {

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

}

func (n *NatsDataSourcePlanner) LeaveField(ref int) {

}

type NatsDataSource struct {
	conn *nats.Conn
	sub  *nats.Subscription
	once sync.Once
}

func (n *NatsDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	var err error
	n.once.Do(func() {
		n.conn, err = nats.Connect(nats.DefaultURL)
		if err != nil {
			panic(err)
		}
		n.sub, err = n.conn.SubscribeSync("time")
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
