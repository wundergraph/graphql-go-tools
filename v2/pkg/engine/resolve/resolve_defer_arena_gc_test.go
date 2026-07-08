package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// benchDeferWriter is a minimal DeferResponseWriter for benchmarks: it buffers
// each frame and discards it on Flush, summing only the byte count so the
// measured allocations belong to the resolve path, not to frame retention.
//
// No mutex is needed: every Write/Flush during deferred resolution happens
// either single-threaded (initial frame) or under the shared DataBuffer lock
// (per-group frames), so calls are serialised by construction.
type benchDeferWriter struct {
	buf   []byte
	bytes int
}

func (w *benchDeferWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *benchDeferWriter) Flush() error {
	w.bytes += len(w.buf)
	w.buf = w.buf[:0]
	return nil
}

func (w *benchDeferWriter) Complete() {}

// benchDeferValue is a representative scalar payload (~48 bytes) so each field
// carries real parse/merge/render work rather than a trivial token.
const benchDeferValue = "the quick brown fox jumps over the lazy dog!!!!!"

// benchGroupPayload renders a subgraph response of shape
// {"data":{"<fieldName>":{"f0":"…","f1":"…",…}}} with fieldCount string fields.
func benchGroupPayload(fieldName string, fieldCount int) string {
	var b strings.Builder
	b.WriteString(`{"data":{"`)
	b.WriteString(fieldName)
	b.WriteString(`":{`)
	for i := range fieldCount {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"f%d":%q`, i, benchDeferValue)
	}
	b.WriteString(`}}}`)
	return b.String()
}

// benchDeferResponse builds a deferred operation with groupCount independent
// top-level @defer fragments, each fetching an object with fieldCount string
// fields. The fragments are independent, so they execute as a DeferParallel and
// every group's parse + merge + render runs concurrently — the load that an
// arena would have to allocate under.
//
// Operation shape (groupCount fragments):
//
//	{ ... @defer { g1 { f0 … fK } }  ... @defer { g2 { f0 … fK } } … }
func benchDeferResponse(groupCount, fieldCount int) *GraphQLDeferResponse {
	topFields := make([]*Field, groupCount)
	descriptors := make(map[int]DeferDescriptor, groupCount)
	leaves := make([]*DeferTreeNode, groupCount)

	for g := range groupCount {
		id := g + 1
		fieldName := fmt.Sprintf("g%d", id)

		// Every field inside the deferred selection set carries the fragment's
		// defer id — the normalizer stamps it onto the whole selection set, so the
		// deferred render only emits fields tagged with the current defer id.
		objFields := make([]*Field, fieldCount)
		for i := range fieldCount {
			objFields[i] = &Field{
				Name:  fmt.Appendf(nil, "f%d", i),
				Defer: &DeferField{DeferID: id},
				Value: &String{Path: []string{fmt.Sprintf("f%d", i)}, Nullable: true},
			}
		}

		group := &DeferFetchGroup{
			DeferID: id,
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(benchGroupPayload(fieldName, fieldCount)),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
		}

		topFields[g] = deferredField(fieldName, id, &Object{
			Path:     []string{fieldName},
			Nullable: true,
			Fields:   objFields,
		}, nil)
		descriptors[id] = DeferDescriptor{ID: id, ParentID: 0}
		leaves[g] = DeferSingle(group)
	}

	return &GraphQLDeferResponse{
		DeferDescriptors: descriptors,
		DeferTree:        DeferParallel(leaves...),
		Response: &GraphQLResponse{
			Info: deferQueryInfo(),
			Data: &Object{Nullable: true, Fields: topFields},
		},
	}
}

// Benchmark_DeferResponse measures the deferred-delivery resolve path under
// increasing fan-out and payload size. Run with -benchmem to read allocs/op and
// B/op — the figures to compare before and after wiring an arena into the defer
// path.
func Benchmark_DeferResponse(b *testing.B) {
	cases := []struct {
		name       string
		groupCount int
		fieldCount int
	}{
		{"4groups_8fields", 4, 8},
		{"16groups_8fields", 16, 8},
		{"16groups_32fields", 16, 32},
		{"64groups_16fields", 64, 16},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			rCtx := b.Context()
			resolver := New(rCtx, baseResolverOpts())
			response := benchDeferResponse(tc.groupCount, tc.fieldCount)
			w := &benchDeferWriter{}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.bytes = 0
				w.buf = w.buf[:0]
				ctx := NewContext(context.Background())
				_, err := resolver.ResolveGraphQLDeferResponse(ctx, response, w)
				if err != nil {
					b.Fatal(err)
				}
				if w.bytes == 0 {
					b.Fatal("empty output")
				}
			}
		})
	}
}

// resolveDeferWithGCPressure runs ResolveGraphQLDeferResponse in a loop, forcing
// GC between iterations to maximise pressure on any heap pointer stored inside an
// arena allocation (arena buffers are noscan — such a pointer is invisible to the
// GC, so if it is the only reference the object is collected and a later read
// SIGSEGVs or returns corruption). Returns the frames flushed on the last
// iteration. Mirrors resolveWithGCPressure for the deferred path.
func resolveDeferWithGCPressure(t *testing.T, resolver *Resolver, setupResp func() *GraphQLDeferResponse) []string {
	t.Helper()
	var last []string
	for i := range gcIterations {
		response := setupResp()
		w := &testDeferWriter{}
		forceGC()
		_, err := resolver.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
		require.NoError(t, err)
		require.NotEmpty(t, w.payloads, "no frames flushed on iteration %d", i)
		require.True(t, w.complete, "stream not completed on iteration %d", i)
		last = w.payloads
	}
	return last
}

// TestDeferGCSafety_SingleGroup asserts the full two-frame payload of a single
// deferred fragment survives repeated GC. One group keeps frame order
// deterministic so the whole output can be asserted exactly.
func TestDeferGCSafety_SingleGroup(t *testing.T) {
	resolver := newTestResolver(t, baseResolverOpts())
	frames := resolveDeferWithGCPressure(t, resolver, func() *GraphQLDeferResponse {
		group := &DeferFetchGroup{
			DeferID: 1,
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"obj":{"a":"alpha","b":"bravo"}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
			}),
		}
		return &GraphQLDeferResponse{
			DeferDescriptors: map[int]DeferDescriptor{1: {ID: 1, ParentID: 0}},
			DeferTree:        DeferSingle(group),
			Response: &GraphQLResponse{
				Info: deferQueryInfo(),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						deferredField("obj", 1, &Object{
							Path:     []string{"obj"},
							Nullable: true,
							Fields: []*Field{
								{Name: []byte("a"), Defer: &DeferField{DeferID: 1}, Value: &String{Path: []string{"a"}, Nullable: true}},
								{Name: []byte("b"), Defer: &DeferField{DeferID: 1}, Value: &String{Path: []string{"b"}, Nullable: true}},
							},
						}, nil),
					},
				},
			},
		}
	})
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"incremental":[{"data":{"obj":{"a":"alpha","b":"bravo"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, frames)
}

// TestDeferGCSafety_ParallelGroups runs many concurrent deferred groups under GC
// pressure. Frame order across parallel groups is nondeterministic, so the
// assertions are order-independent: every frame is valid JSON, each group's data
// appears exactly once, exactly one frame closes the stream, and the initial
// frame announces all groups as pending.
func TestDeferGCSafety_ParallelGroups(t *testing.T) {
	const groupCount, fieldCount = 8, 6
	resolver := newTestResolver(t, baseResolverOpts())
	frames := resolveDeferWithGCPressure(t, resolver, func() *GraphQLDeferResponse {
		return benchDeferResponse(groupCount, fieldCount)
	})

	// Every flushed frame must be valid JSON (no corruption from a collected
	// arena pointer).
	for i, f := range frames {
		require.True(t, json.Valid([]byte(f)), "frame %d is not valid JSON: %s", i, f)
	}

	joined := strings.Join(frames, "\n")

	// Initial frame announces all groups; each group's value is delivered once.
	for g := 1; g <= groupCount; g++ {
		marker := fmt.Sprintf(`"id":"%d"`, g)
		// Each id appears 3 times: once as a pending entry (initial frame), then
		// in its incremental item and its completed entry (delivery frame).
		assert.Equalf(t, 3, strings.Count(joined, marker),
			"group %d must be announced once and delivered once", g)
		assert.Containsf(t, joined, fmt.Sprintf(`"g%d":{"f0":`, g),
			"group %d data must be delivered", g)
	}

	// Exactly one frame ends the stream.
	assert.Equal(t, 1, strings.Count(joined, `"hasNext":false`), "exactly one terminal frame")
	assert.Equal(t, groupCount, strings.Count(joined, `"hasNext":true`), "one pending/continuation frame per non-terminal frame")
}

// TestDeferGCSafety_ErrorFrames keeps error-carrying arena values alive across
// GC: one group delivers data plus a recoverable error, another fully
// null-bubbles into a completed-error frame.
func TestDeferGCSafety_ErrorFrames(t *testing.T) {
	opts := baseResolverOpts()
	opts.SubgraphErrorPropagationMode = SubgraphErrorPropagationModePassThrough
	resolver := newTestResolver(t, opts)

	frames := resolveDeferWithGCPressure(t, resolver, func() *GraphQLDeferResponse {
		recoverable := &DeferFetchGroup{
			DeferID: 1,
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"f1":"hello"},"errors":[{"message":"partial failure"}]}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
			}),
		}
		nullBubble := simpleGroup(2, `{}`)
		return &GraphQLDeferResponse{
			DeferDescriptors: map[int]DeferDescriptor{1: {ID: 1}, 2: {ID: 2}},
			DeferTree:        DeferParallel(DeferSingle(recoverable), DeferSingle(nullBubble)),
			Response: &GraphQLResponse{
				Info: deferQueryInfo(),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						deferredField("f1", 1, &String{Path: []string{"f1"}, Nullable: true}, nil),
						deferredField("f2", 2, &String{Path: []string{"f2"}, Nullable: false}, nil),
					},
				},
			},
		}
	})

	for i, f := range frames {
		require.True(t, json.Valid([]byte(f)), "frame %d is not valid JSON: %s", i, f)
	}
	joined := strings.Join(frames, "\n")
	assert.Contains(t, joined, "partial failure", "recoverable error must survive GC")
	assert.Contains(t, joined, "Cannot return null for non-nullable field 'Query.f2'.", "null-bubble error must survive GC")
}
