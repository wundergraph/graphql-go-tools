package resolve

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/planbytecode"
)

func TestResolveGraphQLResponseBytecodeMatchesFetchTree(t *testing.T) {
	expectedResponse, _ := bytecodeTestResponse()
	actualResponse, actualProgram := bytecodeTestResponse()

	ctx := NewContext(context.Background())
	resolver := newResolver(context.Background())

	var expected bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, expectedResponse, nil, &expected)
	require.NoError(t, err)

	ctx = NewContext(context.Background())
	var actual bytes.Buffer
	_, err = resolver.ResolveGraphQLResponseBytecode(ctx, actualResponse, actualProgram, nil, &actual)
	require.NoError(t, err)

	require.Equal(t, expected.String(), actual.String())
	require.Equal(t, `{"data":{"a":1,"b":2,"c":3}}`, actual.String())
}

func TestResolveGraphQLResponseBytecodeFallsBackOnSubgraphErrors(t *testing.T) {
	expectedResponse, _ := bytecodeTestResponseWithRootError()
	actualResponse, actualProgram := bytecodeTestResponseWithRootError()

	ctx := NewContext(context.Background())
	resolver := newResolver(context.Background())

	var expected bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, expectedResponse, nil, &expected)
	require.NoError(t, err)

	ctx = NewContext(context.Background())
	var actual bytes.Buffer
	_, err = resolver.ResolveGraphQLResponseBytecode(ctx, actualResponse, actualProgram, nil, &actual)
	require.NoError(t, err)

	require.Equal(t, expected.String(), actual.String())
	require.Contains(t, actual.String(), "downstream")
}

func TestResolveGraphQLResponseBytecodeDirectNestedObjectAndArray(t *testing.T) {
	expectedResponse, _ := bytecodeNestedTestResponse()
	actualResponse, actualProgram := bytecodeNestedTestResponse()

	ctx := NewContext(context.Background())
	resolver := newResolver(context.Background())

	var expected bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, expectedResponse, nil, &expected)
	require.NoError(t, err)

	ctx = NewContext(context.Background())
	var actual bytes.Buffer
	_, err = resolver.ResolveGraphQLResponseBytecode(ctx, actualResponse, actualProgram, nil, &actual)
	require.NoError(t, err)

	require.Equal(t, expected.String(), actual.String())
	require.Equal(t, `{"data":{"user":{"id":"1","name":"Ada","age":37,"posts":[{"title":"First","score":10},{"title":"Second","score":20}]}}}`, actual.String())
}

func TestResolveGraphQLResponseBytecodeDirectMergePath(t *testing.T) {
	expectedResponse, _ := bytecodeMergePathTestResponse()
	actualResponse, actualProgram := bytecodeMergePathTestResponse()

	ctx := NewContext(context.Background())
	resolver := newResolver(context.Background())

	var expected bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, expectedResponse, nil, &expected)
	require.NoError(t, err)

	ctx = NewContext(context.Background())
	var actual bytes.Buffer
	_, err = resolver.ResolveGraphQLResponseBytecode(ctx, actualResponse, actualProgram, nil, &actual)
	require.NoError(t, err)

	require.Equal(t, expected.String(), actual.String())
	require.Equal(t, `{"data":{"user":{"id":"1","age":37}}}`, actual.String())
}

func TestResolveGraphQLResponseBytecodeDirectRootArrayBatchEntityFetch(t *testing.T) {
	expectedResponse, _ := bytecodeBatchEntityTestResponse()
	actualResponse, actualProgram := bytecodeBatchEntityTestResponse()

	ctx := NewContext(context.Background())
	resolver := newResolver(context.Background())

	var expected bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, expectedResponse, nil, &expected)
	require.NoError(t, err)

	ctx = NewContext(context.Background())
	var actual bytes.Buffer
	_, err = resolver.ResolveGraphQLResponseBytecode(ctx, actualResponse, actualProgram, nil, &actual)
	require.NoError(t, err)

	require.Equal(t, expected.String(), actual.String())
	require.Equal(t, `{"data":{"products":[{"name":"Table","stock":8},{"name":"Couch","stock":2}]}}`, actual.String())
}

func TestResolveGraphQLResponseBytecodeDirectRootObjectEntityFetch(t *testing.T) {
	expectedResponse, _ := bytecodeEntityTestResponse()
	actualResponse, actualProgram := bytecodeEntityTestResponse()

	ctx := NewContext(context.Background())
	resolver := newResolver(context.Background())

	var expected bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, expectedResponse, nil, &expected)
	require.NoError(t, err)

	ctx = NewContext(context.Background())
	var actual bytes.Buffer
	_, err = resolver.ResolveGraphQLResponseBytecode(ctx, actualResponse, actualProgram, nil, &actual)
	require.NoError(t, err)

	require.Equal(t, expected.String(), actual.String())
	require.Equal(t, `{"data":{"user":{"name":"Ada","age":37}}}`, actual.String())
}

