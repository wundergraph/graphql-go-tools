package astnormalization

import (
	"testing"
)

const inputCoercionForListDefinition = `
schema {
	query: Query
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
	list: [[InputWithList]]
	nested: InputWithList
}

type Query {
	characterById(id: Int): Character
	nestedList(ids: [[Int]]): [Character]
	charactersByIds(ids: [Int]): [Character]
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
}`

func TestInputCoercionForList(t *testing.T) {
	t.Run("convert integer to list of integer", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  charactersByIds(ids: 1) {
    id
    name
  }
}`,
			`
query {
  charactersByIds(ids: [1]) {
    id
    name
  }
}`)
	})

	t.Run("list of integers", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  charactersByIds(ids: [1, 2, 3]) {
    id
    name
  }
}`,
			`
query {
  charactersByIds(ids: [1, 2, 3]) {
    id
    name
  }
}`)
	})

	t.Run("nested list of integers", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  nestedList(ids: [[1], [2, 3]]) {
    id
    name
  }
}`,
			`
query {
  nestedList(ids: [[1], [2, 3]]) {
    id
    name
  }
}`)
	})

	t.Run("list of integers with null value", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  charactersByIds(ids: null) {
    id
    name
  }
}`,
			`
query {
  charactersByIds(ids: null) {
    id
    name
  }
}`)
	})

	t.Run("nested list with null value", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  nestedList(ids: null) {
    id
    name
  }
}`,
			`
query {
  nestedList(ids: null) {
    id
    name
  }
}`)
	})

	t.Run("convert integer to nested list of integer", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  nestedList(ids: 1) {
    id
    name
  }
}`,
			`
query {
  nestedList(ids: [[1]]) {
    id
    name
  }
}`)
	})

	t.Run("integer argument without modification", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query ($id: Int) {
  characterById(id: 1) {
    id
    name
  }
}`,
			`
query ($id: Int) {
  characterById(id: 1) {
    id
    name
  }
}`)
	})

	t.Run("integer variable as input", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `query ($id: Int) {
  characterById(id: $id) {
    id
    name
  }
}`,
			``,
			`
query ($id: Int) {
  characterById(id: $id) {
    id
    name
  }
}`, `{"id":1}`, `{"id":1}`)
	})

	t.Run("non-null integer argument without modification", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query{
  characterByIdNonNullInteger(id: 1) {
    id
    name
  }
}`,
			`
query{
  characterByIdNonNullInteger(id: 1) {
    id
    name
  }
}`)
	})

	t.Run("non-null integer variable as input", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($id: Int!) {
  characterByIdNonNullInteger(id: $id) {
    id
    name
  }
}`,
			``,
			`
query ($id: Int!) {
  characterByIdNonNullInteger(id: $id) {
    id
    name
  }
}`, `{"id":1}`, `{"id":1}`)
	})

	t.Run("do not modify null as variable input", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($id: Int!) {
  characterByIdNonNullInteger(id: $id) {
    id
    name
  }
}`,
			``,
			`
query ($id: Int!) {
  characterByIdNonNullInteger(id: $id) {
    id
    name
  }
}`, `{"id":null}`, `{"id":null}`)
	})

	t.Run("do not modify null as argument", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query{
  characterByIdNonNullInteger(id: null) {
    id
    name
  }
}`,
			`
query{
  characterByIdNonNullInteger(id: null) {
    id
    name
  }
}`)
	})

	t.Run("convert integer variable to list of integers", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [Int]) {
  charactersByIds(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [Int]) {
  charactersByIds(ids: $ids) {
    id
    name
  }
}`, `{"ids":1}`, `{"ids":[1]}`)
	})

	t.Run("null as variable for list of integers", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [Int]) {
  charactersByIds(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [Int]) {
  charactersByIds(ids: $ids) {
    id
    name
  }
}`, `{"ids":null}`, `{"ids":null}`)
	})

	t.Run("send list of integers as variable input", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [Int]) {
  charactersByIds(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [Int]) {
  charactersByIds(ids: $ids) {
    id
    name
  }
}`, `{"ids":[1]}`, `{"ids":[1]}`)
	})

	t.Run("convert integer variable to nested list of integers", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [[Int]]) {
  nestedList(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [[Int]]) {
  nestedList(ids: $ids) {
    id
    name
  }
}`, `{"ids": 1}`, `{"ids": [[1]]}`)
	})

	t.Run("null as variable for nested list of integers", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [[Int]]) {
  nestedList(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [[Int]]) {
  nestedList(ids: $ids) {
    id
    name
  }
}`, `{"ids":null}`, `{"ids":null}`)
	})

	t.Run("convert object type to list of object type", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  charactersByInputs(inputs: { foo: "bar" }) {
    id
    name
  }
}`,
			`
