package uploads_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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

	t.Run("query with upload in the argument passed via top level variable", func(t *testing.T) {
		t.Run("arg of type upload", func(t *testing.T) {
			tc := testCase{
				schema:    `scalar Upload type Query { hello(arg: Upload!): String }`,
				operation: `query Foo($bar: Upload!) { hello(arg: $bar) }`,
				variables: `{"bar": null}`,
			}
			paths, err := runTest(t, tc)
			require.NoError(t, err)
			assert.Equal(t, []uploads.UploadPathMapping{{"bar", "variables.bar", ""}}, paths)
		})

		t.Run("arg has nested upload", func(t *testing.T) {
			tc := testCase{
				schema:    `scalar Upload input Input {f: Upload!} type Mutation { hello(arg: Input!): String }`,
				operation: `mutation Foo($i: Input!) { hello(arg: $i) }`,
				variables: `{"i":{"f":null}}`,
			}
			paths, err := runTest(t, tc)
			require.NoError(t, err)
			assert.Equal(t, []uploads.UploadPathMapping{
				{"i", "variables.i.f", ""},
			}, paths)
		})

		t.Run("arg has nested uploads", func(t *testing.T) {
			tc := testCase{
				schema:    `scalar Upload input Input {f: Upload! f2: Upload!} type Mutation { hello(arg: Input!): String }`,
				operation: `mutation Foo($i: Input!) { hello(arg: $i) }`,
				variables: `{"i":{"f":null,"f2":null}}`,
			}
			paths, err := runTest(t, tc)
			require.NoError(t, err)
			assert.Equal(t, []uploads.UploadPathMapping{
				{"i", "variables.i.f", ""},
				{"i", "variables.i.f2", ""},
			}, paths)
		})

		t.Run("arg has nested uploads in a list", func(t *testing.T) {
			tc := testCase{
				schema:    `scalar Upload input Input {f: [Upload!]! f2: [Upload!]!} type Mutation { hello(arg: Input!): String }`,
				operation: `mutation Foo($i: Input!) { hello(arg: $i) }`,
				variables: `{"i":{"f":[null],"f2":[null,null,null]}}`,
			}
			paths, err := runTest(t, tc)
			require.NoError(t, err)
			assert.Equal(t, []uploads.UploadPathMapping{
				{"i", "variables.i.f.0", ""},
				{"i", "variables.i.f2.0", ""},
				{"i", "variables.i.f2.1", ""},
				{"i", "variables.i.f2.2", ""},
			}, paths)
		})
	})

	t.Run("query with upload in the argument passed via inline input value with variables inside", func(t *testing.T) {
		t.Run("arg has inline object value with upload passed via variable", func(t *testing.T) {
			tc := testCase{
				schema:    `scalar Upload input Input {f: Upload!} type Mutation { hello(arg: Input!): String }`,
				operation: `mutation Foo($i: Upload!) { hello(arg: {f: $i}) }`,
				variables: `{"i":null}`,
			}
			paths, err := runTest(t, tc)
			require.NoError(t, err)
			assert.Equal(t, []uploads.UploadPathMapping{
				{"i", "variables.i", "f"},
			}, paths)
		})

		t.Run("arg has inline objects with variables which have nested file uploads", func(t *testing.T) {
			tc := testCase{
				schema: `
				scalar Upload
				input Input {list: [Upload!]! value: Upload!}
				input Input2 {oneList: [Input!]! one: Input!}
				input Input3 {twoList: [Input2!]! two: Input2!}
				type Mutation { hello(arg: Input3!): String }`,
				operation: `mutation Foo($varOne: [Input2!]! $varTwo: Input2!) { hello(arg: {twoList: $varOne two: $varTwo}) }`,
				variables: `
				{
					"varOne": [
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
					"varTwo": {
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
				}`,
			}
			paths, err := runTest(t, tc)
			require.NoError(t, err)
			assert.Equal(t, []uploads.UploadPathMapping{
				{"varOne", "variables.varOne.0.oneList.0.list.0", "twoList.0.oneList.0.list.0"},
				{"varOne", "variables.varOne.0.oneList.0.list.1", "twoList.0.oneList.0.list.1"},
				{"varOne", "variables.varOne.0.oneList.0.value", "twoList.0.oneList.0.value"},
				{"varOne", "variables.varOne.0.one.list.0", "twoList.0.one.list.0"},
				{"varOne", "variables.varOne.0.one.value", "twoList.0.one.value"},
				{"varTwo", "variables.varTwo.oneList.0.list.0", "two.oneList.0.list.0"},
				{"varTwo", "variables.varTwo.oneList.0.list.1", "two.oneList.0.list.1"},
				{"varTwo", "variables.varTwo.oneList.0.value", "two.oneList.0.value"},
				{"varTwo", "variables.varTwo.one.list.0", "two.one.list.0"},
				{"varTwo", "variables.varTwo.one.value", "two.one.value"},
			}, paths)

		})
	})
}

type testCase struct {
	schema, operation, variables string
	withNormalization            bool
	mapping                      map[string]string
}

func runTest(t *testing.T, tc testCase) (paths []uploads.UploadPathMapping, err error) {
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

	finder := uploads.NewUploadFinder()
	testWalker := astvisitor.NewWalker(4)
	v := &testVisitor{
		Walker:      &testWalker,
		operation:   &op,
		definition:  &def,
		findUploads: finder,
	}
	testWalker.RegisterEnterArgumentVisitor(v)

	report := &operationreport.Report{}
	testWalker.Walk(&op, &def, report)
	if report.HasErrors() {
		t.Fatal(report.Error())
	}

	return v.uploadPaths, nil
}

type testVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	variables             *astjson.Value
	findUploads           *uploads.UploadFinder

	uploadPaths []uploads.UploadPathMapping
}

func (v *testVisitor) EnterArgument(ref int) {
	if v.Ancestor().Kind != ast.NodeKindField {
		return
	}

	definitionRef, exists := v.ArgumentInputValueDefinition(ref)
	if !exists {
		return
	}

	uploadPathMappings, err := v.findUploads.FindUploads(v.operation, v.definition, v.operation.Input.Variables, ref, definitionRef)
	if err != nil {
		return
	}

	if len(uploadPathMappings) > 0 {
		v.uploadPaths = append(v.uploadPaths, uploadPathMappings...)
	}
}
