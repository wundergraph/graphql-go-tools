package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

func multiStaticTemplate(s string) InputTemplate {
	return InputTemplate{
		Segments: []TemplateSegment{
			{SegmentType: StaticSegmentType, Data: []byte(s)},
		},
	}
}

func multiRepresentationsTemplate() InputTemplate {
	return InputTemplate{
		SetTemplateOutputToNullOnVariableNull: true,
		Segments: []TemplateSegment{
			{
				SegmentType:  VariableSegmentType,
				VariableKind: ResolvableObjectVariableKind,
				Renderer: NewGraphQLVariableResolveRenderer(&Object{
					Nullable: true,
					Fields: []*Field{
						{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
						{Name: []byte("id"), Value: &Integer{Path: []string{"id"}}},
					},
				}),
			},
		},
	}
}

func multiContextVariableTemplate(path string) InputTemplate {
	return InputTemplate{
		Segments: []TemplateSegment{
			(&ContextVariable{Path: []string{path}, Renderer: NewJSONVariableRenderer()}).TemplateSegment(),
		},
	}
}

func newMultiEntityLoader(seed string) (*Loader, *Context) {
	ctx := NewContext(context.Background())
	loader := &Loader{dataBuffer: &DataBuffer{data: astjson.MustParse(seed)}}
	loader.ctx = ctx
	return loader, ctx
}

// twoEntryMultiFetch builds an f1 batch entry over employees and an f2 single
// entry over employee. entry2Vars are attached to the f2 entry.
func twoEntryMultiFetch(entry2Vars []MultiEntityFetchVariable) *MultiEntityFetch {
	info := &FetchInfo{OperationType: ast.OperationTypeQuery, DataSourceID: "products-id"}
	entry1 := MultiEntityFetchEntry{
		Alias:                 "f1",
		Item:                  &FetchItem{FetchPath: []FetchItemPathElement{ArrayPath("employees")}, ResponsePath: "employees"},
		Info:                  info,
		PostProcessing:        PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "f1"}, SelectResponseErrorsPath: []string{"errors"}},
		OriginKind:            EntityFetchOriginBatch,
		RepresentationsPrefix: []byte(`"representations_f1":[`),
		Representations:       multiRepresentationsTemplate(),
		IncludePrefix:         []byte(`],"includeF1":`),
		SkipNullItems:         true,
		SkipEmptyObjectItems:  true,
		SkipErrItems:          true,
	}
	entry2 := MultiEntityFetchEntry{
		Alias:                 "f2",
		Item:                  &FetchItem{FetchPath: []FetchItemPathElement{ObjectPath("employee")}, ResponsePath: "employee"},
		Info:                  info,
		PostProcessing:        PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "f2"}, SelectResponseErrorsPath: []string{"errors"}},
		OriginKind:            EntityFetchOriginSingle,
		RepresentationsPrefix: []byte(`,"representations_f2":[`),
		Representations:       multiRepresentationsTemplate(),
		IncludePrefix:         []byte(`],"includeF2":`),
		Variables:             entry2Vars,
		SkipNullItems:         true,
		SkipEmptyObjectItems:  true,
		SkipErrItems:          true,
	}
	return &MultiEntityFetch{
		Input: MultiEntityInput{
			Header:  multiStaticTemplate(`{"method":"POST","url":"http://x","body":{"query":"Q","variables":{`),
			Entries: []MultiEntityFetchEntry{entry1, entry2},
			Footer:  multiStaticTemplate(`}}}`),
		},
		Info: info,
	}
}

