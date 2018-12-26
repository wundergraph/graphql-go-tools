package parser

import (
	"encoding/json"
	"fmt"
	. "github.com/franela/goblin"
	"github.com/jensneuse/diffview"
	. "github.com/onsi/gomega"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"log"
	"os"
	"testing"
)

func TestParser(t *testing.T) {

	g := Goblin(t)
	RegisterFailHandler(func(m string, _ ...int) { g.Fail(m) })

	g.Describe("ParseTypeSystemDefinition()", func() {
		tests := []struct {
			it             string
			inputFileName  string
			goldenFileName string
		}{
			{
				it:             "should parse the starwars schema",
				inputFileName:  "../../starwars.schema.graphql",
				goldenFileName: "parser_starwars_typesystemdefinition",
			},
		}

		for _, test := range tests {
			test := test
			g.It(test.it, func() {
				parser := NewParser()

				starwarsSchema, err := os.Open(test.inputFileName)
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

				goldie.Assert(t, test.goldenFileName, jsonBytes)
				if t.Failed() {

					fixtureData, err := ioutil.ReadFile(fmt.Sprintf("./fixtures/%s.golden", test.goldenFileName))
					if err != nil {
						log.Fatal(err)
					}

					diffview.NewGoland().DiffViewBytes(test.goldenFileName, fixtureData, jsonBytes)
				}
			})
		}
	})

	g.Describe("ParseExecutableDefinition()", func() {
		tests := []struct {
			it             string
			inputFileName  string
			goldenFileName string
		}{
			{
				it:             "should parse the introspection query",
				inputFileName:  "./testdata/introspectionquery.graphql",
				goldenFileName: "introspectionquery",
			},
		}

		for _, test := range tests {
			test := test
			g.It(test.it, func() {

				inputFile, err := os.Open(test.inputFileName)
				if err != nil {
					g.Fail(err)
				}

				defer inputFile.Close()

				parser := NewParser()
				executableDefinition, err := parser.ParseExecutableDefinition(inputFile)
				Expect(err).To(BeNil())

				jsonBytes, err := json.MarshalIndent(executableDefinition, "", "  ")
				if err != nil {
					g.Fail(err)
				}

				goldie.Assert(t, test.goldenFileName, jsonBytes)
			})
		}
	})
}

func BenchmarkParser(b *testing.B) {

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {

		b.StopTimer()

		parser := NewParser()

		introspectionQueryFile, err := os.Open("./testdata/introspectionquery.graphql")
		if err != nil {
			b.Fatal(err)
		}

		b.StartTimer()

		executableDefinition, err := parser.ParseExecutableDefinition(introspectionQueryFile)
		if err != nil {
			introspectionQueryFile.Close()
			b.Fatal(err)
		}

		_ = executableDefinition
		introspectionQueryFile.Close()
	}
}