func TestResolveGraphQLResponseBytecodeDirectNestedDedupBatchEntityFetch(t *testing.T) {
	expectedResponse, _ := bytecodeNestedBatchEntityTestResponse()
	actualResponse, actualProgram := bytecodeNestedBatchEntityTestResponse()

	ctx := NewContext(context.Background())
	resolver := newResolver(context.Background())

	var expected bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, expectedResponse, nil, &expected)
	require.NoError(t, err)

	ctx = NewContext(context.Background())
	var actual bytes.Buffer
	_, err = resolver.ResolveGraphQLResponseBytecode(ctx, actualResponse, actualProgram, nil, &actual)
	require.NoError(t, err)

	require.Equal(t, expected.String(), actual.String())
	require.Equal(t, `{"data":{"products":[{"name":"Table","reviews":[{"body":"Love Table","author":{"name":"user-1"}},{"body":"Prefer Desk","author":{"name":"user-2"}}]},{"name":"Couch","reviews":[{"body":"Too expensive","author":{"name":"user-1"}}]}]}}`, actual.String())
}

func TestLoadAndRenderGraphQLResponseBytecodeDirectNestedDedupBatchEntityFetch(t *testing.T) {
	response, program := bytecodeNestedBatchEntityTestResponse()
	ctx := NewContext(context.Background())
	resolvable := NewResolvable(nil, ResolvableOptions{})
	loader := &Loader{}

	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	var actual bytes.Buffer
	rendered, err := loader.LoadAndRenderGraphQLResponseBytecodeDirect(ctx, response, program, resolvable, &actual)
	require.NoError(t, err)
	require.True(t, rendered)
	require.Equal(t, `{"data":{"products":[{"name":"Table","reviews":[{"body":"Love Table","author":{"name":"user-1"}},{"body":"Prefer Desk","author":{"name":"user-2"}}]},{"name":"Couch","reviews":[{"body":"Too expensive","author":{"name":"user-1"}}]}]}}`, actual.String())
}

func TestResolvableRenderGraphQLResponseBytecodeNestedObjectAndArray(t *testing.T) {
	response, program := bytecodeNestedTestResponse()
	program.DirectResponse = nil

	ctx := NewContext(context.Background())
	resolvable := NewResolvable(nil, ResolvableOptions{})
	loader := &Loader{}

	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)
	err = loader.LoadGraphQLResponseDataBytecode(ctx, response, program, resolvable)
	require.NoError(t, err)

	var actual bytes.Buffer
	rendered, err := resolvable.RenderGraphQLResponseBytecode(program, &actual)
	require.NoError(t, err)
	require.True(t, rendered)
	require.Equal(t, `{"data":{"user":{"id":"1","name":"Ada","age":37,"posts":[{"title":"First","score":10},{"title":"Second","score":20}]}}}`, actual.String())
	require.Equal(t, 2, resolvable.actualListSizes["user.posts"])
}

func TestResolvableRenderGraphQLResponseBytecodeFallsBackWhenErrorsExist(t *testing.T) {
	response, program := bytecodeTestResponseWithRootError()
	program.DirectResponse = nil

	ctx := NewContext(context.Background())
	resolvable := NewResolvable(nil, ResolvableOptions{})
	loader := &Loader{}

	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)
	err = loader.LoadGraphQLResponseDataBytecode(ctx, response, program, resolvable)
	require.NoError(t, err)

	var actual bytes.Buffer
	rendered, err := resolvable.RenderGraphQLResponseBytecode(program, &actual)
	require.NoError(t, err)
	require.False(t, rendered)
	require.Empty(t, actual.String())
}

func TestLoaderLoadGraphQLResponseDataBytecodeExecutesParallelFetches(t *testing.T) {
	response, program := bytecodeTestResponse()
	ctx := NewContext(context.Background())
	resolvable := NewResolvable(nil, ResolvableOptions{})
	loader := &Loader{}

	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)
	err = loader.LoadGraphQLResponseDataBytecode(ctx, response, program, resolvable)
	require.NoError(t, err)

	require.Equal(t, "1", resolvable.data.Get("a").String())
	require.Equal(t, "2", resolvable.data.Get("b").String())
	require.Equal(t, "3", resolvable.data.Get("c").String())
}

func BenchmarkLoaderLoadGraphQLResponseDataFetchTree(b *testing.B) {
	response, _ := bytecodeTestResponse()
	benchmarkBytecodeLoader(b, response, nil)
}

