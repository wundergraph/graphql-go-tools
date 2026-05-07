package engine

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestSkippedFetchOnNullParent(t *testing.T) {
	// Users subgraph: returns null for the "user" field.
	usersServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"data":{"me":null}}`))
	}))
	t.Cleanup(usersServer.Close)

	// Reviews subgraph: tracks all requests. Should never be called at query time
	// because the user is null and the entity fetch must be skipped.
	var reviewsCalls atomic.Int64
	reviewsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reviewsCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"data":{"_entities":[]}}`))
	}))
	t.Cleanup(reviewsServer.Close)

	ctx := t.Context()
	factory := NewFederationEngineConfigFactory(ctx)

	cfgData, err := os.ReadFile("testdata/config_factory_federation/config.json")
	require.NoError(t, err)

	cfgData = bytes.ReplaceAll(cfgData, []byte("http://user.service"), []byte(usersServer.URL))
	cfgData = bytes.ReplaceAll(cfgData, []byte("http://review.service"), []byte(reviewsServer.URL))

	// Build the engine configuration using the router config
	var rc1 nodev1.RouterConfig
	assert.NoError(t, protojson.Unmarshal(cfgData, &rc1))
	engineConfig, err := factory.BuildEngineConfiguration(&rc1)
	require.NoError(t, err)

	eng, err := NewExecutionEngine(ctx, abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	require.NoError(t, err)

	gqlRequest := &graphql.Request{
		Query: `{ me { id username reviews { body } } }`,
	}

	resultWriter := graphql.NewEngineResultWriter()
	err = eng.Execute(ctx, gqlRequest, &resultWriter)
	require.NoError(t, err)

	// The user is null, so the response should reflect that without panic.
	assert.Equal(t, `{"data":{"me":null}}`, resultWriter.String())

	// The reviews subgraph must NOT have been called — the entity fetch was skipped
	// because the parent user is null.
	assert.Equal(t, int64(0), reviewsCalls.Load(), "reviews subgraph should not be called when parent user is null")
}
