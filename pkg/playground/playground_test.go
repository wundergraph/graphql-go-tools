package playground

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
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
			fixture, err := ioutil.ReadFile("./fixtures/handlers.golden")
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

}
