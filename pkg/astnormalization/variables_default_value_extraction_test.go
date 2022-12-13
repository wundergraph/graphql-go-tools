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
			mixed(a: String, b: String, input: String = "foo", nonNullInput: String! = "bar"): String
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
				mutation simple($a: String) {
					simple(input: $a)
				}`, ``, `{"a":"foo"}`)
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
				mutation simple($a: String, $b: String, $c: String!) {
			  		mixed(a: $a, b: "bar", input: $b, nonNullInput: $c)
				}`, `{"a":"aaa"}`, `{"c":"bar","b":"foo","a":"aaa"}`)
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
			mutation simple($a: String, $b: String, $c: String, $d: String!) {
				mixed(a: $a, b: $b, input: $c, nonNullInput: $d)
			}`, `{"a":"aaa"}`, `{"d":"bar","c":"foo","b":"bazz","a":"aaa"}`)

	})
}
