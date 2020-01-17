package playground

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
)

func TestConfigureHandlers(t *testing.T) {
	config := Config{
		PathPrefix:                      "",
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
}
