package subscription

import (
	"bytes"
	"context"
	"sync"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
	"github.com/TykTechnologies/graphql-go-tools/pkg/postprocess"
)

type ExecutorV2PoolOptions struct {
	HeaderModifier postprocess.HeaderModifier
}

type ExecutorV2PoolOptionFunc func(options *ExecutorV2PoolOptions)

func WithExecutorV2HeaderModifier(headerModifier postprocess.HeaderModifier) ExecutorV2PoolOptionFunc {
	return func(options *ExecutorV2PoolOptions) {
		options.HeaderModifier = headerModifier
	}
}

// ExecutorV2Pool - provides reusable executors
type ExecutorV2Pool struct {
	engine               *graphql.ExecutionEngineV2
	executorPool         *sync.Pool
	connectionInitReqCtx context.Context // connectionInitReqCtx - holds original request context used to establish websocket connection
	options              ExecutorV2PoolOptions
}

func NewExecutorV2Pool(engine *graphql.ExecutionEngineV2, connectionInitReqCtx context.Context, options ...ExecutorV2PoolOptionFunc) *ExecutorV2Pool {
	executorV2Pool := &ExecutorV2Pool{
		engine: engine,
		executorPool: &sync.Pool{
			New: func() interface{} {
				return &ExecutorV2{}
			},
		},
		connectionInitReqCtx: connectionInitReqCtx,
		options:              ExecutorV2PoolOptions{},
	}

	for _, optionFunc := range options {
		optionFunc(&executorV2Pool.options)
	}

	return executorV2Pool
}

func (e *ExecutorV2Pool) Get(payload []byte) (Executor, error) {
	operation := graphql.Request{}
	err := graphql.UnmarshalRequest(bytes.NewReader(payload), &operation)
	if err != nil {
		return nil, err
	}

	return &ExecutorV2{
		engine:         e.engine,
		operation:      &operation,
		context:        context.Background(),
		reqCtx:         e.connectionInitReqCtx,
		headerModifier: e.options.HeaderModifier,
	}, nil
}

func (e *ExecutorV2Pool) Put(executor Executor) error {
	executor.Reset()
	e.executorPool.Put(executor)
	return nil
}

type ExecutorV2 struct {
	engine         *graphql.ExecutionEngineV2
	operation      *graphql.Request
	context        context.Context
	reqCtx         context.Context
	headerModifier postprocess.HeaderModifier
}

func (e *ExecutorV2) Execute(writer resolve.FlushWriter) error {
	options := make([]graphql.ExecutionOptionsV2, 0)
	switch ctx := e.reqCtx.(type) {
	case *InitialHttpRequestContext:
		options = append(options, graphql.WithAdditionalHttpHeaders(ctx.Request.Header))
	}

	if e.headerModifier != nil {
		options = append(options, graphql.WithHeaderModifier(e.headerModifier))
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
