package introspection

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/pkg/astprinter"
)

func TestJSONConverter_GraphQLDocument(t *testing.T) {
	starwarsSchemaBytes, err := ioutil.ReadFile("./fixtures/starwars.golden")
	require.NoError(t, err)

	definition, report := astparser.ParseGraphqlDocumentBytes(starwarsSchemaBytes)
	if report.HasErrors() {
		t.Fatal(report)
	}

	gen := NewGenerator()
	var data Data
	gen.Generate(&definition, &report, &data)
	if report.HasErrors() {
		t.Fatal(report)
	}

	outputPretty, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)

	converter := JsonConverter{}
	buf := bytes.NewBuffer(outputPretty)
	doc, err := converter.GraphQLDocument(buf)
	assert.NoError(t, err)

	outWriter := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), outWriter)
	require.NoError(t, err)

	schemaOutputPretty := outWriter.Bytes()
	// fmt.Println(string(schemaOutputPretty))
	// ioutil.WriteFile("./fixtures/starwars_generated.graphql", schemaOutputPretty, os.ModePerm)

	// Ensure that recreated sdl is valid
	definition, report = astparser.ParseGraphqlDocumentBytes(schemaOutputPretty)
	if report.HasErrors() {
		t.Fatal(report)
	}

	// Check that recreated sdl is the same as original
	goldie.Assert(t, "starwars", schemaOutputPretty)
	if t.Failed() {
		fixture, err := ioutil.ReadFile("./fixtures/starwars.golden")
		require.NoError(t, err)

		diffview.NewGoland().DiffViewBytes("startwars", fixture, schemaOutputPretty)
	}
}

func BenchmarkJsonConverter_GraphQLDocument(b *testing.B) {
	introspectedBytes, err := ioutil.ReadFile("./testdata/swapi_introspection_response.json")
	require.NoError(b, err)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf := bytes.NewBuffer(introspectedBytes)
		converter := JsonConverter{}
		_, _ = converter.GraphQLDocument(buf)
	}
}
