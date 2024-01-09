package subscription

//go:generate mockgen -destination=executor_mock_test.go -package=subscription . Executor,ExecutorPool

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Executor is an abstraction for executing a GraphQL engine
type Executor interface {
	Execute(writer resolve.FlushWriter) error
	OperationType() ast.OperationType
	SetContext(context context.Context)
	Reset()
}

// ExecutorPool is an abstraction for creating executors
type ExecutorPool interface {
	Get(payload []byte) (Executor, error)
	Put(executor Executor) error
}
