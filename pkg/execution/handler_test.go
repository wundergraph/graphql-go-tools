package execution

import (
	"bytes"
	"github.com/cespare/xxhash"
	log "github.com/jensneuse/abstractlogger"
	"testing"
)

func TestHandler_VariablesFromRequest(t *testing.T) {

	base, err := NewBaseDataSourcePlanner(nil, PlannerConfiguration{}, log.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(base, nil)

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
