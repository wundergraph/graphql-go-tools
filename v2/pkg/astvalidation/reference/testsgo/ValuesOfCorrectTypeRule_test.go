package testsgo

import (
	"testing"
)

func TestValuesOfCorrectTypeRule(t *testing.T) {

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, ValuesOfCorrectTypeRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Values of correct type", func(t *testing.T) {
		t.Run("Valid values", func(t *testing.T) {
			t.Run("Good int value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            intArgField(intArg: 2)
          }
        }
      `)
			})

			t.Run("Good negative int value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            intArgField(intArg: -2)
          }
        }
      `)
			})

			t.Run("Good boolean value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            booleanArgField(booleanArg: true)
          }
        }
      `)
			})

			t.Run("Good string value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            stringArgField(stringArg: "foo")
          }
        }
      `)
			})

			t.Run("Good float value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            floatArgField(floatArg: 1.1)
          }
        }
      `)
			})

			t.Run("Good negative float value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            floatArgField(floatArg: -1.1)
          }
        }
      `)
			})

			t.Run("Int into Float", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            floatArgField(floatArg: 1)
          }
        }
      `)
			})

			t.Run("Int into ID", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            idArgField(idArg: 1)
          }
        }
      `)
			})

			t.Run("String into ID", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            idArgField(idArg: "someIdString")
          }
        }
      `)
			})

			t.Run("Good enum value", func(t *testing.T) {
				ExpectValid(t, `
        {
          dog {
            doesKnowCommand(dogCommand: SIT)
          }
        }
      `)
			})

			t.Run("Enum with undefined value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            enumArgField(enumArg: UNKNOWN)
          }
        }
      `)
			})

			t.Run("Enum with null value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            enumArgField(enumArg: NO_FUR)
          }
        }
      `)
			})

			t.Run("null into nullable type", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            intArgField(intArg: null)
          }
        }
      `)

				ExpectValid(t, `
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
				ExpectErrors(t, `
        {
          complicatedArgs {
            stringArgField(stringArg: 1)
          }
        }
      `)([]Err{
					{
						message:   "String cannot represent a non string value: 1",
						locations: []Loc{{line: 4, column: 39}},
					},
				})
			})

			t.Run("Float into String", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            stringArgField(stringArg: 1.0)
          }
        }
      `)([]Err{
					{
						message:   "String cannot represent a non string value: 1.0",
						locations: []Loc{{line: 4, column: 39}},
					},
				})
			})

			t.Run("Boolean into String", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            stringArgField(stringArg: true)
          }
        }
      `)([]Err{
					{
						message:   "String cannot represent a non string value: true",
						locations: []Loc{{line: 4, column: 39}},
					},
				})
			})

			t.Run("Unquoted String into String", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            stringArgField(stringArg: BAR)
          }
        }
      `)([]Err{
					{
						message:   "String cannot represent a non string value: BAR",
						locations: []Loc{{line: 4, column: 39}},
					},
				})
			})
		})

		t.Run("Invalid Int values", func(t *testing.T) {
			t.Run("String into Int", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            intArgField(intArg: "3")
          }
        }
      `)([]Err{
					{
						message:   `Int cannot represent non-integer value: "3"`,
						locations: []Loc{{line: 4, column: 33}},
					},
				})
			})

			t.Run("Big Int into Int", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            intArgField(intArg: 829384293849283498239482938)
          }
        }
      `)([]Err{
					{
						message:   "Int cannot represent non 32-bit signed integer value: 829384293849283498239482938",
						locations: []Loc{{line: 4, column: 33}},
					},
				})
			})

			t.Run("Unquoted String into Int", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            intArgField(intArg: FOO)
          }
        }
      `)([]Err{
					{
						message:   "Int cannot represent non-integer value: FOO",
						locations: []Loc{{line: 4, column: 33}},
					},
				})
			})

			t.Run("Simple Float into Int", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            intArgField(intArg: 3.0)
          }
        }
      `)([]Err{
					{
						message:   "Int cannot represent non-integer value: 3.0",
						locations: []Loc{{line: 4, column: 33}},
					},
				})
			})

			t.Run("Float into Int", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            intArgField(intArg: 3.333)
          }
        }
      `)([]Err{
					{
						message:   "Int cannot represent non-integer value: 3.333",
						locations: []Loc{{line: 4, column: 33}},
					},
				})
			})
		})

		t.Run("Invalid Float values", func(t *testing.T) {
			t.Run("String into Float", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            floatArgField(floatArg: "3.333")
          }
        }
      `)([]Err{
					{
						message:   `Float cannot represent non numeric value: "3.333"`,
						locations: []Loc{{line: 4, column: 37}},
					},
				})
			})

			t.Run("Boolean into Float", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            floatArgField(floatArg: true)
          }
        }
      `)([]Err{
					{
						message:   "Float cannot represent non numeric value: true",
						locations: []Loc{{line: 4, column: 37}},
					},
				})
			})

			t.Run("Unquoted into Float", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            floatArgField(floatArg: FOO)
          }
        }
      `)([]Err{
					{
						message:   "Float cannot represent non numeric value: FOO",
						locations: []Loc{{line: 4, column: 37}},
					},
				})
			})
		})

		t.Run("Invalid Boolean value", func(t *testing.T) {
			t.Run("Int into Boolean", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            booleanArgField(booleanArg: 2)
          }
        }
      `)([]Err{
					{
						message:   "Boolean cannot represent a non boolean value: 2",
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("Float into Boolean", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            booleanArgField(booleanArg: 1.0)
          }
        }
      `)([]Err{
					{
						message:   "Boolean cannot represent a non boolean value: 1.0",
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("String into Boolean", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            booleanArgField(booleanArg: "true")
          }
        }
      `)([]Err{
					{
						message:   `Boolean cannot represent a non boolean value: "true"`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("Unquoted into Boolean", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            booleanArgField(booleanArg: TRUE)
          }
        }
      `)([]Err{
					{
						message:   "Boolean cannot represent a non boolean value: TRUE",
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})
		})

		t.Run("Invalid ID value", func(t *testing.T) {
			t.Run("Float into ID", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            idArgField(idArg: 1.0)
          }
        }
      `)([]Err{
					{
						message:   "ID cannot represent a non-string and non-integer value: 1.0",
						locations: []Loc{{line: 4, column: 31}},
					},
				})
			})

			t.Run("Boolean into ID", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            idArgField(idArg: true)
          }
        }
      `)([]Err{
					{
						message:   "ID cannot represent a non-string and non-integer value: true",
						locations: []Loc{{line: 4, column: 31}},
					},
				})
			})

			t.Run("Unquoted into ID", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            idArgField(idArg: SOMETHING)
          }
        }
      `)([]Err{
					{
						message:   "ID cannot represent a non-string and non-integer value: SOMETHING",
						locations: []Loc{{line: 4, column: 31}},
					},
				})
			})
		})

		t.Run("Invalid Enum value", func(t *testing.T) {
			t.Run("Int into Enum", func(t *testing.T) {
				ExpectErrors(t, `
        {
          dog {
            doesKnowCommand(dogCommand: 2)
          }
        }
      `)([]Err{
					{
						message:   `Enum "DogCommand" cannot represent non-enum value: 2.`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("Float into Enum", func(t *testing.T) {
				ExpectErrors(t, `
        {
          dog {
            doesKnowCommand(dogCommand: 1.0)
          }
        }
      `)([]Err{
					{
						message:   `Enum "DogCommand" cannot represent non-enum value: 1.0.`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("String into Enum", func(t *testing.T) {
				ExpectErrors(t, `
        {
          dog {
            doesKnowCommand(dogCommand: "SIT")
          }
        }
      `)([]Err{
					{
						message:   `Enum "DogCommand" cannot represent non-enum value: "SIT".`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("String into Enum with suggestions", func(t *testing.T) {
				t.Skip(NotSupportedSuggestionsSkipMsg)

				ExpectErrors(t, `
        {
          dog {
            doesKnowCommand(dogCommand: "SIT")
          }
        }
      `)([]Err{
					{
						message:   `Enum "DogCommand" cannot represent non-enum value: "SIT". Did you mean the enum value "SIT"?`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("Boolean into Enum", func(t *testing.T) {
				ExpectErrors(t, `
        {
          dog {
            doesKnowCommand(dogCommand: true)
          }
        }
      `)([]Err{
					{
						message:   `Enum "DogCommand" cannot represent non-enum value: true.`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("Unknown Enum Value into Enum", func(t *testing.T) {
				ExpectErrors(t, `
        {
          dog {
            doesKnowCommand(dogCommand: JUGGLE)
          }
        }
      `)([]Err{
					{
						message:   `Value "JUGGLE" does not exist in "DogCommand" enum.`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("Different case Enum Value into Enum", func(t *testing.T) {
				ExpectErrors(t, `
        {
          dog {
            doesKnowCommand(dogCommand: sit)
          }
        }
      `)([]Err{
					{
						message:   `Value "sit" does not exist in "DogCommand" enum.`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("Different case Enum Value into Enum with suggestions", func(t *testing.T) {
				t.Skip(NotSupportedSuggestionsSkipMsg)

				ExpectErrors(t, `
        {
          dog {
            doesKnowCommand(dogCommand: sit)
          }
        }
      `)([]Err{
					{
						message:   `Value "sit" does not exist in "DogCommand" enum. Did you mean the enum value "SIT"?`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})
		})

		t.Run("Valid List value", func(t *testing.T) {
			t.Run("Good list value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            stringListArgField(stringListArg: ["one", null, "two"])
          }
        }
      `)
			})

			t.Run("Empty list value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            stringListArgField(stringListArg: [])
          }
        }
      `)
			})

			t.Run("Null value", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            stringListArgField(stringListArg: null)
          }
        }
      `)
			})

			t.Run("Single value into List", func(t *testing.T) {
				ExpectValid(t, `
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
				ExpectErrors(t, `
        {
          complicatedArgs {
            stringListArgField(stringListArg: ["one", 2])
          }
        }
      `)([]Err{
					{
						message:   "String cannot represent a non string value: 2",
						locations: []Loc{{line: 4, column: 55}},
					},
				})
			})

			t.Run("Single value of incorrect type", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            stringListArgField(stringListArg: 1)
          }
        }
      `)([]Err{
					{
						message:   "String cannot represent a non string value: 1",
						locations: []Loc{{line: 4, column: 47}},
					},
				})
			})
		})

		t.Run("Valid non-nullable value", func(t *testing.T) {
			t.Run("Arg on optional arg", func(t *testing.T) {
				ExpectValid(t, `
        {
          dog {
            isHouseTrained(atOtherHomes: true)
          }
        }
      `)
			})

			t.Run("No Arg on optional arg", func(t *testing.T) {
				ExpectValid(t, `
        {
          dog {
            isHouseTrained
          }
        }
      `)
			})

			t.Run("Multiple args", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            multipleReqs(req1: 1, req2: 2)
          }
        }
      `)
			})

			t.Run("Multiple args reverse order", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            multipleReqs(req2: 2, req1: 1)
          }
        }
      `)
			})

			t.Run("No args on multiple optional", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            multipleOpts
          }
        }
      `)
			})

			t.Run("One arg on multiple optional", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            multipleOpts(opt1: 1)
          }
        }
      `)
			})

			t.Run("Second arg on multiple optional", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            multipleOpts(opt2: 1)
          }
        }
      `)
			})

			t.Run("Multiple required args on mixedList", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            multipleOptAndReq(req1: 3, req2: 4)
          }
        }
      `)
			})

			t.Run("Multiple required and one optional arg on mixedList", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            multipleOptAndReq(req1: 3, req2: 4, opt1: 5)
          }
        }
      `)
			})

			t.Run("All required and optional args on mixedList", func(t *testing.T) {
				ExpectValid(t, `
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
				ExpectErrors(t, `
        {
          complicatedArgs {
            multipleReqs(req2: "two", req1: "one")
          }
        }
      `)([]Err{
					{
						message:   `Int cannot represent non-integer value: "two"`,
						locations: []Loc{{line: 4, column: 32}},
					},
					{
						message:   `Int cannot represent non-integer value: "one"`,
						locations: []Loc{{line: 4, column: 45}},
					},
				})
			})

			t.Run("Incorrect value and missing argument (ProvidedRequiredArgumentsRule)", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            multipleReqs(req1: "one")
          }
        }
      `)([]Err{
					{
						message:   `Int cannot represent non-integer value: "one"`,
						locations: []Loc{{line: 4, column: 32}},
					},
				})
			})

			t.Run("Null value", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            multipleReqs(req1: null)
          }
        }
      `)([]Err{
					{
						message:   `Expected value of type "Int!", found null.`,
						locations: []Loc{{line: 4, column: 32}},
					},
				})
			})
		})

		t.Run("Valid input object value", func(t *testing.T) {
			t.Run("Optional arg, despite required field in type", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            complexArgField
          }
        }
      `)
			})

			t.Run("Partial object, only required", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            complexArgField(complexArg: { requiredField: true })
          }
        }
      `)
			})

			t.Run("Partial object, required field can be falsy", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            complexArgField(complexArg: { requiredField: false })
          }
        }
      `)
			})

			t.Run("Partial object, including required", func(t *testing.T) {
				ExpectValid(t, `
        {
          complicatedArgs {
            complexArgField(complexArg: { requiredField: true, intField: 4 })
          }
        }
      `)
			})

			t.Run("Full object", func(t *testing.T) {
				ExpectValid(t, `
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
				ExpectValid(t, `
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
				ExpectErrors(t, `
        {
          complicatedArgs {
            complexArgField(complexArg: { intField: 4 })
          }
        }
      `)([]Err{
					{
						message:   `Field "ComplexInput.requiredField" of required type "Boolean!" was not provided.`,
						locations: []Loc{{line: 4, column: 41}},
					},
				})
			})

			t.Run("Partial object, invalid field type", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            complexArgField(complexArg: {
              stringListField: ["one", 2],
              requiredField: true,
            })
          }
        }
      `)([]Err{
					{
						message:   "String cannot represent a non string value: 2",
						locations: []Loc{{line: 5, column: 40}},
					},
				})
			})

			t.Run("Partial object, null to non-null field", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            complexArgField(complexArg: {
              requiredField: true,
              nonNullField: null,
            })
          }
        }
      `)([]Err{
					{
						message:   `Expected value of type "Boolean!", found null.`,
						locations: []Loc{{line: 6, column: 29}},
					},
				})
			})

			t.Run("Partial object, unknown field arg", func(t *testing.T) {
				ExpectErrors(t, `
        {
          complicatedArgs {
            complexArgField(complexArg: {
              requiredField: true,
              invalidField: "value"
            })
          }
        }
      `)([]Err{
					{
						message:   `Field "invalidField" is not defined by type "ComplexInput".`,
						locations: []Loc{{line: 6, column: 15}},
					},
				})
			})

			t.Run("Partial object, unknown field arg with suggestions", func(t *testing.T) {
				t.Skip(NotSupportedSuggestionsSkipMsg)

				ExpectErrors(t, `
        {
          complicatedArgs {
            complexArgField(complexArg: {
              requiredField: true,
              invalidField: "value"
            })
          }
        }
      `)([]Err{
					{
						message:   `Field "invalidField" is not defined by type "ComplexInput". Did you mean "intField"?`,
						locations: []Loc{{line: 6, column: 15}},
					},
				})
			})

		})

		t.Run("Directive arguments", func(t *testing.T) {
			t.Run("with directives of valid types", func(t *testing.T) {
				ExpectValid(t, `
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
				ExpectErrors(t, `
        {
          dog @include(if: "yes") {
            name @skip(if: ENUM)
          }
        }
      `)([]Err{
					{
						message:   `Boolean cannot represent a non boolean value: "yes"`,
						locations: []Loc{{line: 3, column: 28}},
					},
					{
						message:   "Boolean cannot represent a non boolean value: ENUM",
						locations: []Loc{{line: 4, column: 28}},
					},
				})
			})
		})

		t.Run("Variable default values", func(t *testing.T) {
			t.Run("variables with valid default values", func(t *testing.T) {
				ExpectValid(t, `
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
				ExpectValid(t, `
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
				ExpectErrors(t, `
        query WithDefaultValues(
          $a: Int! = null,
          $b: String! = null,
          $c: ComplexInput = { requiredField: null, intField: null }
        ) {
          dog { name }
        }
      `)([]Err{
					{
						message:   `Expected value of type "Int!", found null.`,
						locations: []Loc{{line: 3, column: 22}},
					},
					{
						message:   `Expected value of type "String!", found null.`,
						locations: []Loc{{line: 4, column: 25}},
					},
					{
						message:   `Expected value of type "Boolean!", found null.`,
						locations: []Loc{{line: 5, column: 47}},
					},
				})
			})

			t.Run("variables with invalid default values", func(t *testing.T) {
				ExpectErrors(t, `
        query InvalidDefaultValues(
          $a: Int = "one",
          $b: String = 4,
          $c: ComplexInput = "NotVeryComplex"
        ) {
          dog { name }
        }
      `)([]Err{
					{
						message:   `Int cannot represent non-integer value: "one"`,
						locations: []Loc{{line: 3, column: 21}},
					},
					{
						message:   "String cannot represent a non string value: 4",
						locations: []Loc{{line: 4, column: 24}},
					},
					{
						message:   `Expected value of type "ComplexInput", found "NotVeryComplex".`,
						locations: []Loc{{line: 5, column: 30}},
					},
				})
			})

			t.Run("variables with complex invalid default values", func(t *testing.T) {
				ExpectErrors(t, `
        query WithDefaultValues(
          $a: ComplexInput = { requiredField: 123, intField: "abc" }
        ) {
          dog { name }
        }
      `)([]Err{
					{
						message:   "Boolean cannot represent a non boolean value: 123",
						locations: []Loc{{line: 3, column: 47}},
					},
					{
						message:   `Int cannot represent non-integer value: "abc"`,
						locations: []Loc{{line: 3, column: 62}},
					},
				})
			})

			t.Run("complex variables missing required field", func(t *testing.T) {
				ExpectErrors(t, `
        query MissingRequiredField($a: ComplexInput = {intField: 3}) {
          dog { name }
        }
      `)([]Err{
					{
						message:   `Field "ComplexInput.requiredField" of required type "Boolean!" was not provided.`,
						locations: []Loc{{line: 2, column: 55}},
					},
				})
			})

			t.Run("list variables with invalid item", func(t *testing.T) {
				ExpectErrors(t, `
        query InvalidItem($a: [String] = ["one", 2]) {
          dog { name }
        }
      `)([]Err{
					{
						message:   "String cannot represent a non string value: 2",
						locations: []Loc{{line: 2, column: 50}},
					},
				})
			})
		})
	})

}
