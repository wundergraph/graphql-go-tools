package graphql

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/datasource/introspection_datasource"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/TykTechnologies/graphql-go-tools/pkg/pool"
	"github.com/TykTechnologies/graphql-go-tools/pkg/postprocess"
)

type EngineResultWriter struct {
	buf           *bytes.Buffer
	flushCallback func(data []byte)
}

func NewEngineResultWriter() EngineResultWriter {
	return EngineResultWriter{
		buf: &bytes.Buffer{},
	}
}

func NewEngineResultWriterFromBuffer(buf *bytes.Buffer) EngineResultWriter {
	return EngineResultWriter{
		buf: buf,
	}
}

func (e *EngineResultWriter) SetFlushCallback(flushCb func(data []byte)) {
	e.flushCallback = flushCb
}

func (e *EngineResultWriter) Write(p []byte) (n int, err error) {
	return e.buf.Write(p)
}

func (e *EngineResultWriter) Read(p []byte) (n int, err error) {
	return e.buf.Read(p)
}

func (e *EngineResultWriter) Flush() {
	if e.flushCallback != nil {
		e.flushCallback(e.Bytes())
	}

	e.Reset()
}

func (e *EngineResultWriter) Len() int {
	return e.buf.Len()
}

func (e *EngineResultWriter) Bytes() []byte {
	return e.buf.Bytes()
}

func (e *EngineResultWriter) String() string {
	return e.buf.String()
}

func (e *EngineResultWriter) Reset() {
	e.buf.Reset()
}

func (e *EngineResultWriter) AsHTTPResponse(status int, headers http.Header) *http.Response {
	b := &bytes.Buffer{}

	switch headers.Get(httpclient.ContentEncodingHeader) {
	case "gzip":
		gzw := gzip.NewWriter(b)
		_, _ = gzw.Write(e.Bytes())
		_ = gzw.Close()
	case "deflate":
		fw, _ := flate.NewWriter(b, 1)
		_, _ = fw.Write(e.Bytes())
		_ = fw.Close()
	default:
		headers.Del(httpclient.ContentEncodingHeader) // delete unsupported compression header
		b = e.buf
	}

	res := &http.Response{}
	res.Body = io.NopCloser(b)
	res.Header = headers
	res.StatusCode = status
	res.ContentLength = int64(b.Len())
	res.Header.Set("Content-Length", strconv.Itoa(b.Len()))
	return res
}

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
	logger                        abstractlogger.Logger
	config                        EngineV2Configuration
	planner                       *plan.Planner
	plannerMu                     sync.Mutex
	resolver                      *resolve.Resolver
	executionPlanCache            *lru.Cache
	customExecutionEngineExecutor *CustomExecutionEngineV2Executor
}

type WebsocketBeforeStartHook interface {
	OnBeforeStart(reqCtx context.Context, operation *Request) error
}

type ExecutionOptionsV2 func(postProcessor *postprocess.Processor, resolveContext *resolve.Context)

func WithBeforeFetchHook(hook resolve.BeforeFetchHook) ExecutionOptionsV2 {
	return func(postProcessor *postprocess.Processor, resolveContext *resolve.Context) {
		resolveContext.SetBeforeFetchHook(hook)
	}
}

func WithUpstreamHeaders(header http.Header) ExecutionOptionsV2 {
	return func(postProcessor *postprocess.Processor, resolveContext *resolve.Context) {
		postProcessor.AddPostProcessor(postprocess.NewProcessInjectHeader(header))
	}
}

func WithAfterFetchHook(hook resolve.AfterFetchHook) ExecutionOptionsV2 {
	return func(postProcessor *postprocess.Processor, resolveContext *resolve.Context) {
		resolveContext.SetAfterFetchHook(hook)
	}
}

func WithAdditionalHttpHeaders(headers http.Header, excludeByKeys ...string) ExecutionOptionsV2 {
	return func(postProcessor *postprocess.Processor, resolveContext *resolve.Context) {
		if len(headers) == 0 {
			return
		}

		if resolveContext.Request.Header == nil {
			resolveContext.Request.Header = make(http.Header)
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
				resolveContext.Request.Header.Add(headerKey, headerValue)
			}
		}
	}
}

func WithHeaderModifier(modifier postprocess.HeaderModifier) ExecutionOptionsV2 {
	return func(postProcessor *postprocess.Processor, resolveContext *resolve.Context) {
		if modifier == nil {
			return
		}
		postProcessor.AddPostProcessor(postprocess.NewProcessModifyHeader(modifier))
	}
}

