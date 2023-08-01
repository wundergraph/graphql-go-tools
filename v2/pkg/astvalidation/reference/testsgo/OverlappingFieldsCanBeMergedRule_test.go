package testsgo

import (
	"testing"
)

func TestOverlappingFieldsCanBeMergedRule(t *testing.T) {
	t.Skip("part of the tests works. for errors message formats differs. locations is missing")

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, OverlappingFieldsCanBeMergedRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	ExpectErrorsWithSchema := func(t *testing.T, schema string, queryStr string) ResultCompare {
		return ExpectValidationErrorsWithSchema(t,
			schema,
			OverlappingFieldsCanBeMergedRule,
			queryStr,
		)
	}

	ExpectValidWithSchema := func(t *testing.T, schema string, queryStr string) {
		ExpectErrorsWithSchema(t, schema, queryStr)([]Err{})
	}

	t.Run("Validate: Overlapping fields can be merged", func(t *testing.T) {
		t.Run("unique fields", func(t *testing.T) {
			ExpectValid(t, `
      fragment uniqueFields on Dog {
        name
        nickname
      }
    `)
		})

		t.Run("identical fields", func(t *testing.T) {
			ExpectValid(t, `
      fragment mergeIdenticalFields on Dog {
        name
        name
      }
    `)
		})

		t.Run("identical fields with identical args", func(t *testing.T) {
			ExpectValid(t, `
      fragment mergeIdenticalFieldsWithIdenticalArgs on Dog {
        doesKnowCommand(dogCommand: SIT)
        doesKnowCommand(dogCommand: SIT)
      }
    `)
		})

		t.Run("identical fields with identical directives", func(t *testing.T) {
			ExpectValid(t, `
      fragment mergeSameFieldsWithSameDirectives on Dog {
        name @include(if: true)
        name @include(if: true)
      }
    `)
		})

		t.Run("different args with different aliases", func(t *testing.T) {
			ExpectValid(t, `
      fragment differentArgsWithDifferentAliases on Dog {
        knowsSit: doesKnowCommand(dogCommand: SIT)
        knowsDown: doesKnowCommand(dogCommand: DOWN)
      }
    `)
		})

		t.Run("different directives with different aliases", func(t *testing.T) {
			ExpectValid(t, `
      fragment differentDirectivesWithDifferentAliases on Dog {
        nameIfTrue: name @include(if: true)
        nameIfFalse: name @include(if: false)
      }
    `)
		})

		t.Run("different skip/include directives accepted", func(t *testing.T) {
			// Note: Differing skip/include directives don"t create an ambiguous return
			// value and are acceptable in conditions where differing runtime values
			// may have the same desired effect of including or skipping a field.
			ExpectValid(t, `
      fragment differentDirectivesWithDifferentAliases on Dog {
        name @include(if: true)
        name @include(if: false)
      }
    `)
		})

		t.Run("Same aliases with different field targets", func(t *testing.T) {
			ExpectErrors(t, `
      fragment sameAliasesWithDifferentFieldTargets on Dog {
        fido: name
        fido: nickname
      }
    `)([]Err{
				{
					message: `Fields "fido" conflict because "name" and "nickname" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 9},
					},
				},
			})
		})

		t.Run("Same aliases allowed on non-overlapping fields", func(t *testing.T) {
			// This is valid since no object can be both a "Dog" and a "Cat", thus
			// these fields can never overlap.
			ExpectValid(t, `
      fragment sameAliasesWithDifferentFieldTargets on Pet {
        ... on Dog {
          name
        }
        ... on Cat {
          name: nickname
        }
      }
    `)
		})

		t.Run("Alias masking direct field access", func(t *testing.T) {
			ExpectErrors(t, `
      fragment aliasMaskingDirectFieldAccess on Dog {
        name: nickname
        name
      }
    `)([]Err{
				{
					message: `Fields "name" conflict because "nickname" and "name" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 9},
					},
				},
			})
		})

		t.Run("different args, second adds an argument", func(t *testing.T) {
			ExpectErrors(t, `
      fragment conflictingArgs on Dog {
        doesKnowCommand
        doesKnowCommand(dogCommand: HEEL)
      }
    `)([]Err{
				{
					message: `Fields "doesKnowCommand" conflict because they have differing arguments. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 9},
					},
				},
			})
		})

		t.Run("different args, second missing an argument", func(t *testing.T) {
			ExpectErrors(t, `
      fragment conflictingArgs on Dog {
        doesKnowCommand(dogCommand: SIT)
        doesKnowCommand
      }
    `)([]Err{
				{
					message: `Fields "doesKnowCommand" conflict because they have differing arguments. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 9},
					},
				},
			})
		})

		t.Run("conflicting arg values", func(t *testing.T) {
			ExpectErrors(t, `
      fragment conflictingArgs on Dog {
        doesKnowCommand(dogCommand: SIT)
        doesKnowCommand(dogCommand: HEEL)
      }
    `)([]Err{
				{
					message: `Fields "doesKnowCommand" conflict because they have differing arguments. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 9},
					},
				},
			})
		})

		t.Run("conflicting arg names", func(t *testing.T) {
			ExpectErrors(t, `
      fragment conflictingArgs on Dog {
        isAtLocation(x: 0)
        isAtLocation(y: 0)
      }
    `)([]Err{
				{
					message: `Fields "isAtLocation" conflict because they have differing arguments. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 9},
					},
				},
			})
		})

		t.Run("allows different args where no conflict is possible", func(t *testing.T) {
			// This is valid since no object can be both a "Dog" and a "Cat", thus
			// these fields can never overlap.
			ExpectValid(t, `
      fragment conflictingArgs on Pet {
        ... on Dog {
          name(surname: true)
        }
        ... on Cat {
          name
        }
      }
    `)
		})

		t.Run("encounters conflict in fragments", func(t *testing.T) {
			ExpectErrors(t, `
      {
        ...A
        ...B
      }
      fragment A on Type {
        x: a
      }
      fragment B on Type {
        x: b
      }
    `)([]Err{
				{
					message: `Fields "x" conflict because "a" and "b" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 7, column: 9},
						{line: 10, column: 9},
					},
				},
			})
		})

		t.Run("reports each conflict once", func(t *testing.T) {
			ExpectErrors(t, `
      {
        f1 {
          ...A
          ...B
        }
        f2 {
          ...B
          ...A
        }
        f3 {
          ...A
          ...B
          x: c
        }
      }
      fragment A on Type {
        x: a
      }
      fragment B on Type {
        x: b
      }
    `)([]Err{
				{
					message: `Fields "x" conflict because "a" and "b" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 18, column: 9},
						{line: 21, column: 9},
					},
				},
				{
					message: `Fields "x" conflict because "c" and "a" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 14, column: 11},
						{line: 18, column: 9},
					},
				},
				{
					message: `Fields "x" conflict because "c" and "b" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 14, column: 11},
						{line: 21, column: 9},
					},
				},
			})
		})

		t.Run("deep conflict", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field {
          x: a
        },
        field {
          x: b
        }
      }
    `)([]Err{
				{
					message: `Fields "field" conflict because subfields "x" conflict because "a" and "b" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 11},
						{line: 6, column: 9},
						{line: 7, column: 11},
					},
				},
			})
		})

		t.Run("deep conflict with multiple issues", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field {
          x: a
          y: c
        },
        field {
          x: b
          y: d
        }
      }
    `)([]Err{
				{
					message: `Fields "field" conflict because subfields "x" conflict because "a" and "b" are different fields and subfields "y" conflict because "c" and "d" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 11},
						{line: 5, column: 11},
						{line: 7, column: 9},
						{line: 8, column: 11},
						{line: 9, column: 11},
					},
				},
			})
		})

		t.Run("very deep conflict", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field {
          deepField {
            x: a
          }
        },
        field {
          deepField {
            x: b
          }
        }
      }
    `)([]Err{
				{
					message: `Fields "field" conflict because subfields "deepField" conflict because subfields "x" conflict because "a" and "b" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 4, column: 11},
						{line: 5, column: 13},
						{line: 8, column: 9},
						{line: 9, column: 11},
						{line: 10, column: 13},
					},
				},
			})
		})

		t.Run("reports deep conflict to nearest common ancestor", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field {
          deepField {
            x: a
          }
          deepField {
            x: b
          }
        },
        field {
          deepField {
            y
          }
        }
      }
    `)([]Err{
				{
					message: `Fields "deepField" conflict because subfields "x" conflict because "a" and "b" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 4, column: 11},
						{line: 5, column: 13},
						{line: 7, column: 11},
						{line: 8, column: 13},
					},
				},
			})
		})

		t.Run("reports deep conflict to nearest common ancestor in fragments", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field {
          ...F
        }
        field {
          ...F
        }
      }
      fragment F on T {
        deepField {
          deeperField {
            x: a
          }
          deeperField {
            x: b
          }
        },
        deepField {
          deeperField {
            y
          }
        }
      }
    `)([]Err{
				{
					message: `Fields "deeperField" conflict because subfields "x" conflict because "a" and "b" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 12, column: 11},
						{line: 13, column: 13},
						{line: 15, column: 11},
						{line: 16, column: 13},
					},
				},
			})
		})

		t.Run("reports deep conflict in nested fragments", func(t *testing.T) {
			ExpectErrors(t, `
      {
        field {
          ...F
        }
        field {
          ...I
        }
      }
      fragment F on T {
        x: a
        ...G
      }
      fragment G on T {
        y: c
      }
      fragment I on T {
        y: d
        ...J
      }
      fragment J on T {
        x: b
      }
    `)([]Err{
				{
					message: `Fields "field" conflict because subfields "x" conflict because "a" and "b" are different fields and subfields "y" conflict because "c" and "d" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 3, column: 9},
						{line: 11, column: 9},
						{line: 15, column: 9},
						{line: 6, column: 9},
						{line: 22, column: 9},
						{line: 18, column: 9},
					},
				},
			})
		})

		t.Run("ignores unknown fragments", func(t *testing.T) {
			ExpectValid(t, `
      {
        field
        ...Unknown
        ...Known
      }

      fragment Known on T {
        field
        ...OtherUnknown
      }
    `)
		})

		t.Run("return types must be unambiguous", func(t *testing.T) {
			schema := BuildSchema(`
      interface SomeBox {
        deepBox: SomeBox
        unrelatedField: String
      }

      type StringBox implements SomeBox {
        scalar: String
        deepBox: StringBox
        unrelatedField: String
        listStringBox: [StringBox]
        stringBox: StringBox
        intBox: IntBox
      }

      type IntBox implements SomeBox {
        scalar: Int
        deepBox: IntBox
        unrelatedField: String
        listStringBox: [StringBox]
        stringBox: StringBox
        intBox: IntBox
      }

      interface NonNullStringBox1 {
        scalar: String!
      }

      type NonNullStringBox1Impl implements SomeBox & NonNullStringBox1 {
        scalar: String!
        unrelatedField: String
        deepBox: SomeBox
      }

      interface NonNullStringBox2 {
        scalar: String!
      }

      type NonNullStringBox2Impl implements SomeBox & NonNullStringBox2 {
        scalar: String!
        unrelatedField: String
        deepBox: SomeBox
      }

      type Connection {
        edges: [Edge]
      }

      type Edge {
        node: Node
      }

      type Node {
        id: ID
        name: String
      }

      type Query {
        someBox: SomeBox
        connection: Connection
      }
    `)

			t.Run("conflicting return types which potentially overlap", func(t *testing.T) {
				// This is invalid since an object could potentially be both the Object
				// type IntBox and the interface type NonNullStringBox1. While that
				// condition does not exist in the current schema, the schema could
				// expand in the future to allow this. Thus it is invalid.
				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            someBox {
              ...on IntBox {
                scalar
              }
              ...on NonNullStringBox1 {
                scalar
              }
            }
          }
        `,
				)([]Err{
					{
						message: `Fields "scalar" conflict because they return conflicting types "Int" and "String!". Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 5, column: 17},
							{line: 8, column: 17},
						},
					},
				})
			})

			t.Run("compatible return shapes on different return types", func(t *testing.T) {
				// In this case `deepBox` returns `SomeBox` in the first usage, and
				// `StringBox` in the second usage. These return types are not the same!
				// however this is valid because the return *shapes* are compatible.
				ExpectValidWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on SomeBox {
                deepBox {
                  unrelatedField
                }
              }
              ... on StringBox {
                deepBox {
                  unrelatedField
                }
              }
            }
          }
        `,
				)
			})

			t.Run("disallows differing return types despite no overlap", func(t *testing.T) {
				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on IntBox {
                scalar
              }
              ... on StringBox {
                scalar
              }
            }
          }
        `,
				)([]Err{
					{
						message: `Fields "scalar" conflict because they return conflicting types "Int" and "String". Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 5, column: 17},
							{line: 8, column: 17},
						},
					},
				})
			})

			t.Run("reports correctly when a non-exclusive follows an exclusive", func(t *testing.T) {
				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on IntBox {
                deepBox {
                  ...X
                }
              }
            }
            someBox {
              ... on StringBox {
                deepBox {
                  ...Y
                }
              }
            }
            memoed: someBox {
              ... on IntBox {
                deepBox {
                  ...X
                }
              }
            }
            memoed: someBox {
              ... on StringBox {
                deepBox {
                  ...Y
                }
              }
            }
            other: someBox {
              ...X
            }
            other: someBox {
              ...Y
            }
          }
          fragment X on SomeBox {
            scalar
          }
          fragment Y on SomeBox {
            scalar: unrelatedField
          }
        `,
				)([]Err{
					{
						message: `Fields "other" conflict because subfields "scalar" conflict because "scalar" and "unrelatedField" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 31, column: 13},
							{line: 39, column: 13},
							{line: 34, column: 13},
							{line: 42, column: 13},
						},
					},
				})
			})

			t.Run("disallows differing return type nullability despite no overlap", func(t *testing.T) {
				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on NonNullStringBox1 {
                scalar
              }
              ... on StringBox {
                scalar
              }
            }
          }
        `,
				)([]Err{
					{
						message: `Fields "scalar" conflict because they return conflicting types "String!" and "String". Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 5, column: 17},
							{line: 8, column: 17},
						},
					},
				})
			})

			t.Run("disallows differing return type list despite no overlap", func(t *testing.T) {
				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on IntBox {
                box: listStringBox {
                  scalar
                }
              }
              ... on StringBox {
                box: stringBox {
                  scalar
                }
              }
            }
          }
        `,
				)([]Err{
					{
						message: `Fields "box" conflict because they return conflicting types "[StringBox]" and "StringBox". Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 5, column: 17},
							{line: 10, column: 17},
						},
					},
				})

				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on IntBox {
                box: stringBox {
                  scalar
                }
              }
              ... on StringBox {
                box: listStringBox {
                  scalar
                }
              }
            }
          }
        `,
				)([]Err{
					{
						message: `Fields "box" conflict because they return conflicting types "StringBox" and "[StringBox]". Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 5, column: 17},
							{line: 10, column: 17},
						},
					},
				})
			})

			t.Run("disallows differing subfields", func(t *testing.T) {
				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on IntBox {
                box: stringBox {
                  val: scalar
                  val: unrelatedField
                }
              }
              ... on StringBox {
                box: stringBox {
                  val: scalar
                }
              }
            }
          }
        `,
				)([]Err{
					{
						message: `Fields "val" conflict because "scalar" and "unrelatedField" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 6, column: 19},
							{line: 7, column: 19},
						},
					},
				})
			})

			t.Run("disallows differing deep return types despite no overlap", func(t *testing.T) {
				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on IntBox {
                box: stringBox {
                  scalar
                }
              }
              ... on StringBox {
                box: intBox {
                  scalar
                }
              }
            }
          }
        `,
				)([]Err{
					{
						message: `Fields "box" conflict because subfields "scalar" conflict because they return conflicting types "String" and "Int". Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 5, column: 17},
							{line: 6, column: 19},
							{line: 10, column: 17},
							{line: 11, column: 19},
						},
					},
				})
			})

			t.Run("allows non-conflicting overlapping types", func(t *testing.T) {
				ExpectValidWithSchema(t,
					schema,
					`
          {
            someBox {
              ... on IntBox {
                scalar: unrelatedField
              }
              ... on StringBox {
                scalar
              }
            }
          }
        `,
				)
			})

			t.Run("same wrapped scalar return types", func(t *testing.T) {
				ExpectValidWithSchema(t,
					schema,
					`
          {
            someBox {
              ...on NonNullStringBox1 {
                scalar
              }
              ...on NonNullStringBox2 {
                scalar
              }
            }
          }
        `,
				)
			})

			t.Run("allows inline fragments without type condition", func(t *testing.T) {
				ExpectValidWithSchema(t,
					schema,
					`
          {
            a
            ... {
              a
            }
          }
        `,
				)
			})

			t.Run("compares deep types including list", func(t *testing.T) {
				ExpectErrorsWithSchema(t,
					schema,
					`
          {
            connection {
              ...edgeID
              edges {
                node {
                  id: name
                }
              }
            }
          }

          fragment edgeID on Connection {
            edges {
              node {
                id
              }
            }
          }
        `,
				)([]Err{
					{
						message: `Fields "edges" conflict because subfields "node" conflict because subfields "id" conflict because "name" and "id" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
						locations: []Loc{
							{line: 5, column: 15},
							{line: 6, column: 17},
							{line: 7, column: 19},
							{line: 14, column: 13},
							{line: 15, column: 15},
							{line: 16, column: 17},
						},
					},
				})
			})

			t.Run("ignores unknown types", func(t *testing.T) {
				ExpectValidWithSchema(t,
					schema,
					`
          {
            someBox {
              ...on UnknownType {
                scalar
              }
              ...on NonNullStringBox2 {
                scalar
              }
            }
          }
        `,
				)
			})

			t.Run("works for field names that are JS keywords", func(t *testing.T) {
				schemaWithKeywords := BuildSchema(`
        type Foo {
          constructor: String
        }

        type Query {
          foo: Foo
        }
      `)

				ExpectValidWithSchema(t,
					schemaWithKeywords,
					`
          {
            foo {
              constructor
            }
          }
        `,
				)
			})
		})

		t.Run("does not infinite loop on recursive fragment", func(t *testing.T) {
			ExpectValid(t, `
      fragment fragA on Human { name, relatives { name, ...fragA } }
    `)
		})

		t.Run("does not infinite loop on immediately recursive fragment", func(t *testing.T) {
			ExpectValid(t, `
      fragment fragA on Human { name, ...fragA }
    `)
		})

		t.Run("does not infinite loop on transitively recursive fragment", func(t *testing.T) {
			ExpectValid(t, `
      fragment fragA on Human { name, ...fragB }
      fragment fragB on Human { name, ...fragC }
      fragment fragC on Human { name, ...fragA }
    `)
		})

		t.Run("finds invalid case even with immediately recursive fragment", func(t *testing.T) {
			ExpectErrors(t, `
      fragment sameAliasesWithDifferentFieldTargets on Dog {
        ...sameAliasesWithDifferentFieldTargets
        fido: name
        fido: nickname
      }
    `)([]Err{
				{
					message: `Fields "fido" conflict because "name" and "nickname" are different fields. Use different aliases on the fields to fetch both if this was intentional.`,
					locations: []Loc{
						{line: 4, column: 9},
						{line: 5, column: 9},
					},
				},
			})
		})
	})

}
