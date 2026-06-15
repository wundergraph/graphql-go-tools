package resolve

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

type controlledLoaderDataSource struct {
	response []byte
	err      error

	waitFor       <-chan struct{}
	waitForCancel bool

	startedOnce sync.Once
	doneOnce    sync.Once
	started     chan struct{}
	done        chan struct{}

	mu     sync.Mutex
	inputs []string

	cancelled atomic.Bool
	loadCalls atomic.Int64
}

func newControlledLoaderDataSource(response string) *controlledLoaderDataSource {
	return &controlledLoaderDataSource{
		response: []byte(response),
		started:  make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (d *controlledLoaderDataSource) Load(ctx context.Context, _ http.Header, input []byte) ([]byte, error) {
	d.loadCalls.Add(1)

	d.mu.Lock()
	d.inputs = append(d.inputs, string(input))
	d.mu.Unlock()

	d.startedOnce.Do(func() {
		close(d.started)
	})
	defer d.doneOnce.Do(func() {
		close(d.done)
	})

	if d.waitForCancel {
		select {
		case <-ctx.Done():
			d.cancelled.Store(true)
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return nil, errors.New("timed out waiting for cancellation")
		}
	}

	if d.waitFor != nil {
		select {
		case <-d.waitFor:
		case <-ctx.Done():
			d.cancelled.Store(true)
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return nil, errors.New("timed out waiting for load release")
		}
	}

	if d.err != nil {
		return nil, d.err
	}
	return d.response, nil
}

func (d *controlledLoaderDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func (d *controlledLoaderDataSource) requireInputs(t *testing.T, expected ...string) {
	t.Helper()

	d.mu.Lock()
	defer d.mu.Unlock()

	require.Equal(t, expected, d.inputs)
}

func TestLoaderNestedParallelCorrectness(t *testing.T) {
	a := newControlledLoaderDataSource(`{"data":{"a":"A"}}`)
	b := newControlledLoaderDataSource(`{"data":{"b":"B"}}`)
	c := newControlledLoaderDataSource(`{"data":{"c":"C"}}`)

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Parallel(
			Sequence(
				Single(nestedParallelSingleFetch(a, `{"fetch":"A"}`)),
				Single(nestedParallelSingleFetchWithTemplate(b, nestedParallelInputForFields("a"))),
			),
			Single(nestedParallelSingleFetch(c, `{"fetch":"C"}`)),
		),
		Data: nestedParallelData("a", "b", "c"),
	}

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	resolver := newResolver(context.Background())
	buf := &bytes.Buffer{}

	_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, buf)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"a":"A","b":"B","c":"C"}}`, buf.String())
	b.requireInputs(t, `{"a":"A"}`)
}

func TestLoaderNestedParallelRaceMutexDiscipline(t *testing.T) {
	a := newControlledLoaderDataSource(`{"data":{"a":"A"}}`)
	b := newControlledLoaderDataSource(`{"data":{"b":"B"}}`)
	c := newControlledLoaderDataSource(`{"data":{"c":"C"}}`)
	d := newControlledLoaderDataSource(`{"data":{"d":"D"}}`)
	a.waitFor = d.done

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Parallel(
			Sequence(
				Single(nestedParallelSingleFetch(a, `{"fetch":"A"}`)),
				Single(nestedParallelSingleFetchWithTemplate(b, nestedParallelInputForFields("a"))),
			),
			Sequence(
				Single(nestedParallelSingleFetch(c, `{"fetch":"C"}`)),
				Single(nestedParallelSingleFetchWithTemplate(d, nestedParallelInputForFields("c"))),
			),
		),
		Data: nestedParallelData("a", "b", "c", "d"),
	}

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	resolver := newResolver(context.Background())
	buf := &bytes.Buffer{}

	_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, buf)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"a":"A","b":"B","c":"C","d":"D"}}`, buf.String())
	require.Eventually(t, func() bool {
		select {
		case <-a.started:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	require.Eventually(t, func() bool {
		select {
		case <-d.done:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	b.requireInputs(t, `{"a":"A"}`)
	d.requireInputs(t, `{"c":"C"}`)
}

func TestLoaderFlatParallelKeepsFastPathWithoutMergeMutex(t *testing.T) {
	a := newControlledLoaderDataSource(`{"data":{"a":"A"}}`)
	b := newControlledLoaderDataSource(`{"data":{"b":"B"}}`)
	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Parallel(
			Single(nestedParallelSingleFetch(a, `{"fetch":"A"}`)),
			Single(nestedParallelSingleFetch(b, `{"fetch":"B"}`)),
		),
		Data: nestedParallelData("a", "b"),
	}

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	resolvable := NewResolvable(nil, ResolvableOptions{})
	loader := &Loader{}

	require.NoError(t, resolvable.Init(ctx, nil, ast.OperationTypeQuery))
	require.NoError(t, loader.LoadGraphQLResponseData(ctx, response, resolvable))
	require.False(t, loader.useMergeMu)
	require.Equal(t, `{"data":{"a":"A","b":"B"}}`, fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors))
}

func TestFetchTreeHasNestedParallel(t *testing.T) {
	largeFlatChildren := make([]*FetchTreeNode, 100)
	for i := range largeFlatChildren {
		largeFlatChildren[i] = Single(&SingleFetch{})
	}

	tests := []struct {
		name string
		node *FetchTreeNode
		want bool
	}{
		{name: "nil", node: nil, want: false},
		{name: "single", node: Single(&SingleFetch{}), want: false},
		{name: "sequence of singles", node: Sequence(Single(&SingleFetch{}), Single(&SingleFetch{})), want: false},
		{name: "flat parallel of singles", node: Parallel(Single(&SingleFetch{}), Single(&SingleFetch{})), want: false},
		{name: "parallel with sequence child", node: Parallel(Sequence(Single(&SingleFetch{}), Single(&SingleFetch{})), Single(&SingleFetch{})), want: true},
		{name: "deeply buried nested parallel", node: Sequence(Sequence(Sequence(Parallel(Single(&SingleFetch{}), Sequence(Single(&SingleFetch{})))))), want: true},
		{name: "large flat parallel", node: Parallel(largeFlatChildren...), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, fetchTreeHasNestedParallel(tt.node))
		})
	}
}

func TestResolveParallelNestedDoesNotCancelSiblings(t *testing.T) {
	boom := errors.New("boom")
	failing := newControlledLoaderDataSource(``)
	failing.err = boom
	sibling := newControlledLoaderDataSource(`{"data":{"sibling":"ok"}}`)
	sibling.waitFor = failing.done

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Parallel(
			Sequence(SingleWithPath(nestedParallelSingleFetchWithInfo(failing, `{"fetch":"failing"}`, "Failing", "Failing"), "query.failing")),
			Sequence(SingleWithPath(nestedParallelSingleFetchWithInfo(sibling, `{"fetch":"sibling"}`, "Sibling", "Sibling"), "query.sibling")),
		),
		Data: nestedParallelNullableData("failing", "sibling"),
	}

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	resolver := newResolver(context.Background())
	buf := &bytes.Buffer{}

	_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, buf)
	require.NoError(t, err)
	require.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph 'Failing' at Path 'query.failing'."}],"data":{"failing":null,"sibling":"ok"}}`, buf.String())
	require.False(t, sibling.cancelled.Load())
	require.Equal(t, int64(1), sibling.loadCalls.Load())
}

