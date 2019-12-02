package execution

import (
	"bytes"
	"github.com/cespare/xxhash"
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

	/*
		This is necessary because goldie Assert uses AssertWithTemplate internally.
		That is, goldie uses go templating and because the text contains templating syntax itself the assertion will fail without showing an error.
	*/
	goldie.AssertWithTemplate(t, "render_graphql_definitions", struct {
		Id string
	}{
		Id: "{{ .Id }}",
	}, got)

	if t.Failed() {
		want, err := ioutil.ReadFile("./fixtures/render_graphql_definitions.golden")
		if err != nil {
			panic(err)
		}
		diffview.NewGoland().DiffViewBytes("render_graphql_definitions", want, got)
	}
}

func TestHandler_VariablesFromRequest(t *testing.T) {
	handler, err := NewHandler(nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	request := GraphqlRequest{
		Variables: []byte(`{"foo":"bar"}`),
	}
	variables := handler.VariablesFromRequest(request)

	for key,value := range map[string]string{
		"foo": "bar",
	}{
		got := string(variables[xxhash.Sum64String(key)])
		want := value
		if got != want{
			t.Errorf("want {{ %s }}, got: {{ %s }}'",want,got)
		}
	}
}