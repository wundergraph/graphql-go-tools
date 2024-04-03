package main

import (
	"context"
	"net/http"
	"sync"

	log "github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
)

type DataSourceObserver interface {
	UpdateDataSources(subgraphsConfigs []engine.SubgraphConfiguration)
}

type DataSourceSubject interface {
	Register(observer DataSourceObserver)
}

type HandlerFactory interface {
	Make(schema *graphql.Schema, engine *engine.ExecutionEngine) http.Handler
}

type HandlerFactoryFn func(schema *graphql.Schema, engine *engine.ExecutionEngine) http.Handler

func (h HandlerFactoryFn) Make(schema *graphql.Schema, engine *engine.ExecutionEngine) http.Handler {
	return h(schema, engine)
}

func NewGateway(
	ctx context.Context,
	gqlHandlerFactory HandlerFactory,
	httpClient *http.Client,
	logger log.Logger,
) *Gateway {
	return &Gateway{
		engineCtx:         ctx,
		gqlHandlerFactory: gqlHandlerFactory,
		httpClient:        httpClient,
		logger:            logger,

		mu:        &sync.Mutex{},
		readyCh:   make(chan struct{}),
		readyOnce: &sync.Once{},
	}
}

type Gateway struct {
	gqlHandlerFactory HandlerFactory
	httpClient        *http.Client
	logger            log.Logger

	gqlHandler http.Handler
	mu         *sync.Mutex

	readyCh   chan struct{}
	readyOnce *sync.Once
	engineCtx context.Context
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	handler := g.gqlHandler
	g.mu.Unlock()

	handler.ServeHTTP(w, r)
}

func (g *Gateway) Ready() {
	<-g.readyCh
}

func (g *Gateway) UpdateDataSources(subgraphsConfigs []engine.SubgraphConfiguration) {
	engineConfigFactory := engine.NewFederationEngineConfigFactory(g.engineCtx, subgraphsConfigs, engine.WithFederationHttpClient(g.httpClient))

	engineConfig, err := engineConfigFactory.BuildEngineConfiguration()
	if err != nil {
		g.logger.Error("get engine config: %v", log.Error(err))
		return
	}

	executionEngine, err := engine.NewExecutionEngine(g.engineCtx, g.logger, engineConfig)
	if err != nil {
		g.logger.Error("create engine: %v", log.Error(err))
		return
	}

	g.mu.Lock()
	g.gqlHandler = g.gqlHandlerFactory.Make(engineConfig.Schema(), executionEngine)
	g.mu.Unlock()

	g.readyOnce.Do(func() { close(g.readyCh) })
}
