package execution

import (
	"bytes"
	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
	"go.uber.org/zap"
	"io/ioutil"
	"testing"
)

func TestHandler_RenderGraphQLDefinitions(t *testing.T) {
	handler, err := NewHandler(nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	buf := bytes.Buffer{}
	err = handler.RenderGraphQLDefinitions(&buf)
	if err != nil {
		t.Fatal(err)
		return
	}

	got := buf.Bytes()

	goldie.Assert(t, "render_graphql_definitions", got)

	if t.Failed() {
		want, err := ioutil.ReadFile("./fixtures/render_graphql_definitions.golden")
		if err != nil {
			panic(err)
		}
		diffview.NewGoland().DiffViewBytes("render_graphql_definitions", want, got)
	}
}
