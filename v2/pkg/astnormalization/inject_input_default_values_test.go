package astnormalization

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

const testInputDefaultSchema = `
scalar CustomScalar

enum TestEnum {
  ValueOne
  ValueTwo
}

schema {
  mutation: Mutation
}

type Mutation {
  testDefaultValueSimple(data: SimpleTestInput!): String!
  testDefaultValueSimpleNull(data: LowerLevelInput): String!
  testNestedInputField(data: InputWithNestedField!): String!
  mutationExtractDefaultVariable(
    in: PassedWithDefault = { firstField: "test" }
  ): String
  mutationNestedMissing(in: InputWithDefaultFieldsNested): String
  mutationWithListInput(in: InputHasList): String
  mutationWithMultiNestedInput(in: MultiNestedInput): String
  mutationComplexNestedListInput(in: ComplexNestedListInput): String
  mutationSimpleInputList(in: [SimpleTestInput]): String
  mutationUseCustomScalar(in: CustomScalar!): String
  mutationUseCustomScalarList(in: [CustomScalar!]): String
}

input MultiNestedInput {
  nested: [[LowerLevelInput]] = [[{ firstField: 1 }]]
}

input ComplexNestedListInput {
  nested: [[[LowerLevelInput]]]
}

input InputHasList {
  firstList: [LowerLevelInput!]! = [
    { firstField: 1, secondField: ValueOne }
    { firstField: 1 }
  ]
}

input InputWithDefaultFieldsNested {
  first: String!
  nested: LowerLevelInput = { firstField: 0 }
}

input SimpleTestInput {
  firstField: String! = "firstField"
  secondField: Int! = 1
  thirdField: Int!
  fourthField: String
}

input PassedWithDefault {
  firstField: String!
  second: Int! = 0
}

input InputWithNestedField {
  nested: LowerLevelInput!
}

input LowerLevelInput {
  firstField: Int!
  secondField: TestEnum! = ValueOne
}
`

