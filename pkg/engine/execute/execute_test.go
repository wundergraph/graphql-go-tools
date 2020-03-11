package execute

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	statement "github.com/jensneuse/graphql-go-tools/pkg/engine/statementv3"
)

func TestExecutor_Execute_Single(t *testing.T) {
	client := ioutil.Discard // this could be a real client
	executor := Executor{}
	ctx := Context{
		Context: context.Background(),
	}                                   // each GraphQL operation get's a new execution.Context based on the client connection context.Context which holds the execution state
	stmt := statement.SingleStatement{} // the prepared statement from the statement cache

	id, err := executor.PrepareSingleStatement(stmt)

	buf := bytes.Buffer{}
	_, err = executor.ExecutePreparedSingleStatement(ctx, id, &buf)
	if err != nil {
		return
	}
	_, err = buf.WriteTo(client)
	if err != nil {
		return
	}
}

func TestExecutor_Execute_Streaming(t *testing.T) {
	client := ioutil.Discard
	executor := Executor{}
	ctx := Context{
		Context: context.Background(),
	}
	stmt := statement.StreamingStatement{}

	buf := bytes.Buffer{}
	var err error
	hasNext := true
	for hasNext {
		_, hasNext, err = executor.ExecuteStreamingStatement(ctx, &stmt, &buf)
		if err != nil {
			return
		}
		_, err = buf.WriteTo(client)
		if err != nil {
			return
		}
		buf.Reset()
	}
}

func TestExecutor_Execute_Subscription(t *testing.T) {
	executor := Executor{}
	ctx := Context{
		Context: context.Background(),
	}
	stmt := statement.SubscriptionStatement{}
	subStream := fakeSubscriptionStream{}

	err := executor.ExecuteSubscriptionStatement(ctx, &stmt, &subStream)
	if err != nil {
		return
	}
}

type fakeSubscriptionStream struct {
}

func (f *fakeSubscriptionStream) WriteMessage(message []byte) error {
	return nil
}
