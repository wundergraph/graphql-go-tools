package testsgo

import (
	"testing"
)

func TestKnownArgumentNamesRule(t *testing.T) {
	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, KnownArgumentNamesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	ExpectSDLErrors := func(t *testing.T, sdlStr string, schemas ...string) ResultCompare {
		schema := ""
		if len(schemas) > 0 {
			schema = schemas[0]
		}
		return ExpectSDLValidationErrors(t,
			schema,
			KnownArgumentNamesOnDirectivesRule,
			sdlStr,
		)
	}

	ExpectValidSDL := func(t *testing.T, sdlStr string, schema ...string) {
		ExpectSDLErrors(t, sdlStr)([]Err{})
	}

	t.Run("Validate: Known argument names", func(t *testing.T) {
		t.Run("single arg is known", func(t *testing.T) {
			ExpectValid(t, `
      fragment argOnRequiredArg on Dog {
        doesKnowCommand(dogCommand: SIT)
      }
    `)
		})

		t.Run("multiple args are known", func(t *testing.T) {
			ExpectValid(t, `
      fragment multipleArgs on ComplicatedArgs {
        multipleReqs(req1: 1, req2: 2)
      }
    `)
		})

		t.Run("ignores args of unknown fields", func(t *testing.T) {
			ExpectValid(t, `
      fragment argOnUnknownField on Dog {
        unknownField(unknownArg: SIT)
      }
    `)
		})

		t.Run("multiple args in reverse order are known", func(t *testing.T) {
			ExpectValid(t, `
      fragment multipleArgsReverseOrder on ComplicatedArgs {
        multipleReqs(req2: 2, req1: 1)
      }
    `)
		})

		t.Run("no args on optional arg", func(t *testing.T) {
			ExpectValid(t, `
      fragment noArgOnOptionalArg on Dog {
        isHouseTrained
      }
    `)
		})

		t.Run("args are known deeply", func(t *testing.T) {
			ExpectValid(t, `
      {
        dog {
          doesKnowCommand(dogCommand: SIT)
        }
        human {
          pets { # we are using pets instead of pet: to not fail in visitor with unknown field name on a type
            ... on Dog {
              doesKnowCommand(dogCommand: SIT)
            }
          }
        }
      }
    `)
		})

		t.Run("directive args are known", func(t *testing.T) {
			ExpectValid(t, `
      {
        dog @skip(if: true)
      }
    `)
		})

		t.Run("field args are invalid", func(t *testing.T) {
			ExpectErrors(t, `
      {
        dog @skip(unless: true)
      }
    `)([]Err{
				{
					message:   `Unknown argument "unless" on directive "@skip".`,
					locations: []Loc{{line: 3, column: 19}},
				},
			})
		})

		t.Run("directive without args is valid", func(t *testing.T) {
			ExpectValid(t, `
      {
        dog @onField
      }
    `)
		})

		t.Run("arg passed to directive without arg is reported", func(t *testing.T) {
			ExpectErrors(t, `
      {
        dog @onField(if: true)
      }
    `)([]Err{
				{
					message:   `Unknown argument "if" on directive "@onField".`,
					locations: []Loc{{line: 3, column: 22}},
				},
			})
		})

		t.Run("misspelled directive args are reported", func(t *testing.T) {
			ExpectErrors(t, `
      {
        dog @skip(iff: true)
      }
    `)([]Err{
				{
					message:   `Unknown argument "iff" on directive "@skip".`,
					locations: []Loc{{line: 3, column: 19}},
				},
			})
		})

		t.Run("misspelled directive args are reported with suggestions", func(t *testing.T) {
			t.Skip(NotSupportedSuggestionsSkipMsg)

			ExpectErrors(t, `
      {
        dog @skip(iff: true)
      }
    `)([]Err{
				{
					message:   `Unknown argument "iff" on directive "@skip". Did you mean "if"?`,
					locations: []Loc{{line: 3, column: 19}},
				},
			})
		})

		t.Run("invalid arg name", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidArgName on Dog {
        doesKnowCommand(unknown: true)
      }
    `)([]Err{
				{
					message:   `Unknown argument "unknown" on field "Dog.doesKnowCommand".`,
					locations: []Loc{{line: 3, column: 25}},
				},
			})
		})

		t.Run("misspelled arg name is reported", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidArgName on Dog {
        doesKnowCommand(DogCommand: true)
      }
    `)([]Err{
				{
					message:   `Unknown argument "DogCommand" on field "Dog.doesKnowCommand".`,
					locations: []Loc{{line: 3, column: 25}},
				},
			})
		})

		t.Run("misspelled arg name is reported with suggestions", func(t *testing.T) {
			t.Skip(NotSupportedSuggestionsSkipMsg)

			ExpectErrors(t, `
      fragment invalidArgName on Dog {
        doesKnowCommand(DogCommand: true)
      }
    `)([]Err{
				{
					message:   `Unknown argument "DogCommand" on field "Dog.doesKnowCommand". Did you mean "dogCommand"?`,
					locations: []Loc{{line: 3, column: 25}},
				},
			})
		})

		t.Run("unknown args amongst known args", func(t *testing.T) {
			ExpectErrors(t, `
      fragment oneGoodArgOneInvalidArg on Dog {
        doesKnowCommand(whoKnows: 1, dogCommand: SIT, unknown: true)
      }
    `)([]Err{
				{
					message:   `Unknown argument "whoKnows" on field "Dog.doesKnowCommand".`,
					locations: []Loc{{line: 3, column: 25}},
				},
				{
					message:   `Unknown argument "unknown" on field "Dog.doesKnowCommand".`,
					locations: []Loc{{line: 3, column: 55}},
				},
			})
		})

		t.Run("unknown args deeply", func(t *testing.T) {
			ExpectErrors(t, `
      {
        dog {
          doesKnowCommand(unknown: true)
        }
        human {
          pets { # we are using pets instead of pet: to not fail in visitor with unknown field name on a type
            ... on Dog {
              doesKnowCommand(unknown: true)
            }
          }
        }
      }
    `)([]Err{
				{
					message:   `Unknown argument "unknown" on field "Dog.doesKnowCommand".`,
					locations: []Loc{{line: 4, column: 27}},
				},
				{
					message:   `Unknown argument "unknown" on field "Dog.doesKnowCommand".`,
					locations: []Loc{{line: 9, column: 31}},
				},
			})
		})

		t.Run("within SDL", func(t *testing.T) {
			t.Skip("Definition directive args validation is not supported yet")

			t.Run("known arg on directive defined inside SDL", func(t *testing.T) {
				ExpectValidSDL(t, `
        type Query {
          foo: String @test(arg: "")
        }

        directive @test(arg: String) on FIELD_DEFINITION
      `)
			})

			t.Run("unknown arg on directive defined inside SDL", func(t *testing.T) {
				ExpectSDLErrors(t, `
        type Query {
          foo: String @test(unknown: "")
        }

        directive @test(arg: String) on FIELD_DEFINITION
      `)([]Err{
					{
						message:   `Unknown argument "unknown" on directive "@test".`,
						locations: []Loc{{line: 3, column: 29}},
					},
				})
			})

			t.Run("misspelled arg name is reported on directive defined inside SDL", func(t *testing.T) {
				ExpectSDLErrors(t, `
        type Query {
          foo: String @test(agr: "")
        }

        directive @test(arg: String) on FIELD_DEFINITION
      `)([]Err{
					{
						message:   `Unknown argument "agr" on directive "@test". Did you mean "arg"?`,
						locations: []Loc{{line: 3, column: 29}},
					},
				})
			})

			t.Run("unknown arg on standard directive", func(t *testing.T) {
				ExpectSDLErrors(t, `
        type Query {
          foo: String @deprecated(unknown: "")
        }
      `)([]Err{
					{
						message:   `Unknown argument "unknown" on directive "@deprecated".`,
						locations: []Loc{{line: 3, column: 35}},
					},
				})
			})

			t.Run("unknown arg on overridden standard directive", func(t *testing.T) {
				ExpectSDLErrors(t, `
        type Query {
          foo: String @deprecated(reason: "")
        }
        directive @deprecated(arg: String) on FIELD
      `)([]Err{
					{
						message:   `Unknown argument "reason" on directive "@deprecated".`,
						locations: []Loc{{line: 3, column: 35}},
					},
				})
			})

			t.Run("unknown arg on directive defined in schema extension", func(t *testing.T) {
				schema := BuildSchema(`
        type Query {
          foo: String
        }
      `)
				ExpectSDLErrors(t,
					`
          directive @test(arg: String) on OBJECT

          extend type Query  @test(unknown: "")
        `,
					schema,
				)([]Err{
					{
						message:   `Unknown argument "unknown" on directive "@test".`,
						locations: []Loc{{line: 4, column: 36}},
					},
				})
			})

			t.Run("unknown arg on directive used in schema extension", func(t *testing.T) {
				schema := BuildSchema(`
        directive @test(arg: String) on OBJECT

        type Query {
          foo: String
        }
      `)
				ExpectSDLErrors(t,
					`
          extend type Query @test(unknown: "")
        `,
					schema,
				)([]Err{
					{
						message:   `Unknown argument "unknown" on directive "@test".`,
						locations: []Loc{{line: 2, column: 35}},
					},
				})
			})
		})
	})

}
