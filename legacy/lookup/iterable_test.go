package lookup

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"testing"
)

func putLiteralString(p *parser.Parser, literal string) document.ByteSliceReference {
	mod := parser.NewManualAstMod(p)
	ref, _, err := mod.PutLiteralString(literal)
	if err != nil {
		panic(err)
	}
	return ref
}

func literalString(p *parser.Parser, cachedName document.ByteSliceReference) string {
	return string(p.ByteSlice(cachedName))
}

func TestFieldsContainingDirectiveIterator(t *testing.T) {
	p := parser.NewParser()
	look := New(p)
	if err := p.ParseTypeSystemDefinition([]byte(FieldsContainingDirectiveIteratorInput)); err != nil {
		panic(err)
	}

	addArgumentFromContext := putLiteralString(p, "addArgumentFromContext")
	documents := putLiteralString(p, "documents")
	Query := putLiteralString(p, "Query")
	adminField := putLiteralString(p, "adminField")
	Document := putLiteralString(p, "Document")

	walk := NewWalker(512, 8)
	walk.SetLookup(look)
	walk.WalkTypeSystemDefinition()

	iter := walk.FieldsContainingDirectiveIterator(addArgumentFromContext)
	if iter.Next() == false {
		t.Errorf("want true")
	}
	field, object, directive := iter.Value()
	directiveName := look.Directive(directive).Name
	if !look.ByteSliceReferenceContentsEquals(directiveName, addArgumentFromContext) {
		t.Errorf("want directive name: %s, got: %s", "addArgumentFromContext", literalString(p, directiveName))
	}
	fieldName := look.FieldDefinition(field).Name
	if !look.ByteSliceReferenceContentsEquals(fieldName, documents) {
		t.Errorf("want field name: %s, got: %s", "documents", literalString(p, fieldName))
	}
	objectName := look.ObjectTypeDefinition(object).Name
	if !look.ByteSliceReferenceContentsEquals(objectName, Query) {
		t.Errorf("want object type definition name: %s. got: %s", "Query", literalString(p, objectName))
	}
	if iter.Next() == false {
		t.Errorf("want true")
	}
	field, object, directive = iter.Value()
	directiveName = look.Directive(directive).Name
	if !look.ByteSliceReferenceContentsEquals(directiveName, addArgumentFromContext) {
		t.Errorf("want directive name: %s, got: %s", "addArgumentFromContext", literalString(p, directiveName))
	}
	fieldName = look.FieldDefinition(field).Name
	if !look.ByteSliceReferenceContentsEquals(fieldName, adminField) {
		t.Errorf("want field: %s, got: %s", "adminField", literalString(p, fieldName))
	}
	objectName = look.ObjectTypeDefinition(object).Name
	if !look.ByteSliceReferenceContentsEquals(objectName, Document) {
		t.Errorf("want object type definition: %s, got: %s", "Document", literalString(p, objectName))
	}
	if iter.Next() {
		t.Errorf("want false")
	}
}

const FieldsContainingDirectiveIteratorInput = `
directive @addArgumentFromContext(
	name: String!
	contextKey: String!
) on FIELD_DEFINITION

scalar String

schema {
	query: Query
}

type Query {
	documents: [Document] @addArgumentFromContext(name: "user",contextKey: "user")
}

type Document implements Node {
	owner: String
	sensitiveInformation: String
	adminField: String @addArgumentFromContext(name: "admin",contextKey: "admin")
}
`
