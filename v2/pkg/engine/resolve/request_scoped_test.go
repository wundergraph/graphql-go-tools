package resolve

import (
	"bytes"
	"context"
	"reflect"
	"runtime/debug"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// mustParseArena is a test helper that parses JSON into an arena-allocated value.
func mustParseArena(t *testing.T, ar arena.Arena, data string) *astjson.Value {
	t.Helper()
	v, err := astjson.ParseBytesWithArena(ar, []byte(data))
	require.NoError(t, err)
	return v
}

// newViewerObj constructs a ProvidesData Object describing a nullable viewer
// with the given scalar sub-fields. Callers may append alias/CacheArgs fields
// afterwards. ComputeHasAliases is invoked so the HasAliases gate is set.
func newViewerObj(fieldNames ...string) *Object {
	fields := make([]*Field, 0, len(fieldNames))
	for _, name := range fieldNames {
		fields = append(fields, &Field{
			Name:  []byte(name),
			Value: &Scalar{Nullable: true},
		})
	}
	obj := &Object{
		Nullable: true,
		Fields:   fields,
	}
	ComputeHasAliases(obj)
	return obj
}

func valueLivesOnArena(a arena.Arena, value *astjson.Value) bool {
	if a == nil || value == nil {
		return false
	}

	arenaValue := reflect.ValueOf(a)
	if arenaValue.Kind() == reflect.Ptr {
		arenaValue = arenaValue.Elem()
	}
	if !arenaValue.IsValid() {
		return false
	}

	buffers := arenaValue.FieldByName("buffers")
	if !buffers.IsValid() {
		return false
	}

	ptr := uintptr(unsafe.Pointer(value))
	for i := 0; i < buffers.Len(); i++ {
		bufferValue := buffers.Index(i)
		if bufferValue.IsNil() {
			continue
		}
		bufferValue = bufferValue.Elem()
		start := uintptr(bufferValue.FieldByName("ptr").Pointer())
		size := uintptr(bufferValue.FieldByName("size").Uint())
		if start == 0 || size == 0 {
			continue
		}
		if ptr >= start && ptr < start+size {
			return true
		}
	}

	return false
}

func TestRequestScopedInjection_MultipleItemsSurvivesGCWhileRendering(t *testing.T) {
	t.Parallel()

	old := debug.SetGCPercent(1)
	defer debug.SetGCPercent(old)

	renderShape := &Object{
		Nullable: true,
		Fields: []*Field{
			{
				Name: []byte("articles"),
				Value: &Array{
					Path:     []string{"articles"},
					Nullable: true,
					Item: &Object{
						Nullable: true,
						Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: true}},
							{
								Name: []byte("currentViewer"),
								Value: &Object{
									Nullable: true,
									Path:     []string{"currentViewer"},
									Fields: []*Field{
										{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: true}},
										{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: true}},
										{Name: []byte("email"), Value: &String{Path: []string{"email"}, Nullable: true}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	injectCfg := FetchCacheConfiguration{
		RequestScopedFields: []RequestScopedField{
			{
				FieldName:    "currentViewer",
				FieldPath:    []string{"currentViewer"},
				L1Key:        "viewer.Personalized.currentViewer",
				ProvidesData: newViewerObj("id", "name", "email"),
			},
		},
	}

	for i := range gcIterations {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
		ctx := NewContext(context.Background())
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		resolvable := NewResolvable(ar, ResolvableOptions{})
		require.NoError(t, resolvable.Init(ctx, []byte(`{"articles":[{"id":"a1"},{"id":"a2"},{"id":"a3"}]}`), ast.OperationTypeQuery))

		loader := &Loader{
			jsonArena:       ar,
			ctx:             ctx,
			resolvable:      resolvable,
			requestScopedL1: map[string]*astjson.Value{},
		}
		loader.requestScopedL1["viewer.Personalized.currentViewer"] = mustParseArena(t, ar, `{"id":"v1","name":"Alice","email":"alice@example.com"}`)

		items := resolvable.data.Get("articles").GetArray()
		require.Len(t, items, 3)
		require.True(t, loader.tryRequestScopedInjection(&result{}, injectCfg, items))

		forceGC()
		heapChurn := make([][]byte, 0, 256)
		for range 256 {
			heapChurn = append(heapChurn, bytes.Repeat([]byte("x"), 1024))
		}
		forceGC()

		out := &bytes.Buffer{}
		err := resolvable.Resolve(ctx.ctx, renderShape, nil, out)
		require.NoError(t, err, "iteration %d", i)
		assert.Equal(t,
			`{"data":{"articles":[{"id":"a1","currentViewer":{"id":"v1","name":"Alice","email":"alice@example.com"}},{"id":"a2","currentViewer":{"id":"v1","name":"Alice","email":"alice@example.com"}},{"id":"a3","currentViewer":{"id":"v1","name":"Alice","email":"alice@example.com"}}]}}`,
			out.String(),
			"iteration %d",
			i,
		)

		_ = heapChurn
	}
}

func TestRequestScopedInjection_MultipleItemsStoresValuesOnRequestArena(t *testing.T) {
	t.Parallel()

	old := debug.SetGCPercent(1)
	defer debug.SetGCPercent(old)

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	resolvable := NewResolvable(ar, ResolvableOptions{})
	require.NoError(t, resolvable.Init(ctx, []byte(`{"articles":[{"id":"a1"},{"id":"a2"}]}`), ast.OperationTypeQuery))

	loader := &Loader{
		jsonArena:       ar,
		ctx:             ctx,
		resolvable:      resolvable,
		requestScopedL1: map[string]*astjson.Value{},
	}
	loader.requestScopedL1["viewer.Personalized.currentViewer"] = mustParseArena(t, ar, `{"id":"v1","name":"Alice","email":"alice@example.com"}`)

	items := resolvable.data.Get("articles").GetArray()
	require.Len(t, items, 2)
	require.True(t, loader.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
		RequestScopedFields: []RequestScopedField{
			{
				FieldName:    "currentViewer",
				FieldPath:    []string{"currentViewer"},
				L1Key:        "viewer.Personalized.currentViewer",
				ProvidesData: newViewerObj("id", "name", "email"),
			},
		},
	}, items))

	firstInjected := items[0].Get("currentViewer")
	secondInjected := items[1].Get("currentViewer")
	require.NotNil(t, firstInjected)
	require.NotNil(t, secondInjected)
	assert.True(t, valueLivesOnArena(ar, firstInjected), "first injected value must be allocated on the request arena")
	assert.True(t, valueLivesOnArena(ar, secondInjected), "second injected value must be allocated on the request arena")
}

func TestTryRequestScopedInjection(t *testing.T) {
	t.Parallel()

	t.Run("no hints returns false", func(t *testing.T) {
		t.Parallel()

		l := &Loader{
			jsonArena:       arena.NewMonotonicArena(arena.WithMinBufferSize(1024)),
			requestScopedL1: map[string]*astjson.Value{},
		}
		cfg := FetchCacheConfiguration{}
		items := []*astjson.Value{astjson.MustParse(`{"id":"1"}`)}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.False(t, ok)
	})

	t.Run("hint not in cache returns false", func(t *testing.T) {
		t.Parallel()

		l := &Loader{
			jsonArena:       arena.NewMonotonicArena(arena.WithMinBufferSize(1024)),
			requestScopedL1: map[string]*astjson.Value{},
		}
		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName: "currentViewer",
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
				},
			},
		}
		items := []*astjson.Value{astjson.MustParse(`{"id":"1"}`)}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.False(t, ok)
		assert.Equal(t, `{"id":"1"}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("all hints found injects and returns true", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cachedViewer := mustParseArena(t, ar, `{"name":"Alice","role":"admin"}`)
		l.requestScopedL1["viewer.Personalized.currentViewer"] = cachedViewer

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("name", "role"),
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"1"}`),
			mustParseArena(t, ar, `{"id":"2"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.True(t, ok)

		assert.Equal(t, `{"id":"1","currentViewer":{"name":"Alice","role":"admin"}}`, string(items[0].MarshalTo(nil)))
		assert.Equal(t, `{"id":"2","currentViewer":{"name":"Alice","role":"admin"}}`, string(items[1].MarshalTo(nil)))
	})

	t.Run("field widening blocks injection when cached value missing required fields", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cachedViewer := mustParseArena(t, ar, `{"id":"1","name":"Alice"}`)
		l.requestScopedL1["viewer.Personalized.currentViewer"] = cachedViewer

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("id", "name", "email"),
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.False(t, ok)
		// Items should NOT be modified
		assert.Equal(t, `{"id":"99"}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("field widening allows injection when cached value has all required fields", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cachedViewer := mustParseArena(t, ar, `{"id":"1","name":"Alice","email":"a@b.com"}`)
		l.requestScopedL1["viewer.Personalized.currentViewer"] = cachedViewer

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("id", "name"),
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.True(t, ok)
		// DeepCopy preserves all fields from the cached value. Extra fields
		// beyond the hint's ProvidesData are harmless — the response walker
		// only renders fields listed in the query, so "email" is ignored
		// downstream even though it appears in the injected value.
		assert.Equal(t, `{"id":"99","currentViewer":{"id":"1","name":"Alice","email":"a@b.com"}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("nil ProvidesData blocks injection (fail-closed)", func(t *testing.T) {
		// Hints with nil ProvidesData describe fields that aren't selected by
		// THIS fetch. Without ProvidesData we cannot run the widening check, so
		// injecting unconditionally would let the resolver short-circuit a
		// fetch whose real (non-@requestScoped) selections were never loaded.
		// tryRequestScopedInjection therefore returns false and leaves items
		// untouched.
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cachedViewer := mustParseArena(t, ar, `{"id":"1"}`)
		l.requestScopedL1["viewer.Personalized.currentViewer"] = cachedViewer

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName: "currentViewer",
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
					// ProvidesData intentionally nil
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.False(t, ok)
		assert.Equal(t, `{"id":"99"}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("hint without ProvidesData blocks fetch skip even when L1 has the value", func(t *testing.T) {
		// Regression for the over-application bug: if the datasource planner
		// emits a hint for an @requestScoped field that THIS fetch doesn't
		// actually select, populateRequestScopedFieldsProvidesData should drop
		// the hint at plan time. As a defense-in-depth, the runtime injection
		// path here must ALSO refuse to skip the fetch when ProvidesData is
		// nil — even though the L1 entry exists. Otherwise the resolver would
		// short-circuit a fetch whose real selections were never loaded.
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}
		l.requestScopedL1["viewer.Personalized.currentViewer"] = mustParseArena(t, ar, `{"name":"Alice"}`)

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName: "currentViewer",
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
					// ProvidesData nil — field isn't part of THIS fetch's selection.
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.False(t, ok, "must refuse to skip the fetch when hint cannot be widening-checked")
		assert.Equal(t, `{"id":"99"}`, string(items[0].MarshalTo(nil)), "items must not be mutated")
	})

	t.Run("partial hints returns false but does not mutate items", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cachedViewer := mustParseArena(t, ar, `{"name":"Alice"}`)
		l.requestScopedL1["viewer.Personalized.currentViewer"] = cachedViewer

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("name"),
				},
				{
					FieldName:    "settings",
					FieldPath:    []string{"settings"},
					L1Key:        "viewer.Personalized.settings",
					ProvidesData: newViewerObj("theme"),
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"1"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.False(t, ok)

		// With collect-then-inject, items are NOT mutated when any hint fails.
		assert.Equal(t, `{"id":"1"}`, string(items[0].MarshalTo(nil)))
	})
}

func TestExportRequestScopedFields(t *testing.T) {
	t.Parallel()

	t.Run("no exports is a no-op", func(t *testing.T) {
		t.Parallel()

		l := &Loader{
			jsonArena:       arena.NewMonotonicArena(arena.WithMinBufferSize(1024)),
			requestScopedL1: map[string]*astjson.Value{},
		}
		cfg := FetchCacheConfiguration{}
		items := []*astjson.Value{astjson.MustParse(`{"id":"1"}`)}

		l.exportRequestScopedFields(&result{}, cfg, items)
		count := len(l.requestScopedL1)
		assert.Equal(t, 0, count)
	})

	t.Run("exports value from first entity", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"1","currentViewer":{"name":"Alice"}}`),
			mustParseArena(t, ar, `{"id":"2","currentViewer":{"name":"Alice"}}`),
		}

		l.exportRequestScopedFields(&result{}, cfg, items)

		cached, ok := l.requestScopedL1["viewer.Personalized.currentViewer"]
		require.True(t, ok)
		assert.Equal(t, `{"name":"Alice"}`, string(cached.MarshalTo(nil)))
	})

	t.Run("skips null values", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"1","currentViewer":null}`),
		}

		l.exportRequestScopedFields(&result{}, cfg, items)

		_, ok := l.requestScopedL1["viewer.Personalized.currentViewer"]
		assert.False(t, ok)
	})

	t.Run("merges into existing cached value", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		existing := mustParseArena(t, ar, `{"name":"Alice"}`)
		l.requestScopedL1["viewer.Personalized.currentViewer"] = existing

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"1","currentViewer":{"name":"Alice","role":"admin"}}`),
		}

		l.exportRequestScopedFields(&result{}, cfg, items)

		cached, ok := l.requestScopedL1["viewer.Personalized.currentViewer"]
		require.True(t, ok)
		marshaled := string(cached.MarshalTo(nil))
		assert.Equal(t, `{"name":"Alice","role":"admin"}`, marshaled)
	})
}

func TestRequestScopedRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("export then inject round-trip with field widening", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Step 1: Export {id, name} from root field
		exportCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
				},
			},
		}
		exportItems := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"1","currentViewer":{"id":"1","name":"Alice"}}`),
		}
		l.exportRequestScopedFields(&result{}, exportCfg, exportItems)

		// Step 2: Try injection with ProvidesData that demands "email" (missing) — should fail
		injectCfg1 := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("id", "name", "email"),
				},
			},
		}
		injectItems1 := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
		}
		ok := l.tryRequestScopedInjection(&result{}, injectCfg1, injectItems1)
		assert.False(t, ok)

		// Step 3: Try injection with ProvidesData that is satisfied — should succeed
		injectCfg2 := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("id", "name"),
				},
			},
		}
		injectItems2 := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
		}
		ok = l.tryRequestScopedInjection(&result{}, injectCfg2, injectItems2)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"99","currentViewer":{"id":"1","name":"Alice"}}`, string(injectItems2[0].MarshalTo(nil)))
	})

	t.Run("multiple hints one blocked by field widening other cached", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Store two cached values
		l.requestScopedL1["key1"] = mustParseArena(t, ar, `{"id":"1"}`)
		l.requestScopedL1["key2"] = mustParseArena(t, ar, `{"x":"y","z":"w"}`)

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "hint1",
					FieldPath:    []string{"hint1"},
					L1Key:        "key1",
					ProvidesData: newViewerObj("id", "name"), // "name" missing from cached value
				},
				{
					FieldName:    "hint2",
					FieldPath:    []string{"hint2"},
					L1Key:        "key2",
					ProvidesData: newViewerObj("x"), // satisfied
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, cfg, items)
		assert.False(t, ok) // Not all hints satisfied

		// With collect-then-inject, items are NOT mutated when any hint fails.
		// Neither hint1 nor hint2 should be injected.
		marshaled := string(items[0].MarshalTo(nil))
		assert.NotContains(t, marshaled, `"hint2"`)
		assert.NotContains(t, marshaled, `"hint1"`)
	})

	t.Run("export then inject round-trip", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Step 1: First fetch exports the value
		exportCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
				},
			},
		}
		exportItems := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"1","currentViewer":{"name":"Alice","role":"admin"}}`),
		}
		l.exportRequestScopedFields(&result{}, exportCfg, exportItems)

		// Step 2: Second fetch attempts injection. ProvidesData describes what
		// the current fetch actually selects under currentViewer — required so
		// the widening check can verify the cached value covers it.
		injectCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("name", "role"),
				},
			},
		}
		injectItems := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
			mustParseArena(t, ar, `{"id":"100"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, injectCfg, injectItems)
		assert.True(t, ok)

		assert.Equal(t, `{"id":"99","currentViewer":{"name":"Alice","role":"admin"}}`, string(injectItems[0].MarshalTo(nil)))
		assert.Equal(t, `{"id":"100","currentViewer":{"name":"Alice","role":"admin"}}`, string(injectItems[1].MarshalTo(nil)))
	})
}

func TestExportedValuesAreIndependentCopies(t *testing.T) {
	t.Parallel()

	t.Run("exported values are structurally independent from source", func(t *testing.T) {
		t.Parallel()

		// Both source and copy live on the same arena (the Loader's jsonArena),
		// which matches the real runtime: exportRequestScopedFields is called
		// from the main thread where items are already on l.jsonArena.
		// StructuralCopy gives structural isolation (mutating the copy's
		// container nodes doesn't affect the source) while aliasing leaf
		// values for efficiency.
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
				},
			},
		}

		sourceData := mustParseArena(t, ar, `{"id":"1","currentViewer":{"id":"v1","name":"Alice"}}`)
		items := []*astjson.Value{sourceData}

		// Export the value
		l.exportRequestScopedFields(&result{}, cfg, items)

		// Verify the value was stored
		cached, ok := l.requestScopedL1["viewer.Personalized.currentViewer"]
		require.True(t, ok)
		assert.Equal(t, `{"id":"v1","name":"Alice"}`, string(cached.MarshalTo(nil)))

		// Mutate the source to verify structural independence.
		sourceData.Get("currentViewer").Set(ar, "name", astjson.StringValue(ar, "Mutated"))

		// The stored value must still produce the original JSON because
		// exportRequestScopedFields creates a structurally independent copy.
		assert.Equal(t, `{"id":"v1","name":"Alice"}`, string(cached.MarshalTo(nil)))

		// Injection using the stored value must succeed with original data.
		// ProvidesData lists the fields the current fetch's selection covers
		// — required for the widening check.
		injectCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("id", "name"),
				},
			},
		}
		injectItems := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"99"}`),
		}
		injected := l.tryRequestScopedInjection(&result{}, injectCfg, injectItems)
		assert.True(t, injected)
		assert.Equal(t, `{"id":"99","currentViewer":{"id":"v1","name":"Alice"}}`, string(injectItems[0].MarshalTo(nil)))
	})

	t.Run("export then inject with multiple items", func(t *testing.T) {
		t.Parallel()

		// Single arena — mirrors real runtime where all values live on l.jsonArena.
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		cfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"currentViewer"},
					L1Key:     "viewer.Personalized.currentViewer",
				},
			},
		}

		sourceItem := mustParseArena(t, ar, `{"id":"1","currentViewer":{"id":"v1","name":"Alice","role":"admin"}}`)
		l.exportRequestScopedFields(&result{}, cfg, []*astjson.Value{sourceItem})

		injectCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        "viewer.Personalized.currentViewer",
					ProvidesData: newViewerObj("id", "name", "role"),
				},
			},
		}
		injectItems := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"entity1"}`),
			mustParseArena(t, ar, `{"id":"entity2"}`),
		}

		ok := l.tryRequestScopedInjection(&result{}, injectCfg, injectItems)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"entity1","currentViewer":{"id":"v1","name":"Alice","role":"admin"}}`, string(injectItems[0].MarshalTo(nil)))
		assert.Equal(t, `{"id":"entity2","currentViewer":{"id":"v1","name":"Alice","role":"admin"}}`, string(injectItems[1].MarshalTo(nil)))
	})
}

