package astimport

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func TestImporter_ImportType(t *testing.T) {
	for _, typeDef := range []string{
		"ID!",
		"[String]!",
		"[String!]!",
		"FooType",
	} {
		typeBytes := []byte(typeDef)
		from := ast.NewDocument()
		from.Input.AppendInputBytes(typeBytes)
		report := &operationreport.Report{}
		parser := astparser.NewParser()
		parser.PrepareImport(from, report)
		ref := parser.ParseType()

		if report.HasErrors() {
			t.Fatal(report)
		}

		out, err := from.PrintTypeBytes(ref, nil)
		require.NoError(t, err)
		require.Equal(t, typeBytes, out)

		to := ast.NewDocument()
		importer := &Importer{}

		ref = importer.ImportType(ref, from, to)
		out, err = to.PrintTypeBytes(ref, nil)
		require.NoError(t, err)
		require.Equal(t, typeBytes, out)
	}
}

func TestImporter_ImportValue(t *testing.T) {
	for _, tc := range []struct {
		value, name string
		kind        ast.ValueKind
	}{
		{"111", "integer", ast.ValueKindInteger},
		{"-111", "negative integer", ast.ValueKindInteger},
		{"11.1", "float", ast.ValueKindFloat},
		{"-11.1", "negative float", ast.ValueKindFloat},
		{`"bobby"`, "string", ast.ValueKindString},
		{"ENUM_VALUE", "enum", ast.ValueKindEnum},
		{`{one: "one"}`, "object", ast.ValueKindObject},
		{"true", "bool", ast.ValueKindBoolean},
		{"[1,2]", "list", ast.ValueKindList},
		{"[[1,2]]", "nested list", ast.ValueKindList},
		{`[{a: "b"},{c: "d"}]`, "list with objects", ast.ValueKindList},
		{`[[{a: "b",c: [1,2]}]]`, "deep nested list", ast.ValueKindList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valueBytes := []byte(tc.value)

			from := ast.NewDocument()
			from.Input.AppendInputBytes(valueBytes)

			report := &operationreport.Report{}
			parser := astparser.NewParser()
			parser.PrepareImport(from, report)

			value := parser.ParseValue()
			require.Equal(t, value.Kind, tc.kind)

			if report.HasErrors() {
				t.Fatal(report)
			}

			out, err := from.PrintValueBytes(value, nil)
			require.NoError(t, err)
			require.Equal(t, valueBytes, out)

			to := ast.NewDocument()
			importer := &Importer{}

			value = importer.ImportValue(value, from, to)
			out, err = to.PrintValueBytes(value, nil)
			require.NoError(t, err)
			require.Equal(t, valueBytes, out)
		})
	}
}
