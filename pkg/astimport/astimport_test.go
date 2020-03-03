package astimport

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"testing"
)

func TestImporter_ImportType(t *testing.T) {

	for _, typeDef := range []string{
		"ID!",
		"[String]!",
		"[String!]!",
		"FooType",
	} {
		typeBytes := []byte(typeDef)
		from := &ast.Document{}
		from.Input.AppendInputBytes(typeBytes)
		report := &operationreport.Report{}
		parser := astparser.NewParser()
		parser.PrepareImport(from, report)
		ref := parser.ParseType()

		if report.HasErrors() {
			t.Fatal(report)
		}

		out, err := from.PrintTypeBytes(ref, nil)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(out, typeBytes) {
			t.Fatalf("want: {{%s}}\ngot: {{%s}}'", string(typeBytes), string(out))
		}

		to := &ast.Document{}
		importer := &Importer{}

		ref = importer.ImportType(ref, from, to)
		out, err = to.PrintTypeBytes(ref, nil)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(out, typeBytes) {
			t.Fatalf("want: {{%s}}\ngot: {{%s}}'", string(typeBytes), string(out))
		}
	}
}

/*func ExampleImporter_ImportType() {
	typeBytes := []byte("String!")
	doc := ast.Document{}
	importer := NewImporter()

	ref := importer.ImportTypeBytes(typeBytes,&doc)
	// reference to the type
	fmt.Println(ref)
	// reference to the nested type
	fmt.Println(doc.Types[ref].OfType)
	// name of the nested type
	fmt.Println(doc.TypeNameString(doc.Types[ref].OfType))
	// Output:
	// 1
	// 0
	// String
}*/
