package astvalidation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type options struct {
	disableNormalization        bool
	expectNormalizationError    bool
	expectValidationErrors      bool
	expectedValidationErrorMsgs []string
}

type option func(options *options)

// nolint
func withDisableNormalization() option {
	return func(options *options) {
		options.disableNormalization = true
	}
}

func withExpectNormalizationError() option {
	return func(options *options) {
		options.expectNormalizationError = true
	}
}

func withValidationErrors(errMsgs ...string) option {
	return func(options *options) {
		options.expectValidationErrors = true
		options.expectedValidationErrorMsgs = errMsgs
	}
}

func TestExecutionValidation(t *testing.T) {
	must := func(err error) {
		if report, ok := err.(operationreport.Report); ok {
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	mustDocument := func(doc ast.Document, report operationreport.Report) ast.Document {
		if report.HasErrors() {
			must(report)
		}
		return doc
	}

	mustString := func(str string, err error) string {
		must(err)
		return str
	}

	runWithDefinition := func(t *testing.T, definitionInput, operationInput string, rule Rule, expectation ValidationState, opts ...option) {
		t.Helper()

		var options options
		for _, opt := range opts {
			opt(&options)
		}

		definition := mustDocument(astparser.ParseGraphqlDocumentString(definitionInput))
		operation := mustDocument(astparser.ParseGraphqlDocumentString(operationInput))
		report := operationreport.Report{}

		if !options.disableNormalization {
			normalizer := astnormalization.NewWithOpts(
				astnormalization.WithInlineFragmentSpreads(),
			)
			normalizer.NormalizeOperation(&operation, &definition, &report)

			if report.HasErrors() && !options.expectNormalizationError {
				panic(report.Error())
			}
		}

		validator := &OperationValidator{}
		validator.RegisterRule(rule)

		result := validator.Validate(&operation, &definition, &report)

		printedOperation := mustString(astprinter.PrintString(&operation, &definition))

		if options.expectValidationErrors {
			for _, msg := range options.expectedValidationErrorMsgs {
				assert.Contains(t, report.Error(), msg)
			}
		}

		require.Equal(t, expectation, result, "wrong validation result expected: %v got: %v\nreason: %v\noperation:\n%s\n", expectation, result, report.Error(), printedOperation)
	}

	runManyRulesWithDefinition := func(t *testing.T, definitionInput, operationInput string, expectation ValidationState, rules ...Rule) {
		t.Helper()
		for _, rule := range rules {
			runWithDefinition(t, definitionInput, operationInput, rule, expectation)
		}
	}
	_ = runManyRulesWithDefinition

	runManyRules := func(t *testing.T, operationInput string, expectation ValidationState, rules ...Rule) {
		t.Helper()
		for _, rule := range rules {
			runWithDefinition(t, testDefinition, operationInput, rule, expectation)
		}
	}
	_ = runManyRules

	run := func(t *testing.T, operationInput string, rule Rule, expectation ValidationState, opts ...option) {
		t.Helper()
		runWithDefinition(t, testDefinition, operationInput, rule, expectation, opts...)
	}

	// 5.1 Documents
	// 5.1.1 Executable Definitions
	// -> won't be addressed as the parser will only parse operation- and fragment definitions
	// when parsing executable definitions

	t.Run("5.2 Operations", func(t *testing.T) {
		t.Run("5.2.1 Named Operation Definitions", func(t *testing.T) {
			t.Run("5.2.1.1 Operation Name Uniqueness", func(t *testing.T) {
				t.Run("92", func(t *testing.T) {
					run(t, `
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
					run(t, `
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
					run(t, `	
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
					run(t, `	{
  							  		dog {
      									name
    								}
  								}`,
						LoneAnonymousOperation(), Valid)
				})
				t.Run("96", func(t *testing.T) {
					run(t, `	{
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
					run(t, `	query getDogName {
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
					run(t, `
							subscription sub {
								newMessage {
									body
									sender
								}
							}`,
						SubscriptionSingleRootField(), Valid)
				})
				t.Run("97 variant", func(t *testing.T) {
					run(t, `	
								query sub {
  									foo
									bar
								}`,
						SubscriptionSingleRootField(), Valid)
				})
				t.Run("97 variant", func(t *testing.T) {
					run(t, `	
								subscription sub {
  									... { foo }
  									... { bar }
								}`,
						SubscriptionSingleRootField(), Invalid)
				})
				t.Run("98", func(t *testing.T) {
					run(t, `	subscription sub {
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
					run(t, `	
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
					run(t, `	
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
					run(t, `
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
				run(t, `
							{
								dog {
									...aliasedLyingFieldTargetNotDefined
								}
							}
							fragment aliasedLyingFieldTargetNotDefined on Dog {
								barkVolume: kawVolume
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
			})
			t.Run("104 variant", func(t *testing.T) {
				run(t, `
							{
								dog {
									barkVolume: kawVolume
								}
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
			})
			t.Run("103", func(t *testing.T) {
				run(t, `	{
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
				run(t, `
							{
								dog {
									...definedOnImplementorsButNotInterface
								}
							}
							fragment definedOnImplementorsButNotInterface on Pet {
								nickname
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
			})
			t.Run("105", func(t *testing.T) {
				run(t, `	fragment inDirectFieldSelectionOnUnion on CatOrDog {
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
				run(t, `
							fragment inDirectFieldSelectionOnUnion on CatOrDog {
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
				run(t, `
							fragment inDirectFieldSelectionOnUnion on CatOrDog {
								__typename
	  							... on Pet {
	    							name
	  							}
	  							... {
	    							x
	  							}
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
			})
			t.Run("106", func(t *testing.T) {
				run(t, `
							fragment directFieldSelectionOnUnion on CatOrDog {
								name
								barkVolume
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
			})
			t.Run("106 variant", func(t *testing.T) {
				run(t, `
							fragment directFieldSelectionOnUnion on Cat {
								name {
									name
								}
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
			})
		})
		t.Run("5.3.2 Field Selection Merging", func(t *testing.T) {
			t.Run("introspection query", func(t *testing.T) {
				run(t, `query IntrospectionQuery {
						__schema {
							queryType {
								name
							}
							mutationType {
								name
							}
							subscriptionType {
								name
							}
							types {
								kind
								name
								description
								fields(includeDeprecated: true){
									name
									description
									args {
										name
										description
										type {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
																ofType {
																	kind
																	name
																	ofType {
																		kind
																		name
																	}
																}
															}
														}
													}
												}
											}
										}
										defaultValue
									}
									type {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
																ofType {
																	kind
																	name
																}
															}
														}
													}
												}
											}
										}
									}
									isDeprecated
									deprecationReason
								}
								inputFields {
									name
									description
									type {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
																ofType {
																	kind
																	name
																}
															}
														}
													}
												}
											}
										}
									}
									defaultValue
								}
								interfaces {
									kind
									name
									ofType {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
															}
														}
													}
												}
											}
										}
									}
								}
								enumValues(includeDeprecated: true){
									name
									description
									isDeprecated
									deprecationReason
								}
								possibleTypes {
									kind
									name
									ofType {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
															}
														}
													}
												}
											}
										}
									}
								}
							}
							directives {
								name
								description
								locations
								args {
									name
									description
									type {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
																ofType {
																	kind
																	name
																}
															}
														}
													}
												}
											}
										}
									}
									defaultValue
								}
							}
						}
					}`, FieldSelectionMerging(), Valid)
			})
			t.Run("reference implementation tests", func(t *testing.T) {
				t.Run("Same aliases allowed on non-overlapping fields", func(t *testing.T) {
					run(t, `
							fragment sameAliasesWithDifferentFieldTargets on Pet {
								... on Dog {
								  name
								}
								... on Cat {
								  name: nickname
								}
						  	}`, FieldSelectionMerging(), Valid)
				})
				t.Run("allows different args where no conflict is possible", func(t *testing.T) {
					run(t, `
							fragment conflictingArgs on Pet {
								... on Dog {
								  name(surname: true)
								}
								... on Cat {
								  name
								}
						 	 }`, FieldSelectionMerging(), Valid)
				})
				t.Run("encounters conflict in fragments", func(t *testing.T) {
					run(t, `
							{
								...A
								...B
							}
							fragment A on Query {
								x: a
							}
							fragment B on Query {
								x: b
							}`, FieldSelectionMerging(), Invalid)
				})
				t.Run("reports each conflict once", func(t *testing.T) {
					run(t, `
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
							  fragment A on Field {
								x: a
							  }
							  fragment B on Field {
								x: b
							  }`, FieldSelectionMerging(), Invalid)
				})
				t.Run("deep conflict", func(t *testing.T) {
					run(t, `
							{
								field {
								  x: a
								},
								field {
								  x: b
								}
						 	 }`, FieldSelectionMerging(), Invalid)
				})
				t.Run("deep conflict with multiple issues", func(t *testing.T) {
					run(t, `
							{
								field {
								  x: a
								  y: c
								},
								field {
								  x: b
								  y: d
								}
						 	}`, FieldSelectionMerging(), Invalid)
				})
				t.Run("very deep conflict", func(t *testing.T) {
					run(t, `
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
						 	 }`, FieldSelectionMerging(), Invalid)
				})
				t.Run("very deep conflict validated", func(t *testing.T) {
					run(t, `
							{
								field {
								  deepField {
									x: a
								  }
								},
								field {
								  deepField {
									x: a
								  }
								}
						 	 }`, FieldSelectionMerging(), Valid)
				})
				t.Run("reports deep conflict to nearest common ancestor", func(t *testing.T) {
					run(t, `
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
						}`, FieldSelectionMerging(), Invalid)
				})
				t.Run("reports deep conflict to nearest common ancestor in fragments", func(t *testing.T) {
					run(t, `
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
					  	}`, FieldSelectionMerging(), Invalid)
				})
				t.Run("reports deep conflict in nested fragments", func(t *testing.T) {
					run(t, `
							{
								field {
								  ...F
								}
								field {
								  ...I
								}
							  }
							  fragment F on Field {
								x: a
								...G
							  }
							  fragment G on Field {
								y: c
							  }
							  fragment I on Field {
								y: d
								...J
							  }
							  fragment J on Field {
								x: b
						 	 }`, FieldSelectionMerging(), Invalid)
				})
				t.Run("ignores unknown fragments", func(t *testing.T) {
					run(t, `
								{
									field
									...Unknown
									...Known
							 	 }
							 	 fragment Known on T {
									field
									...OtherUnknown
							 	 }`, FieldSelectionMerging(), Invalid, withExpectNormalizationError())
				})
				t.Run("return types must be unambiguous", func(t *testing.T) {
					t.Run("conflicting return types which potentially overlap", func(t *testing.T) {
						/*This is invalid since an object could potentially be both the Object
						type IntBox and the interface type NonNullStringBox1. While that
						condition does not exist in the current schema, the schema could
						expand in the future to allow this. Thus it is invalid.*/
						runWithDefinition(t, boxDefinition, `
							{
								someBox {
							  		...on IntBox {
										scalar
									}
									...on NonNullStringBox1 {
										scalar
									}
								}
							}`, FieldSelectionMerging(), Invalid)
					})
					t.Run("compatible return shapes on different return types", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
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
							}`, FieldSelectionMerging(), Valid)
					})
					t.Run("disallows differing return types despite no overlap", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
								{
									someBox {
								  		... on IntBox {
											scalar
								  		}
								  		... on StringBox {
											scalar
								  		}
									}
								}`, FieldSelectionMerging(), Invalid)
					})
					t.Run("disallows differing return types despite no overlap", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
								{
									someBox {
										... on IntBox {
											b: unrelatedField
								  		}
										... on SomeBox {
											b: unrelatedField
								  		}
										... on StringBox {
											b: scalar
								  		}
									}
								}`, FieldSelectionMerging(), Invalid)
					})
					t.Run("deeply nested", func(t *testing.T) {
						// reports correctly when a non-exclusive follows an exclusive
						runWithDefinition(t, boxDefinition, `
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
								}`, FieldSelectionMerging(), Invalid)
					})
					t.Run("disallows differing return type nullability despite no overlap", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
								{
									someBox {
								  		... on NonNullStringBox1 {
											scalar
								  		}
								  		... on StringBox {
											scalar
								  		}
									}
								}`, FieldSelectionMerging(), Invalid)
					})
					t.Run("disallows differing return type list despite no overlap", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
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
							 	}`, FieldSelectionMerging(), Invalid)
						runWithDefinition(t, boxDefinition, `
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
								  }`, FieldSelectionMerging(), Invalid)
					})
					t.Run("disallows differing subfields", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
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
								}`, FieldSelectionMerging(), Invalid)
					})
					t.Run("disallows differing deep return types despite no overlap", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
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
							}`, FieldSelectionMerging(), Invalid)
					})
					t.Run("allows non-conflicting overlapping types", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
							{
								someBox {
								  ... on IntBox {
									scalar: unrelatedField
								  }
								  ... on StringBox {
									scalar
								  }
								}
							}`, FieldSelectionMerging(), Valid)
					})
					t.Run("same wrapped scalar return types", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
							{
								someBox {
								  ...on NonNullStringBox1 {
									scalar
								  }
								  ...on NonNullStringBox2 {
									scalar
								  }
								}
							}`, FieldSelectionMerging(), Valid)
					})
					t.Run("allows inline typeless fragments", func(t *testing.T) {
						run(t, `
							{
								a
								... {
								  a
								}
							  }`, FieldSelectionMerging(), Valid)
					})
					t.Run("compares deep types including list", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
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
							}`, FieldSelectionMerging(), Invalid)
					})
					t.Run("ignores unknown types", func(t *testing.T) {
						runWithDefinition(t, boxDefinition, `
								{
								someBox {
								  ...on UnknownType {
									scalar
								  }
								  ...on NonNullStringBox2 {
									scalar
								  }
								}
								}`, FieldSelectionMerging(), Invalid, withExpectNormalizationError())
					})
				})
			})
			t.Run("107", func(t *testing.T) {
				run(t, `
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
				run(t, `	
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
				run(t, `
							fragment conflictingBecauseAlias on Dog {
  								name: nickname
  								name
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									name: nickname
  									name
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `
							query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `
							query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { noString: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra { string: noString }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	query conflictingBecauseAlias {
								dog {
  									extra { string }
  									extra: extras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	query conflictingBecauseAlias {
								dog {
  									extras { string }
  									extras: mustExtras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									x: extras { string }
  									x: mustExtras { string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string }
  									extras { string,string3: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string }
  									extras { string,string2: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									extras { string,string2: string2 }
  									extras { string,string2: string,string3: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									extras { ... { string },string2: string }
  									extras { ... { string },... { string },string2: string }
								}
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	query conflictingBecauseAlias {
								dog {
  									extras { ... { string },string: string1 }
  									extras { ... { string1: string },string2: string }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	
							query conflictingBecauseAlias {
								dog {
  									extras { ...frag, ...frag }
  									extras { ...frag }
								}
  							}
							fragment frag on DogExtra { string }`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	query conflictingBecauseAlias {
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
				run(t, `	
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
				run(t, `	query conflictingBecauseAlias {
								dog {
  									extra { looksLikeString: string }
  									extra { looksLikeString: bool }
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	query conflictingBecauseAlias {
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
				run(t, `	query conflictingBecauseAlias {
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
				run(t, `	query conflictingBecauseAlias {
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
				run(t, `	query conflictingBecauseAlias {
								dog {
  									name: nickname
  									... {
										name
									}
								}
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("108 variant", func(t *testing.T) {
				run(t, `	query conflictingBecauseAlias {
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
				run(t, `	query conflictingBecauseAlias {
											dog {
			  									name: nickname
			  									... @include(if: false) {
													name
												}
											}
			  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109", func(t *testing.T) {
				run(t, `
							fragment mergeIdenticalFieldsWithIdenticalArgs on Dog {
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
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1)
    							doesKnowCommand(dogCommand: 1)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	
							fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1)
    							doesKnowCommand(dogCommand: 0)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1.1)
    							doesKnowCommand(dogCommand: 1.1)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: 1.1)
    							doesKnowCommand(dogCommand: 0.1)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: "foo")
    							doesKnowCommand(dogCommand: "foo")
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: "foo")
    							doesKnowCommand(dogCommand: "bar")
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: null)
    							doesKnowCommand(dogCommand: null)
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: null)
    							doesKnowCommand(dogCommand: 0)
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [1.1])
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [0.1])
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: [1.1])
    							doesKnowCommand(dogCommand: [1.1,1.1])
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	
							fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "bar"})
  							}`,
					FieldSelectionMerging(), Valid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	
							fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {bar: "bar"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "baz"})
    							doesKnowCommand(dogCommand: {bar: "baz"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("109 variant", func(t *testing.T) {
				run(t, `	fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
  								doesKnowCommand(dogCommand: {foo: "bar"})
    							doesKnowCommand(dogCommand: {foo: "baz",bar: "bat"})
  							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("110", func(t *testing.T) {
				run(t, `	fragment conflictingArgsOnValues on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand: HEEL)
							}`,
					FieldSelectionMerging(), Invalid)
				run(t, `	fragment conflictingArgsOnValues on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand1: HEEL)
							}`,
					FieldSelectionMerging(), Invalid)
				run(t, `	fragment conflictingArgsValueAndVar on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand(dogCommand: $dogCommand)
							}`,
					FieldSelectionMerging(), Invalid)
				run(t, `	fragment conflictingArgsWithVars on Dog {
								doesKnowCommand(dogCommand: $varOne)
								doesKnowCommand(dogCommand: $varTwo)
							}`,
					FieldSelectionMerging(), Invalid)
				run(t, `	fragment differingArgs on Dog {
								doesKnowCommand(dogCommand: SIT)
								doesKnowCommand
							}`,
					FieldSelectionMerging(), Invalid)
			})
			t.Run("111", func(t *testing.T) {
				run(t, `	
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
				run(t, `
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
				run(t, `	
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
				run(t, `
							fragment conflictingDifferingResponses on Pet {
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
					FieldSelectionMerging(), Valid)
			})
			t.Run("112 variant", func(t *testing.T) {
				run(t, `	fragment conflictingDifferingResponses on Pet {
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
				run(t, `	fragment conflictingDifferingResponses on Pet {
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
				run(t, `	fragment conflictingDifferingResponses on Pet {
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
				run(t, `
							fragment conflictingDifferingResponses on Pet {
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
					FieldSelectionMerging(), Valid)
			})
			t.Run("112 variant", func(t *testing.T) {
				run(t, `	fragment conflictingDifferingResponses on Pet {
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
				run(t, `	fragment conflictingDifferingResponses on Pet {
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
				run(t, `	fragment conflictingDifferingResponses on Pet {
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
				run(t, `	fragment conflictingDifferingResponses on Pet {
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
				run(t, `query conflictingDifferingResponses {
								pet {
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
								}
							}`,
					FieldSelectionMerging(), Invalid, withExpectNormalizationError())
			})
			t.Run("112 variant", func(t *testing.T) {
				run(t, `
							fragment conflictingDifferingResponses on Pet {
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
					FieldSelectionMerging(), Valid)
			})
			t.Run("112 variant", func(t *testing.T) {
				run(t, `
								query conflictingDifferingResponses {
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
					FieldSelectionMerging(), Invalid, withExpectNormalizationError())
			})
			t.Run("112 variant", func(t *testing.T) {
				run(t, `	query conflictingDifferingResponses {
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
				run(t, `	query conflictingDifferingResponses {
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
				run(t, `	query conflictingDifferingResponses {
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
				run(t, `	query conflictingDifferingResponses {
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
					FieldSelectionMerging(), Invalid, withExpectNormalizationError())
			})
			t.Run("112 variant", func(t *testing.T) {
				run(t, `	
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
				run(t, `	fragment conflictingDifferingResponses on Pet {
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
				run(t, `query conflictingDifferingResponses {
								pet {
									...dogFrag
									...catFrag
								}
							}
							fragment dogFrag on Dog {
								someValue: barkVolume
							}
							fragment catFrag on Cat {
								someValue: name
							}`,
					FieldSelectionMerging(), Invalid, withExpectNormalizationError())
			})
			t.Run("112 variant", func(t *testing.T) {
				run(t, `
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
				run(t, `	
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
				run(t, `	query conflictingDifferingResponses {
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
				run(t, `	query conflictingDifferingResponses {
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
				run(t, `
							query conflictingDifferingResponses {
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
					FieldSelectionMerging(), Invalid, withExpectNormalizationError())
			})
			t.Run("112 variant", func(t *testing.T) {
				run(t, `	query conflictingDifferingResponses {
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
				run(t, `	fragment scalarSelection on Dog {
								barkVolume
							}`,
					FieldSelections(), Valid)
			})
			t.Run("114", func(t *testing.T) {
				run(t, `
							fragment scalarSelectionsNotAllowedOnInt on Dog {
								barkVolume {
									sinceWhen
								}
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
			})
			t.Run("116", func(t *testing.T) {
				run(t, `	
							query directQueryOnObjectWithoutSubFields {
								human
							}`,
					FieldSelections(), Invalid)
				run(t, `	query directQueryOnInterfaceWithoutSubFields {
								pet
							}`,
					FieldSelections(), Invalid)
				run(t, `	query directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid)
				run(t, `
							mutation directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
				run(t, `
							subscription directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid, withExpectNormalizationError())
			})
		})
	})
	t.Run("5.4 Arguments", func(t *testing.T) {
		t.Run("5.4.1 Argument Names", func(t *testing.T) {
			t.Run("117", func(t *testing.T) {
				run(t, `	
							fragment argOnRequiredArg on Dog {
								doesKnowCommand(dogCommand: SIT)
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					KnownArguments(), Valid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `	query argOnRequiredArg {
								dog {
									doesKnowCommand(dogCommand: SIT)
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					KnownArguments(), Valid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `	query argOnRequiredArg($dogCommand: DogCommand!) {
								dog {
									doesKnowCommand(dogCommand: $dogCommand)
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					KnownArguments(), Valid)
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `	
							query argOnRequiredArg($dogCommand: DogCommand = SIT) {
								dog {
									doesKnowCommand(dogCommand: $dogCommand)
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: true) @include(if: true)
							}`,
					KnownArguments(), Valid, withDisableNormalization())
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `
							query argOnRequiredArg($catCommand: CatCommand) {
								dog {
									doesKnowCommand(dogCommand: $catCommand)
								}
							}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$catCommand" of type "CatCommand" used in position expecting type "DogCommand!".`))
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `query argOnRequiredArg($dogCommand: CatCommand) {
									dog {
										... on Dog {
											doesKnowCommand(dogCommand: $dogCommand)
										}
									}
								}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$dogCommand" of type "CatCommand" used in position expecting type "DogCommand!".`))
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `	query argOnRequiredArg($booleanArg: Boolean) {
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
				run(t, `	
							query argOnRequiredArg($booleanArg: Boolean!) {
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
				run(t, `	
							query argOnRequiredArg($booleanArg: Boolean) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
							}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$booleanArg" of type "Boolean" used in position expecting type "Boolean!".`))
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `	query argOnRequiredArg($booleanArg: Boolean!) {
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
				run(t, `	query argOnRequiredArg($intArg: Integer) {
								dog {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $intArg) @include(if: true)
							}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$intArg" of type "Integer" used in position expecting type "Boolean".`))
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `	query argOnRequiredArg($intArg: Integer) {
								pet {
									...argOnOptional
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $intArg) @include(if: true)
							}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$intArg" of type "Integer" used in position expecting type "Boolean".`))
			})
			t.Run("117 variant", func(t *testing.T) {
				run(t, `	query argOnRequiredArg($intArg: Integer) {
								pet {
									...on Dog {
										...argOnOptional
									}
								}
							}
							fragment argOnOptional on Dog {
								isHousetrained(atOtherHomes: $intArg) @include(if: true)
							}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$intArg" of type "Integer" used in position expecting type "Boolean".`))
			})
			t.Run("118", func(t *testing.T) {
				run(t, `	
							{
								dog { ...invalidArgName}
							}
							fragment invalidArgName on Dog {
								doesKnowCommand(command: CLEAN_UP_HOUSE)
							}`,
					KnownArguments(), Invalid, withValidationErrors(`Unknown argument "command" on field "Dog.doesKnowCommand"`))
			})
			t.Run("118 variant", func(t *testing.T) {
				run(t, `	
							{
								dog { ...invalidArgName}
							}
							fragment invalidArgName on Dog {
								doesKnowCommand(dogCommand: CLEAN_UP_HOUSE)
							}`,
					Values(),
					Invalid,
					withValidationErrors(`Value "CLEAN_UP_HOUSE" does not exist in "DogCommand" enum.`))
			})
			t.Run("119", func(t *testing.T) {
				run(t, ` 	{
										dog { ...invalidArgName }
									}
									fragment invalidArgName on Dog {
										isHousetrained(atOtherHomes: true) @include(unless: false)
									}`,
					KnownArguments(), Invalid, withValidationErrors(`Unknown argument "unless" on directive "@include".`))
			})
			t.Run("121 args in reversed order", func(t *testing.T) {
				run(t, `	fragment multipleArgs on ValidArguments {
								multipleReqs(x: 1, y: 2)
							}
							fragment multipleArgsReverseOrder on ValidArguments {
								multipleReqs(y: 2, x: 1)
							}`,
					ValidArguments(), Valid)
			})
			t.Run("undefined arg", func(t *testing.T) {
				run(t, `	{
								dog(name: "Goofy"){ 
									name
								}
							}`,
					KnownArguments(), Invalid, withValidationErrors(`Unknown argument "name" on field "Query.dog".`))
			})
		})
		t.Run("5.4.2 Argument Uniqueness", func(t *testing.T) {
			t.Run("121 variant", func(t *testing.T) {
				run(t, `
								{
									arguments { ... multipleArgs }
								}
								fragment multipleArgs on ValidArguments {
									multipleReqs(x: 1, x: 2)
								}`,
					ArgumentUniqueness(), Invalid)
			})
			t.Run("121 variant", func(t *testing.T) {
				run(t, `{
									arguments { ... multipleArgs }
								}
								fragment multipleArgs on ValidArguments {
									multipleReqs(x: 1)
								}`,
					ArgumentUniqueness(), Valid)
			})
		})

		t.Run("Required Invalid Arguments", func(t *testing.T) {
			t.Run("required String", func(t *testing.T) {
				run(t, `	query requiredString {
										args {
											requiredString(s: foo)
										}
									}`,
					Values(), Invalid, withValidationErrors(`String cannot represent a non string value: foo`))
			})
			t.Run("required String", func(t *testing.T) {
				run(t, `	query requiredString {
										args {
											requiredString(s: null)
										}
									}`,
					Values(), Invalid, withValidationErrors(`Expected value of type "String!", found null`))
			})

			t.Run("required Float", func(t *testing.T) {
				run(t, `	query requiredFloat {
										args {
											requiredFloat(f: "1.1")
										}
									}`,
					Values(), Invalid, withValidationErrors(`Float cannot represent non numeric value: "1.1"`))
			})
			t.Run("required Float", func(t *testing.T) {
				run(t, `	query requiredFloat {
										args {
											requiredFloat(f: null)
										}
									}`,
					Values(), Invalid, withValidationErrors(`Expected value of type "Float!", found null`))
			})
		})

		t.Run("5.4.2.1 Required ValidArguments", func(t *testing.T) {
			t.Run("required String", func(t *testing.T) {
				run(t, `	query requiredString {
										args {
											requiredString(s: "foo")
										}
									}`,
					Values(), Valid)
			})

			t.Run("required Float", func(t *testing.T) {
				run(t, `	query requiredFloat {
										args {
											requiredFloat(f: 1.1)
										}
									}`,
					Values(), Valid)
			})
			t.Run("122", func(t *testing.T) {
				run(t, `	{
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
					RequiredArguments(), Valid)
			})
			t.Run("123", func(t *testing.T) {
				run(t, `	{
									arguments {
										...goodBooleanArgDefault
									}
								}
								fragment goodBooleanArgDefault on ValidArguments {
									booleanArgField
								}`,
					RequiredArguments(), Valid)
			})
			t.Run("124", func(t *testing.T) {
				run(t, `
								{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									nonNullBooleanArgField
								}`,
					RequiredArguments(), Invalid)
			})
			t.Run("125", func(t *testing.T) {
				run(t, `	{
									arguments {
										...missingRequiredArg
									}
								}
								fragment missingRequiredArg on ValidArguments {
									nonNullBooleanArgField(nonNullBooleanArg: null)
								}`,
					RequiredArguments(), Invalid)
			})
			t.Run("125 variant", func(t *testing.T) {
				run(t, `	{
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
				run(t, `	{
									booleanList (booleanListArg: [true])
								}`,
					RequiredArguments(), Valid)
			})
		})
	})
	t.Run("5.5 Fragments", func(t *testing.T) {
		t.Run("5.5.1 Fragment Declarations", func(t *testing.T) {
			t.Run("5.5.1.1 Fragment Name Uniqueness", func(t *testing.T) {
				t.Run("126", func(t *testing.T) {
					run(t, `
								{
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
					run(t, `	
								{
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
					run(t, `
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
					run(t, `	
								fragment notOnExistingType on NotInSchema {
  									name
								}`, Fragments(), Invalid, withExpectNormalizationError())
				})
				t.Run("129", func(t *testing.T) {
					run(t, `	
								fragment inlineNotExistingType on Dog {
  									... on NotInSchema {
    									name
  									}
								}`, Fragments(), Invalid, withExpectNormalizationError())
				})
			})
			t.Run("5.5.1.3 Fragments on Composite Types", func(t *testing.T) {
				t.Run("130", func(t *testing.T) {
					run(t, `
								{
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
					run(t, `
								fragment fragOnScalar on Int {
									something
								}`,
						Fragments(), Invalid, withExpectNormalizationError())
				})
				t.Run("131", func(t *testing.T) {
					run(t, `
								fragment inlineFragOnScalar on Dog {
									... on Boolean {
										somethingElse
									}
								}`,
						Fragments(), Invalid, withExpectNormalizationError())
				})
			})
			t.Run("5.5.1.4 Fragments must be used", func(t *testing.T) {
				t.Run("132", func(t *testing.T) {
					run(t, `
								fragment nameFragment on Dog {
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
					run(t, `
								fragment dogNames on Query {
									dog { name }
								}
								{
									...dogNames
								}`,
						Fragments(), Valid)
				})
				t.Run("132 variant", func(t *testing.T) {
					run(t, `
								fragment catNames on Query {
									dog { name }
								}
								{
									...dogNames
								}`,
						Fragments(), Invalid, withExpectNormalizationError())
				})
				t.Run("132 variant", func(t *testing.T) {
					run(t, `	fragment dogNames on Query {
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
				t.Run("Undefined fragment returns ErrFragmentUndefined", func(t *testing.T) {
					run(t, `
								{
									dog {
										...undefinedFragment
									}
								}`,
						Fragments(), Invalid, withExpectNormalizationError(), withValidationErrors("undefinedFragment undefined"))
				})
				t.Run("Undefined fragment after valid fragment returns ErrFragmentUndefined", func(t *testing.T) {
					run(t, `
								{
									cat {
										...validCatFragment
									}
									dog {
										...undefinedFragment
									}
								}
								fragment validCatFragment on Cat {
									name
									meowVolume
								}`,
						Fragments(), Invalid, withExpectNormalizationError(), withValidationErrors("undefinedFragment undefined"))
				})
			})
			t.Run("5.5.2.2 Fragment spreads must not form cycles", func(t *testing.T) {
				t.Run("134", func(t *testing.T) {
					run(t, `
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
						Fragments(), Invalid, withValidationErrors("external: fragment spread: nameFragment forms fragment cycle"), withDisableNormalization())
				})
				t.Run("136", func(t *testing.T) {
					run(t, `
								{
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
						Fragments(), Invalid, withExpectNormalizationError())
				})
				t.Run("136 variant", func(t *testing.T) {
					run(t, `
								{
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
						Fragments(), Invalid, withExpectNormalizationError())
				})
			})
			t.Run("5.5.2.3 Fragment spread is possible", func(t *testing.T) {
				t.Run("5.5.2.3.1 Object Spreads In Object Scope", func(t *testing.T) {
					t.Run("137", func(t *testing.T) {
						run(t, `
									{
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
						run(t, `
									{
										dog {
											...dogFragment
										}
									}
									fragment dogFragment on Dog {
										... on NoDog {
											barkVolume
										}
									}`,
							Fragments(), Invalid, withExpectNormalizationError())
					})
					t.Run("138", func(t *testing.T) {
						run(t, `
									{
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
					t.Run("138 variant", func(t *testing.T) {
						run(t, `
									{
										dog {
											...catInDogFragmentInvalid
										}
									}
									fragment catInDogFragmentInvalid on CatOrDog {
										... on Cat {
											meowVolume
										}
									}`,
							Fragments(), Valid)
					})
					t.Run("Spreading a fragment on an invalid type returns ErrInvalidFragmentSpread", func(t *testing.T) {
						run(t, `
									{
										dog {
											...invalidCatFragment
										}
									}
									fragment invalidCatFragment on Cat {
										meowVolume
									}`,
							Fragments(), Invalid, withValidationErrors("external: fragment spread: fragment invalidCatFragment must be spread on type Cat and not type Dog"))
					})
				})
				t.Run("5.5.2.3.2 Abstract Spreads in Object Scope", func(t *testing.T) {
					t.Run("139", func(t *testing.T) {
						run(t, ` 	{
										dog {
											...on Dog {
												...on Pet {
													name
												}
											}
										}
									}`,
							Fragments(), Valid)
					})
					t.Run("140", func(t *testing.T) {
						run(t, `
									{
										dog {
											...on Dog { ...on CatOrDog { ...on Cat { meowVolume } } }
										}
									}`,
							Fragments(), Valid)
					})
				})
				t.Run("5.5.2.3.3 Object Spreads In Abstract Scope", func(t *testing.T) {
					t.Run("141", func(t *testing.T) {
						run(t, ` {
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
						run(t, ` fragment sentientFragment on Sentient {
										... on Dog {
											barkVolume
										}
									}`,
							Fragments(), Invalid)
					})
					t.Run("142 variant", func(t *testing.T) {
						run(t, ` fragment humanOrAlienFragment on HumanOrAlien {
										... on Cat {
											meowVolume
										}
									}`,
							Fragments(), Invalid)
					})
				})
				t.Run("5.5.2.3.4 Abstract Spreads in Abstract Scope", func(t *testing.T) {
					t.Run("143", func(t *testing.T) {
						run(t, `
									{
										dog {
											...on Pet { ...on DogOrHuman { ...on Dog { barkVolume } } }
										}
									}`,
							Fragments(), Valid)
					})
					t.Run("143 variant", func(t *testing.T) {
						run(t, `
									{
										dog {
											...on DogOrHuman { ...on Pet { ...on Dog { barkVolume } } }
										}
									}`,
							Fragments(), Valid)
					})
					t.Run("144", func(t *testing.T) {
						run(t, `
									{
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
			t.Run("valid ID arguments", func(t *testing.T) {
				t.Run("ID as arg given as string", func(t *testing.T) {
					runWithDefinition(t, countriesDefinition, `{
						country(code: "DE") {
							code
							name
						}
					}`,
						Values(), Valid)
				})
				t.Run("ID as arg given as integer", func(t *testing.T) {
					runWithDefinition(t, countriesDefinition, `{
						country(code: 11) {
							code
							name
						}
					}`,
						Values(), Valid)
				})
			})

			t.Run("145", func(t *testing.T) {
				run(t, `
							query goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								findDog(complex: $search)
							}`,
					Values(), Valid)
				run(t, `
							query goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								...queryFragment
							}
							fragment queryFragment on Query { findDog(complex: $search) }`,
					Values(), Valid)
				run(t, `
							query goodComplexDefaultValue {
								arguments {
									booleanArgField(booleanArg: true)
								}
							}`,
					Values(), Valid)
				run(t, `
							query goodComplexDefaultValue() {
								arguments {
									floatArgField(floatArg: 123)
								}
							}`,
					Values(), Valid)
				run(t, `
							query goodComplexDefaultValue() {
								arguments {
									floatArgField(floatArg: 1.23)
								}
							}`,
					Values(), Valid)
			})
			t.Run("145 variant inline variable", func(t *testing.T) {
				run(t, `
							query goodComplexDefaultValue($name: String = "Fido" ) {
								findDog(complex: { name: $name })
							}`,
					Values(), Valid)
			})
			t.Run("145 variant variable non null into required field", func(t *testing.T) {
				run(t, `
							query goodComplexDefaultValue($name: String ) {
								findDogNonOptional(complex: { name: $name })
							}`,
					Values(), Invalid, withValidationErrors(`Variable "$name" of type "String" used in position expecting type "String!"`))
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `
							query goodComplexDefaultValue($search: ComplexInput = { name: 123 }) {
								findDog(complex: $search)
							}`,
					Values(), Invalid, withDisableNormalization(), withValidationErrors(`String cannot represent a non string value: 123`))
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query goodComplexDefaultValue($search: ComplexInput = { name: "123" }) {
									findDog(complex: $search)
								}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `	query goodComplexDefaultValue {
										findDog(complex: { name: 123 })
									}`,
					Values(), Invalid, withValidationErrors(`String cannot represent a non string value: 123`))
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `	query goodComplexDefaultValue {
										findDog(complex: { name: "123" })
									}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `	{
								dog {
									doesKnowCommand(dogCommand: SIT)
								}
							}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `	{
								dog {
									doesKnowCommand(dogCommand: MEOW)
								}
							}`,
					Values(), Invalid, withValidationErrors(`Value "MEOW" does not exist in "DogCommand" enum`))
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `	{
								dog {
									doesKnowCommand(dogCommand: [true])
								}
							}`,
					Values(), Invalid, withValidationErrors(`Enum "DogCommand" cannot represent non-enum value: [true]`))
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `	{
								dog {
									doesKnowCommand(dogCommand: {foo: "bar"})
								}
							}`,
					Values(), Invalid, withValidationErrors(`Enum "DogCommand" cannot represent non-enum value: {foo: "bar"}`))
			})
			t.Run("146", func(t *testing.T) {
				run(t, `
							{
								arguments { ...stringIntoInt }
							}
							fragment stringIntoInt on ValidArguments {
								intArgField(intArg: "123")
							}`,
					Values(), Invalid, withValidationErrors(`Int cannot represent non-integer value: "123"`))
				run(t, `
							query badComplexValue {
								findDog(complex: { name: 123 })
							}`,
					Values(), Invalid, withValidationErrors(`String cannot represent a non string value: 123`))
			})
			t.Run("146 variant", func(t *testing.T) {
				run(t, `
							query badComplexValue {
								findDog(complex: { name: "123" })
							}`,
					Values(), Valid)
			})
		})
		t.Run("5.6.2 Input Object Field Names", func(t *testing.T) {
			t.Run("147", func(t *testing.T) {
				run(t, `{
  									findDog(complex: { name: "Fido" })
								}`,
					Values(), Valid)
			})
			t.Run("148", func(t *testing.T) {
				run(t, `{
 									findDog(complex: { favoriteCookieFlavor: "Bacon" })
								}`,
					Values(), Invalid, withValidationErrors(`Field "favoriteCookieFlavor" is not defined by type "ComplexInput"`))
			})
		})
		t.Run("5.6.3 Input Object Field Uniqueness", func(t *testing.T) {
			t.Run("149", func(t *testing.T) {
				run(t, `{
									findDog(complex: { name: "Fido", name: "Goofy"})
								}`,
					Values(), Invalid, withValidationErrors(`There can be only one input field named "name"`))
			})
		})
		t.Run("5.6.4 Input Object Required Fields", func(t *testing.T) {
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query goodComplexDefaultValue($search: ComplexNonOptionalInput = { name: "123" }) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query goodComplexDefaultValue($search: ComplexNonOptionalInput = { name: null }) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Invalid, withDisableNormalization(), withValidationErrors(`Expected value of type "String!", found null`))
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query goodComplexDefaultValue($search: ComplexNonOptionalInput = {}) {
									findDogNonOptional(complex: $search)
								}`,
					Values(), Invalid, withDisableNormalization(), withValidationErrors(`Field "ComplexNonOptionalInput.name" of required type "String!" was not provided.`))
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query goodComplexDefaultValue {
									findDogNonOptional(complex: {})
								}`,
					Values(), Invalid, withValidationErrors(`Field "ComplexNonOptionalInput.name" of required type "String!" was not provided.`))
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query goodComplexDefaultValue {
									findDogNonOptional(complex: { name: "Goofy" })
								}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query goodComplexDefaultValue {
									...viaFragment
								}
								fragment viaFragment on Query {
									findDogNonOptional(complex: { name: "Goofy" })
								}`,
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query goodComplexDefaultValue {
									...viaFragment
								}
								fragment viaFragment on Query {
									findDogNonOptional(complex: { name: 123 })
								}`,
					Values(), Invalid, withValidationErrors(`String cannot represent a non string value: 123`))
			})
		})
		t.Run("complex nested validation", func(t *testing.T) {
			t.Run("complex nested 1", func(t *testing.T) {
				run(t, `
						{
							nested(input: {})
						}
						`, Values(), Invalid, withValidationErrors(`Field "NestedInput.requiredString" of required type "String!" was not provided.`))
			})
			t.Run("complex nested ok", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: ["str"]
							})
						}
						`, Values(), Valid)
			})
			t.Run("complex nested 'notList' is not list of Strings should be ok with coersion", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: "notList",
								requiredListOfRequiredStrings: ["str"]
							})
						}
						`, Values(), Valid)
			})
			t.Run("complex nested ok 3", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: ["str"],
								requiredListOfRequiredStrings: ["str"],
								requiredListOfOptionalStringsWithDefault: ["more strings"]
							})
						}
						`, Values(), Valid)
			})
			t.Run("complex nested ok optional list of nested input", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: ["str"],
								requiredListOfRequiredStrings: ["str"],
								requiredListOfOptionalStringsWithDefault: ["more strings"]
								optionalListOfNestedInput: [
									{
										requiredString: "str",
										requiredListOfOptionalStrings: [],
										requiredListOfRequiredStrings: ["str"]
									},
									{
										requiredString: "str",
										requiredListOfOptionalStrings: [],
										requiredListOfRequiredStrings: ["str"]
									}
								]
							})
						}
						`, Values(), Valid)
			})
			t.Run("complex nested ok optional list of nested input, required string missing", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: ["str"],
								requiredListOfRequiredStrings: ["str"],
								requiredListOfOptionalStringsWithDefault: ["more strings"]
								optionalListOfNestedInput: [
									{
										requiredListOfOptionalStrings: [],
										requiredListOfRequiredStrings: ["str"]
									}
								]
							})
						}
						`, Values(), Invalid, withValidationErrors(`Field "NestedInput.requiredString" of required type "String!" was not provided.`))
			})
			t.Run("complex nested 'str' is not String", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [str],
								requiredListOfRequiredStrings: ["str"],
								requiredListOfOptionalStringsWithDefault: ["more strings"]
							})
						}
						`, Values(), Invalid, withValidationErrors(`String cannot represent a non string value: str`))
			})
			t.Run("complex nested requiredListOfRequiredStrings could be empty but not `null` or `[null]`", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: []
							})
						}
						`, Values(), Valid)
			})
			t.Run("complex 2x nested", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: ["str"],
								optionalNestedInput: {
									requiredString: "str",
									requiredListOfOptionalStrings: [],
									requiredListOfRequiredStrings: ["str"],
								}
							})
						}
						`, Values(), Valid)
			})
			t.Run("complex 2x nested required string missing", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: ["str"],
								optionalNestedInput: {
									requiredListOfOptionalStrings: [],
									requiredListOfRequiredStrings: ["str"],
								}
							})
						}
						`, Values(), Invalid, withValidationErrors(`Field "NestedInput.requiredString" of required type "String!" was not provided.`))
			})
			t.Run("complex 2x nested '123' is no String", func(t *testing.T) {
				run(t, `
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: ["str"],
								optionalNestedInput: {
									requiredString: "str",
									requiredListOfOptionalStrings: [123],
									requiredListOfRequiredStrings: ["str"],
								}
							})
						}
						`, Values(), Invalid, withValidationErrors(`String cannot represent a non string value: 123`))
			})
		})
	})
	t.Run("5.7 Directives", func(t *testing.T) {
		t.Run("5.7.1 Directives Are Defined", func(t *testing.T) {
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query definedDirective {
									arguments {
										booleanArgField(booleanArg: true) @skip(if: true)
									}
								}`,
					DirectivesAreDefined(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query undefinedDirective {
									arguments {
										booleanArgField(booleanArg: true) @noSkip(if: true)
									}
								}`,
					DirectivesAreDefined(), Invalid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(t, `query undefinedDirective {
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
				run(t, `query @skip(if: true) {
									dog
								}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `query {
									dog @skip(if: true)
								}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `	{
								... @inline {
									dog
								}
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `	{
								... {
									dog @inline
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `
							{
								...frag @spread
							}
							fragment frag on Query {}`,
					DirectivesAreInValidLocations(), Valid, withDisableNormalization())
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `	{
								... {
									dog @spread
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `	{
								... {
									dog @fragmentDefinition
								}
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `	{
								...frag
							}
							fragment frag on Query @fragmentDefinition {}`,
					DirectivesAreInValidLocations(), Valid, withDisableNormalization())
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `	query @onQuery {
								dog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `	query @onMutation {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `	query @onSubscription {
								dog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `
							mutation @onQuery {
								mutateDog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `
							mutation @onSubscription {
								mutateDog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `
							mutation @onMutation {
								mutateDog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `
							subscription @onQuery {
								subscribeDog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `
							subscription @onMutation {
								foo
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(t, `
							subscription @onSubscription {
								foo
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
		})
		t.Run("5.7.3 Directives Are Unique Per Location", func(t *testing.T) {
			t.Run("151", func(t *testing.T) {
				run(t, `query MyQuery($foo: Boolean = true, $bar: Boolean = false) {
									field @skip(if: $foo) @skip(if: $bar)
								}`,
					DirectivesAreUniquePerLocation(), Invalid)
			})
			t.Run("152", func(t *testing.T) {
				run(t, `query MyQuery($foo: Boolean = true, $bar: Boolean = false) {
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
		t.Run("5.8.1 VariableValue Uniqueness", func(t *testing.T) {
			t.Run("153", func(t *testing.T) {
				run(t, `query houseTrainedQuery($atOtherHomes: Boolean, $atOtherHomes: Boolean) {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					VariableUniqueness(), Invalid)
			})
			t.Run("154", func(t *testing.T) {
				run(t, `
							query A($atOtherHomes: Boolean) {
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
				run(t, `
							query takesBoolean($atOtherHomes: Boolean) {
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
				run(t, `query TakesListOfBooleanBang($booleans: [Boolean!]) {
									booleanList(booleanListArg: $booleans)
								}`,
					VariablesAreInputTypes(), Valid)
			})
			t.Run("157", func(t *testing.T) {
				run(t, `query takesCat($cat: Cat) {}`,
					VariablesAreInputTypes(), Invalid)
				run(t, `query takesDogBang($dog: Dog!) {}`,
					VariablesAreInputTypes(), Invalid)
				run(t, `query takesListOfPet($pets: [Pet]) {}`,
					VariablesAreInputTypes(), Invalid)
				run(t, `query takesCatOrDog($catOrDog: CatOrDog) {}`,
					VariablesAreInputTypes(), Invalid)
				run(t, `query takesCatOrDog($catCommand: CatCommand) {}`,
					VariablesAreInputTypes(), Valid)
			})
		})
		t.Run("5.8.3 All VariableValue Uses Defined", func(t *testing.T) {
			t.Run("158", func(t *testing.T) {
				run(t, `query variableIsDefined($atOtherHomes: Boolean) {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					AllVariableUsesDefined(), Valid)
			})
			t.Run("159", func(t *testing.T) {
				run(t, `query variableIsNotDefined {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					AllVariableUsesDefined(), Invalid)
			})
			t.Run("160", func(t *testing.T) {
				run(t, `query variableIsDefinedUsedInSingleFragment($atOtherHomes: Boolean) {
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
				run(t, `query variableIsNotDefinedUsedInSingleFragment {
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
				run(t, `query variableIsNotDefinedUsedInNestedFragment {
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
					run(t, `query housetrainedQueryOne($atOtherHomes: Boolean) {
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
					run(t, `query housetrainedQueryOne($atOtherHomes: Boolean) {
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
				run(t, `	query variableUnused($name: String) {
										findDog(complex: {name: $name})
									}`,
					AllVariablesUsed(), Valid)
			})
			t.Run("165 variant nested", func(t *testing.T) {
				run(t, `	query variableUnused($name: String) {
										findNestedDog(complex: {nested: {name: $name}})
									}`,
					AllVariablesUsed(), Valid)
			})
			t.Run("165 variant - input object type variable", func(t *testing.T) {
				run(t, `query variableUnused($atOtherHomes: Boolean) {
									dog {
										isHousetrained
									}
								}`,
					AllVariablesUsed(), Invalid)
			})
			t.Run("165 variant", func(t *testing.T) {
				run(t, `query variableUnused($x: Int!) {
									arguments {
										multipleReqs(x: $x, y: 1)
									}
								}`,
					AllVariablesUsed(), Valid)
			})
			t.Run("166", func(t *testing.T) {
				run(t, `query variableUsedInFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedFragment
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained(atOtherHomes: $atOtherHomes)
								}`,
					AllVariablesUsed(), Valid)
			})
			t.Run("variable used in directive on fragment", func(t *testing.T) {
				t.Run("without normalization", func(t *testing.T) {
					run(t, `query variableUsedInFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedFragment @include(if: $atOtherHomes)
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained
								}`,
						AllVariablesUsed(), Valid, withDisableNormalization())
				})
				t.Run("with normalization", func(t *testing.T) {
					run(t, `query variableUsedInFragment($atOtherHomes: Boolean) {
									dog {
										...isHousetrainedFragment @include(if: $atOtherHomes)
									}
								}
								fragment isHousetrainedFragment on Dog {
									isHousetrained
								}`,
						AllVariablesUsed(), Valid)
				})
			})
			t.Run("167", func(t *testing.T) {
				run(t, `query variableNotUsedWithinFragment($atOtherHomes: Boolean) {
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
				run(t, `query queryWithUsedVar($atOtherHomes: Boolean) {
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
			t.Run("variables in array object", func(t *testing.T) {
				runWithDefinition(t, todoSchema, `mutation AddTak($title: String!, $completed: Boolean!, $name: String! @fromClaim(name: "sub")) {
  									addTask(input: [{title: $title, completed: $completed, user: {name: $name}}]){
										task {
										  id
										  title
										  completed
										}
									  }
								}`,
					AllVariablesUsed(), Valid)
			})
			t.Run("variables in nested array object", func(t *testing.T) {
				run(t, `query variableUnused($name: String) {
					dog(where: {
						AND: [
						  { nestedDog: { nickname: { eq: "Scooby Doo" } } }
						  { nestedDog: { name: { eq: $name } } }
						  {
							OR: [
							  {
								AND: [
								  {
									nestedDog: {
									  birthday: { eq: "2021-07-29" }
									}
								  }
								  {
									nestedDog: { barkVolume: { eq: 20 } }
								  }
								]
							  }
							  {
								AND: [
								  {
									nestedDog: {
										birthday: { eq: "2021-08-02" }
									}
								  }
								  {
									nestedDog: { barkVolume: { eq: 35 } }
								  }
								]
							  }
							]
						  }
						]
					  }) {
						id
						name
					}
				}`,
					AllVariablesUsed(), Valid)
			})
		})
		t.Run("5.8.5 All VariableValue Usages are Allowed", func(t *testing.T) {
			t.Run("169", func(t *testing.T) {
				run(t, `query intCannotGoIntoBoolean($intArg: Int) {
									arguments {
										booleanArgField(booleanArg: $intArg)
									}
								}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$intArg" of type "Int" used in position expecting type "Boolean".`))
			})
			t.Run("170", func(t *testing.T) {
				run(t, `query booleanListCannotGoIntoBoolean($booleanListArg: [Boolean]) {
									arguments {
										booleanArgField(booleanArg: $booleanListArg)
									}
								}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$booleanListArg" of type "[Boolean]" used in position expecting type "Boolean".`))
			})
			t.Run("171", func(t *testing.T) {
				run(t, `query booleanArgQuery($booleanArg: Boolean) {
									arguments {
										nonNullBooleanArgField(nonNullBooleanArg: $booleanArg)
									}
								}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$booleanArg" of type "Boolean" used in position expecting type "Boolean!".`))
			})
			// Non-null types are compatible with nullable types.
			t.Run("172", func(t *testing.T) {
				run(t, `query nonNullListToList($nonNullListOfBoolean: [Boolean]!) {
								arguments {
									nonNullListOfBooleanArgField(nonNullListOfBooleanArg: $nonNullListOfBoolean)
								}
							}`,
					ValidArguments(), Valid)
			})
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query listToList($listOfBoolean: [Boolean]) {
									arguments {
										listOfBooleanArgField(listOfBooleanArg: $listOfBoolean)
									}
								}`,
					ValidArguments(), Valid)
			})
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query listOfNonNullToList($listOfNonNullBoolean: [Boolean!]) {
									arguments {
										listOfBooleanArgField(listOfBooleanArg: $listOfNonNullBoolean)
									}
								}`,
					ValidArguments(), Valid)
			})
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query nonNullListOfNonNullToList($nonNullListOfNonNullBoolean: [Boolean!]!) {
									arguments {
										listOfBooleanArgField(listOfBooleanArg: $nonNullListOfNonNullBoolean)
									}
								}`,
					ValidArguments(), Valid)
			})
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query nonNullListToListLiteral {
									arguments {
										nonNullListOfBooleanArgField(nonNullListOfBooleanArg: [true,false,true])
									}
								}`,
					Values(), Valid)
			})
			// Types in lists must match
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query listContainingIncorrectType {
									arguments {
										nonNullListOfBooleanArgField(nonNullListOfBooleanArg: [true,false,"123"])
									}
								}`,
					Values(), Invalid, withValidationErrors(`Boolean cannot represent a non boolean value: "123"`))
			})
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query listContainingIncorrectType {
									arguments {
										nonNullListOfBooleanArgField(nonNullListOfBooleanArg: [true,false,123])
									}
								}`,
					Values(), Invalid, withValidationErrors(`Boolean cannot represent a non boolean value: 123`))
			})
			// Nullable types are NOT compatible with non-null types.
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query listToListOfNonNull($listOfBoolean: [Boolean]) {
									arguments {
										listOfNonNullBooleanArgField(listOfNonNullBooleanArg: $listOfBoolean)
									}
								}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$listOfBoolean" of type "[Boolean]" used in position expecting type "[Boolean!]"`))
			})
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query nonNullListToListOfNonNull($nonNullListOfBoolean: [Boolean]!) {
									arguments {
										listOfNonNullBooleanArgField(listOfNonNullBooleanArg: $nonNullListOfBoolean)
									}
								}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$nonNullListOfBoolean" of type "[Boolean]!" used in position expecting type "[Boolean!]"`))
			})
			t.Run("172 variant", func(t *testing.T) {
				run(t, `query listOfNonNullToNonNullList($listOfNonNullBoolean: [Boolean!]) {
									arguments {
										nonNullListOfBooleanArgField(nonNullListOfBooleanArg: $listOfNonNullBoolean)
									}
								}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$listOfNonNullBoolean" of type "[Boolean!]" used in position expecting type "[Boolean]!"`))
			})
			t.Run("173", func(t *testing.T) {
				run(t, `query listToNonNullList($listOfBoolean: [Boolean]) {
									arguments {
										nonNullListOfBooleanArgField(nonNullListOfBooleanArg: $listOfBoolean)
									}
								}`,
					ValidArguments(), Invalid, withValidationErrors(`Variable "$listOfBoolean" of type "[Boolean]" used in position expecting type "[Boolean]!"`))
			})
			t.Run("174", func(t *testing.T) {
				run(t, `query booleanArgQueryWithDefault($booleanArg: Boolean) {
									arguments {
										nonNullBooleanWithDefaultArgField(nonNullBooleanWithDefaultArg: $booleanArg)
									}
								}`,
					ValidArguments(), Valid)
			})
			t.Run("175", func(t *testing.T) {
				run(t, `query booleanArgQueryWithDefault($booleanArg: Boolean = true) {
									arguments {
										nonNullBooleanArgField(nonNullBooleanArg: $booleanArg)
									}
								}`,
					ValidArguments(), Valid, withDisableNormalization())
			})
			t.Run("complex values", func(t *testing.T) {
				runWithDefinition(t, wundergraphSchema, `
					query FirstNamespace($id: String $mode: QueryMode) {
						findFirstnamespace(where: {id: {equals: $id mode: $mode}}) {
							id
							name
							api {
								id
								name
								created_at
							}
						}
					}
					`, ValidArguments(), Valid)
			})
			t.Run("complex values", func(t *testing.T) {
				runWithDefinition(t, wundergraphSchema, `
					query FirstNamespace($id: String $mode: QueryMode) {
						findFirstnamespace(where: {id: {equals: $id mode: $mode}}) {
							id
							name
							api {
								id
								name
								created_at
							}
						}
					}
					`, Values(), Valid)
			})
			t.Run("complex values with input object", func(t *testing.T) {
				runWithDefinition(t, wundergraphSchema, `
					query FirstAPI($a: String $b: StringFilter) {
						findFirstapi(where: {id: {equals: $a} AND: {name: $b}}) {
							id
							name
							namespace {
								id
								name
							}
						}
					}
					`, Values(), Valid)
			})
			t.Run("with boolean input", func(t *testing.T) {
				runWithDefinition(t, wundergraphSchema, `
					query QueryWithBooleanInput($a: Boolean) {
						findFirstnodepool(
							where: { shared: { equals: $a } }
						) {
							id
						}
					}
					`, Values(), Valid)
			})
			t.Run("with nested boolean where clause", func(t *testing.T) {
				runWithDefinition(t, wundergraphSchema, `
					query QueryWithNestedBooleanClause($a: String) {
						findFirstnodepool(
							where: { id: { equals: $a }, AND: { shared: { equals: true } } }
						) {
							id
						}
					}
					`, Values(), Valid)
			})
			t.Run("with variables inside an input object", func(t *testing.T) {
				runWithDefinition(t, wundergraphSchema, `
					query QueryWithNestedBooleanClause($a: String, $b: Boolean) {
						findFirstnodepool(
							where: { id: { equals: $b }, AND: { shared: { equals: $a } } }
						) {
							id
						}
					}
					`, Values(), Invalid,
					withValidationErrors(
						`Variable "$a" of type "String" used in position expecting type "Boolean"`,
						`Variable "$b" of type "Boolean" used in position expecting type "String"`,
					))
			})

			t.Run("with variables inside an input object", func(t *testing.T) {
				run(t, `
					query booleanIntoStringList($a: Boolean) {
						findDog(complex: {optionalListOfOptionalStrings: $a}) {
							id
						}
					}
					`, Values(), Invalid,
					withValidationErrors(
						`Variable "$a" of type "Boolean" used in position expecting type "[String]"`,
					))
			})
		})
	})
}

func TestValidationEdgeCases(t *testing.T) {
	run := func(definition, operation string, withNormalization bool) func(t *testing.T) {
		return func(t *testing.T) {
			op := unsafeparser.ParseGraphqlDocumentString(operation)
			def := unsafeparser.ParseGraphqlDocumentString(definition)

			if withNormalization {
				report := operationreport.Report{}
				normalizer := astnormalization.NewWithOpts(
					astnormalization.WithExtractVariables(),
					astnormalization.WithRemoveFragmentDefinitions(),
					astnormalization.WithRemoveUnusedVariables(),
					astnormalization.WithNormalizeDefinition(),
				)
				normalizer.NormalizeOperation(&op, &def, &report)
				if report.HasErrors() {
					panic(report.Error())
				}
			}

			validator := DefaultOperationValidator()
			var report operationreport.Report
			validator.Validate(&op, &def, &report)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
		}
	}

	t.Run("validation with typename", run(
		`
		schema {
			query: Query
		}
		type Query {
			api(id: String): ApiResult
		}
		union ApiResult = Api | RequestResult
		type Api {
			id: String
			name: String
		}
		type RequestResult {
			status: String
			message: String
		}
		scalar String
	`,
		`
		query getApi($id: String!) {
		  api(id: $id) {
			__typename
			... on Api {
			  __typename
			  id
			  name
			}
			... on RequestResult {
			  __typename
			  status
			  message
			}  
		  }
		}`, false,
	))

	t.Run("validation for normalized federation schema", run(
		`
		scalar _Any
		scalar String
		union _Entity = User
		
		extend type Query {
			_entities(representations: [_Any!]!): [_Entity]!
		}

		extend type Query {
			me: User!
		}

		extend type User {
			name: String!
		}`,
		`
		query($representations: [_Any!]!) {
			_entities(representations: $representations) {
				... on User { 
					name 
				}
			}
		}`, true,
	))
}

func BenchmarkValidation(b *testing.B) {
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	mustDocument := func(doc ast.Document, report operationreport.Report) ast.Document {
		if report.HasErrors() {
			must(report)
		}
		return doc
	}

	run := func(b *testing.B, definition, operation string, state ValidationState) {
		op, def := mustDocument(astparser.ParseGraphqlDocumentString(operation)), mustDocument(astparser.ParseGraphqlDocumentString(definition))
		report := operationreport.Report{}
		astnormalization.NormalizeOperation(&op, &def, &report)
		if report.HasErrors() {
			panic(report.Error())
		}

		validator := DefaultOperationValidator()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			report.Reset()
			out := validator.Validate(&op, &def, &report)
			if out != state {
				panic(fmt.Errorf("want state: %s, got: %s, reason: %s", state, out, report.Error()))
			}
		}
	}

	b.Run("simple query", func(b *testing.B) {
		run(b, testDefinition, `
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
				}`, Valid)
	})
	b.Run("complex", func(b *testing.B) {
		run(b, testDefinition, `
				query housetrainedQueryOne($atOtherHomes: Boolean) {
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
				}`, Valid)
	})
	b.Run("nested", func(b *testing.B) {
		run(b, testDefinition, `
				{
					nested(input: {
						requiredString: "str",
						requiredListOfOptionalStrings: ["str"],
						requiredListOfRequiredStrings: ["str"],
						requiredListOfOptionalStringsWithDefault: ["more strings"]
						optionalListOfNestedInput: [
							{
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: ["str"]
							},
							{
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: ["str"]
							}
						]
					})
				}`, Valid)
	})
	b.Run("args", func(b *testing.B) {
		run(b, testDefinition, `
				{
					arguments { ...stringIntoInt }
				}
				fragment stringIntoInt on ValidArguments {
					intArgField(intArg: "123")
				}`, Invalid)
	})
	b.Run("interfaces", func(b *testing.B) {
		run(b, testDefinition, `
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
					someValue: barkVolume
				}`, Valid)
	})
	b.Run("union", func(b *testing.B) {
		run(b, testDefinition, `
				fragment inDirectFieldSelectionOnUnion on CatOrDog {
					__typename
					... on Pet {
						name
					}
					... on Dog {
						nickname
					}
				}`, Invalid)
	})
	b.Run("nested object", func(b *testing.B) {
		run(b, nexusSchema, `
				mutation ($drawDate: AWSDate!, $pick: [String!]!) {
					AddTicket: addCartItem(
						item: {
							drawDate: $drawDate
							fractional: false
							play: {
								pick: $pick
							}
							quantity: 1
							regionGameId: "lucky7|UAE"
						}
					) {
						id
					}
				}`, Valid)
	})
	b.Run("nested object wrong type", func(b *testing.B) {
		run(b, nexusSchema, `
				mutation ($drawDate: AWSDate!, $pick: [Int!]!) {
					AddTicket: addCartItem(
						item: {
							drawDate: $drawDate
							fractional: false
							play: {
								pick: $pick
							}
							quantity: 1
							regionGameId: "lucky7|UAE"
						}
					) {
						id
					}
				}`, Invalid)
	})
	b.Run("nested object not optional", func(b *testing.B) {
		run(b, nexusSchema, `
				mutation ($drawDate: AWSDate!, $pick: [String]!) {
					AddTicket: addCartItem(
						item: {
							drawDate: $drawDate
							fractional: false
							play: {
								pick: $pick
							}
							quantity: 1
							regionGameId: "lucky7|UAE"
						}
					) {
						id
					}
				}`, Invalid)
	})
	b.Run("nested object not optional list", func(b *testing.B) {
		run(b, nexusSchema, `
				mutation ($drawDate: AWSDate!, $pick: [String!]) {
					AddTicket: addCartItem(
						item: {
							drawDate: $drawDate
							fractional: false
							play: {
								pick: $pick
							}
							quantity: 1
							regionGameId: "lucky7|UAE"
						}
					) {
						id
					}
				}`, Invalid)
	})
	b.Run("introspection", func(b *testing.B) {
		run(b, testDefinition, `query IntrospectionQuery {
						__schema {
							queryType {
								name
							}
							mutationType {
								name
							}
							subscriptionType {
								name
							}
							types {
								kind
								name
								description
								fields(includeDeprecated: true){
									name
									description
									args {
										name
										description
										type {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
																ofType {
																	kind
																	name
																	ofType {
																		kind
																		name
																	}
																}
															}
														}
													}
												}
											}
										}
										defaultValue
									}
									type {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
																ofType {
																	kind
																	name
																}
															}
														}
													}
												}
											}
										}
									}
									isDeprecated
									deprecationReason
								}
								inputFields {
									name
									description
									type {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
																ofType {
																	kind
																	name
																}
															}
														}
													}
												}
											}
										}
									}
									defaultValue
								}
								interfaces {
									kind
									name
									ofType {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
															}
														}
													}
												}
											}
										}
									}
								}
								enumValues(includeDeprecated: true){
									name
									description
									isDeprecated
									deprecationReason
								}
								possibleTypes {
									kind
									name
									ofType {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
															}
														}
													}
												}
											}
										}
									}
								}
							}
							directives {
								name
								description
								locations
								args {
									name
									description
									type {
										kind
										name
										ofType {
											kind
											name
											ofType {
												kind
												name
												ofType {
													kind
													name
													ofType {
														kind
														name
														ofType {
															kind
															name
															ofType {
																kind
																name
																ofType {
																	kind
																	name
																}
															}
														}
													}
												}
											}
										}
									}
									defaultValue
								}
							}
						}
					}`, Valid)
	})
}

var testDefinition = `
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
	subscribeDog: Dog
	newMessage: Message
	foo: String
	bar: String
	disallowedSecondRootField: Boolean
}

type Mutation {
	mutateDog: Dog
}

input ComplexInput { name: String, owner: String, optionalListOfOptionalStrings: [String]}
input ComplexNestedInput { complex: ComplexInput }
input ComplexNonOptionalInput { name: String! }

input NestedInput {
	requiredString: String!
	requiredStringWithDefault: String! = "defaultString"
	optionalListOfOptionalStrings: [String]
	requiredListOfOptionalStrings: [String]!
	requiredListOfOptionalStringsWithDefault: [String]! = []
	requiredListOfRequiredStrings: [String!]!
	optionalNestedInput: NestedInput
	optionalListOfNestedInput: [NestedInput]
}

type Field {
	subfieldA: String
	subfieldB: String
	deepField: DeepField
	a: String
	b: String
	c: String
	d: String
}

type DeepField {
	a: String
	b: String
	y: String
	deeperField: DeepField
}

type T {
	deepField: DeepField
	a: String
	b: String
	c: String
	d: String
}

type Query {
	__schema: __Schema!
	f1: Field
	f2: Field
	f3: Field
	a: String!
	b: String!
	field: Field
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
	findNestedDog(complex: ComplexNestedInput): Dog
	findDogNonOptional(complex: ComplexNonOptionalInput): Dog
  	booleanList(booleanListArg: [Boolean!]): Boolean
	extra: Extra
	nested(input: NestedInput): Boolean
	args: Arguments
}

type Arguments {
	requiredString(s: String!): String!
	requiredFloat(f: Float!): Float!
}

type ValidArguments {
	multipleReqs(x: Int!, y: Int!): Int!
	floatArgField(floatArg: Float): Float
	intArgField(intArg: Int): Int
	booleanArgField(booleanArg: Boolean): String
	nonNullBooleanArgField(nonNullBooleanArg: Boolean!): String
	listOfBooleanArgField(listOfBooleanArg: [Boolean]): String
	listOfNonNullBooleanArgField(listOfNonNullBooleanArg: [Boolean!]): String
	nonNullListOfBooleanArgField(nonNullListOfBooleanArg: [Boolean]!): String
	nonNullListOfNonNullBooleanArgField(nonNullListOfNonNullBooleanArg: [Boolean!]!): String
	nonNullBooleanWithDefaultArgField(nonNullBooleanWithDefaultArg: Boolean! = false): String
}

enum DogCommand { SIT, DOWN, HEEL }

type Dog implements Pet {
	id: ID
	name: String!
	nickname: String!
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
	string1: String
	string2: String
	string3: String
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
	nickname: String!
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
directive @spread on FRAGMENT_SPREAD | INLINE_FRAGMENT
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
}`

const boxDefinition = `
scalar String
scalar ID
scalar Int
interface SomeBox {
	scalar: String
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
schema {
	query: Query
}`

const countriesDefinition = `directive @cacheControl(maxAge: Int, scope: CacheControlScope) on FIELD_DEFINITION | OBJECT | INTERFACE

scalar String
scalar ID
scalar Boolean

schema {
	query: Query
}

enum CacheControlScope {
  PUBLIC
  PRIVATE
}

type Continent {
  code: ID!
  name: String!
  countries: [Country!]!
}

input ContinentFilterInput {
  code: StringQueryOperatorInput
}

type Country {
  code: ID!
  name: String!
  native: String!
  phone: String!
  continent: Continent!
  capital: String
  currency: String
  languages: [Language!]!
  emoji: String!
  emojiU: String!
  states: [State!]!
}

input CountryFilterInput {
  code: StringQueryOperatorInput
  currency: StringQueryOperatorInput
  continent: StringQueryOperatorInput
}

type Language {
  code: ID!
  name: String
  native: String
  rtl: Boolean!
}

input LanguageFilterInput {
  code: StringQueryOperatorInput
}

type Query {
  continents(filter: ContinentFilterInput): [Continent!]!
  continent(code: ID!): Continent
  countries(filter: CountryFilterInput): [Country!]!
  country(code: ID!): Country
  languages(filter: LanguageFilterInput): [Language!]!
  language(code: ID!): Language
}

type State {
  code: String
  name: String!
  country: Country!
}

input StringQueryOperatorInput {
  eq: String
  ne: String
  in: [String]
  nin: [String]
  regex: String
  glob: String
}

"""The Upload scalar type represents a file upload."""
scalar Upload
`

const todoSchema = `

schema {
	query: Query
	mutation: Mutation
}

scalar ID
scalar String
scalar Boolean

""""""
scalar DateTime

""""""
enum DgraphIndex {
  """"""
  int
  """"""
  float
  """"""
  bool
  """"""
  hash
  """"""
  exact
  """"""
  term
  """"""
  fulltext
  """"""
  trigram
  """"""
  regexp
  """"""
  year
  """"""
  month
  """"""
  day
  """"""
  hour
}

""""""
input DateTimeFilter {
  """"""
  eq: DateTime
  """"""
  le: DateTime
  """"""
  lt: DateTime
  """"""
  ge: DateTime
  """"""
  gt: DateTime
}

""""""
input StringHashFilter {
  """"""
  eq: String
}

""""""
type UpdateTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  numUids: Int
}

""""""
type Subscription {
  """"""
  getTask(id: ID!): Task
  """"""
  queryTask(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  getUser(username: String!): User
  """"""
  queryUser(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
}

""""""
input FloatFilter {
  """"""
  eq: Float
  """"""
  le: Float
  """"""
  lt: Float
  """"""
  ge: Float
  """"""
  gt: Float
}

""""""
input StringTermFilter {
  """"""
  allofterms: String
  """"""
  anyofterms: String
}

""""""
type DeleteTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  msg: String
  """"""
  numUids: Int
}

""""""
type Mutation {
  """"""
  addTask(input: [AddTaskInput!]!): AddTaskPayload
  """"""
  updateTask(input: UpdateTaskInput!): UpdateTaskPayload
  """"""
  deleteTask(filter: TaskFilter!): DeleteTaskPayload
  """"""
  addUser(input: [AddUserInput!]!): AddUserPayload
  """"""
  updateUser(input: UpdateUserInput!): UpdateUserPayload
  """"""
  deleteUser(filter: UserFilter!): DeleteUserPayload
}

""""""
enum HTTPMethod {
  """"""
  GET
  """"""
  POST
  """"""
  PUT
  """"""
  PATCH
  """"""
  DELETE
}

""""""
type DeleteUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  msg: String
  """"""
  numUids: Int
}

""""""
input TaskFilter {
  """"""
  id: [ID!]
  """"""
  title: StringFullTextFilter
  """"""
  completed: Boolean
  """"""
  and: TaskFilter
  """"""
  or: TaskFilter
  """"""
  not: TaskFilter
}

""""""
type UpdateUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  numUids: Int
}

""""""
input TaskRef {
  """"""
  id: ID
  """"""
  title: String
  """"""
  completed: Boolean
  """"""
  user: UserRef
}

""""""
input UserFilter {
  """"""
  username: StringHashFilter
  """"""
  name: StringExactFilter
  """"""
  and: UserFilter
  """"""
  or: UserFilter
  """"""
  not: UserFilter
}

""""""
input UserOrder {
  """"""
  asc: UserOrderable
  """"""
  desc: UserOrderable
  """"""
  then: UserOrder
}

""""""
input AuthRule {
  """"""
  and: [AuthRule]
  """"""
  or: [AuthRule]
  """"""
  not: AuthRule
  """"""
  rule: String
}

""""""
type AddTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  numUids: Int
}

""""""
type AddUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  numUids: Int
}

""""""
type Task {
  """"""
  id: ID!
  """"""
  title: String!
  """"""
  completed: Boolean!
  """"""
  user(filter: UserFilter): User!
}

""""""
input IntFilter {
  """"""
  eq: Int
  """"""
  le: Int
  """"""
  lt: Int
  """"""
  ge: Int
  """"""
  gt: Int
}

""""""
input StringExactFilter {
  """"""
  eq: String
  """"""
  le: String
  """"""
  lt: String
  """"""
  ge: String
  """"""
  gt: String
}

""""""
enum UserOrderable {
  """"""
  username
  """"""
  name
}

""""""
input AddTaskInput {
  """"""
  title: String!
  """"""
  completed: Boolean!
  """"""
  user: UserRef!
}

""""""
input TaskPatch {
  """"""
  title: String
  """"""
  completed: Boolean
  """"""
  user: UserRef
}

""""""
input UserRef {
  """"""
  username: String
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
input StringFullTextFilter {
  """"""
  alloftext: String
  """"""
  anyoftext: String
}

""""""
enum TaskOrderable {
  """"""
  title
}

""""""
input UpdateTaskInput {
  """"""
  filter: TaskFilter!
  """"""
  set: TaskPatch
  """"""
  remove: TaskPatch
}

""""""
input UserPatch {
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
type Query {
  """"""
  getTask(id: ID!): Task
  """"""
  queryTask(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  getUser(username: String!): User
  """"""
  queryUser(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
}

""""""
type User {
  """"""
  username: String!
  """"""
  name: String
  """"""
  tasks(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
}

""""""
enum Mode {
  """"""
  BATCH
  """"""
  SINGLE
}

""""""
input CustomHTTP {
  """"""
  url: String!
  """"""
  method: HTTPMethod!
  """"""
  body: String
  """"""
  graphql: String
  """"""
  mode: Mode
  """"""
  forwardHeaders: [String!]
  """"""
  secretHeaders: [String!]
  """"""
  introspectionHeaders: [String!]
  """"""
  skipIntrospection: Boolean
}

""""""
input StringRegExpFilter {
  """"""
  regexp: String
}

""""""
input AddUserInput {
  """"""
  username: String!
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
input TaskOrder {
  """"""
  asc: TaskOrderable
  """"""
  desc: TaskOrderable
  """"""
  then: TaskOrder
}

""""""
input UpdateUserInput {
  """"""
  filter: UserFilter!
  """"""
  set: UserPatch
  """"""
  remove: UserPatch
}
"""
The @cache directive caches the response server side and sets cache control headers according to the configuration.
With this setting you can reduce the load on your backend systems for operations that get hit a lot while data doesn't change that frequently. 
"""
directive @cache(
  """maxAge defines the maximum time in seconds a response will be understood 'fresh', defaults to 300 (5 minutes)"""
  maxAge: Int! = 300
  """
  vary defines the headers to append to the cache key
  In addition to all possible headers you can also select a custom claim for authenticated requests
  Examples: 'jwt.sub', 'jwt.team' to vary the cache key based on 'sub' or 'team' fields on the jwt. 
  """
  vary: [String]! = []
) on QUERY

"""The @auth directive lets you configure auth for a given operation"""
directive @auth(
  """disable explicitly disables authentication for the annotated operation"""
  disable: Boolean! = false
) on QUERY | MUTATION | SUBSCRIPTION

"""The @fromClaim directive overrides a variable from a select claim in the jwt"""
directive @fromClaim(
  """
  name is the name of the claim you want to use for the variable
  examples: sub, team, custom.nested.claim
  """
  name: String!
) on VARIABLE_DEFINITION
`

const wundergraphSchema = `

schema {
	query: Query
	mutation: Mutation
}

scalar String
scalar Boolean

enum QueryMode {
  default
  insensitive
}

input NestedStringFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  not: NestedStringFilter
}

input StringFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  mode: QueryMode
  not: NestedStringFilter
}

input NestedDateTimeFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: NestedDateTimeFilter
}

input DateTimeFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: NestedDateTimeFilter
}

input NestedStringNullableFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  not: NestedStringNullableFilter
}

input StringNullableFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  mode: QueryMode
  not: NestedStringNullableFilter
}

enum user_role {
  user
  admin
}

input Enumuser_roleFilter {
  equals: user_role
  in: [user_role]
  notIn: [user_role]
  not: user_role
}

input NestedDateTimeNullableFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: NestedDateTimeNullableFilter
}

input DateTimeNullableFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: NestedDateTimeNullableFilter
}

input Access_tokenListRelationFilter {
  every: access_tokenWhereInput
  some: access_tokenWhereInput
  none: access_tokenWhereInput
}

enum membership {
  owner
  maintainer
  viewer
  guest
}

input EnummembershipFilter {
  equals: membership
  in: [membership]
  notIn: [membership]
  not: membership
}

input NestedIntFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: NestedIntFilter
}

input IntFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: NestedIntFilter
}

input NestedBoolFilter {
  equals: Boolean
  not: NestedBoolFilter
}

input BoolFilter {
  equals: Boolean
  not: NestedBoolFilter
}

input NamespaceListRelationFilter {
  every: namespaceWhereInput
  some: namespaceWhereInput
  none: namespaceWhereInput
}

input price_planWhereInput {
  AND: price_planWhereInput
  OR: [price_planWhereInput]
  NOT: price_planWhereInput
  id: IntFilter
  name: StringFilter
  quota_daily_requests: IntFilter
  quota_environments: IntFilter
  quota_members: IntFilter
  quota_apis: IntFilter
  allow_secondary_environments: BoolFilter
  namespace: NamespaceListRelationFilter
}

input Price_planRelationFilter {
  is: price_planWhereInput
  isNot: price_planWhereInput
}

input JsonFilter {
  equals: DateTime
  not: DateTime
}

input ApiRelationFilter {
  is: apiWhereInput
  isNot: apiWhereInput
}

input DeploymentRelationFilter {
  is: deploymentWhereInput
  isNot: deploymentWhereInput
}

input StringNullableListFilter {
  equals: [String]
  has: String
  hasEvery: [String]
  hasSome: [String]
  isEmpty: Boolean
}

input edgeWhereInput {
  AND: edgeWhereInput
  OR: [edgeWhereInput]
  NOT: edgeWhereInput
  id: StringFilter
  name: StringFilter
  location: StringFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  environment_edges: Environment_edgesListRelationFilter
}

input EdgeRelationFilter {
  is: edgeWhereInput
  isNot: edgeWhereInput
}

input environment_edgesWhereInput {
  AND: environment_edgesWhereInput
  OR: [environment_edgesWhereInput]
  NOT: environment_edgesWhereInput
  environment_id: StringFilter
  edge_id: StringFilter
  edge: EdgeRelationFilter
  environment: EnvironmentRelationFilter
}

input Environment_edgesListRelationFilter {
  every: environment_edgesWhereInput
  some: environment_edgesWhereInput
  none: environment_edgesWhereInput
}

input NodepoolListRelationFilter {
  every: nodepoolWhereInput
  some: nodepoolWhereInput
  none: nodepoolWhereInput
}

input wundernodeWhereInput {
  AND: wundernodeWhereInput
  OR: [wundernodeWhereInput]
  NOT: wundernodeWhereInput
  id: StringFilter
  etag: StringFilter
  config: JsonFilter
  ipv4: StringNullableFilter
  ipv6: StringNullableFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  nodepool: NodepoolListRelationFilter
}

input WundernodeRelationFilter {
  is: wundernodeWhereInput
  isNot: wundernodeWhereInput
}

input nodepoolWhereInput {
  AND: nodepoolWhereInput
  OR: [nodepoolWhereInput]
  NOT: nodepoolWhereInput
  id: StringFilter
  wundernode_id: StringFilter
  shared: BoolFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  wundernode: WundernodeRelationFilter
  nodepool_environment: Nodepool_environmentListRelationFilter
}

input NodepoolRelationFilter {
  is: nodepoolWhereInput
  isNot: nodepoolWhereInput
}

input nodepool_environmentWhereInput {
  AND: nodepool_environmentWhereInput
  OR: [nodepool_environmentWhereInput]
  NOT: nodepool_environmentWhereInput
  nodepool_id: StringFilter
  environment_id: StringFilter
  environment: EnvironmentRelationFilter
  nodepool: NodepoolRelationFilter
}

input Nodepool_environmentListRelationFilter {
  every: nodepool_environmentWhereInput
  some: nodepool_environmentWhereInput
  none: nodepool_environmentWhereInput
}

input environmentWhereInput {
  AND: environmentWhereInput
  OR: [environmentWhereInput]
  NOT: environmentWhereInput
  id: StringFilter
  name: StringFilter
  namespace_id: StringFilter
  primary_hostname: StringFilter
  hostnames: StringNullableListFilter
  primary: BoolFilter
  namespace: NamespaceRelationFilter
  deployment_environment: Deployment_environmentListRelationFilter
  environment_edges: Environment_edgesListRelationFilter
  nodepool_environment: Nodepool_environmentListRelationFilter
}

input EnvironmentRelationFilter {
  is: environmentWhereInput
  isNot: environmentWhereInput
}

input deployment_environmentWhereInput {
  AND: deployment_environmentWhereInput
  OR: [deployment_environmentWhereInput]
  NOT: deployment_environmentWhereInput
  deployment_id: StringFilter
  environment_id: StringFilter
  deployment: DeploymentRelationFilter
  environment: EnvironmentRelationFilter
}

input Deployment_environmentListRelationFilter {
  every: deployment_environmentWhereInput
  some: deployment_environmentWhereInput
  none: deployment_environmentWhereInput
}

input deploymentWhereInput {
  AND: deploymentWhereInput
  OR: [deploymentWhereInput]
  NOT: deploymentWhereInput
  id: StringFilter
  api_id: StringFilter
  name: StringFilter
  config: JsonFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  api: ApiRelationFilter
  deployment_environment: Deployment_environmentListRelationFilter
}

input DeploymentListRelationFilter {
  every: deploymentWhereInput
  some: deploymentWhereInput
  none: deploymentWhereInput
}

input apiWhereInput {
  AND: apiWhereInput
  OR: [apiWhereInput]
  NOT: apiWhereInput
  id: StringFilter
  namespace_id: StringFilter
  name: StringFilter
  markdown_description: StringFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  namespace: NamespaceRelationFilter
  deployment: DeploymentListRelationFilter
}

input ApiListRelationFilter {
  every: apiWhereInput
  some: apiWhereInput
  none: apiWhereInput
}

input EnvironmentListRelationFilter {
  every: environmentWhereInput
  some: environmentWhereInput
  none: environmentWhereInput
}

input namespaceWhereInput {
  AND: namespaceWhereInput
  OR: [namespaceWhereInput]
  NOT: namespaceWhereInput
  id: StringFilter
  name: StringFilter
  price_plan_id: IntFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  price_plan: Price_planRelationFilter
  api: ApiListRelationFilter
  environment: EnvironmentListRelationFilter
  namespace_members: Namespace_membersListRelationFilter
}

input NamespaceRelationFilter {
  is: namespaceWhereInput
  isNot: namespaceWhereInput
}

input namespace_membersWhereInput {
  AND: namespace_membersWhereInput
  OR: [namespace_membersWhereInput]
  NOT: namespace_membersWhereInput
  user_id: StringFilter
  namespace_id: StringFilter
  membership: EnummembershipFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  namespace: NamespaceRelationFilter
  users: UsersRelationFilter
}

input Namespace_membersListRelationFilter {
  every: namespace_membersWhereInput
  some: namespace_membersWhereInput
  none: namespace_membersWhereInput
}

input usersWhereInput {
  AND: usersWhereInput
  OR: [usersWhereInput]
  NOT: usersWhereInput
  id: StringFilter
  name: StringNullableFilter
  email: StringFilter
  role: Enumuser_roleFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  access_token: Access_tokenListRelationFilter
  namespace_members: Namespace_membersListRelationFilter
}

input UsersRelationFilter {
  is: usersWhereInput
  isNot: usersWhereInput
}

input access_tokenWhereInput {
  AND: access_tokenWhereInput
  OR: [access_tokenWhereInput]
  NOT: access_tokenWhereInput
  id: StringFilter
  token: StringFilter
  user_id: StringFilter
  name: StringFilter
  created_at: DateTimeFilter
  users: UsersRelationFilter
}

enum SortOrder {
  asc
  desc
}

input access_tokenOrderByInput {
  id: SortOrder
  token: SortOrder
  user_id: SortOrder
  name: SortOrder
  created_at: SortOrder
}

input access_tokenWhereUniqueInput {
  id: String
  token: String
}

enum Access_tokenScalarFieldEnum {
  id
  token
  user_id
  name
  created_at
}

input namespace_membersOrderByInput {
  user_id: SortOrder
  namespace_id: SortOrder
  membership: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input namespace_membersUser_idNamespace_idCompoundUniqueInput {
  user_id: String!
  namespace_id: String!
}

input namespace_membersWhereUniqueInput {
  user_id_namespace_id: namespace_membersUser_idNamespace_idCompoundUniqueInput
}

enum Namespace_membersScalarFieldEnum {
  user_id
  namespace_id
  membership
  created_at
  updated_at
}

input namespaceOrderByInput {
  id: SortOrder
  name: SortOrder
  price_plan_id: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input namespaceWhereUniqueInput {
  id: String
  name: String
}

enum NamespaceScalarFieldEnum {
  id
  name
  price_plan_id
  created_at
  updated_at
}

type price_plan {
  id: Int!
  name: String!
  quota_daily_requests: Int!
  quota_environments: Int!
  quota_members: Int!
  quota_apis: Int!
  allow_secondary_environments: Boolean!
  namespace(where: namespaceWhereInput, orderBy: [namespaceOrderByInput], cursor: namespaceWhereUniqueInput, take: Int, skip: Int, distinct: [NamespaceScalarFieldEnum]): [namespace]
}

input apiOrderByInput {
  id: SortOrder
  namespace_id: SortOrder
  name: SortOrder
  markdown_description: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input apiApi_namespace_id_name_keyCompoundUniqueInput {
  namespace_id: String!
  name: String!
}

input apiWhereUniqueInput {
  id: String
  api_namespace_id_name_key: apiApi_namespace_id_name_keyCompoundUniqueInput
}

enum ApiScalarFieldEnum {
  id
  namespace_id
  name
  markdown_description
  created_at
  updated_at
}

input deploymentOrderByInput {
  id: SortOrder
  api_id: SortOrder
  name: SortOrder
  config: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input deploymentDeployment_api_id_name_keyCompoundUniqueInput {
  api_id: String!
  name: String!
}

input deploymentWhereUniqueInput {
  id: String
  deployment_api_id_name_key: deploymentDeployment_api_id_name_keyCompoundUniqueInput
}

enum DeploymentScalarFieldEnum {
  id
  api_id
  name
  config
  created_at
  updated_at
}

input deployment_environmentOrderByInput {
  deployment_id: SortOrder
  environment_id: SortOrder
}

input deployment_environmentDeployment_idEnvironment_idCompoundUniqueInput {
  deployment_id: String!
  environment_id: String!
}

input deployment_environmentWhereUniqueInput {
  deployment_id_environment_id: deployment_environmentDeployment_idEnvironment_idCompoundUniqueInput
}

enum Deployment_environmentScalarFieldEnum {
  deployment_id
  environment_id
}

input environment_edgesOrderByInput {
  environment_id: SortOrder
  edge_id: SortOrder
}

input environment_edgesEnvironment_idEdge_idCompoundUniqueInput {
  environment_id: String!
  edge_id: String!
}

input environment_edgesWhereUniqueInput {
  environment_id_edge_id: environment_edgesEnvironment_idEdge_idCompoundUniqueInput
}

enum Environment_edgesScalarFieldEnum {
  environment_id
  edge_id
}

type edge {
  id: String!
  name: String!
  location: String!
  created_at: DateTime!
  updated_at: DateTime
  environment_edges(where: environment_edgesWhereInput, orderBy: [environment_edgesOrderByInput], cursor: environment_edgesWhereUniqueInput, take: Int, skip: Int, distinct: [Environment_edgesScalarFieldEnum]): [environment_edges]
}

type environment_edges {
  environment_id: String!
  edge_id: String!
  edge: edge!
  environment: environment!
}

input nodepool_environmentOrderByInput {
  nodepool_id: SortOrder
  environment_id: SortOrder
}

input nodepool_environmentNodepool_idEnvironment_idCompoundUniqueInput {
  nodepool_id: String!
  environment_id: String!
}

input nodepool_environmentWhereUniqueInput {
  nodepool_id_environment_id: nodepool_environmentNodepool_idEnvironment_idCompoundUniqueInput
}

enum Nodepool_environmentScalarFieldEnum {
  nodepool_id
  environment_id
}

input nodepoolOrderByInput {
  id: SortOrder
  wundernode_id: SortOrder
  shared: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input nodepoolWhereUniqueInput {
  id: String
}

enum NodepoolScalarFieldEnum {
  id
  wundernode_id
  shared
  created_at
  updated_at
}

type wundernode {
  id: String!
  etag: String!
  config: Json!
  ipv4: String
  ipv6: String
  created_at: DateTime!
  updated_at: DateTime
  nodepool(where: nodepoolWhereInput, orderBy: [nodepoolOrderByInput], cursor: nodepoolWhereUniqueInput, take: Int, skip: Int, distinct: [NodepoolScalarFieldEnum]): [nodepool]
}

type nodepool {
  id: String!
  wundernode_id: String!
  shared: Boolean!
  created_at: DateTime!
  updated_at: DateTime
  wundernode: wundernode!
  nodepool_environment(where: nodepool_environmentWhereInput, orderBy: [nodepool_environmentOrderByInput], cursor: nodepool_environmentWhereUniqueInput, take: Int, skip: Int, distinct: [Nodepool_environmentScalarFieldEnum]): [nodepool_environment]
}

type nodepool_environment {
  nodepool_id: String!
  environment_id: String!
  environment: environment!
  nodepool: nodepool!
}

type environment {
  id: String!
  name: String!
  namespace_id: String!
  primary_hostname: String!
  hostnames: [String]
  primary: Boolean!
  namespace: namespace!
  deployment_environment(where: deployment_environmentWhereInput, orderBy: [deployment_environmentOrderByInput], cursor: deployment_environmentWhereUniqueInput, take: Int, skip: Int, distinct: [Deployment_environmentScalarFieldEnum]): [deployment_environment]
  environment_edges(where: environment_edgesWhereInput, orderBy: [environment_edgesOrderByInput], cursor: environment_edgesWhereUniqueInput, take: Int, skip: Int, distinct: [Environment_edgesScalarFieldEnum]): [environment_edges]
  nodepool_environment(where: nodepool_environmentWhereInput, orderBy: [nodepool_environmentOrderByInput], cursor: nodepool_environmentWhereUniqueInput, take: Int, skip: Int, distinct: [Nodepool_environmentScalarFieldEnum]): [nodepool_environment]
}

type deployment_environment {
  deployment_id: String!
  environment_id: String!
  deployment: deployment!
  environment: environment!
}

type deployment {
  id: String!
  api_id: String!
  name: String!
  config: Json!
  created_at: DateTime!
  updated_at: DateTime
  api: api!
  deployment_environment(where: deployment_environmentWhereInput, orderBy: [deployment_environmentOrderByInput], cursor: deployment_environmentWhereUniqueInput, take: Int, skip: Int, distinct: [Deployment_environmentScalarFieldEnum]): [deployment_environment]
}

type api {
  id: String!
  namespace_id: String!
  name: String!
  markdown_description: String!
  created_at: DateTime!
  updated_at: DateTime
  namespace: namespace!
  deployment(where: deploymentWhereInput, orderBy: [deploymentOrderByInput], cursor: deploymentWhereUniqueInput, take: Int, skip: Int, distinct: [DeploymentScalarFieldEnum]): [deployment]
}

input environmentOrderByInput {
  id: SortOrder
  name: SortOrder
  namespace_id: SortOrder
  primary_hostname: SortOrder
  hostnames: SortOrder
  primary: SortOrder
}

input environmentEnvironment_namespace_id_name_keyCompoundUniqueInput {
  namespace_id: String!
  name: String!
}

input environmentWhereUniqueInput {
  id: String
  environment_namespace_id_name_key: environmentEnvironment_namespace_id_name_keyCompoundUniqueInput
}

enum EnvironmentScalarFieldEnum {
  id
  name
  namespace_id
  primary_hostname
  hostnames
  primary
}

type namespace {
  id: String!
  name: String!
  price_plan_id: Int!
  created_at: DateTime!
  updated_at: DateTime
  price_plan: price_plan!
  api(where: apiWhereInput, orderBy: [apiOrderByInput], cursor: apiWhereUniqueInput, take: Int, skip: Int, distinct: [ApiScalarFieldEnum]): [api]
  environment(where: environmentWhereInput, orderBy: [environmentOrderByInput], cursor: environmentWhereUniqueInput, take: Int, skip: Int, distinct: [EnvironmentScalarFieldEnum]): [environment]
  namespace_members(where: namespace_membersWhereInput, orderBy: [namespace_membersOrderByInput], cursor: namespace_membersWhereUniqueInput, take: Int, skip: Int, distinct: [Namespace_membersScalarFieldEnum]): [namespace_members]
}

type namespace_members {
  user_id: String!
  namespace_id: String!
  membership: membership!
  created_at: DateTime!
  updated_at: DateTime
  namespace: namespace!
  users: users!
}

type users {
  id: String!
  name: String
  email: String!
  role: user_role!
  created_at: DateTime!
  updated_at: DateTime
  access_token(where: access_tokenWhereInput, orderBy: [access_tokenOrderByInput], cursor: access_tokenWhereUniqueInput, take: Int, skip: Int, distinct: [Access_tokenScalarFieldEnum]): [access_token]
  namespace_members(where: namespace_membersWhereInput, orderBy: [namespace_membersOrderByInput], cursor: namespace_membersWhereUniqueInput, take: Int, skip: Int, distinct: [Namespace_membersScalarFieldEnum]): [namespace_members]
}

type access_token {
  id: String!
  token: String!
  user_id: String!
  name: String!
  created_at: DateTime!
  users: users!
}

type Access_tokenCountAggregateOutputType {
  id: Int!
  token: Int!
  user_id: Int!
  name: Int!
  created_at: Int!
}

type Access_tokenMinAggregateOutputType {
  id: String
  token: String
  user_id: String
  name: String
  created_at: DateTime
}

type Access_tokenMaxAggregateOutputType {
  id: String
  token: String
  user_id: String
  name: String
  created_at: DateTime
}

type AggregateAccess_token {
  count: Access_tokenCountAggregateOutputType
  min: Access_tokenMinAggregateOutputType
  max: Access_tokenMaxAggregateOutputType
}

input NestedStringWithAggregatesFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  not: NestedStringWithAggregatesFilter
  count: NestedIntFilter
  min: NestedStringFilter
  max: NestedStringFilter
}

input StringWithAggregatesFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  mode: QueryMode
  not: NestedStringWithAggregatesFilter
  count: NestedIntFilter
  min: NestedStringFilter
  max: NestedStringFilter
}

input NestedDateTimeWithAggregatesFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: NestedDateTimeWithAggregatesFilter
  count: NestedIntFilter
  min: NestedDateTimeFilter
  max: NestedDateTimeFilter
}

input DateTimeWithAggregatesFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: NestedDateTimeWithAggregatesFilter
  count: NestedIntFilter
  min: NestedDateTimeFilter
  max: NestedDateTimeFilter
}

input access_tokenScalarWhereWithAggregatesInput {
  AND: access_tokenScalarWhereWithAggregatesInput
  OR: [access_tokenScalarWhereWithAggregatesInput]
  NOT: access_tokenScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  token: StringWithAggregatesFilter
  user_id: StringWithAggregatesFilter
  name: StringWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
}

type Access_tokenGroupByOutputType {
  id: String!
  token: String!
  user_id: String!
  name: String!
  created_at: DateTime!
  count: Access_tokenCountAggregateOutputType
  min: Access_tokenMinAggregateOutputType
  max: Access_tokenMaxAggregateOutputType
}

input admin_configWhereInput {
  AND: admin_configWhereInput
  OR: [admin_configWhereInput]
  NOT: admin_configWhereInput
  id: StringFilter
  wundernode_image_tag: StringFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
}

input admin_configOrderByInput {
  id: SortOrder
  wundernode_image_tag: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input admin_configWhereUniqueInput {
  id: String
}

enum Admin_configScalarFieldEnum {
  id
  wundernode_image_tag
  created_at
  updated_at
}

type admin_config {
  id: String!
  wundernode_image_tag: String!
  created_at: DateTime!
  updated_at: DateTime
}

type Admin_configCountAggregateOutputType {
  id: Int!
  wundernode_image_tag: Int!
  created_at: Int!
  updated_at: Int!
}

type Admin_configMinAggregateOutputType {
  id: String
  wundernode_image_tag: String
  created_at: DateTime
  updated_at: DateTime
}

type Admin_configMaxAggregateOutputType {
  id: String
  wundernode_image_tag: String
  created_at: DateTime
  updated_at: DateTime
}

type AggregateAdmin_config {
  count: Admin_configCountAggregateOutputType
  min: Admin_configMinAggregateOutputType
  max: Admin_configMaxAggregateOutputType
}

input NestedIntNullableFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: NestedIntNullableFilter
}

input NestedDateTimeNullableWithAggregatesFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: NestedDateTimeNullableWithAggregatesFilter
  count: NestedIntNullableFilter
  min: NestedDateTimeNullableFilter
  max: NestedDateTimeNullableFilter
}

input DateTimeNullableWithAggregatesFilter {
  equals: DateTime
  in: [DateTime]
  notIn: [DateTime]
  lt: DateTime
  lte: DateTime
  gt: DateTime
  gte: DateTime
  not: NestedDateTimeNullableWithAggregatesFilter
  count: NestedIntNullableFilter
  min: NestedDateTimeNullableFilter
  max: NestedDateTimeNullableFilter
}

input admin_configScalarWhereWithAggregatesInput {
  AND: admin_configScalarWhereWithAggregatesInput
  OR: [admin_configScalarWhereWithAggregatesInput]
  NOT: admin_configScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  wundernode_image_tag: StringWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type Admin_configGroupByOutputType {
  id: String!
  wundernode_image_tag: String!
  created_at: DateTime!
  updated_at: DateTime
  count: Admin_configCountAggregateOutputType
  min: Admin_configMinAggregateOutputType
  max: Admin_configMaxAggregateOutputType
}

type ApiCountAggregateOutputType {
  id: Int!
  namespace_id: Int!
  name: Int!
  markdown_description: Int!
  created_at: Int!
  updated_at: Int!
}

type ApiMinAggregateOutputType {
  id: String
  namespace_id: String
  name: String
  markdown_description: String
  created_at: DateTime
  updated_at: DateTime
}

type ApiMaxAggregateOutputType {
  id: String
  namespace_id: String
  name: String
  markdown_description: String
  created_at: DateTime
  updated_at: DateTime
}

type AggregateApi {
  count: ApiCountAggregateOutputType
  min: ApiMinAggregateOutputType
  max: ApiMaxAggregateOutputType
}

input apiScalarWhereWithAggregatesInput {
  AND: apiScalarWhereWithAggregatesInput
  OR: [apiScalarWhereWithAggregatesInput]
  NOT: apiScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  namespace_id: StringWithAggregatesFilter
  name: StringWithAggregatesFilter
  markdown_description: StringWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type ApiGroupByOutputType {
  id: String!
  namespace_id: String!
  name: String!
  markdown_description: String!
  created_at: DateTime!
  updated_at: DateTime
  count: ApiCountAggregateOutputType
  min: ApiMinAggregateOutputType
  max: ApiMaxAggregateOutputType
}

type DeploymentCountAggregateOutputType {
  id: Int!
  api_id: Int!
  name: Int!
  config: Int!
  created_at: Int!
  updated_at: Int!
}

type DeploymentMinAggregateOutputType {
  id: String
  api_id: String
  name: String
  created_at: DateTime
  updated_at: DateTime
}

type DeploymentMaxAggregateOutputType {
  id: String
  api_id: String
  name: String
  created_at: DateTime
  updated_at: DateTime
}

type AggregateDeployment {
  count: DeploymentCountAggregateOutputType
  min: DeploymentMinAggregateOutputType
  max: DeploymentMaxAggregateOutputType
}

input NestedJsonFilter {
  equals: DateTime
  not: DateTime
}

input JsonWithAggregatesFilter {
  equals: DateTime
  not: DateTime
  count: NestedIntFilter
  min: NestedJsonFilter
  max: NestedJsonFilter
}

input deploymentScalarWhereWithAggregatesInput {
  AND: deploymentScalarWhereWithAggregatesInput
  OR: [deploymentScalarWhereWithAggregatesInput]
  NOT: deploymentScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  api_id: StringWithAggregatesFilter
  name: StringWithAggregatesFilter
  config: JsonWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type DeploymentGroupByOutputType {
  id: String!
  api_id: String!
  name: String!
  config: Json!
  created_at: DateTime!
  updated_at: DateTime
  count: DeploymentCountAggregateOutputType
  min: DeploymentMinAggregateOutputType
  max: DeploymentMaxAggregateOutputType
}

type Deployment_environmentCountAggregateOutputType {
  deployment_id: Int!
  environment_id: Int!
}

type Deployment_environmentMinAggregateOutputType {
  deployment_id: String
  environment_id: String
}

type Deployment_environmentMaxAggregateOutputType {
  deployment_id: String
  environment_id: String
}

type AggregateDeployment_environment {
  count: Deployment_environmentCountAggregateOutputType
  min: Deployment_environmentMinAggregateOutputType
  max: Deployment_environmentMaxAggregateOutputType
}

input deployment_environmentScalarWhereWithAggregatesInput {
  AND: deployment_environmentScalarWhereWithAggregatesInput
  OR: [deployment_environmentScalarWhereWithAggregatesInput]
  NOT: deployment_environmentScalarWhereWithAggregatesInput
  deployment_id: StringWithAggregatesFilter
  environment_id: StringWithAggregatesFilter
}

type Deployment_environmentGroupByOutputType {
  deployment_id: String!
  environment_id: String!
  count: Deployment_environmentCountAggregateOutputType
  min: Deployment_environmentMinAggregateOutputType
  max: Deployment_environmentMaxAggregateOutputType
}

input edgeOrderByInput {
  id: SortOrder
  name: SortOrder
  location: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input edgeWhereUniqueInput {
  id: String
  name: String
}

enum EdgeScalarFieldEnum {
  id
  name
  location
  created_at
  updated_at
}

type EdgeCountAggregateOutputType {
  id: Int!
  name: Int!
  location: Int!
  created_at: Int!
  updated_at: Int!
}

type EdgeMinAggregateOutputType {
  id: String
  name: String
  location: String
  created_at: DateTime
  updated_at: DateTime
}

type EdgeMaxAggregateOutputType {
  id: String
  name: String
  location: String
  created_at: DateTime
  updated_at: DateTime
}

type AggregateEdge {
  count: EdgeCountAggregateOutputType
  min: EdgeMinAggregateOutputType
  max: EdgeMaxAggregateOutputType
}

input edgeScalarWhereWithAggregatesInput {
  AND: edgeScalarWhereWithAggregatesInput
  OR: [edgeScalarWhereWithAggregatesInput]
  NOT: edgeScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  name: StringWithAggregatesFilter
  location: StringWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type EdgeGroupByOutputType {
  id: String!
  name: String!
  location: String!
  created_at: DateTime!
  updated_at: DateTime
  count: EdgeCountAggregateOutputType
  min: EdgeMinAggregateOutputType
  max: EdgeMaxAggregateOutputType
}

type EnvironmentCountAggregateOutputType {
  id: Int!
  name: Int!
  namespace_id: Int!
  primary_hostname: Int!
  hostnames: Int!
  primary: Int!
}

type EnvironmentMinAggregateOutputType {
  id: String
  name: String
  namespace_id: String
  primary_hostname: String
  primary: Boolean
}

type EnvironmentMaxAggregateOutputType {
  id: String
  name: String
  namespace_id: String
  primary_hostname: String
  primary: Boolean
}

type AggregateEnvironment {
  count: EnvironmentCountAggregateOutputType
  min: EnvironmentMinAggregateOutputType
  max: EnvironmentMaxAggregateOutputType
}

input NestedBoolWithAggregatesFilter {
  equals: Boolean
  not: NestedBoolWithAggregatesFilter
  count: NestedIntFilter
  min: NestedBoolFilter
  max: NestedBoolFilter
}

input BoolWithAggregatesFilter {
  equals: Boolean
  not: NestedBoolWithAggregatesFilter
  count: NestedIntFilter
  min: NestedBoolFilter
  max: NestedBoolFilter
}

input environmentScalarWhereWithAggregatesInput {
  AND: environmentScalarWhereWithAggregatesInput
  OR: [environmentScalarWhereWithAggregatesInput]
  NOT: environmentScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  name: StringWithAggregatesFilter
  namespace_id: StringWithAggregatesFilter
  primary_hostname: StringWithAggregatesFilter
  hostnames: StringNullableListFilter
  primary: BoolWithAggregatesFilter
}

type EnvironmentGroupByOutputType {
  id: String!
  name: String!
  namespace_id: String!
  primary_hostname: String!
  hostnames: [String]
  primary: Boolean!
  count: EnvironmentCountAggregateOutputType
  min: EnvironmentMinAggregateOutputType
  max: EnvironmentMaxAggregateOutputType
}

type Environment_edgesCountAggregateOutputType {
  environment_id: Int!
  edge_id: Int!
}

type Environment_edgesMinAggregateOutputType {
  environment_id: String
  edge_id: String
}

type Environment_edgesMaxAggregateOutputType {
  environment_id: String
  edge_id: String
}

type AggregateEnvironment_edges {
  count: Environment_edgesCountAggregateOutputType
  min: Environment_edgesMinAggregateOutputType
  max: Environment_edgesMaxAggregateOutputType
}

input environment_edgesScalarWhereWithAggregatesInput {
  AND: environment_edgesScalarWhereWithAggregatesInput
  OR: [environment_edgesScalarWhereWithAggregatesInput]
  NOT: environment_edgesScalarWhereWithAggregatesInput
  environment_id: StringWithAggregatesFilter
  edge_id: StringWithAggregatesFilter
}

type Environment_edgesGroupByOutputType {
  environment_id: String!
  edge_id: String!
  count: Environment_edgesCountAggregateOutputType
  min: Environment_edgesMinAggregateOutputType
  max: Environment_edgesMaxAggregateOutputType
}

input JsonNullableFilter {
  equals: DateTime
  not: DateTime
}

input Letsencrypt_certificateListRelationFilter {
  every: letsencrypt_certificateWhereInput
  some: letsencrypt_certificateWhereInput
  none: letsencrypt_certificateWhereInput
}

input letsencrypt_userWhereInput {
  AND: letsencrypt_userWhereInput
  OR: [letsencrypt_userWhereInput]
  NOT: letsencrypt_userWhereInput
  zone: StringFilter
  email: StringFilter
  dns_provider_name: StringFilter
  dns_provider_token: StringFilter
  private_key: StringNullableFilter
  registration_resource: JsonNullableFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  letsencrypt_certificate: Letsencrypt_certificateListRelationFilter
}

input Letsencrypt_userRelationFilter {
  is: letsencrypt_userWhereInput
  isNot: letsencrypt_userWhereInput
}

input letsencrypt_certificateWhereInput {
  AND: letsencrypt_certificateWhereInput
  OR: [letsencrypt_certificateWhereInput]
  NOT: letsencrypt_certificateWhereInput
  common_name: StringFilter
  zone: StringFilter
  additional_domains: StringNullableListFilter
  issued: DateTimeNullableFilter
  renewal: DateTimeNullableFilter
  certificate: StringNullableFilter
  private_key: StringNullableFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
  letsencrypt_user: Letsencrypt_userRelationFilter
}

input letsencrypt_certificateOrderByInput {
  common_name: SortOrder
  zone: SortOrder
  additional_domains: SortOrder
  issued: SortOrder
  renewal: SortOrder
  certificate: SortOrder
  private_key: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input letsencrypt_certificateWhereUniqueInput {
  common_name: String
}

enum Letsencrypt_certificateScalarFieldEnum {
  common_name
  zone
  additional_domains
  issued
  renewal
  certificate
  private_key
  created_at
  updated_at
}

type letsencrypt_user {
  zone: String!
  email: String!
  dns_provider_name: String!
  dns_provider_token: String!
  private_key: String
  registration_resource: Json
  created_at: DateTime!
  updated_at: DateTime
  letsencrypt_certificate(where: letsencrypt_certificateWhereInput, orderBy: [letsencrypt_certificateOrderByInput], cursor: letsencrypt_certificateWhereUniqueInput, take: Int, skip: Int, distinct: [Letsencrypt_certificateScalarFieldEnum]): [letsencrypt_certificate]
}

type letsencrypt_certificate {
  common_name: String!
  zone: String!
  additional_domains: [String]
  issued: DateTime
  renewal: DateTime
  certificate: String
  private_key: String
  created_at: DateTime!
  updated_at: DateTime
  letsencrypt_user: letsencrypt_user!
}

type Letsencrypt_certificateCountAggregateOutputType {
  common_name: Int!
  zone: Int!
  additional_domains: Int!
  issued: Int!
  renewal: Int!
  certificate: Int!
  private_key: Int!
  created_at: Int!
  updated_at: Int!
}

type Letsencrypt_certificateMinAggregateOutputType {
  common_name: String
  zone: String
  issued: DateTime
  renewal: DateTime
  certificate: String
  private_key: String
  created_at: DateTime
  updated_at: DateTime
}

type Letsencrypt_certificateMaxAggregateOutputType {
  common_name: String
  zone: String
  issued: DateTime
  renewal: DateTime
  certificate: String
  private_key: String
  created_at: DateTime
  updated_at: DateTime
}

type AggregateLetsencrypt_certificate {
  count: Letsencrypt_certificateCountAggregateOutputType
  min: Letsencrypt_certificateMinAggregateOutputType
  max: Letsencrypt_certificateMaxAggregateOutputType
}

input NestedStringNullableWithAggregatesFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  not: NestedStringNullableWithAggregatesFilter
  count: NestedIntNullableFilter
  min: NestedStringNullableFilter
  max: NestedStringNullableFilter
}

input StringNullableWithAggregatesFilter {
  equals: String
  in: [String]
  notIn: [String]
  lt: String
  lte: String
  gt: String
  gte: String
  contains: String
  startsWith: String
  endsWith: String
  mode: QueryMode
  not: NestedStringNullableWithAggregatesFilter
  count: NestedIntNullableFilter
  min: NestedStringNullableFilter
  max: NestedStringNullableFilter
}

input letsencrypt_certificateScalarWhereWithAggregatesInput {
  AND: letsencrypt_certificateScalarWhereWithAggregatesInput
  OR: [letsencrypt_certificateScalarWhereWithAggregatesInput]
  NOT: letsencrypt_certificateScalarWhereWithAggregatesInput
  common_name: StringWithAggregatesFilter
  zone: StringWithAggregatesFilter
  additional_domains: StringNullableListFilter
  issued: DateTimeNullableWithAggregatesFilter
  renewal: DateTimeNullableWithAggregatesFilter
  certificate: StringNullableWithAggregatesFilter
  private_key: StringNullableWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type Letsencrypt_certificateGroupByOutputType {
  common_name: String!
  zone: String!
  additional_domains: [String]
  issued: DateTime
  renewal: DateTime
  certificate: String
  private_key: String
  created_at: DateTime!
  updated_at: DateTime
  count: Letsencrypt_certificateCountAggregateOutputType
  min: Letsencrypt_certificateMinAggregateOutputType
  max: Letsencrypt_certificateMaxAggregateOutputType
}

input letsencrypt_userOrderByInput {
  zone: SortOrder
  email: SortOrder
  dns_provider_name: SortOrder
  dns_provider_token: SortOrder
  private_key: SortOrder
  registration_resource: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input letsencrypt_userWhereUniqueInput {
  zone: String
}

enum Letsencrypt_userScalarFieldEnum {
  zone
  email
  dns_provider_name
  dns_provider_token
  private_key
  registration_resource
  created_at
  updated_at
}

type Letsencrypt_userCountAggregateOutputType {
  zone: Int!
  email: Int!
  dns_provider_name: Int!
  dns_provider_token: Int!
  private_key: Int!
  registration_resource: Int!
  created_at: Int!
  updated_at: Int!
}

type Letsencrypt_userMinAggregateOutputType {
  zone: String
  email: String
  dns_provider_name: String
  dns_provider_token: String
  private_key: String
  created_at: DateTime
  updated_at: DateTime
}

type Letsencrypt_userMaxAggregateOutputType {
  zone: String
  email: String
  dns_provider_name: String
  dns_provider_token: String
  private_key: String
  created_at: DateTime
  updated_at: DateTime
}

type AggregateLetsencrypt_user {
  count: Letsencrypt_userCountAggregateOutputType
  min: Letsencrypt_userMinAggregateOutputType
  max: Letsencrypt_userMaxAggregateOutputType
}

input NestedJsonNullableFilter {
  equals: DateTime
  not: DateTime
}

input JsonNullableWithAggregatesFilter {
  equals: DateTime
  not: DateTime
  count: NestedIntNullableFilter
  min: NestedJsonNullableFilter
  max: NestedJsonNullableFilter
}

input letsencrypt_userScalarWhereWithAggregatesInput {
  AND: letsencrypt_userScalarWhereWithAggregatesInput
  OR: [letsencrypt_userScalarWhereWithAggregatesInput]
  NOT: letsencrypt_userScalarWhereWithAggregatesInput
  zone: StringWithAggregatesFilter
  email: StringWithAggregatesFilter
  dns_provider_name: StringWithAggregatesFilter
  dns_provider_token: StringWithAggregatesFilter
  private_key: StringNullableWithAggregatesFilter
  registration_resource: JsonNullableWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type Letsencrypt_userGroupByOutputType {
  zone: String!
  email: String!
  dns_provider_name: String!
  dns_provider_token: String!
  private_key: String
  registration_resource: Json
  created_at: DateTime!
  updated_at: DateTime
  count: Letsencrypt_userCountAggregateOutputType
  min: Letsencrypt_userMinAggregateOutputType
  max: Letsencrypt_userMaxAggregateOutputType
}

input NestedBigIntNullableFilter {
  equals: BigInt
  in: [BigInt]
  notIn: [BigInt]
  lt: BigInt
  lte: BigInt
  gt: BigInt
  gte: BigInt
  not: NestedBigIntNullableFilter
}

input BigIntNullableFilter {
  equals: BigInt
  in: [BigInt]
  notIn: [BigInt]
  lt: BigInt
  lte: BigInt
  gt: BigInt
  gte: BigInt
  not: NestedBigIntNullableFilter
}

input locksWhereInput {
  AND: locksWhereInput
  OR: [locksWhereInput]
  NOT: locksWhereInput
  name: StringFilter
  record_version_number: BigIntNullableFilter
  data: StringNullableFilter
  owner: StringNullableFilter
}

input locksOrderByInput {
  name: SortOrder
  record_version_number: SortOrder
  data: SortOrder
  owner: SortOrder
}

input locksWhereUniqueInput {
  name: String
}

enum LocksScalarFieldEnum {
  name
  record_version_number
  data
  owner
}

type locks {
  name: String!
  record_version_number: BigInt
  data: String
  owner: String
}

type LocksCountAggregateOutputType {
  name: Int!
  record_version_number: Int!
  data: Int!
  owner: Int!
}

type LocksAvgAggregateOutputType {
  record_version_number: Float
}

type LocksSumAggregateOutputType {
  record_version_number: BigInt
}

type LocksMinAggregateOutputType {
  name: String
  record_version_number: BigInt
  data: String
  owner: String
}

type LocksMaxAggregateOutputType {
  name: String
  record_version_number: BigInt
  data: String
  owner: String
}

type AggregateLocks {
  count: LocksCountAggregateOutputType
  avg: LocksAvgAggregateOutputType
  sum: LocksSumAggregateOutputType
  min: LocksMinAggregateOutputType
  max: LocksMaxAggregateOutputType
}

input NestedFloatNullableFilter {
  equals: Float
  in: [Float]
  notIn: [Float]
  lt: Float
  lte: Float
  gt: Float
  gte: Float
  not: NestedFloatNullableFilter
}

input NestedBigIntNullableWithAggregatesFilter {
  equals: BigInt
  in: [BigInt]
  notIn: [BigInt]
  lt: BigInt
  lte: BigInt
  gt: BigInt
  gte: BigInt
  not: NestedBigIntNullableWithAggregatesFilter
  count: NestedIntNullableFilter
  avg: NestedFloatNullableFilter
  sum: NestedBigIntNullableFilter
  min: NestedBigIntNullableFilter
  max: NestedBigIntNullableFilter
}

input BigIntNullableWithAggregatesFilter {
  equals: BigInt
  in: [BigInt]
  notIn: [BigInt]
  lt: BigInt
  lte: BigInt
  gt: BigInt
  gte: BigInt
  not: NestedBigIntNullableWithAggregatesFilter
  count: NestedIntNullableFilter
  avg: NestedFloatNullableFilter
  sum: NestedBigIntNullableFilter
  min: NestedBigIntNullableFilter
  max: NestedBigIntNullableFilter
}

input locksScalarWhereWithAggregatesInput {
  AND: locksScalarWhereWithAggregatesInput
  OR: [locksScalarWhereWithAggregatesInput]
  NOT: locksScalarWhereWithAggregatesInput
  name: StringWithAggregatesFilter
  record_version_number: BigIntNullableWithAggregatesFilter
  data: StringNullableWithAggregatesFilter
  owner: StringNullableWithAggregatesFilter
}

type LocksGroupByOutputType {
  name: String!
  record_version_number: BigInt
  data: String
  owner: String
  count: LocksCountAggregateOutputType
  avg: LocksAvgAggregateOutputType
  sum: LocksSumAggregateOutputType
  min: LocksMinAggregateOutputType
  max: LocksMaxAggregateOutputType
}

type NamespaceCountAggregateOutputType {
  id: Int!
  name: Int!
  price_plan_id: Int!
  created_at: Int!
  updated_at: Int!
}

type NamespaceAvgAggregateOutputType {
  price_plan_id: Float
}

type NamespaceSumAggregateOutputType {
  price_plan_id: Int
}

type NamespaceMinAggregateOutputType {
  id: String
  name: String
  price_plan_id: Int
  created_at: DateTime
  updated_at: DateTime
}

type NamespaceMaxAggregateOutputType {
  id: String
  name: String
  price_plan_id: Int
  created_at: DateTime
  updated_at: DateTime
}

type AggregateNamespace {
  count: NamespaceCountAggregateOutputType
  avg: NamespaceAvgAggregateOutputType
  sum: NamespaceSumAggregateOutputType
  min: NamespaceMinAggregateOutputType
  max: NamespaceMaxAggregateOutputType
}

input NestedFloatFilter {
  equals: Float
  in: [Float]
  notIn: [Float]
  lt: Float
  lte: Float
  gt: Float
  gte: Float
  not: NestedFloatFilter
}

input NestedIntWithAggregatesFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: NestedIntWithAggregatesFilter
  count: NestedIntFilter
  avg: NestedFloatFilter
  sum: NestedIntFilter
  min: NestedIntFilter
  max: NestedIntFilter
}

input IntWithAggregatesFilter {
  equals: Int
  in: [Int]
  notIn: [Int]
  lt: Int
  lte: Int
  gt: Int
  gte: Int
  not: NestedIntWithAggregatesFilter
  count: NestedIntFilter
  avg: NestedFloatFilter
  sum: NestedIntFilter
  min: NestedIntFilter
  max: NestedIntFilter
}

input namespaceScalarWhereWithAggregatesInput {
  AND: namespaceScalarWhereWithAggregatesInput
  OR: [namespaceScalarWhereWithAggregatesInput]
  NOT: namespaceScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  name: StringWithAggregatesFilter
  price_plan_id: IntWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type NamespaceGroupByOutputType {
  id: String!
  name: String!
  price_plan_id: Int!
  created_at: DateTime!
  updated_at: DateTime
  count: NamespaceCountAggregateOutputType
  avg: NamespaceAvgAggregateOutputType
  sum: NamespaceSumAggregateOutputType
  min: NamespaceMinAggregateOutputType
  max: NamespaceMaxAggregateOutputType
}

type Namespace_membersCountAggregateOutputType {
  user_id: Int!
  namespace_id: Int!
  membership: Int!
  created_at: Int!
  updated_at: Int!
}

type Namespace_membersMinAggregateOutputType {
  user_id: String
  namespace_id: String
  membership: membership
  created_at: DateTime
  updated_at: DateTime
}

type Namespace_membersMaxAggregateOutputType {
  user_id: String
  namespace_id: String
  membership: membership
  created_at: DateTime
  updated_at: DateTime
}

type AggregateNamespace_members {
  count: Namespace_membersCountAggregateOutputType
  min: Namespace_membersMinAggregateOutputType
  max: Namespace_membersMaxAggregateOutputType
}

input NestedEnummembershipFilter {
  equals: membership
  in: [membership]
  notIn: [membership]
  not: membership
}

input EnummembershipWithAggregatesFilter {
  equals: membership
  in: [membership]
  notIn: [membership]
  not: membership
  count: NestedIntFilter
  min: NestedEnummembershipFilter
  max: NestedEnummembershipFilter
}

input namespace_membersScalarWhereWithAggregatesInput {
  AND: namespace_membersScalarWhereWithAggregatesInput
  OR: [namespace_membersScalarWhereWithAggregatesInput]
  NOT: namespace_membersScalarWhereWithAggregatesInput
  user_id: StringWithAggregatesFilter
  namespace_id: StringWithAggregatesFilter
  membership: EnummembershipWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type Namespace_membersGroupByOutputType {
  user_id: String!
  namespace_id: String!
  membership: membership!
  created_at: DateTime!
  updated_at: DateTime
  count: Namespace_membersCountAggregateOutputType
  min: Namespace_membersMinAggregateOutputType
  max: Namespace_membersMaxAggregateOutputType
}

type NodepoolCountAggregateOutputType {
  id: Int!
  wundernode_id: Int!
  shared: Int!
  created_at: Int!
  updated_at: Int!
}

type NodepoolMinAggregateOutputType {
  id: String
  wundernode_id: String
  shared: Boolean
  created_at: DateTime
  updated_at: DateTime
}

type NodepoolMaxAggregateOutputType {
  id: String
  wundernode_id: String
  shared: Boolean
  created_at: DateTime
  updated_at: DateTime
}

type AggregateNodepool {
  count: NodepoolCountAggregateOutputType
  min: NodepoolMinAggregateOutputType
  max: NodepoolMaxAggregateOutputType
}

input nodepoolScalarWhereWithAggregatesInput {
  AND: nodepoolScalarWhereWithAggregatesInput
  OR: [nodepoolScalarWhereWithAggregatesInput]
  NOT: nodepoolScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  wundernode_id: StringWithAggregatesFilter
  shared: BoolWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type NodepoolGroupByOutputType {
  id: String!
  wundernode_id: String!
  shared: Boolean!
  created_at: DateTime!
  updated_at: DateTime
  count: NodepoolCountAggregateOutputType
  min: NodepoolMinAggregateOutputType
  max: NodepoolMaxAggregateOutputType
}

type Nodepool_environmentCountAggregateOutputType {
  nodepool_id: Int!
  environment_id: Int!
}

type Nodepool_environmentMinAggregateOutputType {
  nodepool_id: String
  environment_id: String
}

type Nodepool_environmentMaxAggregateOutputType {
  nodepool_id: String
  environment_id: String
}

type AggregateNodepool_environment {
  count: Nodepool_environmentCountAggregateOutputType
  min: Nodepool_environmentMinAggregateOutputType
  max: Nodepool_environmentMaxAggregateOutputType
}

input nodepool_environmentScalarWhereWithAggregatesInput {
  AND: nodepool_environmentScalarWhereWithAggregatesInput
  OR: [nodepool_environmentScalarWhereWithAggregatesInput]
  NOT: nodepool_environmentScalarWhereWithAggregatesInput
  nodepool_id: StringWithAggregatesFilter
  environment_id: StringWithAggregatesFilter
}

type Nodepool_environmentGroupByOutputType {
  nodepool_id: String!
  environment_id: String!
  count: Nodepool_environmentCountAggregateOutputType
  min: Nodepool_environmentMinAggregateOutputType
  max: Nodepool_environmentMaxAggregateOutputType
}

input price_planOrderByInput {
  id: SortOrder
  name: SortOrder
  quota_daily_requests: SortOrder
  quota_environments: SortOrder
  quota_members: SortOrder
  quota_apis: SortOrder
  allow_secondary_environments: SortOrder
}

input price_planWhereUniqueInput {
  id: Int
}

enum Price_planScalarFieldEnum {
  id
  name
  quota_daily_requests
  quota_environments
  quota_members
  quota_apis
  allow_secondary_environments
}

type Price_planCountAggregateOutputType {
  id: Int!
  name: Int!
  quota_daily_requests: Int!
  quota_environments: Int!
  quota_members: Int!
  quota_apis: Int!
  allow_secondary_environments: Int!
}

type Price_planAvgAggregateOutputType {
  id: Float
  quota_daily_requests: Float
  quota_environments: Float
  quota_members: Float
  quota_apis: Float
}

type Price_planSumAggregateOutputType {
  id: Int
  quota_daily_requests: Int
  quota_environments: Int
  quota_members: Int
  quota_apis: Int
}

type Price_planMinAggregateOutputType {
  id: Int
  name: String
  quota_daily_requests: Int
  quota_environments: Int
  quota_members: Int
  quota_apis: Int
  allow_secondary_environments: Boolean
}

type Price_planMaxAggregateOutputType {
  id: Int
  name: String
  quota_daily_requests: Int
  quota_environments: Int
  quota_members: Int
  quota_apis: Int
  allow_secondary_environments: Boolean
}

type AggregatePrice_plan {
  count: Price_planCountAggregateOutputType
  avg: Price_planAvgAggregateOutputType
  sum: Price_planSumAggregateOutputType
  min: Price_planMinAggregateOutputType
  max: Price_planMaxAggregateOutputType
}

input price_planScalarWhereWithAggregatesInput {
  AND: price_planScalarWhereWithAggregatesInput
  OR: [price_planScalarWhereWithAggregatesInput]
  NOT: price_planScalarWhereWithAggregatesInput
  id: IntWithAggregatesFilter
  name: StringWithAggregatesFilter
  quota_daily_requests: IntWithAggregatesFilter
  quota_environments: IntWithAggregatesFilter
  quota_members: IntWithAggregatesFilter
  quota_apis: IntWithAggregatesFilter
  allow_secondary_environments: BoolWithAggregatesFilter
}

type Price_planGroupByOutputType {
  id: Int!
  name: String!
  quota_daily_requests: Int!
  quota_environments: Int!
  quota_members: Int!
  quota_apis: Int!
  allow_secondary_environments: Boolean!
  count: Price_planCountAggregateOutputType
  avg: Price_planAvgAggregateOutputType
  sum: Price_planSumAggregateOutputType
  min: Price_planMinAggregateOutputType
  max: Price_planMaxAggregateOutputType
}

input usersOrderByInput {
  id: SortOrder
  name: SortOrder
  email: SortOrder
  role: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input usersWhereUniqueInput {
  id: String
  email: String
}

enum UsersScalarFieldEnum {
  id
  name
  email
  role
  created_at
  updated_at
}

type UsersCountAggregateOutputType {
  id: Int!
  name: Int!
  email: Int!
  role: Int!
  created_at: Int!
  updated_at: Int!
}

type UsersMinAggregateOutputType {
  id: String
  name: String
  email: String
  role: user_role
  created_at: DateTime
  updated_at: DateTime
}

type UsersMaxAggregateOutputType {
  id: String
  name: String
  email: String
  role: user_role
  created_at: DateTime
  updated_at: DateTime
}

type AggregateUsers {
  count: UsersCountAggregateOutputType
  min: UsersMinAggregateOutputType
  max: UsersMaxAggregateOutputType
}

input NestedEnumuser_roleFilter {
  equals: user_role
  in: [user_role]
  notIn: [user_role]
  not: user_role
}

input Enumuser_roleWithAggregatesFilter {
  equals: user_role
  in: [user_role]
  notIn: [user_role]
  not: user_role
  count: NestedIntFilter
  min: NestedEnumuser_roleFilter
  max: NestedEnumuser_roleFilter
}

input usersScalarWhereWithAggregatesInput {
  AND: usersScalarWhereWithAggregatesInput
  OR: [usersScalarWhereWithAggregatesInput]
  NOT: usersScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  name: StringNullableWithAggregatesFilter
  email: StringWithAggregatesFilter
  role: Enumuser_roleWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type UsersGroupByOutputType {
  id: String!
  name: String
  email: String!
  role: user_role!
  created_at: DateTime!
  updated_at: DateTime
  count: UsersCountAggregateOutputType
  min: UsersMinAggregateOutputType
  max: UsersMaxAggregateOutputType
}

input wundernodeOrderByInput {
  id: SortOrder
  etag: SortOrder
  config: SortOrder
  ipv4: SortOrder
  ipv6: SortOrder
  created_at: SortOrder
  updated_at: SortOrder
}

input wundernodeWhereUniqueInput {
  id: String
}

enum WundernodeScalarFieldEnum {
  id
  etag
  config
  ipv4
  ipv6
  created_at
  updated_at
}

type WundernodeCountAggregateOutputType {
  id: Int!
  etag: Int!
  config: Int!
  ipv4: Int!
  ipv6: Int!
  created_at: Int!
  updated_at: Int!
}

type WundernodeMinAggregateOutputType {
  id: String
  etag: String
  ipv4: String
  ipv6: String
  created_at: DateTime
  updated_at: DateTime
}

type WundernodeMaxAggregateOutputType {
  id: String
  etag: String
  ipv4: String
  ipv6: String
  created_at: DateTime
  updated_at: DateTime
}

type AggregateWundernode {
  count: WundernodeCountAggregateOutputType
  min: WundernodeMinAggregateOutputType
  max: WundernodeMaxAggregateOutputType
}

input wundernodeScalarWhereWithAggregatesInput {
  AND: wundernodeScalarWhereWithAggregatesInput
  OR: [wundernodeScalarWhereWithAggregatesInput]
  NOT: wundernodeScalarWhereWithAggregatesInput
  id: StringWithAggregatesFilter
  etag: StringWithAggregatesFilter
  config: JsonWithAggregatesFilter
  ipv4: StringNullableWithAggregatesFilter
  ipv6: StringNullableWithAggregatesFilter
  created_at: DateTimeWithAggregatesFilter
  updated_at: DateTimeNullableWithAggregatesFilter
}

type WundernodeGroupByOutputType {
  id: String!
  etag: String!
  config: Json!
  ipv4: String
  ipv6: String
  created_at: DateTime!
  updated_at: DateTime
  count: WundernodeCountAggregateOutputType
  min: WundernodeMinAggregateOutputType
  max: WundernodeMaxAggregateOutputType
}

type Query {
  findFirstaccess_token(where: access_tokenWhereInput, orderBy: [access_tokenOrderByInput], cursor: access_tokenWhereUniqueInput, take: Int, skip: Int, distinct: [Access_tokenScalarFieldEnum]): access_token
  findManyaccess_token(where: access_tokenWhereInput, orderBy: [access_tokenOrderByInput], cursor: access_tokenWhereUniqueInput, take: Int, skip: Int, distinct: [Access_tokenScalarFieldEnum]): [access_token]!
  aggregateaccess_token(where: access_tokenWhereInput, orderBy: [access_tokenOrderByInput], cursor: access_tokenWhereUniqueInput, take: Int, skip: Int): AggregateAccess_token!
  groupByaccess_token(where: access_tokenWhereInput, orderBy: [access_tokenOrderByInput], by: [Access_tokenScalarFieldEnum]!, having: access_tokenScalarWhereWithAggregatesInput, take: Int, skip: Int): [Access_tokenGroupByOutputType]!
  findUniqueaccess_token(where: access_tokenWhereUniqueInput!): access_token
  findFirstadmin_config(where: admin_configWhereInput, orderBy: [admin_configOrderByInput], cursor: admin_configWhereUniqueInput, take: Int, skip: Int, distinct: [Admin_configScalarFieldEnum]): admin_config
  findManyadmin_config(where: admin_configWhereInput, orderBy: [admin_configOrderByInput], cursor: admin_configWhereUniqueInput, take: Int, skip: Int, distinct: [Admin_configScalarFieldEnum]): [admin_config]!
  aggregateadmin_config(where: admin_configWhereInput, orderBy: [admin_configOrderByInput], cursor: admin_configWhereUniqueInput, take: Int, skip: Int): AggregateAdmin_config!
  groupByadmin_config(where: admin_configWhereInput, orderBy: [admin_configOrderByInput], by: [Admin_configScalarFieldEnum]!, having: admin_configScalarWhereWithAggregatesInput, take: Int, skip: Int): [Admin_configGroupByOutputType]!
  findUniqueadmin_config(where: admin_configWhereUniqueInput!): admin_config
  findFirstapi(where: apiWhereInput, orderBy: [apiOrderByInput], cursor: apiWhereUniqueInput, take: Int, skip: Int, distinct: [ApiScalarFieldEnum]): api
  findManyapi(where: apiWhereInput, orderBy: [apiOrderByInput], cursor: apiWhereUniqueInput, take: Int, skip: Int, distinct: [ApiScalarFieldEnum]): [api]!
  aggregateapi(where: apiWhereInput, orderBy: [apiOrderByInput], cursor: apiWhereUniqueInput, take: Int, skip: Int): AggregateApi!
  groupByapi(where: apiWhereInput, orderBy: [apiOrderByInput], by: [ApiScalarFieldEnum]!, having: apiScalarWhereWithAggregatesInput, take: Int, skip: Int): [ApiGroupByOutputType]!
  findUniqueapi(where: apiWhereUniqueInput!): api
  findFirstdeployment(where: deploymentWhereInput, orderBy: [deploymentOrderByInput], cursor: deploymentWhereUniqueInput, take: Int, skip: Int, distinct: [DeploymentScalarFieldEnum]): deployment
  findManydeployment(where: deploymentWhereInput, orderBy: [deploymentOrderByInput], cursor: deploymentWhereUniqueInput, take: Int, skip: Int, distinct: [DeploymentScalarFieldEnum]): [deployment]!
  aggregatedeployment(where: deploymentWhereInput, orderBy: [deploymentOrderByInput], cursor: deploymentWhereUniqueInput, take: Int, skip: Int): AggregateDeployment!
  groupBydeployment(where: deploymentWhereInput, orderBy: [deploymentOrderByInput], by: [DeploymentScalarFieldEnum]!, having: deploymentScalarWhereWithAggregatesInput, take: Int, skip: Int): [DeploymentGroupByOutputType]!
  findUniquedeployment(where: deploymentWhereUniqueInput!): deployment
  findFirstdeployment_environment(where: deployment_environmentWhereInput, orderBy: [deployment_environmentOrderByInput], cursor: deployment_environmentWhereUniqueInput, take: Int, skip: Int, distinct: [Deployment_environmentScalarFieldEnum]): deployment_environment
  findManydeployment_environment(where: deployment_environmentWhereInput, orderBy: [deployment_environmentOrderByInput], cursor: deployment_environmentWhereUniqueInput, take: Int, skip: Int, distinct: [Deployment_environmentScalarFieldEnum]): [deployment_environment]!
  aggregatedeployment_environment(where: deployment_environmentWhereInput, orderBy: [deployment_environmentOrderByInput], cursor: deployment_environmentWhereUniqueInput, take: Int, skip: Int): AggregateDeployment_environment!
  groupBydeployment_environment(where: deployment_environmentWhereInput, orderBy: [deployment_environmentOrderByInput], by: [Deployment_environmentScalarFieldEnum]!, having: deployment_environmentScalarWhereWithAggregatesInput, take: Int, skip: Int): [Deployment_environmentGroupByOutputType]!
  findUniquedeployment_environment(where: deployment_environmentWhereUniqueInput!): deployment_environment
  findFirstedge(where: edgeWhereInput, orderBy: [edgeOrderByInput], cursor: edgeWhereUniqueInput, take: Int, skip: Int, distinct: [EdgeScalarFieldEnum]): edge
  findManyedge(where: edgeWhereInput, orderBy: [edgeOrderByInput], cursor: edgeWhereUniqueInput, take: Int, skip: Int, distinct: [EdgeScalarFieldEnum]): [edge]!
  aggregateedge(where: edgeWhereInput, orderBy: [edgeOrderByInput], cursor: edgeWhereUniqueInput, take: Int, skip: Int): AggregateEdge!
  groupByedge(where: edgeWhereInput, orderBy: [edgeOrderByInput], by: [EdgeScalarFieldEnum]!, having: edgeScalarWhereWithAggregatesInput, take: Int, skip: Int): [EdgeGroupByOutputType]!
  findUniqueedge(where: edgeWhereUniqueInput!): edge
  findFirstenvironment(where: environmentWhereInput, orderBy: [environmentOrderByInput], cursor: environmentWhereUniqueInput, take: Int, skip: Int, distinct: [EnvironmentScalarFieldEnum]): environment
  findManyenvironment(where: environmentWhereInput, orderBy: [environmentOrderByInput], cursor: environmentWhereUniqueInput, take: Int, skip: Int, distinct: [EnvironmentScalarFieldEnum]): [environment]!
  aggregateenvironment(where: environmentWhereInput, orderBy: [environmentOrderByInput], cursor: environmentWhereUniqueInput, take: Int, skip: Int): AggregateEnvironment!
  groupByenvironment(where: environmentWhereInput, orderBy: [environmentOrderByInput], by: [EnvironmentScalarFieldEnum]!, having: environmentScalarWhereWithAggregatesInput, take: Int, skip: Int): [EnvironmentGroupByOutputType]!
  findUniqueenvironment(where: environmentWhereUniqueInput!): environment
  findFirstenvironment_edges(where: environment_edgesWhereInput, orderBy: [environment_edgesOrderByInput], cursor: environment_edgesWhereUniqueInput, take: Int, skip: Int, distinct: [Environment_edgesScalarFieldEnum]): environment_edges
  findManyenvironment_edges(where: environment_edgesWhereInput, orderBy: [environment_edgesOrderByInput], cursor: environment_edgesWhereUniqueInput, take: Int, skip: Int, distinct: [Environment_edgesScalarFieldEnum]): [environment_edges]!
  aggregateenvironment_edges(where: environment_edgesWhereInput, orderBy: [environment_edgesOrderByInput], cursor: environment_edgesWhereUniqueInput, take: Int, skip: Int): AggregateEnvironment_edges!
  groupByenvironment_edges(where: environment_edgesWhereInput, orderBy: [environment_edgesOrderByInput], by: [Environment_edgesScalarFieldEnum]!, having: environment_edgesScalarWhereWithAggregatesInput, take: Int, skip: Int): [Environment_edgesGroupByOutputType]!
  findUniqueenvironment_edges(where: environment_edgesWhereUniqueInput!): environment_edges
  findFirstletsencrypt_certificate(where: letsencrypt_certificateWhereInput, orderBy: [letsencrypt_certificateOrderByInput], cursor: letsencrypt_certificateWhereUniqueInput, take: Int, skip: Int, distinct: [Letsencrypt_certificateScalarFieldEnum]): letsencrypt_certificate
  findManyletsencrypt_certificate(where: letsencrypt_certificateWhereInput, orderBy: [letsencrypt_certificateOrderByInput], cursor: letsencrypt_certificateWhereUniqueInput, take: Int, skip: Int, distinct: [Letsencrypt_certificateScalarFieldEnum]): [letsencrypt_certificate]!
  aggregateletsencrypt_certificate(where: letsencrypt_certificateWhereInput, orderBy: [letsencrypt_certificateOrderByInput], cursor: letsencrypt_certificateWhereUniqueInput, take: Int, skip: Int): AggregateLetsencrypt_certificate!
  groupByletsencrypt_certificate(where: letsencrypt_certificateWhereInput, orderBy: [letsencrypt_certificateOrderByInput], by: [Letsencrypt_certificateScalarFieldEnum]!, having: letsencrypt_certificateScalarWhereWithAggregatesInput, take: Int, skip: Int): [Letsencrypt_certificateGroupByOutputType]!
  findUniqueletsencrypt_certificate(where: letsencrypt_certificateWhereUniqueInput!): letsencrypt_certificate
  findFirstletsencrypt_user(where: letsencrypt_userWhereInput, orderBy: [letsencrypt_userOrderByInput], cursor: letsencrypt_userWhereUniqueInput, take: Int, skip: Int, distinct: [Letsencrypt_userScalarFieldEnum]): letsencrypt_user
  findManyletsencrypt_user(where: letsencrypt_userWhereInput, orderBy: [letsencrypt_userOrderByInput], cursor: letsencrypt_userWhereUniqueInput, take: Int, skip: Int, distinct: [Letsencrypt_userScalarFieldEnum]): [letsencrypt_user]!
  aggregateletsencrypt_user(where: letsencrypt_userWhereInput, orderBy: [letsencrypt_userOrderByInput], cursor: letsencrypt_userWhereUniqueInput, take: Int, skip: Int): AggregateLetsencrypt_user!
  groupByletsencrypt_user(where: letsencrypt_userWhereInput, orderBy: [letsencrypt_userOrderByInput], by: [Letsencrypt_userScalarFieldEnum]!, having: letsencrypt_userScalarWhereWithAggregatesInput, take: Int, skip: Int): [Letsencrypt_userGroupByOutputType]!
  findUniqueletsencrypt_user(where: letsencrypt_userWhereUniqueInput!): letsencrypt_user
  findFirstlocks(where: locksWhereInput, orderBy: [locksOrderByInput], cursor: locksWhereUniqueInput, take: Int, skip: Int, distinct: [LocksScalarFieldEnum]): locks
  findManylocks(where: locksWhereInput, orderBy: [locksOrderByInput], cursor: locksWhereUniqueInput, take: Int, skip: Int, distinct: [LocksScalarFieldEnum]): [locks]!
  aggregatelocks(where: locksWhereInput, orderBy: [locksOrderByInput], cursor: locksWhereUniqueInput, take: Int, skip: Int): AggregateLocks!
  groupBylocks(where: locksWhereInput, orderBy: [locksOrderByInput], by: [LocksScalarFieldEnum]!, having: locksScalarWhereWithAggregatesInput, take: Int, skip: Int): [LocksGroupByOutputType]!
  findUniquelocks(where: locksWhereUniqueInput!): locks
  findFirstnamespace(where: namespaceWhereInput, orderBy: [namespaceOrderByInput], cursor: namespaceWhereUniqueInput, take: Int, skip: Int, distinct: [NamespaceScalarFieldEnum]): namespace
  findManynamespace(where: namespaceWhereInput, orderBy: [namespaceOrderByInput], cursor: namespaceWhereUniqueInput, take: Int, skip: Int, distinct: [NamespaceScalarFieldEnum]): [namespace]!
  aggregatenamespace(where: namespaceWhereInput, orderBy: [namespaceOrderByInput], cursor: namespaceWhereUniqueInput, take: Int, skip: Int): AggregateNamespace!
  groupBynamespace(where: namespaceWhereInput, orderBy: [namespaceOrderByInput], by: [NamespaceScalarFieldEnum]!, having: namespaceScalarWhereWithAggregatesInput, take: Int, skip: Int): [NamespaceGroupByOutputType]!
  findUniquenamespace(where: namespaceWhereUniqueInput!): namespace
  findFirstnamespace_members(where: namespace_membersWhereInput, orderBy: [namespace_membersOrderByInput], cursor: namespace_membersWhereUniqueInput, take: Int, skip: Int, distinct: [Namespace_membersScalarFieldEnum]): namespace_members
  findManynamespace_members(where: namespace_membersWhereInput, orderBy: [namespace_membersOrderByInput], cursor: namespace_membersWhereUniqueInput, take: Int, skip: Int, distinct: [Namespace_membersScalarFieldEnum]): [namespace_members]!
  aggregatenamespace_members(where: namespace_membersWhereInput, orderBy: [namespace_membersOrderByInput], cursor: namespace_membersWhereUniqueInput, take: Int, skip: Int): AggregateNamespace_members!
  groupBynamespace_members(where: namespace_membersWhereInput, orderBy: [namespace_membersOrderByInput], by: [Namespace_membersScalarFieldEnum]!, having: namespace_membersScalarWhereWithAggregatesInput, take: Int, skip: Int): [Namespace_membersGroupByOutputType]!
  findUniquenamespace_members(where: namespace_membersWhereUniqueInput!): namespace_members
  findFirstnodepool(where: nodepoolWhereInput, orderBy: [nodepoolOrderByInput], cursor: nodepoolWhereUniqueInput, take: Int, skip: Int, distinct: [NodepoolScalarFieldEnum]): nodepool
  findManynodepool(where: nodepoolWhereInput, orderBy: [nodepoolOrderByInput], cursor: nodepoolWhereUniqueInput, take: Int, skip: Int, distinct: [NodepoolScalarFieldEnum]): [nodepool]!
  aggregatenodepool(where: nodepoolWhereInput, orderBy: [nodepoolOrderByInput], cursor: nodepoolWhereUniqueInput, take: Int, skip: Int): AggregateNodepool!
  groupBynodepool(where: nodepoolWhereInput, orderBy: [nodepoolOrderByInput], by: [NodepoolScalarFieldEnum]!, having: nodepoolScalarWhereWithAggregatesInput, take: Int, skip: Int): [NodepoolGroupByOutputType]!
  findUniquenodepool(where: nodepoolWhereUniqueInput!): nodepool
  findFirstnodepool_environment(where: nodepool_environmentWhereInput, orderBy: [nodepool_environmentOrderByInput], cursor: nodepool_environmentWhereUniqueInput, take: Int, skip: Int, distinct: [Nodepool_environmentScalarFieldEnum]): nodepool_environment
  findManynodepool_environment(where: nodepool_environmentWhereInput, orderBy: [nodepool_environmentOrderByInput], cursor: nodepool_environmentWhereUniqueInput, take: Int, skip: Int, distinct: [Nodepool_environmentScalarFieldEnum]): [nodepool_environment]!
  aggregatenodepool_environment(where: nodepool_environmentWhereInput, orderBy: [nodepool_environmentOrderByInput], cursor: nodepool_environmentWhereUniqueInput, take: Int, skip: Int): AggregateNodepool_environment!
  groupBynodepool_environment(where: nodepool_environmentWhereInput, orderBy: [nodepool_environmentOrderByInput], by: [Nodepool_environmentScalarFieldEnum]!, having: nodepool_environmentScalarWhereWithAggregatesInput, take: Int, skip: Int): [Nodepool_environmentGroupByOutputType]!
  findUniquenodepool_environment(where: nodepool_environmentWhereUniqueInput!): nodepool_environment
  findFirstprice_plan(where: price_planWhereInput, orderBy: [price_planOrderByInput], cursor: price_planWhereUniqueInput, take: Int, skip: Int, distinct: [Price_planScalarFieldEnum]): price_plan
  findManyprice_plan(where: price_planWhereInput, orderBy: [price_planOrderByInput], cursor: price_planWhereUniqueInput, take: Int, skip: Int, distinct: [Price_planScalarFieldEnum]): [price_plan]!
  aggregateprice_plan(where: price_planWhereInput, orderBy: [price_planOrderByInput], cursor: price_planWhereUniqueInput, take: Int, skip: Int): AggregatePrice_plan!
  groupByprice_plan(where: price_planWhereInput, orderBy: [price_planOrderByInput], by: [Price_planScalarFieldEnum]!, having: price_planScalarWhereWithAggregatesInput, take: Int, skip: Int): [Price_planGroupByOutputType]!
  findUniqueprice_plan(where: price_planWhereUniqueInput!): price_plan
  findFirstusers(where: usersWhereInput, orderBy: [usersOrderByInput], cursor: usersWhereUniqueInput, take: Int, skip: Int, distinct: [UsersScalarFieldEnum]): users
  findManyusers(where: usersWhereInput, orderBy: [usersOrderByInput], cursor: usersWhereUniqueInput, take: Int, skip: Int, distinct: [UsersScalarFieldEnum]): [users]!
  aggregateusers(where: usersWhereInput, orderBy: [usersOrderByInput], cursor: usersWhereUniqueInput, take: Int, skip: Int): AggregateUsers!
  groupByusers(where: usersWhereInput, orderBy: [usersOrderByInput], by: [UsersScalarFieldEnum]!, having: usersScalarWhereWithAggregatesInput, take: Int, skip: Int): [UsersGroupByOutputType]!
  findUniqueusers(where: usersWhereUniqueInput!): users
  findFirstwundernode(where: wundernodeWhereInput, orderBy: [wundernodeOrderByInput], cursor: wundernodeWhereUniqueInput, take: Int, skip: Int, distinct: [WundernodeScalarFieldEnum]): wundernode
  findManywundernode(where: wundernodeWhereInput, orderBy: [wundernodeOrderByInput], cursor: wundernodeWhereUniqueInput, take: Int, skip: Int, distinct: [WundernodeScalarFieldEnum]): [wundernode]!
  aggregatewundernode(where: wundernodeWhereInput, orderBy: [wundernodeOrderByInput], cursor: wundernodeWhereUniqueInput, take: Int, skip: Int): AggregateWundernode!
  groupBywundernode(where: wundernodeWhereInput, orderBy: [wundernodeOrderByInput], by: [WundernodeScalarFieldEnum]!, having: wundernodeScalarWhereWithAggregatesInput, take: Int, skip: Int): [WundernodeGroupByOutputType]!
  findUniquewundernode(where: wundernodeWhereUniqueInput!): wundernode
  posts: [Post]
  postComments(postID: String!): [Comment]
  users: [User]
  userPosts(userID: String!): [Post]
}

input price_planCreateWithoutNamespaceInput {
  name: String!
  quota_daily_requests: Int!
  quota_environments: Int!
  quota_members: Int
  quota_apis: Int
  allow_secondary_environments: Boolean
}

input price_planCreateOrConnectWithoutNamespaceInput {
  where: price_planWhereUniqueInput!
  create: price_planCreateWithoutNamespaceInput!
}

input price_planCreateNestedOneWithoutNamespaceInput {
  create: price_planCreateWithoutNamespaceInput
  connectOrCreate: price_planCreateOrConnectWithoutNamespaceInput
  connect: price_planWhereUniqueInput
}

input environmentCreatehostnamesInput {
  set: [String]!
}

input access_tokenCreateWithoutUsersInput {
  id: String
  token: String
  name: String!
  created_at: DateTime
}

input access_tokenCreateOrConnectWithoutUsersInput {
  where: access_tokenWhereUniqueInput!
  create: access_tokenCreateWithoutUsersInput!
}

input access_tokenCreateManyUsersInput {
  id: String
  token: String
  name: String!
  created_at: DateTime
}

input access_tokenCreateManyUsersInputEnvelope {
  data: [access_tokenCreateManyUsersInput]!
  skipDuplicates: Boolean
}

input access_tokenCreateNestedManyWithoutUsersInput {
  create: access_tokenCreateWithoutUsersInput
  connectOrCreate: access_tokenCreateOrConnectWithoutUsersInput
  createMany: access_tokenCreateManyUsersInputEnvelope
  connect: access_tokenWhereUniqueInput
}

input usersCreateWithoutNamespace_membersInput {
  id: String
  name: String
  email: String!
  role: user_role
  created_at: DateTime
  updated_at: DateTime
  access_token: access_tokenCreateNestedManyWithoutUsersInput
}

input usersCreateOrConnectWithoutNamespace_membersInput {
  where: usersWhereUniqueInput!
  create: usersCreateWithoutNamespace_membersInput!
}

input usersCreateNestedOneWithoutNamespace_membersInput {
  create: usersCreateWithoutNamespace_membersInput
  connectOrCreate: usersCreateOrConnectWithoutNamespace_membersInput
  connect: usersWhereUniqueInput
}

input namespace_membersCreateWithoutNamespaceInput {
  membership: membership
  created_at: DateTime
  updated_at: DateTime
  users: usersCreateNestedOneWithoutNamespace_membersInput!
}

input namespace_membersCreateOrConnectWithoutNamespaceInput {
  where: namespace_membersWhereUniqueInput!
  create: namespace_membersCreateWithoutNamespaceInput!
}

input namespace_membersCreateManyNamespaceInput {
  user_id: String!
  membership: membership
  created_at: DateTime
  updated_at: DateTime
}

input namespace_membersCreateManyNamespaceInputEnvelope {
  data: [namespace_membersCreateManyNamespaceInput]!
  skipDuplicates: Boolean
}

input namespace_membersCreateNestedManyWithoutNamespaceInput {
  create: namespace_membersCreateWithoutNamespaceInput
  connectOrCreate: namespace_membersCreateOrConnectWithoutNamespaceInput
  createMany: namespace_membersCreateManyNamespaceInputEnvelope
  connect: namespace_membersWhereUniqueInput
}

input namespaceCreateWithoutEnvironmentInput {
  id: String
  name: String!
  created_at: DateTime
  updated_at: DateTime
  price_plan: price_planCreateNestedOneWithoutNamespaceInput
  api: apiCreateNestedManyWithoutNamespaceInput
  namespace_members: namespace_membersCreateNestedManyWithoutNamespaceInput
}

input namespaceCreateOrConnectWithoutEnvironmentInput {
  where: namespaceWhereUniqueInput!
  create: namespaceCreateWithoutEnvironmentInput!
}

input namespaceCreateNestedOneWithoutEnvironmentInput {
  create: namespaceCreateWithoutEnvironmentInput
  connectOrCreate: namespaceCreateOrConnectWithoutEnvironmentInput
  connect: namespaceWhereUniqueInput
}

input edgeCreateWithoutEnvironment_edgesInput {
  id: String
  name: String!
  location: String!
  created_at: DateTime
  updated_at: DateTime
}

input edgeCreateOrConnectWithoutEnvironment_edgesInput {
  where: edgeWhereUniqueInput!
  create: edgeCreateWithoutEnvironment_edgesInput!
}

input edgeCreateNestedOneWithoutEnvironment_edgesInput {
  create: edgeCreateWithoutEnvironment_edgesInput
  connectOrCreate: edgeCreateOrConnectWithoutEnvironment_edgesInput
  connect: edgeWhereUniqueInput
}

input environment_edgesCreateWithoutEnvironmentInput {
  edge: edgeCreateNestedOneWithoutEnvironment_edgesInput!
}

input environment_edgesCreateOrConnectWithoutEnvironmentInput {
  where: environment_edgesWhereUniqueInput!
  create: environment_edgesCreateWithoutEnvironmentInput!
}

input environment_edgesCreateManyEnvironmentInput {
  edge_id: String!
}

input environment_edgesCreateManyEnvironmentInputEnvelope {
  data: [environment_edgesCreateManyEnvironmentInput]!
  skipDuplicates: Boolean
}

input environment_edgesCreateNestedManyWithoutEnvironmentInput {
  create: environment_edgesCreateWithoutEnvironmentInput
  connectOrCreate: environment_edgesCreateOrConnectWithoutEnvironmentInput
  createMany: environment_edgesCreateManyEnvironmentInputEnvelope
  connect: environment_edgesWhereUniqueInput
}

input wundernodeCreateWithoutNodepoolInput {
  id: String
  etag: String!
  config: DateTime!
  ipv4: String
  ipv6: String
  created_at: DateTime
  updated_at: DateTime
}

input wundernodeCreateOrConnectWithoutNodepoolInput {
  where: wundernodeWhereUniqueInput!
  create: wundernodeCreateWithoutNodepoolInput!
}

input wundernodeCreateNestedOneWithoutNodepoolInput {
  create: wundernodeCreateWithoutNodepoolInput
  connectOrCreate: wundernodeCreateOrConnectWithoutNodepoolInput
  connect: wundernodeWhereUniqueInput
}

input nodepoolCreateWithoutNodepool_environmentInput {
  id: String
  shared: Boolean
  created_at: DateTime
  updated_at: DateTime
  wundernode: wundernodeCreateNestedOneWithoutNodepoolInput!
}

input nodepoolCreateOrConnectWithoutNodepool_environmentInput {
  where: nodepoolWhereUniqueInput!
  create: nodepoolCreateWithoutNodepool_environmentInput!
}

input nodepoolCreateNestedOneWithoutNodepool_environmentInput {
  create: nodepoolCreateWithoutNodepool_environmentInput
  connectOrCreate: nodepoolCreateOrConnectWithoutNodepool_environmentInput
  connect: nodepoolWhereUniqueInput
}

input nodepool_environmentCreateWithoutEnvironmentInput {
  nodepool: nodepoolCreateNestedOneWithoutNodepool_environmentInput!
}

input nodepool_environmentCreateOrConnectWithoutEnvironmentInput {
  where: nodepool_environmentWhereUniqueInput!
  create: nodepool_environmentCreateWithoutEnvironmentInput!
}

input nodepool_environmentCreateManyEnvironmentInput {
  nodepool_id: String!
}

input nodepool_environmentCreateManyEnvironmentInputEnvelope {
  data: [nodepool_environmentCreateManyEnvironmentInput]!
  skipDuplicates: Boolean
}

input nodepool_environmentCreateNestedManyWithoutEnvironmentInput {
  create: nodepool_environmentCreateWithoutEnvironmentInput
  connectOrCreate: nodepool_environmentCreateOrConnectWithoutEnvironmentInput
  createMany: nodepool_environmentCreateManyEnvironmentInputEnvelope
  connect: nodepool_environmentWhereUniqueInput
}

input environmentCreateWithoutDeployment_environmentInput {
  id: String
  name: String!
  primary_hostname: String
  primary: Boolean
  hostnames: environmentCreatehostnamesInput
  namespace: namespaceCreateNestedOneWithoutEnvironmentInput!
  environment_edges: environment_edgesCreateNestedManyWithoutEnvironmentInput
  nodepool_environment: nodepool_environmentCreateNestedManyWithoutEnvironmentInput
}

input environmentCreateOrConnectWithoutDeployment_environmentInput {
  where: environmentWhereUniqueInput!
  create: environmentCreateWithoutDeployment_environmentInput!
}

input environmentCreateNestedOneWithoutDeployment_environmentInput {
  create: environmentCreateWithoutDeployment_environmentInput
  connectOrCreate: environmentCreateOrConnectWithoutDeployment_environmentInput
  connect: environmentWhereUniqueInput
}

input deployment_environmentCreateWithoutDeploymentInput {
  environment: environmentCreateNestedOneWithoutDeployment_environmentInput!
}

input deployment_environmentCreateOrConnectWithoutDeploymentInput {
  where: deployment_environmentWhereUniqueInput!
  create: deployment_environmentCreateWithoutDeploymentInput!
}

input deployment_environmentCreateManyDeploymentInput {
  environment_id: String!
}

input deployment_environmentCreateManyDeploymentInputEnvelope {
  data: [deployment_environmentCreateManyDeploymentInput]!
  skipDuplicates: Boolean
}

input deployment_environmentCreateNestedManyWithoutDeploymentInput {
  create: deployment_environmentCreateWithoutDeploymentInput
  connectOrCreate: deployment_environmentCreateOrConnectWithoutDeploymentInput
  createMany: deployment_environmentCreateManyDeploymentInputEnvelope
  connect: deployment_environmentWhereUniqueInput
}

input deploymentCreateWithoutApiInput {
  id: String
  name: String!
  config: DateTime!
  created_at: DateTime
  updated_at: DateTime
  deployment_environment: deployment_environmentCreateNestedManyWithoutDeploymentInput
}

input deploymentCreateOrConnectWithoutApiInput {
  where: deploymentWhereUniqueInput!
  create: deploymentCreateWithoutApiInput!
}

input deploymentCreateManyApiInput {
  id: String
  name: String!
  config: DateTime!
  created_at: DateTime
  updated_at: DateTime
}

input deploymentCreateManyApiInputEnvelope {
  data: [deploymentCreateManyApiInput]!
  skipDuplicates: Boolean
}

input deploymentCreateNestedManyWithoutApiInput {
  create: deploymentCreateWithoutApiInput
  connectOrCreate: deploymentCreateOrConnectWithoutApiInput
  createMany: deploymentCreateManyApiInputEnvelope
  connect: deploymentWhereUniqueInput
}

input apiCreateWithoutNamespaceInput {
  id: String
  name: String!
  markdown_description: String!
  created_at: DateTime
  updated_at: DateTime
  deployment: deploymentCreateNestedManyWithoutApiInput
}

input apiCreateOrConnectWithoutNamespaceInput {
  where: apiWhereUniqueInput!
  create: apiCreateWithoutNamespaceInput!
}

input apiCreateManyNamespaceInput {
  id: String
  name: String!
  markdown_description: String!
  created_at: DateTime
  updated_at: DateTime
}

input apiCreateManyNamespaceInputEnvelope {
  data: [apiCreateManyNamespaceInput]!
  skipDuplicates: Boolean
}

input apiCreateNestedManyWithoutNamespaceInput {
  create: apiCreateWithoutNamespaceInput
  connectOrCreate: apiCreateOrConnectWithoutNamespaceInput
  createMany: apiCreateManyNamespaceInputEnvelope
  connect: apiWhereUniqueInput
}

input namespaceCreateWithoutApiInput {
  id: String
  name: String!
  created_at: DateTime
  updated_at: DateTime
  price_plan: price_planCreateNestedOneWithoutNamespaceInput
  environment: environmentCreateNestedManyWithoutNamespaceInput
  namespace_members: namespace_membersCreateNestedManyWithoutNamespaceInput
}

input namespaceCreateOrConnectWithoutApiInput {
  where: namespaceWhereUniqueInput!
  create: namespaceCreateWithoutApiInput!
}

input namespaceCreateNestedOneWithoutApiInput {
  create: namespaceCreateWithoutApiInput
  connectOrCreate: namespaceCreateOrConnectWithoutApiInput
  connect: namespaceWhereUniqueInput
}

input apiCreateWithoutDeploymentInput {
  id: String
  name: String!
  markdown_description: String!
  created_at: DateTime
  updated_at: DateTime
  namespace: namespaceCreateNestedOneWithoutApiInput!
}

input apiCreateOrConnectWithoutDeploymentInput {
  where: apiWhereUniqueInput!
  create: apiCreateWithoutDeploymentInput!
}

input apiCreateNestedOneWithoutDeploymentInput {
  create: apiCreateWithoutDeploymentInput
  connectOrCreate: apiCreateOrConnectWithoutDeploymentInput
  connect: apiWhereUniqueInput
}

input deploymentCreateWithoutDeployment_environmentInput {
  id: String
  name: String!
  config: DateTime!
  created_at: DateTime
  updated_at: DateTime
  api: apiCreateNestedOneWithoutDeploymentInput!
}

input deploymentCreateOrConnectWithoutDeployment_environmentInput {
  where: deploymentWhereUniqueInput!
  create: deploymentCreateWithoutDeployment_environmentInput!
}

input deploymentCreateNestedOneWithoutDeployment_environmentInput {
  create: deploymentCreateWithoutDeployment_environmentInput
  connectOrCreate: deploymentCreateOrConnectWithoutDeployment_environmentInput
  connect: deploymentWhereUniqueInput
}

input deployment_environmentCreateWithoutEnvironmentInput {
  deployment: deploymentCreateNestedOneWithoutDeployment_environmentInput!
}

input deployment_environmentCreateOrConnectWithoutEnvironmentInput {
  where: deployment_environmentWhereUniqueInput!
  create: deployment_environmentCreateWithoutEnvironmentInput!
}

input deployment_environmentCreateManyEnvironmentInput {
  deployment_id: String!
}

input deployment_environmentCreateManyEnvironmentInputEnvelope {
  data: [deployment_environmentCreateManyEnvironmentInput]!
  skipDuplicates: Boolean
}

input deployment_environmentCreateNestedManyWithoutEnvironmentInput {
  create: deployment_environmentCreateWithoutEnvironmentInput
  connectOrCreate: deployment_environmentCreateOrConnectWithoutEnvironmentInput
  createMany: deployment_environmentCreateManyEnvironmentInputEnvelope
  connect: deployment_environmentWhereUniqueInput
}

input environmentCreateWithoutNamespaceInput {
  id: String
  name: String!
  primary_hostname: String
  primary: Boolean
  hostnames: environmentCreatehostnamesInput
  deployment_environment: deployment_environmentCreateNestedManyWithoutEnvironmentInput
  environment_edges: environment_edgesCreateNestedManyWithoutEnvironmentInput
  nodepool_environment: nodepool_environmentCreateNestedManyWithoutEnvironmentInput
}

input environmentCreateOrConnectWithoutNamespaceInput {
  where: environmentWhereUniqueInput!
  create: environmentCreateWithoutNamespaceInput!
}

input environmentCreateManyhostnamesInput {
  set: [String]!
}

input environmentCreateManyNamespaceInput {
  id: String
  name: String!
  primary_hostname: String
  primary: Boolean
  hostnames: environmentCreateManyhostnamesInput
}

input environmentCreateManyNamespaceInputEnvelope {
  data: [environmentCreateManyNamespaceInput]!
  skipDuplicates: Boolean
}

input environmentCreateNestedManyWithoutNamespaceInput {
  create: environmentCreateWithoutNamespaceInput
  connectOrCreate: environmentCreateOrConnectWithoutNamespaceInput
  createMany: environmentCreateManyNamespaceInputEnvelope
  connect: environmentWhereUniqueInput
}

input namespaceCreateWithoutNamespace_membersInput {
  id: String
  name: String!
  created_at: DateTime
  updated_at: DateTime
  price_plan: price_planCreateNestedOneWithoutNamespaceInput
  api: apiCreateNestedManyWithoutNamespaceInput
  environment: environmentCreateNestedManyWithoutNamespaceInput
}

input namespaceCreateOrConnectWithoutNamespace_membersInput {
  where: namespaceWhereUniqueInput!
  create: namespaceCreateWithoutNamespace_membersInput!
}

input namespaceCreateNestedOneWithoutNamespace_membersInput {
  create: namespaceCreateWithoutNamespace_membersInput
  connectOrCreate: namespaceCreateOrConnectWithoutNamespace_membersInput
  connect: namespaceWhereUniqueInput
}

input namespace_membersCreateWithoutUsersInput {
  membership: membership
  created_at: DateTime
  updated_at: DateTime
  namespace: namespaceCreateNestedOneWithoutNamespace_membersInput!
}

input namespace_membersCreateOrConnectWithoutUsersInput {
  where: namespace_membersWhereUniqueInput!
  create: namespace_membersCreateWithoutUsersInput!
}

input namespace_membersCreateManyUsersInput {
  namespace_id: String!
  membership: membership
  created_at: DateTime
  updated_at: DateTime
}

input namespace_membersCreateManyUsersInputEnvelope {
  data: [namespace_membersCreateManyUsersInput]!
  skipDuplicates: Boolean
}

input namespace_membersCreateNestedManyWithoutUsersInput {
  create: namespace_membersCreateWithoutUsersInput
  connectOrCreate: namespace_membersCreateOrConnectWithoutUsersInput
  createMany: namespace_membersCreateManyUsersInputEnvelope
  connect: namespace_membersWhereUniqueInput
}

input usersCreateWithoutAccess_tokenInput {
  id: String
  name: String
  email: String!
  role: user_role
  created_at: DateTime
  updated_at: DateTime
  namespace_members: namespace_membersCreateNestedManyWithoutUsersInput
}

input usersCreateOrConnectWithoutAccess_tokenInput {
  where: usersWhereUniqueInput!
  create: usersCreateWithoutAccess_tokenInput!
}

input usersCreateNestedOneWithoutAccess_tokenInput {
  create: usersCreateWithoutAccess_tokenInput
  connectOrCreate: usersCreateOrConnectWithoutAccess_tokenInput
  connect: usersWhereUniqueInput
}

input access_tokenCreateInput {
  id: String
  token: String
  name: String!
  created_at: DateTime
  users: usersCreateNestedOneWithoutAccess_tokenInput!
}

input StringFieldUpdateOperationsInput {
  set: String
}

input DateTimeFieldUpdateOperationsInput {
  set: DateTime
}

input NullableStringFieldUpdateOperationsInput {
  set: String
}

input NullableDateTimeFieldUpdateOperationsInput {
  set: DateTime
}

input IntFieldUpdateOperationsInput {
  set: Int
  increment: Int
  decrement: Int
  multiply: Int
  divide: Int
}

input BoolFieldUpdateOperationsInput {
  set: Boolean
}

input price_planUpdateWithoutNamespaceInput {
  name: StringFieldUpdateOperationsInput
  quota_daily_requests: IntFieldUpdateOperationsInput
  quota_environments: IntFieldUpdateOperationsInput
  quota_members: IntFieldUpdateOperationsInput
  quota_apis: IntFieldUpdateOperationsInput
  allow_secondary_environments: BoolFieldUpdateOperationsInput
}

input price_planUpsertWithoutNamespaceInput {
  update: price_planUpdateWithoutNamespaceInput!
  create: price_planCreateWithoutNamespaceInput!
}

input price_planUpdateOneRequiredWithoutNamespaceInput {
  create: price_planCreateWithoutNamespaceInput
  connectOrCreate: price_planCreateOrConnectWithoutNamespaceInput
  upsert: price_planUpsertWithoutNamespaceInput
  connect: price_planWhereUniqueInput
  update: price_planUpdateWithoutNamespaceInput
}

input environmentUpdatehostnamesInput {
  set: [String]
  push: [String]
}

input access_tokenUpdateWithoutUsersInput {
  id: StringFieldUpdateOperationsInput
  token: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
}

input access_tokenUpsertWithWhereUniqueWithoutUsersInput {
  where: access_tokenWhereUniqueInput!
  update: access_tokenUpdateWithoutUsersInput!
  create: access_tokenCreateWithoutUsersInput!
}

input access_tokenUpdateWithWhereUniqueWithoutUsersInput {
  where: access_tokenWhereUniqueInput!
  data: access_tokenUpdateWithoutUsersInput!
}

input access_tokenScalarWhereInput {
  AND: access_tokenScalarWhereInput
  OR: [access_tokenScalarWhereInput]
  NOT: access_tokenScalarWhereInput
  id: StringFilter
  token: StringFilter
  user_id: StringFilter
  name: StringFilter
  created_at: DateTimeFilter
}

input access_tokenUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  token: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
}

input access_tokenUpdateManyWithWhereWithoutUsersInput {
  where: access_tokenScalarWhereInput!
  data: access_tokenUpdateManyMutationInput!
}

input access_tokenUpdateManyWithoutUsersInput {
  create: access_tokenCreateWithoutUsersInput
  connectOrCreate: access_tokenCreateOrConnectWithoutUsersInput
  upsert: access_tokenUpsertWithWhereUniqueWithoutUsersInput
  createMany: access_tokenCreateManyUsersInputEnvelope
  connect: access_tokenWhereUniqueInput
  set: access_tokenWhereUniqueInput
  disconnect: access_tokenWhereUniqueInput
  delete: access_tokenWhereUniqueInput
  update: access_tokenUpdateWithWhereUniqueWithoutUsersInput
  updateMany: access_tokenUpdateManyWithWhereWithoutUsersInput
  deleteMany: access_tokenScalarWhereInput
}

input usersUpdateWithoutNamespace_membersInput {
  id: StringFieldUpdateOperationsInput
  name: NullableStringFieldUpdateOperationsInput
  email: StringFieldUpdateOperationsInput
  role: user_role
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  access_token: access_tokenUpdateManyWithoutUsersInput
}

input usersUpsertWithoutNamespace_membersInput {
  update: usersUpdateWithoutNamespace_membersInput!
  create: usersCreateWithoutNamespace_membersInput!
}

input usersUpdateOneRequiredWithoutNamespace_membersInput {
  create: usersCreateWithoutNamespace_membersInput
  connectOrCreate: usersCreateOrConnectWithoutNamespace_membersInput
  upsert: usersUpsertWithoutNamespace_membersInput
  connect: usersWhereUniqueInput
  update: usersUpdateWithoutNamespace_membersInput
}

input namespace_membersUpdateWithoutNamespaceInput {
  membership: membership
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  users: usersUpdateOneRequiredWithoutNamespace_membersInput
}

input namespace_membersUpsertWithWhereUniqueWithoutNamespaceInput {
  where: namespace_membersWhereUniqueInput!
  update: namespace_membersUpdateWithoutNamespaceInput!
  create: namespace_membersCreateWithoutNamespaceInput!
}

input namespace_membersUpdateWithWhereUniqueWithoutNamespaceInput {
  where: namespace_membersWhereUniqueInput!
  data: namespace_membersUpdateWithoutNamespaceInput!
}

input namespace_membersScalarWhereInput {
  AND: namespace_membersScalarWhereInput
  OR: [namespace_membersScalarWhereInput]
  NOT: namespace_membersScalarWhereInput
  user_id: StringFilter
  namespace_id: StringFilter
  membership: EnummembershipFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
}

input namespace_membersUpdateManyMutationInput {
  membership: membership
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input namespace_membersUpdateManyWithWhereWithoutNamespaceInput {
  where: namespace_membersScalarWhereInput!
  data: namespace_membersUpdateManyMutationInput!
}

input namespace_membersUpdateManyWithoutNamespaceInput {
  create: namespace_membersCreateWithoutNamespaceInput
  connectOrCreate: namespace_membersCreateOrConnectWithoutNamespaceInput
  upsert: namespace_membersUpsertWithWhereUniqueWithoutNamespaceInput
  createMany: namespace_membersCreateManyNamespaceInputEnvelope
  connect: namespace_membersWhereUniqueInput
  set: namespace_membersWhereUniqueInput
  disconnect: namespace_membersWhereUniqueInput
  delete: namespace_membersWhereUniqueInput
  update: namespace_membersUpdateWithWhereUniqueWithoutNamespaceInput
  updateMany: namespace_membersUpdateManyWithWhereWithoutNamespaceInput
  deleteMany: namespace_membersScalarWhereInput
}

input namespaceUpdateWithoutEnvironmentInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  price_plan: price_planUpdateOneRequiredWithoutNamespaceInput
  api: apiUpdateManyWithoutNamespaceInput
  namespace_members: namespace_membersUpdateManyWithoutNamespaceInput
}

input namespaceUpsertWithoutEnvironmentInput {
  update: namespaceUpdateWithoutEnvironmentInput!
  create: namespaceCreateWithoutEnvironmentInput!
}

input namespaceUpdateOneRequiredWithoutEnvironmentInput {
  create: namespaceCreateWithoutEnvironmentInput
  connectOrCreate: namespaceCreateOrConnectWithoutEnvironmentInput
  upsert: namespaceUpsertWithoutEnvironmentInput
  connect: namespaceWhereUniqueInput
  update: namespaceUpdateWithoutEnvironmentInput
}

input edgeUpdateWithoutEnvironment_edgesInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  location: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input edgeUpsertWithoutEnvironment_edgesInput {
  update: edgeUpdateWithoutEnvironment_edgesInput!
  create: edgeCreateWithoutEnvironment_edgesInput!
}

input edgeUpdateOneRequiredWithoutEnvironment_edgesInput {
  create: edgeCreateWithoutEnvironment_edgesInput
  connectOrCreate: edgeCreateOrConnectWithoutEnvironment_edgesInput
  upsert: edgeUpsertWithoutEnvironment_edgesInput
  connect: edgeWhereUniqueInput
  update: edgeUpdateWithoutEnvironment_edgesInput
}

input environment_edgesUpdateWithoutEnvironmentInput {
  edge: edgeUpdateOneRequiredWithoutEnvironment_edgesInput
}

input environment_edgesUpsertWithWhereUniqueWithoutEnvironmentInput {
  where: environment_edgesWhereUniqueInput!
  update: environment_edgesUpdateWithoutEnvironmentInput!
  create: environment_edgesCreateWithoutEnvironmentInput!
}

input environment_edgesUpdateWithWhereUniqueWithoutEnvironmentInput {
  where: environment_edgesWhereUniqueInput!
  data: environment_edgesUpdateWithoutEnvironmentInput!
}

input environment_edgesScalarWhereInput {
  AND: environment_edgesScalarWhereInput
  OR: [environment_edgesScalarWhereInput]
  NOT: environment_edgesScalarWhereInput
  environment_id: StringFilter
  edge_id: StringFilter
}

input environment_edgesUpdateManyWithWhereWithoutEnvironmentInput {
  where: environment_edgesScalarWhereInput!
}

input environment_edgesUpdateManyWithoutEnvironmentInput {
  create: environment_edgesCreateWithoutEnvironmentInput
  connectOrCreate: environment_edgesCreateOrConnectWithoutEnvironmentInput
  upsert: environment_edgesUpsertWithWhereUniqueWithoutEnvironmentInput
  createMany: environment_edgesCreateManyEnvironmentInputEnvelope
  connect: environment_edgesWhereUniqueInput
  set: environment_edgesWhereUniqueInput
  disconnect: environment_edgesWhereUniqueInput
  delete: environment_edgesWhereUniqueInput
  update: environment_edgesUpdateWithWhereUniqueWithoutEnvironmentInput
  updateMany: environment_edgesUpdateManyWithWhereWithoutEnvironmentInput
  deleteMany: environment_edgesScalarWhereInput
}

input wundernodeUpdateWithoutNodepoolInput {
  id: StringFieldUpdateOperationsInput
  etag: StringFieldUpdateOperationsInput
  config: DateTime
  ipv4: NullableStringFieldUpdateOperationsInput
  ipv6: NullableStringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input wundernodeUpsertWithoutNodepoolInput {
  update: wundernodeUpdateWithoutNodepoolInput!
  create: wundernodeCreateWithoutNodepoolInput!
}

input wundernodeUpdateOneRequiredWithoutNodepoolInput {
  create: wundernodeCreateWithoutNodepoolInput
  connectOrCreate: wundernodeCreateOrConnectWithoutNodepoolInput
  upsert: wundernodeUpsertWithoutNodepoolInput
  connect: wundernodeWhereUniqueInput
  update: wundernodeUpdateWithoutNodepoolInput
}

input nodepoolUpdateWithoutNodepool_environmentInput {
  id: StringFieldUpdateOperationsInput
  shared: BoolFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  wundernode: wundernodeUpdateOneRequiredWithoutNodepoolInput
}

input nodepoolUpsertWithoutNodepool_environmentInput {
  update: nodepoolUpdateWithoutNodepool_environmentInput!
  create: nodepoolCreateWithoutNodepool_environmentInput!
}

input nodepoolUpdateOneRequiredWithoutNodepool_environmentInput {
  create: nodepoolCreateWithoutNodepool_environmentInput
  connectOrCreate: nodepoolCreateOrConnectWithoutNodepool_environmentInput
  upsert: nodepoolUpsertWithoutNodepool_environmentInput
  connect: nodepoolWhereUniqueInput
  update: nodepoolUpdateWithoutNodepool_environmentInput
}

input nodepool_environmentUpdateWithoutEnvironmentInput {
  nodepool: nodepoolUpdateOneRequiredWithoutNodepool_environmentInput
}

input nodepool_environmentUpsertWithWhereUniqueWithoutEnvironmentInput {
  where: nodepool_environmentWhereUniqueInput!
  update: nodepool_environmentUpdateWithoutEnvironmentInput!
  create: nodepool_environmentCreateWithoutEnvironmentInput!
}

input nodepool_environmentUpdateWithWhereUniqueWithoutEnvironmentInput {
  where: nodepool_environmentWhereUniqueInput!
  data: nodepool_environmentUpdateWithoutEnvironmentInput!
}

input nodepool_environmentScalarWhereInput {
  AND: nodepool_environmentScalarWhereInput
  OR: [nodepool_environmentScalarWhereInput]
  NOT: nodepool_environmentScalarWhereInput
  nodepool_id: StringFilter
  environment_id: StringFilter
}

input nodepool_environmentUpdateManyWithWhereWithoutEnvironmentInput {
  where: nodepool_environmentScalarWhereInput!
}

input nodepool_environmentUpdateManyWithoutEnvironmentInput {
  create: nodepool_environmentCreateWithoutEnvironmentInput
  connectOrCreate: nodepool_environmentCreateOrConnectWithoutEnvironmentInput
  upsert: nodepool_environmentUpsertWithWhereUniqueWithoutEnvironmentInput
  createMany: nodepool_environmentCreateManyEnvironmentInputEnvelope
  connect: nodepool_environmentWhereUniqueInput
  set: nodepool_environmentWhereUniqueInput
  disconnect: nodepool_environmentWhereUniqueInput
  delete: nodepool_environmentWhereUniqueInput
  update: nodepool_environmentUpdateWithWhereUniqueWithoutEnvironmentInput
  updateMany: nodepool_environmentUpdateManyWithWhereWithoutEnvironmentInput
  deleteMany: nodepool_environmentScalarWhereInput
}

input environmentUpdateWithoutDeployment_environmentInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  primary_hostname: StringFieldUpdateOperationsInput
  primary: BoolFieldUpdateOperationsInput
  hostnames: environmentUpdatehostnamesInput
  namespace: namespaceUpdateOneRequiredWithoutEnvironmentInput
  environment_edges: environment_edgesUpdateManyWithoutEnvironmentInput
  nodepool_environment: nodepool_environmentUpdateManyWithoutEnvironmentInput
}

input environmentUpsertWithoutDeployment_environmentInput {
  update: environmentUpdateWithoutDeployment_environmentInput!
  create: environmentCreateWithoutDeployment_environmentInput!
}

input environmentUpdateOneRequiredWithoutDeployment_environmentInput {
  create: environmentCreateWithoutDeployment_environmentInput
  connectOrCreate: environmentCreateOrConnectWithoutDeployment_environmentInput
  upsert: environmentUpsertWithoutDeployment_environmentInput
  connect: environmentWhereUniqueInput
  update: environmentUpdateWithoutDeployment_environmentInput
}

input deployment_environmentUpdateWithoutDeploymentInput {
  environment: environmentUpdateOneRequiredWithoutDeployment_environmentInput
}

input deployment_environmentUpsertWithWhereUniqueWithoutDeploymentInput {
  where: deployment_environmentWhereUniqueInput!
  update: deployment_environmentUpdateWithoutDeploymentInput!
  create: deployment_environmentCreateWithoutDeploymentInput!
}

input deployment_environmentUpdateWithWhereUniqueWithoutDeploymentInput {
  where: deployment_environmentWhereUniqueInput!
  data: deployment_environmentUpdateWithoutDeploymentInput!
}

input deployment_environmentScalarWhereInput {
  AND: deployment_environmentScalarWhereInput
  OR: [deployment_environmentScalarWhereInput]
  NOT: deployment_environmentScalarWhereInput
  deployment_id: StringFilter
  environment_id: StringFilter
}

input deployment_environmentUpdateManyWithWhereWithoutDeploymentInput {
  where: deployment_environmentScalarWhereInput!
}

input deployment_environmentUpdateManyWithoutDeploymentInput {
  create: deployment_environmentCreateWithoutDeploymentInput
  connectOrCreate: deployment_environmentCreateOrConnectWithoutDeploymentInput
  upsert: deployment_environmentUpsertWithWhereUniqueWithoutDeploymentInput
  createMany: deployment_environmentCreateManyDeploymentInputEnvelope
  connect: deployment_environmentWhereUniqueInput
  set: deployment_environmentWhereUniqueInput
  disconnect: deployment_environmentWhereUniqueInput
  delete: deployment_environmentWhereUniqueInput
  update: deployment_environmentUpdateWithWhereUniqueWithoutDeploymentInput
  updateMany: deployment_environmentUpdateManyWithWhereWithoutDeploymentInput
  deleteMany: deployment_environmentScalarWhereInput
}

input deploymentUpdateWithoutApiInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  config: DateTime
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  deployment_environment: deployment_environmentUpdateManyWithoutDeploymentInput
}

input deploymentUpsertWithWhereUniqueWithoutApiInput {
  where: deploymentWhereUniqueInput!
  update: deploymentUpdateWithoutApiInput!
  create: deploymentCreateWithoutApiInput!
}

input deploymentUpdateWithWhereUniqueWithoutApiInput {
  where: deploymentWhereUniqueInput!
  data: deploymentUpdateWithoutApiInput!
}

input deploymentScalarWhereInput {
  AND: deploymentScalarWhereInput
  OR: [deploymentScalarWhereInput]
  NOT: deploymentScalarWhereInput
  id: StringFilter
  api_id: StringFilter
  name: StringFilter
  config: JsonFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
}

input deploymentUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  config: DateTime
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input deploymentUpdateManyWithWhereWithoutApiInput {
  where: deploymentScalarWhereInput!
  data: deploymentUpdateManyMutationInput!
}

input deploymentUpdateManyWithoutApiInput {
  create: deploymentCreateWithoutApiInput
  connectOrCreate: deploymentCreateOrConnectWithoutApiInput
  upsert: deploymentUpsertWithWhereUniqueWithoutApiInput
  createMany: deploymentCreateManyApiInputEnvelope
  connect: deploymentWhereUniqueInput
  set: deploymentWhereUniqueInput
  disconnect: deploymentWhereUniqueInput
  delete: deploymentWhereUniqueInput
  update: deploymentUpdateWithWhereUniqueWithoutApiInput
  updateMany: deploymentUpdateManyWithWhereWithoutApiInput
  deleteMany: deploymentScalarWhereInput
}

input apiUpdateWithoutNamespaceInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  markdown_description: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  deployment: deploymentUpdateManyWithoutApiInput
}

input apiUpsertWithWhereUniqueWithoutNamespaceInput {
  where: apiWhereUniqueInput!
  update: apiUpdateWithoutNamespaceInput!
  create: apiCreateWithoutNamespaceInput!
}

input apiUpdateWithWhereUniqueWithoutNamespaceInput {
  where: apiWhereUniqueInput!
  data: apiUpdateWithoutNamespaceInput!
}

input apiScalarWhereInput {
  AND: apiScalarWhereInput
  OR: [apiScalarWhereInput]
  NOT: apiScalarWhereInput
  id: StringFilter
  namespace_id: StringFilter
  name: StringFilter
  markdown_description: StringFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
}

input apiUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  markdown_description: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input apiUpdateManyWithWhereWithoutNamespaceInput {
  where: apiScalarWhereInput!
  data: apiUpdateManyMutationInput!
}

input apiUpdateManyWithoutNamespaceInput {
  create: apiCreateWithoutNamespaceInput
  connectOrCreate: apiCreateOrConnectWithoutNamespaceInput
  upsert: apiUpsertWithWhereUniqueWithoutNamespaceInput
  createMany: apiCreateManyNamespaceInputEnvelope
  connect: apiWhereUniqueInput
  set: apiWhereUniqueInput
  disconnect: apiWhereUniqueInput
  delete: apiWhereUniqueInput
  update: apiUpdateWithWhereUniqueWithoutNamespaceInput
  updateMany: apiUpdateManyWithWhereWithoutNamespaceInput
  deleteMany: apiScalarWhereInput
}

input namespaceUpdateWithoutApiInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  price_plan: price_planUpdateOneRequiredWithoutNamespaceInput
  environment: environmentUpdateManyWithoutNamespaceInput
  namespace_members: namespace_membersUpdateManyWithoutNamespaceInput
}

input namespaceUpsertWithoutApiInput {
  update: namespaceUpdateWithoutApiInput!
  create: namespaceCreateWithoutApiInput!
}

input namespaceUpdateOneRequiredWithoutApiInput {
  create: namespaceCreateWithoutApiInput
  connectOrCreate: namespaceCreateOrConnectWithoutApiInput
  upsert: namespaceUpsertWithoutApiInput
  connect: namespaceWhereUniqueInput
  update: namespaceUpdateWithoutApiInput
}

input apiUpdateWithoutDeploymentInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  markdown_description: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  namespace: namespaceUpdateOneRequiredWithoutApiInput
}

input apiUpsertWithoutDeploymentInput {
  update: apiUpdateWithoutDeploymentInput!
  create: apiCreateWithoutDeploymentInput!
}

input apiUpdateOneRequiredWithoutDeploymentInput {
  create: apiCreateWithoutDeploymentInput
  connectOrCreate: apiCreateOrConnectWithoutDeploymentInput
  upsert: apiUpsertWithoutDeploymentInput
  connect: apiWhereUniqueInput
  update: apiUpdateWithoutDeploymentInput
}

input deploymentUpdateWithoutDeployment_environmentInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  config: DateTime
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  api: apiUpdateOneRequiredWithoutDeploymentInput
}

input deploymentUpsertWithoutDeployment_environmentInput {
  update: deploymentUpdateWithoutDeployment_environmentInput!
  create: deploymentCreateWithoutDeployment_environmentInput!
}

input deploymentUpdateOneRequiredWithoutDeployment_environmentInput {
  create: deploymentCreateWithoutDeployment_environmentInput
  connectOrCreate: deploymentCreateOrConnectWithoutDeployment_environmentInput
  upsert: deploymentUpsertWithoutDeployment_environmentInput
  connect: deploymentWhereUniqueInput
  update: deploymentUpdateWithoutDeployment_environmentInput
}

input deployment_environmentUpdateWithoutEnvironmentInput {
  deployment: deploymentUpdateOneRequiredWithoutDeployment_environmentInput
}

input deployment_environmentUpsertWithWhereUniqueWithoutEnvironmentInput {
  where: deployment_environmentWhereUniqueInput!
  update: deployment_environmentUpdateWithoutEnvironmentInput!
  create: deployment_environmentCreateWithoutEnvironmentInput!
}

input deployment_environmentUpdateWithWhereUniqueWithoutEnvironmentInput {
  where: deployment_environmentWhereUniqueInput!
  data: deployment_environmentUpdateWithoutEnvironmentInput!
}

input deployment_environmentUpdateManyWithWhereWithoutEnvironmentInput {
  where: deployment_environmentScalarWhereInput!
}

input deployment_environmentUpdateManyWithoutEnvironmentInput {
  create: deployment_environmentCreateWithoutEnvironmentInput
  connectOrCreate: deployment_environmentCreateOrConnectWithoutEnvironmentInput
  upsert: deployment_environmentUpsertWithWhereUniqueWithoutEnvironmentInput
  createMany: deployment_environmentCreateManyEnvironmentInputEnvelope
  connect: deployment_environmentWhereUniqueInput
  set: deployment_environmentWhereUniqueInput
  disconnect: deployment_environmentWhereUniqueInput
  delete: deployment_environmentWhereUniqueInput
  update: deployment_environmentUpdateWithWhereUniqueWithoutEnvironmentInput
  updateMany: deployment_environmentUpdateManyWithWhereWithoutEnvironmentInput
  deleteMany: deployment_environmentScalarWhereInput
}

input environmentUpdateWithoutNamespaceInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  primary_hostname: StringFieldUpdateOperationsInput
  primary: BoolFieldUpdateOperationsInput
  hostnames: environmentUpdatehostnamesInput
  deployment_environment: deployment_environmentUpdateManyWithoutEnvironmentInput
  environment_edges: environment_edgesUpdateManyWithoutEnvironmentInput
  nodepool_environment: nodepool_environmentUpdateManyWithoutEnvironmentInput
}

input environmentUpsertWithWhereUniqueWithoutNamespaceInput {
  where: environmentWhereUniqueInput!
  update: environmentUpdateWithoutNamespaceInput!
  create: environmentCreateWithoutNamespaceInput!
}

input environmentUpdateWithWhereUniqueWithoutNamespaceInput {
  where: environmentWhereUniqueInput!
  data: environmentUpdateWithoutNamespaceInput!
}

input environmentScalarWhereInput {
  AND: environmentScalarWhereInput
  OR: [environmentScalarWhereInput]
  NOT: environmentScalarWhereInput
  id: StringFilter
  name: StringFilter
  namespace_id: StringFilter
  primary_hostname: StringFilter
  hostnames: StringNullableListFilter
  primary: BoolFilter
}

input environmentUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  primary_hostname: StringFieldUpdateOperationsInput
  primary: BoolFieldUpdateOperationsInput
  hostnames: environmentUpdatehostnamesInput
}

input environmentUpdateManyWithWhereWithoutNamespaceInput {
  where: environmentScalarWhereInput!
  data: environmentUpdateManyMutationInput!
}

input environmentUpdateManyWithoutNamespaceInput {
  create: environmentCreateWithoutNamespaceInput
  connectOrCreate: environmentCreateOrConnectWithoutNamespaceInput
  upsert: environmentUpsertWithWhereUniqueWithoutNamespaceInput
  createMany: environmentCreateManyNamespaceInputEnvelope
  connect: environmentWhereUniqueInput
  set: environmentWhereUniqueInput
  disconnect: environmentWhereUniqueInput
  delete: environmentWhereUniqueInput
  update: environmentUpdateWithWhereUniqueWithoutNamespaceInput
  updateMany: environmentUpdateManyWithWhereWithoutNamespaceInput
  deleteMany: environmentScalarWhereInput
}

input namespaceUpdateWithoutNamespace_membersInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  price_plan: price_planUpdateOneRequiredWithoutNamespaceInput
  api: apiUpdateManyWithoutNamespaceInput
  environment: environmentUpdateManyWithoutNamespaceInput
}

input namespaceUpsertWithoutNamespace_membersInput {
  update: namespaceUpdateWithoutNamespace_membersInput!
  create: namespaceCreateWithoutNamespace_membersInput!
}

input namespaceUpdateOneRequiredWithoutNamespace_membersInput {
  create: namespaceCreateWithoutNamespace_membersInput
  connectOrCreate: namespaceCreateOrConnectWithoutNamespace_membersInput
  upsert: namespaceUpsertWithoutNamespace_membersInput
  connect: namespaceWhereUniqueInput
  update: namespaceUpdateWithoutNamespace_membersInput
}

input namespace_membersUpdateWithoutUsersInput {
  membership: membership
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  namespace: namespaceUpdateOneRequiredWithoutNamespace_membersInput
}

input namespace_membersUpsertWithWhereUniqueWithoutUsersInput {
  where: namespace_membersWhereUniqueInput!
  update: namespace_membersUpdateWithoutUsersInput!
  create: namespace_membersCreateWithoutUsersInput!
}

input namespace_membersUpdateWithWhereUniqueWithoutUsersInput {
  where: namespace_membersWhereUniqueInput!
  data: namespace_membersUpdateWithoutUsersInput!
}

input namespace_membersUpdateManyWithWhereWithoutUsersInput {
  where: namespace_membersScalarWhereInput!
  data: namespace_membersUpdateManyMutationInput!
}

input namespace_membersUpdateManyWithoutUsersInput {
  create: namespace_membersCreateWithoutUsersInput
  connectOrCreate: namespace_membersCreateOrConnectWithoutUsersInput
  upsert: namespace_membersUpsertWithWhereUniqueWithoutUsersInput
  createMany: namespace_membersCreateManyUsersInputEnvelope
  connect: namespace_membersWhereUniqueInput
  set: namespace_membersWhereUniqueInput
  disconnect: namespace_membersWhereUniqueInput
  delete: namespace_membersWhereUniqueInput
  update: namespace_membersUpdateWithWhereUniqueWithoutUsersInput
  updateMany: namespace_membersUpdateManyWithWhereWithoutUsersInput
  deleteMany: namespace_membersScalarWhereInput
}

input usersUpdateWithoutAccess_tokenInput {
  id: StringFieldUpdateOperationsInput
  name: NullableStringFieldUpdateOperationsInput
  email: StringFieldUpdateOperationsInput
  role: user_role
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  namespace_members: namespace_membersUpdateManyWithoutUsersInput
}

input usersUpsertWithoutAccess_tokenInput {
  update: usersUpdateWithoutAccess_tokenInput!
  create: usersCreateWithoutAccess_tokenInput!
}

input usersUpdateOneRequiredWithoutAccess_tokenInput {
  create: usersCreateWithoutAccess_tokenInput
  connectOrCreate: usersCreateOrConnectWithoutAccess_tokenInput
  upsert: usersUpsertWithoutAccess_tokenInput
  connect: usersWhereUniqueInput
  update: usersUpdateWithoutAccess_tokenInput
}

input access_tokenUpdateInput {
  id: StringFieldUpdateOperationsInput
  token: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  users: usersUpdateOneRequiredWithoutAccess_tokenInput
}

input access_tokenCreateManyInput {
  id: String
  token: String
  user_id: String!
  name: String!
  created_at: DateTime
}

type AffectedRowsOutput {
  count: Int!
}

input admin_configCreateInput {
  id: String
  wundernode_image_tag: String!
  created_at: DateTime
  updated_at: DateTime
}

input admin_configUpdateInput {
  id: StringFieldUpdateOperationsInput
  wundernode_image_tag: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input admin_configCreateManyInput {
  id: String
  wundernode_image_tag: String!
  created_at: DateTime
  updated_at: DateTime
}

input admin_configUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  wundernode_image_tag: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input apiCreateInput {
  id: String
  name: String!
  markdown_description: String!
  created_at: DateTime
  updated_at: DateTime
  namespace: namespaceCreateNestedOneWithoutApiInput!
  deployment: deploymentCreateNestedManyWithoutApiInput
}

input apiUpdateInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  markdown_description: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  namespace: namespaceUpdateOneRequiredWithoutApiInput
  deployment: deploymentUpdateManyWithoutApiInput
}

input apiCreateManyInput {
  id: String
  namespace_id: String!
  name: String!
  markdown_description: String!
  created_at: DateTime
  updated_at: DateTime
}

input deploymentCreateInput {
  id: String
  name: String!
  config: DateTime!
  created_at: DateTime
  updated_at: DateTime
  api: apiCreateNestedOneWithoutDeploymentInput!
  deployment_environment: deployment_environmentCreateNestedManyWithoutDeploymentInput
}

input deploymentUpdateInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  config: DateTime
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  api: apiUpdateOneRequiredWithoutDeploymentInput
  deployment_environment: deployment_environmentUpdateManyWithoutDeploymentInput
}

input deploymentCreateManyInput {
  id: String
  api_id: String!
  name: String!
  config: DateTime!
  created_at: DateTime
  updated_at: DateTime
}

input deployment_environmentCreateInput {
  deployment: deploymentCreateNestedOneWithoutDeployment_environmentInput!
  environment: environmentCreateNestedOneWithoutDeployment_environmentInput!
}

input deployment_environmentUpdateInput {
  deployment: deploymentUpdateOneRequiredWithoutDeployment_environmentInput
  environment: environmentUpdateOneRequiredWithoutDeployment_environmentInput
}

input deployment_environmentCreateManyInput {
  deployment_id: String!
  environment_id: String!
}

input environmentCreateWithoutEnvironment_edgesInput {
  id: String
  name: String!
  primary_hostname: String
  primary: Boolean
  hostnames: environmentCreatehostnamesInput
  namespace: namespaceCreateNestedOneWithoutEnvironmentInput!
  deployment_environment: deployment_environmentCreateNestedManyWithoutEnvironmentInput
  nodepool_environment: nodepool_environmentCreateNestedManyWithoutEnvironmentInput
}

input environmentCreateOrConnectWithoutEnvironment_edgesInput {
  where: environmentWhereUniqueInput!
  create: environmentCreateWithoutEnvironment_edgesInput!
}

input environmentCreateNestedOneWithoutEnvironment_edgesInput {
  create: environmentCreateWithoutEnvironment_edgesInput
  connectOrCreate: environmentCreateOrConnectWithoutEnvironment_edgesInput
  connect: environmentWhereUniqueInput
}

input environment_edgesCreateWithoutEdgeInput {
  environment: environmentCreateNestedOneWithoutEnvironment_edgesInput!
}

input environment_edgesCreateOrConnectWithoutEdgeInput {
  where: environment_edgesWhereUniqueInput!
  create: environment_edgesCreateWithoutEdgeInput!
}

input environment_edgesCreateManyEdgeInput {
  environment_id: String!
}

input environment_edgesCreateManyEdgeInputEnvelope {
  data: [environment_edgesCreateManyEdgeInput]!
  skipDuplicates: Boolean
}

input environment_edgesCreateNestedManyWithoutEdgeInput {
  create: environment_edgesCreateWithoutEdgeInput
  connectOrCreate: environment_edgesCreateOrConnectWithoutEdgeInput
  createMany: environment_edgesCreateManyEdgeInputEnvelope
  connect: environment_edgesWhereUniqueInput
}

input edgeCreateInput {
  id: String
  name: String!
  location: String!
  created_at: DateTime
  updated_at: DateTime
  environment_edges: environment_edgesCreateNestedManyWithoutEdgeInput
}

input environmentUpdateWithoutEnvironment_edgesInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  primary_hostname: StringFieldUpdateOperationsInput
  primary: BoolFieldUpdateOperationsInput
  hostnames: environmentUpdatehostnamesInput
  namespace: namespaceUpdateOneRequiredWithoutEnvironmentInput
  deployment_environment: deployment_environmentUpdateManyWithoutEnvironmentInput
  nodepool_environment: nodepool_environmentUpdateManyWithoutEnvironmentInput
}

input environmentUpsertWithoutEnvironment_edgesInput {
  update: environmentUpdateWithoutEnvironment_edgesInput!
  create: environmentCreateWithoutEnvironment_edgesInput!
}

input environmentUpdateOneRequiredWithoutEnvironment_edgesInput {
  create: environmentCreateWithoutEnvironment_edgesInput
  connectOrCreate: environmentCreateOrConnectWithoutEnvironment_edgesInput
  upsert: environmentUpsertWithoutEnvironment_edgesInput
  connect: environmentWhereUniqueInput
  update: environmentUpdateWithoutEnvironment_edgesInput
}

input environment_edgesUpdateWithoutEdgeInput {
  environment: environmentUpdateOneRequiredWithoutEnvironment_edgesInput
}

input environment_edgesUpsertWithWhereUniqueWithoutEdgeInput {
  where: environment_edgesWhereUniqueInput!
  update: environment_edgesUpdateWithoutEdgeInput!
  create: environment_edgesCreateWithoutEdgeInput!
}

input environment_edgesUpdateWithWhereUniqueWithoutEdgeInput {
  where: environment_edgesWhereUniqueInput!
  data: environment_edgesUpdateWithoutEdgeInput!
}

input environment_edgesUpdateManyWithWhereWithoutEdgeInput {
  where: environment_edgesScalarWhereInput!
}

input environment_edgesUpdateManyWithoutEdgeInput {
  create: environment_edgesCreateWithoutEdgeInput
  connectOrCreate: environment_edgesCreateOrConnectWithoutEdgeInput
  upsert: environment_edgesUpsertWithWhereUniqueWithoutEdgeInput
  createMany: environment_edgesCreateManyEdgeInputEnvelope
  connect: environment_edgesWhereUniqueInput
  set: environment_edgesWhereUniqueInput
  disconnect: environment_edgesWhereUniqueInput
  delete: environment_edgesWhereUniqueInput
  update: environment_edgesUpdateWithWhereUniqueWithoutEdgeInput
  updateMany: environment_edgesUpdateManyWithWhereWithoutEdgeInput
  deleteMany: environment_edgesScalarWhereInput
}

input edgeUpdateInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  location: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  environment_edges: environment_edgesUpdateManyWithoutEdgeInput
}

input edgeCreateManyInput {
  id: String
  name: String!
  location: String!
  created_at: DateTime
  updated_at: DateTime
}

input edgeUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  location: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input environmentCreateInput {
  id: String
  name: String!
  primary_hostname: String
  primary: Boolean
  hostnames: environmentCreatehostnamesInput
  namespace: namespaceCreateNestedOneWithoutEnvironmentInput!
  deployment_environment: deployment_environmentCreateNestedManyWithoutEnvironmentInput
  environment_edges: environment_edgesCreateNestedManyWithoutEnvironmentInput
  nodepool_environment: nodepool_environmentCreateNestedManyWithoutEnvironmentInput
}

input environmentUpdateInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  primary_hostname: StringFieldUpdateOperationsInput
  primary: BoolFieldUpdateOperationsInput
  hostnames: environmentUpdatehostnamesInput
  namespace: namespaceUpdateOneRequiredWithoutEnvironmentInput
  deployment_environment: deployment_environmentUpdateManyWithoutEnvironmentInput
  environment_edges: environment_edgesUpdateManyWithoutEnvironmentInput
  nodepool_environment: nodepool_environmentUpdateManyWithoutEnvironmentInput
}

input environmentCreateManyInput {
  id: String
  name: String!
  namespace_id: String!
  primary_hostname: String
  primary: Boolean
  hostnames: environmentCreateManyhostnamesInput
}

input environment_edgesCreateInput {
  edge: edgeCreateNestedOneWithoutEnvironment_edgesInput!
  environment: environmentCreateNestedOneWithoutEnvironment_edgesInput!
}

input environment_edgesUpdateInput {
  edge: edgeUpdateOneRequiredWithoutEnvironment_edgesInput
  environment: environmentUpdateOneRequiredWithoutEnvironment_edgesInput
}

input environment_edgesCreateManyInput {
  environment_id: String!
  edge_id: String!
}

input letsencrypt_certificateCreateadditional_domainsInput {
  set: [String]!
}

input letsencrypt_userCreateWithoutLetsencrypt_certificateInput {
  zone: String!
  email: String!
  dns_provider_name: String!
  dns_provider_token: String!
  private_key: String
  registration_resource: DateTime
  created_at: DateTime
  updated_at: DateTime
}

input letsencrypt_userCreateOrConnectWithoutLetsencrypt_certificateInput {
  where: letsencrypt_userWhereUniqueInput!
  create: letsencrypt_userCreateWithoutLetsencrypt_certificateInput!
}

input letsencrypt_userCreateNestedOneWithoutLetsencrypt_certificateInput {
  create: letsencrypt_userCreateWithoutLetsencrypt_certificateInput
  connectOrCreate: letsencrypt_userCreateOrConnectWithoutLetsencrypt_certificateInput
  connect: letsencrypt_userWhereUniqueInput
}

input letsencrypt_certificateCreateInput {
  common_name: String!
  issued: DateTime
  renewal: DateTime
  certificate: String
  private_key: String
  created_at: DateTime
  updated_at: DateTime
  additional_domains: letsencrypt_certificateCreateadditional_domainsInput
  letsencrypt_user: letsencrypt_userCreateNestedOneWithoutLetsencrypt_certificateInput!
}

input letsencrypt_certificateUpdateadditional_domainsInput {
  set: [String]
  push: [String]
}

input letsencrypt_userUpdateWithoutLetsencrypt_certificateInput {
  zone: StringFieldUpdateOperationsInput
  email: StringFieldUpdateOperationsInput
  dns_provider_name: StringFieldUpdateOperationsInput
  dns_provider_token: StringFieldUpdateOperationsInput
  private_key: NullableStringFieldUpdateOperationsInput
  registration_resource: DateTime
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input letsencrypt_userUpsertWithoutLetsencrypt_certificateInput {
  update: letsencrypt_userUpdateWithoutLetsencrypt_certificateInput!
  create: letsencrypt_userCreateWithoutLetsencrypt_certificateInput!
}

input letsencrypt_userUpdateOneRequiredWithoutLetsencrypt_certificateInput {
  create: letsencrypt_userCreateWithoutLetsencrypt_certificateInput
  connectOrCreate: letsencrypt_userCreateOrConnectWithoutLetsencrypt_certificateInput
  upsert: letsencrypt_userUpsertWithoutLetsencrypt_certificateInput
  connect: letsencrypt_userWhereUniqueInput
  update: letsencrypt_userUpdateWithoutLetsencrypt_certificateInput
}

input letsencrypt_certificateUpdateInput {
  common_name: StringFieldUpdateOperationsInput
  issued: NullableDateTimeFieldUpdateOperationsInput
  renewal: NullableDateTimeFieldUpdateOperationsInput
  certificate: NullableStringFieldUpdateOperationsInput
  private_key: NullableStringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  additional_domains: letsencrypt_certificateUpdateadditional_domainsInput
  letsencrypt_user: letsencrypt_userUpdateOneRequiredWithoutLetsencrypt_certificateInput
}

input letsencrypt_certificateCreateManyadditional_domainsInput {
  set: [String]!
}

input letsencrypt_certificateCreateManyInput {
  common_name: String!
  zone: String!
  issued: DateTime
  renewal: DateTime
  certificate: String
  private_key: String
  created_at: DateTime
  updated_at: DateTime
  additional_domains: letsencrypt_certificateCreateManyadditional_domainsInput
}

input letsencrypt_certificateUpdateManyMutationInput {
  common_name: StringFieldUpdateOperationsInput
  issued: NullableDateTimeFieldUpdateOperationsInput
  renewal: NullableDateTimeFieldUpdateOperationsInput
  certificate: NullableStringFieldUpdateOperationsInput
  private_key: NullableStringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  additional_domains: letsencrypt_certificateUpdateadditional_domainsInput
}

input letsencrypt_certificateCreateWithoutLetsencrypt_userInput {
  common_name: String!
  issued: DateTime
  renewal: DateTime
  certificate: String
  private_key: String
  created_at: DateTime
  updated_at: DateTime
  additional_domains: letsencrypt_certificateCreateadditional_domainsInput
}

input letsencrypt_certificateCreateOrConnectWithoutLetsencrypt_userInput {
  where: letsencrypt_certificateWhereUniqueInput!
  create: letsencrypt_certificateCreateWithoutLetsencrypt_userInput!
}

input letsencrypt_certificateCreateManyLetsencrypt_userInput {
  common_name: String!
  issued: DateTime
  renewal: DateTime
  certificate: String
  private_key: String
  created_at: DateTime
  updated_at: DateTime
  additional_domains: letsencrypt_certificateCreateManyadditional_domainsInput
}

input letsencrypt_certificateCreateManyLetsencrypt_userInputEnvelope {
  data: [letsencrypt_certificateCreateManyLetsencrypt_userInput]!
  skipDuplicates: Boolean
}

input letsencrypt_certificateCreateNestedManyWithoutLetsencrypt_userInput {
  create: letsencrypt_certificateCreateWithoutLetsencrypt_userInput
  connectOrCreate: letsencrypt_certificateCreateOrConnectWithoutLetsencrypt_userInput
  createMany: letsencrypt_certificateCreateManyLetsencrypt_userInputEnvelope
  connect: letsencrypt_certificateWhereUniqueInput
}

input letsencrypt_userCreateInput {
  zone: String!
  email: String!
  dns_provider_name: String!
  dns_provider_token: String!
  private_key: String
  registration_resource: DateTime
  created_at: DateTime
  updated_at: DateTime
  letsencrypt_certificate: letsencrypt_certificateCreateNestedManyWithoutLetsencrypt_userInput
}

input letsencrypt_certificateUpdateWithoutLetsencrypt_userInput {
  common_name: StringFieldUpdateOperationsInput
  issued: NullableDateTimeFieldUpdateOperationsInput
  renewal: NullableDateTimeFieldUpdateOperationsInput
  certificate: NullableStringFieldUpdateOperationsInput
  private_key: NullableStringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  additional_domains: letsencrypt_certificateUpdateadditional_domainsInput
}

input letsencrypt_certificateUpsertWithWhereUniqueWithoutLetsencrypt_userInput {
  where: letsencrypt_certificateWhereUniqueInput!
  update: letsencrypt_certificateUpdateWithoutLetsencrypt_userInput!
  create: letsencrypt_certificateCreateWithoutLetsencrypt_userInput!
}

input letsencrypt_certificateUpdateWithWhereUniqueWithoutLetsencrypt_userInput {
  where: letsencrypt_certificateWhereUniqueInput!
  data: letsencrypt_certificateUpdateWithoutLetsencrypt_userInput!
}

input letsencrypt_certificateScalarWhereInput {
  AND: letsencrypt_certificateScalarWhereInput
  OR: [letsencrypt_certificateScalarWhereInput]
  NOT: letsencrypt_certificateScalarWhereInput
  common_name: StringFilter
  zone: StringFilter
  additional_domains: StringNullableListFilter
  issued: DateTimeNullableFilter
  renewal: DateTimeNullableFilter
  certificate: StringNullableFilter
  private_key: StringNullableFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
}

input letsencrypt_certificateUpdateManyWithWhereWithoutLetsencrypt_userInput {
  where: letsencrypt_certificateScalarWhereInput!
  data: letsencrypt_certificateUpdateManyMutationInput!
}

input letsencrypt_certificateUpdateManyWithoutLetsencrypt_userInput {
  create: letsencrypt_certificateCreateWithoutLetsencrypt_userInput
  connectOrCreate: letsencrypt_certificateCreateOrConnectWithoutLetsencrypt_userInput
  upsert: letsencrypt_certificateUpsertWithWhereUniqueWithoutLetsencrypt_userInput
  createMany: letsencrypt_certificateCreateManyLetsencrypt_userInputEnvelope
  connect: letsencrypt_certificateWhereUniqueInput
  set: letsencrypt_certificateWhereUniqueInput
  disconnect: letsencrypt_certificateWhereUniqueInput
  delete: letsencrypt_certificateWhereUniqueInput
  update: letsencrypt_certificateUpdateWithWhereUniqueWithoutLetsencrypt_userInput
  updateMany: letsencrypt_certificateUpdateManyWithWhereWithoutLetsencrypt_userInput
  deleteMany: letsencrypt_certificateScalarWhereInput
}

input letsencrypt_userUpdateInput {
  zone: StringFieldUpdateOperationsInput
  email: StringFieldUpdateOperationsInput
  dns_provider_name: StringFieldUpdateOperationsInput
  dns_provider_token: StringFieldUpdateOperationsInput
  private_key: NullableStringFieldUpdateOperationsInput
  registration_resource: DateTime
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  letsencrypt_certificate: letsencrypt_certificateUpdateManyWithoutLetsencrypt_userInput
}

input letsencrypt_userCreateManyInput {
  zone: String!
  email: String!
  dns_provider_name: String!
  dns_provider_token: String!
  private_key: String
  registration_resource: DateTime
  created_at: DateTime
  updated_at: DateTime
}

input letsencrypt_userUpdateManyMutationInput {
  zone: StringFieldUpdateOperationsInput
  email: StringFieldUpdateOperationsInput
  dns_provider_name: StringFieldUpdateOperationsInput
  dns_provider_token: StringFieldUpdateOperationsInput
  private_key: NullableStringFieldUpdateOperationsInput
  registration_resource: DateTime
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input locksCreateInput {
  name: String!
  record_version_number: BigInt
  data: String
  owner: String
}

input NullableBigIntFieldUpdateOperationsInput {
  set: BigInt
  increment: BigInt
  decrement: BigInt
  multiply: BigInt
  divide: BigInt
}

input locksUpdateInput {
  name: StringFieldUpdateOperationsInput
  record_version_number: NullableBigIntFieldUpdateOperationsInput
  data: NullableStringFieldUpdateOperationsInput
  owner: NullableStringFieldUpdateOperationsInput
}

input locksCreateManyInput {
  name: String!
  record_version_number: BigInt
  data: String
  owner: String
}

input locksUpdateManyMutationInput {
  name: StringFieldUpdateOperationsInput
  record_version_number: NullableBigIntFieldUpdateOperationsInput
  data: NullableStringFieldUpdateOperationsInput
  owner: NullableStringFieldUpdateOperationsInput
}

input namespaceCreateInput {
  id: String
  name: String!
  created_at: DateTime
  updated_at: DateTime
  price_plan: price_planCreateNestedOneWithoutNamespaceInput
  api: apiCreateNestedManyWithoutNamespaceInput
  environment: environmentCreateNestedManyWithoutNamespaceInput
  namespace_members: namespace_membersCreateNestedManyWithoutNamespaceInput
}

input namespaceUpdateInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  price_plan: price_planUpdateOneRequiredWithoutNamespaceInput
  api: apiUpdateManyWithoutNamespaceInput
  environment: environmentUpdateManyWithoutNamespaceInput
  namespace_members: namespace_membersUpdateManyWithoutNamespaceInput
}

input namespaceCreateManyInput {
  id: String
  name: String!
  price_plan_id: Int
  created_at: DateTime
  updated_at: DateTime
}

input namespaceUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input namespace_membersCreateInput {
  membership: membership
  created_at: DateTime
  updated_at: DateTime
  namespace: namespaceCreateNestedOneWithoutNamespace_membersInput!
  users: usersCreateNestedOneWithoutNamespace_membersInput!
}

input namespace_membersUpdateInput {
  membership: membership
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  namespace: namespaceUpdateOneRequiredWithoutNamespace_membersInput
  users: usersUpdateOneRequiredWithoutNamespace_membersInput
}

input namespace_membersCreateManyInput {
  user_id: String!
  namespace_id: String!
  membership: membership
  created_at: DateTime
  updated_at: DateTime
}

input environmentCreateWithoutNodepool_environmentInput {
  id: String
  name: String!
  primary_hostname: String
  primary: Boolean
  hostnames: environmentCreatehostnamesInput
  namespace: namespaceCreateNestedOneWithoutEnvironmentInput!
  deployment_environment: deployment_environmentCreateNestedManyWithoutEnvironmentInput
  environment_edges: environment_edgesCreateNestedManyWithoutEnvironmentInput
}

input environmentCreateOrConnectWithoutNodepool_environmentInput {
  where: environmentWhereUniqueInput!
  create: environmentCreateWithoutNodepool_environmentInput!
}

input environmentCreateNestedOneWithoutNodepool_environmentInput {
  create: environmentCreateWithoutNodepool_environmentInput
  connectOrCreate: environmentCreateOrConnectWithoutNodepool_environmentInput
  connect: environmentWhereUniqueInput
}

input nodepool_environmentCreateWithoutNodepoolInput {
  environment: environmentCreateNestedOneWithoutNodepool_environmentInput!
}

input nodepool_environmentCreateOrConnectWithoutNodepoolInput {
  where: nodepool_environmentWhereUniqueInput!
  create: nodepool_environmentCreateWithoutNodepoolInput!
}

input nodepool_environmentCreateManyNodepoolInput {
  environment_id: String!
}

input nodepool_environmentCreateManyNodepoolInputEnvelope {
  data: [nodepool_environmentCreateManyNodepoolInput]!
  skipDuplicates: Boolean
}

input nodepool_environmentCreateNestedManyWithoutNodepoolInput {
  create: nodepool_environmentCreateWithoutNodepoolInput
  connectOrCreate: nodepool_environmentCreateOrConnectWithoutNodepoolInput
  createMany: nodepool_environmentCreateManyNodepoolInputEnvelope
  connect: nodepool_environmentWhereUniqueInput
}

input nodepoolCreateInput {
  id: String
  shared: Boolean
  created_at: DateTime
  updated_at: DateTime
  wundernode: wundernodeCreateNestedOneWithoutNodepoolInput!
  nodepool_environment: nodepool_environmentCreateNestedManyWithoutNodepoolInput
}

input environmentUpdateWithoutNodepool_environmentInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  primary_hostname: StringFieldUpdateOperationsInput
  primary: BoolFieldUpdateOperationsInput
  hostnames: environmentUpdatehostnamesInput
  namespace: namespaceUpdateOneRequiredWithoutEnvironmentInput
  deployment_environment: deployment_environmentUpdateManyWithoutEnvironmentInput
  environment_edges: environment_edgesUpdateManyWithoutEnvironmentInput
}

input environmentUpsertWithoutNodepool_environmentInput {
  update: environmentUpdateWithoutNodepool_environmentInput!
  create: environmentCreateWithoutNodepool_environmentInput!
}

input environmentUpdateOneRequiredWithoutNodepool_environmentInput {
  create: environmentCreateWithoutNodepool_environmentInput
  connectOrCreate: environmentCreateOrConnectWithoutNodepool_environmentInput
  upsert: environmentUpsertWithoutNodepool_environmentInput
  connect: environmentWhereUniqueInput
  update: environmentUpdateWithoutNodepool_environmentInput
}

input nodepool_environmentUpdateWithoutNodepoolInput {
  environment: environmentUpdateOneRequiredWithoutNodepool_environmentInput
}

input nodepool_environmentUpsertWithWhereUniqueWithoutNodepoolInput {
  where: nodepool_environmentWhereUniqueInput!
  update: nodepool_environmentUpdateWithoutNodepoolInput!
  create: nodepool_environmentCreateWithoutNodepoolInput!
}

input nodepool_environmentUpdateWithWhereUniqueWithoutNodepoolInput {
  where: nodepool_environmentWhereUniqueInput!
  data: nodepool_environmentUpdateWithoutNodepoolInput!
}

input nodepool_environmentUpdateManyWithWhereWithoutNodepoolInput {
  where: nodepool_environmentScalarWhereInput!
}

input nodepool_environmentUpdateManyWithoutNodepoolInput {
  create: nodepool_environmentCreateWithoutNodepoolInput
  connectOrCreate: nodepool_environmentCreateOrConnectWithoutNodepoolInput
  upsert: nodepool_environmentUpsertWithWhereUniqueWithoutNodepoolInput
  createMany: nodepool_environmentCreateManyNodepoolInputEnvelope
  connect: nodepool_environmentWhereUniqueInput
  set: nodepool_environmentWhereUniqueInput
  disconnect: nodepool_environmentWhereUniqueInput
  delete: nodepool_environmentWhereUniqueInput
  update: nodepool_environmentUpdateWithWhereUniqueWithoutNodepoolInput
  updateMany: nodepool_environmentUpdateManyWithWhereWithoutNodepoolInput
  deleteMany: nodepool_environmentScalarWhereInput
}

input nodepoolUpdateInput {
  id: StringFieldUpdateOperationsInput
  shared: BoolFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  wundernode: wundernodeUpdateOneRequiredWithoutNodepoolInput
  nodepool_environment: nodepool_environmentUpdateManyWithoutNodepoolInput
}

input nodepoolCreateManyInput {
  id: String
  wundernode_id: String!
  shared: Boolean
  created_at: DateTime
  updated_at: DateTime
}

input nodepoolUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  shared: BoolFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input nodepool_environmentCreateInput {
  environment: environmentCreateNestedOneWithoutNodepool_environmentInput!
  nodepool: nodepoolCreateNestedOneWithoutNodepool_environmentInput!
}

input nodepool_environmentUpdateInput {
  environment: environmentUpdateOneRequiredWithoutNodepool_environmentInput
  nodepool: nodepoolUpdateOneRequiredWithoutNodepool_environmentInput
}

input nodepool_environmentCreateManyInput {
  nodepool_id: String!
  environment_id: String!
}

input namespaceCreateWithoutPrice_planInput {
  id: String
  name: String!
  created_at: DateTime
  updated_at: DateTime
  api: apiCreateNestedManyWithoutNamespaceInput
  environment: environmentCreateNestedManyWithoutNamespaceInput
  namespace_members: namespace_membersCreateNestedManyWithoutNamespaceInput
}

input namespaceCreateOrConnectWithoutPrice_planInput {
  where: namespaceWhereUniqueInput!
  create: namespaceCreateWithoutPrice_planInput!
}

input namespaceCreateManyPrice_planInput {
  id: String
  name: String!
  created_at: DateTime
  updated_at: DateTime
}

input namespaceCreateManyPrice_planInputEnvelope {
  data: [namespaceCreateManyPrice_planInput]!
  skipDuplicates: Boolean
}

input namespaceCreateNestedManyWithoutPrice_planInput {
  create: namespaceCreateWithoutPrice_planInput
  connectOrCreate: namespaceCreateOrConnectWithoutPrice_planInput
  createMany: namespaceCreateManyPrice_planInputEnvelope
  connect: namespaceWhereUniqueInput
}

input price_planCreateInput {
  name: String!
  quota_daily_requests: Int!
  quota_environments: Int!
  quota_members: Int
  quota_apis: Int
  allow_secondary_environments: Boolean
  namespace: namespaceCreateNestedManyWithoutPrice_planInput
}

input namespaceUpdateWithoutPrice_planInput {
  id: StringFieldUpdateOperationsInput
  name: StringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  api: apiUpdateManyWithoutNamespaceInput
  environment: environmentUpdateManyWithoutNamespaceInput
  namespace_members: namespace_membersUpdateManyWithoutNamespaceInput
}

input namespaceUpsertWithWhereUniqueWithoutPrice_planInput {
  where: namespaceWhereUniqueInput!
  update: namespaceUpdateWithoutPrice_planInput!
  create: namespaceCreateWithoutPrice_planInput!
}

input namespaceUpdateWithWhereUniqueWithoutPrice_planInput {
  where: namespaceWhereUniqueInput!
  data: namespaceUpdateWithoutPrice_planInput!
}

input namespaceScalarWhereInput {
  AND: namespaceScalarWhereInput
  OR: [namespaceScalarWhereInput]
  NOT: namespaceScalarWhereInput
  id: StringFilter
  name: StringFilter
  price_plan_id: IntFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
}

input namespaceUpdateManyWithWhereWithoutPrice_planInput {
  where: namespaceScalarWhereInput!
  data: namespaceUpdateManyMutationInput!
}

input namespaceUpdateManyWithoutPrice_planInput {
  create: namespaceCreateWithoutPrice_planInput
  connectOrCreate: namespaceCreateOrConnectWithoutPrice_planInput
  upsert: namespaceUpsertWithWhereUniqueWithoutPrice_planInput
  createMany: namespaceCreateManyPrice_planInputEnvelope
  connect: namespaceWhereUniqueInput
  set: namespaceWhereUniqueInput
  disconnect: namespaceWhereUniqueInput
  delete: namespaceWhereUniqueInput
  update: namespaceUpdateWithWhereUniqueWithoutPrice_planInput
  updateMany: namespaceUpdateManyWithWhereWithoutPrice_planInput
  deleteMany: namespaceScalarWhereInput
}

input price_planUpdateInput {
  name: StringFieldUpdateOperationsInput
  quota_daily_requests: IntFieldUpdateOperationsInput
  quota_environments: IntFieldUpdateOperationsInput
  quota_members: IntFieldUpdateOperationsInput
  quota_apis: IntFieldUpdateOperationsInput
  allow_secondary_environments: BoolFieldUpdateOperationsInput
  namespace: namespaceUpdateManyWithoutPrice_planInput
}

input price_planCreateManyInput {
  id: Int
  name: String!
  quota_daily_requests: Int!
  quota_environments: Int!
  quota_members: Int
  quota_apis: Int
  allow_secondary_environments: Boolean
}

input price_planUpdateManyMutationInput {
  name: StringFieldUpdateOperationsInput
  quota_daily_requests: IntFieldUpdateOperationsInput
  quota_environments: IntFieldUpdateOperationsInput
  quota_members: IntFieldUpdateOperationsInput
  quota_apis: IntFieldUpdateOperationsInput
  allow_secondary_environments: BoolFieldUpdateOperationsInput
}

input usersCreateInput {
  id: String
  name: String
  email: String!
  role: user_role
  created_at: DateTime
  updated_at: DateTime
  access_token: access_tokenCreateNestedManyWithoutUsersInput
  namespace_members: namespace_membersCreateNestedManyWithoutUsersInput
}

input usersUpdateInput {
  id: StringFieldUpdateOperationsInput
  name: NullableStringFieldUpdateOperationsInput
  email: StringFieldUpdateOperationsInput
  role: user_role
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  access_token: access_tokenUpdateManyWithoutUsersInput
  namespace_members: namespace_membersUpdateManyWithoutUsersInput
}

input usersCreateManyInput {
  id: String
  name: String
  email: String!
  role: user_role
  created_at: DateTime
  updated_at: DateTime
}

input usersUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  name: NullableStringFieldUpdateOperationsInput
  email: StringFieldUpdateOperationsInput
  role: user_role
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

input nodepoolCreateWithoutWundernodeInput {
  id: String
  shared: Boolean
  created_at: DateTime
  updated_at: DateTime
  nodepool_environment: nodepool_environmentCreateNestedManyWithoutNodepoolInput
}

input nodepoolCreateOrConnectWithoutWundernodeInput {
  where: nodepoolWhereUniqueInput!
  create: nodepoolCreateWithoutWundernodeInput!
}

input nodepoolCreateManyWundernodeInput {
  id: String
  shared: Boolean
  created_at: DateTime
  updated_at: DateTime
}

input nodepoolCreateManyWundernodeInputEnvelope {
  data: [nodepoolCreateManyWundernodeInput]!
  skipDuplicates: Boolean
}

input nodepoolCreateNestedManyWithoutWundernodeInput {
  create: nodepoolCreateWithoutWundernodeInput
  connectOrCreate: nodepoolCreateOrConnectWithoutWundernodeInput
  createMany: nodepoolCreateManyWundernodeInputEnvelope
  connect: nodepoolWhereUniqueInput
}

input wundernodeCreateInput {
  id: String
  etag: String!
  config: DateTime!
  ipv4: String
  ipv6: String
  created_at: DateTime
  updated_at: DateTime
  nodepool: nodepoolCreateNestedManyWithoutWundernodeInput
}

input nodepoolUpdateWithoutWundernodeInput {
  id: StringFieldUpdateOperationsInput
  shared: BoolFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  nodepool_environment: nodepool_environmentUpdateManyWithoutNodepoolInput
}

input nodepoolUpsertWithWhereUniqueWithoutWundernodeInput {
  where: nodepoolWhereUniqueInput!
  update: nodepoolUpdateWithoutWundernodeInput!
  create: nodepoolCreateWithoutWundernodeInput!
}

input nodepoolUpdateWithWhereUniqueWithoutWundernodeInput {
  where: nodepoolWhereUniqueInput!
  data: nodepoolUpdateWithoutWundernodeInput!
}

input nodepoolScalarWhereInput {
  AND: nodepoolScalarWhereInput
  OR: [nodepoolScalarWhereInput]
  NOT: nodepoolScalarWhereInput
  id: StringFilter
  wundernode_id: StringFilter
  shared: BoolFilter
  created_at: DateTimeFilter
  updated_at: DateTimeNullableFilter
}

input nodepoolUpdateManyWithWhereWithoutWundernodeInput {
  where: nodepoolScalarWhereInput!
  data: nodepoolUpdateManyMutationInput!
}

input nodepoolUpdateManyWithoutWundernodeInput {
  create: nodepoolCreateWithoutWundernodeInput
  connectOrCreate: nodepoolCreateOrConnectWithoutWundernodeInput
  upsert: nodepoolUpsertWithWhereUniqueWithoutWundernodeInput
  createMany: nodepoolCreateManyWundernodeInputEnvelope
  connect: nodepoolWhereUniqueInput
  set: nodepoolWhereUniqueInput
  disconnect: nodepoolWhereUniqueInput
  delete: nodepoolWhereUniqueInput
  update: nodepoolUpdateWithWhereUniqueWithoutWundernodeInput
  updateMany: nodepoolUpdateManyWithWhereWithoutWundernodeInput
  deleteMany: nodepoolScalarWhereInput
}

input wundernodeUpdateInput {
  id: StringFieldUpdateOperationsInput
  etag: StringFieldUpdateOperationsInput
  config: DateTime
  ipv4: NullableStringFieldUpdateOperationsInput
  ipv6: NullableStringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
  nodepool: nodepoolUpdateManyWithoutWundernodeInput
}

input wundernodeCreateManyInput {
  id: String
  etag: String!
  config: DateTime!
  ipv4: String
  ipv6: String
  created_at: DateTime
  updated_at: DateTime
}

input wundernodeUpdateManyMutationInput {
  id: StringFieldUpdateOperationsInput
  etag: StringFieldUpdateOperationsInput
  config: DateTime
  ipv4: NullableStringFieldUpdateOperationsInput
  ipv6: NullableStringFieldUpdateOperationsInput
  created_at: DateTimeFieldUpdateOperationsInput
  updated_at: NullableDateTimeFieldUpdateOperationsInput
}

type Mutation {
  createOneaccess_token(data: access_tokenCreateInput!): access_token!
  upsertOneaccess_token(where: access_tokenWhereUniqueInput!, create: access_tokenCreateInput!, update: access_tokenUpdateInput!): access_token!
  createManyaccess_token(data: [access_tokenCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneaccess_token(where: access_tokenWhereUniqueInput!): access_token
  updateOneaccess_token(data: access_tokenUpdateInput!, where: access_tokenWhereUniqueInput!): access_token
  updateManyaccess_token(data: access_tokenUpdateManyMutationInput!, where: access_tokenWhereInput): AffectedRowsOutput!
  deleteManyaccess_token(where: access_tokenWhereInput): AffectedRowsOutput!
  createOneadmin_config(data: admin_configCreateInput!): admin_config!
  upsertOneadmin_config(where: admin_configWhereUniqueInput!, create: admin_configCreateInput!, update: admin_configUpdateInput!): admin_config!
  createManyadmin_config(data: [admin_configCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneadmin_config(where: admin_configWhereUniqueInput!): admin_config
  updateOneadmin_config(data: admin_configUpdateInput!, where: admin_configWhereUniqueInput!): admin_config
  updateManyadmin_config(data: admin_configUpdateManyMutationInput!, where: admin_configWhereInput): AffectedRowsOutput!
  deleteManyadmin_config(where: admin_configWhereInput): AffectedRowsOutput!
  createOneapi(data: apiCreateInput!): api!
  upsertOneapi(where: apiWhereUniqueInput!, create: apiCreateInput!, update: apiUpdateInput!): api!
  createManyapi(data: [apiCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneapi(where: apiWhereUniqueInput!): api
  updateOneapi(data: apiUpdateInput!, where: apiWhereUniqueInput!): api
  updateManyapi(data: apiUpdateManyMutationInput!, where: apiWhereInput): AffectedRowsOutput!
  deleteManyapi(where: apiWhereInput): AffectedRowsOutput!
  createOnedeployment(data: deploymentCreateInput!): deployment!
  upsertOnedeployment(where: deploymentWhereUniqueInput!, create: deploymentCreateInput!, update: deploymentUpdateInput!): deployment!
  createManydeployment(data: [deploymentCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOnedeployment(where: deploymentWhereUniqueInput!): deployment
  updateOnedeployment(data: deploymentUpdateInput!, where: deploymentWhereUniqueInput!): deployment
  updateManydeployment(data: deploymentUpdateManyMutationInput!, where: deploymentWhereInput): AffectedRowsOutput!
  deleteManydeployment(where: deploymentWhereInput): AffectedRowsOutput!
  createOnedeployment_environment(data: deployment_environmentCreateInput!): deployment_environment!
  upsertOnedeployment_environment(where: deployment_environmentWhereUniqueInput!, create: deployment_environmentCreateInput!, update: deployment_environmentUpdateInput!): deployment_environment!
  createManydeployment_environment(data: [deployment_environmentCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOnedeployment_environment(where: deployment_environmentWhereUniqueInput!): deployment_environment
  updateOnedeployment_environment(data: deployment_environmentUpdateInput!, where: deployment_environmentWhereUniqueInput!): deployment_environment
  updateManydeployment_environment(where: deployment_environmentWhereInput): AffectedRowsOutput!
  deleteManydeployment_environment(where: deployment_environmentWhereInput): AffectedRowsOutput!
  createOneedge(data: edgeCreateInput!): edge!
  upsertOneedge(where: edgeWhereUniqueInput!, create: edgeCreateInput!, update: edgeUpdateInput!): edge!
  createManyedge(data: [edgeCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneedge(where: edgeWhereUniqueInput!): edge
  updateOneedge(data: edgeUpdateInput!, where: edgeWhereUniqueInput!): edge
  updateManyedge(data: edgeUpdateManyMutationInput!, where: edgeWhereInput): AffectedRowsOutput!
  deleteManyedge(where: edgeWhereInput): AffectedRowsOutput!
  createOneenvironment(data: environmentCreateInput!): environment!
  upsertOneenvironment(where: environmentWhereUniqueInput!, create: environmentCreateInput!, update: environmentUpdateInput!): environment!
  createManyenvironment(data: [environmentCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneenvironment(where: environmentWhereUniqueInput!): environment
  updateOneenvironment(data: environmentUpdateInput!, where: environmentWhereUniqueInput!): environment
  updateManyenvironment(data: environmentUpdateManyMutationInput!, where: environmentWhereInput): AffectedRowsOutput!
  deleteManyenvironment(where: environmentWhereInput): AffectedRowsOutput!
  createOneenvironment_edges(data: environment_edgesCreateInput!): environment_edges!
  upsertOneenvironment_edges(where: environment_edgesWhereUniqueInput!, create: environment_edgesCreateInput!, update: environment_edgesUpdateInput!): environment_edges!
  createManyenvironment_edges(data: [environment_edgesCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneenvironment_edges(where: environment_edgesWhereUniqueInput!): environment_edges
  updateOneenvironment_edges(data: environment_edgesUpdateInput!, where: environment_edgesWhereUniqueInput!): environment_edges
  updateManyenvironment_edges(where: environment_edgesWhereInput): AffectedRowsOutput!
  deleteManyenvironment_edges(where: environment_edgesWhereInput): AffectedRowsOutput!
  createOneletsencrypt_certificate(data: letsencrypt_certificateCreateInput!): letsencrypt_certificate!
  upsertOneletsencrypt_certificate(where: letsencrypt_certificateWhereUniqueInput!, create: letsencrypt_certificateCreateInput!, update: letsencrypt_certificateUpdateInput!): letsencrypt_certificate!
  createManyletsencrypt_certificate(data: [letsencrypt_certificateCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneletsencrypt_certificate(where: letsencrypt_certificateWhereUniqueInput!): letsencrypt_certificate
  updateOneletsencrypt_certificate(data: letsencrypt_certificateUpdateInput!, where: letsencrypt_certificateWhereUniqueInput!): letsencrypt_certificate
  updateManyletsencrypt_certificate(data: letsencrypt_certificateUpdateManyMutationInput!, where: letsencrypt_certificateWhereInput): AffectedRowsOutput!
  deleteManyletsencrypt_certificate(where: letsencrypt_certificateWhereInput): AffectedRowsOutput!
  createOneletsencrypt_user(data: letsencrypt_userCreateInput!): letsencrypt_user!
  upsertOneletsencrypt_user(where: letsencrypt_userWhereUniqueInput!, create: letsencrypt_userCreateInput!, update: letsencrypt_userUpdateInput!): letsencrypt_user!
  createManyletsencrypt_user(data: [letsencrypt_userCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneletsencrypt_user(where: letsencrypt_userWhereUniqueInput!): letsencrypt_user
  updateOneletsencrypt_user(data: letsencrypt_userUpdateInput!, where: letsencrypt_userWhereUniqueInput!): letsencrypt_user
  updateManyletsencrypt_user(data: letsencrypt_userUpdateManyMutationInput!, where: letsencrypt_userWhereInput): AffectedRowsOutput!
  deleteManyletsencrypt_user(where: letsencrypt_userWhereInput): AffectedRowsOutput!
  createOnelocks(data: locksCreateInput!): locks!
  upsertOnelocks(where: locksWhereUniqueInput!, create: locksCreateInput!, update: locksUpdateInput!): locks!
  createManylocks(data: [locksCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOnelocks(where: locksWhereUniqueInput!): locks
  updateOnelocks(data: locksUpdateInput!, where: locksWhereUniqueInput!): locks
  updateManylocks(data: locksUpdateManyMutationInput!, where: locksWhereInput): AffectedRowsOutput!
  deleteManylocks(where: locksWhereInput): AffectedRowsOutput!
  createOnenamespace(data: namespaceCreateInput!): namespace!
  upsertOnenamespace(where: namespaceWhereUniqueInput!, create: namespaceCreateInput!, update: namespaceUpdateInput!): namespace!
  createManynamespace(data: [namespaceCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOnenamespace(where: namespaceWhereUniqueInput!): namespace
  updateOnenamespace(data: namespaceUpdateInput!, where: namespaceWhereUniqueInput!): namespace
  updateManynamespace(data: namespaceUpdateManyMutationInput!, where: namespaceWhereInput): AffectedRowsOutput!
  deleteManynamespace(where: namespaceWhereInput): AffectedRowsOutput!
  createOnenamespace_members(data: namespace_membersCreateInput!): namespace_members!
  upsertOnenamespace_members(where: namespace_membersWhereUniqueInput!, create: namespace_membersCreateInput!, update: namespace_membersUpdateInput!): namespace_members!
  createManynamespace_members(data: [namespace_membersCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOnenamespace_members(where: namespace_membersWhereUniqueInput!): namespace_members
  updateOnenamespace_members(data: namespace_membersUpdateInput!, where: namespace_membersWhereUniqueInput!): namespace_members
  updateManynamespace_members(data: namespace_membersUpdateManyMutationInput!, where: namespace_membersWhereInput): AffectedRowsOutput!
  deleteManynamespace_members(where: namespace_membersWhereInput): AffectedRowsOutput!
  createOnenodepool(data: nodepoolCreateInput!): nodepool!
  upsertOnenodepool(where: nodepoolWhereUniqueInput!, create: nodepoolCreateInput!, update: nodepoolUpdateInput!): nodepool!
  createManynodepool(data: [nodepoolCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOnenodepool(where: nodepoolWhereUniqueInput!): nodepool
  updateOnenodepool(data: nodepoolUpdateInput!, where: nodepoolWhereUniqueInput!): nodepool
  updateManynodepool(data: nodepoolUpdateManyMutationInput!, where: nodepoolWhereInput): AffectedRowsOutput!
  deleteManynodepool(where: nodepoolWhereInput): AffectedRowsOutput!
  createOnenodepool_environment(data: nodepool_environmentCreateInput!): nodepool_environment!
  upsertOnenodepool_environment(where: nodepool_environmentWhereUniqueInput!, create: nodepool_environmentCreateInput!, update: nodepool_environmentUpdateInput!): nodepool_environment!
  createManynodepool_environment(data: [nodepool_environmentCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOnenodepool_environment(where: nodepool_environmentWhereUniqueInput!): nodepool_environment
  updateOnenodepool_environment(data: nodepool_environmentUpdateInput!, where: nodepool_environmentWhereUniqueInput!): nodepool_environment
  updateManynodepool_environment(where: nodepool_environmentWhereInput): AffectedRowsOutput!
  deleteManynodepool_environment(where: nodepool_environmentWhereInput): AffectedRowsOutput!
  createOneprice_plan(data: price_planCreateInput!): price_plan!
  upsertOneprice_plan(where: price_planWhereUniqueInput!, create: price_planCreateInput!, update: price_planUpdateInput!): price_plan!
  createManyprice_plan(data: [price_planCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneprice_plan(where: price_planWhereUniqueInput!): price_plan
  updateOneprice_plan(data: price_planUpdateInput!, where: price_planWhereUniqueInput!): price_plan
  updateManyprice_plan(data: price_planUpdateManyMutationInput!, where: price_planWhereInput): AffectedRowsOutput!
  deleteManyprice_plan(where: price_planWhereInput): AffectedRowsOutput!
  createOneusers(data: usersCreateInput!): users!
  upsertOneusers(where: usersWhereUniqueInput!, create: usersCreateInput!, update: usersUpdateInput!): users!
  createManyusers(data: [usersCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOneusers(where: usersWhereUniqueInput!): users
  updateOneusers(data: usersUpdateInput!, where: usersWhereUniqueInput!): users
  updateManyusers(data: usersUpdateManyMutationInput!, where: usersWhereInput): AffectedRowsOutput!
  deleteManyusers(where: usersWhereInput): AffectedRowsOutput!
  createOnewundernode(data: wundernodeCreateInput!): wundernode!
  upsertOnewundernode(where: wundernodeWhereUniqueInput!, create: wundernodeCreateInput!, update: wundernodeUpdateInput!): wundernode!
  createManywundernode(data: [wundernodeCreateManyInput]!, skipDuplicates: Boolean): AffectedRowsOutput!
  deleteOnewundernode(where: wundernodeWhereUniqueInput!): wundernode
  updateOnewundernode(data: wundernodeUpdateInput!, where: wundernodeWhereUniqueInput!): wundernode
  updateManywundernode(data: wundernodeUpdateManyMutationInput!, where: wundernodeWhereInput): AffectedRowsOutput!
  deleteManywundernode(where: wundernodeWhereInput): AffectedRowsOutput!
}

scalar DateTime

scalar Json

scalar UUID

scalar BigInt

type Post {
  id: Int
  userId: Int
  title: String
  body: String
}

type Comment {
  id: Int
  name: String
  email: String
  body: String
  postId: Int
}

type User {
  id: Int
  name: String
  username: String
  email: String
  address: Address
  phone: String
  website: String
  company: Company
}

type Address {
  street: String
  suite: String
  city: String
  zipcode: String
  geo: Geo
}

type Geo {
  lat: String
  lng: String
}

type Company {
  name: String
  catchPhrase: String
  bs: String
}
`

const nexusSchema = `

schema {
  query: Query
  mutation: Mutation
}

scalar ID
scalar Float
scalar String
scalar Boolean
scalar Int

type Mutation {
    postPasswordlessStart(postPasswordlessStartInput: postPasswordlessStartInput): PostPasswordlessStartResponse
    postPasswordlessLogin(postPasswordlessLoginInput: postPasswordlessLoginInput): PostPasswordlessLoginResponse
    postJwtRefresh(postJwtRefreshInput: postJwtRefreshInput): PostJwtRefreshResponse
    acceptPoolInvite(poolId: String!): Boolean!
    addCartItem(item: AddCartItemInput!): Cart!
    addTicketToPool(poolId: String!, ticketId: String!): Boolean!
    archiveAggTicket(archived: Boolean!, id: ID!): AggTicket!
    cancelOrder(input: CancelOrderInput!): Order!
    createCancelOrderTask(input: CreateCancelOrderTaskInput!): CancelOrderTask!
    createLocationGame(locationGame: CreateLocationGameInput!): LocationGame!
    createPool(pool: CreatePoolInput!): Pool!
    createRecurringOrder(recurringOrder: CreateRecurringOrderInput!): RecurringOrder!
    createRegionGame(regionGame: CreateRegionGameInput!): RegionGame!
    createRegionGameDraw(regionGameDraw: CreateRegionGameDrawInput!): RegionGameDraw!
    deleteLocationGame(id: ID!): Boolean!
    deletePool(id: ID!): Boolean!
    deleteRecurringOrder(id: ID!): Boolean!
    deleteRegionGame(id: ID!): Boolean!
    emptyCart: Cart!
    expressCheckout: Order!
    generateFreeTicket(freeTicket: GenerateFreeTicketInput!): [Ticket]!
    inviteToPool(poolId: String!, userId: String!): PoolInvite!
    leavePool(poolId: String!): Boolean!
    ledgerTransfer(options: TransferOptions, requests: [TransferRequest!]!): [LedgerTransferResponse!]!
    markTaskComplete(input: MarkTaskCompleteInput!): Task!
    markTaskFailed(input: MarkTaskFailedInput!): Task!
    registerDevice(device: RegisterDeviceInput!): Device!
    rejectPoolInvite(poolId: String!): Boolean!
    removeCartItem(index: Int!): Cart!
    removeTicketFromPool(ticketId: String!): Boolean!
    sendReceiptDuplicate(orderId: String!): Boolean!
    startWinningsProcess(input: StartWinningsProcessInput!): StepFunctionsExecution!
    unregisterDevice(device: UnregisterDeviceInput!): Boolean!
    updateBigWinningTask(bigWinningTask: UpdateBigWinningTaskInput!): BigWinningTask!
    updateCancelOrderTask(input: UpdateCancelOrderTaskInput!): CancelOrderTask!
    updateLocationGame(locationGame: UpdateLocationGameInput!): LocationGame!
    updatePool(pool: UpdatePoolInput!): Pool!
    updatePricingRule(pricingRule: UpdatePricingRuleInput!): PricingRule!
    updateProfile(profile: UpdateProfileInput): User!
    updateRecurringOrder(recurringOrder: UpdateRecurringOrderInput!): RecurringOrder!
    updateRegionGame(regionGame: UpdateRegionGameInput!): RegionGame!
    updateRegionGameDraw(regionGameDraw: UpdateRegionGameDrawInput!): RegionGameDraw!
    validateBigWinningNotificationTask(id: ID!): BigWinningNotificationTask!
}

union PostPasswordlessStartResponse = UnspecifiedHttpResponse | PostPasswordlessStartOK | PostPasswordlessStartBadRequest | PostPasswordlessStartNoAuthProvided | PostPasswordlessStartUserNotFound | PostPasswordlessStartInternalError

type UnspecifiedHttpResponse {
    statusCode: Int!
}

type PostPasswordlessStartOK {
    code: String
}

type PostPasswordlessStartBadRequest {
    message: String
}

type PostPasswordlessStartNoAuthProvided {
    message: String
}

type PostPasswordlessStartUserNotFound {
    message: String
}

type PostPasswordlessStartInternalError {
    message: String
}

input postPasswordlessStartInput {
    applicationId: String!
    loginId: String!
}

union PostPasswordlessLoginResponse = UnspecifiedHttpResponse | PostPasswordlessLoginOK | PostPasswordlessLoginNotRegisteredForApp | PostPasswordlessLoginPasswordChangeRequested | PostPasswordlessLoginEmailNotVerified | PostPasswordlessLoginRegistrationNotVerified | PostPasswordlessLoginTwoFactorEnabled | PostPasswordlessLoginBadRequest | PostPasswordlessLoginInternalError

type PostPasswordlessLoginOK {
    refreshToken: String
    token: String
    user: User
}

type NexusUser {
    username: String
    verified: Boolean
    firstName: String
    lastName: String
    email: String
    mobilePhone: String
    timezone: String
}

type PostPasswordlessLoginNotRegisteredForApp {
    message: String
}

type PostPasswordlessLoginPasswordChangeRequested {
    changePasswordReason: String
}

type PostPasswordlessLoginEmailNotVerified {
    message: String
}

type PostPasswordlessLoginRegistrationNotVerified {
    message: String
}

type PostPasswordlessLoginTwoFactorEnabled {
    twoFactorId: String
}

type PostPasswordlessLoginBadRequest {
    message: String
}

type PostPasswordlessLoginInternalError {
    message: String
}

input postPasswordlessLoginInput {
    code: String!
    ipAddress: String
    metaData: MetaDataInput
}

input MetaDataInput {
    device: DeviceInput
}

input DeviceInput {
    name: String
}

union PostJwtRefreshResponse = UnspecifiedHttpResponse | PostJwtRefreshOK | PostJwtRefreshBadRequest | PostJwtRefreshNoAuthProvided | PostJwtRefreshTokenNotFound | PostJwtRefreshInternalError

type PostJwtRefreshOK {
    refreshToken: String
    token: String
}

type PostJwtRefreshBadRequest {
    message: String
}

type PostJwtRefreshNoAuthProvided {
    message: String
}

type PostJwtRefreshTokenNotFound {
    message: String
}

type PostJwtRefreshInternalError {
    message: String
}

input postJwtRefreshInput {
    refreshToken: String
    token: String
}

scalar AWSDate

scalar AWSDateTime

scalar AWSJSON

scalar AWSTime

scalar AWSEmail

type AggTicket {
    archived: Boolean!
    draw: RegionGameDraw!
    drawDate: AWSDate!
    game: RegionGame!
    id: ID!
    regionGameId: String!
    tickets: [Ticket!]!
    userId: String!
}

type AggTicketsResult {
    items: [AggTicket!]!
    nextToken: String
}

type BigWinningNotificationTask {
    drawDate: AWSDate!
    id: ID!
    regionGameId: String!
    status: BigWinningTaskStatus!
}

type BigWinningNotificationTasksResult {
    items: [BigWinningNotificationTask!]
    nextToken: String
}

type BigWinningTask {
    drawDate: AWSDate!
    id: ID!
    regionGameId: String!
    status: BigWinningTaskStatus!
}

type BigWinningTasksResult {
    items: [BigWinningTask!]
    nextToken: String
}

type CancelOrderTask {
    createdAt: AWSDateTime!
    id: ID!
    orderId: String!
    status: CancelOrderTaskStatus!
    userId: String!
}

type CancelOrderTasksResult {
    items: [CancelOrderTask!]
    nextToken: String
}

type Cart {
    id: ID!
    items: [CartItem!]!
    serviceFee: Price!
    total: Price!
    userId: String!
}

type CartItem {
    drawDate: AWSDate!
    fractional: Boolean!
    play: Play!
    price: Price!
    quantity: Int!
    regionGameId: String!
}

type Currency {
    code: String!
}

type Device {
    deviceId: ID!
    provider: PushNotificationProvider!
    token: String!
}

type DrawResults {
    prizes: AWSJSON
    result: AWSJSON
}

type FreeTicket {
    drawDate: AWSDate!
    generatedTicketId: String
    id: ID!
    regionGameId: String!
    status: String!
}

type FreeTicketsResult {
    items: [FreeTicket!]!
    nextToken: String
}

type GameSchemas {
    play: AWSJSON
    prizes: AWSJSON
    result: AWSJSON
}

type Ledger {
    balance: Price
    id: ID!
    transactions: [LedgerTransaction!]!
    type: LedgerType
}

type LedgerTransaction {
    amount: Price!
    createdAt: AWSDateTime!
    description: String
    id: ID!
    ledgerId: String!
    reference: String!
    relatedTransactionId: String!
}

type LedgerTransactionsResult {
    items: [LedgerTransaction!]!
    nextToken: String
}

type LedgerTransferResponse {
    amount: Price!
    description: String
    destinationLedgerId: String!
    destinationTransactionId: String!
    reference: String!
    sourceLedgerId: String!
    sourceTransactionId: String!
}

type LedgersResult {
    items: [Ledger!]!
    nextToken: String
}

type LocationGame {
    enabled: Boolean!
    fractions: Int
    game: RegionGame!
    id: ID!
}

type LocationGamesResult {
    items: [LocationGame!]!
    nextToken: String
}

type Order {
    createdAt: AWSDateTime
    fulfilledAt: AWSDateTime
    id: ID!
    isCanceled: Boolean!
    items: [OrderItem!]!
    locationId: String
    refundAmount: Price
    refundDestination: RefundDestinationEnum
    serviceFee: Price!
    status: OrderStatus!
    submittedAt: AWSDateTime
    total: Price!
}

type OrderItem {
    cancelAction: ActionEnum
    fractional: Boolean!
    id: ID!
    play: Play!
    price: Price!
    quantity: Int!
    regionGameId: String!
    ticketId: String
}

type OrdersResult {
    items: [Order!]!
    nextToken: String
}

type Play {
    options: AWSJSON
    pick: [String!]!
}

type Pool {
    id: ID!
    name: String!
    userCount: Int!
}

type PoolInvite {
    status: PoolInviteStatus!
    user: User!
    userId: String!
}

type PoolInvitesResult {
    items: [PoolInvite!]!
    nextToken: String
}

type PoolUser {
    joinedAt: AWSDateTime!
    user: User!
}

type PoolUsersResult {
    items: [PoolUser!]!
    nextToken: String
}

type PoolsResult {
    items: [Pool!]!
    nextToken: String
}

type PreNotifications {
    email: Boolean
    push: Boolean
}

type Price {
    amount: Float!
    currency: Currency!
}

type PricingRule {
    actor: String
    id: String!
    latest: Int
    rules: AWSJSON!
    type: PricingRuleType!
    version: Int!
}

type PricingRulesResult {
    items: [PricingRule!]!
    nextToken: String
}

type Query {
    aggTicket(id: ID!): AggTicket!
    aggTickets(filters: AggTicketsFilters, pagination: Pagination): AggTicketsResult!
    bigWinningNotificationTask(id: ID!): BigWinningNotificationTask!
    bigWinningNotificationTasks(filters: BigWinningNotificationTasksFilters!, pagination: Pagination): BigWinningNotificationTasksResult!
    bigWinningTask(id: ID): BigWinningTask!
    bigWinningTasks(filters: BigWinningsTaskFilters!, pagination: Pagination): BigWinningTasksResult!
    cancelOrderTask(id: ID!): CancelOrderTask!
    cancelOrderTasks(filters: CancelOrderTasksFilters!, pagination: Pagination): CancelOrderTasksResult!
    cart(userId: ID): Cart!
    freeTicket(id: ID!): FreeTicket!
    freeTickets(filters: FreeTicketsFilters!, pagination: Pagination): FreeTicketsResult!
    ledger(id: ID!): Ledger!
    ledgerTransaction(ledgerId: ID!, transactionId: String!): LedgerTransaction
    ledgerTransactions(filters: LedgerTransactionsFilters, ledgerId: ID!, pagination: Pagination): LedgerTransactionsResult!
    ledgers: LedgersResult!
    locationGame(id: ID!): LocationGame!
    locationGames(filters: LocationGamesFilters!, pagination: Pagination): LocationGamesResult!
    order(id: ID!): Order!
    orders(filters: OrderFilters, pagination: Pagination): OrdersResult!
    pool(id: ID!): Pool
    poolInvites(id: ID!, pagination: Pagination): PoolInvitesResult!
    poolUsers(id: ID!, pagination: Pagination): PoolUsersResult!
    pools(pagination: Pagination): PoolsResult!
    pricingRule(id: ID!): PricingRule
    pricingRules(pagination: Pagination, type: ID!): PricingRulesResult!
    profile: User!
    quoteRegionGame(fractional: Boolean!, play: AWSJSON!, regionGameId: ID!): Price!
    recurringOrder(id: ID!): RecurringOrder!
    recurringOrders(pagination: Pagination): RecurringOrdersResult!
    regionGame(id: ID!): RegionGame!
    regionGameDraw(id: ID!): RegionGameDraw!
    regionGameDraws(filters: RegionGameDrawsFilters!, pagination: Pagination): RegionGameDrawsResult!
    regionGames(filters: RegionGamesFilters!, pagination: Pagination): RegionGamesResult!
    task(id: ID): Task!
    tasks(filters: TaskFilters): TasksResult!
    ticket(id: ID!): Ticket!
    tickets(filters: TicketsFilters, pagination: Pagination): TicketsResult!
}

type RecurringOrder {
    enabled: Boolean!
    expectedPrice: Price!
    fractional: Boolean!
    id: ID!
    locationId: String!
    play: Play!
    regionGameId: String!
}

type RecurringOrdersResult {
    items: [RecurringOrder!]!
    nextToken: String
}

type RegionGame {
    autoPayoutLimit: Price
    closingTime: Int!
    currency: String!
    drawTime: AWSTime!
    draws: [RegionGameDraw!]
    gameId: String!
    id: ID!
    lastDrawDate: AWSDate
    lastDrawResult: String
    name: String!
    nextDrawDate: AWSDate
    nextDrawPrize: Float
    regionId: String!
    regionName: String!
    resultUpdatedAt: AWSDateTime
    schemas: GameSchemas
    timeZone: String!
}

type RegionGameDraw {
    closingDateTime: AWSDateTime
    date: AWSDate!
    id: ID!
    parsedResult: DrawResults
    prize: Float
    regionGameId: String!
    result: String
    resultUpdatedAt: AWSDateTime
    verifiedResult: DrawResults
}

type RegionGameDrawsResult {
    items: [RegionGameDraw!]
    nextToken: String
}

type RegionGamesResult {
    items: [RegionGame!]
    nextToken: String
}

type StepFunctionsExecution {
    executionArn: String!
    startDate: Float!
}

type Task {
    execution: String
    id: String!
    input: AWSJSON
    output: AWSJSON
    process: TaskProcess!
    state: TaskState!
    status: TaskStatus!
    statusReason: String
    statusUpdatedAt: AWSDateTime
    token: String
}

type TasksResult {
    items: [Task!]
    nextToken: String
}

type Ticket {
    drawDate: AWSDate!
    fraction: Int
    id: ID!
    locationId: String
    options: AWSJSON
    pick: AWSJSON!
    poolId: String
    regionGameId: String!
    totalFractions: Int
    totalWinnings: Price
    type: String!
    winnings: Price
}

type TicketsResult {
    items: [Ticket!]!
    nextToken: String
}

type User {
    email: AWSEmail!
    id: ID!
    name: String!
    preferences: UserPreferences!
    updatedAt: AWSDateTime!
}

type UserPreferences {
    notifications: PreNotifications!
}

enum ActionEnum {
    Keep
    Void
}

enum BigWinningNotificationTaskStatus {
    Complete
    Pending
}

enum BigWinningTaskStatus {
    Complete
    Pending
}

enum CancelOrderTaskStatus {
    Complete
    Pending
}

enum FreeTicketStatus {
    Complete
    Pending
}

enum LedgerType {
    Balance
    Cash
    Credits
    Winnings
}

enum OrderStatus {
    Canceled
    Draft
    Fulfilled
    Paid
    PaymentFailed
    PendingPayment
}

enum PoolInviteStatus {
    Accepted
    Pending
    Rejected
}

enum PricingRuleType {
    CART
    GAME
}

enum PushNotificationProvider {
    FCM
}

enum RefundDestinationEnum {
    Balance
    Credits
    Exact
    PaymentMethod
}

enum TaskProcess {
    Winnings
}

enum TaskState {
    AddPaymentMethod
    BigWinner
    CalculateWinnings
    FulfillOrder
    IssueWinnings
    PreCalculateWinnings
    PreIssueWinnings
    PreVerifyResults
    ProcessPayment
    SendReceipt
    SendResults
    VerifyResults
}

enum TaskStatus {
    Complete
    Failed
    Pending
}

input AddCartItemInput {
    drawDate: AWSDate!
    fractional: Boolean!
    play: PlayInput
    quantity: Int!
    regionGameId: String!
}

input AggTicketsFilters {
    archived: Boolean
    fromDate: AWSDate
    regionGameId: String
    toDate: AWSDate
}

input BigWinningNotificationTasksFilters {
    regionGameDrawId: String!
    status: BigWinningNotificationTaskStatus
}

input BigWinningsTaskFilters {
    regionGameDrawId: String!
    status: BigWinningTaskStatus
}

input CancelItemsInput {
    action: ActionEnum!
    id: ID!
}

input CancelOrderInput {
    action: ActionEnum!
    items: [CancelItemsInput!]
    orderId: ID!
    refundAmount: PriceInput
    refundDestination: RefundDestinationEnum
}

input CancelOrderTasksFilters {
    fromDate: AWSDateTime
    status: CancelOrderTaskStatus!
    toDate: AWSDateTime
    userId: String
}

input CreateCancelOrderTaskInput {
    orderId: String!
}

input CreateLocationGameInput {
    enabled: Boolean!
    fractions: Int
    gameId: String!
    locationId: String!
    regionId: String!
}

input CreatePoolInput {
    name: String!
}

input CreateRecurringOrderInput {
    fractional: Boolean!
    play: PlayInput!
    regionGameId: String!
}

input CreateRegionGameDrawInput {
    closingDateTime: AWSDateTime
    date: AWSDate!
    regionGameId: String!
    result: String
    verifiedResult: RegionGameDrawResultInput
}

input CreateRegionGameInput {
    autoPayoutLimit: PriceInput
    closingTime: Int!
    currency: String!
    drawTime: AWSTime!
    gameId: ID!
    lastDrawDate: AWSDate
    lastDrawResult: String
    name: String!
    nextDrawDate: AWSDate
    nextDrawPrize: Float
    prizes: AWSJSON
    regionId: ID!
    regionName: String!
    resultUpdatedAt: AWSDateTime
    timeZone: String!
}

input CurrencyInput {
    code: String!
}

input FreeTicketsFilters {
    regionGameDrawId: String!
    status: FreeTicketStatus
}

input FulfilledItem {
    id: ID!
    ticketId: String
}

input GenerateFreeTicketInput {
    drawDate: AWSDate!
    id: String!
    play: PlayInput!
}

input LedgerTransactionsFilters {
    fromDate: AWSDateTime
    toDate: AWSDateTime
}

input LocationGamesFilters {
    locationId: String!
}

input MarkTaskCompleteInput {
    id: ID!
}

input MarkTaskFailedInput {
    id: ID!
    reason: String
}

input OrderFilters {
    fromDate: AWSDateTime
    status: OrderStatus
    toDate: AWSDateTime
    userId: String
}

input Pagination {
    limit: Int
    nextToken: String
}

input PlayInput {
    options: AWSJSON
    pick: [String!]!
}

input PreNotificationsInput {
    email: Boolean
    push: Boolean
}

input PriceInput {
    amount: Float!
    currency: CurrencyInput!
}

input RegionGameDrawResultInput {
    prizes: AWSJSON!
    result: AWSJSON!
}

input RegionGameDrawsFilters {
    regionGameId: String!
}

input RegionGamesFilters {
    regionId: String!
}

input RegisterDeviceInput {
    deviceId: ID!
    provider: PushNotificationProvider!
    token: String!
}

input StartWinningsProcessInput {
    date: String!
    regionGameId: String!
}

input TaskFilters {
    process: TaskProcess
    state: TaskState
    status: TaskStatus
}

input TicketsFilters {
    fromDate: AWSDate
    regionGameId: String
    toDate: AWSDate
}

input TransferOptions {
    idempotencyKey: String
}

input TransferRequest {
    amount: PriceInput!
    description: String
    destinationLedgerId: String!
    reference: String!
    sourceLedgerId: String!
}

input UnregisterDeviceInput {
    deviceId: ID!
    provider: PushNotificationProvider!
}

input UpdateBigWinningTaskInput {
    id: ID!
    status: BigWinningTaskStatus!
}

input UpdateCancelOrderTaskInput {
    id: ID!
    status: CancelOrderTaskStatus!
}

input UpdateLocationGameInput {
    enabled: Boolean!
    fractions: Int
    id: ID!
    regionId: String!
}

input UpdatePoolInput {
    id: ID!
    name: String
}

input UpdatePricingRuleInput {
    latest: Int!
    rules: AWSJSON!
    type: PricingRuleType!
}

input UpdateProfileInput {
    email: AWSEmail
    name: String
    preferences: UserPreferencesInput
}

input UpdateRecurringOrderInput {
    enabled: Boolean!
    fractional: Boolean!
    id: ID!
    play: PlayInput!
    regionGameId: String!
}

input UpdateRegionGameDrawInput {
    closingDateTime: AWSDateTime
    id: ID!
    result: String
    verifiedResult: RegionGameDrawResultInput
}

input UpdateRegionGameInput {
    autoPayoutLimit: PriceInput
    closingTime: Int
    currency: String
    drawTime: AWSTime
    id: ID!
    lastDrawDate: AWSDate
    lastDrawResult: String
    name: String
    nextDrawDate: AWSDate
    nextDrawPrize: Float
    prizes: AWSJSON
    regionName: String
    resultUpdatedAt: AWSDateTime
    timeZone: String
}

input UserPreferencesInput {
    notifications: PreNotificationsInput!
}`
