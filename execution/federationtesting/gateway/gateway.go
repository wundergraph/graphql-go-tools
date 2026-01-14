package gateway

import (
	"context"
	"net/http"
	"sync"

	log "github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// GatewayOption is a function that configures a Gateway
type GatewayOption func(*Gateway)

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
	gqlHandlerFactory HandlerFactory,
	httpClient *http.Client,
	logger log.Logger,
	loaderCaches map[string]resolve.LoaderCache,
	opts ...GatewayOption,
) *Gateway {
	g := &Gateway{
		gqlHandlerFactory: gqlHandlerFactory,
		httpClient:        httpClient,
		logger:            logger,
		loaderCaches:      loaderCaches,

		mu:        &sync.Mutex{},
		readyCh:   make(chan struct{}),
		readyOnce: &sync.Once{},
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

type Gateway struct {
	gqlHandlerFactory            HandlerFactory
	httpClient                   *http.Client
	logger                       log.Logger
	loaderCaches                 map[string]resolve.LoaderCache
	subgraphEntityCachingConfigs engine.SubgraphCachingConfigs

	gqlHandler http.Handler
	mu         *sync.Mutex

	readyCh   chan struct{}
	readyOnce *sync.Once
}

// WithSubgraphEntityCachingConfigs configures per-subgraph entity caching for the gateway
func WithSubgraphEntityCachingConfigs(configs engine.SubgraphCachingConfigs) GatewayOption {
	return func(g *Gateway) {
		g.subgraphEntityCachingConfigs = configs
	}
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
	ctx := context.Background()

	opts := []engine.FederationEngineConfigFactoryOption{
		engine.WithFederationHttpClient(g.httpClient),
	}
	if len(g.subgraphEntityCachingConfigs) > 0 {
		opts = append(opts, engine.WithSubgraphEntityCachingConfigs(g.subgraphEntityCachingConfigs))
	}

	engineConfigFactory := engine.NewFederationEngineConfigFactory(ctx, subgraphsConfigs, opts...)

	engineConfig, err := engineConfigFactory.BuildEngineConfiguration()
	if err != nil {
		g.logger.Error("get engine config: %v", log.Error(err))
		return
	}

	executionEngine, err := engine.NewExecutionEngine(ctx, g.logger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
		Caches:         g.loaderCaches,
	})
	if err != nil {
		g.logger.Error("create engine: %v", log.Error(err))
		return
	}

	g.mu.Lock()
	g.gqlHandler = g.gqlHandlerFactory.Make(engineConfig.Schema(), executionEngine)
	g.mu.Unlock()

	g.readyOnce.Do(func() { close(g.readyCh) })
}
