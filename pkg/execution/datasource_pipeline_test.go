package execution

import (
	"bytes"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/pipeline/pkg/pipe"
	"os"
	"testing"
)

func TestPipelineDataSource_Resolve(t *testing.T) {

	configFile, err := os.Open("./testdata/simple_pipeline.json")
	if err != nil {
		t.Fatal(err)
	}

	defer configFile.Close()

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
	_,err = source.Resolve(Context{}, args, &out)
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	want := `{"foo":"bar"}`

	if want != got {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}