func TestPrepareMultiEntityFetch_Assembly(t *testing.T) {
	loader, ctx := newMultiEntityLoader(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
	ctx.Variables = astjson.MustParse(`{"first":10}`)

	multi := twoEntryMultiFetch([]MultiEntityFetchVariable{
		{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")},
	})

	prepared, err := loader.preparePhase(&FetchItem{Fetch: multi})
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.False(t, prepared.skipLoad)

	expected := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2}],"includeF1":true,"representations_f2":[{"__typename":"Employee","id":9}],"includeF2":true,"first_f2":10}}}`
	assert.Equal(t, expected, string(prepared.input))

	require.Len(t, prepared.multiEntries, 2)
	stats := prepared.multiEntries[0].res.batchStats
	require.Len(t, stats, 2)
	assert.Len(t, stats[0], 2)
	assert.Len(t, stats[1], 1)
	assert.Len(t, prepared.multiEntries[1].res.batchStats, 1)
}

func TestPrepareMultiEntityFetch_EmptyEntry(t *testing.T) {
	loader, ctx := newMultiEntityLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":null}`)
	ctx.Variables = astjson.MustParse(`{"first":10}`)

	multi := twoEntryMultiFetch([]MultiEntityFetchVariable{
		{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")},
	})

	prepared, err := loader.preparePhase(&FetchItem{Fetch: multi})
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.False(t, prepared.skipLoad)

	assert.Contains(t, string(prepared.input), `"representations_f1":[{"__typename":"Employee","id":1}],"includeF1":true`)
	assert.Contains(t, string(prepared.input), `,"representations_f2":[],"includeF2":false`)
	// A skipped entry still renders its non-representations variables.
	assert.Contains(t, string(prepared.input), `,"first_f2":10`)
	assert.True(t, prepared.multiEntries[1].res.fetchSkipped)
}

func TestPrepareMultiEntityFetch_DeniedEntry(t *testing.T) {
	loader, ctx := newMultiEntityLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
	ctx.SetPreFetchFieldAuthorizer(&batchTestAuthorizer{})

	multi := twoEntryMultiFetch(nil)
	deniedField := GraphCoordinate{TypeName: "Employee", FieldName: "secret", HasAuthorizationRule: true}
	multi.Input.Entries[1].Info = &FetchInfo{
		OperationType: ast.OperationTypeQuery,
		DataSourceID:  "products-id",
		RootFields:    []GraphCoordinate{deniedField},
	}

	auth := NewFieldAuthorization(ctx)
	auth.seedDeny("products-id", deniedField, "missing scope")
	loader.authorization = auth

	prepared, err := loader.preparePhase(&FetchItem{Fetch: multi})
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.False(t, prepared.skipLoad)

	assert.Contains(t, string(prepared.input), `"representations_f1":[{"__typename":"Employee","id":1}],"includeF1":true`)
	assert.Contains(t, string(prepared.input), `,"representations_f2":[],"includeF2":false`)
	assert.True(t, prepared.multiEntries[1].res.fetchSkipped)
	assert.False(t, prepared.multiEntries[1].res.authorizationRejected)
}

func TestPrepareMultiEntityFetch_AllExcluded(t *testing.T) {
	loader, ctx := newMultiEntityLoader(`{"employees":[],"employee":null}`)
	ctx.Variables = astjson.MustParse(`{"first":10}`)

	multi := twoEntryMultiFetch(nil)

	prepared, err := loader.preparePhase(&FetchItem{Fetch: multi})
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.True(t, prepared.skipLoad)
	assert.True(t, prepared.res.fetchSkipped)
}

