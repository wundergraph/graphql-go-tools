// Package http handles GraphQL HTTP Requests including WebSocket Upgrades.
package http

import (
	"bytes"
	"net/http"
	"strconv"

	log "github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

const (
	httpHeaderContentType          string = "Content-Type"
	httpContentTypeApplicationJson string = "application/json"

	// Cache stats headers - used for testing L1/L2 cache behavior
	httpHeaderCacheL1Hits   string = "X-Cache-L1-Hits"
	httpHeaderCacheL1Misses string = "X-Cache-L1-Misses"
	httpHeaderCacheL2Hits   string = "X-Cache-L2-Hits"
	httpHeaderCacheL2Misses string = "X-Cache-L2-Misses"
)

func (g *GraphQLHTTPRequestHandler) handleHTTP(w http.ResponseWriter, r *http.Request) {
	var err error

	var gqlRequest graphql.Request
	if err = graphql.UnmarshalHttpRequest(r, &gqlRequest); err != nil {
		g.log.Error("UnmarshalHttpRequest", log.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var opts []engine.ExecutionOptions

	if g.enableART {
		tracingOpts := resolve.TraceOptions{
			Enable:                                 true,
			ExcludePlannerStats:                    false,
			ExcludeRawInputData:                    false,
			ExcludeInput:                           false,
			ExcludeOutput:                          false,
			ExcludeLoadStats:                       false,
			EnablePredictableDebugTimings:          true,
			Debug:                                  true,
			IncludeTraceOutputInResponseExtensions: true,
		}

		opts = append(opts, engine.WithRequestTraceOptions(tracingOpts))
	}

	if g.subgraphHeadersBuilder != nil {
		opts = append(opts, engine.WithSubgraphHeadersBuilder(g.subgraphHeadersBuilder))
	}

	// Add caching options if L1 or L2 cache is enabled
	if g.cachingOptions.EnableL1Cache || g.cachingOptions.EnableL2Cache {
		opts = append(opts, engine.WithCachingOptions(g.cachingOptions))
	}

	if g.debugMode {
		opts = append(opts, engine.WithDebugMode())
	}

	// Capture cache stats for debugging/testing
	var cacheStats resolve.CacheStatsSnapshot
	opts = append(opts, engine.WithCacheStatsOutput(&cacheStats))

	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	resultWriter := graphql.NewEngineResultWriterFromBuffer(buf)
	if err = g.engine.Execute(r.Context(), &gqlRequest, &resultWriter, opts...); err != nil {
		g.log.Error("engine.Execute", log.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add(httpHeaderContentType, httpContentTypeApplicationJson)

	// Add cache stats headers for debugging/testing
	w.Header().Add(httpHeaderCacheL1Hits, strconv.FormatInt(cacheStats.L1Hits, 10))
	w.Header().Add(httpHeaderCacheL1Misses, strconv.FormatInt(cacheStats.L1Misses, 10))
	w.Header().Add(httpHeaderCacheL2Hits, strconv.FormatInt(cacheStats.L2Hits, 10))
	w.Header().Add(httpHeaderCacheL2Misses, strconv.FormatInt(cacheStats.L2Misses, 10))

	w.WriteHeader(http.StatusOK)
	if _, err = w.Write(buf.Bytes()); err != nil {
		g.log.Error("write response", log.Error(err))
		return
	}
}
