package codegen

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/goldie"
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
			methods: Methods!
			nullableStringWithDefault: String = "defaultValue"
			nonNullStringWithDefault: String! = "defaultValue"
			intWithDefault: Int = 123
			floatWithDefault: Float = 1.23
			booleanWithDefault: Boolean = true
			stringWithDefaultOverride: String = "foo"
			inputWithDefaultChildField: InputWithDefault!
		) on FIELD_DEFINITION

		input InputWithDefault {
			nullableString: String
			stringWithDefault: String = "defaultValue"
			intWithDefault: Int = 123
			booleanWithDefault: Boolean = true
			floatWithDefault: Float = 1.23
		}

		input Methods {
			list: [HTTP_METHOD!]!
		}

		input Header {
			key: String!
			value: String!
		}

		input Parameter {
			name: String!
			sourceKind: PARAMETER_SOURCE!
			sourceName: String!
			variableName: String!
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

	config := Config{
		PackageName:           "codegen",
		DirectiveStructSuffix: "Config",
	}

	gen := New(&doc, config)
	out := bytes.Buffer{}
	_, err := gen.Generate(&out)
	if err != nil {
		t.Fatal(err)
	}

	data := out.Bytes()

	goldie.Assert(t, "DataSource", data)
	if t.Failed() {

		fixture, err := os.ReadFile("./fixtures/DataSource.golden")
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, string(data), string(fixture))
	}
}
