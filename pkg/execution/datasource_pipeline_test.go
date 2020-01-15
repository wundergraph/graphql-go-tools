package execution

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/pipeline/pkg/pipe"
	log "github.com/jensneuse/abstractlogger"
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

	source := PipelineDataSource{
		log:      log.NoopLogger,
		pipeline: pipeline,
	}

	args := []ResolvedArgument{
		{
			Key:   literal.INPUT_JSON,
			Value: []byte(`{"foo":"bar"}`),
		},
	}

	var out bytes.Buffer
	source.Resolve(Context{}, args, &out)

	got := out.String()
	want := `{"foo":"bar"}`

	if want != got {
		t.Fatalf("want: %s\ngot: %s\n", want, got)
	}
}
