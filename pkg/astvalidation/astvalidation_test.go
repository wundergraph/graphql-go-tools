package astvalidation

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"testing"
)

func TestExecutionValidation(t *testing.T) {

	must := func(err error) {
		if report, ok := err.(operationreport.Report); ok {
			if report.HasErrors() {
				panic(report.Error())
			}
			return
		}
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

	mustString := func(str string, err error) string {
		must(err)
		return str
	}

	runWithDefinition := func(definitionInput, operationInput string, rule Rule, expectation ValidationState, expectFailedNormalization ...bool) {

		definition := mustDocument(astparser.ParseGraphqlDocumentString(definitionInput))
		operation := mustDocument(astparser.ParseGraphqlDocumentString(operationInput))
		report := operationreport.Report{}

		astnormalization.NormalizeOperation(&operation, &definition, &report)
		if report.HasErrors() {
			if (len(expectFailedNormalization) == 1 && !expectFailedNormalization[0]) || len(expectFailedNormalization) == 0 {
				panic(report.Error())
			}
		}

		validator := &OperationValidator{}
		validator.RegisterRule(rule)

		result := validator.Validate(&operation, &definition, &report)

		printedOperation := mustString(astprinter.PrintString(&operation, &definition))

		if expectation != result {
			panic(fmt.Errorf("want expectation: %s, got: %s\nreason: %v\noperation:\n%s\n", expectation, result, report.Error(), printedOperation))
		}
	}

	run := func(operationInput string, rule Rule, expectation ValidationState, expectFailedNormalization ...bool) {
		runWithDefinition(testDefinition, operationInput, rule, expectation, expectFailedNormalization...)
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
					FieldSelections(), Invalid, true)
			})
			t.Run("104 variant", func(t *testing.T) {
				run(`
							{
								dog {
									barkVolume: kawVolume
								}
							}`,
					FieldSelections(), Invalid, true)
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
					FieldSelections(), Invalid, true)
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
				run(`
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
				run(`
							fragment inDirectFieldSelectionOnUnion on CatOrDog {
								__typename
	  							... on Pet {
	    							name
	  							}
	  							... {
	    							x
	  							}
							}`,
					FieldSelections(), Invalid, true)
			})
			t.Run("106", func(t *testing.T) {
				run(`
							fragment directFieldSelectionOnUnion on CatOrDog {
								name
								barkVolume
							}`,
					FieldSelections(), Invalid, true)
			})
			t.Run("106 variant", func(t *testing.T) {
				run(`
							fragment directFieldSelectionOnUnion on Cat {
								name {
									name
								}
							}`,
					FieldSelections(), Invalid, true)
			})
		})
		t.Run("5.3.2 Field Selection Merging", func(t *testing.T) {
			t.Run("introspection query", func(t *testing.T) {
				run(`query IntrospectionQuery {
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
					run(`
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
					run(`
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
					run(`
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
					run(`
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
					run(`
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
					run(`
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
					run(`
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
					run(`
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
					run(`
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
					run(`
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
					run(`
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
				/*
					Why ignore when this still will result in an error because T is undefined?
					t.Run("ignores unknown fragments", func(t *testing.T) {
						run(`
								{
									field
									...Unknown
									...Known
							 	 }
							 	 fragment Known on T {
									field
									...OtherUnknown
							 	 }`, FieldSelectionMerging(), Valid)
					})*/
				t.Run("return types must be unambiguous", func(t *testing.T) {
					t.Run("conflicting return types which potentially overlap", func(t *testing.T) {
						/*This is invalid since an object could potentially be both the Object
						type IntBox and the interface type NonNullStringBox1. While that
						condition does not exist in the current schema, the schema could
						expand in the future to allow this. Thus it is invalid.*/
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						runWithDefinition(boxDefinition, `
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
						run(`
							{
								a
								... {
								  a
								}
							  }`, FieldSelectionMerging(), Valid)
					})
					t.Run("compares deep types including list", func(t *testing.T) {
						runWithDefinition(boxDefinition, `
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
					/*
						Why ignore?
						t.Run("ignores unknown types", func(t *testing.T) {
							runWithDefinition(boxDefinition, `
								{
								someBox {
								  ...on UnknownType {
									scalar
								  }
								  ...on NonNullStringBox2 {
									scalar
								  }
								}
								}`, FieldSelectionMerging(), Valid)
						})*/
				})
			})
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
							fragment frag on DogExtra { string }`,
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
			t.Run("108 variant", func(t *testing.T) {
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
			t.Run("109", func(t *testing.T) {
				run(`
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
				run(`	
							fragment mergeIdenticalFieldsWithIdenticalValues on Dog {
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
				run(`
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
				run(`
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
					FieldSelectionMerging(), Invalid, true)
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
										... on CatExtra {
											noString: string
										}
									}
								}
							}`,
					FieldSelectionMerging(), Valid)
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
					FieldSelectionMerging(), Invalid, true)
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
				run(`
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
					FieldSelectionMerging(), Invalid, true)
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
				run(`
							fragment scalarSelectionsNotAllowedOnInt on Dog {
								barkVolume {
									sinceWhen
								}
							}`,
					FieldSelections(), Invalid, true)
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
				run(`
							mutation directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid, true)
				run(`
							subscription directQueryOnUnionWithoutSubFields {
								catOrDog
							}`,
					FieldSelections(), Invalid, true)
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
				run(`	
							query argOnRequiredArg($dogCommand: DogCommand = SIT) {
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
				run(`	
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
				run(`	
							query argOnRequiredArg($booleanArg: Boolean) {
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
				run(`	
							{
								dog { ...invalidArgName}
							}
							fragment invalidArgName on Dog {
								doesKnowCommand(command: CLEAN_UP_HOUSE)
							}`,
					ValidArguments(), Invalid)
			})
			t.Run("118 variant", func(t *testing.T) {
				run(`	
							{
								dog { ...invalidArgName}
							}
							fragment invalidArgName on Dog {
								doesKnowCommand(dogCommand: CLEAN_UP_HOUSE)
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
				run(`
								{
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
					run(`
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
					run(`
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
					run(`	
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
					run(`	
								fragment notOnExistingType on NotInSchema {
  									name
								}`, Fragments(), Invalid, true)
				})
				t.Run("129", func(t *testing.T) {
					run(`	
								fragment inlineNotExistingType on Dog {
  									... on NotInSchema {
    									name
  									}
								}`, Fragments(), Invalid, true)
				})
			})
			t.Run("5.5.1.3 Fragments on Composite Types", func(t *testing.T) {
				t.Run("130", func(t *testing.T) {
					run(`
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
					run(`
								fragment fragOnScalar on Int {
									something
								}`,
						Fragments(), Invalid, true)
				})
				t.Run("131", func(t *testing.T) {
					run(`
								fragment inlineFragOnScalar on Dog {
									... on Boolean {
										somethingElse
									}
								}`,
						Fragments(), Invalid, true)
				})
			})
			t.Run("5.5.1.4 Fragments must be used", func(t *testing.T) {
				t.Run("132", func(t *testing.T) {
					run(`
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
					run(`
								fragment dogNames on Query {
									dog { name }
								}
								{
									...dogNames
								}`,
						Fragments(), Valid)
				})
				t.Run("132 variant", func(t *testing.T) {
					run(`
								fragment catNames on Query {
									dog { name }
								}
								{
									...dogNames
								}`,
						Fragments(), Invalid, true)
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
					run(`
								{
									dog {
										...undefinedFragment
									}
								}`,
						Fragments(), Invalid, true)
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
					run(`
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
						Fragments(), Invalid, true)
				})
				t.Run("136 variant", func(t *testing.T) {
					run(`
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
						Fragments(), Invalid, true)
				})
			})
			t.Run("5.5.2.3 Fragment spread is possible", func(t *testing.T) {
				t.Run("5.5.2.3.1 Object Spreads In Object Scope", func(t *testing.T) {
					t.Run("137", func(t *testing.T) {
						run(`
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
						run(`
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
							Fragments(), Invalid, true)
					})
					t.Run("138", func(t *testing.T) {
						run(`
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
						run(`
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
						run(`
									{
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
						run(`
									{
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
						run(`
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
			t.Run("145", func(t *testing.T) {
				run(`
							query goodComplexDefaultValue($search: ComplexInput = { name: "Fido" }) {
								findDog(complex: $search)
							}`,
					Values(), Valid)
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
					Values(), Valid)
			})
			t.Run("145 variant", func(t *testing.T) {
				run(`
							query goodComplexDefaultValue($search: ComplexInput = { name: 123 }) {
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
							query badComplexValue {
								findDog(complex: { name: 123 })
							}`,
					Values(), Invalid)
			})
			t.Run("146 variant", func(t *testing.T) {
				run(`
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
		t.Run("complex nested validation", func(t *testing.T) {
			t.Run("complex nested 1", func(t *testing.T) {
				run(`
						{
							nested(input: {})
						}
						`, Values(), Invalid)
			})
			t.Run("complex nested ok", func(t *testing.T) {
				run(`
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: ["str"]
							})
						}
						`, Values(), Valid)
			})
			t.Run("complex nested 'notList' is not list of Strings", func(t *testing.T) {
				run(`
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: "notList",
								requiredListOfRequiredStrings: ["str"]
							})
						}
						`, Values(), Invalid)
			})
			t.Run("complex nested ok 3", func(t *testing.T) {
				run(`
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
				run(`
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
				run(`
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
						`, Values(), Invalid)
			})
			t.Run("complex nested 'str' is not String", func(t *testing.T) {
				run(`
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [str],
								requiredListOfRequiredStrings: ["str"],
								requiredListOfOptionalStringsWithDefault: ["more strings"]
							})
						}
						`, Values(), Invalid)
			})
			t.Run("complex nested requiredListOfRequiredStrings must not be empty", func(t *testing.T) {
				run(`
						{
							nested(input: {
								requiredString: "str",
								requiredListOfOptionalStrings: [],
								requiredListOfRequiredStrings: []
							})
						}
						`, Values(), Invalid)
			})
			t.Run("complex 2x nested", func(t *testing.T) {
				run(`
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
				run(`
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
						`, Values(), Invalid)
			})
			t.Run("complex 2x nested '123' is no String", func(t *testing.T) {
				run(`
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
						`, Values(), Invalid)
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
				run(`
							{
								...frag @spread
							}
							fragment frag on Query {}`,
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
				run(`
							mutation @onQuery {
								mutateDog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`
							mutation @onSubscription {
								mutateDog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`
							mutation @onMutation {
								mutateDog
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`
							subscription @onQuery {
								subscribeDog
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`
							subscription @onMutation {
								foo
							}`,
					DirectivesAreInValidLocations(), Invalid)
			})
			t.Run("150 variant", func(t *testing.T) {
				run(`
							subscription @onSubscription {
								foo
							}`,
					DirectivesAreInValidLocations(), Valid)
			})
		})
		t.Run("5.7.3 Directives Are Unique Per Location", func(t *testing.T) {
			t.Run("151", func(t *testing.T) {
				run(`query MyQuery($foo: Boolean = true, $bar: Boolean = false) {
									field @skip(if: $foo) @skip(if: $bar)
								}`,
					DirectivesAreUniquePerLocation(), Invalid)
			})
			t.Run("152", func(t *testing.T) {
				run(`query MyQuery($foo: Boolean = true, $bar: Boolean = false) {
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
				run(`query houseTrainedQuery($atOtherHomes: Boolean, $atOtherHomes: Boolean) {
									dog {
										isHousetrained(atOtherHomes: $atOtherHomes)
									}
								}`,
					VariableUniqueness(), Invalid)
			})
			t.Run("154", func(t *testing.T) {
				run(`
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
				run(`
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
		t.Run("5.8.3 All VariableValue Uses Defined", func(t *testing.T) {
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
		t.Run("5.8.5 All VariableValue Usages are Allowed", func(t *testing.T) {
			t.Run("169", func(t *testing.T) {
				run(`query intCannotGoIntoBoolean($intArg: Int) {
									arguments {
										booleanArgField(booleanArg: $intArg)
									}
								}`,
					ValidArguments(), Invalid)
			})
			t.Run("170", func(t *testing.T) {
				run(`query booleanListCannotGoIntoBoolean($booleanListArg: [Boolean]) {
									arguments {
										booleanArgField(booleanArg: $booleanListArg)
									}
								}`,
					ValidArguments(), Invalid)
			})
			t.Run("171", func(t *testing.T) {
				run(`query booleanArgQuery($booleanArg: Boolean) {
									arguments {
										nonNullBooleanArgField(nonNullBooleanArg: $booleanArg)
									}
								}`,
					ValidArguments(), Invalid)
			})
			t.Run("172", func(t *testing.T) {
				run(`query nonNullListToList($nonNullBooleanList: [Boolean]!) {
								arguments {
									booleanListArgField(booleanListArg: $nonNullBooleanList)
								}
							}`,
					ValidArguments(), Valid)
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
					ValidArguments(), Invalid)
			})
			t.Run("173", func(t *testing.T) {
				run(`query listToNonNullList($booleanList: [Boolean]) {
									arguments {
										nonNullBooleanListField(nonNullBooleanListArg: $booleanList)
									}
								}`,
					ValidArguments(), Invalid)
			})
			t.Run("174", func(t *testing.T) {
				run(`query booleanArgQueryWithDefault($booleanArg: Boolean) {
									arguments {
										optionalNonNullBooleanArgField(optionalBooleanArg: $booleanArg)
									}
								}`,
					ValidArguments(), Valid)
			})
			t.Run("175", func(t *testing.T) {
				run(`query booleanArgQueryWithDefault($booleanArg: Boolean = true) {
									arguments {
										nonNullBooleanArgField(nonNullBooleanArg: $booleanArg)
									}
								}`,
					ValidArguments(), Valid)
			})
		})
	})
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

input ComplexInput { name: String, owner: String }
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
	findDogNonOptional(complex: ComplexNonOptionalInput): Dog
  	booleanList(booleanListArg: [Boolean!]): Boolean
	extra: Extra
	nested(input: NestedInput): Boolean
}

type ValidArguments {
	multipleReqs(x: Int!, y: Int!): Int!
	booleanArgField(booleanArg: Boolean): Boolean
	floatArgField(floatArg: Float): Float
	intArgField(intArg: Int): Int
	nonNullBooleanArgField(nonNullBooleanArg: Boolean!): Boolean!
	nonNullBooleanListField(nonNullBooleanListArg: [Boolean]!): Boolean!
	booleanListArgField(booleanListArg: [Boolean]!): [Boolean]
	optionalNonNullBooleanArgField(optionalBooleanArg: Boolean! = false): Boolean!
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
