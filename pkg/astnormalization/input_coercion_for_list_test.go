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

type Input {
	foo: String
}

type Query {
	characterById(id: Int): Character
	nestedList(ids: [[Int]]): [Character]
	charactersByIds(ids: [Int]): [Character]
	characterByInput(input: Input): [Character]
	charactersByInputs(inputs: [Input]): [Character]
	charactersByIdsNonNull(ids: [Int]!): [Character]
	charactersByIdsNonNullInteger(ids: [Int!]!): [Character]
	nestedListNonNull(ids: [[Int!]!]!): [Character]
	innerListNonNull(ids: [[Int]!]): [Character]
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

	t.Run("object type definition with variables", func(t *testing.T) {
		runWithVariablesAssert(t, inputCoercionForList, inputCoercionForListDefinition, `
query ($input: Input) {
  charactersByInputs(input: $input) {
    id
    name
  }
}`,
			``,
			`
query ($input: Input) {
  charactersByInputs(input: $input) {
    id
    name
  }
}`, `{"inputs": {"foo": "bar"}}`, `{"inputs": {"foo": "bar"}}`,
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

	t.Run("convert integer to list of integer, non-null", func(t *testing.T) {
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

	t.Run("convert integer to nested list of integer, non-null", func(t *testing.T) {
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

	t.Run("convert integer to nested list of integer, inner list non-null", func(t *testing.T) {
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

	t.Run("send list of integers as variable input, non-null", func(t *testing.T) {
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

	t.Run("send list of integers as variable input", func(t *testing.T) {
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
}
