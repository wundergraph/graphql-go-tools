package astnormalization

import (
	"testing"
)

const inputCoercionForListDefinition = `
schema {
	query: Query
	mutation: Mutation
}

type Character {
	id: Int
	name: String
}

input Input {
	foo: String
}

input InputWithList {
	foo: String
	list: [InputWithList]
	nested: InputWithList
}

input InputWithListNonNull {
	foo: String
	list: [InputWithList!]!
	nested: InputWithList!
}

input InputWithListNestedList {
	foo: String
	doubleList: [[InputWithList]]
	nested: InputWithList
}

type Query {
	characterById(id: Int): Character
	nestedList(ids: [[Int]]): [Character]
	charactersByIds(ids: [Int]): [Character]
	charactersByStringIds(ids: [String]): [Character]
	charactersByIdScalarIds(ids: [ID]): [Character]
	characterByInput(input: Input): Character
	charactersByInputs(inputs: [Input]): [Character]
	charactersByIdsNonNull(ids: [Int]!): [Character]
	charactersByIdsNonNullInteger(ids: [Int!]!): [Character]
	nestedListNonNull(ids: [[Int!]!]!): [Character]
	innerListNonNull(ids: [[Int]!]): [Character]
	characterByIdNonNullInteger(id: Int!): Character
	inputWithList(input: InputWithList): Character
	inputWithListNonNull(input: InputWithListNonNull): Character
	inputWithListNestedList(input: InputWithListNestedList): Character
	inputWithNestedScalar(input: InputWithNestedScalarList): String
}

type Mutation {
	mutate(input: InputWithNestedScalarList): String
	mutateNested(input: Nested): String
	mutateDeepNested(input: DeepNested): String
	mutateWithList(input: [InputWithNestedScalarList]): String
	mutateNestedExtremeList(input: ExtremeNesting): String
	mutateMultiArg(arg1: InputWithNestedScalarList, arg2: InputWithNestedScalarList): String
}

input DeepNested {
	deepNested: Nested
}

input Nested {
	nested: InputWithNestedScalarList
}

input ExtremeNesting {
	nested: [[[[[InputWithNestedScalarList]]]]]
}

input InputWithNestedScalarList {
	stringList: [String!]
	intList: [Int!]
}`

