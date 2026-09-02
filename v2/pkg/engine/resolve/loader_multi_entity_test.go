package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

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

	ctx := NewContext(t.Context())
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
// captures the last received input. A non-nil responseHeaders publishes those
// headers through the httpclient response context, which is where the response
// cache reads Cache-Control from.
type recordingDataSource struct {
	response        []byte
	err             error
	responseHeaders http.Header
	calls           int
	lastInput       []byte
}

func (r *recordingDataSource) Load(ctx context.Context, _ http.Header, input []byte) ([]byte, error) {
	r.record(ctx, input)
	return r.response, r.err
}

func (r *recordingDataSource) LoadWithFiles(ctx context.Context, _ http.Header, input []byte, _ []*httpclient.FileUpload) ([]byte, error) {
	r.record(ctx, input)
	return r.response, r.err
}

func (r *recordingDataSource) record(ctx context.Context, input []byte) {
	r.calls++
	r.lastInput = append(r.lastInput[:0], input...)
	if r.responseHeaders == nil {
		return
	}
	responseContext := httpclient.GetResponseContext(ctx)
	if responseContext == nil {
		return
	}
	responseContext.StatusCode = http.StatusOK
	responseContext.Response = &http.Response{
		StatusCode: http.StatusOK,
		Header:     r.responseHeaders.Clone(),
	}
}

func TestMergeMultiEntityResult_FanOut(t *testing.T) {
	f := multiFetchFixture(t, withSeed(`{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2},{"__typename":"Employee","id":1}],"employee":{"__typename":"Employee","id":9}}`))
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[{"upc":"1"}]},{"products":[{"upc":"2"}]}],"f2":[{"notes":"n"}]}}`)}

	require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

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

		require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

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

		require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

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

		require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

		assertMergedErrors(t, f.loader, "")
		expected := `{"employees":[{"__typename":"Employee","id":1,"products":[{"upc":"1"}]}],"employee":{"__typename":"Employee","id":9}}`
		assert.JSONEq(t, expected, string(f.loader.dataBuffer.Get().MarshalTo(nil)))
	})

	t.Run("batch origin empty array errors like unmerged", func(t *testing.T) {
		f := multiFetchFixture(t)
		f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[],"f2":[{"notes":"n"}]}}`)}

		require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

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

		require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

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

		require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

		// Only the live employees entry gets a transport error; the excluded
		// employee entry (parent null) produces none.
		assertMergedErrors(t, f.loader, `[{"message":"Failed to fetch from Subgraph at Path 'employees'."}]`)
	})
}

func TestMergeMultiEntityResult_InvalidResponse(t *testing.T) {
	f := multiFetchFixture(t)
	f.fetch.FetchID = 5
	f.fetch.DataSource = &recordingDataSource{response: []byte(`not json`)}

	require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

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

	require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

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

	require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

	assert.Len(t, f.loader.taintedObjs, 1)
}

func TestMergeMultiEntityResult_ExtensionsOnce(t *testing.T) {
	f := multiFetchFixture(t)
	f.loader.allowCustomExtensionProperties = true
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[]}],"f2":[{"notes":"n"}]},"extensions":{"foo":"bar"}}`)}

	require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

	assert.Len(t, f.loader.subgraphExtensions, 1)
}

func TestMergeMultiEntityResult_HooksOnce(t *testing.T) {
	f := multiFetchFixture(t)
	hooks := NewTestLoaderHooks()
	f.ctx.LoaderHooks = hooks
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[]}],"f2":[{"notes":"n"}]}}`)}

	require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

	assert.Equal(t, int64(1), hooks.preFetchCalls.Load())
	assert.Equal(t, int64(1), hooks.postFetchCalls.Load())
}

