package gateway

import (
	"net/http"
	"time"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"

	http2 "github.com/wundergraph/graphql-go-tools/v2/pkg/testing/federationtesting/gateway/http"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphql"
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
) *Gateway {
	upgrader := &ws.DefaultHTTPUpgrader
	upgrader.Header = http.Header{}
	// upgrader.Header.Add("Sec-Websocket-Protocol", "graphql-ws")

	datasourceWatcher := datasourcePoller

	var gqlHandlerFactory HandlerFactoryFn = func(schema *graphql.Schema, engine *graphql.ExecutionEngineV2) http.Handler {
		return http2.NewGraphqlHTTPHandler(schema, engine, upgrader, logger)
	}

	gateway := NewGateway(gqlHandlerFactory, httpClient, logger)

	datasourceWatcher.Register(gateway)

	return gateway
}
