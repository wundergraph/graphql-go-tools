package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func deferExtensionFetch(fetchID int, sourceName, payload string) *FetchTreeNode {
	return Single(&SingleFetch{
		FetchConfiguration: FetchConfiguration{
			DataSource: FakeDataSource(payload),
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath:   []string{"data"},
				SelectResponseErrorsPath: []string{"errors"},
			},
		},
		FetchDependencies: FetchDependencies{FetchID: fetchID},
		InputTemplate: InputTemplate{Segments: []TemplateSegment{{
			SegmentType: StaticSegmentType,
			Data:        []byte(`{}`),
		}}},
		DataSourceIdentifier: []byte(sourceName),
		Info: &FetchInfo{
			DataSourceID:   sourceName,
			DataSourceName: sourceName,
			OperationType:  ast.OperationTypeQuery,
		},
	})
}

func parallelDeferExtensionResponse(primaryPayload, firstPayload, secondPayload string) *GraphQLDeferResponse {
	first := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(2, "deferred-one", firstPayload)}
	second := &DeferFetchGroup{DeferID: 2, Fetches: deferExtensionFetch(3, "deferred-two", secondPayload)}
	return &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Label: "First"},
			2: {ID: 2, Label: "Second"},
		},
		DeferTree: DeferParallel(DeferSingle(first), DeferSingle(second)),
		Response: &GraphQLResponse{
			Info:    &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: deferExtensionFetch(1, "primary", primaryPayload),
			Data: &Object{Nullable: true, Fields: []*Field{
				{Name: []byte("fast"), Value: &String{Path: []string{"fast"}, Nullable: true}},
				deferredField("f1", 1, &String{Path: []string{"f1"}, Nullable: true}, nil),
				deferredField("f2", 2, &String{Path: []string{"f2"}, Nullable: true}, nil),
			}},
		},
	}
}

func deferExtensionContext() *Context {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.IncludeQueryPlanInResponse = true
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.TracingOptions = TraceOptions{
		Enable:                                 true,
		IncludeTraceOutputInResponseExtensions: true,
		EnablePredictableDebugTimings:          true,
		Debug:                                  true,
	}
	ctx.ctx = SetTraceStart(ctx.ctx, true)
	return ctx
}

func decodeDeferPayload(t *testing.T, payload string) map[string]any {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	return decoded
}

type signalingDeferWriter struct {
	testDeferWriter
	flushed chan struct{}
}

type evolvingDeferAuthorizer struct {
	renders int
}

func (*evolvingDeferAuthorizer) AuthorizePreFetch(*Context, string, json.RawMessage, GraphCoordinate) (*AuthorizationDeny, error) {
	return nil, nil
}

func (*evolvingDeferAuthorizer) AuthorizeObjectField(*Context, string, json.RawMessage, GraphCoordinate) (*AuthorizationDeny, error) {
	return nil, nil
}

func (*evolvingDeferAuthorizer) HasResponseExtensionData(*Context) bool { return true }

func (a *evolvingDeferAuthorizer) RenderResponseExtension(_ *Context, out io.Writer) error {
	a.renders++
	_, err := fmt.Fprintf(out, `{"render":%d}`, a.renders)
	return err
}

type evolvingDeferRateLimiter struct {
	calls int
}

func (l *evolvingDeferRateLimiter) RateLimitPreFetch(*Context, *FetchInfo, json.RawMessage) (*RateLimitDeny, error) {
	l.calls++
	return nil, nil
}

func (l *evolvingDeferRateLimiter) RenderResponseExtension(_ *Context, out io.Writer) error {
	_, err := fmt.Fprintf(out, `{"calls":%d}`, l.calls)
	return err
}

type terminalFailingDeferAuthorizer struct {
	renders int
}

func (*terminalFailingDeferAuthorizer) AuthorizePreFetch(*Context, string, json.RawMessage, GraphCoordinate) (*AuthorizationDeny, error) {
	return nil, nil
}

