package astnormalization

import (
	"testing"
)

const (
	variablesDefaultValueExtractionDefinition = `
		schema { mutation: Mutation }
		type Query {
			complex(input: ComplexInput): String
			mixedArgs(a: String, b: String!): String
			objectInList(input: [Nested]): String
			objectInNestedList(input: [[Nested]]): String
			stringInNestedList(input: [[String!]]): String
			nullableStringInNestedList(input: [[String]]): String
			notNullableInt(input: Int! = 5): String
			notNullableString(input: String!): String
			withoutArguments: String
		}
		input Nested {
			NotNullable: String!
			Nullable: String
		}
		input ComplexInput {
			NotNullable: String!
			Nullable: String
			list: [String]
			listOfNonNull: [String!]
			listNonNullItemNullable: [String]!
			listNonNullItemNotNull: [String!]!
		}
		type Mutation {
			simple(input: String = "foo"): String
			mixed(a: String, b: String, input: String = "foo", nonNullInput: String! = "bar", nullableWithNullDefault: String = null): String
		}
		scalar String
		input ComplexInput {
			NotNullable: String!
			Nullable: String
			list: [String]
			listNonNull: [String]!
			listNonNullItemNonNull: [String!]!
			listNonNull: [String!]
		}
	`
)

func TestVariablesDefaultValueExtraction(t *testing.T) {
	t.Run("field argument default value", func(t *testing.T) {
		t.Run("no value provided", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
				mutation simple {
			  		simple
				}`, "", `
				mutation simple {
					simple
				}`, ``, ``)
		})

		t.Run("value provided", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
				mutation simple {
					simple(input: "bazz")
				}`, "", `
				mutation simple {
					simple(input: "bazz")
				}`, ``, ``)
		})

		t.Run("mixed", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
				mutation simple($a: String) {
			  		mixed(a: $a, b: "bar")
				}`, "", `
				mutation simple($a: String) {
			  		mixed(a: $a, b: "bar")
				}`, `{"a":"aaa"}`, `{"a":"aaa"}`)
		})
	})

	t.Run("variable with default value", func(t *testing.T) {
		t.Run("no value provided", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
				mutation simple($in: String = "bar" ) {
					simple(input: $in)
				}`, "", `
				mutation simple($in: String) {
			  		simple(input: $in)
				}`, ``, `{"in":"bar"}`)
		})
		t.Run("value provided", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
				mutation simple($in: String = "bar" ) {
			  		simple(input: $in)
				}`, "", `
				mutation simple($in: String) {
			  		simple(input: $in)
				}`, `{"in":"foo"}`, `{"in":"foo"}`)
		})

		t.Run("multiple variables with default values", func(t *testing.T) {
			t.Run("vars inside object and lists", func(t *testing.T) {
				runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
					query q(
						$nullable: String = "a",
						$notNullable: String = "b",
						$strIntolist: String = "1",
						$strIntolistOfNonNull: String = "2"
						$strIntolistNonNullItemNullable: String = "3",
						$strIntolistNonNullItemNotNull: String = "4"
					) {
						complex(input: {
							NotNullable: $notNullable,
							Nullable: $nullable,
							list: [$strIntolist],
							listOfNonNull: [$strIntolistOfNonNull],
							listNonNullItemNullable: [$strIntolistNonNullItemNullable],
							listNonNullItemNotNull: [$strIntolistNonNullItemNotNull],
						})
					}`, "", `
					query q(
						$nullable: String,
						$notNullable: String!,
						$strIntolist: String,
						$strIntolistOfNonNull: String!
						$strIntolistNonNullItemNullable: String,
						$strIntolistNonNullItemNotNull: String!
					) {
						complex(input: {
							NotNullable: $notNullable,
							Nullable: $nullable,
							list: [$strIntolist],
							listOfNonNull: [$strIntolistOfNonNull],
							listNonNullItemNullable: [$strIntolistNonNullItemNullable],
							listNonNullItemNotNull: [$strIntolistNonNullItemNotNull],
						})
					}`,
					``,
					`{"strIntolistNonNullItemNotNull":"4","strIntolistNonNullItemNullable":"3","strIntolistOfNonNull":"2","strIntolist":"1","notNullable":"b","nullable":"a"}`)
			})
			t.Run("vars as lists", func(t *testing.T) {
				runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
					query q(
						$nullable: String = "a",
						$notNullable: String = "b",
						$list: [String] = ["1"],
						$listOfNonNull: [String!] = ["2"]
						$listNonNullItemNullable: [String] = ["3"],
						$listNonNullItemNotNull: [String!] = ["4"]
					) {
						complex(input: {
							NotNullable: $notNullable,
							Nullable: $nullable,
							list: $list,
							listOfNonNull: $listOfNonNull
							listNonNullItemNullable: $listNonNullItemNullable,
							listNonNullItemNotNull: $listNonNullItemNotNull,
						})
					}`, ``, `
					query q(
						$nullable: String,
						$notNullable: String!,
						$list: [String],
						$listOfNonNull: [String!]
						$listNonNullItemNullable: [String]!,
						$listNonNullItemNotNull: [String!]!
					) {
						complex(input: {
							NotNullable: $notNullable,
							Nullable: $nullable,
							list: $list,
							listOfNonNull: $listOfNonNull
							listNonNullItemNullable: $listNonNullItemNullable,
							listNonNullItemNotNull: $listNonNullItemNotNull,
						})
					}`,
					``,
					`{"listNonNullItemNotNull":["4"],"listNonNullItemNullable":["3"],"listOfNonNull":["2"],"list":["1"],"notNullable":"b","nullable":"a"}`)
			})
		})

		t.Run("variables in lists", func(t *testing.T) {
			t.Run("object in list", func(t *testing.T) {
				runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
					query q(
						$nullable: String = "a",
						$notNullable: String = "b",
					) {
						objectInList(input: [{NotNullable: $notNullable, Nullable: $nullable}])
					}`, "", `
					query q(
						$nullable: String,
						$notNullable: String!,
					) {
						objectInList(input: [{NotNullable: $notNullable, Nullable: $nullable}])
					}`, ``, `{"notNullable":"b","nullable":"a"}`)
			})

			t.Run("object in nested list", func(t *testing.T) {
				runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
					query q(
						$nullable: String = "a",
						$notNullable: String = "b",
					) {
						objectInNestedList(input: [[{NotNullable: $notNullable, Nullable: $nullable}]])
					}`, "", `
					query q(
						$nullable: String,
						$notNullable: String!,
					) {
						objectInNestedList(input: [[{NotNullable: $notNullable, Nullable: $nullable}]])
					}`, ``, `{"notNullable":"b","nullable":"a"}`)
			})

			t.Run("not nullable string in nested list", func(t *testing.T) {
				runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
					query q(
						$notNullable: String = "foo",
					) {
						stringInNestedList(input: [["a", $notNullable]])
					}`, "", `
					query q(
						$notNullable: String!,
					) {
						stringInNestedList(input: [["a", $notNullable]])
					}`, ``, `{"notNullable":"foo"}`)
			})

			t.Run("nullable string in nested list", func(t *testing.T) {
				runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
					query q(
						$nullable: String = "foo",
					) {
						nullableStringInNestedList(input: [["a", null, $nullable]])
					}`, "", `
					query q(
						$nullable: String,
					) {
						nullableStringInNestedList(input: [["a", null, $nullable]])
					}`, ``, `{"nullable":"foo"}`)
			})
		})
	})

	t.Run("mixed nullable and not nullable variables", func(t *testing.T) {
		runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			query q(
				$nullable: String = "a",
				$notNullable: String = "b",
			) {
				mixedArgs(a: $nullable, b: $notNullable)
			}`, "", `
			query q(
				$nullable: String,
				$notNullable: String!,
			) {
				mixedArgs(a: $nullable, b: $notNullable)
			}`, ``, `{"notNullable":"b","nullable":"a"}`)
	})

	t.Run("mixed default values of field args and variables", func(t *testing.T) {
		runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			mutation simple($a: String = "bar", $b: String = "bazz") {
				mixed(a: $a, b: $b)
			}`, "", `
			mutation simple($a: String, $b: String) {
				mixed(a: $a, b: $b)
			}`, `{"a":"aaa"}`, `{"b":"bazz","a":"aaa"}`)

	})

	t.Run("variable with null default value", func(t *testing.T) {
		runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			query q($data: ComplexInput = null) {
				complex(input: $data)
			}`, "", `
			query q($data: ComplexInput) {
				complex(input: $data)
			}`, ``, `{"data":null}`)
	})

	t.Run("variables used in position expecting non null value", func(t *testing.T) {
		t.Run("Not nullable int with default value", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			 query q(
				 $input: Int = 4,
			 ) {
				 notNullableInt(input: $input)
			 }`, "", `
			 query q(
				 $input: Int!,
			 ) {
				 notNullableInt(input: $input)
			 }`, ``, `{"input":4}`)
		})

		t.Run("not nullable int with default value and variable value exists", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			 query q(
				 $input: Int = 4,
			 ) {
				 notNullableInt(input: $input)
			 }`, "", `
			 query q(
				 $input: Int!,
			 ) {
				 notNullableInt(input: $input)
			 }`, `{"input":6}`, `{"input":6}`)
		})

		t.Run("nullable string with default value", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			 query q(
				 $input: String = "DefaultInOperation",
			 ) {
				 notNullableString(input: $input)
			 }`, "", `
			 query q(
				 $input: String!,
			 ) {
				 notNullableString(input: $input)
			 }`, ``, `{"input":"DefaultInOperation"}`)
		})

		t.Run("not nullable string with default value", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			 query q(
				 $input: String! = "DefaultInOperation",
			 ) {
				 notNullableString(input: $input)
			 }`, "", `
			 query q(
				 $input: String!,
			 ) {
				 notNullableString(input: $input)
			 }`, ``, `{"input":"DefaultInOperation"}`)
		})

		t.Run("not nullable string with default value and variable value exists", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			 query q(
				 $input: String = "DefaultInOperation",
			 ) {
				 notNullableString(input: $input)
			 }`, "", `
			 query q(
				 $input: String!,
			 ) {
				 notNullableString(input: $input)
			 }`, `{"input":"ValueInVariable"}`, `{"input":"ValueInVariable"}`)
		})
	})

	t.Run("variables used in directive argument expecting non null value", func(t *testing.T) {
		t.Run("nullable boolean with default value and variable value exists", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			 query q(
				 $flag: Boolean = false,
			 ) {
				 withoutArguments @skip(if: $flag)
			 }`, "", `
			 query q(
				 $flag: Boolean!,
			 ) {
				 withoutArguments @skip(if: $flag)
			 }`, `{"flag":true}`, `{"flag":true}`)
		})

		t.Run("nullable boolean used twice", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			 query q(
				 $flag: Boolean = false,
			 ) {
				 withoutArguments @skip(if: $flag) @include(if: $flag)
			 }`, "", `
			 query q(
				 $flag: Boolean!,
			 ) {
				 withoutArguments @skip(if: $flag) @include(if: $flag)
			 }`, `{"flag":true}`, `{"flag":true}`)
		})

		t.Run("not nullable boolean with default value and variable value exists", func(t *testing.T) {
			runWithVariablesDefaultValues(t, extractVariablesDefaultValue, variablesDefaultValueExtractionDefinition, `
			 query q(
				 $flag: Boolean! = false,
			 ) {
				 withoutArguments @skip(if: $flag)
			 }`, "", `
			 query q(
				 $flag: Boolean!,
			 ) {
				 withoutArguments @skip(if: $flag)
			 }`, `{"flag":true}`, `{"flag":true}`)
		})
	})
}
