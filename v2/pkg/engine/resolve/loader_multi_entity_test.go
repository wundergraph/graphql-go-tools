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

// ---------------------------------------------------------------------------
// Input-template builders. These stay small and named so the fetch shape below
// reads declaratively; each returns one InputTemplate segment kind.
// ---------------------------------------------------------------------------

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

// multiFetch bundles the pieces a MultiEntityFetch test drives, with named
// fields so a test reads: build the fixture, tweak the fetch, run, assert.
type multiFetch struct {
	loader *Loader
	ctx    *Context
	fetch  *MultiEntityFetch
	item   *FetchItem
}

type multiFetchConfig struct {
	seed       string
	variables  string
	entry2Vars []MultiEntityFetchVariable
}

type multiFetchOption func(*multiFetchConfig)

// withSeed overrides the parent data the fetch reads representations from.
func withSeed(seed string) multiFetchOption {
	return func(c *multiFetchConfig) { c.seed = seed }
}

// withVariables sets ctx.Variables from the given JSON object.
func withVariables(variables string) multiFetchOption {
	return func(c *multiFetchConfig) { c.variables = variables }
}

// withEntry2Vars attaches the given variables to the f2 entry.
func withEntry2Vars(vars ...MultiEntityFetchVariable) multiFetchOption {
	return func(c *multiFetchConfig) { c.entry2Vars = vars }
}

// multiFetchFixture builds the MultiEntityFetch, its loader, and the parent
// data in one call. Defaults to a single-employee parent; override with
// withSeed / withVariables / withEntry2Vars.
func multiFetchFixture(t *testing.T, opts ...multiFetchOption) multiFetch {
	t.Helper()
	cfg := multiFetchConfig{
		seed: `{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	if cfg.variables != "" {
		ctx.Variables = astjson.MustParse(cfg.variables)
	}

	loader := &Loader{dataBuffer: &DataBuffer{data: astjson.MustParse(cfg.seed)}}
	loader.ctx = ctx
	loader.taintedObjs = make(taintedObjects)

	fetch := twoEntryMultiFetch(cfg.entry2Vars)
	return multiFetch{
		loader: loader,
		ctx:    ctx,
		fetch:  fetch,
		item:   &FetchItem{Fetch: fetch},
	}
}

// assertMergedErrors marshals the loader's accumulated errors and compares the
// whole document. An empty expectedJSON asserts no errors were accumulated.
func assertMergedErrors(t *testing.T, loader *Loader, expectedJSON string) {
	t.Helper()
	var got string
	if loader.errors != nil {
		got = string(loader.errors.MarshalTo(nil))
	}
	if expectedJSON == "" {
		assert.Empty(t, got, "expected no accumulated errors")
		return
	}
	assert.JSONEq(t, expectedJSON, got)
}

// ---------------------------------------------------------------------------
// Prepare phase
// ---------------------------------------------------------------------------

func TestPrepareMultiEntityFetch_Assembly(t *testing.T) {
	f := multiFetchFixture(t,
		withSeed(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`),
		withVariables(`{"first":10}`),
		withEntry2Vars(MultiEntityFetchVariable{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")}),
	)

	prepared, err := f.loader.preparePhase(f.item)
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
	f := multiFetchFixture(t,
		withSeed(`{"employees":[{"__typename":"Employee","id":1}],"employee":null}`),
		withVariables(`{"first":10}`),
		withEntry2Vars(MultiEntityFetchVariable{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")}),
	)

	prepared, err := f.loader.preparePhase(f.item)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.False(t, prepared.skipLoad)

	// The excluded f2 entry renders empty representations + includeF2:false, but
	// still emits its non-representations variable (first_f2).
	expected := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[{"__typename":"Employee","id":1}],"includeF1":true,"representations_f2":[],"includeF2":false,"first_f2":10}}}`
	assert.Equal(t, expected, string(prepared.input))
	assert.True(t, prepared.multiEntries[1].res.fetchSkipped)
}

func TestPrepareMultiEntityFetch_DeniedEntry(t *testing.T) {
	f := multiFetchFixture(t)
	f.ctx.SetPreFetchFieldAuthorizer(&batchTestAuthorizer{})

	deniedField := GraphCoordinate{TypeName: "Employee", FieldName: "secret", HasAuthorizationRule: true}
	f.fetch.Input.Entries[1].Info = &FetchInfo{
		OperationType: ast.OperationTypeQuery,
		DataSourceID:  "products-id",
		RootFields:    []GraphCoordinate{deniedField},
	}

	auth := NewFieldAuthorization(f.ctx)
	auth.seedDeny("products-id", deniedField, "missing scope")
	f.loader.authorization = auth

	prepared, err := f.loader.preparePhase(f.item)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.False(t, prepared.skipLoad)

	// A denied entry is excluded (empty representations, includeF2:false) and its
	// representations are never rendered into the request.
	expected := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[{"__typename":"Employee","id":1}],"includeF1":true,"representations_f2":[],"includeF2":false}}}`
	assert.Equal(t, expected, string(prepared.input))
	assert.True(t, prepared.multiEntries[1].res.fetchSkipped)
	assert.False(t, prepared.multiEntries[1].res.authorizationRejected)
}

