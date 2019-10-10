package introspection

import (
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

func TestGenerator_Generate(t *testing.T) {
	starwarsSchemaBytes, err := ioutil.ReadFile("./testdata/starwars.schema.graphql")
	if err != nil {
		panic(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, "startwars_introspected", outputPretty)
}
