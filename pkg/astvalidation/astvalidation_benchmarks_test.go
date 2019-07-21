package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"testing"
)

func BenchmarkExecutionValidation(b *testing.B) {

	run := func(b *testing.B, operation string, rule Rule, valid ValidationState) {

		schemaInput := &input.Input{}
		schemaInput.ResetInputBytes([]byte(testDefinition))

		operationInput := &input.Input{}
		operationInput.ResetInputBytes([]byte(operation))

		schemaDocument := ast.NewDocument()
		operationDocument := ast.NewDocument()

		parse := astparser.NewParser()

		err := parse.Parse(schemaInput, schemaDocument)
		if err != nil {
			b.Fatal(err)
		}
		err = parse.Parse(operationInput, operationDocument)
		if err != nil {
			b.Fatal(err)
		}

		validator := NewOperationValidator(rule)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			validator.Validate(operationInput, schemaInput, operationDocument, schemaDocument)
		}
	}

	// 5.1 Documents
	// 5.1.1 Executable Definitions
	// -> won't be addressed as the parser will only parse operation- and fragment definitions
	// when parsing executable definitions

	b.Run("5.2 Operations", func(b *testing.B) {
		b.Run("5.2.1 Named Operation Definitions", func(b *testing.B) {
			b.Run("5.2.1.1 Operation Name Uniqueness", func(b *testing.B) {
				b.Run("92", func(b *testing.B) {
					run(b, `
								query getDogName {
  									dog {
    									name
  									}
								}
								query getOwnerName {
  									dog {
    									owner {
      										name
    									}
  									}
								}`,
						OperationNameUniqueness(), Valid)
				})
				b.Run("93", func(b *testing.B) {
					run(b, `
								query getName {
									dog {
    									name
									}
								}
								query getName {
  									dog {
    									owner {
      										name
    									}
  									}
								}`,
						OperationNameUniqueness(), Invalid)
				})
				b.Run("94", func(b *testing.B) {
					run(b, `	query dogOperation {
  									dog {
  										name
  									}
  								}
  								mutation dogOperation {
    								mutateDog {
      									id
    								}
  								}`,
						OperationNameUniqueness(), Invalid)
				})
			})
		})
		b.Run("5.2.2 Anonymous Operation Definitions", func(b *testing.B) {
			b.Run("5.2.2.1 Lone Anonymous Operation", func(b *testing.B) {
				b.Run("95", func(b *testing.B) {
					run(b, `	{
  							  		dog {
      									name
    								}
  								}`,
						LoneAnonymousOperation(), Valid)
				})
				b.Run("96", func(b *testing.B) {
					run(b, `	{
  									dog {
  										name
  									}
  								}
  								query getName {
    								dog {
  										owner {
  											name
  										}
  									}
  								}`,
						LoneAnonymousOperation(), Invalid)
				})
				b.Run("96 variant", func(b *testing.B) {
					run(b, `	query getDogName {
  									dog {
  										name
  									}
  								}
  								query getOwnerName {
    								dog {
  										owner {
  											name
  										}
  									}
  								}`,
						LoneAnonymousOperation(), Valid)
				})
			})
		})
		b.Run("5.2.3 Subscription Operation Definitions", func(b *testing.B) {
			b.Run("5.2.3.1 Single root field", func(b *testing.B) {
				b.Run("97", func(b *testing.B) {
					run(b, `	subscription sub {
  									newMessage {
    									body
    									sender
  									}
								}`,
						SubscriptionSingleRootField(), Valid)
				})
				b.Run("97 variant", func(b *testing.B) {
					run(b, `	query sub {
  									foo
									bar
								}`,
						SubscriptionSingleRootField(), Valid)
				})
				b.Run("97 variant", func(b *testing.B) {
					run(b, `	subscription sub {
  									... { foo }
  									... { bar }
								}`,
						SubscriptionSingleRootField(), Invalid)
				})
				b.Run("98", func(b *testing.B) {
					run(b, `	subscription sub {
  									...newMessageFields
								}
								fragment newMessageFields on Subscription {
  									newMessage {
    									body
    									sender
  									}
								}`,
						SubscriptionSingleRootField(), Valid)
				})
				b.Run("99", func(b *testing.B) {
					run(b, `	subscription sub {
  									newMessage {
    									body
    									sender
  									}
  									disallowedSecondRootField
								}`,
						SubscriptionSingleRootField(), Invalid)
				})
				b.Run("100", func(b *testing.B) {
					run(b, `	subscription sub {
  									...multipleSubscriptions
								}
								fragment multipleSubscriptions on Subscription {
  									newMessage {
    									body
    									sender
  									}
  									disallowedSecondRootField
								}`,
						SubscriptionSingleRootField(), Invalid)
				})
				b.Run("101", func(b *testing.B) {
					run(b, `	subscription sub {
  									newMessage {
    									body
    									sender
  									}
  									__typename
								}`,
						SubscriptionSingleRootField(), Invalid)
				})
			})
		})
	})
	b.Run("5.3 FieldSelections", func(b *testing.B) {
		b.Run("5.3.1 Field Selections on Objects, Interfaces, and Unions Types", func(b *testing.B) {
			b.Run("102", func(b *testing.B) {
				run(b, `	{
								dog {
									...aliasedLyingFieldTargetNotDefined
								}
							}
							fragment aliasedLyingFieldTargetNotDefined on Dog {
								barkVolume: kawVolume
							}`,
					FieldSelections(), Invalid)
			})
			b.Run("102 variant", func(b *testing.B) {
				run(b, `	{
								dog {
									barkVolume: kawVolume
								}
							}`,
					FieldSelections(), Invalid)
			})
			b.Run("103", func(b *testing.B) {
				run(b, `	{
								dog {
									...interfaceFieldSelection
								}
							}
							fragment interfaceFieldSelection on Pet {
								name
							}`,
					FieldSelections(), Valid)
			})
			b.Run("104", func(b *testing.B) {
				run(b, `	{
								dog {
									...definedOnImplementorsButNotInterface
								}
							}
							fragment definedOnImplementorsButNotInterface on Pet {
								nickname
							}`,
					FieldSelections(), Invalid)
			})
			b.Run("105", func(b *testing.B) {
				run(b, `	fragment inDirectFieldSelectionOnUnion on CatOrDog {
								__typename
	  							... on Pet {
	    							name
	  							}
	  							... on Dog {
	    							name
	  							}
							}`,
					FieldSelections(), Valid)
			})
			b.Run("105 variant", func(b *testing.B) {
				run(b, `	fragment inDirectFieldSelectionOnUnion on CatOrDog {
								__typename
	  							... on Pet {
	    							name
	  							}
	  							... on Dog {
	    							x
	  							}
							}`,
					FieldSelections(), Invalid)
			})
			b.Run("105 variant", func(b *testing.B) {
				run(b, `	fragment inDirectFieldSelectionOnUnion on CatOrDog {
								__typename
	  							... on Pet {
	    							name
	  							}
	  							... {
	    							x
	  							}
							}`,
					FieldSelections(), Invalid)
			})
			b.Run("106", func(b *testing.B) {
				run(b, ` fragment directFieldSelectionOnUnion on CatOrDog {
								name
								barkVolume
							}`,
					FieldSelections(), Invalid)
			})
			b.Run("106 variant", func(b *testing.B) {
				run(b, ` fragment directFieldSelectionOnUnion on Cat {
								name {
									name
								}
							}`,
					FieldSelections(), Invalid)
			})
		})
		b.Run("5.3.2 Field Selection Merging", func(b *testing.B) {
			b.Run("107", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFields on Dog {
  								name
  								name
  							}
  							fragment mergeIdenticalAliasesAndFields on Dog {
  								otherName: name
  								otherName: name
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("107 variant", func(b *testing.B) {
				run(b, `	query mergeIdenticalFields {
  								dog {
									name
  									name
								}
  							}
  							query mergeIdenticalAliasesAndFields {
  								dog {
									otherName: name
  									otherName: name
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("108", func(b *testing.B) {
				run(b, `	fragment conflictingBecauseAlias on Dog {
  								name: nickname
  								name
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									name: nickname
  									name
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	mutation conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	subscription conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { noString: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra: extras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras { string }
  									extras: mustExtras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									x: extras { string }
  									x: mustExtras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string }
  									extras { string,string3: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string }
  									extras { string,string2: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string }
  									extras { string,string2: string,string3: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras { ... { string },string2: string }
  									extras { ... { string },... { string },string2: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras { ... { string },string2: string }
  									extras { ... { string1: string },string2: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras { ...frag, ...frag }
  									extras { ...frag }
								}
  							}
							fragment frag on Extras { string }`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras {
										... {
											string1: bool
										}
									}
									extras {
										... {
											string1: string
										}
									}
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extras { ...frag }
  									extras { ...frag2 }
								}
  							}
							fragment frag on Extras { string }
							fragment frag2 on Extras { string1: string }`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									extra { looksLikeString: string }
  									extra { looksLikeString: bool }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									name: nickname
  									...nameFrag
								}
  							}
							fragment nameFrag on Dog {
								name
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									name: nickname
  									...nameFrag
								}
  							}
							fragment nameFrag on Dog {
								...nameFrag2
							}
							fragment nameFrag2 on Dog {
								name
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									name: nickname
  									... on Dog {
										... nameFrag
									}
								}
  							}
							fragment nameFrag on Dog {
								name
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("108 variant", func(b *testing.B) {
				run(b, `	query conflictingBecauseAlias {
								dog {
  									name: nickname
  									... {
										name
									}
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			/*b.Run("108 variant", func(b *testing.B) { TODO: uncomment and implement
							run(b,`	query conflictingBecauseAlias {
											dog {
			  									name: nickname
			  									... @include(if: true) {
													name
												}
											}
			  							}`,
								FieldSelectionMerging(), Invalid)
						})
						b.Run("108 variant", func(b *testing.B) {
							run(b,`	query conflictingBecauseAlias {
											dog {
			  									name: nickname
			  									... @include(if: false) {
													name
												}
											}
			  							}`,
								FieldSelectionMerging(), Valid)
						})
						b.Run("108 variant", func(b *testing.B) {
							run(b,`	query conflictingBecauseAlias($include: false) {
											dog {
			  									name: nickname
			  									... @include(if: $include) {
													name
												}
											}
			  							}`,
								FieldSelectionMerging(), Valid)
						})*/
			b.Run("109", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalArgs on Dog {
  								doesKnowCommand(dogCommand: SIT)
  								doesKnowCommand(dogCommand: SIT)
  							}
  							fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: $dogCommand)
    							doesKnowCommand(dogCommand: $dogCommand)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1)
    							doesKnowCommand(dogCommand: 1)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1)
    							doesKnowCommand(dogCommand: 0)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1.1)
    							doesKnowCommand(dogCommand: 1.1)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1.1)
    							doesKnowCommand(dogCommand: 0.1)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: "foo")
    							doesKnowCommand(dogCommand: "foo")
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: "foo")
    							doesKnowCommand(dogCommand: "bar")
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: null)
    							doesKnowCommand(dogCommand: null)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: null)
    							doesKnowCommand(dogCommand: 0)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [1.1])
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [0.1])
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [1.1,1.1])
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "bar"})
  							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {bar: "bar"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "baz"})
    							doesKnowCommand(dogCommand: {bar: "baz"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("109 variant", func(b *testing.B) {
				run(b, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "baz",bar: "bat"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("110", func(b *testing.B) {
				run(b, `	fragment conflictingArgsOnValues on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand: HEEL)
							}`,
					FieldSelectionMerging(), Invalid)
				run(b, `	fragment conflictingArgsOnValues on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand1: HEEL)
							}`,
					FieldSelectionMerging(), Invalid)
				run(b, `	fragment conflictingArgsValueAndVar on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand: $dogCommand)
							}`,
					FieldSelectionMerging(), Invalid)
				run(b, `	fragment conflictingArgsWithVars on Dog {
								doesKnowCommand(dogCommand: $varOne)
								doesKnowCommand(dogCommand: $varTwo)
							}`,
					FieldSelectionMerging(), Invalid)
				run(b, `	fragment differingArgs on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("111", func(b *testing.B) {
				run(b, `	fragment safeDifferingFields on Pet {
								... on Dog {
									volume: barkVolume
								}
								... on Cat {
									volume: meowVolume
								}
							}
							fragment safeDifferingArgs on Pet {
								... on Dog {
									doesKnowCommand(dogCommand: SIT)
								}
								... on Cat {
									doesKnowCommand(catCommand: JUMP)
								}
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									someValue: nickname
								}
								... on Cat {
									someValue: meowVolume
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										string
									}
								}
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										strings
									}
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										string: strings
									}
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										string: mustStrings
									}
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										string: string2
									}
								}
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										noString: string
									}
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										string: bool
									}
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										... {
											string: bool
										}
									}
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										... on CatExtra {
											string
										}
									}
								}
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										... on CatExtra {
											... { string }
										}
									}
								}
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										... on CatExtra {
											...spreadNotExists
										}
									}
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								... on Dog {
									extra {
										string
									}
								}
								... on Cat {
									extra {
										... on CatExtra {
											noString: string
										}
									}
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								...dogFrag
								...catFrag
							}
							fragment dogFrag on Dog {
								someValue: nickname
							}
							fragment catFrag on Cat {
								someValue: meowVolume
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								pet {
									...dogFrag
									...catFrag
								}
							}
							fragment dogFrag on Dog {
								someValue: nickname
							}
							fragment catFrag on Cat {
								someValue: meowVolume
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								catOrDog {
									...catDogFrag
								}
							}
							fragment catDogFrag on CatOrDog {
								...catFrag
								...dogFrag
							}
							fragment catFrag on Cat {
								someValue: meowVolume
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								pet {
									...pet1
									...pet2
								}
							}
							fragment pet1 on Pet {
								name
							}
							fragment pet2 on Pet {
								name
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								pet {
									...pet1
									...pet2
								}
							}
							fragment pet1 on Pet {
								name1: name
							}
							fragment pet2 on Pet {
								name1: nickname
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								catOrDog {
									...catDogFrag
								}
							}
							fragment catDogFrag on CatOrDog {
								...catFrag
								...dogFrag
							}
							fragment catFrag on Cat {
								someValue: meowVolume
							}
							fragment dogFrag on Dog {
								someValue: name
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								...dogFrag
								...catFrag
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}
							fragment catFrag on Cat {
								someValue: meowVolume
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								...dogFrag
								...catFrag
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}
							fragment catFrag on Cat {
								someValue: name
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								...dogFrag
								...catFrag
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}
							fragment catFrag on Cats {
								someValue: meowVolume
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	fragment conflictingDifferingResponses on Pet {
								...dogFrag
								... on Cat {
									someValue: meowVolume
								}
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								pet {
									...dogFrag
									...catFrag
								}
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}
							fragment catFrag on Cat {
								someValue: meowVolume
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								pet {
									...dogFrag
									... on Cat {
										someValue: meowVolume
									}
								}
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}`,
					FieldSelectionMerging(), Valid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								pet {
									...dogFrag
									... on Cat {
										foo
									}
								}
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}`,
					FieldSelectionMerging(), Invalid)
			})
			b.Run("112 variant", func(b *testing.B) {
				run(b, `	query conflictingDifferingResponses {
								extra {
									... on CatExtra { value: bool }
									... on DogExtra { value: bool }
								}	
							}`,
					FieldSelectionMerging(), Invalid)
			})
		})
		b.Run("5.3.3 Leaf Field Selections", func(b *testing.B) {
			b.Run("113", func(b *testing.B) {
				run(b, `	fragment scalarSelection on Dog {
								barkVolume
							}`,
					FieldSelections(), Valid)
			})
			b.Run("114", func(b *testing.B) {
				run(b, `	fragment scalarSelectionsNotAllowedOnInt on Dog {
								barkVolume {
									sinceWhen
								}
							}`,
					FieldSelections(), Invalid)
			})
			b.Run("116", func(b *testing.B) {
				run(b, `	query directQueryOnObjectWithoutSubFields {
								human
							}`,
					FieldSelections(), Invalid)
				run(b, `	query directQueryOnInterfaceWithoutSubFields {
								pet
							}`,
					FieldSelections(), Invalid)
				run(b, `	query directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid)
				run(b, `	mutation directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid)
				run(b, `	subscription directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid)
			})
		})
	})
	b.Run("5.4 Arguments", func(b *testing.B) {
		b.Run("5.4.1 Argument Names", func(b *testing.B) {
			b.Run("117", func(b *testing.B) {
				run(b, `	fragment argOnRequiredArg on Dog {
								doesKnowCommand(dogCommand: SIT)
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					ValidArguments(), Valid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg {
								dog {
									doesKnowCommand(dogCommand: SIT)
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					ValidArguments(), Valid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($dogCommand: DogCommand!) {
								dog {
									doesKnowCommand(dogCommand: $dogCommand)
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					ValidArguments(), Valid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($dogCommand: DogCommand = SIT) {
								dog {
									doesKnowCommand(dogCommand: $dogCommand)
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					ValidArguments(), Valid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `query argOnRequiredArg($catCommand: CatCommand) {
								dog {
									doesKnowCommand(dogCommand: $catCommand)
								}
							}`,
					ValidArguments(), Invalid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `query argOnRequiredArg($dogCommand: CatCommand) {
									dog {
										... on Dog {
											doesKnowCommand(dogCommand: $dogCommand)
										}
									}
								}`,
					ValidArguments(), Invalid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($booleanArg: Boolean) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $booleanArg) @include(if: true)
							}`,
					ValidArguments(), Valid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($booleanArg: Boolean!) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
							}`,
					ValidArguments(), Valid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($booleanArg: Boolean) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
							}`,
					ValidArguments(), Invalid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($booleanArg: Boolean!) {
										dog {
											...argOnOptional
										}
									}
									fragment argOnOptional on Dog {
										... {
											isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
										}
									}`,
					ValidArguments(), Valid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($booleanArg: Boolean) {
										dog {
											...argOnOptional
										}
									}
									fragment argOnOptional on Dog {
										...on Dog {
											isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
										}
									}`,
					ValidArguments(), Invalid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($intArg: Integer) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $intArg) @include(if: true)
							}`,
					ValidArguments(), Invalid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($intArg: Integer) {
								pet {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $intArg) @include(if: true)
							}`,
					ValidArguments(), Invalid)
			})
			b.Run("117 variant", func(b *testing.B) {
				run(b, `	query argOnRequiredArg($intArg: Integer) {
								pet {
									...on Dog {
										...argOnOptional
									}
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $intArg) @include(if: true)
							}`,
					ValidArguments(), Invalid)
			})
			b.Run("118", func(b *testing.B) {
				run(b, `	{
								dog { ...invalidArgName}
							}
							fragment invalidArgName on Dog {
								doesKnowCommand(command: CLEAN_UP_HOUSE)
							}`,
					ValidArguments(), Invalid)
			})
			b.Run("119", func(b *testing.B) {
				run(b, ` 	{
										dog { ...invalidArgName }
									}
									fragment invalidArgName on Dog {
										isHousetrained(atOtherHomes: true) @include(unless: false)
									}`,
					ValidArguments(), Invalid)
			})
			b.Run("121", func(b *testing.B) {
				run(b, `	fragment multipleArgs on ValidArguments {
								multipleReqs(x: 1, y: 2)
							}
							fragment multipleArgsReverseOrder on ValidArguments {
								multipleReqs(y: 2, x: 1)
							}`,
					ValidArguments(), Valid)
			})
			b.Run("undefined arg", func(b *testing.B) {
				run(b, `	{
								dog(name: "Goofy"){ 
									name
								}
							}`,
					ValidArguments(), Invalid)
			})
		})
		b.Run("5.4.2 Argument Uniqueness", func(b *testing.B) {
			b.Run("121 variant", func(b *testing.B) {
				run(b, `{
									arguments { ... multipleArgs }
								}
								fragment multipleArgs on ValidArguments {
									multipleReqs(x: 1, x: 2)
								}`,
					ArgumentUniqueness(), Invalid)
			})
			b.Run("121 variant", func(b *testing.B) {
				run(b, `{
									arguments { ... multipleArgs }
								}
								fragment multipleArgs on ValidArguments {
									multipleReqs(x: 1)
								}`,
					ArgumentUniqueness(), Valid)
			})
			b.Run("5.4.2.1 Required ValidArguments", func(b *testing.B) {
				b.Run("122", func(b *testing.B) {
					run(b, `	{
									arguments {
										...goodBooleanArg
										...goodNonNullArg
									}
								}
								fragment goodBooleanArg on ValidArguments {
									booleanArgField(booleanArg: true)
								}
								fragment goodNonNullArg on ValidArguments {
									nonNullBooleanArgField(nonNullBooleanArg: true)
								}`,
						ValidArguments(), Valid)
				})
				b.Run("123", func(b *testing.B) {
					run(b, `	{
									arguments {
										...goodBooleanArgDefault
									}
								}
								fragment goodBooleanArgDefault on ValidArguments {
									booleanArgField
								}`,
						ValidArguments(), Valid)
				})
				b.Run("124", func(b *testing.B) {
					run(b, `	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									nonNullBooleanArgField
								}`,
						RequiredArguments(), Invalid)
				})
				b.Run("124 variant", func(b *testing.B) {
					run(b, `	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									foo
								}`,
						RequiredArguments(), Invalid)
				})
				b.Run("125", func(b *testing.B) {
					run(b, `	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									nonNullBooleanArgField(nonNullBooleanArg: null)
								}`,
						ValidArguments(), Invalid)
				})
				b.Run("125 variant", func(b *testing.B) {
					run(b, `	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									nonNullBooleanArgField(nonNullBooleanArg: true)
								}`,
						RequiredArguments(), Valid)
				})
				b.Run("125 variant", func(b *testing.B) {
					run(b, `	{
									booleanList (booleanListArg: [true])
								}`,
						RequiredArguments(), Valid)
				})
			})
		})
	})
	b.Run("5.5 Fragments", func(b *testing.B) {
		b.Run("5.5.1 Fragment Declarations", func(b *testing.B) {
			b.Run("5.5.1.1 Fragment Name Uniqueness", func(b *testing.B) {
				b.Run("126", func(b *testing.B) {
					run(b, `	{
  									dog {
    									...fragmentOne
    									...fragmentTwo
  									}
								}
								fragment fragmentOne on Dog {
  									name
								}
								fragment fragmentTwo on Dog {
  									owner {
    									name
  									}
								}`,
						Fragments(), Valid)
				})
				b.Run("127", func(b *testing.B) {
					run(b, `	{
  									dog {
    									...fragmentOne
  									}
								}
								fragment fragmentOne on Dog {
  									name
								}
								fragment fragmentOne on Dog {
  									owner {
    									name
  									}
								}`,
						Fragments(), Invalid)
				})
			})
			b.Run("5.5.1.2 Fragment Spread Existence", func(b *testing.B) {
				b.Run("128", func(b *testing.B) {
					run(b, `
							{
								dog {
									...inlineFragment
									...inlineFragment2
								}
							}
							fragment correctType on Dog {
								name
							}
							fragment inlineFragment on Dog {
  								... on Dog {
    								name
  								}
								...correctType
							}
							fragment inlineFragment2 on Dog {
  								... @include(if: true) {
    								name
  								}
							}`, Fragments(), Valid)
				})
				b.Run("129", func(b *testing.B) {
					run(b, `	fragment notOnExistingType on NotInSchema {
  									name
								}`, Fragments(), Invalid)
				})
				b.Run("129", func(b *testing.B) {
					run(b, `	fragment inlineNotExistingType on Dog {
  									... on NotInSchema {
    									name
  									}
								}`, Fragments(), Invalid)
				})
			})
			b.Run("5.5.1.3 Fragments on Composite Types", func(b *testing.B) {
				b.Run("130", func(b *testing.B) {
					run(b, ` {
									dog {
										...fragOnObject
										...fragOnInterface
										...fragOnUnion
									}
								}
								fragment fragOnObject on Dog {
									name
								}
								fragment fragOnInterface on Pet {
									name
								}
								fragment fragOnUnion on CatOrDog {
									... on Dog {
										name
									}
								}`,
						Fragments(), Valid)
				})
				b.Run("131", func(b *testing.B) {
					run(b, ` fragment fragOnScalar on Int {
									something
								}`,
						Fragments(), Invalid)
				})
				b.Run("131", func(b *testing.B) {
					run(b, ` fragment inlineFragOnScalar on Dog {
									... on Boolean {
										somethingElse
									}
								}`,
						Fragments(), Invalid)
				})
			})
			b.Run("5.5.1.4 Fragments must be used", func(b *testing.B) {
				b.Run("132", func(b *testing.B) {
					run(b, `	fragment nameFragment on Dog {
									name
									...nameFragment2
								}
								fragment nameFragment2 on Dog {
									name
								}
								fragment nameFragment3 on Dog {
									name
								}
								{
									dog {
										...nameFragment
										...nameFragment2
										...nameFragment2
									}
								}`,
						Fragments(), Invalid)
				})
				b.Run("132 variant", func(b *testing.B) {
					run(b, `	fragment dogNames on Query {
									dog { name }
								}
								{
									...dogNames
								}`,
						Fragments(), Valid)
				})
				b.Run("132 variant", func(b *testing.B) {
					run(b, `	fragment catNames on Query {
									dog { name }
								}
								{
									...dogNames
								}`,
						Fragments(), Invalid)
				})
				b.Run("132 variant", func(b *testing.B) {
					run(b, `	fragment dogNames on Query {
									dog { name }
								}
								{
									... { ...dogNames }
								}`,
						Fragments(), Valid)
				})
			})
		})
		b.Run("5.5.2 Fragment Spreads", func(b *testing.B) {
			b.Run("5.5.2.1 Fragment spread target defined", func(b *testing.B) {
				b.Run("133", func(b *testing.B) {
					run(b, `	{
									dog {
										...undefinedFragment
									}
								}`,
						Fragments(), Invalid)
				})
			})
			b.Run("5.5.2.2 Fragment spreads must not form cycles", func(b *testing.B) {
				b.Run("134", func(b *testing.B) {
					run(b, `
					{
						dog {
							...nameFragment
						}
					}

					fragment nameFragment on Dog {
						name
						...barkVolumeFragment
					}

					fragment barkVolumeFragment on Dog {
						barkVolume
						...nameFragment
					}`,
						Fragments(), Invalid)
				})
				b.Run("136", func(b *testing.B) {
					run(b, `	{
									dog {
										...dogFragment
									}
								}
								fragment dogFragment on Dog {
									name
									owner {
										...ownerFragment
									}
								}
								fragment ownerFragment on Dog {
									name
									pets {
										...dogFragment
									}
								}`,
						Fragments(), Invalid)
				})
				b.Run("136 variant", func(b *testing.B) {
					run(b, `	{
									dog {
										...dogFragment
									}
								}
								fragment dogFragment on Dog {
									name
									owner {
										...ownerFragment
									}
								}
								fragment ownerFragment on Dog {
									name
									pets {
										... { ...dogFragment }
									}
								}`,
						Fragments(), Invalid)
				})
			})
			b.Run("5.5.2.3 Fragment spread is possible", func(b *testing.B) {
				b.Run("5.5.2.3.1 Object Spreads In Object Scope", func(b *testing.B) {
					b.Run("137", func(b *testing.B) {
						run(b, ` {
										dog {
											...dogFragment
										}
									}
									fragment dogFragment on Dog {
										... on Dog {
											barkVolume
										}
									}`,
							Fragments(), Valid)
					})
					b.Run("137 variant", func(b *testing.B) {
						run(b, ` {
										dog {
											...dogFragment
										}
									}
									fragment dogFragment on Dog {
										... on NoDog {
											barkVolume
										}
									}`,
							Fragments(), Invalid)
					})
					b.Run("138", func(b *testing.B) {
						run(b, ` {
										dog {
											...catInDogFragmentInvalid
										}
									}
									fragment catInDogFragmentInvalid on Dog {
										... on Cat {
											meowVolume
										}
									}`,
							Fragments(), Invalid)
					})
				})
				b.Run("5.5.2.3.2 Abstract Spreads in Object Scope", func(b *testing.B) {
					b.Run("139", func(b *testing.B) {
						run(b, ` 	{
										dog {
											...interfaceWithinObjectFragment
										}
									}
									fragment petNameFragment on Pet {
										name
									}
									fragment interfaceWithinObjectFragment on Dog {
										...petNameFragment
									}`,
							Fragments(), Valid)
					})
					b.Run("140", func(b *testing.B) {
						run(b, ` 	{
										dog {
											...unionWithObjectFragment
										}
									}
									fragment catOrDogNameFragment on CatOrDog {
										... on Cat {
											meowVolume
										}
									}
									fragment unionWithObjectFragment on Dog {
  										...catOrDogNameFragment
									}`,
							Fragments(), Valid)
					})
				})
				b.Run("5.5.2.3.3 Object Spreads In Abstract Scope", func(b *testing.B) {
					b.Run("141", func(b *testing.B) {
						run(b, ` {
										dog {
											...petFragment
											...catOrDogFragment
										}
									}
									fragment petFragment on Pet {
										name
										... on Dog {
											barkVolume
										}
									}
									fragment catOrDogFragment on CatOrDog {
										... on Cat {
											meowVolume
										}
									}`,
							Fragments(), Valid)
					})
					b.Run("142", func(b *testing.B) {
						run(b, ` fragment sentientFragment on Sentient {
										... on Dog {
											barkVolume
										}
									}`,
							Fragments(), Invalid)
					})
					b.Run("142", func(b *testing.B) {
						run(b, ` fragment humanOrAlienFragment on HumanOrAlien {
										... on Cat {
											meowVolume
										}
									}`,
							Fragments(), Invalid)
					})
				})
				b.Run("5.5.2.3.4 Abstract Spreads in Abstract Scope", func(b *testing.B) {
					b.Run("143", func(b *testing.B) {
						run(b, `	{
										dog {
											...unionWithInterface
										}
									}
									fragment unionWithInterface on Pet {
										...dogOrHumanFragment
									}
									fragment dogOrHumanFragment on DogOrHuman {
										... on Dog {
											barkVolume
										}
									}`,
							Fragments(), Valid)
					})
					b.Run("144", func(b *testing.B) {
						run(b, `	{
										dog {
											...nonIntersectingInterfaces
										}
									}
									fragment nonIntersectingInterfaces on Pet {
										...sentientFragment
									}
									fragment sentientFragment on Sentient {
										name
									}`,
							Fragments(), Invalid)
					})
				})
			})
		})
	})
	b.Run("5.6 Values", func(b *testing.B) {
		b.Run("5.6.1 Values of Correct Type", func(b *testing.B) {
			b.Run("145", func(b *testing.B) {
				run(b, `
							query goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								findDog(complex: $search)
							}`,
					Values(), Valid)
				/*run(b,`
							mutation goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								findDog(complex: $search)
							}`,
					Values(), Invalid)
				run(b,`
							subscription goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								findDog(complex: $search)
							}`,
					Values(), Invalid)
				run(b,`
							query goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								...queryFragment
							}
							fragment queryFragment on Query { findDog(complex: $search) }`,
					Values(), Valid)
				run(b,`
							query goodComplexDefaultValue {
								arguments {
									booleanArgField(booleanArg: true)
								}
							}`,
					Values(), Valid)
				run(b,`
							query goodComplexDefaultValue() {
								arguments {
									floatArgField(floatArg: 123)
								}
							}`,
					Values(), Valid)
				run(b,`
							query goodComplexDefaultValue() {
								arguments {
									floatArgField(floatArg: 1.23)
								}
							}`,
					Values(), Valid)*/
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue($search: ComplexInput = { name: 123 }) {
									findDog(complex: $search)
								}`,
					Values(), Invalid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue($search: ComplexInput = { name: "123" }) {
									findDog(complex: $search)
								}`,
					Values(), Valid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `	query goodComplexDefaultValue {
										findDog(complex: { name: 123 })
									}`,
					Values(), Invalid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `	query goodComplexDefaultValue {
										findDog(complex: { name: "123" })
									}`,
					Values(), Valid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `	{
								dog {
									doesKnowCommand(dogCommand: SIT)
								}
							}`,
					Values(), Valid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `	{
								dog {
									doesKnowCommand(dogCommand: MEOW)
								}
							}`,
					Values(), Invalid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `	{
								dog {
									doesKnowCommand(dogCommand: [true])
								}
							}`,
					Values(), Invalid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `	{
								dog {
									doesKnowCommand(dogCommand: {foo: "bar"})
								}
							}`,
					Values(), Invalid)
			})
			b.Run("146", func(b *testing.B) {
				run(b, `
							{
								arguments { ...stringIntoInt }
							}
							fragment stringIntoInt on ValidArguments {
								intArgField(intArg: "123")
							}`,
					Values(), Invalid)
				run(b, `
							{
								arguments { ...badComplexValue }
							}
							query badComplexValue {
								findDog(complex: { name: 123 })
							}`,
					Values(), Invalid)
			})
			b.Run("146 variant", func(b *testing.B) {
				run(b, `
							{
								arguments { ...badComplexValue }
							}
							query badComplexValue {
								findDog(complex: { name: "123" })
							}`,
					Values(), Valid)
			})
		})
		b.Run("5.6.2 Input Object Field Names", func(b *testing.B) {
			b.Run("147", func(b *testing.B) {
				run(b, `{
  									findDog(complex: { name: "Fido" })
								}`,
					Values(), Valid)
			})
			b.Run("148", func(b *testing.B) {
				run(b, `{
 									findDog(complex: { favoriteCookieFlavor: "Bacon" })
								}`,
					Values(), Invalid)
			})
		})
		b.Run("5.6.3 Input Object Field Uniqueness", func(b *testing.B) {
			b.Run("149", func(b *testing.B) {
				run(b, `{
									findDog(complex: { name: "Fido", name: "Goofy"})
								}`,
					Values(), Invalid)
			})
		})
		b.Run("5.6.4 Input Object Required Fields", func(b *testing.B) {
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue($search: ComplexNonOptionalInput = { name: "123" }) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Valid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue($search: ComplexNonOptionalInput = { name: null }) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Invalid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue($search: ComplexNonOptionalInput = {}) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Invalid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue {
									findDogNonOptional(complex: {})
								}`,
					Values(), Invalid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue {
									findDogNonOptional(complex: { name: "Goofy" })
								}`,
					Values(), Valid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue {
									...viaFragment
								}
								fragment viaFragment on Query {
									findDogNonOptional(complex: { name: "Goofy" })
								}`,
					Values(), Valid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query goodComplexDefaultValue {
									...viaFragment
								}
								fragment viaFragment on Query {
									findDogNonOptional(complex: { name: 123 })
								}`,
					Values(), Invalid)
			})
		})
	})
	b.Run("5.7 Directives", func(b *testing.B) {
		b.Run("5.7.1 Directives Are Defined", func(b *testing.B) {
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query definedDirective {
									arguments {
										booleanArgField(booleanArg: true) @skip(if: true)
									}
								}`,
					DirectivesAreDefined(), Valid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query undefinedDirective {
									arguments {
										booleanArgField(booleanArg: true) @noSkip(if: true)
									}
								}`,
					DirectivesAreDefined(), Invalid)
			})
			b.Run("145 variant", func(b *testing.B) {
				run(b, `query undefinedDirective {
									arguments {
										...viaFragment
									}
								}
								fragment viaFragment on ValidArguments {
									booleanArgField(booleanArg: true) @noSkip(if: true)
								}`,
					DirectivesAreDefined(), Invalid)
			})
		})
		b.Run("5.7.2 Directives Are In Valid Locations", func(b *testing.B) {
			b.Run("150 variant", func(b *testing.B) {
				run(b, `query @skip(if: true) {
									dog
								}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `query @noSkip(if: true) {
									dog
								}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `query {
									dog @skip(if: true)
								}`,
					DirectivesAreInValidLocations(), Valid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	{
								... @inline {
									dog
								}
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	{
								... {
									dog @inline
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	{
								...frag @spread
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	{
								... {
									dog @spread
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	{
								... {
									dog @fragmentDefinition
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	{
								...frag
							}
							fragment frag on Query @fragmentDefinition {}`,
					DirectivesAreInValidLocations(), Valid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	query @onQuery {
								dog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	query @onMutation {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	query @onSubscription {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	mutation @onQuery {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	mutation @onSubscription {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	mutation @onMutation {
								dog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	subscription @onQuery {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	subscription @onMutation {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			b.Run("150 variant", func(b *testing.B) {
				run(b, `	subscription @onSubscription {
								dog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
		})
		b.Run("5.7.3 Directives Are Unique Per Location", func(b *testing.B) {
			b.Run("151", func(b *testing.B) {
				run(b, `query ($foo: Boolean = true, $bar: Boolean = false) {
									field @skip(if: $foo) @skip(if: $bar)
								}`,
					DirectivesAreUniquePerLocation(), Invalid)
			})
			b.Run("152", func(b *testing.B) {
				run(b, `query ($foo: Boolean = true, $bar: Boolean = false) {
									field @skip(if: $foo) {
										subfieldA
									}
									field @skip(if: $bar) {
										subfieldB
									}
								}`,
					DirectivesAreUniquePerLocation(), Valid)
			})
		})
	})
	b.Run("5.8 Variables", func(b *testing.B) {
		b.Run("5.8.1 Variable Uniqueness", func(b *testing.B) {
			b.Run("153", func(b *testing.B) {
				run(b, `query houseTrainedQuery($atOtherHomes: Boolean, $atOtherHomes: Boolean) {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					VariableUniqueness(), Invalid)
			})
			b.Run("154", func(b *testing.B) {
				run(b, `query A($atOtherHomes: Boolean) {
								...HouseTrainedFragment
							}
							query B($atOtherHomes: Boolean) {
								...HouseTrainedFragment
							}
							fragment HouseTrainedFragment on Query {
								dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}
							}`,
					VariableUniqueness(), Valid)
			})
		})
		b.Run("5.8.2 Variables Are Input Types", func(b *testing.B) {
			b.Run("156", func(b *testing.B) {
				run(b, `query takesBoolean($atOtherHomes: Boolean) {
								dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}
							}
							query takesComplexInput($complexInput: ComplexInput) {
								findDog(complex: $complexInput) {
									name
								}
							}
							query TakesListOfBooleanBang($booleans: [Boolean!]) {
								booleanList(booleanListArg: $booleans)
							}`,
					VariablesAreInputTypes(), Valid)
			})
			b.Run("156", func(b *testing.B) {
				run(b, `query TakesListOfBooleanBang($booleans: [Boolean!]) {
									booleanList(booleanListArg: $booleans)
								}`,
					VariablesAreInputTypes(), Valid)
			})
			b.Run("157", func(b *testing.B) {
				run(b, `query takesCat($cat: Cat) {}`,
					VariablesAreInputTypes(), Invalid)
				run(b, `query takesDogBang($dog: Dog!) {}`,
					VariablesAreInputTypes(), Invalid)
				run(b, `query takesListOfPet($pets: [Pet]) {}`,
					VariablesAreInputTypes(), Invalid)
				run(b, `query takesCatOrDog($catOrDog: CatOrDog) {}`,
					VariablesAreInputTypes(), Invalid)
				run(b, `query takesCatOrDog($catCommand: CatCommand) {}`,
					VariablesAreInputTypes(), Valid)
			})
		})
		b.Run("5.8.3 All Variable Uses Defined", func(b *testing.B) {
			b.Run("158", func(b *testing.B) {
				run(b, `query variableIsDefined($atOtherHomes: Boolean) {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					AllVariableUsesDefined(), Valid)
			})
			b.Run("159", func(b *testing.B) {
				run(b, `query variableIsNotDefined {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					AllVariableUsesDefined(), Invalid)
			})
			b.Run("160", func(b *testing.B) {
				run(b, `query variableIsDefinedUsedInSingleFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedFragment
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariableUsesDefined(), Valid)
			})
			b.Run("161", func(b *testing.B) {
				run(b, `query variableIsNotDefinedUsedInSingleFragment {
									dog {
										...isHousetrainedFragment
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariableUsesDefined(), Invalid)
			})
			b.Run("162", func(b *testing.B) {
				run(b, `query variableIsNotDefinedUsedInNestedFragment {
									dog {
										...outerHousetrainedFragment
									}
								}
								fragment outerHousetrainedFragment on Dog {
									...isHousetrainedFragment
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariableUsesDefined(), Invalid)
				b.Run("163", func(b *testing.B) {
					run(b, `query housetrainedQueryOne($atOtherHomes: Boolean) {
										dog {
											...isHousetrainedFragment
										}
									}
									query housetrainedQueryTwo($atOtherHomes: Boolean) {
										dog {
											...isHousetrainedFragment
										}
									}
									fragment isHousetrainedFragment on Dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}`,
						AllVariableUsesDefined(), Valid)
				})
				b.Run("164", func(b *testing.B) {
					run(b, `query housetrainedQueryOne($atOtherHomes: Boolean) {
										dog {
											...isHousetrainedFragment
										}
									}
									query housetrainedQueryTwoNotDefined {
										dog {
											...isHousetrainedFragment
										}
									}
									fragment isHousetrainedFragment on Dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}`,
						AllVariableUsesDefined(), Invalid)
				})
			})
		})
		b.Run("5.8.4 All Variables Used", func(b *testing.B) {
			b.Run("165", func(b *testing.B) {
				run(b, `query variableUnused($atOtherHomes: Boolean) {
									dog {
										isHousetrained
									}
								}`,
					AllVariablesUsed(), Invalid)
			})
			b.Run("165 variant", func(b *testing.B) {
				run(b, `query variableUnused($x: Int!) {
									arguments {
										multipleReqs(x: $x, y: 1)
									}
								}`,
					AllVariablesUsed(), Valid)
			})
			b.Run("166", func(b *testing.B) {
				run(b, `query variableUsedInFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedFragment
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariablesUsed(), Valid)
			})
			b.Run("167", func(b *testing.B) {
				run(b, `query variableNotUsedWithinFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedWithoutVariableFragment
									}
								}
								fragment isHousetrainedWithoutVariableFragment on Dog {
									isHousetrained
								}`,
					AllVariablesUsed(), Invalid)
			})
			b.Run("168", func(b *testing.B) {
				run(b, `query queryWithUsedVar($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedFragment
									}
								}
								query queryWithExtraVar($atOtherHomes: Boolean, $extra: Int) {
									dog {
										...isHousetrainedFragment
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariablesUsed(), Invalid)
			})
		})
		b.Run("5.8.5 All Variable Usages are Allowed", func(b *testing.B) {
			b.Run("169", func(b *testing.B) {
				run(b, `query intCannotGoIntoBoolean($intArg: Int) {
									arguments {
										booleanArgField(booleanArg: $intArg)
									}
								}`,
					Values(), Invalid)
			})
			b.Run("170", func(b *testing.B) {
				run(b, `query booleanListCannotGoIntoBoolean($booleanListArg: [Boolean]) {
									arguments {
										booleanArgField(booleanArg: $booleanListArg)
									}
								}`,
					Values(), Invalid)
			})
			b.Run("171", func(b *testing.B) {
				run(b, `query booleanArgQuery($booleanArg: Boolean) {
									arguments {
										nonNullBooleanArgField(nonNullBooleanArg: $booleanArg)
									}
								}`,
					Values(), Invalid)
			})
			b.Run("172", func(b *testing.B) {
				run(b, `query nonNullListToList($nonNullBooleanList: [Boolean]!) {
								arguments {
									booleanListArgField(booleanListArg: $nonNullBooleanList)
								}
							}`,
					Values(), Valid)
			})
			b.Run("172 variant", func(b *testing.B) {
				run(b, `query nonNullListToList {
									arguments {
										booleanListArgField(booleanListArg: [true,false,true])
									}
								}`,
					Values(), Valid)
			})
			b.Run("172 variant", func(b *testing.B) {
				run(b, `query nonNullListToList {
									arguments {
										booleanListArgField(booleanListArg: [true,false,"123"])
									}
								}`,
					Values(), Invalid)
			})
			b.Run("172 variant", func(b *testing.B) {
				run(b, `query nonNullListToList {
									arguments {
										booleanListArgField(booleanListArg: [true,false,123])
									}
								}`,
					Values(), Invalid)
			})
			b.Run("172 variant", func(b *testing.B) {
				run(b, `query nonNullListToList($nonNullBooleanList: [Boolean]) {
									arguments {
										booleanListArgField(booleanListArg: $nonNullBooleanList)
									}
								}`,
					Values(), Invalid)
			})
			b.Run("173", func(b *testing.B) {
				run(b, `query listToNonNullList($booleanList: [Boolean]) {
									arguments {
										nonNullBooleanListField(nonNullBooleanListArg: $booleanList)
									}
								}`,
					Values(), Invalid)
			})
			b.Run("174", func(b *testing.B) {
				run(b, `query booleanArgQueryWithDefault($booleanArg: Boolean) {
									arguments {
										optionalNonNullBooleanArgField(optionalBooleanArg: $booleanArg)
									}
								}`,
					Values(), Valid)
			})
			b.Run("175", func(b *testing.B) {
				run(b, `query booleanArgQueryWithDefault($booleanArg: Boolean = true) {
									arguments {
										nonNullBooleanArgField(nonNullBooleanArg: $booleanArg)
									}
								}`,
					Values(), Valid)
			})
		})
	})
}
