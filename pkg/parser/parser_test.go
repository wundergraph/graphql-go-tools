package parser

import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"log"
	"strings"
	"testing"
)

func (p ParsedDefinitions) initEmptySlices() ParsedDefinitions {

	if p.OperationDefinitions == nil {
		p.OperationDefinitions = []document.OperationDefinition{}
	}
	if p.FragmentDefinitions == nil {
		p.FragmentDefinitions = []document.FragmentDefinition{}
	}
	if p.VariableDefinitions == nil {
		p.VariableDefinitions = []document.VariableDefinition{}
	}
	if p.Fields == nil {
		p.Fields = []document.Field{}
	}
	if p.InlineFragments == nil {
		p.InlineFragments = []document.InlineFragment{}
	}
	if p.FragmentSpreads == nil {
		p.FragmentSpreads = []document.FragmentSpread{}
	}
	if p.Arguments == nil {
		p.Arguments = []document.Argument{}
	}
	if p.Directives == nil {
		p.Directives = []document.Directive{}
	}
	if p.EnumTypeDefinitions == nil {
		p.EnumTypeDefinitions = []document.EnumTypeDefinition{}
	}
	if p.EnumValuesDefinitions == nil {
		p.EnumValuesDefinitions = []document.EnumValueDefinition{}
	}
	if p.FieldDefinitions == nil {
		p.FieldDefinitions = []document.FieldDefinition{}
	}
	if p.InputValueDefinitions == nil {
		p.InputValueDefinitions = []document.InputValueDefinition{}
	}
	if p.InputObjectTypeDefinitions == nil {
		p.InputObjectTypeDefinitions = []document.InputObjectTypeDefinition{}
	}
	if p.DirectiveDefinitions == nil {
		p.DirectiveDefinitions = []document.DirectiveDefinition{}
	}

	if p.InterfaceTypeDefinitions == nil {
		p.InterfaceTypeDefinitions = []document.InterfaceTypeDefinition{}
	}

	if p.ObjectTypeDefinitions == nil {
		p.ObjectTypeDefinitions = []document.ObjectTypeDefinition{}
	}

	if p.ScalarTypeDefinitions == nil {
		p.ScalarTypeDefinitions = []document.ScalarTypeDefinition{}
	}

	if p.UnionTypeDefinitions == nil {
		p.UnionTypeDefinitions = []document.UnionTypeDefinition{}
	}

	return p
}

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
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(executableDefinition, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes = append(jsonBytes, []byte("\n\n")...)

	parserData, err := json.MarshalIndent(parser, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes = append(jsonBytes, parserData...)

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