func TestPrepareMultiEntityFetch_AllExcluded(t *testing.T) {
	f := multiFetchFixture(t,
		withSeed(`{"employees":[],"employee":null}`),
		withVariables(`{"first":10}`),
	)

	prepared, err := f.loader.preparePhase(f.item)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.True(t, prepared.skipLoad)
	assert.True(t, prepared.res.fetchSkipped)
}

func TestPrepareMultiEntityFetch_UndefinedVariable(t *testing.T) {
	entry2Var := MultiEntityFetchVariable{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")}

	t.Run("undefined omits pair", func(t *testing.T) {
		f := multiFetchFixture(t, withVariables(`{}`), withEntry2Vars(entry2Var))

		prepared, err := f.loader.preparePhase(f.item)
		require.NoError(t, err)
		require.NotNil(t, prepared)

		// An undefined client variable drops the whole first_f2 pair.
		expected := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[{"__typename":"Employee","id":1}],"includeF1":true,"representations_f2":[{"__typename":"Employee","id":9}],"includeF2":true}}}`
		assert.Equal(t, expected, string(prepared.input))
	})

	t.Run("explicit null kept", func(t *testing.T) {
		f := multiFetchFixture(t, withVariables(`{"first":null}`), withEntry2Vars(entry2Var))

		prepared, err := f.loader.preparePhase(f.item)
		require.NoError(t, err)
		require.NotNil(t, prepared)

		// An explicit client null keeps first_f2:null in place.
		expected := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[{"__typename":"Employee","id":1}],"includeF1":true,"representations_f2":[{"__typename":"Employee","id":9}],"includeF2":true,"first_f2":null}}}`
		assert.Equal(t, expected, string(prepared.input))
	})
}

func TestPrepareMultiEntityFetch_DedupStateIsolation(t *testing.T) {
	// employee renders the same representation as an employees element; the
	// per-entry dedup scope must not drop it from f2.
	f := multiFetchFixture(t,
		withSeed(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2}],"employee":{"__typename":"Employee","id":1}}`),
		withVariables(`{"first":10}`),
	)

	prepared, err := f.loader.preparePhase(f.item)
	require.NoError(t, err)
	require.NotNil(t, prepared)

	expected := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2}],"includeF1":true,"representations_f2":[{"__typename":"Employee","id":1}],"includeF2":true}}}`
	assert.Equal(t, expected, string(prepared.input))
	assert.Len(t, prepared.multiEntries[1].res.batchStats, 1)
}

// ---------------------------------------------------------------------------
// Merge phase
// ---------------------------------------------------------------------------

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

func TestMergeMultiEntityResult_FanOut(t *testing.T) {
	f := multiFetchFixture(t, withSeed(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`))
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[{"upc":"1"}]},{"products":[{"upc":"2"}]}],"f2":[{"notes":"n"}]}}`)}

	require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

	expected := `{"employees":[{"__typename":"Employee","id":1,"products":[{"upc":"1"}]},{"__typename":"Employee","id":2,"products":[{"upc":"2"}]},{"__typename":"Employee","id":1,"products":[{"upc":"1"}]}],"employee":{"__typename":"Employee","id":9,"notes":"n"}}`
	assert.JSONEq(t, expected, string(f.loader.dataBuffer.Get().MarshalTo(nil)))
	assertMergedErrors(t, f.loader, "")
}

