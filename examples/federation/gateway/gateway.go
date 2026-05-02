package gateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/examples/federation/gateway/httphandler"
	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func NewGateway(
	ctx context.Context,
	configFileContent []byte,
	httpClient *http.Client,
	logger log.Logger,
	enableART bool,
) (*Gateway, error) {
	var rc nodev1.RouterConfig
	if err := protojson.Unmarshal(configFileContent, &rc); err != nil {
		return nil, fmt.Errorf("can't unmarshal composed config: %w", err)
	}

	engineConfigFactory := engine.NewFederationEngineConfigFactory(ctx, engine.WithFederationHttpClient(httpClient))

	engineConfig, err := engineConfigFactory.BuildEngineConfiguration(&rc)
	if err != nil {
		return nil, fmt.Errorf("can't build engine configuration: %w", err)
	}

	executionEngine, err := engine.NewExecutionEngine(ctx, logger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("can't create an engine: %w", err)
	}

	upgrader := &ws.HTTPUpgrader{
		Header: http.Header{},
	}

	handler := httphandler.NewGraphqlHTTPHandler(engineConfig.Schema(), executionEngine, upgrader, logger, enableART)

	return &Gateway{handler}, nil
}

type Gateway struct {
	http.Handler
}
