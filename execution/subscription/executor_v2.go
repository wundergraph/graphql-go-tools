package subscription

import (
	"bytes"
	"context"
	"sync"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// ExecutorV2Pool - provides reusable executors
type ExecutorV2Pool struct {
	engine               *engine.ExecutionEngine
	executorPool         *sync.Pool
	connectionInitReqCtx context.Context // connectionInitReqCtx - holds original request context used to establish websocket connection
}

func NewExecutorV2Pool(engine *engine.ExecutionEngine, connectionInitReqCtx context.Context) *ExecutorV2Pool {
	return &ExecutorV2Pool{
		engine: engine,
		executorPool: &sync.Pool{
			New: func() interface{} {
				return &ExecutorV2{}
			},
		},
		connectionInitReqCtx: connectionInitReqCtx,
	}
}

func (e *ExecutorV2Pool) Get(payload []byte) (Executor, error) {
	operation := graphql.Request{}
	err := graphql.UnmarshalRequest(bytes.NewReader(payload), &operation)
	if err != nil {
		return nil, err
	}

	return &ExecutorV2{
		engine:    e.engine,
		operation: &operation,
		context:   context.Background(),
		reqCtx:    e.connectionInitReqCtx,
	}, nil
}

func (e *ExecutorV2Pool) Put(executor Executor) error {
	executor.Reset()
	e.executorPool.Put(executor)
	return nil
}

type ExecutorV2 struct {
	engine    *engine.ExecutionEngine
	operation *graphql.Request
	context   context.Context
	reqCtx    context.Context
}

func (e *ExecutorV2) Execute(writer resolve.SubscriptionResponseWriter) error {
	options := make([]engine.ExecutionOptions, 0)
	switch ctx := e.reqCtx.(type) {
	case *InitialHttpRequestContext:
		options = append(options, engine.WithAdditionalHttpHeaders(ctx.Request.Header))
	}

	return e.engine.Execute(e.context, e.operation, writer, options...)
}

func (e *ExecutorV2) OperationType() ast.OperationType {
	opType, err := e.operation.OperationType()
	if err != nil {
		return ast.OperationTypeUnknown
	}

	return ast.OperationType(opType)
}

func (e *ExecutorV2) SetContext(context context.Context) {
	e.context = context
}

func (e *ExecutorV2) Reset() {
	e.engine = nil
	e.operation = nil
	e.context = context.Background()
	e.reqCtx = context.TODO()
}
