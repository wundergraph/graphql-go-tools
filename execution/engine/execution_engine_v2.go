package engine

import (
	"context"
	"errors"
	"net/http"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/introspection_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type internalExecutionContext struct {
	resolveContext *resolve.Context
	postProcessor  *postprocess.Processor
}

func newInternalExecutionContext() *internalExecutionContext {
	return &internalExecutionContext{
		resolveContext: resolve.NewContext(context.Background()),
		postProcessor:  postprocess.DefaultProcessor(),
	}
}

func (e *internalExecutionContext) prepare(ctx context.Context, variables []byte, request resolve.Request) {
	e.setContext(ctx)
	e.setVariables(variables)
	e.setRequest(request)
}

func (e *internalExecutionContext) setRequest(request resolve.Request) {
	e.resolveContext.Request = request
}

func (e *internalExecutionContext) setContext(ctx context.Context) {
	e.resolveContext = e.resolveContext.WithContext(ctx)
}

func (e *internalExecutionContext) setVariables(variables []byte) {
	e.resolveContext.Variables = variables
}

func (e *internalExecutionContext) reset() {
	e.resolveContext.Free()
}

type ExecutionEngineV2 struct {
	logger                       abstractlogger.Logger
	config                       EngineV2Configuration
	planner                      *plan.Planner
	plannerMu                    sync.Mutex
	resolver                     *resolve.Resolver
	internalExecutionContextPool sync.Pool
	executionPlanCache           *lru.Cache
}

type WebsocketBeforeStartHook interface {
	OnBeforeStart(reqCtx context.Context, operation *graphql.Request) error
}

type ExecutionOptionsV2 func(ctx *internalExecutionContext)

func WithAdditionalHttpHeaders(headers http.Header, excludeByKeys ...string) ExecutionOptionsV2 {
	return func(ctx *internalExecutionContext) {
		if len(headers) == 0 {
			return
		}

		if ctx.resolveContext.Request.Header == nil {
			ctx.resolveContext.Request.Header = make(http.Header)
		}

		excludeMap := make(map[string]bool)
		for _, key := range excludeByKeys {
			excludeMap[key] = true
		}

		for headerKey, headerValues := range headers {
			if excludeMap[headerKey] {
				continue
			}

			for _, headerValue := range headerValues {
				ctx.resolveContext.Request.Header.Add(headerKey, headerValue)
			}
		}
	}
}

func NewExecutionEngineV2(ctx context.Context, logger abstractlogger.Logger, engineConfig EngineV2Configuration) (*ExecutionEngineV2, error) {
	executionPlanCache, err := lru.New(1024)
	if err != nil {
		return nil, err
	}

	introspectionCfg, err := introspection_datasource.NewIntrospectionConfigFactory(engineConfig.schema.Document())
	if err != nil {
		return nil, err
	}

	for _, dataSource := range introspectionCfg.BuildDataSourceConfigurations() {
		engineConfig.AddDataSource(dataSource)
	}

	for _, fieldCfg := range introspectionCfg.BuildFieldConfigurations() {
		engineConfig.AddFieldConfiguration(fieldCfg)
	}

	planner, err := plan.NewPlanner(engineConfig.plannerConfig)
	if err != nil {
		return nil, err
	}

	return &ExecutionEngineV2{
		logger:  logger,
		config:  engineConfig,
		planner: planner,
		resolver: resolve.New(ctx, resolve.ResolverOptions{
			MaxConcurrency: 1024,
		}),
		internalExecutionContextPool: sync.Pool{
			New: func() interface{} {
				return newInternalExecutionContext()
			},
		},
		executionPlanCache: executionPlanCache,
	}, nil
}

func (e *ExecutionEngineV2) Execute(ctx context.Context, operation *graphql.Request, writer resolve.SubscriptionResponseWriter, options ...ExecutionOptionsV2) error {
	if !operation.IsNormalized() {
		result, err := operation.Normalize(e.config.schema)
		if err != nil {
			return err
		}

		if !result.Successful {
			return result.Errors
		}
	}

	result, err := operation.ValidateForSchema(e.config.schema)
	if err != nil {
		return err
	}
	if !result.Valid {
		return result.Errors
	}

	execContext := e.getExecutionCtx()
	defer e.putExecutionCtx(execContext)

	execContext.prepare(ctx, operation.Variables, operation.InternalRequest())

	for i := range options {
		options[i](execContext)
	}

	var report operationreport.Report

	cachedPlan := e.getCachedPlan(execContext, operation.Document(), e.config.schema.Document(), operation.OperationName, &report)
	if report.HasErrors() {
		return report
	}

	switch p := cachedPlan.(type) {
	case *plan.SynchronousResponsePlan:
		err = e.resolver.ResolveGraphQLResponse(execContext.resolveContext, p.Response, nil, writer)
	case *plan.SubscriptionResponsePlan:
		err = e.resolver.AsyncResolveGraphQLSubscription(execContext.resolveContext, p.Response, writer, resolve.SubscriptionIdentifier{})
	default:
		return errors.New("execution of operation is not possible")
	}

	return err
}

func (e *ExecutionEngineV2) getCachedPlan(ctx *internalExecutionContext, operation, definition *ast.Document, operationName string, report *operationreport.Report) plan.Plan {

	hash := pool.Hash64.Get()
	hash.Reset()
	defer pool.Hash64.Put(hash)
	err := astprinter.Print(operation, definition, hash)
	if err != nil {
		report.AddInternalError(err)
		return nil
	}

	cacheKey := hash.Sum64()

	if cached, ok := e.executionPlanCache.Get(cacheKey); ok {
		if p, ok := cached.(plan.Plan); ok {
			return p
		}
	}

	e.plannerMu.Lock()
	defer e.plannerMu.Unlock()
	planResult := e.planner.Plan(operation, definition, operationName, report)
	if report.HasErrors() {
		return nil
	}

	p := ctx.postProcessor.Process(planResult)
	e.executionPlanCache.Add(cacheKey, p)
	return p
}

func (e *ExecutionEngineV2) GetWebsocketBeforeStartHook() WebsocketBeforeStartHook {
	return e.config.websocketBeforeStartHook
}

func (e *ExecutionEngineV2) getExecutionCtx() *internalExecutionContext {
	return e.internalExecutionContextPool.Get().(*internalExecutionContext)
}

func (e *ExecutionEngineV2) putExecutionCtx(ctx *internalExecutionContext) {
	ctx.reset()
	e.internalExecutionContextPool.Put(ctx)
}
