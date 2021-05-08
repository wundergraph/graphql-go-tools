package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestNoDeprecatedCustomRule(t *testing.T) {

	t.Run("Validate: no deprecated", func(t *testing.T) {
		t.Run("no deprecated fields", func(t *testing.T) {
			expectValid, expectErrors := buildAssertion(`
      type Query {
        normalField: String
        deprecatedField: String @deprecated(reason: "Some field reason.")
      }
    `)

			t.Run("ignores fields that are not deprecated", func(t *testing.T) {
				expectValid(`
        {
          normalField
        }
      `)
			})

			t.Run("ignores unknown fields", func(t *testing.T) {
				expectValid(`
        {
          unknownField
        }

        fragment UnknownFragment on UnknownType {
          deprecatedField
        }
      `)
			})

			t.Run("reports error when a deprecated field is selected", func(t *testing.T) {
				message :=
					"The field Query.deprecatedField is deprecated. Some field reason."

				expectErrors(`
        {
          deprecatedField
        }

        fragment QueryFragment on Query {
          deprecatedField
        }
      `)(`[
        {` + message + `, locations: [{ line: 3, column: 11 }] },
        {` + message + `, locations: [{ line: 7, column: 11 }] },
]`)
			})
		})

		t.Run("no deprecated arguments on fields", func(t *testing.T) {
			expectValid, expectErrors := buildAssertion(`
      type Query {
        someField(
          normalArg: String,
          deprecatedArg: String @deprecated(reason: "Some arg reason."),
        ): String
      }
    `)

			t.Run("ignores arguments that are not deprecated", func(t *testing.T) {
				expectValid(`
        {
          normalField(normalArg: "")
        }
      `)
			})

			t.Run("ignores unknown arguments", func(t *testing.T) {
				expectValid(`
        {
          someField(unknownArg: "")
          unknownField(deprecatedArg: "")
        }
      `)
			})

			t.Run("reports error when a deprecated argument is used", func(t *testing.T) {
				expectErrors(`
        {
          someField(deprecatedArg: "")
        }
      `)(`[
        {
          message:
            'Field "Query.someField" argument "deprecatedArg" is deprecated. Some arg reason.',
          locations: [{ line: 3, column: 21 }],
        },
]`)
			})
		})

		t.Run("no deprecated arguments on directives", func(t *testing.T) {
			expectValid, expectErrors := buildAssertion(`
      type Query {
        someField: String
      }

      directive @someDirective(
        normalArg: String,
        deprecatedArg: String @deprecated(reason: "Some arg reason."),
      ) on FIELD
    `)

			t.Run("ignores arguments that are not deprecated", func(t *testing.T) {
				expectValid(`
        {
          someField @someDirective(normalArg: "")
        }
      `)
			})

			t.Run("ignores unknown arguments", func(t *testing.T) {
				expectValid(`
        {
          someField @someDirective(unknownArg: "")
          someField @unknownDirective(deprecatedArg: "")
        }
      `)
			})

			t.Run("reports error when a deprecated argument is used", func(t *testing.T) {
				expectErrors(`
        {
          someField @someDirective(deprecatedArg: "")
        }
      `)(`[
        {
          message:
            'Directive "@someDirective" argument "deprecatedArg" is deprecated. Some arg reason.',
          locations: [{ line: 3, column: 36 }],
        },
]`)
			})
		})

		t.Run("no deprecated input fields", func(t *testing.T) {
			expectValid, expectErrors := buildAssertion(`
      input InputType {
        normalField: String
        deprecatedField: String @deprecated(reason: "Some input field reason.")
      }

      type Query {
        someField(someArg: InputType): String
      }

      directive @someDirective(someArg: InputType) on FIELD
    `)

			t.Run("ignores input fields that are not deprecated", func(t *testing.T) {
				expectValid(`
        {
          someField(
            someArg: { normalField: "" }
          ) @someDirective(someArg: { normalField: "" })
        }
      `)
			})

			t.Run("ignores unknown input fields", func(t *testing.T) {
				expectValid(`
        {
          someField(
            someArg: { unknownField: "" }
          )

          someField(
            unknownArg: { unknownField: "" }
          )

          unknownField(
            unknownArg: { unknownField: "" }
          )
        }
      `)
			})

			t.Run("reports error when a deprecated input field is used", func(t *testing.T) {
				message :=
					"The input field InputType.deprecatedField is deprecated. Some input field reason."

				expectErrors(`
        {
          someField(
            someArg: { deprecatedField: "" }
          ) @someDirective(someArg: { deprecatedField: "" })
        }
      `)(`[
        {` + message + `, locations: [{ line: 4, column: 24 }] },
        {` + message + `, locations: [{ line: 5, column: 39 }] },
]`)
			})
		})

		t.Run("no deprecated enum values", func(t *testing.T) {
			expectValid, expectErrors := buildAssertion(`
      enum EnumType {
        NORMAL_VALUE
        DEPRECATED_VALUE @deprecated(reason: "Some enum reason.")
      }

      type Query {
        someField(enumArg: EnumType): String
      }
    `)

			t.Run("ignores enum values that are not deprecated", func(t *testing.T) {
				expectValid(`
        {
          normalField(enumArg: NORMAL_VALUE)
        }
      `)
			})

			t.Run("ignores unknown enum values", func(t *testing.T) {
				expectValid(`
        query (
          $unknownValue: EnumType = UNKNOWN_VALUE
          $unknownType: UnknownType = UNKNOWN_VALUE
        ) {
          someField(enumArg: UNKNOWN_VALUE)
          someField(unknownArg: UNKNOWN_VALUE)
          unknownField(unknownArg: UNKNOWN_VALUE)
        }

        fragment SomeFragment on Query {
          someField(enumArg: UNKNOWN_VALUE)
        }
      `)
			})

			t.Run("reports error when a deprecated enum value is used", func(t *testing.T) {
				message :=
					`The enum value "EnumType.DEPRECATED_VALUE" is deprecated. Some enum reason.`

				expectErrors(`
        query (
          $variable: EnumType = DEPRECATED_VALUE
        ) {
          someField(enumArg: DEPRECATED_VALUE)
        }
      `)(`[
        {` + message + `, locations: [{ line: 3, column: 33 }] },
        {` + message + `, locations: [{ line: 5, column: 30 }] },
]`)
			})
		})
	})

}

type AssertQuery func(queryStr string) helpers.ResultCompare

func buildAssertion(sdlStr string) (expectValid func(queryStr string), expectErrors AssertQuery) {
	schema := helpers.BuildSchema(sdlStr)

	expectErrors = func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrorsWithSchema(
			schema,
			"NoDeprecatedCustomRule",
			queryStr,
		)
	}

	expectValid = func(queryStr string) {
		expectErrors(queryStr)("[]")
	}

	return
}
