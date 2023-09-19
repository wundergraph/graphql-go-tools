// Package http handles GraphQL HTTP Requests including WebSocket Upgrades.
package http

import (
	"bytes"
	"net/http"

	log "github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/graphql"
)

const (
	httpHeaderContentType          string = "Content-Type"
	httpContentTypeApplicationJson string = "application/json"
)

func (g *GraphQLHTTPRequestHandler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	var err error

	var gqlRequest graphql.Request
	if err = graphql.UnmarshalHttpRequest(r, &gqlRequest); err != nil {
		g.log.Error("UnmarshalHttpRequest", log.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)
	if err = g.engine.Execute(r.Context(), &gqlRequest, &resultWriter); err != nil {
		g.log.Error("engine.Execute", log.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add(httpHeaderContentType, httpContentTypeApplicationJson)
	w.WriteHeader(http.StatusOK)
	if _, err = w.Write(buf.Bytes()); err != nil {
		g.log.Error("write response", log.Error(err))
		return
	}
}