func BenchmarkLoaderLoadGraphQLResponseDataBytecode(b *testing.B) {
	response, program := bytecodeTestResponse()
	benchmarkBytecodeLoader(b, response, program)
}

func BenchmarkResolveGraphQLResponseFetchTree(b *testing.B) {
	response, _ := bytecodeTestResponse()
	benchmarkResolveGraphQLResponse(b, response, nil)
}

func BenchmarkResolveGraphQLResponseBytecode(b *testing.B) {
	response, program := bytecodeTestResponse()
	benchmarkResolveGraphQLResponse(b, response, program)
}

func BenchmarkResolveGraphQLResponseBytecodeResponseOps(b *testing.B) {
	response, program := bytecodeTestResponse()
	program.DirectResponse = nil
	benchmarkResolveGraphQLResponse(b, response, program)
}

func BenchmarkResolveGraphQLResponseNestedFetchTree(b *testing.B) {
	response, _ := bytecodeNestedTestResponse()
	benchmarkResolveGraphQLResponseWithExpected(b, response, nil, `{"data":{"user":{"id":"1","name":"Ada","age":37,"posts":[{"title":"First","score":10},{"title":"Second","score":20}]}}}`)
}

func BenchmarkResolveGraphQLResponseNestedBytecode(b *testing.B) {
	response, program := bytecodeNestedTestResponse()
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"user":{"id":"1","name":"Ada","age":37,"posts":[{"title":"First","score":10},{"title":"Second","score":20}]}}}`)
}

func BenchmarkResolveGraphQLResponseNestedBytecodeResponseOps(b *testing.B) {
	response, program := bytecodeNestedTestResponse()
	program.DirectResponse = nil
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"user":{"id":"1","name":"Ada","age":37,"posts":[{"title":"First","score":10},{"title":"Second","score":20}]}}}`)
}

func BenchmarkResolveGraphQLResponseBatchEntityFetchTree(b *testing.B) {
	response, _ := bytecodeBatchEntityTestResponse()
	benchmarkResolveGraphQLResponseWithExpected(b, response, nil, `{"data":{"products":[{"name":"Table","stock":8},{"name":"Couch","stock":2}]}}`)
}

func BenchmarkResolveGraphQLResponseBatchEntityBytecode(b *testing.B) {
	response, program := bytecodeBatchEntityTestResponse()
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"products":[{"name":"Table","stock":8},{"name":"Couch","stock":2}]}}`)
}

func BenchmarkResolveGraphQLResponseBatchEntityBytecodeResponseOps(b *testing.B) {
	response, program := bytecodeBatchEntityTestResponse()
	program.DirectResponse = nil
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"products":[{"name":"Table","stock":8},{"name":"Couch","stock":2}]}}`)
}

func BenchmarkResolveGraphQLResponseEntityFetchTree(b *testing.B) {
	response, _ := bytecodeEntityTestResponse()
	benchmarkResolveGraphQLResponseWithExpected(b, response, nil, `{"data":{"user":{"name":"Ada","age":37}}}`)
}

func BenchmarkResolveGraphQLResponseEntityBytecode(b *testing.B) {
	response, program := bytecodeEntityTestResponse()
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"user":{"name":"Ada","age":37}}}`)
}

func BenchmarkResolveGraphQLResponseEntityBytecodeResponseOps(b *testing.B) {
	response, program := bytecodeEntityTestResponse()
	program.DirectResponse = nil
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"user":{"name":"Ada","age":37}}}`)
}

func BenchmarkResolveGraphQLResponseNestedBatchEntityFetchTree(b *testing.B) {
	response, _ := bytecodeNestedBatchEntityTestResponse()
	benchmarkResolveGraphQLResponseWithExpected(b, response, nil, `{"data":{"products":[{"name":"Table","reviews":[{"body":"Love Table","author":{"name":"user-1"}},{"body":"Prefer Desk","author":{"name":"user-2"}}]},{"name":"Couch","reviews":[{"body":"Too expensive","author":{"name":"user-1"}}]}]}}`)
}

func BenchmarkResolveGraphQLResponseNestedBatchEntityBytecode(b *testing.B) {
	response, program := bytecodeNestedBatchEntityTestResponse()
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"products":[{"name":"Table","reviews":[{"body":"Love Table","author":{"name":"user-1"}},{"body":"Prefer Desk","author":{"name":"user-2"}}]},{"name":"Couch","reviews":[{"body":"Too expensive","author":{"name":"user-1"}}]}]}}`)
}

func BenchmarkResolveGraphQLResponseNestedBatchEntityBytecodeResponseOps(b *testing.B) {
	response, program := bytecodeNestedBatchEntityTestResponse()
	program.DirectResponse = nil
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"products":[{"name":"Table","reviews":[{"body":"Love Table","author":{"name":"user-1"}},{"body":"Prefer Desk","author":{"name":"user-2"}}]},{"name":"Couch","reviews":[{"body":"Too expensive","author":{"name":"user-1"}}]}]}}`)
}