func TestMergeMultiEntityResult_ErrorPartitioning(t *testing.T) {
	body := `{"data":{"f1":[{"products":[{"upc":"1"}]},{"products":[{"upc":"2"}]}],"f2":[{"notes":"n"}]},"errors":[{"message":"a","path":["f1",0,"products"]},{"message":"b","path":["f2"]},{"message":"c"}]}`
	seed := `{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`

	t.Run("wrap mode rewrites paths and hides aliases", func(t *testing.T) {
		f := multiFetchFixture(t, withSeed(seed))
		f.loader.propagateSubgraphErrors = true
		f.loader.rewriteSubgraphErrorPaths = true
		f.fetch.DataSource = &recordingDataSource{response: []byte(body)}

		require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

		// "a" attributes to f1 (employees, index dropped), "b" to f2 (employee),
		// "c" (no path) to the merged fetch. No internal alias leaks.
		expected := `[` +
			`{"message":"Failed to fetch from Subgraph.","extensions":{"errors":[{"message":"c"}]}},` +
			`{"message":"Failed to fetch from Subgraph at Path 'employees'.","extensions":{"errors":[{"message":"a","path":["products"]}]}},` +
			`{"message":"Failed to fetch from Subgraph at Path 'employee'.","extensions":{"errors":[{"message":"b","path":[]}]}}` +
			`]`
		assertMergedErrors(t, f.loader, expected)
	})

	t.Run("pass-through never leaks aliases", func(t *testing.T) {
		f := multiFetchFixture(t, withSeed(seed))
		f.loader.subgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
		f.loader.rewriteSubgraphErrorPaths = false
		f.loader.allowedSubgraphErrorFields = map[string]struct{}{"message": {}, "path": {}}
		f.fetch.DataSource = &recordingDataSource{response: []byte(body)}

		require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

		// Pass-through keeps subgraph errors verbatim, but leading aliases are
		// rewritten to _entities so internal alias names never leak.
		expected := `[` +
			`{"message":"c"},` +
			`{"message":"a","path":["_entities",0,"products"]},` +
			`{"message":"b","path":["_entities"]}` +
			`]`
		assertMergedErrors(t, f.loader, expected)
	})
}

func TestMergeMultiEntityResult_EmptyArraySingleOrigin(t *testing.T) {
	t.Run("single origin empty array is benign", func(t *testing.T) {
		f := multiFetchFixture(t)
		f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[{"upc":"1"}]}],"f2":[]}}`)}

		require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

		assertMergedErrors(t, f.loader, "")
		expected := `{"employees":[{"__typename":"Employee","id":1,"products":[{"upc":"1"}]}],"employee":{"__typename":"Employee","id":9}}`
		assert.JSONEq(t, expected, string(f.loader.dataBuffer.Get().MarshalTo(nil)))
	})

	t.Run("batch origin empty array errors like unmerged", func(t *testing.T) {
		f := multiFetchFixture(t)
		f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[],"f2":[{"notes":"n"}]}}`)}

		require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

		// An empty _entities array yields GetArray()==nil, so mergeResult renders the
		// same invalidGraphQLResponseShape error an unmerged BatchEntityFetch would.
		assertMergedErrors(t, f.loader, `[{"message":"Failed to fetch from Subgraph at Path 'employees', Reason: no data or errors in response."}]`)
		expected := `{"employees":[{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9,"notes":"n"}}`
		assert.JSONEq(t, expected, string(f.loader.dataBuffer.Get().MarshalTo(nil)))
	})
}

func TestMergeMultiEntityResult_TransportError(t *testing.T) {
	t.Run("error at every non-excluded entry path", func(t *testing.T) {
		f := multiFetchFixture(t)
		f.fetch.FetchID = 5
		f.fetch.DataSource = &recordingDataSource{err: errors.New("boom")}

		require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

		assertMergedErrors(t, f.loader, `[`+
			`{"message":"Failed to fetch from Subgraph at Path 'employees'."},`+
			`{"message":"Failed to fetch from Subgraph at Path 'employee'."}`+
			`]`)
		assert.Contains(t, f.loader.erroredFetchIDs, 5)
	})

	t.Run("excluded entry gets no error", func(t *testing.T) {
		f := multiFetchFixture(t, withSeed(`{"employees":[{"__typename":"Employee","id":1}],"employee":null}`))
		f.fetch.FetchID = 5
		f.fetch.DataSource = &recordingDataSource{err: errors.New("boom")}

		require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

		// Only the live employees entry gets a transport error; the excluded
		// employee entry (parent null) produces none.
		assertMergedErrors(t, f.loader, `[{"message":"Failed to fetch from Subgraph at Path 'employees'."}]`)
	})
}

