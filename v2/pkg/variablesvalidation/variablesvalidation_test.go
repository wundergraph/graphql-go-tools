package variablesvalidation

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/apollocompatibility"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/errorcodes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestVariablesValidation(t *testing.T) {
	t.Run("required field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: String!): String }`,
			operation: `query Foo($bar: String!) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required field argument not provided - with mapping", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: String!): String }`,
			operation: `query Foo($bar: String!) { hello }`,
			variables: `{}`,
			mapping:   map[string]string{"bar": "bazz"},
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bazz" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("a missing required input produces an error", func(t *testing.T) {
		tc := testCase{
			schema:    inputSchema,
			operation: `query Foo($input: SelfSatisfiedInput!) { satisfied }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" of required type "SelfSatisfiedInput!" was not provided.`, err.Error())
	})

	t.Run("provided required input fields with default values do not produce validation errors", func(t *testing.T) {
		tc := testCase{
			schema:    inputSchema,
			operation: `query Foo($input: SelfSatisfiedInput!) { satisfied(input: $input) }`,
			variables: `{ "input": { } }`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("unprovided required input fields without default values produce validation errors #1 - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    inputSchema,
			operation: `query Foo($input: SelfUnsatisfiedInput!) { unsatisfied(input: $input) }`,
			variables: `{ "input": { } }`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {}; Field "nested" of required type "NestedSelfSatisfiedInput!" was not provided.`, err.Error())
	})

	t.Run("unprovided required input fields without default values produce validation errors #1", func(t *testing.T) {
		tc := testCase{
			schema:    inputSchema,
			operation: `query Foo($input: SelfUnsatisfiedInput!) { unsatisfied(input: $input) }`,
			variables: `{ "input": { } }`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value; Field "nested" of required type "NestedSelfSatisfiedInput!" was not provided.`, err.Error())
	})

	t.Run("unprovided required input fields without default values produce validation errors #2 - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    inputSchema,
			operation: `query Foo($input: SelfUnsatisfiedInput!) { unsatisfied(input: $input) }`,
			variables: `{ "input": { "nested": { }, "value": "string" } }`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"nested":{},"value":"string"}; Field "secondNested" of required type "NestedSelfSatisfiedInput!" was not provided.`, err.Error())
	})

	t.Run("unprovided required input fields without default values produce validation errors #2", func(t *testing.T) {
		tc := testCase{
			schema:    inputSchema,
			operation: `query Foo($input: SelfUnsatisfiedInput!) { unsatisfied(input: $input) }`,
			variables: `{ "input": { "nested": { }, "value": "string" } }`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value; Field "secondNested" of required type "NestedSelfSatisfiedInput!" was not provided.`, err.Error())
	})

	t.Run("provided but empty nested required inputs with default values do not produce validation errors", func(t *testing.T) {
		tc := testCase{
			schema:    inputSchema,
			operation: `query Foo($input: SelfUnsatisfiedInput!) { unsatisfied(input: $input) }`,
			variables: `{ "input": { "nested": { }, "secondNested": { } } }`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("not required field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: String): String }`,
			operation: `query Foo($bar: String) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("required field argument provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: String!): String }`,
			operation: `query Foo($bar: String!) { hello(arg: $bar) }`,
			variables: `{"bar": "world"}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("nested argument is value instead of list - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(input: Input): String } input Input { bar: [String]! }`,
			operation: `query Foo($input: Input) { hello(input: $input) }`,
			variables: `{"input":{"bar":"world"}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.NotNil(t, err)
		assert.Equal(t, `Variable "$input" got invalid value "world" at "input.bar"; Got input type "String", want: "[String]"`, err.Error())
	})

	t.Run("nested argument is value instead of list", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(input: Input): String } input Input { bar: [String]! }`,
			operation: `query Foo($input: Input) { hello(input: $input) }`,
			variables: `{"input":{"bar":"world"}}`,
		}
		err := runTest(t, tc)
		require.NotNil(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; Got input type "String", want: "[String]"`, err.Error())
	})

	t.Run("nested argument is value instead of list - with mapping", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(input: Input): String } input Input { bar: [String]! }`,
			operation: `query Foo($input: Input) { hello(input: $input) }`,
			variables: `{"honeypot":{"bar":"world"}}`,
			mapping:   map[string]string{"input": "honeypot"},
		}
		err := runTest(t, tc)
		require.NotNil(t, err)
		assert.Equal(t, `Variable "$honeypot" got invalid value at "honeypot.bar"; Got input type "String", want: "[String]"`, err.Error())
	})

	t.Run("nested enum argument is value instead of list - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(input: Input): String } input Input { bar: [MyNum]! } enum MyNum { ONE TWO }`,
			operation: `query Foo($input: Input) { hello(input: $input) }`,
			variables: `{"input":{"bar":"ONE"}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.NotNil(t, err)
		assert.Equal(t, `Variable "$input" got invalid value "ONE" at "input.bar"; Got input type "MyNum", want: "[MyNum]"`, err.Error())
	})

	t.Run("nested enum argument is value instead of list", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(input: Input): String } input Input { bar: [MyNum]! } enum MyNum { ONE TWO }`,
			operation: `query Foo($input: Input) { hello(input: $input) }`,
			variables: `{"input":{"bar":"ONE"}}`,
		}
		err := runTest(t, tc)
		require.NotNil(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; Got input type "MyNum", want: "[MyNum]"`, err.Error())
	})

	t.Run("required field argument of custom scalar type not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Baz!): String } scalar Baz`,
			operation: `query Foo($bar: Baz!) { hello(arg: $bar) }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$bar" of required type "Baz!" was not provided.`, err.Error())
	})

	t.Run("required field argument of custom scalar type was null", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Baz!): String } scalar Baz`,
			operation: `query Foo($bar: Baz!) { hello(arg: $bar) }`,
			variables: `{"bar":null}`,
		}
		err := runTest(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value null; Expected non-nullable type "Baz!" not to be null.`, err.Error())
	})

	t.Run("required nested field field argument of custom scalar not provided - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Foo!): String } input Foo { bar: Baz! } scalar Baz`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value {}; Field "bar" of required type "Baz!" was not provided.`, err.Error())
	})

	t.Run("required nested field field argument of custom scalar not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Foo!): String } input Foo { bar: Baz! } scalar Baz`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{}}`,
		}
		err := runTest(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Field "bar" of required type "Baz!" was not provided.`, err.Error())
	})

	t.Run("required nested field field argument of custom scalar not provided - with mapping", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Foo!): String } input Foo { bar: Baz! } scalar Baz`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"abc":{}}`,
			mapping:   map[string]string{"bar": "abc"},
		}
		err := runTest(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$abc" got invalid value; Field "bar" of required type "Baz!" was not provided.`, err.Error())
	})

	t.Run("required nested field field argument of custom scalar was null - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Foo!): String } input Foo { bar: Baz! } scalar Baz`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{"bar":null}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value {"bar":null}; Field "bar" of required type "Baz!" was not provided.`, err.Error())
	})

	t.Run("required nested field field argument of custom scalar was null", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Foo!): String } input Foo { bar: Baz! } scalar Baz`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{"bar":null}}`,
		}
		err := runTest(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Field "bar" of required type "Baz!" was not provided.`, err.Error())
	})

	t.Run("required field argument provided with default value", func(t *testing.T) {
		tc := testCase{
			schema:            `type Query { hello(arg: String!): String }`,
			operation:         `query Foo($bar: String! = "world") { hello(arg: $bar) }`,
			variables:         `{}`,
			withNormalization: true,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("required Int field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Int!): String }`,
			operation: `query Foo($bar: Int!) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" of required type "Int!" was not provided.`, err.Error())
	})

	t.Run("required Float field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Float!): String }`,
			operation: `query Foo($bar: Float!) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" of required type "Float!" was not provided.`, err.Error())
	})

	t.Run("required Boolean field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Boolean!): String }`,
			operation: `query Foo($bar: Boolean!) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" of required type "Boolean!" was not provided.`, err.Error())
	})

	t.Run("required ID field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: ID!): String }`,
			operation: `query Foo($bar: ID!) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" of required type "ID!" was not provided.`, err.Error())
	})

	t.Run("required ID field argument provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: ID!): String }`,
			operation: `query Foo($bar: ID!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("required ID field argument provided with String", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: ID!): String }`,
			operation: `query Foo($bar: ID!) { hello(arg: $bar) }`,
			variables: `{"bar":"hello"}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("required Enum field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" of required type "Foo!" was not provided.`, err.Error())
	})

	t.Run("required input object field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" of required type "Foo!" was not provided.`, err.Error())
	})

	t.Run("required string list field argument not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: [String]!): String }`,
			operation: `query Foo($bar: [String]!) { hello }`,
			variables: `{}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" of required type "[String]!" was not provided.`, err.Error())
	})

	t.Run("wrong Boolean value for input object field - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":true}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value true; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("wrong Boolean value for input object field", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":true}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("wrong Integer value for input object field - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("wrong Integer value for input object field", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("required field on present input object not provided - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value {}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required field on present input object not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required field on present input object provided with correct type", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{"bar":"hello"}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("required field on present input object provided with wrong type - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required field on present input object provided with wrong type", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; String cannot represent a non string value`, err.Error())
	})

	t.Run("required field on present input object not provided - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required field on present input object not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on input object provided with null - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":null}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"bar":null}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on input object provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":null}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on input object provided with Int - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string field on input object provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; String cannot represent a non string value`, err.Error())
	})

	t.Run("required string field on input object provided with Float - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123.456}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123.456 at "input.bar"; String cannot represent a non string value: 123.456`, err.Error())
	})

	t.Run("required string field on input object provided with Float", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123.456}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; String cannot represent a non string value`, err.Error())
	})

	t.Run("required string field on input object provided with Boolean - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":true}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value true at "input.bar"; String cannot represent a non string value: true`, err.Error())
	})

	t.Run("required string field on input object provided with Boolean", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":true}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; String cannot represent a non string value`, err.Error())
	})

	t.Run("required string field on nested input object not provided - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{}}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":{}}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on nested input object not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on nested input object provided with null - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":null}}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":{"bar":null}}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on nested input object provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":null}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on nested input object provided with Int - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":123}}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.foo.bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string field on nested input object provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":123}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.foo.bar"; String cannot represent a non string value`, err.Error())
	})

	t.Run("required string field on nested input object array provided with Int - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: [Foo!]! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":[{"bar":123}]}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.foo.[0].bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string field on nested input object array provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: [Foo!]! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":[{"bar":123}]}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.foo.[0].bar"; String cannot represent a non string value`, err.Error())
	})

	t.Run("required string field on nested input object array index 1 provided with Int - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: [Foo!]! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":[{"bar":"hello"},{"bar":123}]}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.foo.[1].bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string field on nested input object array index 1 provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: [Foo!]! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":[{"bar":"hello"},{"bar":123}]}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.foo.[1].bar"; String cannot represent a non string value`, err.Error())
	})

	t.Run("non existing field on nested input object - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":"hello","baz":"world"}}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":{"bar":"hello","baz":"world"}} at "input.foo"; Field "baz" is not defined by type "Foo".`, err.Error())
	})

	t.Run("non existing field on nested input object", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":"hello","baz":"world"}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.foo"; Field "baz" is not defined by type "Foo".`, err.Error())
	})

	t.Run("required enum argument provided with correct value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":"BAR"}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("required enum argument provided with wrong value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":"BAZ"}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value "BAZ"; Value "BAZ" does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("required enum argument provided with wrong value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":"BAZ"}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Value does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("required enum argument provided with Int value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123; Enum "Foo" cannot represent non-string value: 123.`, err.Error())
	})

	t.Run("required enum argument provided with Int value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Enum "Foo" cannot represent non-string value.`, err.Error())
	})

	t.Run("required enum argument provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":null}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value null; Expected non-nullable type "Foo!" not to be null.`, err.Error())
	})

	t.Run("required nested enum argument provided with null - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":null}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":null}; Field "foo" of required type "Foo!" was not provided.`, err.Error())
	})

	t.Run("required nested enum argument provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":null}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value; Field "foo" of required type "Foo!" was not provided.`, err.Error())
	})

	t.Run("required nested enum argument provided with correct value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":"BAR"}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("required nested enum argument provided with wrong value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":"BAZ"}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":"BAZ"} at "input.foo"; Value "BAZ" does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("required nested enum argument provided with wrong value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":"BAZ"}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.foo"; Value does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("optional enum argument provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($bar: Foo) { hello(arg: $bar) }`,
			variables: `{"bar":null}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("optional nested enum argument provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo } type Query { hello(arg: Bar): String }`,
			operation: `query Foo($input: Bar) { hello(arg: $input) }`,
			variables: `{"input":{"foo":null}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("optional nested enum argument provided with incorrect value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo } type Query { hello(arg: Bar): String }`,
			operation: `query Foo($input: Bar) { hello(arg: $input) }`,
			variables: `{"input":{"foo":"BAZ"}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":"BAZ"} at "input.foo"; Value "BAZ" does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("optional nested enum argument provided with incorrect value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo } type Query { hello(arg: Bar): String }`,
			operation: `query Foo($input: Bar) { hello(arg: $input) }`,
			variables: `{"input":{"foo":"BAZ"}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.foo"; Value does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("optional enum argument provided with correct value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($bar: Foo) { hello(arg: $bar) }`,
			variables: `{"bar":"BAR"}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("optional enum argument provided with wrong value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($bar: Foo) { hello(arg: $bar) }`,
			variables: `{"bar":"BAZ"}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value "BAZ"; Value "BAZ" does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("optional enum argument provided with wrong value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($bar: Foo) { hello(arg: $bar) }`,
			variables: `{"bar":"BAZ"}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Value does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("required string list field argument provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: [String]!): String }`,
			operation: `query Foo($bar: [String]!) { hello(arg: $bar) }`,
			variables: `{"bar":null}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value null; Expected non-nullable type "[String]!" not to be null.`, err.Error())
	})

	t.Run("required string list field argument provided with non list Int value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:            `type Query { hello(arg: [String]!): String }`,
			operation:         `query Foo($bar: [String]!) { hello(arg: $bar) }`,
			variables:         `{"bar":123}`,
			withNormalization: true,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123 at "bar.[0]"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string list field argument provided with non list Int value", func(t *testing.T) {
		tc := testCase{
			schema:            `type Query { hello(arg: [String]!): String }`,
			operation:         `query Foo($bar: [String]!) { hello(arg: $bar) }`,
			variables:         `{"bar":123}`,
			withNormalization: true,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value at "bar.[0]"; String cannot represent a non string value`, err.Error())
	})

	t.Run("required string argument on input object list provided with correct value", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: [Foo!]!): String }`,
			operation: `query Foo($bar: [Foo!]!) { hello(arg: $bar) }`,
			variables: `{"bar":[{"bar":"hello"}]}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("required string argument on input object list provided with wrong value", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: [Foo!]!): String }`,
			operation: `query Foo($bar: [Foo!]!) { hello(arg: $bar) }`,
			variables: `{"bar":[{"bar":123}]}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value at "bar.[0].bar"; String cannot represent a non string value`, err.Error())
	})

	t.Run("required string argument on input object list provided with wrong value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: [Foo!]!): String }`,
			operation: `query Foo($bar: [Foo!]!) { hello(arg: $bar) }`,
			variables: `{"bar":[{"bar":123}]}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123 at "bar.[0].bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string argument provided with string list", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: String!): String }`,
			operation: `query Foo($bar: String!) { hello(arg: $bar) }`,
			variables: `{"bar":["hello"]}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; String cannot represent a non string value`, err.Error())
	})

	t.Run("required input object list field argument provided with non list Int value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: [Foo!]!): String }`,
			operation: `query Foo($bar: [Foo!]!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("required input object list field argument provided with non list Int value", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: [Foo!]!): String }`,
			operation: `query Foo($bar: [Foo!]!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("required input object field argument provided with list input object value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":[{"bar":"hello"}]}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value [{"bar":"hello"}]; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("required input object field argument provided with list input object value", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":[{"bar":"hello"}]}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("required enum list argument provided with non list Int value - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:            `enum Foo { BAR } type Query { hello(arg: [Foo]!): String }`,
			operation:         `query Foo($bar: [Foo]!) { hello(arg: $bar) }`,
			variables:         `{"bar":123}`,
			withNormalization: true,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123 at "bar.[0]"; Enum "Foo" cannot represent non-string value: 123.`, err.Error())
	})

	t.Run("required enum list argument provided with non list Int value", func(t *testing.T) {
		tc := testCase{
			schema:            `enum Foo { BAR } type Query { hello(arg: [Foo]!): String }`,
			operation:         `query Foo($bar: [Foo]!) { hello(arg: $bar) }`,
			variables:         `{"bar":123}`,
			withNormalization: true,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value at "bar.[0]"; Enum "Foo" cannot represent non-string value.`, err.Error())
	})

	t.Run("required string list field argument provided with Int - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:            `type Query { hello(arg: [String]!): String }`,
			operation:         `query Foo($bar: [String]!) { hello(arg: $bar) }`,
			variables:         `{"bar":123}`,
			withNormalization: true,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123 at "bar.[0]"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string list field argument provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:            `type Query { hello(arg: [String]!): String }`,
			operation:         `query Foo($bar: [String]!) { hello(arg: $bar) }`,
			variables:         `{"bar":123}`,
			withNormalization: true,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value at "bar.[0]"; String cannot represent a non string value`, err.Error())
	})

	t.Run("optional nested list argument provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bars : [String] bat: Int! } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bars":null,"bat":1}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("optional nested list argument provided with empty list", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bars : [String] bat: Int! } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bars":[],"bat":1}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("optional nested list argument provided with empty list and missing Int - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bars : [String] bat: Int! } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bars":[]}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"bars":[]}; Field "bat" of required type "Int!" was not provided.`, err.Error())
	})

	t.Run("optional nested list argument provided with empty list and missing Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bars : [String] bat: Int! } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bars":[]}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value; Field "bat" of required type "Int!" was not provided.`, err.Error())
	})

	t.Run("optional nested field is null followed by required nested field of wrong type - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo bat: Int! } type Query { hello(arg: Bar): String }`,
			operation: `query Foo($input: Bar) { hello(arg: $input) }`,
			variables: `{"input":{"foo":null,"bat":"hello"}}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value "hello" at "input.bat"; Int cannot represent non-integer value: "hello"`, err.Error())
	})

	t.Run("optional nested field is null followed by required nested field of wrong type", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo bat: Int! } type Query { hello(arg: Bar): String }`,
			operation: `query Foo($input: Bar) { hello(arg: $input) }`,
			variables: `{"input":{"foo":null,"bat":"hello"}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bat"; Int cannot represent non-integer value`, err.Error())
	})

	t.Run("input field is a double nested list", func(t *testing.T) {
		tc := testCase{
			schema:    `input Filter { option: String! } input FilterWrapper { filters: [[Filter!]!] } type Query { hello(filter: FilterWrapper): String }`,
			operation: `query Foo($input: FilterWrapper) { hello(filter: $input) }`,
			variables: `{"input":{"filters":[[{"option": "a"}]]}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("variable of double nested list type", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(filter: [[String]]): String }`,
			operation: `query Foo($input: [[String]]) { hello(filter: $input) }`,
			variables: `{"input":[["value"]]}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("triple nested value into variable of double nested list type - with variable content", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(filter: [[String]]): String }`,
			operation: `query Foo($input: [[String]]) { hello(filter: $input) }`,
			variables: `{"input":[[["value"]]]}`,
		}
		err := runTestWithVariablesContentEnabled(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value ["value"] at "input.[0].[0]"; String cannot represent a non string value: ["value"]`, err.Error())
	})

	t.Run("triple nested value into variable of double nested list type", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(filter: [[String]]): String }`,
			operation: `query Foo($input: [[String]]) { hello(filter: $input) }`,
			variables: `{"input":[[["value"]]]}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.[0].[0]"; String cannot represent a non string value`, err.Error())
	})

	t.Run("null into non required list value", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(filter: [String]): String }`,
			operation: `query Foo($input: [String]) { hello(filter: $input) }`,
			variables: `{"input":[null]}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("value and null into non required list value", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(filter: [String]): String }`,
			operation: `query Foo($input: [String]) { hello(filter: $input) }`,
			variables: `{"input":["ok", null]}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})

	t.Run("null into non required value", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(filter: String): String }`,
			operation: `query Foo($input: String) { hello(filter: $input) }`,
			variables: `{"input":null}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("extension code is propagated with apollo compatibility flag", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(filter: String!): String }`,
			operation: `query Foo($input: String!) { hello(filter: $input) }`,
			variables: `{"input":null}`,
		}
		err := runTestWithOptions(t, tc, VariablesValidatorOptions{
			ApolloCompatibilityFlags: apollocompatibility.Flags{
				ReplaceInvalidVarError: true,
			},
		})
		assert.Equal(t, &InvalidVariableError{
			ExtensionCode: errorcodes.BadUserInput,
			Message:       `Variable "$input" got invalid value null; Expected non-nullable type "String!" not to be null.`,
		}, err)
	})

	t.Run("extension code is propagated with apollo compatibility flag", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(filter: String!): String }`,
			operation: `query Foo($input: String!) { hello(filter: $input) }`,
			variables: `{"input":null}`,
		}
		err := runTestWithOptions(t, tc, VariablesValidatorOptions{
			ApolloRouterCompatabilityFlags: apollocompatibility.RouterFlags{
				ReplaceInvalidVarError: true,
			},
		})
		assert.Equal(t, &InvalidVariableError{
			ExtensionCode: errorcodes.BadUserInput,
			Message:       `invalid type for variable: 'input'`,
		}, err)
	})

	t.Run("optional Int input object field provided with 1", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: Int } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":1}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("optional Float input object field provided with 1.1", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: Float } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":1.1}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("optional Float input object field provided with true", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: Float } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":true}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; Float cannot represent non numeric value`, err.Error())
	})
	t.Run("optional Boolean input object field provided with true", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: Boolean } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":true}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("optional Boolean input object field provided with false", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: Boolean } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":false}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("optional Boolean input object field provided with 1", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: Boolean } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":1}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; Boolean cannot represent a non boolean value`, err.Error())
	})
	t.Run("optional ID input object field provided with 1", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: ID } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":1}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("optional ID input object field provided with string 123", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: ID } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":"123"}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("optional ID input object field provided with string hello", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: ID } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":"hello"}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("optional ID input object field provided with true", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: ID } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":true}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.bar"; ID cannot represent a non-string and non-integer value`, err.Error())
	})
	t.Run("optional ID input object field provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: ID } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bar":null}}`,
		}
		err := runTest(t, tc)
		require.NoError(t, err)
	})
	t.Run("nested input object on nested input object provided with incorrect value 1", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":1}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value at "input.foo.bar"; String cannot represent a non string value`, err.Error())
	})
}