func benchmarkBytecodeLoader(b *testing.B, response *GraphQLResponse, program *planbytecode.Program) {
	ctx := NewContext(context.Background())
	resolvable := NewResolvable(nil, ResolvableOptions{})
	loader := &Loader{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loader.Free()
		resolvable.Reset()
		err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
		if err != nil {
			b.Fatal(err)
		}
		if program == nil {
			err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
		} else {
			err = loader.LoadGraphQLResponseDataBytecode(ctx, response, program, resolvable)
		}
		if err != nil {
			b.Fatal(err)
		}
		if resolvable.data.Get("c") == nil {
			b.Fatal("missing merged field")
		}
	}
}

func benchmarkResolveGraphQLResponse(b *testing.B, response *GraphQLResponse, program *planbytecode.Program) {
	benchmarkResolveGraphQLResponseWithExpected(b, response, program, `{"data":{"a":1,"b":2,"c":3}}`)
}

func benchmarkResolveGraphQLResponseWithExpected(b *testing.B, response *GraphQLResponse, program *planbytecode.Program, expected string) {
	resolver := newResolver(context.Background())
	program = bytecodeTestQuoteProgramStrings(program)
	var out bytes.Buffer

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewContext(context.Background())
		out.Reset()
		var err error
		if program == nil {
			_, err = resolver.ResolveGraphQLResponse(ctx, response, nil, &out)
		} else {
			_, err = resolver.ResolveGraphQLResponseBytecode(ctx, response, program, nil, &out)
		}
		if err != nil {
			b.Fatal(err)
		}
		if out.String() != expected {
			b.Fatalf("unexpected response: %s", out.String())
		}
	}
}

func bytecodeTestQuoteProgramStrings(program *planbytecode.Program) *planbytecode.Program {
	if program == nil || len(program.QuotedStrings) == len(program.Strings) {
		return program
	}
	program.QuotedStrings = make([]string, len(program.Strings))
	for i := range program.Strings {
		program.QuotedStrings[i] = strconv.Quote(program.Strings[i])
	}
	return program
}

func bytecodeTestNodeFlags(kind NodeKind, nullable bool) uint32 {
	return planbytecode.EncodeDirectFieldFlags(uint32(kind), nullable, false)
}

func bytecodeTestResponse() (*GraphQLResponse, *planbytecode.Program) {
	root := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"a":1}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "root"},
		},
	}
	left := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"b":2}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "left"},
		},
	}
	right := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"c":3}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "right"},
		},
	}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: root},
			Parallel(
				&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: left},
				&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: right},
			),
		),
		Data: &Object{
			Fields: []*Field{
				{Name: []byte("a"), Value: &Integer{Path: []string{"a"}}},
				{Name: []byte("b"), Value: &Integer{Path: []string{"b"}}},
				{Name: []byte("c"), Value: &Integer{Path: []string{"c"}}},
			},
		},
	}
	program := &planbytecode.Program{
		Strings: []string{"a", "b", "c"},
		Paths:   [][]string{nil, {"a"}, {"b"}, {"c"}},
		Ops: []planbytecode.Op{
			{Code: planbytecode.OpEnterSequence, A: 2, B: 9},
			{Code: planbytecode.OpFetchSubgraph, A: 0},
			{Code: planbytecode.OpPasteAtPointer, A: 0},
			{Code: planbytecode.OpEnterParallel, A: 2, B: 8},
			{Code: planbytecode.OpFetchSubgraph, A: 1},
			{Code: planbytecode.OpPasteAtPointer, A: 1},
			{Code: planbytecode.OpFetchSubgraph, A: 2},
			{Code: planbytecode.OpPasteAtPointer, A: 2},
			{Code: planbytecode.OpLeaveParallel},
			{Code: planbytecode.OpLeaveSequence},
			{Code: planbytecode.OpEnterObject, A: 0, B: 3},
			{Code: planbytecode.OpProjectField, A: 0, B: 1, C: bytecodeTestNodeFlags(NodeKindInteger, false)},
			{Code: planbytecode.OpProjectField, A: 1, B: 2, C: bytecodeTestNodeFlags(NodeKindInteger, false)},
			{Code: planbytecode.OpProjectField, A: 2, B: 3, C: bytecodeTestNodeFlags(NodeKindInteger, false)},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpEmitResponse},
		},
		Fetches: []planbytecode.Fetch{
			{Item: root},
			{Item: left},
			{Item: right},
		},
		DirectResponse: &planbytecode.DirectResponse{
			Fields: []planbytecode.DirectField{
				{NameRef: 0, PathRef: 1, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindInteger), false, false)},
				{NameRef: 1, PathRef: 2, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindInteger), false, false)},
				{NameRef: 2, PathRef: 3, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindInteger), false, false)},
			},
		},
	}
	return response, program
}