func TestInputCoercionForList(t *testing.T) {
	t.Run("convert integer to list of integer", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIds(ids: 1) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Int]){
			  charactersByIds(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[1]}`, inputCoercionForList)
	})

	t.Run("strings list variants", func(t *testing.T) {
		t.Run("convert string to list of strings", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				query {
				  charactersByStringIds(ids: "id") {
					id
					name
				  }
				}`, ``,
				`
				query ($a: [String]){
				  charactersByStringIds(ids: $a) {
					id
					name
				  }
				}`, `{}`, `{"a":["id"]}`, inputCoercionForList)
		})

		t.Run("convert string id to list of ID", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				query {
				  charactersByIdScalarIds(ids: "id") {
					id
					name
				  }
				}`, ``,
				`
				query ($a: [ID]){
				  charactersByIdScalarIds(ids: $a) {
					id
					name
				  }
				}`, `{}`, `{"a":["id"]}`, inputCoercionForList)
		})

		t.Run("convert int id to list of ID", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				query Q {
				  charactersByIdScalarIds(ids: 1) {
					id
					name
				  }
				}`, `Q`,
				`
				query Q ($a: [ID]){
				  charactersByIdScalarIds(ids: $a) {
					id
					name
				  }
				}`, `{}`, `{"a":[1]}`, inputCoercionForList)
		})

	})

	t.Run("input with nested scalar list", func(t *testing.T) {
		t.Run("query", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				query Q {
					inputWithNestedScalar(input: {
						stringList: "str",
						intList: 1
					}) 
				}`, `Q`,
				`
				query Q($a: InputWithNestedScalarList) {
					inputWithNestedScalar(input: $a) 
				}`, `{}`, `{"a":{"stringList":["str"],"intList":[1]}}`, inputCoercionForList)
		})

		t.Run("query with null values", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				query Q {
					inputWithNestedScalar(input: {
						stringList: null,
						intList: null
					}) 
				}`, `Q`,
				`
				query Q($a: InputWithNestedScalarList) {
					inputWithNestedScalar(input: $a) 
				}`, `{}`, `{"a":{"stringList":null,"intList":null}}`, inputCoercionForList)
		})

		t.Run("mutation", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate {
					mutate(input: {
						stringList: "str"
					}) 
				}`, `Mutate`,
				`
				mutation Mutate($a: InputWithNestedScalarList) {
					mutate(input: $a) 
				}`, `{}`, `{"a":{"stringList":["str"]}}`, inputCoercionForList)
		})

		t.Run("mutation with list of inputs with nested list fields", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate {
					mutateWithList(input: {
						stringList: "str"
					}) 
				}`, `Mutate`,
				`
				mutation Mutate($a: [InputWithNestedScalarList]) {
					mutateWithList(input: $a) 
				}`, `{}`, `{"a":[{"stringList":["str"]}]}`, inputCoercionForList)
		})

		t.Run("mutation: should coerse each nested field of element of a list", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate {
					mutateWithList(input: [
						{stringList: "str"},
						{intList: 1}
					]) 
				}`, `Mutate`,
				`
				mutation Mutate($a: [InputWithNestedScalarList]) {
					mutateWithList(input: $a) 
				}`, `{}`, `{"a":[{"stringList":["str"]},{"intList":[1]}]}`, inputCoercionForList)
		})

		t.Run("mutation list items as variables", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate($a: InputWithNestedScalarList, $b: InputWithNestedScalarList) {
					mutateWithList(input: [
						$a,
						$b
					])
				}`, `Mutate`,
				`
				mutation Mutate($a: InputWithNestedScalarList, $b: InputWithNestedScalarList) {
					mutateWithList(input: [
						$a,
						$b
					]) 
				}`,
				`{"a":{"stringList":"str"},"b":{"intList":1}}`,
				`{"a":{"stringList":["str"]},"b":{"intList":[1]}}`, inputCoercionForList)
		})

		t.Run("mutation list items as variables with additional variables", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate($a: InputWithNestedScalarList, $b: InputWithNestedScalarList) {
					mutateWithList(input: [
						$a,
						$b,
						{stringList: "str2"},
						{intList: 1}
					])
				}`, `Mutate`,
				`
				mutation Mutate($a: InputWithNestedScalarList, $b: InputWithNestedScalarList, $c: [String!], $d: [Int!]) {
					mutateWithList(input: [
						$a,
						$b,
						{stringList: $c},
						{intList: $d}
					]) 
				}`,
				`{"a":{"stringList":"str"},"b":{"intList":1}}`,
				`{"d":[1],"c":["str2"],"a":{"stringList":["str"]},"b":{"intList":[1]}}`, inputCoercionForList)
		})

		t.Run("mutation nested", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate {
					mutateNested(input: {
						nested: {
							stringList: "str"
						}
					}) 
				}`, `Mutate`,
				`
				mutation Mutate($a: Nested) {
					mutateNested(input: $a) 
				}`, `{}`, `{"a":{"nested":{"stringList":["str"]}}}`, inputCoercionForList)
		})

		t.Run("mutation deep nested", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate {
					mutateDeepNested(input: {
						deepNested: {
							nested: {
								stringList: "str"
							}
						}
					}) 
				}`, `Mutate`,
				`
				mutation Mutate($a: DeepNested) {
					mutateDeepNested(input: $a) 
				}`, `{}`, `{"a":{"deepNested":{"nested":{"stringList":["str"]}}}}`, inputCoercionForList)
		})

		t.Run("mutation deep nested in extreme list", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate {
					mutateNestedExtremeList(input: {
						nested: {
							stringList: "str"
						}
					}) 
				}`, `Mutate`,
				`
				mutation Mutate($a: ExtremeNesting) {
					mutateNestedExtremeList(input: $a) 
				}`, `{}`, `{"a":{"nested":[[[[[{"stringList":["str"]}]]]]]}}`, inputCoercionForList)
		})

		t.Run("mutation multi arg", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				mutation Mutate {
					mutateMultiArg(arg1: {stringList: "str"}, arg2: {intList: 1})
				}`, `Mutate`,
				`
				mutation Mutate($a: InputWithNestedScalarList, $b: InputWithNestedScalarList) {
					mutateMultiArg(arg1: $a, arg2: $b) 
				}`, `{}`, `{"b":{"intList":[1]},"a":{"stringList":["str"]}}`, inputCoercionForList)
		})

	})

	t.Run("list of integers", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIds(ids: [1, 2, 3]) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Int]){
			  charactersByIds(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[1,2,3]}`, inputCoercionForList)
	})

	t.Run("nested list of integers", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  nestedList(ids: [[1], [2, 3]]) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [[Int]]) {
			  nestedList(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[[1],[2,3]]}`, inputCoercionForList)
	})

	t.Run("list of integers with null value", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIds(ids: null) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Int]){
			  charactersByIds(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":null}`, inputCoercionForList)
	})

	t.Run("nested list with null value", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  nestedList(ids: null) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [[Int]]){
			  nestedList(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":null}`, inputCoercionForList)
	})

	t.Run("convert integer to nested list of integer", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  nestedList(ids: 1) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [[Int]]){
			  nestedList(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[[1]]}`, inputCoercionForList)
	})

	t.Run("integer argument without modification", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  characterById(id: 1) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: Int) {
			  characterById(id: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":1}`)
	})

	t.Run("non-null integer argument without modification", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  characterByIdNonNullInteger(id: 1) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: Int!){
			  characterByIdNonNullInteger(id: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":1}`, inputCoercionForList)
	})

	t.Run("do not modify null as argument", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  characterByIdNonNullInteger(id: null) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: Int!){
			  characterByIdNonNullInteger(id: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":null}`, inputCoercionForList)
	})

	t.Run("convert object type to list of object type", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByInputs(inputs: { foo: "bar" }) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Input]){
			  charactersByInputs(inputs: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[{"foo":"bar"}]}`, inputCoercionForList)
	})

	t.Run("list of object type to list of object type remains unchanged", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByInputs(inputs: [{ foo: "bar" },{ foo: "bazz" }]) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Input]){
			  charactersByInputs(inputs: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[{"foo":"bar"},{"foo":"bazz"}]}`, inputCoercionForList)
	})

	t.Run("object type definition", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  characterByInput(input: { foo: "bar" }) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: Input){
			  characterByInput(input: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":{"foo":"bar"}}`, inputCoercionForList)
	})

	t.Run("handle non-existent variable", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
			query ($ids: [[Int]]) {
			  nestedList(ids: $ids) {
				id
				name
			  }
			}`, ``,
			`
			query ($ids: [[Int]]) {
			  nestedList(ids: $ids) {
				id
				name
			  }
			}`, `{"foo": "bar"}`, `{"foo": "bar"}`)
	})

	t.Run("convert integer to list of integer, non-null list", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIdsNonNull(ids: 1) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Int]!){
			  charactersByIdsNonNull(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[1]}`, inputCoercionForList)
	})

	t.Run("convert integer to list of integer, non-null integer", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIdsNonNullInteger(ids: 1) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Int!]!){
			  charactersByIdsNonNullInteger(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[1]}`, inputCoercionForList)
	})

	t.Run("send list of integers as argument, non-null integer", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIdsNonNullInteger(ids: [1]) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Int!]!) {
			  charactersByIdsNonNullInteger(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[1]}`, inputCoercionForList)
	})

	t.Run("send list of integers as argument, non-null list", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIdsNonNull(ids: [1]) {
				id
				name
			  }
			}`,
			``,
			`
			query ($a: [Int]!) {
			  charactersByIdsNonNull(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[1]}`, inputCoercionForList)
	})

	t.Run("convert integer to nested list of integer, non-null nested list", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  nestedListNonNull(ids: 1) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [[Int!]!]!){
			  nestedListNonNull(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[[1]]}`, inputCoercionForList)
	})

	t.Run("convert integer to nested list of integer, non-null inner list", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  innerListNonNull(ids: 1) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [[Int]!]){
			  innerListNonNull(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":[[1]]}`, inputCoercionForList)
	})

	t.Run("send list of integers as variable input", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
			query ($ids: [[Int]]) {
			  nestedList(ids: $ids) {
				id
				name
			  }
			}`, ``,
			`
			query ($ids: [[Int]]) {
			  nestedList(ids: $ids) {
				id
				name
			  }
			}`, `{"ids":[1]}`, `{"ids":[[1]]}`)
	})

	t.Run("send inline null to charactersByIdsNonNull", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIdsNonNull(ids: null) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Int]!){
			  charactersByIdsNonNull(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":null}`, inputCoercionForList)
	})

	t.Run("send inline null to nestedListNonNull", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  nestedListNonNull(ids: null) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [[Int!]!]!){
			  nestedListNonNull(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":null}`, inputCoercionForList)
	})

	t.Run("send inline null to charactersByIdsNonNullInteger", func(t *testing.T) {
		runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
			query {
			  charactersByIdsNonNullInteger(ids: null) {
				id
				name
			  }
			}`, ``,
			`
			query ($a: [Int!]!){
			  charactersByIdsNonNullInteger(ids: $a) {
				id
				name
			  }
			}`, `{}`, `{"a":null}`, inputCoercionForList)
	})

	t.Run("nested variants", func(t *testing.T) {
		t.Run("nested variables", func(t *testing.T) {
			runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
				query ($input: InputWithList) {
				  inputWithList(input: $input) {
					id
					name
				  }
				}`,
				``, `
				query ($input: InputWithList) {
				  inputWithList(input: $input) {
					id
					name
				  }
				}`,
				`{"input":{"list":{"foo":"bar","list":{"foo":"bar2","list":{"nested":{"foo":"bar3","list":{"foo":"bar4"}}}}}}}`,
				`{"input":{"list":[{"foo":"bar","list":[{"foo":"bar2","list":[{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}]}}`)
		})

		t.Run("nested variables, non-null", func(t *testing.T) {
			runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
				query ($input: InputWithListNonNull) {
				  inputWithListNonNull(input: $input) {
					id
					name
				  }
				}`,
				``, `
				query ($input: InputWithListNonNull) {
				  inputWithListNonNull(input: $input) {
					id
					name
				  }
				}`,
				`{"input":{"list":{"foo":"bar","list":{"foo":"bar2","list":{"nested":{"foo":"bar3","list":{"foo":"bar4"}}}}}}}`,
				`{"input":{"list":[{"foo":"bar","list":[{"foo":"bar2","list":[{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}]}}`)
		})

		t.Run("nested variables, list", func(t *testing.T) {
			runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
				query ($input: InputWithListNestedList) {
				  inputWithListNestedList(input: $input) {
					id
					name
				  }
				}`,
				``, `
				query ($input: InputWithListNestedList) {
				  inputWithListNestedList(input: $input) {
					id
					name
				  }
				}`,
				`{"input":{"doubleList":{"foo":"bar","list":{"foo":"bar2","list":{"nested":{"foo":"bar3","list":{"foo":"bar4"}}}}}}}`,
				`{"input":{"doubleList":[[{"foo":"bar","list":[{"foo":"bar2","list":[{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}]]}}`)
		})

		t.Run("nested test with inline values", func(t *testing.T) {
			runWithVariables(t, extractVariables, inputCoercionForListDefinition, `
				query Foo {
				  inputWithList(input: {list:{foo:"bar",list:{foo:"bar2",list:{nested:{foo:"bar3",list:{foo:"bar4"}}}}}}) {
					id
					name
				  }
				}`, `Foo`,
				`
				query Foo($a: InputWithList) {
				  inputWithList(input: $a) {
					id
					name
				  }
				}`,
				`{}`,
				`{"a":{"list":[{"foo":"bar","list":[{"foo":"bar2","list":[{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}]}}`, inputCoercionForList)
		})
		t.Run("walk into nested non existing object", func(t *testing.T) {
			runWithExpectedErrors(t, extractVariables, inputCoercionForListDefinition, `
				query {
				  inputWithListNestedList(input: {
				    nested: {non_existing_field: true},
				  }) {
 				   id
 				   name
 				 }
				}
`, "nested field non_existing_field is not defined in any subfield of type InputWithList", inputCoercionForList)
		})
	})
}
