package playground

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

func TestConfigureHandlers(t *testing.T) {

	config := Config{
		URLPrefix:                      "",
		PlaygroundURL:                  "/playground",
		GraphqlEndpointURL:             "/graphql",
		GraphQLSubscriptionEndpointURL: "/graphqlws",
	}

	var handlers Handlers

	err := ConfigureHandlers(config,&handlers)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	spew.Fdump(&out,handlers)

	goldie.Assert(t,"handlers",out.Bytes())
	if t.Failed() {
		fixture, err := ioutil.ReadFile("./fixtures/handlers.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("handlers", fixture, out.Bytes())
	}
}
