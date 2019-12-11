package codegen

import (
	"bytes"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"testing"
)

func TestCodeGen_GenerateDirectiveDefinitionStruct(t *testing.T) {

	doc := unsafeparser.ParseGraphqlDocumentString(`
		directive @DataSource (
			brokerAddr: String!
			clientID: String!
			topic: String!
			intField: Int!
			boolField: Boolean!
			nullableBool: Boolean
			nullableListOfNullableString: [String]
			nonNullListOfNullableString: [String]!
			nonNullListOfNonNullString: [String!]!
			nonNullListOfNullableCustom: [CustomStruct]!
		) on FIELD_DEFINITION`)

	gen := NewCodeGen(&doc, "main")
	out := bytes.Buffer{}
	_, err := gen.Generate(&out)
	if err != nil {
		t.Fatal(err)
	}

	data := out.Bytes()

	goldie.Assert(t, "MQTTDataSource", data)
	if t.Failed() {

		fixture, err := ioutil.ReadFile("./fixtures/MQTTDataSource.golden")
		if err != nil {
			t.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes("MQTTDataSource", fixture, data)
	}
}
