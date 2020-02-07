package codegen

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"testing"
)

func TestManual(t *testing.T) {
	schema := `
		type Query {
			rootField: String
				@DataSource (
					nonNullString: "nonNullString"
					nonNullInt: 2
					nullableInt: 1
					nonNullBoolean: true
					nullableBoolean: true
					nonNullFloat: 13.37
					nullableFloat: 13.37
					nullableListOfNullableString: ["foo","bar","baz"]
					nonNullListOfNullableString: ["foo","bar","baz"]
					nonNullListOfNonNullString: ["foo","bar","baz"]
					nullableListOfNullableHeader: [
						{
							key: "foo"
							value: "bar"
						},
						{
							key: "baz"
							value: "bal"
						},
					]
					nonNullListOfNullableHeader: []
					nonNullListOfNonNullParameter: [
						{
							name: "foo"
							sourceKind: CONTEXT_VARIABLE
							sourceName: "bar"
							variableName: "baz"
						}
					]
					methods: {
						list: [GET,POST]
					}
				)
		}
	`

	doc := unsafeparser.ParseGraphqlDocumentString(schema)

	var d DataSourceConfig
	d.Unmarshal(&doc, doc.FieldDefinitionDirectives(0)[0])

	if d.NonNullString != "nonNullString" {
		t.Fatalf("field: NonNullString want: nonNullString, got: %s\n", d.NonNullString)
	}
	if d.NullableString != nil {
		t.Fatalf("field: NullableString want: nil\n")
	}
	if d.NonNullInt != 2 {
		t.Fatalf("field: NonNullInt want: 2, got: %d\n", d.NonNullInt)
	}
	if *d.NullableInt != 1 {
		t.Fatalf("field: NullableInt want: 1, got: %d\n", *d.NullableInt)
	}
	if d.NonNullBoolean != true {
		t.Fatalf("field: NonNullBoolean want: true, got: %t\n", d.NonNullBoolean)
	}
	if *d.NullableBoolean != true {
		t.Fatalf("field: NullableBoolean want: true, got: %t\n", *d.NullableBoolean)
	}
	if d.NonNullFloat != 13.37 {
		t.Fatalf("field: NonNullFloat want: 13.37, got: %v\n", d.NonNullFloat)
	}
	if *d.NullableFloat != 13.37 {
		t.Fatalf("field: NullableFloat want: 13.37, got: %v\n", *d.NullableFloat)
	}
	if *(*d.NullableListOfNullableString)[0] != "foo" {
		t.Fatal("want foo")
	}
	if *(*d.NullableListOfNullableString)[1] != "bar" {
		t.Fatal("want bar")
	}
	if *(*d.NullableListOfNullableString)[2] != "baz" {
		t.Fatal("want baz")
	}
	if *d.NonNullListOfNullableString[0] != "foo" {
		t.Fatal("want foo")
	}
	if *d.NonNullListOfNullableString[1] != "bar" {
		t.Fatal("want bar")
	}
	if *d.NonNullListOfNullableString[2] != "baz" {
		t.Fatal("want baz")
	}
	if d.NonNullListOfNonNullString[0] != "foo" {
		t.Fatal("want foo")
	}
	if d.NonNullListOfNonNullString[1] != "bar" {
		t.Fatal("want bar")
	}
	if d.NonNullListOfNonNullString[2] != "baz" {
		t.Fatal("want baz")
	}
	if (*d.NullableListOfNullableHeader)[0].Key != "foo" {
		t.Fatal("want foo")
	}
	if (*d.NullableListOfNullableHeader)[0].Value != "bar" {
		t.Fatal("want bar")
	}
	if (*d.NullableListOfNullableHeader)[1].Key != "baz" {
		t.Fatal("want baz")
	}
	if (*d.NullableListOfNullableHeader)[1].Value != "bal" {
		t.Fatal("want bal")
	}
	if len(d.NonNullListOfNullableHeader) != 0 {
		t.Fatal("want empty array")
	}
	if d.NonNullListOfNullableHeader == nil {
		t.Fatal("want != nil")
	}
	if d.NonNullListOfNonNullParameter[0].Name != "foo" {
		t.Fatal("want foo")
	}
	if d.NonNullListOfNonNullParameter[0].SourceKind != PARAMETER_SOURCE_CONTEXT_VARIABLE {
		t.Fatal("want CONTEXT_VARIABLE")
	}
	if d.NonNullListOfNonNullParameter[0].SourceName != "bar" {
		t.Fatal("want bar")
	}
	if d.NonNullListOfNonNullParameter[0].VariableName != "baz" {
		t.Fatal("want baz")
	}
	if d.Methods.List[0] != HTTP_METHOD_GET {
		t.Fatal("want HTTP_METHOD_GET")
	}
	if d.Methods.List[1] != HTTP_METHOD_POST {
		t.Fatal("want HTTP_METHOD_POST")
	}
}
