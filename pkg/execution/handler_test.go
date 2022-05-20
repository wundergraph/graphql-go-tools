package execution

import (
	"bytes"
	"testing"

	"github.com/cespare/xxhash/v2"

	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
)

func TestHandler_VariablesFromRequest(t *testing.T) {
	request := GraphqlRequest{
		Variables: []byte(`{"foo":"bar"}`),
	}

	extra := []byte(`{"request":{"headers":{"Authorization":"Bearer foo123"}}}`)

	variables, extraArguments := VariablesFromJson(request.Variables, extra)

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

	if !bytes.Equal(extraArguments[0].(*datasource.ContextVariableArgument).Name, []byte("request")) {
		t.Fatalf("unexpected")
	}
	if !bytes.Equal(extraArguments[0].(*datasource.ContextVariableArgument).VariableName, []byte("request")) {
		t.Fatalf("unexpected")
	}
}