func TestPrepareMultiEntityFetch_UndefinedVariable(t *testing.T) {
	t.Run("undefined omits pair", func(t *testing.T) {
		loader, ctx := newMultiEntityLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
		ctx.Variables = astjson.MustParse(`{}`)

		multi := twoEntryMultiFetch([]MultiEntityFetchVariable{
			{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")},
		})

		prepared, err := loader.preparePhase(&FetchItem{Fetch: multi})
		require.NoError(t, err)
		require.NotNil(t, prepared)
		assert.NotContains(t, string(prepared.input), `first_f2`)
	})

	t.Run("explicit null kept", func(t *testing.T) {
		loader, ctx := newMultiEntityLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
		ctx.Variables = astjson.MustParse(`{"first":null}`)

		multi := twoEntryMultiFetch([]MultiEntityFetchVariable{
			{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")},
		})

		prepared, err := loader.preparePhase(&FetchItem{Fetch: multi})
		require.NoError(t, err)
		require.NotNil(t, prepared)
		assert.Contains(t, string(prepared.input), `,"first_f2":null`)
	})
}

func TestPrepareMultiEntityFetch_DedupStateIsolation(t *testing.T) {
	// employee renders the same representation as an employees element; the
	// per-entry dedup scope must not drop it from f2.
	loader, ctx := newMultiEntityLoader(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2}],"employee":{"__typename":"Employee","id":1}}`)
	ctx.Variables = astjson.MustParse(`{"first":10}`)

	multi := twoEntryMultiFetch(nil)

	prepared, err := loader.preparePhase(&FetchItem{Fetch: multi})
	require.NoError(t, err)
	require.NotNil(t, prepared)

	assert.Contains(t, string(prepared.input), `,"representations_f2":[{"__typename":"Employee","id":1}]`)
	assert.Len(t, prepared.multiEntries[1].res.batchStats, 1)
}

// recordingDataSource returns a canned body or error, counts Load calls, and
// captures the last received input.
type recordingDataSource struct {
	response  []byte
	err       error
	calls     int
	lastInput []byte
}

func (r *recordingDataSource) Load(_ context.Context, _ http.Header, input []byte) ([]byte, error) {
	r.calls++
	r.lastInput = append(r.lastInput[:0], input...)
	return r.response, r.err
}

func (r *recordingDataSource) LoadWithFiles(_ context.Context, _ http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	r.calls++
	r.lastInput = append(r.lastInput[:0], input...)
	return r.response, r.err
}

func newMultiMergeLoader(seed string) (*Loader, *Context) {
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	loader := &Loader{dataBuffer: &DataBuffer{data: astjson.MustParse(seed)}}
	loader.ctx = ctx
	loader.taintedObjs = make(taintedObjects)
	return loader, ctx
}

func mergeErrorsJSON(l *Loader) string {
	if l.errors == nil {
		return ""
	}
	return string(l.errors.MarshalTo(nil))
}

func TestMergeMultiEntityResult_FanOut(t *testing.T) {
	loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)

	multi := twoEntryMultiFetch(nil)
	multi.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[{"upc":"1"}]},{"products":[{"upc":"2"}]}],"f2":[{"notes":"n"}]}}`)}

	require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

	expected := `{"employees":[{"__typename":"Employee","id":1,"products":[{"upc":"1"}]},{"__typename":"Employee","id":2,"products":[{"upc":"2"}]},{"__typename":"Employee","id":1,"products":[{"upc":"1"}]}],"employee":{"__typename":"Employee","id":9,"notes":"n"}}`
	assert.JSONEq(t, expected, string(loader.dataBuffer.Get().MarshalTo(nil)))
	assert.Empty(t, mergeErrorsJSON(loader))
}

func TestMergeMultiEntityResult_ErrorPartitioning(t *testing.T) {
	body := `{"data":{"f1":[{"products":[{"upc":"1"}]},{"products":[{"upc":"2"}]}],"f2":[{"notes":"n"}]},"errors":[{"message":"a","path":["f1",0,"products"]},{"message":"b","path":["f2"]},{"message":"c"}]}`

	t.Run("wrap mode rewrites paths and hides aliases", func(t *testing.T) {
		loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
		loader.propagateSubgraphErrors = true
		loader.rewriteSubgraphErrorPaths = true
		multi := twoEntryMultiFetch(nil)
		multi.DataSource = &recordingDataSource{response: []byte(body)}

		require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

		errStr := mergeErrorsJSON(loader)
		assert.Contains(t, errStr, `"message":"a"`)
		assert.Contains(t, errStr, `"path":["products"]`)
		assert.Contains(t, errStr, `"message":"c"`)
		assert.NotContains(t, errStr, `"f1"`)
		assert.NotContains(t, errStr, `"f2"`)
	})

	t.Run("pass-through never leaks aliases", func(t *testing.T) {
		loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
		loader.subgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
		loader.rewriteSubgraphErrorPaths = false
		loader.allowedSubgraphErrorFields = map[string]struct{}{"message": {}, "path": {}}
		multi := twoEntryMultiFetch(nil)
		multi.DataSource = &recordingDataSource{response: []byte(body)}

		require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

		errStr := mergeErrorsJSON(loader)
		assert.Contains(t, errStr, `_entities`)
		assert.NotContains(t, errStr, `"f1"`)
		assert.NotContains(t, errStr, `"f2"`)
	})
}