query {
  charactersByInputs(inputs: [{ foo: "bar" }]) {
    id
    name
  }
}`)
	})

	t.Run("convert object type to list of object type with variable definition", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($inputs: [Input]) {
  charactersByInputs(inputs: $inputs) {
    id
    name
  }
}`,
			``,
			`
query ($inputs: [Input]) {
  charactersByInputs(inputs: $inputs) {
    id
    name
  }
}`, `{"inputs": {"foo": "bar"}}`, `{"inputs": [{"foo": "bar"}]}`)
	})

	t.Run("object type definition", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  characterByInput(input: { foo: "bar" }) {
    id
    name
  }
}`,
			`
query {
  characterByInput(input: { foo: "bar" }) {
    id
    name
  }
}`)
	})

	t.Run("non-list object type definition with variables", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($input: Input) {
  characterByInput(input: $input) {
    id
    name
  }
}`,
			``,
			`
query ($input: Input) {
  characterByInput(input: $input) {
    id
    name
  }
}`, `{"input": {"foo": "bar"}}`, `{"input": {"foo": "bar"}}`,
		)
	})

	t.Run("handle non-existent variable", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [[Int]]) {
  nestedList(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [[Int]]) {
  nestedList(ids: $ids) {
    id
    name
  }
}`, `{"foo": "bar"}`, `{"foo": "bar"}`)
	})

	t.Run("convert integer to list of integer, non-null list", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  charactersByIdsNonNull(ids: 1) {
    id
    name
  }
}`,
			`
query {
  charactersByIdsNonNull(ids: [1]) {
    id
    name
  }
}`)
	})

	t.Run("convert integer to list of integer, non-null integer", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  charactersByIdsNonNullInteger(ids: 1) {
    id
    name
  }
}`,
			`
query {
  charactersByIdsNonNullInteger(ids: [1]) {
    id
    name
  }
}`)
	})

	t.Run("send list of integers as variable input, non-null integer", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [Int!]) {
  charactersByIdsNonNullInteger(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [Int!]) {
  charactersByIdsNonNullInteger(ids: $ids) {
    id
    name
  }
}`, `{"ids":[1]}`, `{"ids":[1]}`)
	})

	t.Run("send list of integers as variable input, non-null list", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [Int]!) {
  charactersByIdsNonNull(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [Int]!) {
  charactersByIdsNonNull(ids: $ids) {
    id
    name
  }
}`, `{"ids":[1]}`, `{"ids":[1]}`)
	})

	t.Run("convert integer to nested list of integer, non-null nested list", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  nestedListNonNull(ids: 1) {
    id
    name
  }
}`,
			`
query {
  nestedListNonNull(ids: [[1]]) {
    id
    name
  }
}`)
	})

	t.Run("convert integer to nested list of integer, non-null inner list", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  innerListNonNull(ids: 1) {
    id
    name
  }
}`,
			`
query {
  innerListNonNull(ids: [[1]]) {
    id
    name
  }
}`)
	})

	t.Run("send list of integers as variable input, non-null nested list", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [[Int!]!]!) {
  nestedListNonNull(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [[Int!]!]!) {
  nestedListNonNull(ids: $ids) {
    id
    name
  }
}`, `{"ids":1}`, `{"ids":[[1]]}`)
	})

	t.Run("send list of integers as variable input, remains untouched", func(t *testing.T) {
		// [1] is an invalid input for nestedList(ids: [[Int]]). It should be handled by the
		// validator.
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [[Int]]) {
  nestedList(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [[Int]]) {
  nestedList(ids: $ids) {
    id
    name
  }
}`, `{"ids":[1]}`, `{"ids":[1]}`)
	})

	t.Run("send null as variable to nestedListNonNull", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [[Int!]!]!) {
  nestedListNonNull(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [[Int!]!]!) {
  nestedListNonNull(ids: $ids) {
    id
    name
  }
}`, `{"ids":null}`, `{"ids":null}`)
	})

	t.Run("send inline null to charactersByIdsNonNull", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  charactersByIdsNonNull(ids: null) {
    id
    name
  }
}`,
			`
