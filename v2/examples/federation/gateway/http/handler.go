package http

import (
	"net/http"

	"github.com/gobwas/ws"
	log "github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphql"
)

const (
	httpHeaderUpgrade string = "Upgrade"
)

func NewGraphqlHTTPHandler(
	schema *graphql.Schema,
	engine *graphql.ExecutionEngineV2,
	upgrader *ws.HTTPUpgrader,
	logger log.Logger,
) http.Handler {
	return &GraphQLHTTPRequestHandler{
		schema:     schema,
		engine:     engine,
		wsUpgrader: upgrader,
		log:        logger,
	}
}

type GraphQLHTTPRequestHandler struct {
	log        log.Logger
	wsUpgrader *ws.HTTPUpgrader
	engine     *graphql.ExecutionEngineV2
	schema     *graphql.Schema
}

func (g *GraphQLHTTPRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isUpgrade := g.isWebsocketUpgrade(r)
	if isUpgrade {
		err := g.upgradeWithNewGoroutine(w, r)
		if err != nil {
			g.log.Error("GraphQLHTTPRequestHandler.ServeHTTP",
				log.Error(err),
			)
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}
	g.handleHTTP(w, r)
}

func (g *GraphQLHTTPRequestHandler) upgradeWithNewGoroutine(w http.ResponseWriter, r *http.Request) error {
	conn, _, _, err := g.wsUpgrader.Upgrade(r, w)
	if err != nil {
		return err
	}
	g.handleWebsocket(r.Context(), conn)
	return nil
}

func (g *GraphQLHTTPRequestHandler) isWebsocketUpgrade(r *http.Request) bool {
	for _, header := range r.Header[httpHeaderUpgrade] {
		if header == "websocket" {
			return true
		}
	}
	return false
}
