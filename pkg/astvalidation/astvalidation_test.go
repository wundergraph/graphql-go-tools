package astvalidation

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"testing"
)

func TestExecutionValidation(t *testing.T) {

	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	mustDocument := func(doc *ast.Document, err error) *ast.Document {
		must(err)
		return doc
	}

	mustString := func(str string, err error) string {
		must(err)
		return str
	}

	run := func(operationInput string, rule Rule, expectation ValidationState) {

		definition := mustDocument(astparser.ParseGraphqlDocumentBytes(testDefinition))
		operation := mustDocument(astparser.ParseGraphqlDocumentString(operationInput))

		err := astnormalization.NormalizeOperation(operation, definition)
		if err != nil {
			if expectation != Invalid {
				panic(err)
			}
			return
		}

		validator := &OperationValidator{}
		validator.RegisterRule(rule)

		result := validator.Validate(operation, definition)

		printedOperation := mustString(astprinter.PrintString(operation, definition))

		if expectation != result.ValidationState {
			panic(fmt.Errorf("want expectation: %s, got: %s\noperation:\n%s\n", expectation, result.ValidationState, printedOperation))
		}
	}

	// 5.1 Documents
	// 5.1.1 Executable Definitions
	// -> won't be addressed as the parser will only parse operation- and fragment definitions
	// when parsing executable definitions

	t.Run("5.2 Operations", func(t *testing.T) {
		t.Run("5.2.1 Named Operation Definitions", func(t *testing.T) {
			t.Run("5.2.1.1 Operation Name Uniqueness", func(t *testing.T) {
				t.Run("92", func(t *testing.T) {
					run(`
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
				t.Run("93", func(t *testing.T) {
					run(`
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
				t.Run("94", func(t *testing.T) {
					run(`	
								query dogOperation {
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
		t.Run("5.2.2 Anonymous Operation Definitions", func(t *testing.T) {
			t.Run("5.2.2.1 Lone Anonymous Operation", func(t *testing.T) {
				t.Run("95", func(t *testing.T) {
					run(`	{
  							  		dog {
      									name
    								}
  								}`,
						LoneAnonymousOperation(), Valid)
				})
				t.Run("96", func(t *testing.T) {
					run(`	{
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
				t.Run("96 variant", func(t *testing.T) {
					run(`	query getDogName {
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
		t.Run("5.2.3 Subscription Operation Definitions", func(t *testing.T) {
			t.Run("5.2.3.1 Single root field", func(t *testing.T) {
				t.Run("97", func(t *testing.T) {
					run(`
							subscription sub {
								newMessage {
									body
									sender
								}
							}`,
						SubscriptionSingleRootField(), Valid)
				})
				t.Run("97 variant", func(t *testing.T) {
					run(`	
								query sub {
  									foo
									bar
								}`,
						SubscriptionSingleRootField(), Valid)
				})
				t.Run("97 variant", func(t *testing.T) {
					run(`	
								subscription sub {
  									... { foo }
  									... { bar }
								}`,
						SubscriptionSingleRootField(), Invalid)
				})
				t.Run("98", func(t *testing.T) {
					run(`	subscription sub {
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
				t.Run("99", func(t *testing.T) {
					run(`	
								subscription sub {
  									newMessage {
    									body
    									sender
  									}
  									disallowedSecondRootField
								}`,
						SubscriptionSingleRootField(), Invalid)
				})
				t.Run("100", func(t *testing.T) {
					run(`	
								subscription sub {
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
				t.Run("101", func(t *testing.T) {
					run(`
							subscription sub {
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
	t.Run("5.3 FieldSelections", func(t *testing.T) {
		t.Run("5.3.1 Field Selections on Objects, Interfaces, and Unions Types", func(t *testing.T) {
			t.Run("104", func(t *testing.T) {
				run(`
							{
								dog {
									...aliasedLyingFieldTargetNotDefined
								}
							}
							fragment aliasedLyingFieldTargetNotDefined on Dog {
								barkVolume: kawVolume
							}`,
					FieldSelections(), Invalid)
			})
			t.Run("104 variant", func(t *testing.T) {
				run(`	{
								dog {
									barkVolume: kawVolume
								}
							}`,
					FieldSelections(), Invalid)
			})
			t.Run("103", func(t *testing.T) {
				run(`	{
								dog {
									...interfaceFieldSelection
								}
							}
							fragment interfaceFieldSelection on Pet {
								name
							}`,
					FieldSelections(), Valid)
			})
			t.Run("104", func(t *testing.T) {
				run(`
							{
								dog {
									...definedOnImplementorsButNotInterface
								}
							}
							fragment definedOnImplementorsButNotInterface on Pet {
								nickname
							}`,
					FieldSelections(), Invalid)
			})
			t.Run("105", func(t *testing.T) {
				run(`	fragment inDirectFieldSelectionOnUnion on CatOrDog {
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
			t.Run("105 variant", func(t *testing.T) {
				run(`	fragment inDirectFieldSelectionOnUnion on CatOrDog {
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
			t.Run("105 variant", func(t *testing.T) {
				run(`	fragment inDirectFieldSelectionOnUnion on CatOrDog {
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
			t.Run("106", func(t *testing.T) {
				run(` fragment directFieldSelectionOnUnion on CatOrDog {
								name
								barkVolume
							}`,
					FieldSelections(), Invalid)
			})
			t.Run("106 variant", func(t *testing.T) {
				run(`
							fragment directFieldSelectionOnUnion on Cat {
								name {
									name
								}
							}`,
					FieldSelections(), Invalid)
			})
		})
		t.Run("5.3.2 Field Selection Merging", func(t *testing.T) {
			t.Run("107", func(t *testing.T) {
				run(`
							fragment mergeIdenticalFields on Dog {
  								name
  								name
  							}
  							fragment mergeIdenticalAliasesAndFields on Dog {
  								otherName: name
  								otherName: name
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("107 variant", func(t *testing.T) {
				run(`	
							query mergeIdenticalFields {
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
			t.Run("108", func(t *testing.T) {
				run(`
							fragment conflictingBecauseAlias on Dog {
  								name: nickname
  								name
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									name: nickname
  									name
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`
							query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`
							query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { noString: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string: noString }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra: extras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
								dog {
  									extras { string }
  									extras: mustExtras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									x: extras { string }
  									x: mustExtras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string }
  									extras { string,string3: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string }
  									extras { string,string2: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string2 }
  									extras { string,string2: string,string3: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									extras { ... { string },string2: string }
  									extras { ... { string },... { string },string2: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
								dog {
  									extras { ... { string },string: string1 }
  									extras { ... { string1: string },string2: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									extras { ...frag, ...frag }
  									extras { ...frag }
								}
  							}
							fragment frag on Extra { string }`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
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
			t.Run("108 variant", func(t *testing.T) {
				run(`	
							query conflictingBecauseAlias {
								dog {
  									extras { ...frag }
  									extras { ...frag2 }
								}
  							}
							fragment frag on DogExtra { string1 }
							fragment frag2 on DogExtra { string1: string }`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
								dog {
  									extra { looksLikeString: string }
  									extra { looksLikeString: bool }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
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
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
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
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
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
			t.Run("108 variant", func(t *testing.T) {
				run(`	query conflictingBecauseAlias {
								dog {
  									name: nickname
  									... {
										name
									}
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			/*t.Run("108 variant", func(t *testing.T) { TODO: uncomment and implement
							run(`	query conflictingBecauseAlias {
											dog {
			  									name: nickname
			  									... @include(if: true) {
													name
												}
											}
			  							}`,
								FieldSelectionMerging(), Invalid)
						})
						t.Run("108 variant", func(t *testing.T) {
							run(`	query conflictingBecauseAlias {
											dog {
			  									name: nickname
			  									... @include(if: false) {
													name
												}
											}
			  							}`,
								FieldSelectionMerging(), Valid)
						})
						t.Run("108 variant", func(t *testing.T) {
							run(`	query conflictingBecauseAlias($include: false) {
											dog {
			  									name: nickname
			  									... @include(if: $include) {
													name
												}
											}
			  							}`,
								FieldSelectionMerging(), Valid)
						})*/
			t.Run("109", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalArgs on Dog {
  								doesKnowCommand(dogCommand: SIT)
  								doesKnowCommand(dogCommand: SIT)
  							}
  							fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: $dogCommand)
    							doesKnowCommand(dogCommand: $dogCommand)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1)
    							doesKnowCommand(dogCommand: 1)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	
							fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1)
    							doesKnowCommand(dogCommand: 0)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1.1)
    							doesKnowCommand(dogCommand: 1.1)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1.1)
    							doesKnowCommand(dogCommand: 0.1)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: "foo")
    							doesKnowCommand(dogCommand: "foo")
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: "foo")
    							doesKnowCommand(dogCommand: "bar")
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: null)
    							doesKnowCommand(dogCommand: null)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: null)
    							doesKnowCommand(dogCommand: 0)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [1.1])
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [0.1])
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [1.1,1.1])
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "bar"})
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	
							fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {bar: "bar"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "baz"})
    							doesKnowCommand(dogCommand: {bar: "baz"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(`	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "baz",bar: "bat"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("110", func(t *testing.T) {
				run(`	fragment conflictingArgsOnValues on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand: HEEL)
							}`,
					FieldSelectionMerging(), Invalid)
				run(`	fragment conflictingArgsOnValues on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand1: HEEL)
							}`,
					FieldSelectionMerging(), Invalid)
				run(`	fragment conflictingArgsValueAndVar on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand: $dogCommand)
							}`,
					FieldSelectionMerging(), Invalid)
				run(`	fragment conflictingArgsWithVars on Dog {
								doesKnowCommand(dogCommand: $varOne)
								doesKnowCommand(dogCommand: $varTwo)
							}`,
					FieldSelectionMerging(), Invalid)
				run(`	fragment differingArgs on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand
							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("111", func(t *testing.T) {
				run(`	
							fragment safeDifferingFields on Pet {
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
			t.Run("112", func(t *testing.T) {
				run(`
							fragment conflictingDifferingResponses on Pet {
								... on Dog {
									someValue: nickname
								}
								... on Cat {
									someValue: meowVolume
								}
							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("112 variant", func(t *testing.T) {
				run(`	
							fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	
							fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	query conflictingDifferingResponses {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	query conflictingDifferingResponses {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	query conflictingDifferingResponses {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	query conflictingDifferingResponses {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	
							query conflictingDifferingResponses {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`
							fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	
							fragment conflictingDifferingResponses on Pet {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	query conflictingDifferingResponses {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	query conflictingDifferingResponses {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	query conflictingDifferingResponses {
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
			t.Run("112 variant", func(t *testing.T) {
				run(`	query conflictingDifferingResponses {
								extra {
									... on CatExtra { value: bool }
									... on DogExtra { value: bool }
								}	
							}`,
					FieldSelectionMerging(), Invalid)
			})
		})
		t.Run("5.3.3 Leaf Field Selections", func(t *testing.T) {
			t.Run("113", func(t *testing.T) {
				run(`	fragment scalarSelection on Dog {
								barkVolume
							}`,
					FieldSelections(), Valid)
			})
			t.Run("114", func(t *testing.T) {
				run(`	fragment scalarSelectionsNotAllowedOnInt on Dog {
								barkVolume {
									sinceWhen
								}
							}`,
					FieldSelections(), Invalid)
			})
			t.Run("116", func(t *testing.T) {
				run(`	
							query directQueryOnObjectWithoutSubFields {
								human
							}`,
					FieldSelections(), Invalid)
				run(`	query directQueryOnInterfaceWithoutSubFields {
								pet
							}`,
					FieldSelections(), Invalid)
				run(`	query directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid)
				run(`	mutation directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid)
				run(`	subscription directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid)
			})
		})
	})
	t.Run("5.4 Arguments", func(t *testing.T) {
		t.Run("5.4.1 Argument Names", func(t *testing.T) {
			t.Run("117", func(t *testing.T) {
				run(`	
							fragment argOnRequiredArg on Dog {
								doesKnowCommand(dogCommand: SIT)
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					ValidArguments(), Valid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg {
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
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($dogCommand: DogCommand!) {
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
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($dogCommand: DogCommand = SIT) {
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
			t.Run("117 variant", func(t *testing.T) {
				run(`
							query argOnRequiredArg($catCommand: CatCommand) {
								dog {
									doesKnowCommand(dogCommand: $catCommand)
								}
							}`,
					ValidArguments(), Invalid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(`query argOnRequiredArg($dogCommand: CatCommand) {
									dog {
										... on Dog {
											doesKnowCommand(dogCommand: $dogCommand)
										}
									}
								}`,
					ValidArguments(), Invalid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($booleanArg: Boolean) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $booleanArg) @include(if: true)
							}`,
					ValidArguments(), Valid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($booleanArg: Boolean!) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
							}`,
					ValidArguments(), Valid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($booleanArg: Boolean) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
							}`,
					ValidArguments(), Invalid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($booleanArg: Boolean!) {
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
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($booleanArg: Boolean) {
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
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($intArg: Integer) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $intArg) @include(if: true)
							}`,
					ValidArguments(), Invalid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($intArg: Integer) {
								pet {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $intArg) @include(if: true)
							}`,
					ValidArguments(), Invalid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(`	query argOnRequiredArg($intArg: Integer) {
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
			t.Run("118", func(t *testing.T) {
				run(`	{
								dog { ...invalidArgName}
							}
							fragment invalidArgName on Dog {
								doesKnowCommand(command: CLEAN_UP_HOUSE)
							}`,
					ValidArguments(), Invalid)
			})
			t.Run("119", func(t *testing.T) {
				run(` 	{
										dog { ...invalidArgName }
									}
									fragment invalidArgName on Dog {
										isHousetrained(atOtherHomes: true) @include(unless: false)
									}`,
					ValidArguments(), Invalid)
			})
			t.Run("121", func(t *testing.T) {
				run(`	fragment multipleArgs on ValidArguments {
								multipleReqs(x: 1, y: 2)
							}
							fragment multipleArgsReverseOrder on ValidArguments {
								multipleReqs(y: 2, x: 1)
							}`,
					ValidArguments(), Valid)
			})
			t.Run("undefined arg", func(t *testing.T) {
				run(`	{
								dog(name: "Goofy"){ 
									name
								}
							}`,
					ValidArguments(), Invalid)
			})
		})
		t.Run("5.4.2 Argument Uniqueness", func(t *testing.T) {
			t.Run("121 variant", func(t *testing.T) {
				run(`{
									arguments { ... multipleArgs }
								}
								fragment multipleArgs on ValidArguments {
									multipleReqs(x: 1, x: 2)
								}`,
					ArgumentUniqueness(), Invalid)
			})
			t.Run("121 variant", func(t *testing.T) {
				run(`{
									arguments { ... multipleArgs }
								}
								fragment multipleArgs on ValidArguments {
									multipleReqs(x: 1)
								}`,
					ArgumentUniqueness(), Valid)
			})
			t.Run("5.4.2.1 Required ValidArguments", func(t *testing.T) {
				t.Run("122", func(t *testing.T) {
					run(`	{
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
				t.Run("123", func(t *testing.T) {
					run(`	{
									arguments {
										...goodBooleanArgDefault
									}
								}
								fragment goodBooleanArgDefault on ValidArguments {
									booleanArgField
								}`,
						ValidArguments(), Valid)
				})
				t.Run("124", func(t *testing.T) {
					run(`	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									nonNullBooleanArgField
								}`,
						RequiredArguments(), Invalid)
				})
				t.Run("124 variant", func(t *testing.T) {
					run(`	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									foo
								}`,
						RequiredArguments(), Invalid)
				})
				t.Run("125", func(t *testing.T) {
					run(`	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									nonNullBooleanArgField(nonNullBooleanArg: null)
								}`,
						ValidArguments(), Invalid)
				})
				t.Run("125 variant", func(t *testing.T) {
					run(`	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									nonNullBooleanArgField(nonNullBooleanArg: true)
								}`,
						RequiredArguments(), Valid)
				})
				t.Run("125 variant", func(t *testing.T) {
					run(`	{
									booleanList (booleanListArg: [true])
								}`,
						RequiredArguments(), Valid)
				})
			})
		})
	})
	t.Run("5.5 Fragments", func(t *testing.T) {
		t.Run("5.5.1 Fragment Declarations", func(t *testing.T) {
			t.Run("5.5.1.1 Fragment Name Uniqueness", func(t *testing.T) {
				t.Run("126", func(t *testing.T) {
					run(`	{
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
				t.Run("127", func(t *testing.T) {
					run(`	{
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
			t.Run("5.5.1.2 Fragment Spread Existence", func(t *testing.T) {
				t.Run("128", func(t *testing.T) {
					run(`
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
				t.Run("129", func(t *testing.T) {
					run(`	fragment notOnExistingType on NotInSchema {
  									name
								}`, Fragments(), Invalid)
				})
				t.Run("129", func(t *testing.T) {
					run(`	fragment inlineNotExistingType on Dog {
  									... on NotInSchema {
    									name
  									}
								}`, Fragments(), Invalid)
				})
			})
			t.Run("5.5.1.3 Fragments on Composite Types", func(t *testing.T) {
				t.Run("130", func(t *testing.T) {
					run(` {
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
				t.Run("131", func(t *testing.T) {
					run(` fragment fragOnScalar on Int {
									something
								}`,
						Fragments(), Invalid)
				})
				t.Run("131", func(t *testing.T) {
					run(` fragment inlineFragOnScalar on Dog {
									... on Boolean {
										somethingElse
									}
								}`,
						Fragments(), Invalid)
				})
			})
			t.Run("5.5.1.4 Fragments must be used", func(t *testing.T) {
				t.Run("132", func(t *testing.T) {
					run(`	fragment nameFragment on Dog {
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
				t.Run("132 variant", func(t *testing.T) {
					run(`	fragment dogNames on Query {
									dog { name }
								}
								{
									...dogNames
								}`,
						Fragments(), Valid)
				})
				t.Run("132 variant", func(t *testing.T) {
					run(`	fragment catNames on Query {
									dog { name }
								}
								{
									...dogNames
								}`,
						Fragments(), Invalid)
				})
				t.Run("132 variant", func(t *testing.T) {
					run(`	fragment dogNames on Query {
									dog { name }
								}
								{
									... { ...dogNames }
								}`,
						Fragments(), Valid)
				})
			})
		})
		t.Run("5.5.2 Fragment Spreads", func(t *testing.T) {
			t.Run("5.5.2.1 Fragment spread target defined", func(t *testing.T) {
				t.Run("133", func(t *testing.T) {
					run(`	{
									dog {
										...undefinedFragment
									}
								}`,
						Fragments(), Invalid)
				})
			})
			t.Run("5.5.2.2 Fragment spreads must not form cycles", func(t *testing.T) {
				t.Run("134", func(t *testing.T) {
					run(`
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
				t.Run("136", func(t *testing.T) {
					run(`	{
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
				t.Run("136 variant", func(t *testing.T) {
					run(`	{
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
			t.Run("5.5.2.3 Fragment spread is possible", func(t *testing.T) {
				t.Run("5.5.2.3.1 Object Spreads In Object Scope", func(t *testing.T) {
					t.Run("137", func(t *testing.T) {
						run(` {
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
					t.Run("137 variant", func(t *testing.T) {
						run(` {
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
					t.Run("138", func(t *testing.T) {
						run(` {
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
				t.Run("5.5.2.3.2 Abstract Spreads in Object Scope", func(t *testing.T) {
					t.Run("139", func(t *testing.T) {
						run(` 	{
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
					t.Run("140", func(t *testing.T) {
						run(` 	{
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
				t.Run("5.5.2.3.3 Object Spreads In Abstract Scope", func(t *testing.T) {
					t.Run("141", func(t *testing.T) {
						run(` {
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
					t.Run("142", func(t *testing.T) {
						run(` fragment sentientFragment on Sentient {
										... on Dog {
											barkVolume
										}
									}`,
							Fragments(), Invalid)
					})
					t.Run("142", func(t *testing.T) {
						run(` fragment humanOrAlienFragment on HumanOrAlien {
										... on Cat {
											meowVolume
										}
									}`,
							Fragments(), Invalid)
					})
				})
				t.Run("5.5.2.3.4 Abstract Spreads in Abstract Scope", func(t *testing.T) {
					t.Run("143", func(t *testing.T) {
						run(`	{
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
					t.Run("144", func(t *testing.T) {
						run(`	{
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
	t.Run("5.6 Values", func(t *testing.T) {
		t.Run("5.6.1 Values of Correct Type", func(t *testing.T) {
			t.Run("145", func(t *testing.T) {
				run(`
							query goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								findDog(complex: $search)
							}`,
					Values(), Valid)
				/*run(`
							mutation goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								findDog(complex: $search)
							}`,
					Values(), Invalid)
				run(`
							subscription goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								findDog(complex: $search)
							}`,
					Values(), Invalid)
				run(`
							query goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								...queryFragment
							}
							fragment queryFragment on Query { findDog(complex: $search) }`,
					Values(), Valid)
				run(`
							query goodComplexDefaultValue {
								arguments {
									booleanArgField(booleanArg: true)
								}
							}`,
					Values(), Valid)
				run(`
							query goodComplexDefaultValue() {
								arguments {
									floatArgField(floatArg: 123)
								}
							}`,
					Values(), Valid)
				run(`
							query goodComplexDefaultValue() {
								arguments {
									floatArgField(floatArg: 1.23)
								}
							}`,
					Values(), Valid)*/
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue($search: ComplexInput = { name: 123 }) {
									findDog(complex: $search)
								}`,
					Values(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue($search: ComplexInput = { name: "123" }) {
									findDog(complex: $search)
								}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`	query goodComplexDefaultValue {
										findDog(complex: { name: 123 })
									}`,
					Values(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`	query goodComplexDefaultValue {
										findDog(complex: { name: "123" })
									}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`	{
								dog {
									doesKnowCommand(dogCommand: SIT)
								}
							}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`	{
								dog {
									doesKnowCommand(dogCommand: MEOW)
								}
							}`,
					Values(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`	{
								dog {
									doesKnowCommand(dogCommand: [true])
								}
							}`,
					Values(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`	{
								dog {
									doesKnowCommand(dogCommand: {foo: "bar"})
								}
							}`,
					Values(), Invalid)
			})
			t.Run("146", func(t *testing.T) {
				run(`
							{
								arguments { ...stringIntoInt }
							}
							fragment stringIntoInt on ValidArguments {
								intArgField(intArg: "123")
							}`,
					Values(), Invalid)
				run(`
							{
								arguments { ...badComplexValue }
							}
							query badComplexValue {
								findDog(complex: { name: 123 })
							}`,
					Values(), Invalid)
			})
			t.Run("146 variant", func(t *testing.T) {
				run(`
							{
								arguments { ...badComplexValue }
							}
							query badComplexValue {
								findDog(complex: { name: "123" })
							}`,
					Values(), Valid)
			})
		})
		t.Run("5.6.2 Input Object Field Names", func(t *testing.T) {
			t.Run("147", func(t *testing.T) {
				run(`{
  									findDog(complex: { name: "Fido" })
								}`,
					Values(), Valid)
			})
			t.Run("148", func(t *testing.T) {
				run(`{
 									findDog(complex: { favoriteCookieFlavor: "Bacon" })
								}`,
					Values(), Invalid)
			})
		})
		t.Run("5.6.3 Input Object Field Uniqueness", func(t *testing.T) {
			t.Run("149", func(t *testing.T) {
				run(`{
									findDog(complex: { name: "Fido", name: "Goofy"})
								}`,
					Values(), Invalid)
			})
		})
		t.Run("5.6.4 Input Object Required Fields", func(t *testing.T) {
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue($search: ComplexNonOptionalInput = { name: "123" }) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue($search: ComplexNonOptionalInput = { name: null }) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue($search: ComplexNonOptionalInput = {}) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue {
									findDogNonOptional(complex: {})
								}`,
					Values(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue {
									findDogNonOptional(complex: { name: "Goofy" })
								}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue {
									...viaFragment
								}
								fragment viaFragment on Query {
									findDogNonOptional(complex: { name: "Goofy" })
								}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query goodComplexDefaultValue {
									...viaFragment
								}
								fragment viaFragment on Query {
									findDogNonOptional(complex: { name: 123 })
								}`,
					Values(), Invalid)
			})
		})
	})
	t.Run("5.7 Directives", func(t *testing.T) {
		t.Run("5.7.1 Directives Are Defined", func(t *testing.T) {
			t.Run("145 variant", func(t *testing.T) {
				run(`query definedDirective {
									arguments {
										booleanArgField(booleanArg: true) @skip(if: true)
									}
								}`,
					DirectivesAreDefined(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query undefinedDirective {
									arguments {
										booleanArgField(booleanArg: true) @noSkip(if: true)
									}
								}`,
					DirectivesAreDefined(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`query undefinedDirective {
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
		t.Run("5.7.2 Directives Are In Valid Locations", func(t *testing.T) {
			t.Run("150 variant", func(t *testing.T) {
				run(`query @skip(if: true) {
									dog
								}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`query @noSkip(if: true) {
									dog
								}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`query {
									dog @skip(if: true)
								}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	{
								... @inline {
									dog
								}
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	{
								... {
									dog @inline
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	{
								...frag @spread
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	{
								... {
									dog @spread
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	{
								... {
									dog @fragmentDefinition
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	{
								...frag
							}
							fragment frag on Query @fragmentDefinition {}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	query @onQuery {
								dog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	query @onMutation {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	query @onSubscription {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	mutation @onQuery {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	mutation @onSubscription {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	mutation @onMutation {
								dog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	subscription @onQuery {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	subscription @onMutation {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`	subscription @onSubscription {
								dog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
		})
		t.Run("5.7.3 Directives Are Unique Per Location", func(t *testing.T) {
			t.Run("151", func(t *testing.T) {
				run(`query ($foo: Boolean = true, $bar: Boolean = false) {
									field @skip(if: $foo) @skip(if: $bar)
								}`,
					DirectivesAreUniquePerLocation(), Invalid)
			})
			t.Run("152", func(t *testing.T) {
				run(`query ($foo: Boolean = true, $bar: Boolean = false) {
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
	t.Run("5.8 Variables", func(t *testing.T) {
		t.Run("5.8.1 Variable Uniqueness", func(t *testing.T) {
			t.Run("153", func(t *testing.T) {
				run(`query houseTrainedQuery($atOtherHomes: Boolean, $atOtherHomes: Boolean) {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					VariableUniqueness(), Invalid)
			})
			t.Run("154", func(t *testing.T) {
				run(`query A($atOtherHomes: Boolean) {
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
		t.Run("5.8.2 Variables Are Input Types", func(t *testing.T) {
			t.Run("156", func(t *testing.T) {
				run(`query takesBoolean($atOtherHomes: Boolean) {
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
			t.Run("156", func(t *testing.T) {
				run(`query TakesListOfBooleanBang($booleans: [Boolean!]) {
									booleanList(booleanListArg: $booleans)
								}`,
					VariablesAreInputTypes(), Valid)
			})
			t.Run("157", func(t *testing.T) {
				run(`query takesCat($cat: Cat) {}`,
					VariablesAreInputTypes(), Invalid)
				run(`query takesDogBang($dog: Dog!) {}`,
					VariablesAreInputTypes(), Invalid)
				run(`query takesListOfPet($pets: [Pet]) {}`,
					VariablesAreInputTypes(), Invalid)
				run(`query takesCatOrDog($catOrDog: CatOrDog) {}`,
					VariablesAreInputTypes(), Invalid)
				run(`query takesCatOrDog($catCommand: CatCommand) {}`,
					VariablesAreInputTypes(), Valid)
			})
		})
		t.Run("5.8.3 All Variable Uses Defined", func(t *testing.T) {
			t.Run("158", func(t *testing.T) {
				run(`query variableIsDefined($atOtherHomes: Boolean) {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					AllVariableUsesDefined(), Valid)
			})
			t.Run("159", func(t *testing.T) {
				run(`query variableIsNotDefined {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					AllVariableUsesDefined(), Invalid)
			})
			t.Run("160", func(t *testing.T) {
				run(`query variableIsDefinedUsedInSingleFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedFragment
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariableUsesDefined(), Valid)
			})
			t.Run("161", func(t *testing.T) {
				run(`query variableIsNotDefinedUsedInSingleFragment {
									dog {
										...isHousetrainedFragment
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariableUsesDefined(), Invalid)
			})
			t.Run("162", func(t *testing.T) {
				run(`query variableIsNotDefinedUsedInNestedFragment {
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
				t.Run("163", func(t *testing.T) {
					run(`query housetrainedQueryOne($atOtherHomes: Boolean) {
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
				t.Run("164", func(t *testing.T) {
					run(`query housetrainedQueryOne($atOtherHomes: Boolean) {
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
		t.Run("5.8.4 All Variables Used", func(t *testing.T) {
			t.Run("165", func(t *testing.T) {
				run(`query variableUnused($atOtherHomes: Boolean) {
									dog {
										isHousetrained
									}
								}`,
					AllVariablesUsed(), Invalid)
			})
			t.Run("165 variant", func(t *testing.T) {
				run(`query variableUnused($x: Int!) {
									arguments {
										multipleReqs(x: $x, y: 1)
									}
								}`,
					AllVariablesUsed(), Valid)
			})
			t.Run("166", func(t *testing.T) {
				run(`query variableUsedInFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedFragment
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariablesUsed(), Valid)
			})
			t.Run("167", func(t *testing.T) {
				run(`query variableNotUsedWithinFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedWithoutVariableFragment
									}
								}
								fragment isHousetrainedWithoutVariableFragment on Dog {
									isHousetrained
								}`,
					AllVariablesUsed(), Invalid)
			})
			t.Run("168", func(t *testing.T) {
				run(`query queryWithUsedVar($atOtherHomes: Boolean) {
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
		t.Run("5.8.5 All Variable Usages are Allowed", func(t *testing.T) {
			t.Run("169", func(t *testing.T) {
				run(`query intCannotGoIntoBoolean($intArg: Int) {
									arguments {
										booleanArgField(booleanArg: $intArg)
									}
								}`,
					Values(), Invalid)
			})
			t.Run("170", func(t *testing.T) {
				run(`query booleanListCannotGoIntoBoolean($booleanListArg: [Boolean]) {
									arguments {
										booleanArgField(booleanArg: $booleanListArg)
									}
								}`,
					Values(), Invalid)
			})
			t.Run("171", func(t *testing.T) {
				run(`query booleanArgQuery($booleanArg: Boolean) {
									arguments {
										nonNullBooleanArgField(nonNullBooleanArg: $booleanArg)
									}
								}`,
					Values(), Invalid)
			})
			t.Run("172", func(t *testing.T) {
				run(`query nonNullListToList($nonNullBooleanList: [Boolean]!) {
								arguments {
									booleanListArgField(booleanListArg: $nonNullBooleanList)
								}
							}`,
					Values(), Valid)
			})
			t.Run("172 variant", func(t *testing.T) {
				run(`query nonNullListToList {
									arguments {
										booleanListArgField(booleanListArg: [true,false,true])
									}
								}`,
					Values(), Valid)
			})
			t.Run("172 variant", func(t *testing.T) {
				run(`query nonNullListToList {
									arguments {
										booleanListArgField(booleanListArg: [true,false,"123"])
									}
								}`,
					Values(), Invalid)
			})
			t.Run("172 variant", func(t *testing.T) {
				run(`query nonNullListToList {
									arguments {
										booleanListArgField(booleanListArg: [true,false,123])
									}
								}`,
					Values(), Invalid)
			})
			t.Run("172 variant", func(t *testing.T) {
				run(`query nonNullListToList($nonNullBooleanList: [Boolean]) {
									arguments {
										booleanListArgField(booleanListArg: $nonNullBooleanList)
									}
								}`,
					Values(), Invalid)
			})
			t.Run("173", func(t *testing.T) {
				run(`query listToNonNullList($booleanList: [Boolean]) {
									arguments {
										nonNullBooleanListField(nonNullBooleanListArg: $booleanList)
									}
								}`,
					Values(), Invalid)
			})
			t.Run("174", func(t *testing.T) {
				run(`query booleanArgQueryWithDefault($booleanArg: Boolean) {
									arguments {
										optionalNonNullBooleanArgField(optionalBooleanArg: $booleanArg)
									}
								}`,
					Values(), Valid)
			})
			t.Run("175", func(t *testing.T) {
				run(`query booleanArgQueryWithDefault($booleanArg: Boolean = true) {
									arguments {
										nonNullBooleanArgField(nonNullBooleanArg: $booleanArg)
									}
								}`,
					Values(), Valid)
			})
		})
	})
}

var testDefinition = []byte(`
schema {
	query: Query
	mutation: Mutation
	subscription: Subscription
}

type Message {
	sender: String
	body: String
}

type Subscription {
	newMessage: Message
	foo: String
	bar: String
	disallowedSecondRootField: Boolean
}

type Mutation {
	mutateDog: Dog
}

input ComplexInput { name: String, owner: String }
input ComplexNonOptionalInput { name: String! }

type Query {
	foo: String
	bar: String
	human: Human
  	pet: Pet
  	dog: Dog
	cat: Cat
	catOrDog: CatOrDog
	dogOrHuman: DogOrHuman
	humanOrAlien: HumanOrAlien
	arguments: ValidArguments
	findDog(complex: ComplexInput): Dog
	findDogNonOptional(complex: ComplexNonOptionalInput): Dog
  	booleanList(booleanListArg: [Boolean!]): Boolean
	extra: Extra
}

type ValidArguments {
	multipleReqs(x: Int!, y: Int!): Int!
	booleanArgField(booleanArg: Boolean): Boolean
	floatArgField(floatArg: Float): Float
	intArgField(intArg: Int): Int
	nonNullBooleanArgField(nonNullBooleanArg: Boolean!): Boolean!
	booleanListArgField(booleanListArg: [Boolean]!): [Boolean]
	optionalNonNullBooleanArgField(optionalBooleanArg: Boolean! = false): Boolean!
}

enum DogCommand { SIT, DOWN, HEEL }

type Dog implements Pet {
	id: ID
	name: String!
	nickname: String
	barkVolume: Int
	doesKnowCommand(dogCommand: DogCommand!): Boolean!
	isHousetrained(atOtherHomes: Boolean): Boolean!
	owner: Human
	extra: DogExtra
	extras: [DogExtra]
	mustExtra: DogExtra!
	mustExtras: [DogExtra]!
	mustMustExtras: [DogExtra!]!
}

type DogExtra {
	string: String
	noString: Boolean
	strings: [String]
	mustStrings: [String]!
	bool: Int
}

interface Sentient {
  name: String!
}

interface Pet {
  name: String!
}

type Alien implements Sentient {
  name: String!
  homePlanet: String
}

type Human implements Sentient {
  name: String!
}

enum CatCommand { JUMP }

type Cat implements Pet {
	name: String!
	nickname: String
	doesKnowCommand(catCommand: CatCommand!): Boolean!
	meowVolume: Int
	extra: CatExtra
}

type CatExtra {
	string: String
	string2: String
	strings: [String]
	mustStrings: [String]!
	bool: Boolean
}

union CatOrDog = Cat | Dog
union DogOrHuman = Dog | Human
union HumanOrAlien = Human | Alien
union Extra = CatExtra | DogExtra

directive @inline on INLINE_FRAGMENT
directive @spread on FRAGMENT_SPREAD
directive @fragmentDefinition on FRAGMENT_DEFINITION
directive @onQuery on QUERY
directive @onMutation on MUTATION
directive @onSubscription on SUBSCRIPTION

"The Int scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1."
scalar Int
"The Float scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point)."
scalar Float
"The String scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text."
scalar String
"The Boolean scalar type represents true or false ."
scalar Boolean
"The ID scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as 4) or integer (such as 4) input value will be accepted as an ID."
scalar ID @custom(typeName: "string")
"Directs the executor to include this field or fragment only when the argument is true."
directive @include(
    " Included when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Directs the executor to skip this field or fragment when the argument is true."
directive @skip(
    "Skipped when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Marks an element of a GraphQL schema as no longer supported."
directive @deprecated(
    """
    Explains why this element was deprecated, usually also including a suggestion
    for how to access supported similar data. Formatted in
    [Markdown](https://daringfireball.net/projects/markdown/).
    """
    reason: String = "No longer supported"
) on FIELD_DEFINITION | ENUM_VALUE

"""
A Directive provides a way to describe alternate runtime execution and type validation behavior in a GraphQL document.
In some cases, you need to provide options to alter GraphQL's execution behavior
in ways field arguments will not suffice, such as conditionally including or
skipping a field. Directives provide this by describing additional information
to the executor.
"""
type __Directive {
    name: String!
    description: String
    locations: [__DirectiveLocation!]!
    args: [__InputValue!]!
}

"""
A Directive can be adjacent to many parts of the GraphQL language, a
__DirectiveLocation describes one such possible adjacencies.
"""
enum __DirectiveLocation {
    "Location adjacent to a query operation."
    QUERY
    "Location adjacent to a mutation operation."
    MUTATION
    "Location adjacent to a subscription operation."
    SUBSCRIPTION
    "Location adjacent to a field."
    FIELD
    "Location adjacent to a fragment definition."
    FRAGMENT_DEFINITION
    "Location adjacent to a fragment spread."
    FRAGMENT_SPREAD
    "Location adjacent to an inline fragment."
    INLINE_FRAGMENT
    "Location adjacent to a schema definition."
    SCHEMA
    "Location adjacent to a scalar definition."
    SCALAR
    "Location adjacent to an object type definition."
    OBJECT
    "Location adjacent to a field definition."
    FIELD_DEFINITION
    "Location adjacent to an argument definition."
    ARGUMENT_DEFINITION
    "Location adjacent to an interface definition."
    INTERFACE
    "Location adjacent to a union definition."
    UNION
    "Location adjacent to an enum definition."
    ENUM
    "Location adjacent to an enum value definition."
    ENUM_VALUE
    "Location adjacent to an input object type definition."
    INPUT_OBJECT
    "Location adjacent to an input object field definition."
    INPUT_FIELD_DEFINITION
}
"""
One possible value for a given Enum. Enum values are unique values, not a
placeholder for a string or numeric value. However an Enum value is returned in
a JSON response as a string.
"""
type __EnumValue {
    name: String!
    description: String
    isDeprecated: Boolean!
    deprecationReason: String
}

"""
Object and Interface types are described by a list of FieldSelections, each of which has
a name, potentially a list of arguments, and a return type.
"""
type __Field {
    name: String!
    description: String
    args: [__InputValue!]!
    type: __Type!
    isDeprecated: Boolean!
    deprecationReason: String
}

"""ValidArguments provided to FieldSelections or Directives and the input fields of an
InputObject are represented as Input Values which describe their type and
optionally a default value.
"""
type __InputValue {
    name: String!
    description: String
    type: __Type!
    "A GraphQL-formatted string representing the default value for this input value."
    defaultValue: String
}

"""
A GraphQL Schema defines the capabilities of a GraphQL server. It exposes all
available types and directives on the server, as well as the entry points for
query, mutation, and subscription operations.
"""
type __Schema {
    "A list of all types supported by this server."
    types: [__Type!]!
    "The type that query operations will be rooted at."
    queryType: __Type!
    "If this server supports mutation, the type that mutation operations will be rooted at."
    mutationType: __Type
    "If this server support subscription, the type that subscription operations will be rooted at."
    subscriptionType: __Type
    "A list of all directives supported by this server."
    directives: [__Directive!]!
}

"""
The fundamental unit of any GraphQL Schema is the type. There are many kinds of
types in GraphQL as represented by the __TypeKind enum.

Depending on the kind of a type, certain fields describe information about that
type. Scalar types provide no information beyond a name and description, while
Enum types provide their values. Object and Interface types provide the fields
they describe. Abstract types, Union and Interface, provide the Object types
possible at runtime. List and NonNull types compose other types.
"""
type __Type {
    kind: __TypeKind!
    name: String
    description: String
    fields(includeDeprecated: Boolean = false): [__Field!]
    interfaces: [__Type!]
    possibleTypes: [__Type!]
    enumValues(includeDeprecated: Boolean = false): [__EnumValue!]
    inputFields: [__InputValue!]
    ofType: __Type
}

"An enum describing what kind of type a given __Type is."
enum __TypeKind {
    "Indicates this type is a scalar."
    SCALAR
    "Indicates this type is an object. fields and interfaces are valid fields."
    OBJECT
    "Indicates this type is an interface. fields  and  possibleTypes are valid fields."
    INTERFACE
    "Indicates this type is a union. possibleTypes is a valid field."
    UNION
    "Indicates this type is an enum. enumValues is a valid field."
    ENUM
    "Indicates this type is an input object. inputFields is a valid field."
    INPUT_OBJECT
    "Indicates this type is a list. ofType is a valid field."
    LIST
    "Indicates this type is a non-null. ofType is a valid field."
    NON_NULL
}`)
