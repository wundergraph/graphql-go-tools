// Package execute takes a prepared statement as well as an execution context with variables and executed it
package execute

import (
	"context"
	"io"

	statement "github.com/jensneuse/graphql-go-tools/pkg/engine/statementv3"
)

type Executor struct {
	resolvers        map[string]statement.Resolver
	singleStatements []statement.SingleStatement
}

func New() *Executor {
	return &Executor{
		resolvers: map[string]statement.Resolver{},
	}
}

type Context struct {
	context.Context
}

func (e *Executor) RegisterResolver(name string, resolver statement.Resolver) {
	e.resolvers[name] = resolver
}

func (e *Executor) ExecuteStreamingStatement(ctx Context, stmt *statement.StreamingStatement, out io.Writer) (n int, hasNext bool, err error) {
	return
}

type SubscriptionStream interface {
	WriteMessage(message []byte) error
}

func (e *Executor) ExecuteSubscriptionStatement(ctx Context, stmt *statement.SubscriptionStatement, stream SubscriptionStream) (err error) {
	return
}
