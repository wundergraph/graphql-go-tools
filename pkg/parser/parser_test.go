package parser

import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"log"
	"testing"
)

func TestParser(t *testing.T) {
	t.Run("newErrInvalidType", func(t *testing.T) {
		want := "parser:a:invalidType - expected 'b', got 'c' @ 1:3-2:4"
		got := newErrInvalidType(position.Position{1, 2, 3, 4}, "a", "b", "c").Error()

		if want != got {
			t.Fatalf("newErrInvalidType: \nwant: %s\ngot: %s", want, got)
		}
	})
}

func TestParser_ParseExecutableDefinition(t *testing.T) {
	parser := NewParser()
	input := make([]byte, 65536)
	err := parser.ParseTypeSystemDefinition(input)
	if err == nil {
		t.Fatal("want err, got nil")
	}

	parser = NewParser()

	err = parser.ParseExecutableDefinition(input)
	if err == nil {
		t.Fatal("want err, got nil")
	}
}

func TestParser_CachedByteSlice(t *testing.T) {
	parser := NewParser()
	if parser.CachedByteSlice(-1) != nil {
		panic("want nil")
	}
}

func TestParser_putListValue(t *testing.T) {
	parser := NewParser()

	value, valueIndex := parser.makeValue()
	value.ValueType = document.ValueTypeInt
	value.Reference = parser.putInteger(1234)
	parser.putValue(value, valueIndex)

	var listValueIndex int
	var listValueIndex2 int

	listValue := parser.makeListValue(&listValueIndex)
	listValue2 := parser.makeListValue(&listValueIndex2)
	listValue = append(listValue, valueIndex)
	listValue2 = append(listValue2, valueIndex)

	parser.putListValue(listValue, &listValueIndex)
	parser.putListValue(listValue2, &listValueIndex2)

	if listValueIndex != listValueIndex2 {
		panic("expect lists to be merged")
	}

	if len(parser.ParsedDefinitions.ListValues) != 1 {
		panic("want duplicate to be deleted")
	}
}

func TestParser_putObjectValue(t *testing.T) {
	parser := NewParser()
	if err := parser.l.SetTypeSystemInput([]byte("foo bar")); err != nil {
		panic(err)
	}

	var iFoo document.Value
	var iBar document.Value
	parser.parsePeekedByteSlice(&iFoo)
	parser.parsePeekedByteSlice(&iBar)

	value1, iValue1 := parser.makeValue()
	value1.ValueType = document.ValueTypeInt
	value1.Reference = parser.putInteger(1234)
	parser.putValue(value1, iValue1)

	value2, iValue2 := parser.makeValue()
	value2.ValueType = document.ValueTypeInt
	value2.Reference = parser.putInteger(1234)
	parser.putValue(value2, iValue2)

	field1 := parser.putObjectField(document.ObjectField{
		Name:  iFoo.Reference,
		Value: iValue1,
	})

	field3 := parser.putObjectField(document.ObjectField{
		Name:  iFoo.Reference,
		Value: iValue1,
	})

	if field1 != field3 {
		panic("want identical fields to be merged")
	}

	field2 := parser.putObjectField(document.ObjectField{
		Name:  iBar.Reference,
		Value: iValue2,
	})

	var iObjectValue1 int
	objectValue1 := parser.makeObjectValue(&iObjectValue1)
	objectValue1 = append(objectValue1, field1, field2)

	var iObjectValue2 int
	objectValue2 := parser.makeObjectValue(&iObjectValue2)
	objectValue2 = append(objectValue2, field1, field2)

	parser.putObjectValue(objectValue1, &iObjectValue1)
	parser.putObjectValue(objectValue2, &iObjectValue2)

	if iObjectValue1 != iObjectValue2 {
		panic("expected object values to merge")
	}

	if len(parser.ParsedDefinitions.ObjectValues) != 1 {
		panic("want duplicated to be deleted")
	}
}

func TestParser_Starwars(t *testing.T) {

	inputFileName := "../../starwars.schema.graphql"
	fixtureFileName := "type_system_definition_parsed_starwars"

	parser := NewParser(WithPoolSize(2), WithMinimumSliceSize(2))

	starwarsSchema, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	err = parser.ParseTypeSystemDefinition(starwarsSchema)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(parser.ParsedDefinitions.TypeSystemDefinition, "", "  ")
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

	parser := NewParser()
	err = parser.ParseExecutableDefinition(inputFileData)
	if err != nil {
		t.Fatal(err)
	}

	err = parser.ParseExecutableDefinition(inputFileData)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(parser.ParsedDefinitions.ExecutableDefinition, "", "  ")
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

	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		err := parser.ParseExecutableDefinition(testData)
		if err != nil {
			b.Fatal(err)
		}

	}

}