func bytecodeTestResponseWithRootError() (*GraphQLResponse, *planbytecode.Program) {
	response, program := bytecodeTestResponse()
	root := response.Fetches.ChildNodes[0].Item.Fetch.(*SingleFetch)
	root.FetchConfiguration.DataSource = FakeDataSource(`{"errors":[{"message":"downstream"}],"data":{"a":1}}`)
	root.PostProcessing.SelectResponseErrorsPath = []string{"errors"}
	return response, program
}

func bytecodeNestedTestResponse() (*GraphQLResponse, *planbytecode.Program) {
	root := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"user":{"id":"1","name":"Ada","posts":[{"title":"First"},{"title":"Second"}]}}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "root"},
		},
	}
	extra := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"user":{"age":37,"posts":[{"score":10},{"score":20}]}}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "extra"},
		},
	}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: root},
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: extra},
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Path: []string{"user"},
						Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							{Name: []byte("age"), Value: &Integer{Path: []string{"age"}}},
							{
								Name: []byte("posts"),
								Value: &Array{
									Path: []string{"posts"},
									Item: &Object{
										Fields: []*Field{
											{Name: []byte("title"), Value: &String{Path: []string{"title"}}},
											{Name: []byte("score"), Value: &Integer{Path: []string{"score"}}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	program := &planbytecode.Program{
		Strings: []string{"user", "id", "name", "age", "posts", "title", "score"},
		Paths:   [][]string{nil, {"user"}, {"id"}, {"name"}, {"age"}, {"posts"}, {"title"}, {"score"}},
		Ops: []planbytecode.Op{
			{Code: planbytecode.OpEnterSequence, A: 2, B: 5},
			{Code: planbytecode.OpFetchSubgraph, A: 0},
			{Code: planbytecode.OpPasteAtPointer, A: 0},
			{Code: planbytecode.OpFetchSubgraph, A: 1},
			{Code: planbytecode.OpPasteAtPointer, A: 1},
			{Code: planbytecode.OpLeaveSequence},
			{Code: planbytecode.OpEnterObject, A: 0, B: 1},
			{Code: planbytecode.OpProjectField, A: 0, B: 1, C: bytecodeTestNodeFlags(NodeKindObject, false)},
			{Code: planbytecode.OpEnterObject, A: 1, B: 4},
			{Code: planbytecode.OpProjectField, A: 1, B: 2, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpProjectField, A: 2, B: 3, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpProjectField, A: 3, B: 4, C: bytecodeTestNodeFlags(NodeKindInteger, false)},
			{Code: planbytecode.OpProjectField, A: 4, B: 5, C: bytecodeTestNodeFlags(NodeKindArray, false)},
			{Code: planbytecode.OpEnterArray, A: 5},
			{Code: planbytecode.OpEnterObject, A: 0, B: 2},
			{Code: planbytecode.OpProjectField, A: 5, B: 6, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpProjectField, A: 6, B: 7, C: bytecodeTestNodeFlags(NodeKindInteger, false)},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpLeaveArray},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpEmitResponse},
		},
		Fetches: []planbytecode.Fetch{
			{Item: root},
			{Item: extra},
		},
		DirectResponse: &planbytecode.DirectResponse{
			Fields: []planbytecode.DirectField{
				{
					NameRef: 0,
					PathRef: 1,
					Flags:   planbytecode.EncodeDirectFieldFlags(uint32(NodeKindObject), false, false),
					Children: []planbytecode.DirectField{
						{NameRef: 1, PathRef: 2, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
						{NameRef: 2, PathRef: 3, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
						{NameRef: 3, PathRef: 4, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindInteger), false, false)},
						{
							NameRef:   4,
							PathRef:   5,
							Flags:     planbytecode.EncodeDirectFieldFlags(uint32(NodeKindArray), false, false),
							ItemFlags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindObject), false, false),
							Children: []planbytecode.DirectField{
								{NameRef: 5, PathRef: 6, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
								{NameRef: 6, PathRef: 7, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindInteger), false, false)},
							},
						},
					},
				},
			},
		},
	}
	return response, program
}

