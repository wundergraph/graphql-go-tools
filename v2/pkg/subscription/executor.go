package subscription

//go:generate mockgen -destination=executor_mock_test.go -package=subscription . Executor,ExecutorPool

import (
	"context"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

// Executor is an abstraction for executing a GraphQL engine
type Executor interface {
	Execute(writer resolve.SubscriptionResponseWriter) error
	OperationType() ast.OperationType
	SetContext(context context.Context)
	Reset()
}

// ExecutorPool is an abstraction for creating executors
type ExecutorPool interface {
	Get(payload []byte) (Executor, error)
	Put(executor Executor) error
}