func TestMergeMultiEntityResult_EmptyArraySingleOrigin(t *testing.T) {
	t.Run("single origin empty array is benign", func(t *testing.T) {
		loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
		multi := twoEntryMultiFetch(nil)
		multi.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[{"upc":"1"}]}],"f2":[]}}`)}

		require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

		assert.Empty(t, mergeErrorsJSON(loader))
		out := string(loader.dataBuffer.Get().MarshalTo(nil))
		assert.Contains(t, out, `"products":[{"upc":"1"}]`)
		assert.NotContains(t, out, `notes`)
	})

	t.Run("batch origin empty array errors like unmerged", func(t *testing.T) {
		loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
		multi := twoEntryMultiFetch(nil)
		multi.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[],"f2":[{"notes":"n"}]}}`)}

		require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

		// An empty _entities array yields GetArray()==nil, so mergeResult renders the
		// same invalidGraphQLResponseShape error an unmerged BatchEntityFetch would.
		assert.Contains(t, mergeErrorsJSON(loader), "no data or errors in response")
		assert.Contains(t, string(loader.dataBuffer.Get().MarshalTo(nil)), `"notes":"n"`)
	})
}

func TestMergeMultiEntityResult_TransportError(t *testing.T) {
	t.Run("error at every non-excluded entry path", func(t *testing.T) {
		loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
		multi := twoEntryMultiFetch(nil)
		multi.FetchID = 5
		multi.DataSource = &recordingDataSource{err: errors.New("boom")}

		require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

		errStr := mergeErrorsJSON(loader)
		assert.Contains(t, errStr, "at Path 'employees'")
		assert.Contains(t, errStr, "at Path 'employee'")
		assert.Contains(t, loader.erroredFetchIDs, 5)
	})

	t.Run("excluded entry gets no error", func(t *testing.T) {
		loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":null}`)
		multi := twoEntryMultiFetch(nil)
		multi.FetchID = 5
		multi.DataSource = &recordingDataSource{err: errors.New("boom")}

		require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

		errStr := mergeErrorsJSON(loader)
		assert.Contains(t, errStr, "at Path 'employees'")
		assert.NotContains(t, errStr, "at Path 'employee'.")
	})
}

func TestMergeMultiEntityResult_InvalidResponse(t *testing.T) {
	loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
	multi := twoEntryMultiFetch(nil)
	multi.FetchID = 5
	multi.DataSource = &recordingDataSource{response: []byte(`not json`)}

	require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

	assert.Contains(t, mergeErrorsJSON(loader), "invalid JSON")
	assert.NotContains(t, loader.erroredFetchIDs, 5)
}

func TestMergeMultiEntityResult_RateLimitRejected(t *testing.T) {
	loader, ctx := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
	ctx.RateLimitOptions = RateLimitOptions{Enable: true}
	ctx.rateLimiter = &testRateLimiter{allowFn: func(*Context, *FetchInfo, json.RawMessage) (*RateLimitDeny, error) {
		return &RateLimitDeny{Reason: "over limit"}, nil
	}}
	ds := &recordingDataSource{response: []byte(`{"data":{}}`)}
	multi := twoEntryMultiFetch(nil)
	multi.DataSource = ds

	require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

	assert.Equal(t, 0, ds.calls, "no subgraph request when rate limited")
	errStr := mergeErrorsJSON(loader)
	assert.Contains(t, errStr, "at Path 'employees'")
	assert.Contains(t, errStr, "at Path 'employee'")
	assert.Contains(t, errStr, "Rate limit exceeded")
}

