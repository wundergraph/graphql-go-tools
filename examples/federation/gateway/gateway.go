package main

import (
	"net/http"
	"sync"

	log "github.com/jensneuse/abstractlogger"

	graphqlDataSource "github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/federation"
)

type DataSourceObserver interface {
	UpdateDataSources(dataSourceConfig []graphqlDataSource.Configuration)
}

type DataSourceSubject interface {
	Register(observer DataSourceObserver)
}

type HandlerFactory interface {
	Make(schema *graphql.Schema, engine *graphql.ExecutionEngineV2) http.Handler
}

type HandlerFactoryFn func(schema *graphql.Schema, engine *graphql.ExecutionEngineV2) http.Handler

func (h HandlerFactoryFn) Make(schema *graphql.Schema, engine *graphql.ExecutionEngineV2) http.Handler {
	return h(schema, engine)
}

func NewGateway(
	gqlHandlerFactory HandlerFactory,
	httpClient *http.Client,
	logger log.Logger,
) *Gateway {
	return &Gateway{
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

	engineCloser chan struct{}
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

// Error handling is not finished.
func (g *Gateway) UpdateDataSources(newDataSourcesConfig []graphqlDataSource.Configuration) {

	if g.engineCloser != nil {
		close(g.engineCloser)
		g.engineCloser = make(chan struct{})
	}

	engineConfigFactory := federation.NewEngineConfigV2Factory(g.httpClient, graphqlDataSource.NewBatchFactory(), newDataSourcesConfig...)

	schema, err := engineConfigFactory.MergedSchema()
	if err != nil {
		g.logger.Error("get schema:", log.Error(err))
		return
	}

	datasourceConfig, err := engineConfigFactory.EngineV2Configuration()
	if err != nil {
		g.logger.Error("get engine config: %v", log.Error(err))
		return
	}

	datasourceConfig.EnableDataLoader(true)

	engine, err := graphql.NewExecutionEngineV2(g.logger, datasourceConfig, g.engineCloser)
	if err != nil {
		g.logger.Error("create engine: %v", log.Error(err))
		return
	}

	g.mu.Lock()
	g.gqlHandler = g.gqlHandlerFactory.Make(schema, engine)
	g.mu.Unlock()

	g.readyOnce.Do(func() { close(g.readyCh) })
}
