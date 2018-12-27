package parser

import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	. "github.com/onsi/gomega"
	"github.com/sebdah/goldie"
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"
)

func TestParser_Starwars(t *testing.T) {

	inputFileName := "../../starwars.schema.graphql"
	fixtureFileName := "type_system_definition_parsed_starwars"

	parser := NewParser()

	starwarsSchema, err := os.Open(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	defer starwarsSchema.Close()

	def, err := parser.ParseTypeSystemDefinition(starwarsSchema)
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

	inputFile, err := os.Open(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	defer inputFile.Close()

	parser := NewParser()
	executableDefinition, err := parser.ParseExecutableDefinition(inputFile)
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

	introspectionQueryFile, err := os.Open("./testdata/introspectionquery.graphql")
	if err != nil {
		b.Fatal(err)
	}

	parser.ParseExecutableDefinition(introspectionQueryFile)

	defer introspectionQueryFile.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		b.StopTimer()
		_, err = introspectionQueryFile.Seek(0, io.SeekStart)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		executableDefinition, err := parser.ParseExecutableDefinition(introspectionQueryFile)
		if err != nil {
			b.Fatal(err)
		}

		_ = executableDefinition

	}
}
