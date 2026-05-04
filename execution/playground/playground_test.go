package playground

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		assert.Equal(t, "/graphql", playground.data.EndpointURL)
		assert.Equal(t, "/graphqlws", playground.data.SubscriptionEndpointURL)
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

		assert.Equal(t, "/playground", handlers[0].Path)
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