func TestResolveParallelNestedSequenceStopsAfterFailedDependency(t *testing.T) {
	boom := errors.New("boom")
	a := newControlledLoaderDataSource(``)
	a.err = boom
	b := newControlledLoaderDataSource(`{"data":{"b":"should not run"}}`)
	c := newControlledLoaderDataSource(`{"data":{"c":"ok"}}`)
	aFetch := nestedParallelSingleFetchWithInfo(a, `{"fetch":"a"}`, "A", "A")
	aFetch.FetchDependencies.FetchID = 0
	bFetch := nestedParallelSingleFetchWithInfoAndTemplate(b, nestedParallelInputForFields("a"), "B", "B")
	bFetch.FetchDependencies.FetchID = 1
	bFetch.FetchDependencies.DependsOnFetchIDs = []int{0}
	bFetch.PostProcessing.SelectResponseErrorsPath = []string{"errors"}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Parallel(
			Sequence(
				SingleWithPath(aFetch, "query.a"),
				SingleWithPath(bFetch, "query.b"),
			),
			SingleWithPath(nestedParallelSingleFetchWithInfo(c, `{"fetch":"c"}`, "C", "C"), "query.c"),
		),
		Data: nestedParallelNullableData("a", "b", "c"),
	}

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	resolver := newResolver(context.Background())
	buf := &bytes.Buffer{}

	_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, buf)
	require.NoError(t, err)
	require.Equal(t, int64(0), b.loadCalls.Load())
	require.Equal(t, int64(1), c.loadCalls.Load())
	require.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph 'A' at Path 'query.a'."}],"data":{"a":null,"b":null,"c":"ok"}}`, buf.String())
}

