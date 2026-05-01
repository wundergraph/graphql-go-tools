package gateway

import (
	"context"
	"net/http"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway/httphandler"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func NewGateway(
	config []byte,
	httpClient *http.Client,
	logger log.Logger,
	enableART bool,
) *Gateway {
	return &Gateway{
		configFileContent: config,
		httpClient:        httpClient,
		logger:            logger,
		enableART:         enableART,
	}
}

type Gateway struct {
	configFileContent []byte
	httpClient        *http.Client
	logger            log.Logger
	enableART         bool
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var rc nodev1.RouterConfig
	if err := protojson.Unmarshal(g.configFileContent, &rc); err != nil {
		g.logger.Fatal("can't unmarshal composed config: %v", log.Error(err))
	}

	ctx := context.Background()
	engineConfigFactory := engine.NewFederationEngineConfigFactory(ctx, engine.WithFederationHttpClient(g.httpClient))

	engineConfig, err := engineConfigFactory.BuildEngineConfiguration(&rc)
	if err != nil {
		g.logger.Fatal("can't build engine configuration: %v", log.Error(err))
		return
	}

	executionEngine, err := engine.NewExecutionEngine(ctx, g.logger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	if err != nil {
		g.logger.Fatal("can't create an engine: %v", log.Error(err))
		return
	}

	upgrader := &ws.HTTPUpgrader{
		Header: http.Header{},
	}

	handler := httphandler.NewGraphqlHTTPHandler(engineConfig.Schema(), executionEngine, upgrader, g.logger, g.enableART)

	handler.ServeHTTP(w, r)
}
