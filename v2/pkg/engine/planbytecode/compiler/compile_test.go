package compiler

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/planbytecode"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestCompileFlattensFetchTreeAndResponseShape(t *testing.T) {
	p := syncResponsePlan(
		resolve.Sequence(
			resolve.SingleWithPath(singleFetch("users", "query Users { user { id name } }", []string{"data"}, nil), "query.user"),
			resolve.Parallel(
				resolve.SingleWithPath(singleFetch("reviews", "query Reviews { reviews { body } }", []string{"data"}, []string{"reviews"}), "query.user.reviews"),
				resolve.SingleWithPath(singleFetch("products", "query Products { products { upc } }", []string{"data"}, []string{"products"}), "query.user.products"),
			),
		),
		rootObject(
			field("user", objectValue("user",
				field("id", stringValue("id")),
				field("kind", staticStringAt("kind", "User")),
			)),
		),
	)

	program, err := Compile(p)
	require.NoError(t, err)
	require.True(t, program.FastPathReady())
	require.Empty(t, program.Unsupported)
	require.Equal(t, 3, program.Stats.Fetches)
	require.Equal(t, 3, len(program.Fetches))
	require.Equal(t, 2, program.Stats.Objects)
	require.Equal(t, 3, program.Stats.Fields)
	require.Equal(t, 1, program.Stats.Literals)

	require.Equal(t, []planbytecode.Opcode{
		planbytecode.OpEnterSequence,
		planbytecode.OpFetchSubgraph,
		planbytecode.OpPasteAtPointer,
		planbytecode.OpEnterParallel,
		planbytecode.OpFetchSubgraph,
		planbytecode.OpPasteAtPointer,
		planbytecode.OpFetchSubgraph,
		planbytecode.OpPasteAtPointer,
		planbytecode.OpLeaveParallel,
		planbytecode.OpLeaveSequence,
		planbytecode.OpEnterObject,
		planbytecode.OpProjectField,
		planbytecode.OpEnterObject,
		planbytecode.OpProjectField,
		planbytecode.OpProjectField,
		planbytecode.OpEmitLiteral,
		planbytecode.OpLeaveObject,
		planbytecode.OpLeaveObject,
		planbytecode.OpEmitResponse,
	}, opcodes(program.Ops))
	require.Equal(t, uint32(2), program.Ops[0].A)
	require.Equal(t, uint32(9), program.Ops[0].B)
	require.Equal(t, uint32(2), program.Ops[3].A)
	require.Equal(t, uint32(8), program.Ops[3].B)

	require.Contains(t, program.Strings, "users")
	require.Contains(t, program.Strings, "reviews")
	require.Contains(t, program.Strings, "products")
	require.Equal(t, len(program.Strings), len(program.QuotedStrings))
	for i := range program.Strings {
		require.Equal(t, strconv.Quote(program.Strings[i]), program.QuotedStrings[i])
	}
	require.Contains(t, program.Paths, []string{"data"})
	require.Contains(t, program.Paths, []string{"reviews"})
}

func TestCompileDCEsPlaceholderOnlyFetch(t *testing.T) {
	p := syncResponsePlan(
		resolve.Sequence(
			resolve.SingleWithPath(singleFetch("empty", "query Empty { __internal__typename_placeholder: __typename }", []string{"data"}, nil), "query.empty"),
			resolve.SingleWithPath(singleFetch("users", "query Users { user { id } }", []string{"data"}, nil), "query.user"),
		),
		rootObject(),
	)

	program, err := Compile(p)
	require.NoError(t, err)
	require.True(t, program.FastPathReady())
	require.Equal(t, 1, program.Stats.DCEFetches)
	require.Equal(t, 1, program.Stats.Fetches)
	require.Equal(t, []planbytecode.Opcode{
		planbytecode.OpEnterSequence,
		planbytecode.OpFetchSubgraph,
		planbytecode.OpPasteAtPointer,
		planbytecode.OpLeaveSequence,
		planbytecode.OpEnterObject,
		planbytecode.OpLeaveObject,
		planbytecode.OpEmitResponse,
	}, opcodes(program.Ops))
	require.Equal(t, uint32(1), program.Ops[0].A)
	require.Equal(t, uint32(3), program.Ops[0].B)
	require.Equal(t, "users", program.Strings[program.Fetches[0].DataSourceNameRef])
}