func TestResolveSerialStopsTransitivelyOnFailedDependency(t *testing.T) {
	boom := errors.New("boom")
	a := newControlledLoaderDataSource(``)
	a.err = boom
	b := newControlledLoaderDataSource(`{"data":{"b":"should not run"}}`)
	c := newControlledLoaderDataSource(`{"data":{"c":"should not run"}}`)

	aFetch := nestedParallelSingleFetchWithInfo(a, `{"fetch":"a"}`, "A", "A")
	aFetch.FetchDependencies.FetchID = 0
	bFetch := nestedParallelSingleFetchWithInfoAndTemplate(b, nestedParallelInputForFields("a"), "B", "B")
	bFetch.FetchDependencies.FetchID = 1
	bFetch.FetchDependencies.DependsOnFetchIDs = []int{0}
	cFetch := nestedParallelSingleFetchWithInfoAndTemplate(c, nestedParallelInputForFields("b"), "C", "C")
	cFetch.FetchDependencies.FetchID = 2
	cFetch.FetchDependencies.DependsOnFetchIDs = []int{1}

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(aFetch, "query.a"),
			SingleWithPath(bFetch, "query.b"),
			SingleWithPath(cFetch, "query.c"),
		),
		Data: nestedParallelNullableData("a", "b", "c"),
	}

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	resolver := newResolver(context.Background())
	buf := &bytes.Buffer{}

	_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, buf)
	require.NoError(t, err)
	require.Equal(t, int64(0), b.loadCalls.Load())
	require.Equal(t, int64(0), c.loadCalls.Load())
	require.Equal(t, `{"errors":[{"message":"Failed to fetch from Subgraph 'A' at Path 'query.a'."}],"data":{"a":null,"b":null,"c":null}}`, buf.String())
}

func nestedParallelSingleFetch(ds DataSource, input string) *SingleFetch {
	return nestedParallelSingleFetchWithTemplate(ds, InputTemplate{
		Segments: []TemplateSegment{{
			SegmentType: StaticSegmentType,
			Data:        []byte(input),
		}},
	})
}

func nestedParallelSingleFetchWithInfo(ds DataSource, input, id, name string) *SingleFetch {
	return nestedParallelSingleFetchWithInfoAndTemplate(ds, InputTemplate{
		Segments: []TemplateSegment{{
			SegmentType: StaticSegmentType,
			Data:        []byte(input),
		}},
	}, id, name)
}

func nestedParallelSingleFetchWithInfoAndTemplate(ds DataSource, input InputTemplate, id, name string) *SingleFetch {
	fetch := nestedParallelSingleFetchWithTemplate(ds, input)
	fetch.Info = &FetchInfo{
		DataSourceID:   id,
		DataSourceName: name,
	}
	return fetch
}

func nestedParallelSingleFetchWithTemplate(ds DataSource, input InputTemplate) *SingleFetch {
	return &SingleFetch{
		InputTemplate: input,
		FetchConfiguration: FetchConfiguration{
			DataSource: ds,
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath: []string{"data"},
			},
		},
	}
}

func nestedParallelInputForFields(fields ...string) InputTemplate {
	object := &Object{Fields: make([]*Field, 0, len(fields))}
	for _, field := range fields {
		object.Fields = append(object.Fields, &Field{
			Name: []byte(field),
			Value: &String{
				Path: []string{field},
			},
		})
	}
	return InputTemplate{
		Segments: []TemplateSegment{{
			SegmentType:  VariableSegmentType,
			VariableKind: ResolvableObjectVariableKind,
			Renderer:     NewGraphQLVariableResolveRenderer(object),
		}},
	}
}

func nestedParallelData(fields ...string) *Object {
	data := &Object{Fields: make([]*Field, 0, len(fields))}
	for _, field := range fields {
		data.Fields = append(data.Fields, &Field{
			Name: []byte(field),
			Value: &String{
				Path: []string{field},
			},
		})
	}
	return data
}

func nestedParallelNullableData(fields ...string) *Object {
	data := &Object{Fields: make([]*Field, 0, len(fields))}
	for _, field := range fields {
		data.Fields = append(data.Fields, &Field{
			Name: []byte(field),
			Value: &String{
				Path:     []string{field},
				Nullable: true,
			},
		})
	}
	return data
}
