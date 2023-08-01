package astimport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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

func TestImporter_ImportVariableDefinitions(t *testing.T) {
	run := func(source ast.Document, target ast.Document, refs []int, expectedDocument ast.Document, expectedRefs []int) func(t *testing.T) {
		return func(t *testing.T) {
			importer := &Importer{}
			actualRefs := importer.ImportVariableDefinitions(refs, &source, &target)

			assert.Equal(t, expectedRefs, actualRefs)
			assert.Equal(t, expectedDocument, target)
		}
	}

	t.Run("should not import variables if source does not have variables", run(
		ast.Document{},
		ast.Document{},
		[]int{},
		ast.Document{},
		[]int{},
	))

	t.Run("should import a single variable with value", run(
		ast.Document{
			Input: ast.Input{
				RawBytes: []byte("abEUDE"),
			},
			Types: []ast.Type{
				{
					TypeKind: ast.TypeKindNamed,
					Name: ast.ByteSliceReference{
						Start: 2,
						End:   4,
					},
					OfType: -1,
				},
				{
					TypeKind: ast.TypeKindNamed,
					Name: ast.ByteSliceReference{
						Start: 4,
						End:   6,
					},
					OfType: -1,
				},
			},
			VariableValues: []ast.VariableValue{
				{
					Name: ast.ByteSliceReference{
						Start: 0,
						End:   1,
					},
				},
				{
					Name: ast.ByteSliceReference{
						Start: 1,
						End:   2,
					},
				},
			},
			VariableDefinitions: []ast.VariableDefinition{
				{
					VariableValue: ast.Value{
						Kind: ast.ValueKindVariable,
						Ref:  0,
					},
					Type: 0,
				},
				{
					VariableValue: ast.Value{
						Kind: ast.ValueKindVariable,
						Ref:  1,
					},
					Type: 1,
				},
			},
		},
		ast.Document{},
		[]int{1},
		ast.Document{
			Input: ast.Input{
				RawBytes: []byte("bDE"),
				Length:   3,
			},
			Types: []ast.Type{
				{
					TypeKind: ast.TypeKindNamed,
					Name: ast.ByteSliceReference{
						Start: 1,
						End:   3,
					},
					OfType: -1,
				},
			},
			VariableValues: []ast.VariableValue{
				{
					Name: ast.ByteSliceReference{
						Start: 0,
						End:   1,
					},
				},
			},
			VariableDefinitions: []ast.VariableDefinition{
				{
					VariableValue: ast.Value{
						Kind: ast.ValueKindVariable,
						Ref:  0,
					},
					Type: 0,
				},
			},
		},
		[]int{0},
	))

	t.Run("should import all variables with values", run(
		ast.Document{
			Input: ast.Input{
				RawBytes: []byte("abEUDE"),
			},
			Types: []ast.Type{
				{
					TypeKind: ast.TypeKindNamed,
					Name: ast.ByteSliceReference{
						Start: 2,
						End:   4,
					},
					OfType: -1,
				},
				{
					TypeKind: ast.TypeKindNamed,
					Name: ast.ByteSliceReference{
						Start: 4,
						End:   6,
					},
					OfType: -1,
				},
			},
			VariableValues: []ast.VariableValue{
				{
					Name: ast.ByteSliceReference{
						Start: 0,
						End:   1,
					},
				},
				{
					Name: ast.ByteSliceReference{
						Start: 1,
						End:   2,
					},
				},
			},
			VariableDefinitions: []ast.VariableDefinition{
				{
					VariableValue: ast.Value{
						Kind: ast.ValueKindVariable,
						Ref:  0,
					},
					Type: 0,
				},
				{
					VariableValue: ast.Value{
						Kind: ast.ValueKindVariable,
						Ref:  1,
					},
					Type: 1,
				},
			},
		},
		ast.Document{},
		[]int{0, 1},
		ast.Document{
			Input: ast.Input{
				RawBytes: []byte("aEUbDE"),
				Length:   6,
			},
			Types: []ast.Type{
				{
					TypeKind: ast.TypeKindNamed,
					Name: ast.ByteSliceReference{
						Start: 1,
						End:   3,
					},
					OfType: -1,
				},
				{
					TypeKind: ast.TypeKindNamed,
					Name: ast.ByteSliceReference{
						Start: 4,
						End:   6,
					},
					OfType: -1,
				},
			},
			VariableValues: []ast.VariableValue{
				{
					Name: ast.ByteSliceReference{
						Start: 0,
						End:   1,
					},
				},
				{
					Name: ast.ByteSliceReference{
						Start: 3,
						End:   4,
					},
				},
			},
			VariableDefinitions: []ast.VariableDefinition{
				{
					VariableValue: ast.Value{
						Kind: ast.ValueKindVariable,
						Ref:  0,
					},
					Type: 0,
				},
				{
					VariableValue: ast.Value{
						Kind: ast.ValueKindVariable,
						Ref:  1,
					},
					Type: 1,
				},
			},
		},
		[]int{0, 1},
	))
}