func TestInputDefaultValueExtraction(t *testing.T) {
	t.Run("should not change", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation testDefaultValueSimple($a: SimpleTestInput!) {
  				testDefaultValueSimple(data: $a)
			}`, "", `
			mutation testDefaultValueSimple($a: SimpleTestInput!) {
  				testDefaultValueSimple(data: $a)
			}`, `{"a":{"firstField":"test","secondField":2}}`, `{"a":{"firstField":"test","secondField":2}}`)
	})
	t.Run("simple default value extract", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation testDefaultValueSimple($a: SimpleTestInput!) {
  				testDefaultValueSimple(data: $a)
			}`, "", `
			mutation testDefaultValueSimple($a: SimpleTestInput!) {
  				testDefaultValueSimple(data: $a)
			}`, `{"a":{"firstField":"test"}}`, `{"a":{"firstField":"test","secondField":1}}`)
	})

	t.Run("simple default value nullable extract", func(t *testing.T) {
		runWithVariables(t, extractVariables, testInputDefaultSchema, `
			mutation{
  				testDefaultValueSimpleNull(data: {firstField: "test"})
			}`, "", `
			mutation($a: LowerLevelInput) {
  				testDefaultValueSimpleNull(data: $a)
			}`, "", `{"a":{"firstField":"test","secondField":"ValueOne"}}`,
			func(walker *astvisitor.Walker) {
				injectInputFieldDefaults(walker)
			})
	})

	t.Run("nested input field with default values", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation testNestedInputField($a: InputWithNestedField) {
			  testNestedInputField(data: $a)
			}`, "", `
			mutation testNestedInputField($a: InputWithNestedField) {
  				testNestedInputField(data: $a)
			}`, `{"a":{"nested":{}}}`, `{"a":{"nested":{"secondField":"ValueOne"}}}`)
	})

	t.Run("multiple variables for operation", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation combinedMutation($a: SimpleTestInput, $b: InputWithNestedField) {
  				testDefaultValueSimple(data: $a)
  				testNestedInputField(data: $b)
			}`, "", `
			mutation combinedMutation($a: SimpleTestInput, $b: InputWithNestedField) {
  				testDefaultValueSimple(data: $a)
  				testNestedInputField(data: $b)
			}`, `{"b":{"nested":{}},"a":{"firstField":"test"}}`,
			`{"b":{"nested":{"secondField":"ValueOne"}},"a":{"firstField":"test","secondField":1}}`,
		)
	})

	t.Run("default field object is partial", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation($a: InputWithDefaultFieldsNested){
				mutationNestedMissing(in: $a)
			}`, "", `
			mutation($a: InputWithDefaultFieldsNested){
				mutationNestedMissing(in: $a)
			}`, `{"a":{"first":"test"}}`, `{"a":{"first":"test","nested":{"firstField":0,"secondField":"ValueOne"}}}`)
	})

	t.Run("variable for input field as object is partial", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation($a: InputWithDefaultFieldsNested){
				mutationNestedMissing(in: $a)
			}`, "", `
			mutation($a: InputWithDefaultFieldsNested){
				mutationNestedMissing(in: $a)
			}`, `{"a":{"first":"test","nested":{"firstField":1}}}`, `{"a":{"first":"test","nested":{"firstField":1,"secondField":"ValueOne"}}}`)
	})

	t.Run("run with extract variables", func(t *testing.T) {
		runWithVariables(t, extractVariables, testInputDefaultSchema, `
		mutation {
  			testNestedInputField(data: { nested: { firstField: 1 } })
		}`, "", `
		mutation($a: InputWithNestedField!) {
  				testNestedInputField(data: $a)
		}`, "", `{"a":{"nested":{"firstField":1,"secondField":"ValueOne"}}}`, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		})
	})

	t.Run("list default value", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation mutationWithListInput($a: InputHasList) {
			  mutationWithListInput(data: $a)
			}`, "", `
			mutation mutationWithListInput($a: InputHasList) {
			  mutationWithListInput(data: $a)
			}
`, `{"a":{}}`, `{"a":{"firstList":[{"firstField":1,"secondField":"ValueOne"},{"firstField":1,"secondField":"ValueOne"}]}}`)
	})

	t.Run("list object partial value", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation mutationWithListInput($a: InputHasList) {
			  mutationWithListInput(data: $a)
			}`, "", `
			mutation mutationWithListInput($a: InputHasList) {
			  mutationWithListInput(data: $a)
			}
`, `{"a":{"firstList":[{"firstField":10}]}}`, `{"a":{"firstList":[{"firstField":10,"secondField":"ValueOne"}]}}`)
	})

	t.Run("nested list default value", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation mutationWithMultiNestedInput($a: MultiNestedInput) {
			  mutationWithMultiNestedInput(data: $a)
			}`, "", `
			mutation mutationWithMultiNestedInput($a: MultiNestedInput) {
			  mutationWithMultiNestedInput(data: $a)
			}
`, `{"a":{}}`, `{"a":{"nested":[[{"firstField":1,"secondField":"ValueOne"}]]}}`)
	})

	t.Run("complex nested list partial in variable", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation mutationComplexNestedListInput($a: ComplexNestedListInput) {
			  mutationComplexNestedListInput(data: $a)
			}`, "", `
			mutation mutationComplexNestedListInput($a: ComplexNestedListInput) {
			  mutationComplexNestedListInput(data: $a)
			}
`, `{"a":{"nested":[[[{"firstField":2}]]]}}`, `{"a":{"nested":[[[{"firstField":2,"secondField":"ValueOne"}]]]}}`)
	})

	t.Run("simple list nested input", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation mutationSimpleInputList($a: [SimpleTestInput]) {
			  mutationSimpleInputList(data: $a)
			}`, "", `
			mutation mutationSimpleInputList($a: [SimpleTestInput]) {
			  mutationSimpleInputList(data: $a)
			}`, `{"a":[{"thirdField":1}]}`, `{"a":[{"thirdField":1,"firstField":"firstField","secondField":1}]}`)
	})

	t.Run("use custom scalar variable", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation mutationUseCustomScalar($a: CustomScalar) {
			  mutationUseCustomScalar(in: $a)
			}`, "", `
			mutation mutationUseCustomScalar($a: CustomScalar) {
			  mutationUseCustomScalar(in: $a)
			}`, `{"a":{}}`, `{"a":{}}`)
	})

	t.Run("custom scalar variable list", func(t *testing.T) {
		runWithVariablesAssert(t, func(walker *astvisitor.Walker) {
			injectInputFieldDefaults(walker)
		}, testInputDefaultSchema, `
			mutation mutationUseCustomScalarList($a: [CustomScalar!]) {
			  mutationUseCustomScalarList(in: $a)
			}`, "", `
			mutation mutationUseCustomScalarList($a: [CustomScalar!]) {
			  mutationUseCustomScalarList(in: $a)
			}`, `{"a":[{"test": "testval"}]}`, `{"a":[{"test": "testval"}]}`)
	})
}