func (*terminalFailingDeferAuthorizer) AuthorizeObjectField(*Context, string, json.RawMessage, GraphCoordinate) (*AuthorizationDeny, error) {
	return nil, nil
}

func (*terminalFailingDeferAuthorizer) HasResponseExtensionData(*Context) bool { return true }

func (a *terminalFailingDeferAuthorizer) RenderResponseExtension(_ *Context, out io.Writer) error {
	a.renders++
	if a.renders == 2 {
		return errors.New("terminal extension render failed")
	}
	_, err := io.WriteString(out, `{}`)
	return err
}

func newSignalingDeferWriter() *signalingDeferWriter {
	return &signalingDeferWriter{flushed: make(chan struct{}, 4)}
}

func (w *signalingDeferWriter) Flush() error {
	if err := w.testDeferWriter.Flush(); err != nil {
		return err
	}
	w.flushed <- struct{}{}
	return nil
}

func deferStatuses(t *testing.T, trace any) map[int]string {
	t.Helper()
	statuses := make(map[int]string)
	traceObject, ok := trace.(map[string]any)
	require.True(t, ok)
	root, ok := traceObject["fetches"].(map[string]any)
	require.True(t, ok)
	var walk func(map[string]any)
	walk = func(node map[string]any) {
		if descriptor, ok := node["defer"].(map[string]any); ok {
			id, idOK := descriptor["id"].(float64)
			status, statusOK := descriptor["status"].(string)
			require.True(t, idOK)
			require.True(t, statusOK)
			statuses[int(id)] = status
		}
		if children, ok := node["children"].([]any); ok {
			for _, child := range children {
				childObject, childOK := child.(map[string]any)
				require.True(t, childOK)
				walk(childObject)
			}
		}
	}
	walk(root)
	return statuses
}

func TestDeferExtensions_InitialSnapshotAndTerminalCumulativeState(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{
		MaxConcurrency:                 32,
		AllowCustomExtensionProperties: true,
	})
	response := parallelDeferExtensionResponse(
		`{"data":{"fast":"ready"},"extensions":{"initialOnly":"primary"}}`,
		`{"data":{"f1":"one"},"extensions":{"deferredOne":1}}`,
		`{"data":{"f2":"two"},"extensions":{"deferredTwo":2}}`,
	)

	writer := &testDeferWriter{}
	resolveContext := deferExtensionContext()
	_, err := resolver.ResolveGraphQLDeferResponse(resolveContext, response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 3)
	require.True(t, writer.complete)

	initial := decodeDeferPayload(t, writer.payloads[0])
	require.Equal(t, true, initial["hasNext"])
	initialExtensions := initial["extensions"].(map[string]any)
	require.Equal(t, "primary", initialExtensions["initialOnly"])
	require.NotContains(t, initialExtensions, "deferredOne")
	require.NotContains(t, initialExtensions, "deferredTwo")
	require.Empty(t, deferStatuses(t, initialExtensions["trace"]),
		"the active-stream trace must remain primary-only")
	queryPlan, err := json.Marshal(initialExtensions["queryPlan"])
	require.NoError(t, err)
	require.Contains(t, string(queryPlan), `"defer":{"id":1`)
	require.Contains(t, string(queryPlan), `"defer":{"id":2`)

	intermediate := decodeDeferPayload(t, writer.payloads[1])
	require.Equal(t, true, intermediate["hasNext"])
	require.NotContains(t, intermediate, "extensions",
		"non-terminal patches must not gain redundant extension objects")

	terminal := decodeDeferPayload(t, writer.payloads[2])
	require.Equal(t, false, terminal["hasNext"])
	terminalExtensions := terminal["extensions"].(map[string]any)
	require.Equal(t, "primary", terminalExtensions["initialOnly"])
	require.Equal(t, float64(1), terminalExtensions["deferredOne"])
	require.Equal(t, float64(2), terminalExtensions["deferredTwo"])
	require.Equal(t, map[int]string{
		1: string(DeferExecutionStatusCompleted),
		2: string(DeferExecutionStatusCompleted),
	}, deferStatuses(t, terminalExtensions["trace"]))
	require.Equal(t, initialExtensions["queryPlan"], terminalExtensions["queryPlan"])
}

