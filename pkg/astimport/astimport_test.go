package astimport

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"testing"
)

func TestImporter_ImportType(t *testing.T) {

	for _,typeDef := range []string{
		"ID!",
		"[String]!",
		"[String!]!",
		"FooType",
	}{
		typeBytes := []byte(typeDef)
		doc := ast.Document{}
		importer := NewImporter()

		ref := importer.ImportType(typeBytes,&doc)

		out,err := doc.PrintTypeBytes(ref,nil)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(out,typeBytes) {
			t.Fatalf("want: {{%s}}\ngot: {{%s}}'",string(typeBytes),string(out))
		}
	}
}

func ExampleImporter_ImportType() {
	typeBytes := []byte("String!")
	doc := ast.Document{}
	importer := NewImporter()

	ref := importer.ImportType(typeBytes,&doc)
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
}