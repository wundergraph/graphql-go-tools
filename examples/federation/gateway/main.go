package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
	"github.com/jensneuse/graphql-go-tools/pkg/playground"
	"go.uber.org/zap"

	http2 "github.com/jensneuse/federation-example/gateway/http"
)

// It's just a simple example of graphql federation gateway server, it's NOT a production ready code.
//
func logger() log.Logger {
	logger, err := zap.NewDevelopmentConfig().Build()
	if err != nil {
		panic(err)
	}

	return log.NewZapLogger(logger, log.DebugLevel)
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

	schema, err := graphql.NewSchemaFromString(baseSchema)
	if err != nil {
		logger.Fatal("create schema", log.Error(err))
	}
	// Create and run subscription manager with websocket stream
	subscriptionManager := newSubscriptionManager()
	go subscriptionManager.Run(ctx.Done())
	// Configure execution engine
	engine, err := newEngine(logger, schema, subscriptionManager)
	if err != nil {
		logger.Fatal("create engine", log.Error(err))
	}
	// Create graphql handler
	gqlHandler := http2.NewGraphqlHTTPHandler(schema, engine, upgrader, logger)

	mux.Handle("/query", gqlHandler)

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
