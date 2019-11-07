package http

import (
	"bytes"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jensneuse/graphql-go-tools/pkg/execution"
	"go.uber.org/zap"
	"io/ioutil"
	"net"
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

func (g *GraphQLHTTPRequestHandler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		g.log.Error("GraphQLHTTPRequestHandler.handleHTTP",
			zap.Error(err),
		)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	executor, rootNode, ctx, err := g.executionHandler.Handle(data)
	if err != nil {
		g.log.Error("executionHandler.Handle",
			zap.Error(err),
		)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	_, err = executor.Execute(ctx, rootNode, buf)
	if err != nil {
		g.log.Error("executor.Execute",
			zap.Error(err),
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	_, _ = buf.WriteTo(w)
}

func (g *GraphQLHTTPRequestHandler) handleWebsocket(conn net.Conn) {
	defer conn.Close()

	for {
		msg, op, err := wsutil.ReadClientData(conn)
		if err != nil {
			g.log.Error("GraphQLHTTPRequestHandler.handleWebsocket",
				zap.Error(err),
				zap.ByteString("message", msg),
			)
			return
		}
		g.log.Debug("GraphQLHTTPRequestHandler.handleWebsocket",
			zap.ByteString("message", msg),
			zap.String("opCode", string(op)),
		)
		/*err = wsutil.WriteServerMessage(conn, op, msg)
		if err != nil {
			// handle error
		}*/
	}
}

func (g *GraphQLHTTPRequestHandler) upgradeWithNewGoroutine(w http.ResponseWriter, r *http.Request) error {
	conn, _, _, err := g.wsUpgrader.Upgrade(r, w)
	if err != nil {
		return err
	}
	go g.handleWebsocket(conn)
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
