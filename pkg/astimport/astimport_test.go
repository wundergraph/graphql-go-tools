package astimport

import (
	"bytes"
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