func TestDeferExtensions_CrossPhaseConflictUsesConfiguredShallowAlgorithm(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		algorithm ExtensionForwardingAlgorithm
		want      string
	}{
		{name: "first write", algorithm: ExtensionForwardingAlgorithmFirstWrite, want: "primary"},
		{name: "last write", algorithm: ExtensionForwardingAlgorithmLastWrite, want: "deferred"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			resolver := New(t.Context(), ResolverOptions{
				MaxConcurrency:                 32,
				AllowCustomExtensionProperties: true,
				ResolvableOptions: ResolvableOptions{
					ExtensionForwardingAlgorithm: test.algorithm,
				},
			})
			group := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(2, "deferred", `{"data":{"f1":"done"},"extensions":{"winner":"deferred"}}`)}
			response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, nil, group)
			response.Response.Fetches = deferExtensionFetch(1, "primary", `{"data":{},"extensions":{"winner":"primary"}}`)

			writer := &testDeferWriter{}
			_, err := resolver.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, writer)
			require.NoError(t, err)
			require.Len(t, writer.payloads, 2)
			initial := decodeDeferPayload(t, writer.payloads[0])
			require.Equal(t, "primary", initial["extensions"].(map[string]any)["winner"])
			terminal := decodeDeferPayload(t, writer.payloads[1])
			require.Equal(t, test.want, terminal["extensions"].(map[string]any)["winner"])
		})
	}
}

func TestDeferExtensions_ParallelConflictUsesSharedMergeOrder(t *testing.T) {
	for _, test := range []struct {
		name      string
		algorithm ExtensionForwardingAlgorithm
		want      string
	}{
		{name: "first write", algorithm: ExtensionForwardingAlgorithmFirstWrite, want: "second released"},
		{name: "last write", algorithm: ExtensionForwardingAlgorithmLastWrite, want: "first released"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := New(t.Context(), ResolverOptions{
				MaxConcurrency:                 32,
				AllowCustomExtensionProperties: true,
				ResolvableOptions: ResolvableOptions{
					ExtensionForwardingAlgorithm: test.algorithm,
				},
			})

			firstSource := newBlockingDataSource([]byte(`{"data":{"f1":"one"},"extensions":{"winner":"first released"}}`))
			secondSource := newBlockingDataSource([]byte(`{"data":{"f2":"two"},"extensions":{"winner":"second released"}}`))
			response := parallelDeferExtensionResponse(`{"data":{"fast":"ready"}}`, `{}`, `{}`)
			response.DeferTree.ChildNodes[0].Item.Fetches.Item.Fetch.(*SingleFetch).DataSource = firstSource
			response.DeferTree.ChildNodes[1].Item.Fetches.Item.Fetch.(*SingleFetch).DataSource = secondSource

			writer := newSignalingDeferWriter()
			done := make(chan error, 1)
			go func() {
				_, err := resolver.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, writer)
				done <- err
			}()

			<-writer.flushed // initial frame
			<-firstSource.Ready()
			<-secondSource.Ready()
			secondSource.Release()
			<-writer.flushed // secondSource merged and rendered first
			runtime.GC()     // accumulator must retain the arena-backed extension
			firstSource.Release()
			require.NoError(t, <-done)

			require.Len(t, writer.payloads, 3)
			terminal := decodeDeferPayload(t, writer.payloads[2])
			require.Equal(t, test.want, terminal["extensions"].(map[string]any)["winner"])
		})
	}
}

