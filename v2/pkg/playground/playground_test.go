package playground

import (
	"bytes"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/jensneuse/diffview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func TestNew(t *testing.T) {
	t.Run("should prefix asset paths with slash (/) even when prefix path is empty", func(t *testing.T) {
		config := Config{
			PathPrefix:                      "",
			PlaygroundPath:                  "/playground",
			GraphqlEndpointPath:             "/graphql",
			GraphQLSubscriptionEndpointPath: "/graphqlws",
		}

		playground := New(config)

		assert.Equal(t, playground.data.CssURL, "playground/playground.css")
		assert.Equal(t, playground.data.JsURL, "playground/playground.js")
		assert.Equal(t, playground.data.FavIconURL, "playground/favicon.png")
		assert.Equal(t, playground.data.LogoURL, "playground/logo.png")
	})
}

func TestConfigureHandlers(t *testing.T) {
	t.Run("should create handlers", func(t *testing.T) {
		config := Config{
			PathPrefix:                      "/",
			PlaygroundPath:                  "/playground",
			GraphqlEndpointPath:             "/graphql",
			GraphQLSubscriptionEndpointPath: "/graphqlws",
		}

		p := New(config)

		handlers, err := p.Handlers()
		if err != nil {
			t.Fatal(err)
		}

		for i := range handlers {
			handlers[i].Handler = nil
		}

		var out bytes.Buffer
		spew.Fdump(&out, handlers)

		goldie.Assert(t, "handlers", out.Bytes())
		if t.Failed() {
			fixture, err := os.ReadFile("./fixtures/handlers.golden")
			if err != nil {
				t.Fatal(err)
			}

			diffview.NewGoland().DiffViewBytes("handlers", fixture, out.Bytes())
		}
	})

	t.Run("should respect trailing slash for playground path", func(t *testing.T) {
		config := Config{
			PathPrefix:                      "/",
			PlaygroundPath:                  "/playground/",
			GraphqlEndpointPath:             "/graphql",
			GraphQLSubscriptionEndpointPath: "/graphqlws",
		}

		p := New(config)
		handlers, err := p.Handlers()
		require.NoError(t, err)
		assert.Equal(t, handlers[0].Path, "/playground/")
	})

	t.Run("should be / when path prefix and playground path /,empty combinations", func(t *testing.T) {
		combinations := [][]string{
			{"/", "/"},
			{"", "/"},
			{"/", ""},
			{"", ""},
		}

		for _, combination := range combinations {
			config := Config{
				PathPrefix:                      combination[0],
				PlaygroundPath:                  combination[1],
				GraphqlEndpointPath:             "/graphql",
				GraphQLSubscriptionEndpointPath: "/graphqlws",
			}

			p := New(config)
			handlers, err := p.Handlers()
			require.NoError(t, err)
			assert.Equal(t, "/", handlers[0].Path)
		}
	})
}
