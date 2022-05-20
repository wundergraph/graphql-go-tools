package introspection

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/jensneuse/diffview"
	"github.com/sebdah/goldie"
	"github.com/wundergraph/graphql-go-tools/pkg/astparser"
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

	goldie.Assert(t, "starwars_introspected", outputPretty)
	if t.Failed() {
		fixture, err := ioutil.ReadFile("./fixtures/starwars_introspected.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("startwars_introspected", fixture, outputPretty)
	}
}

func TestGenerator_Generate_Interfaces_Implementing_Interfaces(t *testing.T) {
	interfacesSchemaBytes, err := ioutil.ReadFile("./testdata/interfaces_implementing_interfaces.graphql")
	if err != nil {
		panic(err)
	}

	definition, report := astparser.ParseGraphqlDocumentBytes(interfacesSchemaBytes)
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

	goldie.Assert(t, "interfaces_implementing_interfaces", outputPretty)
	if t.Failed() {
		fixture, err := ioutil.ReadFile("./fixtures/interfaces_implementing_interfaces.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("interfaces_implements_interfaces", fixture, outputPretty)
	}
}
