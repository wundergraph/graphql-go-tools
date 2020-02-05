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
			nonNullString: String!
			nullableString: String
			nonNullInt: Int!
			nullableInt: Int
			nonNullBoolean: Boolean!
			nullableBoolean: Boolean
			nonNullFloat: Float!
			nullableFloat: Float
			nullableListOfNullableString: [String]
			nonNullListOfNullableString: [String]!
			nonNullListOfNonNullString: [String!]!
			nullableListOfNullableHeader: [Header]
			nonNullListOfNullableHeader: [Header]!
			nonNullListOfNonNullParameter: [Parameter!]!
		) on FIELD_DEFINITION

		input Header {
			key: String!
			value: String!
		}

		input Parameter {
			name: String!
			sourceKind: PARAMETER_SOURCE!
			sourceName: String!
			variableType: String!
		}

		enum HTTP_METHOD {
			GET
			POST
			UPDATE
			DELETE
		}

		enum PARAMETER_SOURCE {
			CONTEXT_VARIABLE
			OBJECT_VARIABLE_ARGUMENT
			FIELD_ARGUMENTS
		}
	`)

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