// TestRequestScopedAliasHandling verifies that aliasing of the @requestScoped field
// is transparent to the L1 cache: the L1Key is schema-based and the stored value is
// normalized to schema field names, so any alias combination on export and inject
// operates on the same cache entry.
func TestRequestScopedAliasHandling(t *testing.T) {
	t.Parallel()

	const l1Key = "viewer.Personalized.currentViewer"

	t.Run("root uses alias, entity fetch uses schema name", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Root query: { myViewer: currentViewer { id name } }
		// Response shape has the field under the alias "myViewer".
		rootData := mustParseArena(t, ar, `{"myViewer":{"id":"v1","name":"Alice"}}`)
		exportCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"myViewer"}, // response path (alias)
					L1Key:     l1Key,
				},
			},
		}
		l.exportRequestScopedFields(&result{}, exportCfg, []*astjson.Value{rootData})

		// Verify L1 stored the inner object keyed by schema field names
		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"id":"v1","name":"Alice"}`, string(cached.MarshalTo(nil)))

		// Entity fetch uses schema name "currentViewer" (no alias at entity-fetch location)
		injectCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer", // response key at entity-fetch location
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: newViewerObj("id", "name"),
				},
			},
		}
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		injected := l.tryRequestScopedInjection(&result{}, injectCfg, items)
		assert.True(t, injected)
		assert.Equal(t, `{"id":"a1","currentViewer":{"id":"v1","name":"Alice"}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("root no alias, entity fetch uses alias", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Root query: { currentViewer { id name } } — no alias
		rootData := mustParseArena(t, ar, `{"currentViewer":{"id":"v1","name":"Alice"}}`)
		exportCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"currentViewer"},
					L1Key:     l1Key,
				},
			},
		}
		l.exportRequestScopedFields(&result{}, exportCfg, []*astjson.Value{rootData})

		// Entity fetch: { articles { cv: currentViewer { id name } } } — alias "cv"
		injectCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "cv", // alias at entity-fetch location
					FieldPath:    []string{"cv"},
					L1Key:        l1Key,
					ProvidesData: newViewerObj("id", "name"),
				},
			},
		}
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		injected := l.tryRequestScopedInjection(&result{}, injectCfg, items)
		assert.True(t, injected)
		assert.Equal(t, `{"id":"a1","cv":{"id":"v1","name":"Alice"}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("root uses alias A, entity fetch uses alias B", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Root: { myViewer: currentViewer { id name } }
		rootData := mustParseArena(t, ar, `{"myViewer":{"id":"v1","name":"Alice"}}`)
		exportCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath: []string{"myViewer"},
					L1Key:     l1Key,
				},
			},
		}
		l.exportRequestScopedFields(&result{}, exportCfg, []*astjson.Value{rootData})

		// Entity: { articles { cv: currentViewer { id name } } }
		injectCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "cv",
					FieldPath:    []string{"cv"},
					L1Key:        l1Key,
					ProvidesData: newViewerObj("id", "name"),
				},
			},
		}
		items := []*astjson.Value{
			mustParseArena(t, ar, `{"id":"a1"}`),
			mustParseArena(t, ar, `{"id":"a2"}`),
		}
		injected := l.tryRequestScopedInjection(&result{}, injectCfg, items)
		assert.True(t, injected)
		assert.Equal(t, `{"id":"a1","cv":{"id":"v1","name":"Alice"}}`, string(items[0].MarshalTo(nil)))
		assert.Equal(t, `{"id":"a2","cv":{"id":"v1","name":"Alice"}}`, string(items[1].MarshalTo(nil)))
	})

	t.Run("sub-field alias on export is normalized to schema name in L1", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Root: { currentViewer { id displayName: name } }
		// The response has the aliased sub-field "displayName".
		// L1 must store it under the schema name "name" so that a later
		// entity fetch requesting { currentViewer { id name } } finds it.
		rootData := mustParseArena(t, ar, `{"currentViewer":{"id":"v1","displayName":"Alice"}}`)

		// ProvidesData describes the response shape at the export location.
		// Field "displayName" is an alias of schema field "name".
		exportProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("displayName"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(exportProvides)
		require.True(t, exportProvides.HasAliases)

		exportCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: exportProvides,
				},
			},
		}
		l.exportRequestScopedFields(&result{}, exportCfg, []*astjson.Value{rootData})

		// Verify L1 stored the value with schema field names (normalized)
		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"id":"v1","name":"Alice"}`, string(cached.MarshalTo(nil)))

		// Entity fetch requesting { currentViewer { id name } } — uses schema name
		injectCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: newViewerObj("id", "name"),
				},
			},
		}
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		injected := l.tryRequestScopedInjection(&result{}, injectCfg, items)
		assert.True(t, injected)
		assert.Equal(t, `{"id":"a1","currentViewer":{"id":"v1","name":"Alice"}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("sub-field alias on inject re-applies alias from schema-name L1", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// L1 already has the schema-normalized value
		l.requestScopedL1[l1Key] = mustParseArena(t, ar, `{"id":"v1","name":"Alice"}`)

		// Entity fetch: { articles { currentViewer { id displayName: name } } }
		// ProvidesData tells the loader: cached field "name" should appear in
		// the injected value as "displayName".
		injectProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("displayName"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(injectProvides)
		require.True(t, injectProvides.HasAliases)

		injectCfg := FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: injectProvides,
				},
			},
		}
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		injected := l.tryRequestScopedInjection(&result{}, injectCfg, items)
		assert.True(t, injected)
		assert.Equal(t, `{"id":"a1","currentViewer":{"id":"v1","displayName":"Alice"}}`, string(items[0].MarshalTo(nil)))
	})
}

// TestRequestScopedProvidesDataShapes covers Object-tree-based scenarios that the
// old flat RequiredFields / SubFieldAliases API could not express: nested aliases,
// arrays of objects with aliased item fields, arg-variant fields, mixed aliases at
// multiple depths, __typename preservation, and nested null sub-objects.
func TestRequestScopedProvidesDataShapes(t *testing.T) {
	t.Parallel()

	const l1Key = "viewer.Personalized.currentViewer"

	t.Run("nested sub-field alias round-trip", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Query: { currentViewer { profile { displayName: name } } }
		// profile is a nested object; its sub-field "name" is aliased to "displayName".
		profileObj := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("displayName"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		provides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("profile"), Value: profileObj},
			},
		}
		ComputeHasAliases(provides)
		require.True(t, provides.HasAliases)

		// Export: the response has "displayName" under profile — must be
		// normalized to "name" for cache storage.
		rootData := mustParseArena(t, ar, `{"currentViewer":{"profile":{"displayName":"Alice"}}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{FieldPath: []string{"currentViewer"}, L1Key: l1Key, ProvidesData: provides},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"profile":{"name":"Alice"}}`, string(cached.MarshalTo(nil)))

		// Inject: same shape, alias must be re-applied on read.
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: provides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"a1","currentViewer":{"profile":{"displayName":"Alice"}}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("array of objects with aliased item field", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Query: { currentViewer { posts { heading: title } } }
		itemObj := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("heading"), OriginalName: []byte("title"), Value: &Scalar{}},
			},
		}
		postsArr := &Array{Item: itemObj}
		provides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("posts"), Value: postsArr},
			},
		}
		ComputeHasAliases(provides)
		require.True(t, provides.HasAliases)

		rootData := mustParseArena(t, ar, `{"currentViewer":{"posts":[{"heading":"First"},{"heading":"Second"}]}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{FieldPath: []string{"currentViewer"}, L1Key: l1Key, ProvidesData: provides},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"posts":[{"title":"First"},{"title":"Second"}]}`, string(cached.MarshalTo(nil)))

		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: provides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"a1","currentViewer":{"posts":[{"heading":"First"},{"heading":"Second"}]}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("arg-variant sub-field uses hashed field name in cache", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		ctx.ExecutionOptions.Caching.EnableL1Cache = true

		l := &Loader{
			jsonArena:       ar,
			ctx:             ctx,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Query: { currentViewer { posts(first: 5) { id } } }
		// posts has CacheArgs — cache stores the field under "posts_<hash>".
		postsItem := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
			},
		}
		postsField := &Field{
			Name:      []byte("posts"),
			Value:     &Array{Item: postsItem},
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		provides := &Object{
			Nullable: true,
			Fields:   []*Field{postsField},
		}
		ComputeHasAliases(provides)
		require.True(t, provides.HasAliases, "HasAliases must be set for CacheArgs fields")

		rootData := mustParseArena(t, ar, `{"currentViewer":{"posts":[{"id":"p1"},{"id":"p2"}]}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{FieldPath: []string{"currentViewer"}, L1Key: l1Key, ProvidesData: provides},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		suffix := l.computeArgSuffix(postsField.CacheArgs)
		// Under the hood the cache key includes the arg hash suffix.
		assert.Equal(t, `{"posts`+suffix+`":[{"id":"p1"},{"id":"p2"}]}`, string(cached.MarshalTo(nil)))

		// Inject: ProvidesData with the same CacheArgs re-reads the hashed key
		// and writes the response-visible name "posts".
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: provides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"a1","currentViewer":{"posts":[{"id":"p1"},{"id":"p2"}]}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("mixed aliases at multiple depths", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Query:
		//   { currentViewer {
		//       uid: id
		//       prof: profile { label: name }
		//   } }
		profileObj := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("label"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		provides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("uid"), OriginalName: []byte("id"), Value: &Scalar{}},
				{Name: []byte("prof"), OriginalName: []byte("profile"), Value: profileObj},
			},
		}
		ComputeHasAliases(provides)
		require.True(t, provides.HasAliases)

		rootData := mustParseArena(t, ar, `{"currentViewer":{"uid":"v1","prof":{"label":"Alice"}}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{FieldPath: []string{"currentViewer"}, L1Key: l1Key, ProvidesData: provides},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"id":"v1","profile":{"name":"Alice"}}`, string(cached.MarshalTo(nil)))

		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: provides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"a1","currentViewer":{"uid":"v1","prof":{"label":"Alice"}}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("__typename is preserved through export normalization", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Query has an alias sub-field so HasAliases is set, forcing the
		// normalize path that must also preserve __typename.
		provides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("displayName"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(provides)
		require.True(t, provides.HasAliases)

		rootData := mustParseArena(t, ar, `{"currentViewer":{"__typename":"Viewer","displayName":"Alice"}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{FieldPath: []string{"currentViewer"}, L1Key: l1Key, ProvidesData: provides},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `"Viewer"`, string(cached.Get("__typename").MarshalTo(nil)))
		assert.Equal(t, `"Alice"`, string(cached.Get("name").MarshalTo(nil)))
	})

	t.Run("nullable nested object stored as null survives validation", func(t *testing.T) {
		t.Parallel()

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		// Query: { currentViewer { profile { id } } } — profile is nullable.
		profileObj := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
			},
		}
		provides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("profile"), Value: profileObj},
			},
		}
		ComputeHasAliases(provides)

		// Response has profile: null — the nullable nested object must not
		// block validation or cause a panic.
		rootData := mustParseArena(t, ar, `{"currentViewer":{"profile":null}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{FieldPath: []string{"currentViewer"}, L1Key: l1Key, ProvidesData: provides},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"profile":null}`, string(cached.MarshalTo(nil)))

		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: provides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"a1","currentViewer":{"profile":null}}`, string(items[0].MarshalTo(nil)))
	})
}