func bytecodeMergePathTestResponse() (*GraphQLResponse, *planbytecode.Program) {
	root := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"user":{"id":"1"}}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "root"},
		},
	}
	extra := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"age":37}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
					MergePath:              []string{"user"},
				},
			},
			Info: &FetchInfo{DataSourceName: "extra"},
		},
	}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: root},
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: extra},
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Path: []string{"user"},
						Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							{Name: []byte("age"), Value: &Integer{Path: []string{"age"}}},
						},
					},
				},
			},
		},
	}
	program := &planbytecode.Program{
		Strings: []string{"user", "id", "age"},
		Paths:   [][]string{nil, {"user"}, {"id"}, {"age"}},
		Ops: []planbytecode.Op{
			{Code: planbytecode.OpEnterSequence, A: 2, B: 5},
			{Code: planbytecode.OpFetchSubgraph, A: 0},
			{Code: planbytecode.OpPasteAtPointer, A: 0},
			{Code: planbytecode.OpFetchSubgraph, A: 1},
			{Code: planbytecode.OpPasteAtPointer, A: 1},
			{Code: planbytecode.OpLeaveSequence},
			{Code: planbytecode.OpEnterObject, A: 0, B: 1},
			{Code: planbytecode.OpProjectField, A: 0, B: 1, C: bytecodeTestNodeFlags(NodeKindObject, false)},
			{Code: planbytecode.OpEnterObject, A: 1, B: 2},
			{Code: planbytecode.OpProjectField, A: 1, B: 2, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpProjectField, A: 2, B: 3, C: bytecodeTestNodeFlags(NodeKindInteger, false)},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpEmitResponse},
		},
		Fetches: []planbytecode.Fetch{
			{Item: root},
			{Item: extra},
		},
		DirectResponse: &planbytecode.DirectResponse{
			Fields: []planbytecode.DirectField{
				{
					NameRef: 0,
					PathRef: 1,
					Flags:   planbytecode.EncodeDirectFieldFlags(uint32(NodeKindObject), false, false),
					Children: []planbytecode.DirectField{
						{NameRef: 1, PathRef: 2, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
						{NameRef: 2, PathRef: 3, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindInteger), false, false)},
					},
				},
			},
		},
	}
	return response, program
}

func bytecodeBatchEntityTestResponse() (*GraphQLResponse, *planbytecode.Program) {
	root := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"products":[{"name":"Table","upc":"1"},{"name":"Couch","upc":"2"}]}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "products"},
		},
	}
	stock := &FetchItem{
		FetchPath: []FetchItemPathElement{ArrayPath("products")},
		Fetch: &BatchEntityFetch{
			Input: BatchInput{
				Header: InputTemplate{
					Segments: []TemplateSegment{{Data: []byte(`[`), SegmentType: StaticSegmentType}},
				},
				Items: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
									Fields: []*Field{
										{Name: []byte("upc"), Value: &String{Path: []string{"upc"}}},
									},
								}),
							},
						},
					},
				},
				Separator: InputTemplate{
					Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}},
				},
				Footer: InputTemplate{
					Segments: []TemplateSegment{{Data: []byte(`]`), SegmentType: StaticSegmentType}},
				},
			},
			DataSource: FakeDataSource(`{"data":{"_entities":[{"stock":8},{"stock":2}]}}`),
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data", "_entities"},
			},
			Info: &FetchInfo{DataSourceName: "stock"},
		},
	}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: root},
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: stock},
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Path: []string{"products"},
						Item: &Object{
							Fields: []*Field{
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{Name: []byte("stock"), Value: &Integer{Path: []string{"stock"}}},
							},
						},
					},
				},
			},
		},
	}
	program := &planbytecode.Program{
		Strings: []string{"products", "name", "stock"},
		Paths:   [][]string{nil, {"products"}, {"name"}, {"stock"}},
		Ops: []planbytecode.Op{
			{Code: planbytecode.OpEnterSequence, A: 2, B: 5},
			{Code: planbytecode.OpFetchSubgraph, A: 0},
			{Code: planbytecode.OpPasteAtPointer, A: 0},
			{Code: planbytecode.OpFetchSubgraph, A: 1},
			{Code: planbytecode.OpPasteAtPointer, A: 1},
			{Code: planbytecode.OpLeaveSequence},
			{Code: planbytecode.OpEnterObject, A: 0, B: 1},
			{Code: planbytecode.OpProjectField, A: 0, B: 1, C: bytecodeTestNodeFlags(NodeKindArray, false)},
			{Code: planbytecode.OpEnterArray, A: 1},
			{Code: planbytecode.OpEnterObject, A: 0, B: 2},
			{Code: planbytecode.OpProjectField, A: 1, B: 2, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpProjectField, A: 2, B: 3, C: bytecodeTestNodeFlags(NodeKindInteger, false)},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpLeaveArray},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpEmitResponse},
		},
		Fetches: []planbytecode.Fetch{
			{Item: root},
			{Item: stock},
		},
		DirectResponse: &planbytecode.DirectResponse{
			Fields: []planbytecode.DirectField{
				{
					NameRef:   0,
					PathRef:   1,
					Flags:     planbytecode.EncodeDirectFieldFlags(uint32(NodeKindArray), false, false),
					ItemFlags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindObject), false, false),
					Children: []planbytecode.DirectField{
						{NameRef: 1, PathRef: 2, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
						{NameRef: 2, PathRef: 3, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindInteger), false, false)},
					},
				},
			},
		},
	}
	return response, program
}

