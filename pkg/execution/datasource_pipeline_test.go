package execution

import (
	"bytes"
	"os"
	"testing"

	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/pipeline/pkg/pipe"

	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
)

func TestPipelineDataSource_Resolve(t *testing.T) {

	configFile, err := os.Open("./testdata/simple_pipeline.json")
	if err != nil {
		t.Fatal(err)
	}

	defer func(configFile *os.File) {
		err := configFile.Close()
		if err != nil {
			t.Fatalf("Failed to close configFile: %s", err)
		}
	}(configFile)

	var pipeline pipe.Pipeline
	err = pipeline.FromConfig(configFile)
	if err != nil {
		t.Fatal(err)
	}

	source := datasource.PipelineDataSource{
		Log:      log.NoopLogger,
		Pipeline: pipeline,
	}

	args := ResolvedArgs{
		{
			Key:   literal.INPUT_JSON,
			Value: []byte(`{"foo":"bar"}`),
		},
	}

	var out bytes.Buffer
	_, err = source.Resolve(Context{}, args, &out)
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	want := `{"foo":"bar"}`

	if want != got {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}
