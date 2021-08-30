package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	log "github.com/jensneuse/abstractlogger"

	graphqlDataSource "github.com/jensneuse/graphql-go-tools/pkg/engine/datasource/graphql_datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql/federation"

	"github.com/jensneuse/graphql-go-tools/examples/federation/gateway/authorization"
)

type DataSourceObserver interface {
	UpdateDataSources(dataSourceConfig []graphqlDataSource.Configuration)
}

type DataSourceSubject interface {
	Register(observer DataSourceObserver)
}

type HandlerFactory interface {
	Make(schema *graphql.Schema, engine *graphql.ExecutionEngineV2) http.Handler
}

type HandlerFactoryFn func(schema *graphql.Schema, engine *graphql.ExecutionEngineV2) http.Handler

func (h HandlerFactoryFn) Make(schema *graphql.Schema, engine *graphql.ExecutionEngineV2) http.Handler {
	return h(schema, engine)
}

func NewGateway(
	gqlHandlerFactory HandlerFactory,
	httpClient *http.Client,
	logger log.Logger,
	currentRoles []string,
) *Gateway {
	return &Gateway{
		gqlHandlerFactory: gqlHandlerFactory,
		httpClient:        httpClient,
		logger:            logger,

		mu:        &sync.Mutex{},
		readyCh:   make(chan struct{}),
		readyOnce: &sync.Once{},

		currentRoles: currentRoles,
	}
}

type Gateway struct {
	gqlHandlerFactory HandlerFactory
	httpClient        *http.Client
	logger            log.Logger
	currentRoles []string

	gqlHandler http.Handler

	mu         *sync.Mutex
	readyCh   chan struct{}

	readyOnce *sync.Once
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	handler := g.gqlHandler
	g.mu.Unlock()

	handler.ServeHTTP(w, r)
}

func (g *Gateway) Ready() {
	<-g.readyCh
}

// Error handling is not finished.
func (g *Gateway) UpdateDataSources(newDataSourcesConfig []graphqlDataSource.Configuration) {
	ctx := context.Background()
	engineConfigFactory := federation.NewEngineConfigV2Factory(g.httpClient, newDataSourcesConfig...)

	schema, err := engineConfigFactory.MergedSchema()
	if err != nil {
		g.logger.Error("get schema:", log.Error(err))
		return
	}

	res, err := schema.Validate()
	if err != nil {
		g.logger.Error("validate schema:", log.Error(err))
		return
	}

	if !res.Valid {
		for i:= 0; i < res.Errors.Count(); i++ {
			g.logger.Info("validate error", log.Error(res.Errors.ErrorByIndex(i)))
		}
	}

	normRes, err := schema.Normalize()
	if err != nil {
		g.logger.Error("normalize schema:", log.Error(err))
		return
	}

	if !normRes.Successful {
		g.logger.Error("normalize schema:", log.Error(res.Errors))
		return
	}

	datasourceConfig, err := engineConfigFactory.EngineV2Configuration()
	if err != nil {
		g.logger.Error("get engine config: %v", log.Error(err))
		return
	}

	engine, err := graphql.NewExecutionEngineV2(ctx, g.logger, datasourceConfig)
	if err != nil {
		g.logger.Error("create engine: %v", log.Error(err))
		return
	}

	engine.UseOperation(authorization.NewMiddleware(g.checkUserRole, schema.ASTDocument()))

	g.mu.Lock()
	g.gqlHandler = g.gqlHandlerFactory.Make(schema, engine)
	g.mu.Unlock()

	g.readyOnce.Do(func() { close(g.readyCh) })
}

func (g *Gateway) checkUserRole(ctx context.Context, requiredRoles []string) error {
	// must be http middleware for authentication http response (creates user session from token (auth header or cookie) and keeps it in context)
	// must be websocket init function for authentication ws connect (create user session from token (ws init message payload) and keeps it in context)
	requiredHighestRole := highestRole(requiredRoles)
	userHighestRole := highestRole(g.getUserRoles(ctx))

	if lessRoles(userHighestRole, requiredHighestRole) {
		return fmt.Errorf("required role: %s, got: %s", requiredHighestRole, userHighestRole)
	}

	return nil
}

func (g *Gateway) getUserRoles(ctx context.Context) []string {
	// Here could be getting of the user session from context
	return g.currentRoles
}