func bytecodeEntityTestResponse() (*GraphQLResponse, *planbytecode.Program) {
	root := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"user":{"id":"1","name":"Ada"}}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "users"},
		},
	}
	profile := &FetchItem{
		FetchPath: []FetchItemPathElement{ObjectPath("user")},
		Fetch: &EntityFetch{
			Input: EntityInput{
				Header: InputTemplate{
					Segments: []TemplateSegment{{Data: []byte(`[`), SegmentType: StaticSegmentType}},
				},
				Item: InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType:  VariableSegmentType,
							VariableKind: ResolvableObjectVariableKind,
							Renderer: NewGraphQLVariableResolveRenderer(&Object{
								Fields: []*Field{
									{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
								},
							}),
						},
					},
				},
				Footer: InputTemplate{
					Segments: []TemplateSegment{{Data: []byte(`]`), SegmentType: StaticSegmentType}},
				},
			},
			DataSource: FakeDataSource(`{"data":{"_entities":[{"age":37}]}}`),
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data", "_entities", "0"},
			},
			Info: &FetchInfo{DataSourceName: "profiles"},
		},
	}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: root},
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: profile},
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Path: []string{"user"},
						Fields: []*Field{
							{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
							{Name: []byte("age"), Value: &Integer{Path: []string{"age"}}},
						},
					},
				},
			},
		},
	}
	program := &planbytecode.Program{
		Strings: []string{"user", "name", "age"},
		Paths:   [][]string{nil, {"user"}, {"name"}, {"age"}},
		Ops: []planbytecode.Op{
			{Code: planbytecode.OpEnterSequence, A: 2, B: 5},
			{Code: planbytecode.OpFetchSubgraph, A: 0},
			{Code: planbytecode.OpPasteAtPointer, A: 0},
			{Code: planbytecode.OpFetchSubgraph, A: 1},
			{Code: planbytecode.OpPasteAtPointer, A: 1},
			{Code: planbytecode.OpLeaveSequence},
			{Code: planbytecode.OpEnterObject, A: 0, B: 1},
			{Code: planbytecode.OpProjectField, A: 0, B: 1, C: bytecodeTestNodeFlags(NodeKindObject, false)},
			{Code: planbytecode.OpEnterObject, A: 1, B: 2},
			{Code: planbytecode.OpProjectField, A: 1, B: 2, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpProjectField, A: 2, B: 3, C: bytecodeTestNodeFlags(NodeKindInteger, false)},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpEmitResponse},
		},
		Fetches: []planbytecode.Fetch{
			{Item: root},
			{Item: profile},
		},
		DirectResponse: &planbytecode.DirectResponse{
			Fields: []planbytecode.DirectField{
				{
					NameRef: 0,
					PathRef: 1,
					Flags:   planbytecode.EncodeDirectFieldFlags(uint32(NodeKindObject), false, false),
					Children: []planbytecode.DirectField{
						{NameRef: 1, PathRef: 2, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
						{NameRef: 2, PathRef: 3, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindInteger), false, false)},
					},
				},
			},
		},
	}
	return response, program
}

