package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/introspection_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/variablesvalidation"
)

type internalExecutionContext struct {
	resolveContext *resolve.Context
	postProcessor  *postprocess.Processor
}

func newInternalExecutionContext() *internalExecutionContext {
	return &internalExecutionContext{
		resolveContext: resolve.NewContext(context.Background()),
		postProcessor:  postprocess.NewProcessor(),
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

type ExecutionEngine struct {
	logger             abstractlogger.Logger
	config             Configuration
	resolver           *resolve.Resolver
	executionPlanCache *lru.Cache
}

type WebsocketBeforeStartHook interface {
	OnBeforeStart(reqCtx context.Context, operation *graphql.Request) error
}

type ExecutionOptions func(ctx *internalExecutionContext)

func WithAdditionalHttpHeaders(headers http.Header, excludeByKeys ...string) ExecutionOptions {
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

func WithRequestTraceOptions(options resolve.TraceOptions) ExecutionOptions {
	return func(ctx *internalExecutionContext) {
		ctx.resolveContext.TracingOptions = options
	}
}

func NewExecutionEngine(ctx context.Context, logger abstractlogger.Logger, engineConfig Configuration, resolverOptions resolve.ResolverOptions) (*ExecutionEngine, error) {
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

	dsIDs := make(map[string]struct{}, len(engineConfig.plannerConfig.DataSources))
	for _, ds := range engineConfig.plannerConfig.DataSources {
		if _, ok := dsIDs[ds.Id()]; ok {
			return nil, fmt.Errorf("duplicate datasource id: %s", ds.Id())
		}
		dsIDs[ds.Id()] = struct{}{}
	}

	return &ExecutionEngine{
		logger:             logger,
		config:             engineConfig,
		resolver:           resolve.New(ctx, resolverOptions),
		executionPlanCache: executionPlanCache,
	}, nil
}

func (e *ExecutionEngine) Execute(ctx context.Context, operation *graphql.Request, writer resolve.SubscriptionResponseWriter, options ...ExecutionOptions) error {

	normalize := !operation.IsNormalized()
	if normalize {
		// Normalize the operation, but extract variables later so ValidateForSchema can return correct error messages for bad arguments.
		result, err := operation.Normalize(e.config.schema,
			astnormalization.WithRemoveFragmentDefinitions(),
			astnormalization.WithRemoveUnusedVariables(),
			astnormalization.WithInlineFragmentSpreads(),
		)
		if err != nil {
			return err
		} else if !result.Successful {
			return result.Errors
		}
		normalize = true
	}

	// Validate the operation against the schema.
	if result, err := operation.ValidateForSchema(e.config.schema); err != nil {
		return err
	} else if !result.Valid {
		return result.Errors
	}

	if normalize {
		// Normalize the operation again, this time just extracting additional variables from arguments.
		result, err := operation.Normalize(e.config.schema,
			astnormalization.WithExtractVariables(),
		)
		if err != nil {
			return err
		} else if !result.Successful {
			return result.Errors
		}
	}

	// Validate user-supplied and extracted variables against the operation.
	if len(operation.Variables) > 0 && operation.Variables[0] == '{' {
		validator := variablesvalidation.NewVariablesValidator()
		if err := validator.Validate(operation.Document(), e.config.schema.Document(), operation.Variables); err != nil {
			return err
		}
	}

	execContext := newInternalExecutionContext()
	execContext.prepare(ctx, operation.Variables, operation.InternalRequest())
	for i := range options {
		options[i](execContext)
	}

	if execContext.resolveContext.TracingOptions.Enable {
		traceCtx := resolve.SetTraceStart(execContext.resolveContext.Context(), execContext.resolveContext.TracingOptions.EnablePredictableDebugTimings)
		execContext.setContext(traceCtx)
	}

	var tracePlanStart int64
	if execContext.resolveContext.TracingOptions.Enable && !execContext.resolveContext.TracingOptions.ExcludePlannerStats {
		tracePlanStart = resolve.GetDurationNanoSinceTraceStart(execContext.resolveContext.Context())
	}

	var report operationreport.Report
	cachedPlan := e.getCachedPlan(execContext, operation.Document(), e.config.schema.Document(), operation.OperationName, &report)
	if report.HasErrors() {
		return report
	}

	if execContext.resolveContext.TracingOptions.Enable && !execContext.resolveContext.TracingOptions.ExcludePlannerStats {
		planningTime := resolve.GetDurationNanoSinceTraceStart(execContext.resolveContext.Context()) - tracePlanStart
		resolve.SetPlannerStats(execContext.resolveContext.Context(), resolve.PhaseStats{
			DurationSinceStartNano:   tracePlanStart,
			DurationSinceStartPretty: time.Duration(tracePlanStart).String(),
			DurationNano:             planningTime,
			DurationPretty:           time.Duration(planningTime).String(),
		})
	}

	switch p := cachedPlan.(type) {
	case *plan.SynchronousResponsePlan:
		_, err := e.resolver.ResolveGraphQLResponse(execContext.resolveContext, p.Response, nil, writer)
		return err
	case *plan.SubscriptionResponsePlan:
		return e.resolver.ResolveGraphQLSubscription(execContext.resolveContext, p.Response, writer)
	default:
		return errors.New("execution of operation is not possible")
	}
}

func (e *ExecutionEngine) getCachedPlan(ctx *internalExecutionContext, operation, definition *ast.Document, operationName string, report *operationreport.Report) plan.Plan {

	hash := pool.Hash64.Get()
	hash.Reset()
	defer pool.Hash64.Put(hash)
	err := astprinter.Print(operation, hash)
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

	planner, _ := plan.NewPlanner(e.config.plannerConfig)
	planResult := planner.Plan(operation, definition, operationName, report)
	if report.HasErrors() {
		return nil
	}

	p := ctx.postProcessor.Process(planResult)
	e.executionPlanCache.Add(cacheKey, p)
	return p
}

func (e *ExecutionEngine) GetWebsocketBeforeStartHook() WebsocketBeforeStartHook {
	return e.config.websocketBeforeStartHook
}
