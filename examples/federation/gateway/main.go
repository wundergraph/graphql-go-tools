package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"
	graphqlDataSource "github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/federation"
	"github.com/jensneuse/graphql-go-tools/pkg/playground"
	"go.uber.org/zap"

	http2 "federation/gateway/http"
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

	httpClient := http.DefaultClient

	mux := NewRouterSwapper()
	datasourceWatcher := NewDatasourceWatcher(httpClient, DatasourceWatcherConfig{
		Services: []ServiceConfig{
			{Name: "accounts", URL: "http://localhost:4001/query"},
			{Name: "products", URL: "http://localhost:4002/query"},
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

	mux.Register(func(mux *http.ServeMux) {
		for i := range handlers {
			mux.Handle(handlers[i].Path, handlers[i].Handler)
		}
	})

	var dataSourceConfig []graphqlDataSource.Configuration
	var dataSourceMu sync.Mutex

	datasourceWatcher.Register(UpdateDatasourceHandlerFn(func(newDataSourceConfig ...graphqlDataSource.Configuration) {
		dataSourceMu.Lock()
		dataSourceConfig = newDataSourceConfig
		dataSourceMu.Unlock()

		mux.Swap()
	}))

	readyCh := make(chan struct{})
	var readyOnce sync.Once

	mux.Register(func(mux *http.ServeMux) {
		dataSourceMu.Lock()
		newDataSourceConfig := dataSourceConfig
		dataSourceMu.Unlock()

		engineConfigFactory := federation.NewEngineConfigV2Factory(httpClient, newDataSourceConfig...)
		schema, err := engineConfigFactory.Schema()
		if err != nil {
			logger.Fatal("failed to get schema", log.Error(err))
		}
		datasourceConfig, err := engineConfigFactory.EngineV2Configuration()
		if err != nil {
			logger.Fatal("failed to get engine config", log.Error(err))
		}

		engine, err := graphql.NewExecutionEngineV2(logger, datasourceConfig)
		if err != nil {
			logger.Fatal("failed to create engine", log.Error(err))
		}

		gqlHandler := http2.NewGraphqlHTTPHandler(schema, engine, upgrader, logger)
		mux.Handle(graphqlEndpoint, gqlHandler)

		readyOnce.Do(func() { close(readyCh) })
	})

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		datasourceWatcher.Start(ctx)
		wg.Done()
	}()

	<- readyCh

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
