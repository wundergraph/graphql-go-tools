package imports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jensneuse/diffview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
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

	goldie.Assert(t, "scanner_result", dump, true)
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/scanner_result.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("scanner_result", fixture, dump)
	}
}

func TestScanner_ScanRegex(t *testing.T) {
	scanner := Scanner{}
	file, err := scanner.ScanRegex("./testdata/regexonly/*.graphql")
	if err != nil {
		t.Fatal(err)
	}

	dump, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, "scanner_regex", dump, true)
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/scanner_regex.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("scanner_regex", fixture, dump)
	}

	buf := bytes.Buffer{}
	err = file.render(false, &buf)
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, "scanner_regex_render", buf.Bytes())
	if t.Failed() {
		fixture, err := os.ReadFile("./fixtures/scanner_regex_render.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("scanner_regex_render", fixture, buf.Bytes())
	}
}

func TestScannerImportCycle(t *testing.T) {
	scanner := Scanner{}
	file, err := scanner.ScanFile("./testdata/import_cycle.graphql")
	_ = file
	require.Error(t, err)

	cycleFilePath := filepath.Join("testdata", "/cycle/a/a.graphql")
	assert.Equal(t, fmt.Sprintf("file forms import cycle: %s", cycleFilePath), err.Error())
}