func TestMergeMultiEntityResult_TaintPerEntry(t *testing.T) {
	loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
	loader.validateRequiredExternalFields = true
	multi := twoEntryMultiFetch(nil)
	multi.Input.Entries[0].Info = &FetchInfo{
		OperationType: ast.OperationTypeQuery,
		DataSourceID:  "products-id",
		FetchReasons:  []FetchReason{{TypeName: "Employee", FieldName: "x", IsRequires: true, Nullable: true}},
	}
	multi.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"__typename":"Employee","x":null}],"f2":[{"notes":"n"}]},"errors":[{"message":"e","path":["f1",0,"x"]}]}`)}

	require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

	assert.Len(t, loader.taintedObjs, 1)
}

func TestMergeMultiEntityResult_ExtensionsOnce(t *testing.T) {
	loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
	loader.allowCustomExtensionProperties = true
	multi := twoEntryMultiFetch(nil)
	multi.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[]}],"f2":[{"notes":"n"}]},"extensions":{"foo":"bar"}}`)}

	require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

	assert.Len(t, loader.subgraphExtensions, 1)
}

func TestMergeMultiEntityResult_HooksOnce(t *testing.T) {
	loader, ctx := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`)
	hooks := NewTestLoaderHooks()
	ctx.LoaderHooks = hooks
	multi := twoEntryMultiFetch(nil)
	multi.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[]}],"f2":[{"notes":"n"}]}}`)}

	require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

	assert.Equal(t, int64(1), hooks.preFetchCalls.Load())
	assert.Equal(t, int64(1), hooks.postFetchCalls.Load())
}