type testCase struct {
	schema, operation, variables string
	withNormalization            bool
	mapping                      map[string]string
}

func runTest(t *testing.T, tc testCase) error {
	return runTestWithOptions(t, tc, VariablesValidatorOptions{DisableExposingVariablesContent: true})
}

func runTestWithVariablesContentEnabled(t *testing.T, tc testCase) error {
	return runTestWithOptions(t, tc, VariablesValidatorOptions{DisableExposingVariablesContent: false})
}

func runTestWithOptions(t *testing.T, tc testCase, options VariablesValidatorOptions) error {
	t.Helper()
	def := unsafeparser.ParseGraphqlDocumentString(tc.schema)
	op := unsafeparser.ParseGraphqlDocumentString(tc.operation)
	op.Input.Variables = []byte(tc.variables)
	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	if err != nil {
		t.Fatal(err)
	}
	if tc.withNormalization {
		report := &operationreport.Report{}
		norm := astnormalization.NewNormalizer(true, true)
		norm.NormalizeOperation(&op, &def, report)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}
	}
	validator := NewVariablesValidator(options)

	return validator.ValidateWithRemap(&op, &def, op.Input.Variables, tc.mapping)
}

var inputSchema = `
	type Query {
		satisfied(input: SelfSatisfiedInput!): Boolean
		unsatisfied(input: SelfUnsatisfiedInput!): Boolean
	}
	
	input NestedSelfSatisfiedInput {
		a: String
		b: Int! = 1
	}
	
	input SelfSatisfiedInput {
		nested: NestedSelfSatisfiedInput
		value: String
	}

	input SelfUnsatisfiedInput {
		nested: NestedSelfSatisfiedInput!
		secondNested: NestedSelfSatisfiedInput!
		value: String
	}
`