func TestDeferExtensions_AllowListAndReservedKeys(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{
		MaxConcurrency:                 32,
		AllowCustomExtensionProperties: true,
		ResolvableOptions: ResolvableOptions{AllowedSubgraphExtensions: map[string]struct{}{
			"keep":            {},
			"trace":           {},
			"queryPlan":       {},
			"authorization":   {},
			"rateLimit":       {},
			"valueCompletion": {},
		}},
	})
	group := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(2, "deferred", `{"data":{"f1":"done"},"extensions":{"keep":"deferred","drop":"deferred","trace":"spoofed","queryPlan":"spoofed","authorization":"spoofed","rateLimit":"spoofed","valueCompletion":"spoofed"}}`)}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, nil, group)
	response.Response.Fetches = deferExtensionFetch(1, "primary", `{"data":{},"extensions":{"keep":"primary","drop":"primary","trace":"spoofed","queryPlan":"spoofed","authorization":"spoofed","rateLimit":"spoofed","valueCompletion":"spoofed"}}`)

	writer := &testDeferWriter{}
	_, err := resolver.ResolveGraphQLDeferResponse(deferExtensionContext(), response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 2)
	for _, payload := range writer.payloads {
		extensions := decodeDeferPayload(t, payload)["extensions"].(map[string]any)
		require.Equal(t, "primary", extensions["keep"])
		require.NotContains(t, extensions, "drop")
		require.IsType(t, map[string]any{}, extensions["trace"])
		require.IsType(t, map[string]any{}, extensions["queryPlan"])
		require.NotContains(t, extensions, "authorization")
		require.NotContains(t, extensions, "rateLimit")
		require.NotContains(t, extensions, "valueCompletion")
	}
}

func TestDeferExtensions_RequestLevelExtensionsAreRerenderedTerminally(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{MaxConcurrency: 32})
	group := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(1, "deferred", `{"data":{"f1":"done"}}`)}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, nil, group)
	authorizer := &evolvingDeferAuthorizer{}
	limiter := &evolvingDeferRateLimiter{}
	ctx := NewContext(context.Background())
	ctx.SetAuthorizer(authorizer)
	ctx.SetRateLimiter(limiter)
	ctx.RateLimitOptions = RateLimitOptions{Enable: true, IncludeStatsInResponseExtension: true}

	writer := &testDeferWriter{}
	_, err := resolver.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 2)
	initial := decodeDeferPayload(t, writer.payloads[0])["extensions"].(map[string]any)
	require.Equal(t, float64(1), initial["authorization"].(map[string]any)["render"])
	require.Equal(t, float64(0), initial["rateLimit"].(map[string]any)["calls"])
	terminal := decodeDeferPayload(t, writer.payloads[1])["extensions"].(map[string]any)
	require.Equal(t, float64(2), terminal["authorization"].(map[string]any)["render"])
	require.Equal(t, float64(1), terminal["rateLimit"].(map[string]any)["calls"])
}

func TestDeferExtensions_ParallelTerminalRenderErrorPropagates(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{MaxConcurrency: 32})
	response := parallelDeferExtensionResponse(
		`{"data":{"fast":"ready"}}`,
		`{"data":{"f1":"one"}}`,
		`{"data":{"f2":"two"}}`,
	)
	ctx := NewContext(context.Background())
	ctx.SetAuthorizer(&terminalFailingDeferAuthorizer{})
	writer := &testDeferWriter{}

	_, err := resolver.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.EqualError(t, err, "terminal extension render failed")
	require.True(t, writer.complete)
}

