package http

import (
	"encoding/json"
	"github.com/gobwas/ws"
	"github.com/jensneuse/graphql-go-tools/pkg/execution"
	"go.uber.org/zap"
	"io"
	"net/http"
)

func NewGraphqlHTTPHandlerFunc(executionHandler *execution.Handler, logger *zap.Logger, upgrader *ws.HTTPUpgrader) http.Handler {
	return &GraphQLHTTPRequestHandler{
		log:              logger,
		executionHandler: executionHandler,
		wsUpgrader:       upgrader,
	}
}

type GraphQLHTTPRequestHandler struct {
	log              *zap.Logger
	executionHandler *execution.Handler
	wsUpgrader       *ws.HTTPUpgrader
}

func (g *GraphQLHTTPRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isUpgrade := g.isWebsocketUpgrade(r)
	if isUpgrade {
		err := g.upgradeWithNewGoroutine(w, r)
		if err != nil {
			g.log.Error("GraphQLHTTPRequestHandler.ServeHTTP",
				zap.Error(err),
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
	go g.handleWebsocket(r, conn)
	return nil
}

func (g *GraphQLHTTPRequestHandler) isWebsocketUpgrade(r *http.Request) bool {
	for _, header := range r.Header["Upgrade"] {
		if header == "websocket" {
			return true
		}
	}
	return false
}

func (g *GraphQLHTTPRequestHandler) extraVariables(r *http.Request, out io.Writer) error {
	headers := map[string]string{}
	for key := range r.Header {
		headers[key] = r.Header.Get(key)
	}

	extra := map[string]interface{}{
		"request": map[string]interface{}{
			"uri":     r.RequestURI,
			"method":  r.Method,
			"host":    r.Host,
			"headers": headers,
		},
	}

	return json.NewEncoder(out).Encode(extra)
}
