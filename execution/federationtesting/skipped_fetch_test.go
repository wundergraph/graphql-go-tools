package federationtesting

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestSkippedFetchOnNullParent(t *testing.T) {
	// Users subgraph: returns null for the "user" field.
	usersServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"data":{"user":null}}`))
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

	const usersSDL = `type Query { user(id: ID!): User } type User @key(fields: "id") { id: ID! name: String! }`
	const reviewsSDL = `type User @key(fields: "id") { id: ID! @external reviews: [Review] } type Review { body: String! }`

	ctx := context.Background()
	factory := engine.NewFederationEngineConfigFactory(ctx, []engine.SubgraphConfiguration{
		{Name: "users", URL: usersServer.URL, SDL: usersSDL},
		{Name: "reviews", URL: reviewsServer.URL, SDL: reviewsSDL},
	})

	engineConfig, err := factory.BuildEngineConfiguration()
	require.NoError(t, err)

	eng, err := engine.NewExecutionEngine(ctx, abstractlogger.NoopLogger, engineConfig, resolve.ResolverOptions{
		MaxConcurrency: 1024,
	})
	require.NoError(t, err)

	gqlRequest := &graphql.Request{
		Query: `{ user(id: "1") { id name reviews { body } } }`,
	}

	resultWriter := graphql.NewEngineResultWriter()
	err = eng.Execute(ctx, gqlRequest, &resultWriter)
	require.NoError(t, err)

	// The user is null, so the response should reflect that without panic.
	assert.Equal(t, `{"data":{"user":null}}`, resultWriter.String())

	// The reviews subgraph must NOT have been called — the entity fetch was skipped
	// because the parent user is null.
	assert.Equal(t, int64(0), reviewsCalls.Load(), "reviews subgraph should not be called when parent user is null")
}