func TestDeferExtensions_HardParentErrorMarksDescendantsSkipped(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{MaxConcurrency: 32})
	parent := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(2, "parent", `{"data":{"f1":"unused"}}`)}
	child := &DeferFetchGroup{DeferID: 2, Fetches: deferExtensionFetch(3, "child", `{"data":{"f2":"must-not-run"}}`)}
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1},
			2: {ID: 2, ParentID: 1},
		},
		DeferTree: DeferSequence(DeferSingle(parent), DeferSingle(child)),
		Response: &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Data: &Object{Nullable: true, Fields: []*Field{
				deferredField("f1", 1, &String{Path: []string{"f1"}, Nullable: true}, nil),
				deferredField("f2", 2, &String{Path: []string{"f2"}, Nullable: true}, nil),
			}},
		},
	}

	ctx := deferExtensionContext()
	ctx.RateLimitOptions = RateLimitOptions{Enable: true}
	ctx.SetRateLimiter(&deferTestRateLimiter{errOnDataSourceID: "parent"})
	writer := &testDeferWriter{}
	_, err := resolver.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 2)
	terminal := decodeDeferPayload(t, writer.payloads[1])
	require.Equal(t, false, terminal["hasNext"])
	extensions := terminal["extensions"].(map[string]any)
	require.Equal(t, map[int]string{
		1: string(DeferExecutionStatusError),
		2: string(DeferExecutionStatusSkipped),
	}, deferStatuses(t, extensions["trace"]))
	require.Contains(t, writer.payloads[1], "rate limiter hard error on parent")
}

func TestDeferExtensions_DiscardedParentRenderSkipsNestedChildren(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{MaxConcurrency: 32})
	parent := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(2, "parent", `{"data":{"f1":"parent","f2":"child"}}`)}
	child := &DeferFetchGroup{DeferID: 2, Fetches: deferExtensionFetch(3, "child", `{"data":{"f2":"must-not-run"}}`)}
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1},
			2: {ID: 2, ParentID: 1},
		},
		DeferTree: DeferSequence(DeferSingle(parent), DeferSingle(child)),
		Response: &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Data: &Object{Nullable: true, Fields: []*Field{
				deferredField("f1", 1, &String{Path: []string{"f1"}, Nullable: true}, &FieldInfo{Name: "f1"}),
				deferredField("f2", 2, &String{Path: []string{"f2"}, Nullable: true}, nil),
			}},
		},
	}

	ctx := deferExtensionContext()
	ctx.SetFieldValueRenderer(&deferTestFieldRenderer{errOnField: "f1"})
	writer := &testDeferWriter{}
	_, err := resolver.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 2, "a child whose parent payload was discarded must not execute")
	terminal := decodeDeferPayload(t, writer.payloads[1])
	require.Equal(t, false, terminal["hasNext"])
	require.NotContains(t, terminal, "pending")
	require.Equal(t, map[int]string{
		1: string(DeferExecutionStatusError),
		2: string(DeferExecutionStatusSkipped),
	}, deferStatuses(t, terminal["extensions"].(map[string]any)["trace"]))
}

func TestDeferExtensions_FinalParallelErrorHasAuthoritativeTrace(t *testing.T) {
	resolver := New(t.Context(), ResolverOptions{
		MaxConcurrency:               32,
		PropagateSubgraphErrors:      true,
		SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
		AllowAllErrorExtensionFields: true,
	})
	response := parallelDeferExtensionResponse(`{"data":{"fast":"ready"}}`, `{}`, `{}`)
	successSource := newBlockingDataSource([]byte(`{"data":{"f1":"one"}}`))
	errorSource := newBlockingDataSource([]byte(`{"data":{"f2":"two"},"errors":[{"message":"deferred failure","extensions":{"code":"DEFER_FAIL"}}]}`))
	response.DeferTree.ChildNodes[0].Item.Fetches.Item.Fetch.(*SingleFetch).DataSource = successSource
	response.DeferTree.ChildNodes[1].Item.Fetches.Item.Fetch.(*SingleFetch).DataSource = errorSource

	writer := newSignalingDeferWriter()
	done := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveGraphQLDeferResponse(deferExtensionContext(), response, writer)
		done <- err
	}()

	<-writer.flushed
	<-successSource.Ready()
	<-errorSource.Ready()
	successSource.Release()
	<-writer.flushed
	errorSource.Release()
	require.NoError(t, <-done)
	require.Len(t, writer.payloads, 3)

	terminal := decodeDeferPayload(t, writer.payloads[2])
	require.Equal(t, false, terminal["hasNext"])
	statuses := deferStatuses(t, terminal["extensions"].(map[string]any)["trace"])
	require.Equal(t, string(DeferExecutionStatusCompleted), statuses[1])
	require.Equal(t, string(DeferExecutionStatusError), statuses[2])
	require.Contains(t, writer.payloads[2], `"code":"DEFER_FAIL"`)
}

