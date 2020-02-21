package execution

import (
	"bytes"
	"github.com/cespare/xxhash"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

func TestHandler_RenderGraphQLDefinitions(t *testing.T) {
	handler, err := NewHandler(nil, nil, log.NoopLogger)
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
	handler, err := NewHandler(nil, nil, log.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}
	request := GraphqlRequest{
		Variables: []byte(`{"foo":"bar"}`),
	}

	extra := []byte(`{"request":{"headers":{"Authorization":"Bearer foo123"}}}`)

	variables, extraArguments := handler.VariablesFromJson(request.Variables, extra)

	for key, value := range map[string]string{
		"foo":     "bar",
		"request": `{"headers":{"Authorization":"Bearer foo123"}}`,
	} {
		got := string(variables[xxhash.Sum64String(key)])
		want := value
		if got != want {
			t.Errorf("want {{ %s }}, got: {{ %s }}'", want, got)
		}
	}

	if len(extraArguments) != 1 {
		t.Fatalf("want 1")
	}

	if !bytes.Equal(extraArguments[0].(*ContextVariableArgument).Name, []byte("request")) {
		t.Fatalf("unexpected")
	}
	if !bytes.Equal(extraArguments[0].(*ContextVariableArgument).VariableName, []byte("request")) {
		t.Fatalf("unexpected")
	}
}
