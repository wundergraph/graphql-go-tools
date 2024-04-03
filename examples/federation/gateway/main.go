package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"
	"go.uber.org/zap"

	gatewayHttp "github.com/wundergraph/graphql-go-tools/examples/federation/gateway/http"
	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/playground"
)

// It's just a simple example of graphql federation gateway server, it's NOT a production ready code.
func logger() log.Logger {
	logger, err := zap.NewDevelopmentConfig().Build()
	if err != nil {
		panic(err)
	}

	return log.NewZapLogger(logger, log.DebugLevel)
}

func fallback(sc *ServiceConfig) (string, error) {
	dat, err := os.ReadFile(sc.Name + "/graph/schema.graphqls")
	if err != nil {
		return "", err
	}

	return string(dat), nil
}

func startServer() {
	logger := logger()
	logger.Info("logger initialized")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upgrader := &ws.DefaultHTTPUpgrader
	upgrader.Header = http.Header{}
	upgrader.Header.Add("Sec-Websocket-Protocol", "graphql-ws")

	graphqlEndpoint := "/query"
	playgroundURLPrefix := "/playground"
	playgroundURL := ""

	httpClient := http.DefaultClient

	mux := http.NewServeMux()

	datasourceWatcher := NewDatasourcePoller(httpClient, DatasourcePollerConfig{
		Services: []ServiceConfig{
			{Name: "accounts", URL: "http://localhost:4001/query", Fallback: fallback},
			{Name: "products", URL: "http://localhost:4002/query", WS: "ws://localhost:4002/query"},
			{Name: "reviews", URL: "http://localhost:4003/query"},
		},
		PollingInterval: 30 * time.Second,
	})

	p := playground.New(playground.Config{
		PathPrefix:                      "",
		PlaygroundPath:                  playgroundURLPrefix,
		GraphqlEndpointPath:             graphqlEndpoint,
		GraphQLSubscriptionEndpointPath: graphqlEndpoint,
	})

	handlers, err := p.Handlers()
	if err != nil {
		logger.Fatal("configure handlers", log.Error(err))
		return
	}

	for i := range handlers {
		mux.Handle(handlers[i].Path, handlers[i].Handler)
	}

	enableART := true

	var gqlHandlerFactory HandlerFactoryFn = func(schema *graphql.Schema, engine *engine.ExecutionEngine) http.Handler {
		return gatewayHttp.NewGraphqlHTTPHandler(schema, engine, upgrader, logger, enableART)
	}

	gateway := NewGateway(ctx, gqlHandlerFactory, httpClient, logger)

	datasourceWatcher.Register(gateway)
	go datasourceWatcher.Run(ctx)

	gateway.Ready()

	mux.Handle("/query", gateway)

	addr := "0.0.0.0:4000"
	logger.Info("Listening",
		log.String("add", addr),
	)
	fmt.Printf("Access Playground on: http://%s%s%s\n", prettyAddr(addr), playgroundURLPrefix, playgroundURL)
	logger.Fatal("failed listening",
		log.Error(http.ListenAndServe(addr, mux)),
	)
}

func prettyAddr(addr string) string {
	return strings.Replace(addr, "0.0.0.0", "localhost", -1)
}

func main() {
	startServer()
}
