package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestKnownDirectivesRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("KnownDirectivesRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	expectSDLErrors := func(sdlStr string, sch ...string) helpers.ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return helpers.ExpectSDLValidationErrors(schema, "KnownDirectivesRule", sdlStr)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr, schema...)(`[]`)
	}

	schemaWithSDLDirectives := helpers.BuildSchema(`
  directive @onSchema on SCHEMA
  directive @onScalar on SCALAR
  directive @onObject on OBJECT
  directive @onFieldDefinition on FIELD_DEFINITION
  directive @onArgumentDefinition on ARGUMENT_DEFINITION
  directive @onInterface on INTERFACE
  directive @onUnion on UNION
  directive @onEnum on ENUM
  directive @onEnumValue on ENUM_VALUE
  directive @onInputObject on INPUT_OBJECT
  directive @onInputFieldDefinition on INPUT_FIELD_DEFINITION
`)

	t.Run("Validate: Known directives", func(t *testing.T) {
		t.Run("with no directives", func(t *testing.T) {
			expectValid(`
      query Foo {
        name
        ...Frag
      }

      fragment Frag on Dog {
        name
      }
    `)
		})

		t.Run("with known directives", func(t *testing.T) {
			expectValid(`
      {
        dog @include(if: true) {
          name
        }
        human @skip(if: false) {
          name
        }
      }
    `)
		})

		t.Run("with unknown directive", func(t *testing.T) {
			expectErrors(`
      {
        dog @unknown(directive: "value") {
          name
        }
      }
    `)(`[
      {
        message: 'Unknown directive "@unknown".',
        locations: [{ line: 3, column: 13 }],
      },
]`)
		})

		t.Run("with many unknown directives", func(t *testing.T) {
			expectErrors(`
      {
        dog @unknown(directive: "value") {
          name
        }
        human @unknown(directive: "value") {
          name
          pets @unknown(directive: "value") {
            name
          }
        }
      }
    `)(`[
      {
        message: 'Unknown directive "@unknown".',
        locations: [{ line: 3, column: 13 }],
      },
      {
        message: 'Unknown directive "@unknown".',
        locations: [{ line: 6, column: 15 }],
      },
      {
        message: 'Unknown directive "@unknown".',
        locations: [{ line: 8, column: 16 }],
      },
]`)
		})

		t.Run("with well placed directives", func(t *testing.T) {
			expectValid(`
      query ($var: Boolean) @onQuery {
        name @include(if: $var)
        ...Frag @include(if: true)
        skippedField @skip(if: true)
        ...SkippedFrag @skip(if: true)

        ... @skip(if: true) {
          skippedField
        }
      }

      mutation @onMutation {
        someField
      }

      subscription @onSubscription {
        someField
      }

      fragment Frag on SomeType @onFragmentDefinition {
        someField
      }
    `)
		})

		t.Run("with well placed variable definition directive", func(t *testing.T) {
			expectValid(`
      query Foo($var: Boolean @onVariableDefinition) {
        name
      }
    `)
		})

		t.Run("with misplaced directives", func(t *testing.T) {
			expectErrors(`
      query Foo($var: Boolean) @include(if: true) {
        name @onQuery @include(if: $var)
        ...Frag @onQuery
      }

      mutation Bar @onQuery {
        someField
      }
    `)(`[
      {
        message: 'Directive "@include" may not be used on QUERY.',
        locations: [{ line: 2, column: 32 }],
      },
      {
        message: 'Directive "@onQuery" may not be used on FIELD.',
        locations: [{ line: 3, column: 14 }],
      },
      {
        message: 'Directive "@onQuery" may not be used on FRAGMENT_SPREAD.',
        locations: [{ line: 4, column: 17 }],
      },
      {
        message: 'Directive "@onQuery" may not be used on MUTATION.',
        locations: [{ line: 7, column: 20 }],
      },
]`)
		})

		t.Run("with misplaced variable definition directive", func(t *testing.T) {
			expectErrors(`
      query Foo($var: Boolean @onField) {
        name
      }
    `)(`[
      {
        message: 'Directive "@onField" may not be used on VARIABLE_DEFINITION.',
        locations: [{ line: 2, column: 31 }],
      },
]`)
		})

		t.Run("within SDL", func(t *testing.T) {
			t.Run("with directive defined inside SDL", func(t *testing.T) {
				expectValidSDL(`
        type Query {
          foo: String @test
        }

        directive @test on FIELD_DEFINITION
      `)
			})

			t.Run("with standard directive", func(t *testing.T) {
				expectValidSDL(`
        type Query {
          foo: String @deprecated
        }
      `)
			})

			t.Run("with overridden standard directive", func(t *testing.T) {
				expectValidSDL(`
        schema @deprecated {
          query: Query
        }
        directive @deprecated on SCHEMA
      `)
			})

			t.Run("with directive defined in schema extension", func(t *testing.T) {
				schema := helpers.BuildSchema(`
        type Query {
          foo: String
        }
      `)
				expectValidSDL(
					`
          directive @test on OBJECT

          extend type Query @test
        `,
					schema,
				)
			})

			t.Run("with directive used in schema extension", func(t *testing.T) {
				schema := helpers.BuildSchema(`
        directive @test on OBJECT

        type Query {
          foo: String
        }
      `)
				expectValidSDL(
					`
          extend type Query @test
        `,
					schema,
				)
			})

			t.Run("with unknown directive in schema extension", func(t *testing.T) {
				schema := helpers.BuildSchema(`
        type Query {
          foo: String
        }
      `)
				expectSDLErrors(
					`
          extend type Query @unknown
        `,
					schema,
				)(`[
        {
          message: 'Unknown directive "@unknown".',
          locations: [{ line: 2, column: 29 }],
        },
]`)
			})

			t.Run("with well placed directives", func(t *testing.T) {
				expectValidSDL(
					`
          type MyObj implements MyInterface @onObject {
            myField(myArg: Int @onArgumentDefinition): String @onFieldDefinition
          }

          extend type MyObj @onObject

          scalar MyScalar @onScalar

          extend scalar MyScalar @onScalar

          interface MyInterface @onInterface {
            myField(myArg: Int @onArgumentDefinition): String @onFieldDefinition
          }

          extend interface MyInterface @onInterface

          union MyUnion @onUnion = MyObj | Other

          extend union MyUnion @onUnion

          enum MyEnum @onEnum {
            MY_VALUE @onEnumValue
          }

          extend enum MyEnum @onEnum

          input MyInput @onInputObject {
            myField: Int @onInputFieldDefinition
          }

          extend input MyInput @onInputObject

          schema @onSchema {
            query: MyQuery
          }

          extend schema @onSchema
        `,
					schemaWithSDLDirectives,
				)
			})

			t.Run("with misplaced directives", func(t *testing.T) {
				expectSDLErrors(
					`
          type MyObj implements MyInterface @onInterface {
            myField(myArg: Int @onInputFieldDefinition): String @onInputFieldDefinition
          }

          scalar MyScalar @onEnum

          interface MyInterface @onObject {
            myField(myArg: Int @onInputFieldDefinition): String @onInputFieldDefinition
          }

          union MyUnion @onEnumValue = MyObj | Other

          enum MyEnum @onScalar {
            MY_VALUE @onUnion
          }

          input MyInput @onEnum {
            myField: Int @onArgumentDefinition
          }

          schema @onObject {
            query: MyQuery
          }

          extend schema @onObject
        `,
					schemaWithSDLDirectives,
				)(`[
        {
          message: 'Directive "@onInterface" may not be used on OBJECT.',
          locations: [{ line: 2, column: 45 }],
        },
        {
          message:
            'Directive "@onInputFieldDefinition" may not be used on ARGUMENT_DEFINITION.',
          locations: [{ line: 3, column: 32 }],
        },
        {
          message:
            'Directive "@onInputFieldDefinition" may not be used on FIELD_DEFINITION.',
          locations: [{ line: 3, column: 65 }],
        },
        {
          message: 'Directive "@onEnum" may not be used on SCALAR.',
          locations: [{ line: 6, column: 27 }],
        },
        {
          message: 'Directive "@onObject" may not be used on INTERFACE.',
          locations: [{ line: 8, column: 33 }],
        },
        {
          message:
            'Directive "@onInputFieldDefinition" may not be used on ARGUMENT_DEFINITION.',
          locations: [{ line: 9, column: 32 }],
        },
        {
          message:
            'Directive "@onInputFieldDefinition" may not be used on FIELD_DEFINITION.',
          locations: [{ line: 9, column: 65 }],
        },
        {
          message: 'Directive "@onEnumValue" may not be used on UNION.',
          locations: [{ line: 12, column: 25 }],
        },
        {
          message: 'Directive "@onScalar" may not be used on ENUM.',
          locations: [{ line: 14, column: 23 }],
        },
        {
          message: 'Directive "@onUnion" may not be used on ENUM_VALUE.',
          locations: [{ line: 15, column: 22 }],
        },
        {
          message: 'Directive "@onEnum" may not be used on INPUT_OBJECT.',
          locations: [{ line: 18, column: 25 }],
        },
        {
          message:
            'Directive "@onArgumentDefinition" may not be used on INPUT_FIELD_DEFINITION.',
          locations: [{ line: 19, column: 26 }],
        },
        {
          message: 'Directive "@onObject" may not be used on SCHEMA.',
          locations: [{ line: 22, column: 18 }],
        },
        {
          message: 'Directive "@onObject" may not be used on SCHEMA.',
          locations: [{ line: 26, column: 25 }],
        },
]`)
			})
		})
	})

}
