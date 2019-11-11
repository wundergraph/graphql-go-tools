package imports

import (
	"encoding/json"
	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

func TestScanner(t *testing.T) {
	scanner := Scanner{}
	file, err := scanner.ScanFile("./testdata/schema.graphql")
	if err != nil {
		t.Fatal(err)
	}

	dump, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, "scanner_result", dump)
	if t.Failed() {
		fixture, err := ioutil.ReadFile("./fixtures/scanner_result.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("scanner_result", fixture, dump)
	}
}

func TestScannerImportCycle(t *testing.T) {
	scanner := Scanner{}
	_, err := scanner.ScanFile("./testdata/import_cycle.graphql")
	if err == nil {
		t.Fatal("want err")
	}
	want := "file forms import cycle: testdata/cycle/a/a.graphql"
	got := err.Error()
	if want != got {
		t.Fatalf("want err:\n\"%s\"\ngot:\n\"%s\"\n", want, got)
	}
}