func TestMergeMultiEntityResult_ExcludedEntry(t *testing.T) {
	f := multiFetchFixture(t, withSeed(`{"employees":[{"__typename":"Employee","id":1}],"employee":null}`))
	f.fetch.DataSource = &recordingDataSource{response: []byte(`{"data":{"f1":[{"products":[{"upc":"1"}]}]}}`)}

	require.NoError(t, f.loader.resolveSingle(t.Context(), f.item))

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

// multiEntityFirstVar is the f2 entry's non-representations variable, shared by
// every merged tree below.
func multiEntityFirstVar() []MultiEntityFetchVariable {
	return []MultiEntityFetchVariable{
		{KeyPrefix: []byte(`,"first_f2":`), Value: multiContextVariableTemplate("first")},
	}
}

// multiEntityMergedResponse is what the merged subgraph request answers when
// both entries are included.
const multiEntityMergedResponse = `{"data":{"f1":[{"products":["a"]},{"products":["b"]}],"f2":[{"notes":"n"}]}}`

// multiEntityExpectedData is the data buffer both the merged and the unmerged
// tree produce.
const multiEntityExpectedData = `{"employees":[{"__typename":"Employee","id":1,"products":["a"]},{"__typename":"Employee","id":2,"products":["b"]},{"__typename":"Employee","id":1,"products":["a"]}],"employee":{"__typename":"Employee","id":9,"notes":"n"}}`

// multiEntityMergedTree is the root fetch plus the MultiEntityFetch that depends
// on it: one merged request covering both the employees batch and the employee
// single entry.
func multiEntityMergedTree(rootDS, multiDS DataSource) *GraphQLResponse {
	multi := twoEntryMultiFetch(multiEntityFirstVar())
	multi.FetchID = 1
	multi.DependsOnFetchIDs = []int{0}
	multi.DataSource = multiDS
	return &GraphQLResponse{Fetches: Sequence(
		Single(multiEntityRootFetch(rootDS)),
		Single(multi),
	)}
}

// multiEntityUnmergedTree is the same work expressed as the two separate entity
// fetches MultiFetch merges, the baseline the merged run must match.
func multiEntityUnmergedTree(rootDS, batchDS, entityDS DataSource) *GraphQLResponse {
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
		DataSource:     batchDS,
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
		DataSource:     entityDS,
		PostProcessing: PostProcessingConfiguration{SelectResponseDataPath: []string{"data", "_entities", "0"}},
		Info:           &FetchInfo{OperationType: ast.OperationTypeQuery, DataSourceID: "products-id"},
	}
	return &GraphQLResponse{Fetches: Sequence(
		Single(multiEntityRootFetch(rootDS)),
		SingleWithPath(batch, "employees", ArrayPath("employees")),
		SingleWithPath(entity, "employee", ObjectPath("employee")),
	)}
}

// multiEntityContext is the request context both trees run under.
func multiEntityContext(t *testing.T) *Context {
	t.Helper()
	ctx := NewContext(t.Context())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.Variables = astjson.MustParse(`{"first":10}`)
	return ctx
}

// TestLoadGraphQLResponseData_MultiEntity drives prepare+load+merge for a full
// tree and asserts the merged run issues one subgraph request with the expected
// assembled input and produces a data buffer byte-identical to the unmerged run.
func TestLoadGraphQLResponseData_MultiEntity(t *testing.T) {
	runMerged := func(t *testing.T) (out string, multiDS *recordingDataSource) {
		t.Helper()
		multiDS = &recordingDataSource{response: []byte(multiEntityMergedResponse)}
		response := multiEntityMergedTree(
			&recordingDataSource{response: []byte(multiEntityRootResponse)},
			multiDS,
		)
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(multiEntityContext(t), response))
		assertMergedErrors(t, loader, "")
		return string(loader.dataBuffer.Get().MarshalTo(nil)), multiDS
	}

	runUnmerged := func(t *testing.T) string {
		t.Helper()
		response := multiEntityUnmergedTree(
			&recordingDataSource{response: []byte(multiEntityRootResponse)},
			&recordingDataSource{response: []byte(`{"data":{"_entities":[{"products":["a"]},{"products":["b"]}]}}`)},
			&recordingDataSource{response: []byte(`{"data":{"_entities":[{"notes":"n"}]}}`)},
		)
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(multiEntityContext(t), response))
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
	ctx := NewContext(t.Context())
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

// ---------------------------------------------------------------------------
// Response cache
// ---------------------------------------------------------------------------