func TestRequestScopedSyntheticAliasRoundTrip(t *testing.T) {
	t.Parallel()

	const l1Key = "viewer.Personalized.currentViewer"

	t.Run("field conflict round-trip keeps synthetic alias mapping stable across export and injection", func(t *testing.T) {
		t.Parallel()

		// Export under one alias layout, then inject under a conflicting layout.
		// The cache entry must normalize to schema names and denormalize back into the
		// consumer's alias layout without swapping the values.
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		exportProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
				{Name: []byte("__request_scoped__name_0"), OriginalName: []byte("email"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(exportProvides)
		require.True(t, exportProvides.HasAliases)

		// Export writes schema-name-normalized data into requestScoped L1.
		rootData := mustParseArena(t, ar, `{"currentViewer":{"id":"v1","name":"Alice","__request_scoped__name_0":"alice@example.com"}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: exportProvides,
				},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"id":"v1","name":"Alice","email":"alice@example.com"}`, string(cached.MarshalTo(nil)))

		injectProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), OriginalName: []byte("email"), Value: &Scalar{}},
				{Name: []byte("__request_scoped__name_1"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(injectProvides)
		require.True(t, injectProvides.HasAliases)

		// Injection must remap the schema-name entry into the consumer's synthetic aliases.
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: injectProvides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"a1","currentViewer":{"id":"v1","name":"alice@example.com","__request_scoped__name_1":"Alice"}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("argument conflict round-trip keeps synthetic alias mapping and arg-hash normalization aligned", func(t *testing.T) {
		t.Parallel()

		// Export and inject the same field under two argument variants. The L1 entry must
		// normalize to schema-name-plus-arg-suffix keys so each variant survives widening.
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.ExecutionOptions.Caching.EnableL1Cache = true
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"1","b":"2"}`))

		l := &Loader{
			jsonArena:       ar,
			ctx:             ctx,
			requestScopedL1: map[string]*astjson.Value{},
		}

		exportNaturalPosts := &Field{
			Name:      []byte("posts"),
			Value:     &Array{Item: &Object{Nullable: true, Fields: []*Field{{Name: []byte("id"), Value: &Scalar{}}}}},
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		exportSyntheticPosts := &Field{
			Name:         []byte("__request_scoped__posts_1"),
			OriginalName: []byte("posts"),
			Value: &Array{Item: &Object{Nullable: true, Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("title"), Value: &Scalar{}},
			}}},
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariableName: "b"}},
		}
		exportProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				exportNaturalPosts,
				exportSyntheticPosts,
			},
		}
		ComputeHasAliases(exportProvides)
		require.True(t, exportProvides.HasAliases)

		// Export writes both argument variants into requestScoped L1 under their normalized keys.
		rootData := mustParseArena(t, ar, `{"currentViewer":{"id":"v1","posts":[{"id":"p1"}],"__request_scoped__posts_1":[{"id":"p2","title":"Second"}]}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: exportProvides,
				},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t,
			`{"id":"v1","posts`+l.computeArgSuffix(exportNaturalPosts.CacheArgs)+`":[{"id":"p1"}],"posts`+l.computeArgSuffix(exportSyntheticPosts.CacheArgs)+`":[{"id":"p2","title":"Second"}]}`,
			string(cached.MarshalTo(nil)),
		)

		injectNaturalPosts := &Field{
			Name:         []byte("posts"),
			OriginalName: nil,
			Value: &Array{Item: &Object{Nullable: true, Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("title"), Value: &Scalar{}},
			}}},
			CacheArgs: []CacheFieldArg{{ArgName: "first", VariableName: "b"}},
		}
		injectSyntheticPosts := &Field{
			Name:         []byte("__request_scoped__posts_0"),
			OriginalName: []byte("posts"),
			Value:        &Array{Item: &Object{Nullable: true, Fields: []*Field{{Name: []byte("id"), Value: &Scalar{}}}}},
			CacheArgs:    []CacheFieldArg{{ArgName: "first", VariableName: "a"}},
		}
		injectProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				injectNaturalPosts,
				injectSyntheticPosts,
			},
		}
		ComputeHasAliases(injectProvides)
		require.True(t, injectProvides.HasAliases)

		// Injection must reconstruct the caller's argument layout from the normalized cache entry.
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: injectProvides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"a1","currentViewer":{"id":"v1","posts":[{"id":"p2","title":"Second"}],"__request_scoped__posts_0":[{"id":"p1"}]}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("three conflicting field variants round-trip through schema-name storage and synthetic alias remapping", func(t *testing.T) {
		t.Parallel()

		// Three participants map different schema fields into the same response position.
		// Export must keep the schema fields distinct, and injection must rebuild the
		// consumer-specific alias layout from that shared cache entry.
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		exportProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
				{Name: []byte("__request_scoped__name_0"), OriginalName: []byte("email"), Value: &Scalar{}},
				{Name: []byte("__request_scoped__name_1"), OriginalName: []byte("handle"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(exportProvides)
		require.True(t, exportProvides.HasAliases)

		// Export writes the shared schema-name view into requestScoped L1.
		rootData := mustParseArena(t, ar, `{"currentViewer":{"id":"v1","name":"Alice","__request_scoped__name_0":"alice@example.com","__request_scoped__name_1":"alice-handle"}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: exportProvides,
				},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"id":"v1","name":"Alice","email":"alice@example.com","handle":"alice-handle"}`, string(cached.MarshalTo(nil)))

		injectProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), OriginalName: []byte("handle"), Value: &Scalar{}},
				{Name: []byte("__request_scoped__name_0"), OriginalName: []byte("email"), Value: &Scalar{}},
				{Name: []byte("__request_scoped__name_2"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(injectProvides)
		require.True(t, injectProvides.HasAliases)

		// Injection remaps that shared entry into a different alias layout for the consumer.
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"r1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: injectProvides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"r1","currentViewer":{"id":"v1","name":"alice-handle","__request_scoped__name_0":"alice@example.com","__request_scoped__name_2":"Alice"}}`, string(items[0].MarshalTo(nil)))
	})

	t.Run("hidden requires dependency round-trips from an aliased root participant into the entity participant", func(t *testing.T) {
		t.Parallel()

		const l1Key = "viewer.currentViewer"

		// The root participant exports name under a user alias, while the entity participant
		// later needs the schema field name for a hidden @requires dependency.
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		l := &Loader{
			jsonArena:       ar,
			requestScopedL1: map[string]*astjson.Value{},
		}

		exportProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("viewerName"), OriginalName: []byte("name"), Value: &Scalar{}},
				{Name: []byte("__typename"), Value: &Scalar{}},
				{Name: []byte("id"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(exportProvides)
		require.True(t, exportProvides.HasAliases)

		// Export must normalize the aliased root field back to the schema field name.
		rootData := mustParseArena(t, ar, `{"currentViewer":{"viewerName":"Alice","__typename":"Viewer","id":"v1"}}`)
		l.exportRequestScopedFields(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: exportProvides,
				},
			},
		}, []*astjson.Value{rootData})

		cached, ok := l.requestScopedL1[l1Key]
		require.True(t, ok)
		assert.Equal(t, `{"name":"Alice","__typename":"Viewer","id":"v1"}`, string(cached.MarshalTo(nil)))

		injectProvides := &Object{
			Nullable: true,
			Fields: []*Field{
				{Name: []byte("name"), Value: &Scalar{}},
				{Name: []byte("__typename"), Value: &Scalar{}},
				{Name: []byte("id"), Value: &Scalar{}},
			},
		}
		ComputeHasAliases(injectProvides)
		require.False(t, injectProvides.HasAliases)

		// Injection into the entity participant must supply the hidden dependency fields
		// exactly as the downstream subgraph expects them.
		items := []*astjson.Value{mustParseArena(t, ar, `{"id":"a1"}`)}
		ok = l.tryRequestScopedInjection(&result{}, FetchCacheConfiguration{
			RequestScopedFields: []RequestScopedField{
				{
					FieldName:    "currentViewer",
					FieldPath:    []string{"currentViewer"},
					L1Key:        l1Key,
					ProvidesData: injectProvides,
				},
			},
		}, items)
		assert.True(t, ok)
		assert.Equal(t, `{"id":"a1","currentViewer":{"name":"Alice","__typename":"Viewer","id":"v1"}}`, string(items[0].MarshalTo(nil)))
	})
}
