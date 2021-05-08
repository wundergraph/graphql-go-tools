package testsgo

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation/reference/helpers"
)

func TestValuesOfCorrectTypeRule(t *testing.T) {

	expectErrors := func(queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrors("ValuesOfCorrectTypeRule", queryStr)
	}

	expectErrorsWithSchema := func(schema string, queryStr string) helpers.ResultCompare {
		return helpers.ExpectValidationErrorsWithSchema(
			schema,
			"ValuesOfCorrectTypeRule",
			queryStr,
		)
	}

	expectValid := func(queryStr string) {
		expectErrors(queryStr)(`[]`)
	}

	expectValidWithSchema := func(schema string, queryStr string) {
		expectErrorsWithSchema(schema, queryStr)(`[]`)
	}
	_ = expectValidWithSchema // FIXME: remove me I'm unused

	t.Run("Validate: Values of correct type", func(t *testing.T) {
		t.Run("Valid values", func(t *testing.T) {
			t.Run("Good int value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            intArgField(intArg: 2)
          }
        }
      `)
			})

			t.Run("Good negative int value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            intArgField(intArg: -2)
          }
        }
      `)
			})

			t.Run("Good boolean value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            booleanArgField(booleanArg: true)
          }
        }
      `)
			})

			t.Run("Good string value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            stringArgField(stringArg: "foo")
          }
        }
      `)
			})

			t.Run("Good float value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            floatArgField(floatArg: 1.1)
          }
        }
      `)
			})

			t.Run("Good negative float value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            floatArgField(floatArg: -1.1)
          }
        }
      `)
			})

			t.Run("Int into Float", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            floatArgField(floatArg: 1)
          }
        }
      `)
			})

			t.Run("Int into ID", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            idArgField(idArg: 1)
          }
        }
      `)
			})

			t.Run("String into ID", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            idArgField(idArg: "someIdString")
          }
        }
      `)
			})

			t.Run("Good enum value", func(t *testing.T) {
				expectValid(`
        {
          dog {
            doesKnowCommand(dogCommand: SIT)
          }
        }
      `)
			})

			t.Run("Enum with undefined value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            enumArgField(enumArg: UNKNOWN)
          }
        }
      `)
			})

			t.Run("Enum with null value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            enumArgField(enumArg: NO_FUR)
          }
        }
      `)
			})

			t.Run("null into nullable type", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            intArgField(intArg: null)
          }
        }
      `)

				expectValid(`
        {
          dog(a: null, b: null, c:{ requiredField: true, intField: null }) {
            name
          }
        }
      `)
			})
		})

		t.Run("Invalid String values", func(t *testing.T) {
			t.Run("Int into String", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            stringArgField(stringArg: 1)
          }
        }
      `)(`[
        {
          message: "String cannot represent a non string value: 1",
          locations: [{ line: 4, column: 39 }],
        },
]`)
			})

			t.Run("Float into String", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            stringArgField(stringArg: 1.0)
          }
        }
      `)(`[
        {
          message: "String cannot represent a non string value: 1.0",
          locations: [{ line: 4, column: 39 }],
        },
]`)
			})

			t.Run("Boolean into String", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            stringArgField(stringArg: true)
          }
        }
      `)(`[
        {
          message: "String cannot represent a non string value: true",
          locations: [{ line: 4, column: 39 }],
        },
]`)
			})

			t.Run("Unquoted String into String", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            stringArgField(stringArg: BAR)
          }
        }
      `)(`[
        {
          message: "String cannot represent a non string value: BAR",
          locations: [{ line: 4, column: 39 }],
        },
]`)
			})
		})

		t.Run("Invalid Int values", func(t *testing.T) {
			t.Run("String into Int", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            intArgField(intArg: "3")
          }
        }
      `)(`[
        {
          message: 'Int cannot represent non-integer value: "3"',
          locations: [{ line: 4, column: 33 }],
        },
]`)
			})

			t.Run("Big Int into Int", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            intArgField(intArg: 829384293849283498239482938)
          }
        }
      `)(`[
        {
          message:
            "Int cannot represent non 32-bit signed integer value: 829384293849283498239482938",
          locations: [{ line: 4, column: 33 }],
        },
]`)
			})

			t.Run("Unquoted String into Int", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            intArgField(intArg: FOO)
          }
        }
      `)(`[
        {
          message: "Int cannot represent non-integer value: FOO",
          locations: [{ line: 4, column: 33 }],
        },
]`)
			})

			t.Run("Simple Float into Int", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            intArgField(intArg: 3.0)
          }
        }
      `)(`[
        {
          message: "Int cannot represent non-integer value: 3.0",
          locations: [{ line: 4, column: 33 }],
        },
]`)
			})

			t.Run("Float into Int", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            intArgField(intArg: 3.333)
          }
        }
      `)(`[
        {
          message: "Int cannot represent non-integer value: 3.333",
          locations: [{ line: 4, column: 33 }],
        },
]`)
			})
		})

		t.Run("Invalid Float values", func(t *testing.T) {
			t.Run("String into Float", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            floatArgField(floatArg: "3.333")
          }
        }
      `)(`[
        {
          message: 'Float cannot represent non numeric value: "3.333"',
          locations: [{ line: 4, column: 37 }],
        },
]`)
			})

			t.Run("Boolean into Float", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            floatArgField(floatArg: true)
          }
        }
      `)(`[
        {
          message: "Float cannot represent non numeric value: true",
          locations: [{ line: 4, column: 37 }],
        },
]`)
			})

			t.Run("Unquoted into Float", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            floatArgField(floatArg: FOO)
          }
        }
      `)(`[
        {
          message: "Float cannot represent non numeric value: FOO",
          locations: [{ line: 4, column: 37 }],
        },
]`)
			})
		})

		t.Run("Invalid Boolean value", func(t *testing.T) {
			t.Run("Int into Boolean", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            booleanArgField(booleanArg: 2)
          }
        }
      `)(`[
        {
          message: "Boolean cannot represent a non boolean value: 2",
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("Float into Boolean", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            booleanArgField(booleanArg: 1.0)
          }
        }
      `)(`[
        {
          message: "Boolean cannot represent a non boolean value: 1.0",
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("String into Boolean", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            booleanArgField(booleanArg: "true")
          }
        }
      `)(`[
        {
          message: 'Boolean cannot represent a non boolean value: "true"',
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("Unquoted into Boolean", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            booleanArgField(booleanArg: TRUE)
          }
        }
      `)(`[
        {
          message: "Boolean cannot represent a non boolean value: TRUE",
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})
		})

		t.Run("Invalid ID value", func(t *testing.T) {
			t.Run("Float into ID", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            idArgField(idArg: 1.0)
          }
        }
      `)(`[
        {
          message:
            "ID cannot represent a non-string and non-integer value: 1.0",
          locations: [{ line: 4, column: 31 }],
        },
]`)
			})

			t.Run("Boolean into ID", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            idArgField(idArg: true)
          }
        }
      `)(`[
        {
          message:
            "ID cannot represent a non-string and non-integer value: true",
          locations: [{ line: 4, column: 31 }],
        },
]`)
			})

			t.Run("Unquoted into ID", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            idArgField(idArg: SOMETHING)
          }
        }
      `)(`[
        {
          message:
            "ID cannot represent a non-string and non-integer value: SOMETHING",
          locations: [{ line: 4, column: 31 }],
        },
]`)
			})
		})

		t.Run("Invalid Enum value", func(t *testing.T) {
			t.Run("Int into Enum", func(t *testing.T) {
				expectErrors(`
        {
          dog {
            doesKnowCommand(dogCommand: 2)
          }
        }
      `)(`[
        {
          message: 'Enum "DogCommand" cannot represent non-enum value: 2.',
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("Float into Enum", func(t *testing.T) {
				expectErrors(`
        {
          dog {
            doesKnowCommand(dogCommand: 1.0)
          }
        }
      `)(`[
        {
          message: 'Enum "DogCommand" cannot represent non-enum value: 1.0.',
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("String into Enum", func(t *testing.T) {
				expectErrors(`
        {
          dog {
            doesKnowCommand(dogCommand: "SIT")
          }
        }
      `)(`[
        {
          message:
            'Enum "DogCommand" cannot represent non-enum value: "SIT". Did you mean the enum value "SIT"?',
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("Boolean into Enum", func(t *testing.T) {
				expectErrors(`
        {
          dog {
            doesKnowCommand(dogCommand: true)
          }
        }
      `)(`[
        {
          message: 'Enum "DogCommand" cannot represent non-enum value: true.',
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("Unknown Enum Value into Enum", func(t *testing.T) {
				expectErrors(`
        {
          dog {
            doesKnowCommand(dogCommand: JUGGLE)
          }
        }
      `)(`[
        {
          message: 'Value "JUGGLE" does not exist in "DogCommand" enum.',
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("Different case Enum Value into Enum", func(t *testing.T) {
				expectErrors(`
        {
          dog {
            doesKnowCommand(dogCommand: sit)
          }
        }
      `)(`[
        {
          message:
            'Value "sit" does not exist in "DogCommand" enum. Did you mean the enum value "SIT"?',
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})
		})

		t.Run("Valid List value", func(t *testing.T) {
			t.Run("Good list value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            stringListArgField(stringListArg: ["one", null, "two"])
          }
        }
      `)
			})

			t.Run("Empty list value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            stringListArgField(stringListArg: [])
          }
        }
      `)
			})

			t.Run("Null value", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            stringListArgField(stringListArg: null)
          }
        }
      `)
			})

			t.Run("Single value into List", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            stringListArgField(stringListArg: "one")
          }
        }
      `)
			})
		})

		t.Run("Invalid List value", func(t *testing.T) {
			t.Run("Incorrect item type", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            stringListArgField(stringListArg: ["one", 2])
          }
        }
      `)(`[
        {
          message: "String cannot represent a non string value: 2",
          locations: [{ line: 4, column: 55 }],
        },
]`)
			})

			t.Run("Single value of incorrect type", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            stringListArgField(stringListArg: 1)
          }
        }
      `)(`[
        {
          message: "String cannot represent a non string value: 1",
          locations: [{ line: 4, column: 47 }],
        },
]`)
			})
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
			t.Run("Incorrect value type", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            multipleReqs(req2: "two", req1: "one")
          }
        }
      `)(`[
        {
          message: 'Int cannot represent non-integer value: "two"',
          locations: [{ line: 4, column: 32 }],
        },
        {
          message: 'Int cannot represent non-integer value: "one"',
          locations: [{ line: 4, column: 45 }],
        },
]`)
			})

			t.Run("Incorrect value and missing argument (ProvidedRequiredArgumentsRule)", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            multipleReqs(req1: "one")
          }
        }
      `)(`[
        {
          message: 'Int cannot represent non-integer value: "one"',
          locations: [{ line: 4, column: 32 }],
        },
]`)
			})

			t.Run("Null value", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            multipleReqs(req1: null)
          }
        }
      `)(`[
        {
          message: 'Expected value of type "Int!", found null.',
          locations: [{ line: 4, column: 32 }],
        },
]`)
			})
		})

		t.Run("Valid input object value", func(t *testing.T) {
			t.Run("Optional arg, despite required field in type", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            complexArgField
          }
        }
      `)
			})

			t.Run("Partial object, only required", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            complexArgField(complexArg: { requiredField: true })
          }
        }
      `)
			})

			t.Run("Partial object, required field can be falsy", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            complexArgField(complexArg: { requiredField: false })
          }
        }
      `)
			})

			t.Run("Partial object, including required", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            complexArgField(complexArg: { requiredField: true, intField: 4 })
          }
        }
      `)
			})

			t.Run("Full object", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            complexArgField(complexArg: {
              requiredField: true,
              intField: 4,
              stringField: "foo",
              booleanField: false,
              stringListField: ["one", "two"]
            })
          }
        }
      `)
			})

			t.Run("Full object with fields in different order", func(t *testing.T) {
				expectValid(`
        {
          complicatedArgs {
            complexArgField(complexArg: {
              stringListField: ["one", "two"],
              booleanField: false,
              requiredField: true,
              stringField: "foo",
              intField: 4,
            })
          }
        }
      `)
			})
		})

		t.Run("Invalid input object value", func(t *testing.T) {
			t.Run("Partial object, missing required", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            complexArgField(complexArg: { intField: 4 })
          }
        }
      `)(`[
        {
          message:
            'Field "ComplexInput.requiredField" of required type "Boolean!" was not provided.',
          locations: [{ line: 4, column: 41 }],
        },
]`)
			})

			t.Run("Partial object, invalid field type", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            complexArgField(complexArg: {
              stringListField: ["one", 2],
              requiredField: true,
            })
          }
        }
      `)(`[
        {
          message: "String cannot represent a non string value: 2",
          locations: [{ line: 5, column: 40 }],
        },
]`)
			})

			t.Run("Partial object, null to non-null field", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            complexArgField(complexArg: {
              requiredField: true,
              nonNullField: null,
            })
          }
        }
      `)(`[
        {
          message: 'Expected value of type "Boolean!", found null.',
          locations: [{ line: 6, column: 29 }],
        },
]`)
			})

			t.Run("Partial object, unknown field arg", func(t *testing.T) {
				expectErrors(`
        {
          complicatedArgs {
            complexArgField(complexArg: {
              requiredField: true,
              invalidField: "value"
            })
          }
        }
      `)(`[
        {
          message:
            'Field "invalidField" is not defined by type "ComplexInput". Did you mean "intField"?',
          locations: [{ line: 6, column: 15 }],
        },
]`)
			})

		})

		t.Run("Directive arguments", func(t *testing.T) {
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

			t.Run("with directive with incorrect types", func(t *testing.T) {
				expectErrors(`
        {
          dog @include(if: "yes") {
            name @skip(if: ENUM)
          }
        }
      `)(`[
        {
          message: 'Boolean cannot represent a non boolean value: "yes"',
          locations: [{ line: 3, column: 28 }],
        },
        {
          message: "Boolean cannot represent a non boolean value: ENUM",
          locations: [{ line: 4, column: 28 }],
        },
]`)
			})
		})

		t.Run("Variable default values", func(t *testing.T) {
			t.Run("variables with valid default values", func(t *testing.T) {
				expectValid(`
        query WithDefaultValues(
          $a: Int = 1,
          $b: String = "ok",
          $c: ComplexInput = { requiredField: true, intField: 3 }
          $d: Int! = 123
        ) {
          dog { name }
        }
      `)
			})

			t.Run("variables with valid default null values", func(t *testing.T) {
				expectValid(`
        query WithDefaultValues(
          $a: Int = null,
          $b: String = null,
          $c: ComplexInput = { requiredField: true, intField: null }
        ) {
          dog { name }
        }
      `)
			})

			t.Run("variables with invalid default null values", func(t *testing.T) {
				expectErrors(`
        query WithDefaultValues(
          $a: Int! = null,
          $b: String! = null,
          $c: ComplexInput = { requiredField: null, intField: null }
        ) {
          dog { name }
        }
      `)(`[
        {
          message: 'Expected value of type "Int!", found null.',
          locations: [{ line: 3, column: 22 }],
        },
        {
          message: 'Expected value of type "String!", found null.',
          locations: [{ line: 4, column: 25 }],
        },
        {
          message: 'Expected value of type "Boolean!", found null.',
          locations: [{ line: 5, column: 47 }],
        },
]`)
			})

			t.Run("variables with invalid default values", func(t *testing.T) {
				expectErrors(`
        query InvalidDefaultValues(
          $a: Int = "one",
          $b: String = 4,
          $c: ComplexInput = "NotVeryComplex"
        ) {
          dog { name }
        }
      `)(`[
        {
          message: 'Int cannot represent non-integer value: "one"',
          locations: [{ line: 3, column: 21 }],
        },
        {
          message: "String cannot represent a non string value: 4",
          locations: [{ line: 4, column: 24 }],
        },
        {
          message:
            'Expected value of type "ComplexInput", found "NotVeryComplex".',
          locations: [{ line: 5, column: 30 }],
        },
]`)
			})

			t.Run("variables with complex invalid default values", func(t *testing.T) {
				expectErrors(`
        query WithDefaultValues(
          $a: ComplexInput = { requiredField: 123, intField: "abc" }
        ) {
          dog { name }
        }
      `)(`[
        {
          message: "Boolean cannot represent a non boolean value: 123",
          locations: [{ line: 3, column: 47 }],
        },
        {
          message: 'Int cannot represent non-integer value: "abc"',
          locations: [{ line: 3, column: 62 }],
        },
]`)
			})

			t.Run("complex variables missing required field", func(t *testing.T) {
				expectErrors(`
        query MissingRequiredField($a: ComplexInput = {intField: 3}) {
          dog { name }
        }
      `)(`[
        {
          message:
            'Field "ComplexInput.requiredField" of required type "Boolean!" was not provided.',
          locations: [{ line: 2, column: 55 }],
        },
]`)
			})

			t.Run("list variables with invalid item", func(t *testing.T) {
				expectErrors(`
        query InvalidItem($a: [String] = ["one", 2]) {
          dog { name }
        }
      `)(`[
        {
          message: "String cannot represent a non string value: 2",
          locations: [{ line: 2, column: 50 }],
        },
]`)
			})
		})
	})

}