// cacheableHeaders makes a subgraph response eligible for the response cache.
func cacheableHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Cache-Control", "public, max-age=60")
	return headers
}

// cachedEntityValues is what the cache holds, sorted so the assertion does not
// depend on map order.
func cachedEntityValues(cache *testCache) []string {
	values := make([]string, 0, len(cache.items))
	for _, item := range cache.items {
		values = append(values, string(item.Value))
	}
	slices.Sort(values)
	return values
}

// TestLoadGraphQLResponseData_MultiEntity_ResponseCache pins the response-cache
// behaviour a merged fetch must have: the merged response is broken down per
// alias and cached one entity object at a time, exactly like the unmerged
// entity fetches it replaces, so a warm cache keeps the origin calls down to
// what is actually missing.
func TestLoadGraphQLResponseData_MultiEntity_ResponseCache(t *testing.T) {
	// runMerged loads the merged tree against the given cache and data source
	// response, and returns the resulting data buffer.
	runMerged := func(t *testing.T, cache *testCache, multiDS *recordingDataSource) string {
		t.Helper()
		ctx := multiEntityContext(t)
		ctx.SetResponseCache(cache, 60*time.Second, func(err error) {
			t.Errorf("response cache error: %v", err)
		})
		response := multiEntityMergedTree(
			&recordingDataSource{response: []byte(multiEntityRootResponse)},
			multiDS,
		)
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, response))
		assertMergedErrors(t, loader, "")
		return string(loader.dataBuffer.Get().MarshalTo(nil))
	}

	t.Run("caches one entity per alias, in the same shape as an unmerged fetch", func(t *testing.T) {
		cache := newTestCache()
		multiDS := &recordingDataSource{
			response:        []byte(multiEntityMergedResponse),
			responseHeaders: cacheableHeaders(),
		}

		assert.JSONEq(t, multiEntityExpectedData, runMerged(t, cache, multiDS))
		require.Equal(t, 1, multiDS.calls)

		// Three entities go in, not one merged blob: two from the f1 batch entry
		// (the third employee deduplicates onto the first) and one from f2. Each
		// value is the bare entity object a single-entity cache hit is rebuilt
		// from, byte-identical to what the unmerged fetches would have stored.
		require.Len(t, cache.items, 3, "expected 3 cached entities: 2 for f1 + 1 for f2")
		assert.Equal(t, []string{`{"notes":"n"}`, `{"products":["a"]}`, `{"products":["b"]}`}, cachedEntityValues(cache))
	})

	t.Run("a fully warm cache issues no subgraph request", func(t *testing.T) {
		cache := newTestCache()
		warm := &recordingDataSource{
			response:        []byte(multiEntityMergedResponse),
			responseHeaders: cacheableHeaders(),
		}
		expected := runMerged(t, cache, warm)
		require.Equal(t, 1, warm.calls)

		// Second run over the same cache: every entry of the merged fetch hits,
		// so the merged request is never sent and the data buffer is unchanged.
		cached := &recordingDataSource{err: errors.New("subgraph must not be called")}
		assert.Equal(t, expected, runMerged(t, cache, cached))
		assert.Equal(t, 0, cached.calls, "a fully warm merged fetch must not reach the subgraph")
	})

	t.Run("a partially warm cache only asks for the missing entry", func(t *testing.T) {
		cache := newTestCache()
		warm := &recordingDataSource{
			response:        []byte(multiEntityMergedResponse),
			responseHeaders: cacheableHeaders(),
		}
		expected := runMerged(t, cache, warm)

		// Evict f2's entity, leaving both f1 entities warm.
		for key, item := range cache.items {
			if bytes.Contains(item.Value, []byte("notes")) {
				delete(cache.items, key)
			}
		}
		require.Len(t, cache.items, 2)

		partial := &recordingDataSource{
			response:        []byte(`{"data":{"f2":[{"notes":"n"}]}}`),
			responseHeaders: cacheableHeaders(),
		}
		assert.Equal(t, expected, runMerged(t, cache, partial))

		// A cache hit is per alias: f1 is switched off exactly like an entry with
		// no live representations, so the origin only answers for f2.
		require.Equal(t, 1, partial.calls)
		expectedInput := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[],"includeF1":false,"representations_f2":[{"__typename":"Employee","id":9}],"includeF2":true,"first_f2":10}}}`
		assert.Equal(t, expectedInput, string(partial.lastInput))
	})
}

// TestLoadGraphQLResponseData_MultiEntity_ResponseCacheKeys pins what a merged
// entry's cache key covers. A merged request has one header and footer spanning
// every alias, so the two things an unmerged fetch gets for free — its own
// variables and its own selection — have to be hashed in per entry.
func TestLoadGraphQLResponseData_MultiEntity_ResponseCacheKeys(t *testing.T) {
	runMerged := func(t *testing.T, cache *testCache, variables string, multiDS *recordingDataSource, rootDS *recordingDataSource) {
		t.Helper()
		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		ctx.Variables = astjson.MustParse(variables)
		ctx.SetResponseCache(cache, 60*time.Second, func(err error) {
			t.Errorf("response cache error: %v", err)
		})
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, multiEntityMergedTree(rootDS, multiDS)))
		assertMergedErrors(t, loader, "")
	}

	t.Run("an entry variable is part of its key", func(t *testing.T) {
		cache := newTestCache()
		root := func() *recordingDataSource {
			return &recordingDataSource{response: []byte(multiEntityRootResponse)}
		}

		warm := &recordingDataSource{response: []byte(multiEntityMergedResponse), responseHeaders: cacheableHeaders()}
		runMerged(t, cache, `{"first":10}`, warm, root())
		require.Equal(t, 1, warm.calls)

		// Same entities, different client variable: the f2 entry renders
		// "first_f2":999, so its cached entity is a different entity and the
		// origin has to answer for it.
		other := &recordingDataSource{response: []byte(multiEntityMergedResponse), responseHeaders: cacheableHeaders()}
		runMerged(t, cache, `{"first":999}`, other, root())
		require.Equal(t, 1, other.calls, "a different entry variable must not be served from the cache")
		expectedInput := `{"method":"POST","url":"http://x","body":{"query":"Q","variables":{"representations_f1":[],"includeF1":false,"representations_f2":[{"__typename":"Employee","id":9}],"includeF2":true,"first_f2":999}}}`
		assert.Equal(t, expectedInput, string(other.lastInput),
			"f1 has no variables, so it stays warm; only f2 is re-requested")
	})

	t.Run("two entries over the same entity do not share a key", func(t *testing.T) {
		cache := newTestCache()
		// Employee id=1 is reachable through both aliases, the
		// DedupStateIsolation shape. f1 selects products, f2 selects notes: one
		// key each, or one overwrites the other.
		root := &recordingDataSource{response: []byte(`{"data":{"employees":[{"__typename":"Employee","id":1},{"__typename":"Employee","id":2}],"employee":{"__typename":"Employee","id":1}}}`)}
		multiDS := &recordingDataSource{
			response:        []byte(`{"data":{"f1":[{"products":["a"]},{"products":["b"]}],"f2":[{"notes":"n"}]}}`),
			responseHeaders: cacheableHeaders(),
		}
		runMerged(t, cache, `{"first":10}`, multiDS, root)

		require.Len(t, cache.items, 3, "3 entities cached under 3 distinct keys")
		assert.Equal(t, []string{`{"notes":"n"}`, `{"products":["a"]}`, `{"products":["b"]}`}, cachedEntityValues(cache))
	})
}

// TestLoadGraphQLResponseData_MultiEntity_ResponseCachePartialFailure covers the
// two ways a merged fetch is only partly usable: one alias errors, or the request
// fails after some entries were already served from the cache. In both cases the
// entries that are fine must not be dragged down with the ones that are not.
func TestLoadGraphQLResponseData_MultiEntity_ResponseCachePartialFailure(t *testing.T) {
	load := func(t *testing.T, cache *testCache, multiDS *recordingDataSource) (string, *Loader) {
		t.Helper()
		ctx := multiEntityContext(t)
		ctx.SetResponseCache(cache, 60*time.Second, func(err error) {
			t.Errorf("response cache error: %v", err)
		})
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		response := multiEntityMergedTree(
			&recordingDataSource{response: []byte(multiEntityRootResponse)},
			multiDS,
		)
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, response))
		return string(loader.dataBuffer.Get().MarshalTo(nil)), loader
	}

	t.Run("an alias with errors is not cached, the other alias still is", func(t *testing.T) {
		cache := newTestCache()
		multiDS := &recordingDataSource{
			response: []byte(`{"data":{"f1":[{"products":["a"]},{"products":["b"]}],"f2":[null]},` +
				`"errors":[{"message":"boom","path":["f2"]}]}`),
			responseHeaders: cacheableHeaders(),
		}
		_, loader := load(t, cache, multiDS)
		require.NotNil(t, loader.errors, "the f2 error is propagated")

		// f1 answered cleanly, so its entities are stored; f2 carries an error and
		// is left out. Unmerged, these are two independent fetches and one
		// failing never stopped the other being cached.
		assert.Equal(t, []string{`{"products":["a"]}`, `{"products":["b"]}`}, cachedEntityValues(cache))
	})

	t.Run("a failing request does not discard the entries already served from the cache", func(t *testing.T) {
		cache := newTestCache()
		warm := &recordingDataSource{
			response:        []byte(multiEntityMergedResponse),
			responseHeaders: cacheableHeaders(),
		}
		load(t, cache, warm)

		// Evict f2, then fail the request that goes out for it. f1 is warm and was
		// switched off, so it keeps its data and only f2 reports the failure.
		for key, item := range cache.items {
			if bytes.Contains(item.Value, []byte("notes")) {
				delete(cache.items, key)
			}
		}
		failing := &recordingDataSource{err: errors.New("boom")}
		out, loader := load(t, cache, failing)

		expected := `{"employees":[{"__typename":"Employee","id":1,"products":["a"]},{"__typename":"Employee","id":2,"products":["b"]},{"__typename":"Employee","id":1,"products":["a"]}],"employee":{"__typename":"Employee","id":9}}`
		assert.JSONEq(t, expected, out)
		assertMergedErrors(t, loader, `[{"message":"Failed to fetch from Subgraph at Path 'employee'."}]`)
	})
}

// TestLoadGraphQLResponseData_MultiEntity_ResponseCacheReporting pins what a
// merged fetch reports to the engine loader hooks. OnFinished fires once for the
// merged request, so a hit is reported only when every entry was warm, with the
// life left on the least fresh of them.
func TestLoadGraphQLResponseData_MultiEntity_ResponseCacheReporting(t *testing.T) {
	type cacheReport struct {
		hit bool
		ttl time.Duration
	}

	load := func(t *testing.T, cache *testCache, multiDS *recordingDataSource) []cacheReport {
		t.Helper()
		ctx := multiEntityContext(t)
		ctx.SetResponseCache(cache, 60*time.Second, func(err error) {
			t.Errorf("response cache error: %v", err)
		})
		var reports []cacheReport
		ctx.SetEngineLoaderHooks(&spyLoaderHooks{
			onFinished: func(_ context.Context, _ DataSourceInfo, info *ResponseInfo) {
				reports = append(reports, cacheReport{hit: info.ResponseCacheHit, ttl: info.ResponseCacheTTL})
			},
		})
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		response := multiEntityMergedTree(
			&recordingDataSource{response: []byte(multiEntityRootResponse)},
			multiDS,
		)
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, response))
		assertMergedErrors(t, loader, "")
		return reports
	}

	cache := newTestCache()
	warm := &recordingDataSource{
		response:        []byte(multiEntityMergedResponse),
		responseHeaders: cacheableHeaders(),
	}

	// The root fetch is not cacheable, so it reports no hit on either run; the
	// merged fetch reports one only on the second.
	require.Equal(t, []cacheReport{{}, {}}, load(t, cache, warm))

	cached := &recordingDataSource{err: errors.New("subgraph must not be called")}
	require.Equal(t, []cacheReport{
		{hit: false, ttl: 0},
		{hit: true, ttl: 60 * time.Second},
	}, load(t, cache, cached))
}
