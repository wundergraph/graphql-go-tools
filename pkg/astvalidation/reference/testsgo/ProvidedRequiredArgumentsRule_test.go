package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestProvidedRequiredArgumentsRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("ProvidedRequiredArgumentsRule", queryStr)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	expectSDLErrors := func(sdlStr string, sch ...string) helpers.ResultCompare {
		schema := ""
		if len(sch) > 0 {
			schema = sch[0]
		}
		return helpers.ExpectSDLValidationErrors(
			schema,
			"ProvidedRequiredArgumentsOnDirectivesRule",
			sdlStr,
		)
	}

	expectValidSDL := func(sdlStr string, schema ...string) {
		expectSDLErrors(sdlStr)(`[]`)
	}

	t.Run("Validate: Provided required arguments", func(t *testing.T) {
		t.Run("ignores unknown arguments", func(t *testing.T) {
			expectValid(`
      {
        dog {
          isHouseTrained(unknownArgument: true)
        }
      }
    `)
		})

		t.Run("Valid non-nullable value", func(t *testing.T) {
			t.Run("Arg on optional arg", func(t *testing.T) {
				expectValid(`
        {
          dog {
            isHouseTrained(atOtherHomes: true)
          }
        }
      `)
			})

			t.Run("No Arg on optional arg", func(t *testing.T) {
				expectValid(`
        {
          dog {
            isHouseTrained
          }
        }
      `)
			})

			t.Run("No arg on non-null field with default", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            nonNullFieldWithDefault
          }
        }
      `)
			})

			t.Run("Multiple args", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            multipleReqs(req1: 1, req2: 2)
          }
        }
      `)
			})

			t.Run("Multiple args reverse order", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            multipleReqs(req2: 2, req1: 1)
          }
        }
      `)
			})

			t.Run("No args on multiple optional", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            multipleOpts
          }
        }
      `)
			})

			t.Run("One arg on multiple optional", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            multipleOpts(opt1: 1)
          }
        }
      `)
			})

			t.Run("Second arg on multiple optional", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            multipleOpts(opt2: 1)
          }
        }
      `)
			})

			t.Run("Multiple required args on mixedList", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            multipleOptAndReq(req1: 3, req2: 4)
          }
        }
      `)
			})

			t.Run("Multiple required and one optional arg on mixedList", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            multipleOptAndReq(req1: 3, req2: 4, opt1: 5)
          }
        }
      `)
			})

			t.Run("All required and optional args on mixedList", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            multipleOptAndReq(req1: 3, req2: 4, opt1: 5, opt2: 6)
          }
        }
      `)
			})
		})

		t.Run("Invalid non-nullable value", func(t *testing.T) {
			t.Run("Missing one non-nullable argument", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            multipleReqs(req2: 2)
          }
        }
      `)(`[
        {
          message:
            'Field "multipleReqs" argument "req1" of type "Int!" is required, but it was not provided.',
          locations: [{ line: 4, column: 13 }],
        },
]`)
			})

			t.Run("Missing multiple non-nullable arguments", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            multipleReqs
          }
        }
      `)(`[
        {
          message:
            'Field "multipleReqs" argument "req1" of type "Int!" is required, but it was not provided.',
          locations: [{ line: 4, column: 13 }],
        },
        {
          message:
            'Field "multipleReqs" argument "req2" of type "Int!" is required, but it was not provided.',
          locations: [{ line: 4, column: 13 }],
        },
]`)
			})

			t.Run("Incorrect value and missing argument", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            multipleReqs(req1: "one")
          }
        }
      `)(`[
        {
          message:
            'Field "multipleReqs" argument "req2" of type "Int!" is required, but it was not provided.',
          locations: [{ line: 4, column: 13 }],
        },
]`)
			})
		})

		t.Run("Directive arguments", func(t *testing.T) {
			t.Run("ignores unknown directives", func(t *testing.T) {
				expectValid(`
        {
          dog @unknown
        }
      `)
			})

			t.Run("with directives of valid types", func(t *testing.T) {
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

			t.Run("with directive with missing types", func(t *testing.T) {
				expectErrors(`
        {
          dog @include {
            name @skip
          }
        }
      `)(`[
        {
          message:
            'Directive "@include" argument "if" of type "Boolean!" is required, but it was not provided.',
          locations: [{ line: 3, column: 15 }],
        },
        {
          message:
            'Directive "@skip" argument "if" of type "Boolean!" is required, but it was not provided.',
          locations: [{ line: 4, column: 18 }],
        },
]`)
			})
		})

		t.Run("within SDL", func(t *testing.T) {
			t.Run("Missing optional args on directive defined inside SDL", func(t *testing.T) {
				expectValidSDL(`
        type Query {
          foo: String @test
        }

        directive @test(arg1: String, arg2: String! = "") on FIELD_DEFINITION
      `)
			})

			t.Run("Missing arg on directive defined inside SDL", func(t *testing.T) {
				expectSDLErrors(`
        type Query {
          foo: String @test
        }

        directive @test(arg: String!) on FIELD_DEFINITION
      `)(`[
        {
          message:
            'Directive "@test" argument "arg" of type "String!" is required, but it was not provided.',
          locations: [{ line: 3, column: 23 }],
        },
]`)
			})

			t.Run("Missing arg on standard directive", func(t *testing.T) {
				expectSDLErrors(`
        type Query {
          foo: String @include
        }
      `)(`[
        {
          message:
            'Directive "@include" argument "if" of type "Boolean!" is required, but it was not provided.',
          locations: [{ line: 3, column: 23 }],
        },
]`)
			})

			t.Run("Missing arg on overridden standard directive", func(t *testing.T) {
				expectSDLErrors(`
        type Query {
          foo: String @deprecated
        }
        directive @deprecated(reason: String!) on FIELD
      `)(`[
        {
          message:
            'Directive "@deprecated" argument "reason" of type "String!" is required, but it was not provided.',
          locations: [{ line: 3, column: 23 }],
        },
]`)
			})

			t.Run("Missing arg on directive defined in schema extension", func(t *testing.T) {
				schema := helpers.BuildSchema(`
        type Query {
          foo: String
        }
      `)
				expectSDLErrors(
					`
          directive @test(arg: String!) on OBJECT

          extend type Query  @test
        `,
					schema,
				)(`[
        {
          message:
            'Directive "@test" argument "arg" of type "String!" is required, but it was not provided.',
          locations: [{ line: 4, column: 30 }],
        },
]`)
			})

			t.Run("Missing arg on directive used in schema extension", func(t *testing.T) {
				schema := helpers.BuildSchema(`
        directive @test(arg: String!) on OBJECT

        type Query {
          foo: String
        }
      `)
				expectSDLErrors(
					`
          extend type Query @test
        `,
					schema,
				)(`[
        {
          message:
            'Directive "@test" argument "arg" of type "String!" is required, but it was not provided.',
          locations: [{ line: 2, column: 29 }],
        },
]`)
			})
		})
	})

}
