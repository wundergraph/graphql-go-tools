package uploads

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestUploadsFinder(t *testing.T) {
	t.Run("no uploads in a query", func(t *testing.T) {
		tc := testCase{
			schema:    `type Query { hello(arg: String!): String }`,
			operation: `query Foo($bar: String!) { hello }`,
			variables: `{}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Nil(t, paths)
	})

	t.Run("query with upload in a variable", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload type Query { hello(arg: Upload!): String }`,
			operation: `query Foo($bar: Upload!) { hello(arg: $bar) }`,
			variables: `{"bar": null}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.bar"}, paths)
	})

	t.Run("mutation with upload in a variable", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload type Mutation { hello(arg: Upload!): String }`,
			operation: `mutation Foo($bar: Upload!) { hello(arg: $bar) }`,
			variables: `{"bar": null}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.bar"}, paths)
	})

	t.Run("mutation with upload in a variable named file", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload type Mutation { hello(arg: Upload!): String }`,
			operation: `mutation Foo($file: Upload!) { hello(arg: $file) }`,
			variables: `{"file": null}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.file"}, paths)
	})

	t.Run("mutation with multiple uploads in a different args", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload type Mutation { hello(arg: Upload! arg2: Upload!): String }`,
			operation: `mutation Foo($file: Upload! $file2: Upload!) { hello(arg: $file arg2: $file2) }`,
			variables: `{"file": null,"file2": null}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.file", "variables.file2"}, paths)
	})

	t.Run("mutation with multiple uploads in a list", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload type Mutation { hello(arg: [Upload!]!): String }`,
			operation: `mutation Foo($files: [Upload!]!) { hello(arg: $files) }`,
			variables: `{"files": [null, null]}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.files.0", "variables.files.1"}, paths)
	})

	t.Run("mutation with multiple uploads in a different args", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload type Mutation { hello(arg: [Upload!]! arg2: [Upload!]!): String }`,
			operation: `mutation Foo($files: [Upload!]!, $files2: [Upload!]!) { hello(arg: $files arg2: $files) }`,
			variables: `{"files": [null, null], "files2": [null, null]}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.files.0", "variables.files.1", "variables.files2.0", "variables.files2.1"}, paths)
	})

	t.Run("mutation with nested upload", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload input Input {f: Upload!} type Mutation { hello(arg: Input!): String }`,
			operation: `mutation Foo($i: Input!) { hello(arg: $i) }`,
			variables: `{"i":{"f":null}}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.i.f"}, paths)
	})

	t.Run("mutation with nested uploads", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload input Input {f: Upload! f2: Upload!} type Mutation { hello(arg: Input!): String }`,
			operation: `mutation Foo($i: Input!) { hello(arg: $i) }`,
			variables: `{"i":{"f":null,"f2":null}}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.i.f", "variables.i.f2"}, paths)
	})

	t.Run("mutation with nested upload in lists", func(t *testing.T) {
		tc := testCase{
			schema:    `scalar Upload input Input {f: [Upload!]! f2: [Upload!]!} type Mutation { hello(arg: Input!): String }`,
			operation: `mutation Foo($i: Input!) { hello(arg: $i) }`,
			variables: `{"i":{"f":[null],"f2":[null,null,null]}}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{"variables.i.f.0", "variables.i.f2.0", "variables.i.f2.1", "variables.i.f2.2"}, paths)
	})

	t.Run("mutation with deeply nested uploads in lists", func(t *testing.T) {
		tc := testCase{
			schema: `
				scalar Upload
				input Input {list: [Upload!]! value: Upload!}
				input Input2 {oneList: [Input!]! one: Input!}
				input Input3 {twoList: [Input2!]! two: Input2!}
				type Mutation { hello(arg: Input3!): String }`,
			operation: `mutation Foo($arg: Input3!) { hello(arg: $arg) }`,
			variables: `
				{
					"arg": {
						"twoList": [
							{
								"oneList": [
									{
										"list": [
											null,
											null
										],
										"value": null
									}
								],
								"one": {
									"list": [
										null
									],
									"value": null
								}
							}
						],
						"two": {
							"oneList": [
								{
									"list": [
										null,
										null
									],
									"value": null
								}
							],
							"one": {
								"list": [
									null
								],
								"value": null
							}
						}
					}
				}`,
		}
		paths, err := runTest(t, tc)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"variables.arg.twoList.0.oneList.0.list.0",
			"variables.arg.twoList.0.oneList.0.list.1",
			"variables.arg.twoList.0.oneList.0.value",
			"variables.arg.twoList.0.one.list.0",
			"variables.arg.twoList.0.one.value",
			"variables.arg.two.oneList.0.list.0",
			"variables.arg.two.oneList.0.list.1",
			"variables.arg.two.oneList.0.value",
			"variables.arg.two.one.list.0",
			"variables.arg.two.one.value",
		}, paths)
	})

}

type testCase struct {
	schema, operation, variables string
	withNormalization            bool
	mapping                      map[string]string
}

func runTest(t *testing.T, tc testCase) (paths []string, err error) {
	t.Helper()
	def := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(tc.schema)
	op := unsafeparser.ParseGraphqlDocumentString(tc.operation)
	op.Input.Variables = []byte(tc.variables)
	if tc.withNormalization {
		report := &operationreport.Report{}
		norm := astnormalization.NewNormalizer(true, true)
		norm.NormalizeOperation(&op, &def, report)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}
	}
	finder := NewUploadFinder()
	return finder.FindUploads(&op, &def, op.Input.Variables)
}