query {
  charactersByIdsNonNull(ids: null) {
    id
    name
  }
}`)
	})

	t.Run("send null as variable to charactersByIdsNonNull", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [Int]!) {
  charactersByIdsNonNull(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [Int]!) {
  charactersByIdsNonNull(ids: $ids) {
    id
    name
  }
}`, `{"ids":null}`, `{"ids":null}`)
	})

	t.Run("send inline null to nestedListNonNull", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  nestedListNonNull(ids: null) {
    id
    name
  }
}`,
			`
query {
  nestedListNonNull(ids: null) {
    id
    name
  }
}`)
	})

	t.Run("send inline null to charactersByIdsNonNullInteger", func(t *testing.T) {
		run(inputCoercionForList, inputCoercionForListDefinition, `
query {
  charactersByIdsNonNullInteger(ids: null) {
    id
    name
  }
}`,
			`
query {
  charactersByIdsNonNullInteger(ids: null) {
    id
    name
  }
}`)
	})

	t.Run("send null as variable to charactersByIdsNonNullInteger", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($ids: [Int!]) {
  charactersByIdsNonNullInteger(ids: $ids) {
    id
    name
  }
}`,
			``,
			`
query ($ids: [Int!]) {
  charactersByIdsNonNullInteger(ids: $ids) {
    id
    name
  }
}`, `{"ids":null}`, `{"ids":null}`)
	})

	t.Run("nested variables", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($input: InputWithList) {
  inputWithList(input: $input) {
    id
    name
  }
}`,
			``,
			`
query ($input: InputWithList) {
  inputWithList(input: $input) {
    id
    name
  }
}`, `{"input":{"list":{"foo":"bar","list":{"foo":"bar2","list":{"nested":{"foo":"bar3","list":{"foo":"bar4"}}}}}}}`, `{"input":{"list":[{"foo":"bar","list":[{"foo":"bar2","list":[{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}]}}`)
	})

	t.Run("nested variables, non-null", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($input: InputWithListNonNull) {
  inputWithListNonNull(input: $input) {
    id
    name
  }
}`,
			``,
			`
query ($input: InputWithListNonNull) {
  inputWithListNonNull(input: $input) {
    id
    name
  }
}`, `{"input":{"list":{"foo":"bar","list":{"foo":"bar2","list":{"nested":{"foo":"bar3","list":{"foo":"bar4"}}}}}}}`, `{"input":{"list":[{"foo":"bar","list":[{"foo":"bar2","list":[{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}]}}`)
	})

	t.Run("nested variables, list", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($input: InputWithListNestedList) {
  inputWithListNestedList(input: $input) {
    id
    name
  }
}`,
			``,
			`
query ($input: InputWithListNestedList) {
  inputWithListNestedList(input: $input) {
    id
    name
  }
}`, `{"input":{"list":{"foo":"bar","list":{"foo":"bar2","list":{"nested":{"foo":"bar3","list":{"foo":"bar4"}}}}}}}`, `{"input":{"list":[[{"foo":"bar","list":[[{"foo":"bar2","list":[[{"nested":{"foo":"bar3","list":[[{"foo":"bar4"}]]}}]]}]]}]]}}`)
	})

	t.Run("nested test with inline values", func(t *testing.T) {
		t.Skip("not implemented yet")
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query {
  inputWithList(input: {list:{foo:"bar",input:{foo:"bar2",input:{nested:{foo:"bar3",list:{foo:"bar4"}}}}}}) {
    id
    name
  }
}`,
			``,
			`
query ($input: InputWithList) {
  inputWithList(input: $input) {
    id
    name
  }
}`, `{}`, `{"a":{"list":[{"foo":"bar","input":[{"foo":"bar2","input":{"nested":{"foo":"bar3","list":[{"foo":"bar4"}]}}]}]}}`)
	})
}