func TestCompileDirectResponseForFlatRootScalars(t *testing.T) {
	p := syncResponsePlan(
		resolve.Sequence(
			resolve.SingleWithPath(singleFetch("users", "query Users { a b }", []string{"data"}, nil), "query"),
			resolve.SingleWithPath(singleFetch("inventory", "query Inventory { c }", []string{"data"}, nil), "query"),
		),
		rootObject(
			field("a", integerValue("a")),
			field("b", stringValue("b", true)),
			field("c", booleanValue("c")),
			field("kind", staticStringValue("User")),
		),
	)

	program, err := Compile(p)
	require.NoError(t, err)
	require.True(t, program.DirectResponseReady())
	require.Len(t, program.DirectResponse.Fields, 4)
	require.Equal(t, uint32(resolve.NodeKindInteger), planbytecode.DirectFieldKind(program.DirectResponse.Fields[0].Flags))
	require.Equal(t, uint32(resolve.NodeKindString), planbytecode.DirectFieldKind(program.DirectResponse.Fields[1].Flags))
	require.True(t, planbytecode.DirectFieldIsNullable(program.DirectResponse.Fields[1].Flags))
	require.True(t, planbytecode.DirectFieldIsLiteral(program.DirectResponse.Fields[3].Flags))
}

func TestCompileDirectResponseForNestedShape(t *testing.T) {
	p := syncResponsePlan(
		resolve.SingleWithPath(singleFetch("users", "query Users { user { id posts { title } } }", []string{"data"}, nil), "query"),
		rootObject(
			field("user", objectValue("user",
				field("id", stringValue("id")),
				field("posts", arrayValue("posts",
					rootObject(field("title", stringValue("title"))),
				)),
			)),
		),
	)

	program, err := Compile(p)
	require.NoError(t, err)
	require.True(t, program.DirectResponseReady())
	require.Len(t, program.DirectResponse.Fields, 1)
	require.Equal(t, uint32(resolve.NodeKindObject), planbytecode.DirectFieldKind(program.DirectResponse.Fields[0].Flags))
	require.Len(t, program.DirectResponse.Fields[0].Children, 2)
	require.Equal(t, uint32(resolve.NodeKindArray), planbytecode.DirectFieldKind(program.DirectResponse.Fields[0].Children[1].Flags))
	require.Equal(t, uint32(resolve.NodeKindObject), planbytecode.DirectFieldKind(program.DirectResponse.Fields[0].Children[1].ItemFlags))
}

func TestCompileDirectResponseForRootArrayBatchEntityFetch(t *testing.T) {
	p := syncResponsePlan(
		resolve.Sequence(
			resolve.SingleWithPath(singleFetch("products", "query Products { products { name upc } }", []string{"data"}, nil), "query"),
			resolve.SingleWithPath(batchEntityFetch("stock", "data", "_entities"), "query.products", resolve.ArrayPath("products")),
		),
		rootObject(
			field("products", arrayValue("products",
				rootObject(
					field("name", stringValue("name")),
					field("stock", integerValue("stock")),
				),
			)),
		),
	)

	program, err := Compile(p)
	require.NoError(t, err)
	require.True(t, program.DirectResponseReady())
	require.Len(t, program.DirectResponse.Fields, 1)
	require.Equal(t, uint32(resolve.NodeKindArray), planbytecode.DirectFieldKind(program.DirectResponse.Fields[0].Flags))
	require.Len(t, program.DirectResponse.Fields[0].Children, 2)
}

func TestCompileDirectResponseForRootObjectEntityFetch(t *testing.T) {
	p := syncResponsePlan(
		resolve.Sequence(
			resolve.SingleWithPath(singleFetch("users", "query Users { user { id name } }", []string{"data"}, nil), "query"),
			resolve.SingleWithPath(entityFetch("profiles", "data", "_entities", "0"), "query.user", resolve.ObjectPath("user")),
		),
		rootObject(
			field("user", objectValue("user",
				field("name", stringValue("name")),
				field("age", integerValue("age")),
			)),
		),
	)

	program, err := Compile(p)
	require.NoError(t, err)
	require.True(t, program.DirectResponseReady())
	require.Len(t, program.DirectResponse.Fields, 1)
	require.Equal(t, uint32(resolve.NodeKindObject), planbytecode.DirectFieldKind(program.DirectResponse.Fields[0].Flags))
	require.Len(t, program.DirectResponse.Fields[0].Children, 2)
}

