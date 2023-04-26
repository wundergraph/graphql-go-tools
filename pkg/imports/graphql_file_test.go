package imports

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
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

	goldie.Assert(t, "render_result", dump)
	if t.Failed() {
		fixture, err := ioutil.ReadFile("./fixtures/render_result.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("render_result", fixture, dump)
	}
}