func TestDeferExtensions_ValueCompletionSuppressionIsRequestWide(t *testing.T) {
	resolver := New(t.Context(), ResolverOptions{
		MaxConcurrency: 32,
		ResolvableOptions: ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		},
	})
	response := parallelDeferExtensionResponse(`{"data":{"fast":"ready"}}`, `{}`, `{}`)
	// The first group to finish returns only errors and activates suppression.
	// The final group would otherwise add a valueCompletion entry for f1:String!.
	errorsOnly := newBlockingDataSource([]byte(`{"data":null,"errors":[{"message":"upstream failed"}]}`))
	missingRequired := newBlockingDataSource([]byte(`{"data":{}}`))
	response.DeferTree.ChildNodes[0].Item.Fetches.Item.Fetch.(*SingleFetch).DataSource = missingRequired
	response.DeferTree.ChildNodes[1].Item.Fetches.Item.Fetch.(*SingleFetch).DataSource = errorsOnly
	response.Response.Data.Fields[1].Value = &String{Path: []string{"f1"}, Nullable: false}

	writer := newSignalingDeferWriter()
	done := make(chan error, 1)
	go func() {
		_, err := resolver.ResolveGraphQLDeferResponse(deferExtensionContext(), response, writer)
		done <- err
	}()

	<-writer.flushed
	<-missingRequired.Ready()
	<-errorsOnly.Ready()
	errorsOnly.Release()
	<-writer.flushed
	missingRequired.Release()
	require.NoError(t, <-done)
	require.Len(t, writer.payloads, 3)

	terminal := decodeDeferPayload(t, writer.payloads[2])
	extensions := terminal["extensions"].(map[string]any)
	require.NotContains(t, extensions, "valueCompletion")
}

func TestDeferExtensions_DeferredValueCompletionIsTerminal(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{
		MaxConcurrency: 32,
		ResolvableOptions: ResolvableOptions{
			ApolloCompatibilityValueCompletionInExtensions: true,
		},
	})
	group := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(1, "deferred", `{"data":{}}`)}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: false}, nil, group)

	writer := &testDeferWriter{}
	_, err := resolver.ResolveGraphQLDeferResponse(deferExtensionContext(), response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 2)
	terminal := decodeDeferPayload(t, writer.payloads[1])
	extensions := terminal["extensions"].(map[string]any)
	valueCompletion, ok := extensions["valueCompletion"].([]any)
	require.True(t, ok)
	require.Len(t, valueCompletion, 1)
	require.Equal(t, []any{"f1"}, valueCompletion[0].(map[string]any)["path"])
}

func TestDeferExtensions_ClientRequestExtensionsReachDeferredFetches(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{MaxConcurrency: 32})
	expectedInput := []byte(`{"body":{"extensions":{"client":"value"}}}`)
	primary := deferExtensionFetch(1, "primary", `{}`)
	primary.Item.Fetch.(*SingleFetch).DataSource = fakeDataSourceWithInputCheck(t, expectedInput, []byte(`{"data":{}}`))
	deferred := deferExtensionFetch(2, "deferred", `{}`)
	deferred.Item.Fetch.(*SingleFetch).DataSource = fakeDataSourceWithInputCheck(t, expectedInput, []byte(`{"data":{"f1":"done"}}`))
	group := &DeferFetchGroup{DeferID: 1, Fetches: deferred}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, nil, group)
	response.Response.Fetches = primary

	ctx := NewContext(context.Background())
	ctx.Extensions = []byte(`{"client":"value"}`)
	writer := &testDeferWriter{}
	_, err := resolver.ResolveGraphQLDeferResponse(ctx, response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 2)
}

