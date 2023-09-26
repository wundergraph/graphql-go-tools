package imports

import (
	"bytes"
	"os"
	"testing"

	"github.com/jensneuse/diffview"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
)

func TestGraphQLFile_Render(t *testing.T) {
	scanner := Scanner{}
	file, err := scanner.ScanFile("testdata/schema.graphql")
	if err != nil {
		t.Fatal(err)
	}

	out := bytes.Buffer{}
	err = file.Render(true, &out)
	if err != nil {
		t.Fatal(err)
	}

	dump := out.Bytes()

	goldie.Assert(t, "render_result", dump, true)
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/render_result.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("render_result", fixture, dump)
	}
}
