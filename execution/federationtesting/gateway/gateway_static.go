package gateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/gateway/httphandler"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// NewStaticGateway builds an http.Handler from a pre-composed router config
// (master's #1483 wgc-driven flow). It is the static counterpart to NewGateway,
// which still backs the dynamic, options-based path used by the caching e2e
// suite and HandlerWithCachingAndOpts.
//
// The static path is intentionally minimal — no caching scaffolding, no
// observer/poller wiring — because the federation integration tests it serves
// only need a vanilla federated gateway.
func NewStaticGateway(
	configFileContent []byte,
	httpClient *http.Client,
	logger log.Logger,
	enableART bool,
) (http.Handler, error) {
	var rc nodev1.RouterConfig
	if err := protojson.Unmarshal(configFileContent, &rc); err != nil {
		return nil, fmt.Errorf("can't unmarshal composed config: %w", err)
	}

	ctx := context.Background()
	engineConfigFactory := engine.NewFederationEngineConfigFactory(ctx, nil, engine.WithFederationHttpClient(httpClient))

	engineConfig, err := engineConfigFactory.BuildEngineConfigurationWithRouterConfig(&rc)
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

	return httphandler.NewGraphqlHTTPHandler(
		engineConfig.Schema(),
		executionEngine,
		upgrader,
		logger,
		enableART,
		nil, // subgraphHeadersBuilder — static gateway has no per-subgraph header propagation
		resolve.CachingOptions{},
		false,
		nil, // remapVariables
	), nil
}