func bytecodeNestedBatchEntityTestResponse() (*GraphQLResponse, *planbytecode.Program) {
	root := &FetchItem{
		Fetch: &SingleFetch{
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{{Data: []byte(`{}`), SegmentType: StaticSegmentType}},
			},
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"data":{"products":[{"name":"Table","reviews":[{"body":"Love Table","author":{"id":"1"}},{"body":"Prefer Desk","author":{"id":"2"}}]},{"name":"Couch","reviews":[{"body":"Too expensive","author":{"id":"1"}}]}]}}`),
				PostProcessing: PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
			Info: &FetchInfo{DataSourceName: "products"},
		},
	}
	authors := &FetchItem{
		FetchPath: []FetchItemPathElement{ArrayPath("products"), ArrayPath("reviews"), ObjectPath("author")},
		Fetch: &BatchEntityFetch{
			Input: BatchInput{
				Header: InputTemplate{
					Segments: []TemplateSegment{{Data: []byte(`[`), SegmentType: StaticSegmentType}},
				},
				Items: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:  VariableSegmentType,
								VariableKind: ResolvableObjectVariableKind,
								Renderer: NewGraphQLVariableResolveRenderer(&Object{
									Fields: []*Field{
										{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
									},
								}),
							},
						},
					},
				},
				Separator: InputTemplate{
					Segments: []TemplateSegment{{Data: []byte(`,`), SegmentType: StaticSegmentType}},
				},
				Footer: InputTemplate{
					Segments: []TemplateSegment{{Data: []byte(`]`), SegmentType: StaticSegmentType}},
				},
			},
			DataSource: FakeDataSource(`{"data":{"_entities":[{"name":"user-1"},{"name":"user-2"}]}}`),
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data", "_entities"},
			},
			Info: &FetchInfo{DataSourceName: "authors"},
		},
	}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: root},
			&FetchTreeNode{Kind: FetchTreeNodeKindSingle, Item: authors},
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("products"),
					Value: &Array{
						Path: []string{"products"},
						Item: &Object{
							Fields: []*Field{
								{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
								{
									Name: []byte("reviews"),
									Value: &Array{
										Path: []string{"reviews"},
										Item: &Object{
											Fields: []*Field{
												{Name: []byte("body"), Value: &String{Path: []string{"body"}}},
												{
													Name: []byte("author"),
													Value: &Object{
														Path: []string{"author"},
														Fields: []*Field{
															{Name: []byte("name"), Value: &String{Path: []string{"name"}}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	program := &planbytecode.Program{
		Strings: []string{"products", "name", "reviews", "body", "author"},
		Paths:   [][]string{nil, {"products"}, {"name"}, {"reviews"}, {"body"}, {"author"}},
		Ops: []planbytecode.Op{
			{Code: planbytecode.OpEnterSequence, A: 2, B: 5},
			{Code: planbytecode.OpFetchSubgraph, A: 0},
			{Code: planbytecode.OpPasteAtPointer, A: 0},
			{Code: planbytecode.OpFetchSubgraph, A: 1},
			{Code: planbytecode.OpPasteAtPointer, A: 1},
			{Code: planbytecode.OpLeaveSequence},
			{Code: planbytecode.OpEnterObject, A: 0, B: 1},
			{Code: planbytecode.OpProjectField, A: 0, B: 1, C: bytecodeTestNodeFlags(NodeKindArray, false)},
			{Code: planbytecode.OpEnterArray, A: 1},
			{Code: planbytecode.OpEnterObject, A: 0, B: 2},
			{Code: planbytecode.OpProjectField, A: 1, B: 2, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpProjectField, A: 2, B: 3, C: bytecodeTestNodeFlags(NodeKindArray, false)},
			{Code: planbytecode.OpEnterArray, A: 3},
			{Code: planbytecode.OpEnterObject, A: 0, B: 2},
			{Code: planbytecode.OpProjectField, A: 3, B: 4, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpProjectField, A: 4, B: 5, C: bytecodeTestNodeFlags(NodeKindObject, false)},
			{Code: planbytecode.OpEnterObject, A: 5, B: 1},
			{Code: planbytecode.OpProjectField, A: 1, B: 2, C: bytecodeTestNodeFlags(NodeKindString, false)},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpLeaveArray},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpLeaveArray},
			{Code: planbytecode.OpLeaveObject},
			{Code: planbytecode.OpEmitResponse},
		},
		Fetches: []planbytecode.Fetch{
			{Item: root},
			{Item: authors},
		},
		DirectResponse: &planbytecode.DirectResponse{
			Fields: []planbytecode.DirectField{
				{
					NameRef:   0,
					PathRef:   1,
					Flags:     planbytecode.EncodeDirectFieldFlags(uint32(NodeKindArray), false, false),
					ItemFlags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindObject), false, false),
					Children: []planbytecode.DirectField{
						{NameRef: 1, PathRef: 2, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
						{
							NameRef:   2,
							PathRef:   3,
							Flags:     planbytecode.EncodeDirectFieldFlags(uint32(NodeKindArray), false, false),
							ItemFlags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindObject), false, false),
							Children: []planbytecode.DirectField{
								{NameRef: 3, PathRef: 4, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
								{
									NameRef: 4,
									PathRef: 5,
									Flags:   planbytecode.EncodeDirectFieldFlags(uint32(NodeKindObject), false, false),
									Children: []planbytecode.DirectField{
										{NameRef: 1, PathRef: 2, Flags: planbytecode.EncodeDirectFieldFlags(uint32(NodeKindString), false, false)},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return response, program
}
