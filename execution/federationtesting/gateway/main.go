package gateway

import (
	"net/http"
	"time"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	http2 "github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway/http"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func NewDatasource(serviceConfig []ServiceConfig, httpClient *http.Client) *DatasourcePollerPoller {
	return NewDatasourcePoller(httpClient, DatasourcePollerConfig{
		Services:        serviceConfig,
		PollingInterval: 30 * time.Second,
	})
}

func Handler(
	logger log.Logger,
	datasourcePoller *DatasourcePollerPoller,
	httpClient *http.Client,
	enableART bool,
	loaderCaches map[string]resolve.LoaderCache,
	subgraphHeadersBuilder resolve.SubgraphHeadersBuilder,
) *Gateway {
	return HandlerWithCaching(logger, datasourcePoller, httpClient, enableART, loaderCaches, subgraphHeadersBuilder, resolve.CachingOptions{}, nil, false)
}

func HandlerWithCaching(
	logger log.Logger,
	datasourcePoller *DatasourcePollerPoller,
	httpClient *http.Client,
	enableART bool,
	loaderCaches map[string]resolve.LoaderCache,
	subgraphHeadersBuilder resolve.SubgraphHeadersBuilder,
	cachingOptions resolve.CachingOptions,
	subgraphEntityCachingConfigs engine.SubgraphCachingConfigs,
	debugMode bool,
) *Gateway {
	return HandlerWithCachingAndOpts(logger, datasourcePoller, httpClient, enableART, loaderCaches, subgraphHeadersBuilder, cachingOptions, subgraphEntityCachingConfigs, debugMode)
}

// HandlerWithCachingAndOpts is like HandlerWithCaching but accepts additional GatewayOptions
// for configuring resolver-level options (e.g., OnSubscriptionCacheWrite callbacks).
func HandlerWithCachingAndOpts(
	logger log.Logger,
	datasourcePoller *DatasourcePollerPoller,
	httpClient *http.Client,
	enableART bool,
	loaderCaches map[string]resolve.LoaderCache,
	subgraphHeadersBuilder resolve.SubgraphHeadersBuilder,
	cachingOptions resolve.CachingOptions,
	subgraphEntityCachingConfigs engine.SubgraphCachingConfigs,
	debugMode bool,
	extraOpts ...GatewayOption,
) *Gateway {
	upgrader := &ws.HTTPUpgrader{
		Header: http.Header{},
	}

	datasourceWatcher := datasourcePoller

	// remapVariables is captured by the handler factory closure.
	// The extraction opt (appended last) copies the value set by extraOpts.
	var remapVariables map[string]string

	var gqlHandlerFactory HandlerFactoryFn = func(schema *graphql.Schema, engine *engine.ExecutionEngine) http.Handler {
		return http2.NewGraphqlHTTPHandler(schema, engine, upgrader, logger, enableART, subgraphHeadersBuilder, cachingOptions, debugMode, remapVariables)
	}

	var gatewayOpts []GatewayOption
	if len(subgraphEntityCachingConfigs) > 0 {
		gatewayOpts = append(gatewayOpts, WithSubgraphEntityCachingConfigs(subgraphEntityCachingConfigs))
	}
	gatewayOpts = append(gatewayOpts, extraOpts...)
	gatewayOpts = append(gatewayOpts, func(g *Gateway) {
		remapVariables = g.remapVariables
	})

	gateway := NewGateway(gqlHandlerFactory, httpClient, logger, loaderCaches, gatewayOpts...)

	datasourceWatcher.Register(gateway)

	return gateway
}