func TestCompileDirectResponseRequiresParentDataBeforeEntityFetch(t *testing.T) {
	p := syncResponsePlan(
		resolve.Parallel(
			resolve.SingleWithPath(singleFetch("users", "query Users { user { id name } }", []string{"data"}, nil), "query"),
			resolve.SingleWithPath(entityFetch("profiles", "data", "_entities", "0"), "query.user", resolve.ObjectPath("user")),
		),
		rootObject(
			field("user", objectValue("user",
				field("name", stringValue("name")),
				field("age", integerValue("age")),
			)),
		),
	)

	program, err := Compile(p)
	require.NoError(t, err)
	require.False(t, program.DirectResponseReady())
	require.True(t, program.FastPathReady())
}

func TestCompileDirectResponseForNestedBatchEntityFetch(t *testing.T) {
	p := syncResponsePlan(
		resolve.Sequence(
			resolve.SingleWithPath(singleFetch("products", "query Products { products { reviews { author { id } } } }", []string{"data"}, nil), "query"),
			resolve.SingleWithPath(batchEntityFetch("authors", "data", "_entities"), "query.products.reviews.author", resolve.ArrayPath("products"), resolve.ArrayPath("reviews"), resolve.ObjectPath("author")),
		),
		rootObject(
			field("products", arrayValue("products",
				rootObject(
					field("reviews", arrayValue("reviews",
						rootObject(
							field("author", objectValue("author",
								field("name", stringValue("name")),
							)),
						),
					)),
				),
			)),
		),
	)

	program, err := Compile(p)
	require.NoError(t, err)
	require.True(t, program.DirectResponseReady())
	require.Len(t, program.DirectResponse.Fields, 1)
	require.Equal(t, uint32(resolve.NodeKindArray), planbytecode.DirectFieldKind(program.DirectResponse.Fields[0].Flags))
}

func TestKnownVariableSkipIncludeNormalizesToDCEableSelection(t *testing.T) {
	definition, report := astparser.ParseGraphqlDocumentString(`
		schema { query: Query }
		type Query {
			id: ID
			name: String
		}
	`)
	require.False(t, report.HasErrors(), report.Error())
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))

	operation, report := astparser.ParseGraphqlDocumentString(`
		query Q($withName: Boolean!, $withoutID: Boolean!) {
			id @skip(if: $withoutID)
			name @include(if: $withName)
		}
	`)
	require.False(t, report.HasErrors(), report.Error())
	operation.Input.Variables = []byte(`{"withName":false,"withoutID":true}`)

	normalizer := astnormalization.NewWithOpts(
		astnormalization.WithRemoveNotMatchingOperationDefinitions(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
	)
	normalizationReport := &operationreport.Report{}
	normalizer.NormalizeNamedOperation(&operation, &definition, []byte("Q"), normalizationReport)
	require.False(t, normalizationReport.HasErrors(), normalizationReport.Error())
	require.True(t, selectionSetIsEmpty(&operation, operation.OperationDefinitions[0].SelectionSet))
	require.JSONEq(t, `{}`, string(operation.Input.Variables))
}

func TestCompileMarksInterpretedOnlyFeatures(t *testing.T) {
	p := syncResponsePlan(nil, rootObject(&resolve.Field{
		Name:  []byte("deferred"),
		Defer: &resolve.DeferField{},
		Value: &resolve.Array{
			Path:     []string{"deferred"},
			SkipItem: func(ctx *resolve.Context, arrayItem *astjson.Value) bool { return false },
			Item:     stringValue("name"),
		},
	}))

	program, err := Compile(p)
	require.NoError(t, err)
	require.False(t, program.FastPathReady())
	require.ElementsMatch(t, []string{"defer", "array_skip_item"}, unsupportedFeatures(program.Unsupported))
}

func TestCompileSubscriptionRemainsInterpreted(t *testing.T) {
	program, err := Compile(&plan.SubscriptionResponsePlan{})
	require.NoError(t, err)
	require.False(t, program.FastPathReady())
	require.Equal(t, []planbytecode.UnsupportedFeature{{
		Feature: "subscription",
		Reason:  "subscriptions remain on the interpreted resolver path",
	}}, program.Unsupported)
}

func opcodes(ops []planbytecode.Op) []planbytecode.Opcode {
	out := make([]planbytecode.Opcode, len(ops))
	for i := range ops {
		out[i] = ops[i].Code
	}
	return out
}

func unsupportedFeatures(features []planbytecode.UnsupportedFeature) []string {
	out := make([]string, len(features))
	for i := range features {
		out[i] = features[i].Feature
	}
	return out
}
