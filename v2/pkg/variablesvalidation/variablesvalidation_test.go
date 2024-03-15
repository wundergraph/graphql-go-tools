package variablesvalidation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/asttransform"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
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

	t.Run("required nested field field argument of custom scalar not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Foo!): String } input Foo { bar: Baz! } scalar Baz`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{}}`,
		}
		err := runTest(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value {}; Field "bar" of required type "Baz!" was not provided.`, err.Error())
	})

	t.Run("required nested field field argument of custom scalar was null", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: Foo!): String } input Foo { bar: Baz! } scalar Baz`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{"bar":null}}`,
		}
		err := runTest(t, tc)
		assert.NotNil(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value {"bar":null}; Field "bar" of required type "Baz!" was not provided.`, err.Error())
	})

	t.Run("required field argument provided with default value", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: String!): String }`,
			operation: `query Foo($bar: String! = "world") { hello(arg: $bar) }`,
			variables: `{}`,
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

	t.Run("wrong Boolean value for input object field", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":true}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value true; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("wrong Integer value for input object field", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("required field on present input object not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":{}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value {}; Field "bar" of required type "String!" was not provided.`, err.Error())
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

	t.Run("required field on present input object provided with wrong type", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required field on present input object not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on input object provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":null}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"bar":null}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on input object provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string field on input object provided with Float", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":123.456}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123.456 at "input.bar"; String cannot represent a non string value: 123.456`, err.Error())
	})

	t.Run("required string field on input object provided with Boolean", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($input: Foo!) { hello(arg: $input) }`,
			variables: `{"input":{"bar":true}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value true at "input.bar"; String cannot represent a non string value: true`, err.Error())
	})

	t.Run("required string field on nested input object not provided", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":{}}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on nested input object provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":null}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":{"bar":null}}; Field "bar" of required type "String!" was not provided.`, err.Error())
	})

	t.Run("required string field on nested input object provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":123}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.foo.bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string field on nested input object array provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: [Foo!]! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":[{"bar":123}]}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.foo.[0].bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("required string field on nested input object array index 1 provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: [Foo!]! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":[{"bar":"hello"},{"bar":123}]}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value 123 at "input.foo.[1].bar"; String cannot represent a non string value: 123`, err.Error())
	})

	t.Run("non existing field on nested input object", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":{"bar":"hello","baz":"world"}}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":{"bar":"hello","baz":"world"}} at "input.foo"; Field "baz" is not defined by type "Foo".`, err.Error())
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

	t.Run("required enum argument provided with wrong value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":"BAZ"}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value "BAZ"; Value "BAZ" does not exist in "Foo" enum.`, err.Error())
	})

	t.Run("required enum argument provided with Int value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123; Enum "Foo" cannot represent non-string value: 123.`, err.Error())
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

	t.Run("required nested enum argument provided with null", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":null}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":null}; Field "foo" of required type "Foo!" was not provided.`, err.Error())
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

	t.Run("required nested enum argument provided with wrong value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo! } type Query { hello(arg: Bar!): String }`,
			operation: `query Foo($input: Bar!) { hello(arg: $input) }`,
			variables: `{"input":{"foo":"BAZ"}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":"BAZ"} at "input.foo"; Value "BAZ" does not exist in "Foo" enum.`, err.Error())
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

	t.Run("optional nested enum argument provided with incorrect value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } input Bar { foo: Foo } type Query { hello(arg: Bar): String }`,
			operation: `query Foo($input: Bar) { hello(arg: $input) }`,
			variables: `{"input":{"foo":"BAZ"}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"foo":"BAZ"} at "input.foo"; Value "BAZ" does not exist in "Foo" enum.`, err.Error())
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

	t.Run("optional enum argument provided with wrong value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($bar: Foo) { hello(arg: $bar) }`,
			variables: `{"bar":"BAZ"}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value "BAZ"; Value "BAZ" does not exist in "Foo" enum.`, err.Error())
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

	t.Run("required string list field argument provided with non list Int value", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: [String]!): String }`,
			operation: `query Foo($bar: [String]!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123 at "bar.[0]"; String cannot represent a non string value: 123`, err.Error())
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
		assert.Equal(t, `Variable "$bar" got invalid value ["hello"]; String cannot represent a non string value: ["hello"]`, err.Error())
	})

	t.Run("required input object list field argument provided with non list Int value", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: [Foo!]!): String }`,
			operation: `query Foo($bar: [Foo!]!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("required input object field argument provided with list input object value", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } type Query { hello(arg: Foo!): String }`,
			operation: `query Foo($bar: Foo!) { hello(arg: $bar) }`,
			variables: `{"bar":[{"bar":"hello"}]}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value [{"bar":"hello"}]; Expected type "Foo" to be an object.`, err.Error())
	})

	t.Run("required enum list argument provided with non list Int value", func(t *testing.T) {
		tc := testCase{
			schema:    `enum Foo { BAR } type Query { hello(arg: [Foo]!): String }`,
			operation: `query Foo($bar: [Foo]!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123 at "bar.[0]"; Enum "Foo" cannot represent non-string value: 123.`, err.Error())
	})

	t.Run("required string list field argument provided with Int", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: [String]!): String }`,
			operation: `query Foo($bar: [String]!) { hello(arg: $bar) }`,
			variables: `{"bar":123}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$bar" got invalid value 123 at "bar.[0]"; String cannot represent a non string value: 123`, err.Error())
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

	t.Run("optional nested list argument provided with empty list and missing Int", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bars : [String] bat: Int! } type Query { hello(arg: Foo): String }`,
			operation: `query Foo($input: Foo) { hello(arg: $input) }`,
			variables: `{"input":{"bars":[]}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value {"bars":[]}; Field "bat" of required type "Int!" was not provided.`, err.Error())
	})

	t.Run("optional nested field is null followed by required nested field of wrong type", func(t *testing.T) {
		tc := testCase{
			schema:    `input Foo { bar: String! } input Bar { foo: Foo bat: Int! } type Query { hello(arg: Bar): String }`,
			operation: `query Foo($input: Bar) { hello(arg: $input) }`,
			variables: `{"input":{"foo":null,"bat":"hello"}}`,
		}
		err := runTest(t, tc)
		require.Error(t, err)
		assert.Equal(t, `Variable "$input" got invalid value "hello" at "input.bat"; Int cannot represent non-integer value: "hello"`, err.Error())
	})
}

type testCase struct {
	schema, operation, variables string
}

func runTest(t *testing.T, tc testCase) error {
	t.Helper()
	def := unsafeparser.ParseGraphqlDocumentString(tc.schema)
	op := unsafeparser.ParseGraphqlDocumentString(tc.operation)
	op.Input.Variables = []byte(tc.variables)
	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	if err != nil {
		t.Fatal(err)
	}
	report := &operationreport.Report{}
	norm := astnormalization.NewNormalizer(true, true)
	norm.NormalizeOperation(&op, &def, report)
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
	validator := NewVariablesValidator()
	return validator.Validate(&op, &def, op.Input.Variables)
}