func TestMergeMultiEntityResult_ExcludedEntry(t *testing.T) {
	loader, _ := newMultiMergeLoader(`{"employees":[{"__typename":"Employee","id":1}],"employee":null}`)
	multi := twoEntryMultiFetch(nil)
	multi.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[{"upc":"1"}]}]}}`)}

	require.NoError(t, loader.resolveSingle(context.Background(), &FetchItem{Fetch: multi}))

	assert.Empty(t, mergeErrorsJSON(loader))
	out := string(loader.dataBuffer.Get().MarshalTo(nil))
	assert.Contains(t, out, `"products":[{"upc":"1"}]`)
	assert.Contains(t, out, `"employee":null`)
}

// multiEntityRootFetch seeds the shared parent data (three employees, one
// employee) that both merged and unmerged integration trees fetch on.
func multiEntityRootFetch(ds DataSource) *SingleFetch {
	return &SingleFetch{
		FetchDependencies: FetchDependencies{FetchID: 0},
		InputTemplate:     multiStaticTemplate(`{"method":"POST","url":"http://root","body":{"query":"{employees{__typename id} employee{__typename id}}"}}`),
		FetchConfiguration: FetchConfiguration{
			DataSource:     ds,
			PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}},
		},
	}
}

const multiEntityRootResponse = `{"data":{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}}`

// TestLoadGraphQLResponseData_MultiEntity drives prepare+load+merge for a full
// tree and asserts the merged run issues one subgraph request with the expected
// assembled input and produces a data buffer byte-identical to the unmerged run.
func TestLoadGraphQLResponseData_MultiEntity(t *testing.T) {
	entry2Vars := func() []MultiEntityFetchVariable {
		return []MultiEntityFetchVariable{
			{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")},
		}
	}

	runMerged := func(t *testing.T) (out string, multiDS *recordingDataSource) {
		t.Helper()
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.Variables = astjson.MustParse(`{"first":10}`)

		multi := twoEntryMultiFetch(entry2Vars())
		multi.FetchID = 1
		multi.DependsOnFetchIDs = []int{0}
		multiDS = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":["a"]},{"products":["b"]}],"f2":[{"notes":"n"}]}}`)}
		multi.DataSource = multiDS

		tree := Sequence(
			Single(multiEntityRootFetch(&recordingDataSource{response: []byte(multiEntityRootResponse)})),
			Single(multi),
		)
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: tree}))
		require.Empty(t, mergeErrorsJSON(loader))
		return string(loader.dataBuffer.Get().MarshalTo(nil)), multiDS
	}

	runUnmerged := func(t *testing.T) string {
		t.Helper()
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.Variables = astjson.MustParse(`{"first":10}`)

		batch := &BatchEntityFetch{
			FetchDependencies: FetchDependencies{FetchID: 1, DependsOnFetchIDs: []int{0}},
			Input: BatchInput{
				Header:               multiStaticTemplate(`{"method":"POST","url":"http://products","body":{"query":"products","variables":{"representations":[`),
				Items:                []InputTemplate{multiRepresentationsTemplate()},
				Separator:            multiStaticTemplate(`,`),
				Footer:               multiStaticTemplate(`]}}}`),
				SkipNullItems:        true,
				SkipEmptyObjectItems: true,
				SkipErrItems:         true,
			},
			DataSource:     &recordingDataSource{response: []byte(`{"data":{"_entities":[{"products":["a"]},{"products":["b"]}]}}`)},
			PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities"}},
			Info:           &FetchInfo{OperationType: ast.OperationTypeQuery, DataSourceID: "products-id"},
		}
		entity := &EntityFetch{
			FetchDependencies: FetchDependencies{FetchID: 2, DependsOnFetchIDs: []int{0}},
			Input: EntityInput{
				Header: multiStaticTemplate(`{"method":"POST","url":"http://products","body":{"query":"notes","variables":{"representations":[`),
				Item:   multiRepresentationsTemplate(),
				Footer: multiStaticTemplate(`]}}}`),
			},
			DataSource:     &recordingDataSource{response: []byte(`{"data":{"_entities":[{"notes":"n"}]}}`)},
			PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
			Info:           &FetchInfo{OperationType: ast.OperationTypeQuery, DataSourceID: "products-id"},
		}
		tree := Sequence(
			Single(multiEntityRootFetch(&recordingDataSource{response: []byte(multiEntityRootResponse)})),
			SingleWithPath(batch, "employees", ArrayPath("employees")),
			SingleWithPath(entity, "employee", ObjectPath("employee")),
		)
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: tree}))
		require.Empty(t, mergeErrorsJSON(loader))
		return string(loader.dataBuffer.Get().MarshalTo(nil))
	}

	merged, multiDS := runMerged(t)
	unmerged := runUnmerged(t)

	assert.Equal(t, 1, multiDS.calls, "exactly one subgraph request for the merged fetch")

	expectedInput := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2}],"includeF1":true,"representations_f2":[{"__typename":"Employee","id":9}],"includeF2":true,"first_f2":10}}}`
	assert.Equal(t, expectedInput, string(multiDS.lastInput), "merged input matches the assembly golden")

	assert.Equal(t, unmerged, merged, "merged data buffer is byte-identical to the unmerged run")
}

// TestLoadGraphQLResponseData_MultiEntity_SingleFlight verifies the merged fetch
// is single-flight compatible: it is query-typed, and two identical multi loads
// collapse into one leader request with a shared follower.
func TestLoadGraphQLResponseData_MultiEntity_SingleFlight(t *testing.T) {
	ctx := NewContext(context.Background())
	loader := &Loader{}
	loader.ctx = ctx
	multi := twoEntryMultiFetch(nil)
	item := &FetchItem{Fetch: multi}

	assert.True(t, loader.singleFlightAllowed(item),
		"merged fetch is query-typed, so subgraph request de-duplication applies")

	sf := NewSingleFlight(1)
	input := []byte(`{"method":"POST","url":"http://x","body":{"query":"Q","variables":{}}}`)
	_, sharedLeader := sf.GetOrCreateItem(item, input, 0)
	assert.False(t, sharedLeader, "first caller leads the request")
	_, sharedFollower := sf.GetOrCreateItem(item, input, 0)
	assert.True(t, sharedFollower, "second identical caller shares the leader's request")
}
