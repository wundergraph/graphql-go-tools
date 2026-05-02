package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"
	"go.uber.org/zap"

	"github.com/wundergraph/graphql-go-tools/examples/federation/gateway"
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

func main() {
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

	configFileContent, err := os.ReadFile("config.json")
	if err != nil {
		logger.Fatal("read config", log.Error(err))
		return
	}

	gw, err := gateway.NewGateway(ctx, configFileContent, httpClient, logger, true)
	if err != nil {
		logger.Fatal("create gateway", log.Error(err))
		return
	}

	mux.Handle("/query", gw)

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
