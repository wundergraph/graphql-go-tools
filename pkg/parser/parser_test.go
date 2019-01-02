package parser

import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	. "github.com/onsi/gomega"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"log"
	"strings"
	"testing"
)

func TestParser_Starwars(t *testing.T) {

	inputFileName := "../../starwars.schema.graphql"
	fixtureFileName := "type_system_definition_parsed_starwars"

	parser := NewParser()

	starwarsSchema, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	builder := &strings.Builder{}
	builder.Write(starwarsSchema)

	def, err := parser.ParseTypeSystemDefinition(builder.String())
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, fixtureFileName, jsonBytes)
	if t.Failed() {

		fixtureData, err := ioutil.ReadFile(fmt.Sprintf("./fixtures/%s.golden", fixtureFileName))
		if err != nil {
			log.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes(fixtureFileName, fixtureData, jsonBytes)
	}
}

func TestParser_IntrospectionQuery(t *testing.T) {

	inputFileName := "./testdata/introspectionquery.graphql"
	fixtureFileName := "type_system_definition_parsed_introspection"

	inputFileData, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	builder := &strings.Builder{}
	builder.Write(inputFileData)

	parser := NewParser()
	executableDefinition, err := parser.ParseExecutableDefinition(builder.String())
	Expect(err).To(BeNil())

	jsonBytes, err := json.MarshalIndent(executableDefinition, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, fixtureFileName, jsonBytes)
	if t.Failed() {

		fixtureData, err := ioutil.ReadFile(fmt.Sprintf("./fixtures/%s.golden", fixtureFileName))
		if err != nil {
			log.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes(fixtureFileName, fixtureData, jsonBytes)
	}
}

func BenchmarkParser(b *testing.B) {

	b.ReportAllocs()

	parser := NewParser()

	testData, err := ioutil.ReadFile("./testdata/introspectionquery.graphql")
	if err != nil {
		b.Fatal(err)
	}

	builder := &strings.Builder{}
	builder.Write(testData)

	inputString := builder.String()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		executableDefinition, err := parser.ParseExecutableDefinition(inputString)
		if err != nil {
			b.Fatal(err)
		}

		_ = executableDefinition

	}
}