func NewExecutionEngineV2(ctx context.Context, logger abstractlogger.Logger, engineConfig EngineV2Configuration) (*ExecutionEngineV2, error) {
	executionPlanCache, err := lru.New(1024)
	if err != nil {
		return nil, err
	}
	fetcher := resolve.NewFetcher(engineConfig.dataLoaderConfig.EnableSingleFlightLoader)

	introspectionCfg, err := introspection_datasource.NewIntrospectionConfigFactory(&engineConfig.schema.document)
	if err != nil {
		return nil, err
	}

	engineConfig.AddDataSource(introspectionCfg.BuildDataSourceConfiguration())
	for _, fieldCfg := range introspectionCfg.BuildFieldConfigurations() {
		engineConfig.AddFieldConfiguration(fieldCfg)
	}

	executionEngine := &ExecutionEngineV2{
		logger:             logger,
		config:             engineConfig,
		planner:            plan.NewPlanner(ctx, engineConfig.plannerConfig),
		resolver:           resolve.New(ctx, fetcher, engineConfig.dataLoaderConfig.EnableDataLoader),
		executionPlanCache: executionPlanCache,
	}

	executor, err := NewCustomExecutionEngineV2Executor(executionEngine)
	if err != nil {
		return nil, err
	}
	executionEngine.customExecutionEngineExecutor = executor
	return executionEngine, nil
}

func (e *ExecutionEngineV2) Normalize(operation *Request) error {
	if !operation.IsNormalized() {
		result, err := operation.Normalize(e.config.schema)
		if err != nil {
			return err
		}

		if !result.Successful {
			return result.Errors
		}
	}
	return nil
}

func (e *ExecutionEngineV2) ValidateForSchema(operation *Request) error {
	result, err := operation.ValidateForSchema(e.config.schema)
	if err != nil {
		return err
	}
	if !result.Valid {
		return result.Errors
	}
	return nil
}

func (e *ExecutionEngineV2) InputValidation(operation *Request) error {
	result, err := operation.ValidateInput(e.config.schema)
	if err != nil {
		return err
	}
	if !result.Valid {
		return result.Errors
	}
	return nil
}

func (e *ExecutionEngineV2) Setup(ctx context.Context, postProcessor *postprocess.Processor, resolveContext *resolve.Context, operation *Request, options ...ExecutionOptionsV2) {
	for i := range options {
		options[i](postProcessor, resolveContext)
	}
}

func (e *ExecutionEngineV2) Plan(postProcessor *postprocess.Processor, operation *Request, report *operationreport.Report) (plan.Plan, error) {
	cachedPlan := e.getCachedPlan(postProcessor, &operation.document, &e.config.schema.document, operation.OperationName, report)
	if report.HasErrors() {
		return nil, report
	}
	return cachedPlan, nil
}

func (e *ExecutionEngineV2) Resolve(resolveContext *resolve.Context, planResult plan.Plan, writer resolve.FlushWriter) error {
	var err error
	switch p := planResult.(type) {
	case *plan.SynchronousResponsePlan:
		err = e.resolver.ResolveGraphQLResponse(resolveContext, p.Response, nil, writer)
	case *plan.SubscriptionResponsePlan:
		err = e.resolver.ResolveGraphQLSubscription(resolveContext, p.Response, writer)
	default:
		return errors.New("execution of operation is not possible")
	}

	return err
}

func (e *ExecutionEngineV2) Teardown() {
}

func (e *ExecutionEngineV2) Execute(ctx context.Context, operation *Request, writer resolve.FlushWriter, options ...ExecutionOptionsV2) error {
	return e.customExecutionEngineExecutor.Execute(ctx, operation, writer, options...)
}

func (e *ExecutionEngineV2) getCachedPlan(postProcessor *postprocess.Processor, operation, definition *ast.Document, operationName string, report *operationreport.Report) plan.Plan {

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

	p := postProcessor.Process(planResult)
	e.executionPlanCache.Add(cacheKey, p)
	return p
}

func (e *ExecutionEngineV2) GetWebsocketBeforeStartHook() WebsocketBeforeStartHook {
	return e.config.websocketBeforeStartHook
}

// Interface Guards
var (
	_ CustomExecutionEngineV2   = (*ExecutionEngineV2)(nil)
	_ ExecutionEngineV2Executor = (*ExecutionEngineV2)(nil)
)
