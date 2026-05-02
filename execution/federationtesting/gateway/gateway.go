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
	configFileContent []byte,
	httpClient *http.Client,
	logger log.Logger,
	enableART bool,
) *Gateway {
	var rc nodev1.RouterConfig
	if err := protojson.Unmarshal(configFileContent, &rc); err != nil {
		logger.Fatal("can't unmarshal composed config: %v", log.Error(err))
		return nil
	}

	ctx := context.Background()
	engineConfigFactory := engine.NewFederationEngineConfigFactory(ctx, engine.WithFederationHttpClient(httpClient))

	engineConfig, err := engineConfigFactory.BuildEngineConfiguration(&rc)
	if err != nil {
		logger.Fatal("can't build engine configuration: %v", log.Error(err))
		return nil
	}

	executionEngine, err := engine.NewExecutionEngine(ctx, logger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	if err != nil {
		logger.Fatal("can't create an engine: %v", log.Error(err))
		return nil
	}

	upgrader := &ws.HTTPUpgrader{
		Header: http.Header{},
	}

	handler := httphandler.NewGraphqlHTTPHandler(engineConfig.Schema(), executionEngine, upgrader, logger, enableART)

	return &Gateway{handler}
}

type Gateway struct {
	http.Handler
}
