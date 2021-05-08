package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestNoSchemaIntrospectionCustomRule(t *testing.T) {
	// FIXME: add var
	var schema string
	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrorsWithSchema(
			schema,
			"NoSchemaIntrospectionCustomRule",
			queryStr,
		)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	schema = helpers.BuildSchema(`
  type Query {
    someQuery: SomeType
  }

  type SomeType {
    someField: String
    introspectionField: __EnumValue
  }
`)

	t.Run("Validate: Prohibit introspection queries", func(t *testing.T) {
		t.Run("ignores valid fields including __typename", func(t *testing.T) {
			expectValid(`
      {
        someQuery {
          __typename
          someField
        }
      }
    `)
		})

		t.Run("ignores fields not in the schema", func(t *testing.T) {
			expectValid(`
      {
        __introspect
      }
    `)
		})

		t.Run("reports error when a field with an introspection type is requested", func(t *testing.T) {
			expectErrors(`
      {
        __schema {
          queryType {
            name
          }
        }
      }
    `)(`[
      {
        message:
          'GraphQL introspection has been disabled, but the requested query contained the field "__schema".',
        locations: [{ line: 3, column: 9 }],
      },
      {
        message:
          'GraphQL introspection has been disabled, but the requested query contained the field "queryType".',
        locations: [{ line: 4, column: 11 }],
      },
]`)
		})

		t.Run("reports error when a field with an introspection type is requested and aliased", func(t *testing.T) {
			expectErrors(`
      {
        s: __schema {
          queryType {
            name
          }
        }
      }
      `)(`[
      {
        message:
          'GraphQL introspection has been disabled, but the requested query contained the field "__schema".',
        locations: [{ line: 3, column: 9 }],
      },
      {
        message:
          'GraphQL introspection has been disabled, but the requested query contained the field "queryType".',
        locations: [{ line: 4, column: 11 }],
      },
]`)
		})

		t.Run("reports error when using a fragment with a field with an introspection type", func(t *testing.T) {
			expectErrors(`
      {
        ...QueryFragment
      }

      fragment QueryFragment on Query {
        __schema {
          queryType {
            name
          }
        }
      }
    `)(`[
      {
        message:
          'GraphQL introspection has been disabled, but the requested query contained the field "__schema".',
        locations: [{ line: 7, column: 9 }],
      },
      {
        message:
          'GraphQL introspection has been disabled, but the requested query contained the field "queryType".',
        locations: [{ line: 8, column: 11 }],
      },
]`)
		})

		t.Run("reports error for non-standard introspection fields", func(t *testing.T) {
			expectErrors(`
      {
        someQuery {
          introspectionField
        }
      }
    `)(`[
      {
        message:
          'GraphQL introspection has been disabled, but the requested query contained the field "introspectionField".',
        locations: [{ line: 4, column: 11 }],
      },
]`)
		})
	})

}