func TestDeferExtensions_AccumulatorIsIsolatedBetweenRequests(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{
		MaxConcurrency:                 32,
		AllowCustomExtensionProperties: true,
	})
	source := FakeDataSource(`{"data":{"f1":"first"},"extensions":{"request":"first"}}`)
	deferred := deferExtensionFetch(1, "deferred", `{}`)
	deferred.Item.Fetch.(*SingleFetch).DataSource = source
	group := &DeferFetchGroup{DeferID: 1, Fetches: deferred}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, nil, group)

	resolve := func(want string) {
		writer := &testDeferWriter{}
		_, err := resolver.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, writer)
		require.NoError(t, err)
		require.Len(t, writer.payloads, 2)
		terminal := decodeDeferPayload(t, writer.payloads[1])
		require.Equal(t, want, terminal["extensions"].(map[string]any)["request"])
	}

	resolve("first")
	source.data = []byte(`{"data":{"f1":"second"},"extensions":{"request":"second"}}`)
	resolve("second")
}

func TestDeferExtensions_AllPrunedInitialFrameIsAuthoritative(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{
		MaxConcurrency:                 32,
		AllowCustomExtensionProperties: true,
	})
	parent := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(2, "deferred-parent", `{"data":{"p":"should-not-run"},"extensions":{"deferred":true}}`)}
	child := &DeferFetchGroup{DeferID: 2, Fetches: deferExtensionFetch(3, "deferred-child", `{"data":{"c":"should-not-run"}}`)}
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: []string{"user"}},
			2: {ID: 2, ParentID: 1, Path: []string{"user", "p"}},
		},
		DeferTree: DeferSequence(DeferSingle(parent), DeferSingle(child)),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: deferExtensionFetch(1, "primary", `{"data":{"user":null},"extensions":{"initialOnly":true}}`),
			Data: &Object{Nullable: true, Fields: []*Field{
				{Name: []byte("user"), Value: &Object{Path: []string{"user"}, Nullable: true, Fields: []*Field{
					deferredField("p", 1, &Object{Path: []string{"p"}, Nullable: true, Fields: []*Field{
						deferredField("c", 2, &String{Path: []string{"c"}, Nullable: true}, nil),
					}}, nil),
				}}},
			}},
		},
	}

	writer := &testDeferWriter{}
	_, err := resolver.ResolveGraphQLDeferResponse(deferExtensionContext(), response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 1, "pruned groups must not execute or flush patches")
	frame := decodeDeferPayload(t, writer.payloads[0])
	require.Equal(t, false, frame["hasNext"])
	extensions := frame["extensions"].(map[string]any)
	require.Equal(t, true, extensions["initialOnly"])
	require.NotContains(t, extensions, "deferred")
	require.Equal(t, map[int]string{
		1: string(DeferExecutionStatusSkipped),
		2: string(DeferExecutionStatusSkipped),
	}, deferStatuses(t, extensions["trace"]))
	planBytes, err := json.Marshal(extensions["queryPlan"])
	require.NoError(t, err)
	assert.Contains(t, string(planBytes), fmt.Sprintf(`"defer":{"id":%d`, 1))
	assert.Contains(t, string(planBytes), fmt.Sprintf(`"defer":{"id":%d`, 2))
}

func TestDeferExtensions_DisabledPropagationDoesNotAddEmptyExtensions(t *testing.T) {
	t.Parallel()
	resolver := New(t.Context(), ResolverOptions{MaxConcurrency: 32})
	group := &DeferFetchGroup{DeferID: 1, Fetches: deferExtensionFetch(2, "deferred", `{"data":{"f1":"done"},"extensions":{"ignored":true}}`)}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, nil, group)
	response.Response.Fetches = deferExtensionFetch(1, "primary", `{"data":{},"extensions":{"ignored":true}}`)

	writer := &testDeferWriter{}
	_, err := resolver.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, writer)
	require.NoError(t, err)
	require.Len(t, writer.payloads, 2)
	for _, payload := range writer.payloads {
		require.NotContains(t, decodeDeferPayload(t, payload), "extensions")
	}
}