func TestMergeMultiEntityResult_InvalidResponse(t *testing.T) {
	f := multiFetchFixture(t)
	f.fetch.FetchID = 5
	f.fetch.DataSource = &recordingDataSource{response: []byte(`not json`)}

	require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

	assertMergedErrors(t, f.loader, `[`+
		`{"message":"Failed to fetch from Subgraph at Path 'employees', Reason: invalid JSON."},`+
		`{"message":"Failed to fetch from Subgraph at Path 'employee', Reason: invalid JSON."}`+
		`]`)
	assert.NotContains(t, f.loader.erroredFetchIDs, 5)
}

func TestMergeMultiEntityResult_RateLimitRejected(t *testing.T) {
	f := multiFetchFixture(t)
	f.ctx.RateLimitOptions = RateLimitOptions{Enable: true}
	f.ctx.rateLimiter = &testRateLimiter{allowFn: func(*Context, *FetchInfo, json.RawMessage) (*RateLimitDeny, error) {
		return &RateLimitDeny{Reason: "over limit"}, nil
	}}
	ds := &recordingDataSource{response: []byte(`{"data":{}}`)}
	f.fetch.DataSource = ds

	require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

	assert.Equal(t, 0, ds.calls, "no subgraph request when rate limited")
	assertMergedErrors(t, f.loader, `[`+
		`{"message":"Rate limit exceeded for Subgraph request at Path 'employees', Reason: over limit."},`+
		`{"message":"Rate limit exceeded for Subgraph request at Path 'employee', Reason: over limit."}`+
		`]`)
}

func TestMergeMultiEntityResult_TaintPerEntry(t *testing.T) {
	f := multiFetchFixture(t)
	f.loader.validateRequiredExternalFields = true
	f.fetch.Input.Entries[0].Info = &FetchInfo{
		OperationType: ast.OperationTypeQuery,
		DataSourceID:  "products-id",
		FetchReasons:  []FetchReason{{TypeName: "Employee", FieldName: "x", IsRequires: true, Nullable: true}},
	}
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"__typename":"Employee","x":null}],"f2":[{"notes":"n"}]},"errors":[{"message":"e","path":["f1",0,"x"]}]}`)}

	require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

	assert.Len(t, f.loader.taintedObjs, 1)
}

func TestMergeMultiEntityResult_ExtensionsOnce(t *testing.T) {
	f := multiFetchFixture(t)
	f.loader.allowCustomExtensionProperties = true
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[]}],"f2":[{"notes":"n"}]},"extensions":{"foo":"bar"}}`)}

	require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

	assert.Len(t, f.loader.subgraphExtensions, 1)
}

func TestMergeMultiEntityResult_HooksOnce(t *testing.T) {
	f := multiFetchFixture(t)
	hooks := NewTestLoaderHooks()
	f.ctx.LoaderHooks = hooks
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[]}],"f2":[{"notes":"n"}]}}`)}

	require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

	assert.Equal(t, int64(1), hooks.preFetchCalls.Load())
	assert.Equal(t, int64(1), hooks.postFetchCalls.Load())
}

func TestMergeMultiEntityResult_ExcludedEntry(t *testing.T) {
	f := multiFetchFixture(t, withSeed(`{"employees":[{"__typename":"Employee","id":1}],"employee":null}`))
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[{"upc":"1"}]}]}}`)}

	require.NoError(t, f.loader.resolveSingle(context.Background(), f.item))

	assertMergedErrors(t, f.loader, "")
	expected := `{"employees":[{"__typename":"Employee","id":1,"products":[{"upc":"1"}]}],"employee":null}`
	assert.JSONEq(t, expected, string(f.loader.dataBuffer.Get().MarshalTo(nil)))
}

// ---------------------------------------------------------------------------
// Full-tree integration (prepare + load + merge)
// ---------------------------------------------------------------------------

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
		assertMergedErrors(t, loader, "")
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
		assertMergedErrors(t, loader, "")
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
