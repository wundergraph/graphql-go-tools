package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// This file documents how @defer interacts with errors, mirroring the reference
// graphql-js incremental-delivery suite
// (graphql-js/src/execution/incremental/__tests__/defer-test.ts). Each test names
// both the equivalent GraphQL @defer operation it represents and the graphql-js
// analog it is derived from, and asserts the FULL multipart payload sequence
// (one string per flushed frame) so the exact wire shape is visible and doubles
// as documentation of each case.

// ---- test doubles ---------------------------------------------------------

// deferTestAuthorizer simulates the authorizer during the deferred render walk:
// errOnField yields a hard Go error (F04), denyOnField yields an AuthorizationDeny
// (the common production path — rendered as a rejected-field error).
type deferTestAuthorizer struct {
	errOnField  string
	denyOnField string
}

func (a *deferTestAuthorizer) AuthorizePreFetch(_ *Context, _ string, _ json.RawMessage, _ GraphCoordinate) (*AuthorizationDeny, error) {
	return nil, nil
}

func (a *deferTestAuthorizer) AuthorizeObjectField(_ *Context, _ string, _ json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
	if coordinate.FieldName == a.errOnField {
		return nil, errors.New("authorizer hard error on " + coordinate.FieldName)
	}
	if coordinate.FieldName == a.denyOnField {
		return &AuthorizationDeny{Reason: "missing scope"}, nil
	}
	return nil, nil
}

func (a *deferTestAuthorizer) HasResponseExtensionData(_ *Context) bool { return false }

func (a *deferTestAuthorizer) RenderResponseExtension(_ *Context, _ io.Writer) error { return nil }

// deferTestFieldRenderer returns an error when rendering a chosen field value,
// simulating a custom field-value renderer failure during the deferred render
// walk (F05). Any other field is written through unchanged.
type deferTestFieldRenderer struct {
	errOnField string
}

func (r *deferTestFieldRenderer) RenderFieldValue(_ *Context, value FieldValue, out io.Writer) error {
	if value.Name == r.errOnField {
		return errors.New("field renderer error on " + value.Name)
	}
	_, err := out.Write(value.Data)
	return err
}

// deferTestRateLimiter returns a hard error from a deferred group's pre-fetch.
type deferTestRateLimiter struct {
	errOnDataSourceID string
}

func (l *deferTestRateLimiter) RateLimitPreFetch(_ *Context, info *FetchInfo, _ json.RawMessage) (*RateLimitDeny, error) {
	if info != nil && info.DataSourceID == l.errOnDataSourceID {
		return nil, errors.New("rate limiter hard error on " + info.DataSourceID)
	}
	return nil, nil
}

func (l *deferTestRateLimiter) RenderResponseExtension(_ *Context, _ io.Writer) error { return nil }

// ---- builders -------------------------------------------------------------

func deferQueryInfo() *GraphQLResponseInfo {
	return &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery}
}

// simpleFetch returns a fetch tree whose single fetch yields fetchJSON verbatim.
func simpleFetch(fetchJSON string) *FetchTreeNode {
	return Single(&SingleFetch{
		FetchConfiguration: FetchConfiguration{
			DataSource: FakeDataSource(fetchJSON),
		},
	})
}

// simpleGroup returns a deferred group whose fetch yields fetchJSON verbatim.
func simpleGroup(deferID int, fetchJSON string) *DeferFetchGroup {
	return &DeferFetchGroup{
		DeferID: deferID,
		Fetches: simpleFetch(fetchJSON),
	}
}

// deferredField builds a deferred field with the given name and defer id.
func deferredField(name string, deferID int, value Node, info *FieldInfo) *Field {
	return &Field{
		Name:  []byte(name),
		Defer: &DeferField{DeferID: deferID},
		Value: value,
		Info:  info,
	}
}

// rootDeferResponse builds a response whose single root-level deferred field
// (id 1, path []) carries the given Value/Info and is fetched by the group. It
// represents the operation:
//
//	{ ... @defer { f1 } }
func rootDeferResponse(deferredValue Node, deferredInfo *FieldInfo, group *DeferFetchGroup) *GraphQLDeferResponse {
	return &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, ParentID: 0, Path: nil},
		},
		DeferTree: DeferSingle(group),
		Response: &GraphQLResponse{
			Info: deferQueryInfo(),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					deferredField("f1", 1, deferredValue, deferredInfo),
				},
			},
		},
	}
}

// ---- error in incremental -------------------------------------------------

// TestDefer_ErrorInIncremental: a deferred fragment that still has deliverable
// data but also carries a recoverable (subgraph) error renders the error inside
// the incremental item (incremental[].errors), with a bare completed entry.
//
// Operation:
//
//	{ ... @defer { f1 } }   # f1: String — subgraph returns f1 AND a recoverable error
//
// graphql-js analog: "Cancels deferred fields when deferred result exhibits null
// bubbling" (incremental{data,errors} + completed).
func TestDefer_ErrorInIncremental(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := &DeferFetchGroup{
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
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, nil, group)

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"incremental":[{"data":{"f1":"hello"},"id":"1","errors":[{"message":"Failed to fetch from Subgraph.","extensions":{"errors":[{"message":"partial failure"}]}}]}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// TestDefer_ErrorWithoutDeliverableIncrementalData: the initial fetch resolves
// a nullable entity object, then the deferred entity fetch returns a null entity
// plus an error. The non-null deferred field null-propagates to the already
// delivered nullable object, so the deferred render has no incremental item.
// The announced pending still has to complete, and the error must remain
// observable on completed[].errors instead of being dropped beside an empty
// incremental array.
//
// Operation:
//
//	employee(id: 1) {
//	  id
//	  ... @defer(label: "Mood") { currentMood }
//	}
func TestDefer_ErrorWithoutDeliverableIncrementalData(t *testing.T) {
	t.Parallel()
	r := New(t.Context(), ResolverOptions{
		MaxConcurrency:               32,
		PropagateSubgraphErrors:      true,
		SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
		AllowedErrorExtensionFields:  []string{"code", "detail"},
	})

	group := &DeferFetchGroup{
		DeferID: 1,
		Fetches: SingleWithPath(&EntityFetch{
			FetchDependencies: FetchDependencies{FetchID: 2, DeferID: 1},
			Input: EntityInput{
				Header: InputTemplate{Segments: []TemplateSegment{{
					SegmentType: StaticSegmentType,
					Data:        []byte(`{"body":{"variables":{"representations":[`),
				}}},
				Item: InputTemplate{Segments: []TemplateSegment{{
					SegmentType: StaticSegmentType,
					Data:        []byte(`{"__typename":"Employee","id":1}`),
				}}},
				Footer: InputTemplate{Segments: []TemplateSegment{{
					SegmentType: StaticSegmentType,
					Data:        []byte(`]}}}`),
				}}},
				SkipErrItem: true,
			},
			DataSource: FakeDataSource(`{"data":{"_entities":[null]},"errors":[{"message":"deferred mood failed","path":["_entities",0,"currentMood"],"extensions":{"code":"MOOD_FAIL","detail":"deferred"}}]}`),
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath:   []string{"data", "_entities", "0"},
				SelectResponseErrorsPath: []string{"errors"},
			},
			Info: &FetchInfo{
				DataSourceID:   "mood",
				DataSourceName: "mood",
				OperationType:  ast.OperationTypeQuery,
			},
		}, "employee", ObjectPath("employee")),
	}
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Label: "Mood", Path: []string{"employee"}},
		},
		DeferTree: DeferSingle(group),
		Response: &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: FakeDataSource(`{"data":{"employee":{"id":1,"__typename":"Employee"}}}`),
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				FetchDependencies: FetchDependencies{FetchID: 1},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{{
					SegmentType: StaticSegmentType,
					Data:        []byte(`{}`),
				}}},
				Info: &FetchInfo{
					DataSourceID:   "employees",
					DataSourceName: "employees",
					OperationType:  ast.OperationTypeQuery,
				},
			}),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{{
					Name: []byte("employee"),
					Value: &Object{
						Path:     []string{"employee"},
						Nullable: true,
						TypeName: "Employee",
						Fields: []*Field{
							{Name: []byte("id"), Value: &Integer{Path: []string{"id"}, Nullable: false}},
							deferredField("currentMood", 1, &String{Path: []string{"currentMood"}, Nullable: false}, nil),
						},
					},
				}},
			},
		},
	}

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(deferExtensionContext(), response, w)
	require.NoError(t, err)
	require.Len(t, w.payloads, 2)

	initial := decodeDeferPayload(t, w.payloads[0])
	require.Equal(t, map[string]any{"employee": map[string]any{"id": float64(1)}}, initial["data"])

	terminal := decodeDeferPayload(t, w.payloads[1])
	require.Equal(t, false, terminal["hasNext"])
	require.NotContains(t, terminal, "incremental")
	completed := terminal["completed"].([]any)
	require.Len(t, completed, 1)
	completedErrors := completed[0].(map[string]any)["errors"].([]any)
	require.NotEmpty(t, completedErrors)
	require.Contains(t, completedErrors, map[string]any{
		"message": "deferred mood failed",
		"path":    []any{"_entities", float64(0), "currentMood"},
		"extensions": map[string]any{
			"code":   "MOOD_FAIL",
			"detail": "deferred",
		},
	})
	require.Equal(t, map[int]string{1: string(DeferExecutionStatusError)}, deferStatuses(t, terminal["extensions"].(map[string]any)["trace"]))
	require.True(t, w.complete)
}

func TestDefer_ErrorWithoutIncrementalItemSkipsNestedDefers(t *testing.T) {
	t.Parallel()
	r := New(t.Context(), ResolverOptions{
		MaxConcurrency:               32,
		PropagateSubgraphErrors:      true,
		SubgraphErrorPropagationMode: SubgraphErrorPropagationModePassThrough,
	})

	parent := &DeferFetchGroup{
		DeferID: 1,
		Fetches: deferExtensionFetch(2, "parent", `{"data":{},"errors":[{"message":"parent failed"}]}`),
	}
	child := &DeferFetchGroup{
		DeferID: 2,
		Fetches: deferExtensionFetch(3, "child", `{"data":{"child":"must not run"}}`),
	}
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Label: "Parent"},
			2: {ID: 2, ParentID: 1, Label: "Child"},
		},
		DeferTree: DeferSequence(DeferSingle(parent), DeferSingle(child)),
		Response: &GraphQLResponse{
			Info:    &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: deferExtensionFetch(1, "primary", `{"data":{"__typename":"Query"}}`),
			Data: &Object{Nullable: true, Fields: []*Field{
				{
					Name:        []byte("parent"),
					Defer:       &DeferField{DeferID: 1},
					OnTypeNames: [][]byte{[]byte("OtherQuery")},
					Value:       &String{Path: []string{"parent"}, Nullable: true},
				},
				deferredField("child", 2, &String{Path: []string{"child"}, Nullable: true}, nil),
			}},
		},
	}

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(deferExtensionContext(), response, w)
	require.NoError(t, err)
	require.Len(t, w.payloads, 2, "a defer without an incremental item cannot release nested work")

	terminal := decodeDeferPayload(t, w.payloads[1])
	require.NotContains(t, terminal, "incremental")
	require.NotContains(t, terminal, "pending")
	require.Equal(t, map[int]string{
		1: string(DeferExecutionStatusError),
		2: string(DeferExecutionStatusSkipped),
	}, deferStatuses(t, terminal["extensions"].(map[string]any)["trace"]))
}

// ---- error in completed ---------------------------------------------------

// TestDefer_ErrorInCompleted: when the deferred fragment's data fully null-bubbles
// (a non-null field comes back null) there is no deliverable incremental data, so
// the error is reported on the completed entry (completed[].errors), no incremental.
//
// Operation:
//
//	{ ... @defer { f1 } }   # f1: String! — subgraph returns null -> null-bubbles
//
// graphql-js analog: "Handles multiple erroring deferred grouped field sets".
func TestDefer_ErrorInCompleted(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := simpleGroup(1, `{}`)
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: false}, nil, group)

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"completed":[{"id":"1","errors":[{"message":"Cannot return null for non-nullable field 'Query.f1'.","path":["f1"]}]}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// TestDefer_MultipleErroringGroups_AllCompleted: two independent top-level defers
// that both fully null-bubble each report their error on their own completed entry.
//
// Operation:
//
//	{ ... @defer { f1 }  ... @defer { f2 } }   # f1: String!, f2: String!
//
// They are independent, so the execution tree is a DeferParallel and the two
// completion frames may arrive in either order; assertions are order-independent.
//
// graphql-js analog: "Handles multiple erroring deferred grouped field sets".
func TestDefer_MultipleErroringGroups_AllCompleted(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	groupA := simpleGroup(1, `{}`)
	groupB := simpleGroup(2, `{}`)
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1},
			2: {ID: 2},
		},
		DeferTree: DeferParallel(DeferSingle(groupA), DeferSingle(groupB)),
		Response: &GraphQLResponse{
			Info: deferQueryInfo(),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					deferredField("f1", 1, &String{Path: []string{"f1"}, Nullable: false}, nil),
					deferredField("f2", 2, &String{Path: []string{"f2"}, Nullable: false}, nil),
				},
			},
		},
	}

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Len(t, w.payloads, 3)

	// Initial frame announces both top-level defers (sorted by id).
	require.Equal(t,
		`{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":[]}],"hasNext":true}`,
		w.payloads[0])

	// Each defer reports its non-null error on its own completed entry; order of
	// the two completion frames is non-deterministic.
	rest := strings.Join(w.payloads[1:], "\n")
	assert.Contains(t, rest, `"completed":[{"id":"1","errors":[{"message":"Cannot return null for non-nullable field 'Query.f1'.","path":["f1"]}]}]`)
	assert.Contains(t, rest, `"completed":[{"id":"2","errors":[{"message":"Cannot return null for non-nullable field 'Query.f2'.","path":["f2"]}]}]`)
	assert.Equal(t, 1, strings.Count(rest, `"hasNext":false`))
	require.True(t, w.complete)
}

// ---- error in root: nulls the defer's own anchor --------------------------

// TestDefer_InitialErrorNullsAnchor_DeferCancelled: a recoverable error in the
// initial response that null-bubbles up to the defer's OWN anchor (`user`) cancels
// the defer — it is never announced — and the stream terminates cleanly in the
// initial frame.
//
// Operation:
//
//	{
//	  user {                    # user: User (nullable)
//	    name                    # name: String! -> returns null -> nulls `user`
//	    ... @defer { articles }
//	  }
//	}
//
// graphql-js analog: "Cancels deferred fields when initial result exhibits null
// bubbling cancelling the defer" (data:{user:null}, no pending/completed).
func TestDefer_InitialErrorNullsAnchor_DeferCancelled(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := simpleGroup(1, `{"articles":"x"}`)
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: []string{"user"}},
		},
		DeferTree: DeferSingle(group),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"user":{"name":null}}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"user"},
							Fields: []*Field{
								// non-null name comes back null -> nulls `user`, the defer anchor.
								{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: false}},
								deferredField("articles", 1, &String{Path: []string{"articles"}, Nullable: true}, nil),
							},
						},
					},
				},
			},
		},
	}

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":{"user":null},"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// TestDefer_NestedChildCancelledWithDeadParent: a nested @defer rides with its
// top-level ancestor — when the ancestor's anchor null-propagates, both the
// parent and the nested child are cancelled (neither announced nor delivered) and
// the stream terminates cleanly in the initial frame.
//
// Operation:
//
//	{
//	  user {                       # user: User (nullable)
//	    boom                       # String! -> null -> nulls `user`
//	    ... @defer {               # defer 1 (parent), anchor ["user"]
//	      p { ... @defer { c } }   # defer 2 (nested child), ParentID 1
//	    }
//	  }
//	}
func TestDefer_NestedChildCancelledWithDeadParent(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: []string{"user"}},
			2: {ID: 2, Path: []string{"user", "p"}, ParentID: 1},
		},
		DeferTree: DeferSequence(
			DeferSingle(simpleGroup(1, `{}`)),
			DeferSingle(simpleGroup(2, `{}`)),
		),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"user":{"boom":null}}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"user"},
							Fields: []*Field{
								{Name: []byte("boom"), Value: &String{Path: []string{"boom"}, Nullable: false}},
								{
									Name:  []byte("p"),
									Defer: &DeferField{DeferID: 1},
									Value: &Object{
										Nullable: true,
										Path:     []string{"p"},
										Fields: []*Field{
											{Name: []byte("c"), Defer: &DeferField{DeferID: 2}, Value: &String{Path: []string{"c"}, Nullable: true}},
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

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.boom'.","path":["user","boom"]}],"data":{"user":null},"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// ---- error on a different root path: anchor survives ----------------------

// TestDefer_ErrorOnDifferentRootPath_DeferStillDelivered: a recoverable error on
// an UNRELATED path must not drop a defer whose anchor survived. The defer must be
// announced, delivered, completed, and the stream terminated.
//
// Operation:
//
//	{
//	  erroring { name }     # erroring: E (nullable), name: String! -> nulls `erroring` (bounded)
//	  safe                  # survives
//	  ... @defer { f1 }     # anchored at the root object, which survives
//	}
//
// graphql-js analog: "Keeps deferred work outside nulled error paths".
func TestDefer_ErrorOnDifferentRootPath_DeferStillDelivered(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := simpleGroup(1, `{"f1":"deferred"}`)
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: nil},
		},
		DeferTree: DeferSingle(group),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"erroring":{"name":null},"safe":"ok"}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("erroring"),
						Value: &Object{
							Nullable: true, // nullable -> the error is bounded here, root survives
							Path:     []string{"erroring"},
							Fields: []*Field{
								{Name: []byte("name"), Value: &String{Path: []string{"name"}, Nullable: false}},
							},
						},
					},
					{Name: []byte("safe"), Value: &String{Path: []string{"safe"}, Nullable: true}},
					deferredField("f1", 1, &String{Path: []string{"f1"}, Nullable: true}, nil),
				},
			},
		},
	}

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.erroring.name'.","path":["erroring","name"]}],"data":{"erroring":null,"safe":"ok"},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"incremental":[{"data":{"f1":"deferred"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// ---- independent top-level defers: per-anchor gating ----------------------

// TestDefer_TwoRootDefers_IndependentGating: two independent top-level defers on
// separate root fields are gated independently. When one field's anchor
// null-propagates, only that defer is cancelled; the sibling defer on a surviving
// field is still announced and delivered.
//
// Operation:
//
//	{
//	  a { boom ... @defer { x } }   # boom: String! -> null -> nulls `a`, cancels defer 1
//	  b { id   ... @defer { y } }   # `b` survives -> defer 2 delivered
//	}
//
// (The deferred values ride in the initial fetch; each group fetch is a no-op, so
// the defer just renders the already-present fields.)
func TestDefer_TwoRootDefers_IndependentGating(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: []string{"a"}},
			2: {ID: 2, Path: []string{"b"}},
		},
		DeferTree: DeferParallel(
			DeferSingle(simpleGroup(1, `{}`)),
			DeferSingle(simpleGroup(2, `{}`)),
		),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"a":{"boom":null,"x":"xval"},"b":{"id":"b1","y":"yval"}}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("a"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"a"},
							Fields: []*Field{
								// non-null boom comes back null -> nulls anchor `a`.
								{Name: []byte("boom"), Value: &String{Path: []string{"boom"}, Nullable: false}},
								deferredField("x", 1, &String{Path: []string{"x"}, Nullable: true}, nil),
							},
						},
					},
					{
						Name: []byte("b"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"b"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: true}},
								deferredField("y", 2, &String{Path: []string{"y"}, Nullable: true}, nil),
							},
						},
					},
				},
			},
		},
	}

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.a.boom'.","path":["a","boom"]}],"data":{"a":null,"b":{"id":"b1"}},"pending":[{"id":"2","path":["b"]}],"hasNext":true}`,
		`{"incremental":[{"data":{"y":"yval"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// ---- defer whose content is an empty list ---------------------------------

// TestDefer_EmptyListInDeferredFragment: a defer whose content resolves to an
// empty list is alive (its anchor is present) and is delivered as an empty array.
// Only null/absent anchors are cancelled — an empty result is real data.
//
// Operation:
//
//	{ ... @defer { f1 } }   # f1: [Item!]! resolves to []
func TestDefer_EmptyListInDeferredFragment(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := simpleGroup(1, `{"f1":[]}`)
	response := rootDeferResponse(
		&Array{
			Path:     []string{"f1"},
			Nullable: false,
			Item: &Object{
				Fields: []*Field{
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			},
		},
		nil, group)

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"incremental":[{"data":{"f1":[]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// ---- initial-response render error: returned to the router ----------------

// TestDefer_InitialRenderError_ReturnsToRouter: a hard error during the INITIAL
// (non-deferred) render — before the first frame is flushed — must be returned to
// the caller (router) so it can format a top-level error response, and must NOT
// commit or terminate the multipart stream. Contrast with deferred-fragment
// errors, which happen after the initial frame is on the wire and are scoped
// inline into completed.errors.
//
// Operation:
//
//	{
//	  f0                  # initial, non-deferred; the authorizer hard-errors here
//	  ... @defer { f1 }
//	}
//
// This locks in the Complete() ordering: writer.Complete() must not fire on a
// pre-flush error, otherwise the multipart terminator races onto the socket
// before the router can write its top-level error.
func TestDefer_InitialRenderError_ReturnsToRouter(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := simpleGroup(1, `{"f1":"deferred"}`)
	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1},
		},
		DeferTree: DeferSingle(group),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"f0":"v"}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name:  []byte("f0"),
						Value: &String{Path: []string{"f0"}, Nullable: true},
						Info: &FieldInfo{
							Name:                 "f0",
							HasAuthorizationRule: true,
							Source:               TypeFieldSource{IDs: []string{"ds"}, Names: []string{"ds"}},
						},
					},
					deferredField("f1", 1, &String{Path: []string{"f1"}, Nullable: true}, nil),
				},
			},
		},
	}

	ctx := NewContext(context.Background())
	ctx.SetAuthorizer(&deferTestAuthorizer{errOnField: "f0"})

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(ctx, response, w)

	// The error is returned for the router to format as a top-level response...
	require.Error(t, err)
	// ...and nothing was committed to the wire: no frame flushed and the stream
	// was not terminated, so the router can still write a clean error response.
	require.Empty(t, w.payloads)
	require.False(t, w.complete)
}

// ---- authorizer error during deferred render ------------------------------

// TestDefer_AuthErrorDuringDeferredRender_MustComplete: a hard authorizer error
// while rendering a deferred field is delivered as that defer's error; the
// announced pending is completed and the stream terminates.
//
// Operation:
//
//	{ ... @defer { f1 } }   # f1 carries @requiresScopes; the authorizer hard-errors
func TestDefer_AuthErrorDuringDeferredRender_MustComplete(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := simpleGroup(1, `{"f1":"secret"}`)
	info := &FieldInfo{
		Name:                 "f1",
		HasAuthorizationRule: true,
		Source:               TypeFieldSource{IDs: []string{"ds"}, Names: []string{"ds"}},
	}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, info, group)

	ctx := NewContext(context.Background())
	ctx.SetAuthorizer(&deferTestAuthorizer{errOnField: "f1"})

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(ctx, response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"completed":[{"id":"1","errors":[{"message":"authorizer hard error on f1"}]}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// TestDefer_AuthDenyDuringDeferredRender: an authorization DENY (the common
// production path, distinct from a hard error) on a deferred field renders as a
// rejected-field error inside the incremental item; the field is nulled and the
// pending still completes.
//
// Operation:
//
//	{ ... @defer { f1 } }   # f1 has an authorization rule; the authorizer denies it
func TestDefer_AuthDenyDuringDeferredRender(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	info := &FieldInfo{
		Name:                 "f1",
		HasAuthorizationRule: true,
		Source:               TypeFieldSource{IDs: []string{"ds"}, Names: []string{"ds"}},
	}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, info, simpleGroup(1, `{"f1":"secret"}`))

	ctx := NewContext(context.Background())
	ctx.SetAuthorizer(&deferTestAuthorizer{denyOnField: "f1"})

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(ctx, response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"incremental":[{"data":{"f1":null},"id":"1","errors":[{"message":"Unauthorized to load field 'Query.f1', Reason: missing scope.","path":["f1"],"extensions":{"code":"UNAUTHORIZED_FIELD_OR_TYPE"}}]}],"completed":[{"id":"1"}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// ---- field-renderer error during deferred render --------------------------

// TestDefer_RenderErrorDuringDeferredRender_MustComplete: a custom field-value
// renderer error while rendering a deferred field is scoped to that defer's
// completed entry; the pending is completed and the stream terminates.
//
// Operation:
//
//	{ ... @defer { f1 } }   # a custom field-value renderer errors on f1
func TestDefer_RenderErrorDuringDeferredRender_MustComplete(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := simpleGroup(1, `{"f1":"value"}`)
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, &FieldInfo{Name: "f1"}, group)

	ctx := NewContext(context.Background())
	ctx.SetFieldValueRenderer(&deferTestFieldRenderer{errOnField: "f1"})

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(ctx, response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"completed":[{"id":"1","errors":[{"message":"field renderer error on f1"}]}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// ---- rate-limiter (hard pre-fetch) error during deferred fetch ------------

// TestDefer_RateLimitErrorDuringDeferredFetch_MustComplete: a hard error from the
// deferred group's pre-fetch (e.g. rate limiter) is scoped to that defer's
// completed entry; the pending is completed and the stream terminates.
//
// Operation:
//
//	{ ... @defer { f1 } }   # the deferred group's pre-fetch is rate-limited (hard error)
func TestDefer_RateLimitErrorDuringDeferredFetch_MustComplete(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	group := &DeferFetchGroup{
		DeferID: 1,
		Fetches: Single(&SingleFetch{
			FetchConfiguration: FetchConfiguration{
				DataSource: FakeDataSource(`{"f1":"value"}`),
			},
			Info: &FetchInfo{DataSourceID: "ds", DataSourceName: "ds"},
		}),
	}
	response := rootDeferResponse(&String{Path: []string{"f1"}, Nullable: true}, nil, group)

	ctx := NewContext(context.Background())
	ctx.RateLimitOptions = RateLimitOptions{Enable: true}
	ctx.SetRateLimiter(&deferTestRateLimiter{errOnDataSourceID: "ds"})

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(ctx, response, w)
	require.NoError(t, err)
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}`,
		`{"completed":[{"id":"1","errors":[{"message":"rate limiter hard error on ds"}]}],"hasNext":false}`,
	}, w.payloads)
	require.True(t, w.complete)
}

// ---- nested defers: lazy announcement -------------------------------------

// TestDefer_NestedDefer_LazyAnnouncement: a nested @defer is announced in its
// parent's release frame, not eagerly in the initial frame.
//
// Operation:
//
//	{ user { id ... @defer { profile { bio ... @defer { avatar } } } } }
//	  defer 1 anchor ["user"]; defer 2 anchor ["user","profile"] (ParentID 1)
func TestDefer_NestedDefer_LazyAnnouncement(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: []string{"user"}},
			2: {ID: 2, Path: []string{"user", "profile"}, ParentID: 1},
		},
		DeferTree: DeferSequence(
			DeferSingle(simpleGroup(1, `{}`)),
			DeferSingle(simpleGroup(2, `{}`)),
		),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"user":{"id":"u1","profile":{"bio":"hi","avatar":"img"}}}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"user"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: true}},
								{
									Name:  []byte("profile"),
									Defer: &DeferField{DeferID: 1},
									Value: &Object{
										Nullable: true,
										Path:     []string{"profile"},
										Fields: []*Field{
											{Name: []byte("bio"), Defer: &DeferField{DeferID: 1}, Value: &String{Path: []string{"bio"}, Nullable: true}},
											{Name: []byte("avatar"), Defer: &DeferField{DeferID: 2}, Value: &String{Path: []string{"avatar"}, Nullable: true}},
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

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Len(t, w.payloads, 3)

	// Initial frame announces ONLY the top-level defer (id 1), not the nested id 2.
	require.Equal(t,
		`{"data":{"user":{"id":"u1"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}`,
		w.payloads[0])

	// Parent release frame announces the nested child (id 2) and completes id 1.
	assert.Contains(t, w.payloads[1], `"completed":[{"id":"1"}]`)
	assert.Contains(t, w.payloads[1], `"pending":[{"id":"2","path":["user","profile"]}]`)
	assert.Contains(t, w.payloads[1], `"hasNext":true`)

	// Child release frame completes id 2 and terminates.
	assert.Contains(t, w.payloads[2], `"completed":[{"id":"2"}]`)
	assert.Contains(t, w.payloads[2], `"hasNext":false`)
	require.True(t, w.complete)
}

// TestDefer_NestedChild_AnchorDies_Cancelled: when a nested child's anchor
// null-propagates in the parent's rendered data, the child is cancelled — never
// announced in the parent frame, never delivered — and the stream terminates on
// the parent frame.
//
// Operation:
//
//	{ user { id ... @defer { profile { boom ... @defer { avatar } } } } }
//	  boom: String! -> null -> nulls `profile` (defer 2's anchor)
func TestDefer_NestedChild_AnchorDies_Cancelled(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: []string{"user"}},
			2: {ID: 2, Path: []string{"user", "profile"}, ParentID: 1},
		},
		DeferTree: DeferSequence(
			DeferSingle(simpleGroup(1, `{}`)),
			DeferSingle(simpleGroup(2, `{}`)),
		),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"user":{"id":"u1","profile":{"boom":null,"avatar":"img"}}}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"user"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: true}},
								{
									Name:  []byte("profile"),
									Defer: &DeferField{DeferID: 1},
									Value: &Object{
										Nullable: true,
										Path:     []string{"profile"},
										Fields: []*Field{
											// non-null boom comes back null -> nulls `profile` (defer 2 anchor).
											{Name: []byte("boom"), Defer: &DeferField{DeferID: 1}, Value: &String{Path: []string{"boom"}, Nullable: false}},
											{Name: []byte("avatar"), Defer: &DeferField{DeferID: 2}, Value: &String{Path: []string{"avatar"}, Nullable: true}},
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

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)
	require.Len(t, w.payloads, 2)

	// Initial: only the top-level defer.
	require.Equal(t,
		`{"data":{"user":{"id":"u1"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}`,
		w.payloads[0])

	// Parent release frame: profile null-propagated, so child id 2 is NOT
	// announced, and this is the terminal frame.
	assert.Contains(t, w.payloads[1], `"completed":[{"id":"1"}`)
	assert.NotContains(t, w.payloads[1], `"id":"2"`)
	assert.Contains(t, w.payloads[1], `"hasNext":false`)
	require.True(t, w.complete)
}

// TestDefer_NestedDefer_ThreeLevels: defer 1 (user) -> defer 2 (user.profile) ->
// defer 3 (user.profile.contact). Each level is announced only when its parent is
// released; exactly one terminal frame.
//
//	{ user { id ...@defer{ profile { bio ...@defer{ contact { phone ...@defer{ ext } } } } } } }
func TestDefer_NestedDefer_ThreeLevels(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: []string{"user"}},
			2: {ID: 2, Path: []string{"user", "profile"}, ParentID: 1},
			3: {ID: 3, Path: []string{"user", "profile", "contact"}, ParentID: 2},
		},
		DeferTree: DeferSequence(
			DeferSingle(simpleGroup(1, `{}`)),
			DeferSequence(
				DeferSingle(simpleGroup(2, `{}`)),
				DeferSingle(simpleGroup(3, `{}`)),
			),
		),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"user":{"id":"u1","profile":{"bio":"hi","contact":{"phone":"p","ext":"e"}}}}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"user"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: true}},
								{
									Name:  []byte("profile"),
									Defer: &DeferField{DeferID: 1},
									Value: &Object{
										Nullable: true,
										Path:     []string{"profile"},
										Fields: []*Field{
											{Name: []byte("bio"), Defer: &DeferField{DeferID: 1}, Value: &String{Path: []string{"bio"}, Nullable: true}},
											{
												Name:  []byte("contact"),
												Defer: &DeferField{DeferID: 2},
												Value: &Object{
													Nullable: true,
													Path:     []string{"contact"},
													Fields: []*Field{
														{Name: []byte("phone"), Defer: &DeferField{DeferID: 2}, Value: &String{Path: []string{"phone"}, Nullable: true}},
														{Name: []byte("ext"), Defer: &DeferField{DeferID: 3}, Value: &String{Path: []string{"ext"}, Nullable: true}},
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

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)

	// Initial frame announces ONLY the top-level defer.
	require.Equal(t,
		`{"data":{"user":{"id":"u1"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}`,
		w.payloads[0])

	// id 2 is announced only after id 1; id 3 only after id 2.
	after1 := strings.Join(w.payloads[1:], "\n")
	assert.Contains(t, after1, `"id":"2","path":["user","profile"]`)
	assert.Contains(t, after1, `"id":"3","path":["user","profile","contact"]`)
	// neither nested defer leaks into the initial frame.
	assert.NotContains(t, w.payloads[0], `"id":"2"`)
	// Exactly one terminal frame across the whole stream.
	assert.Equal(t, 1, strings.Count(strings.Join(w.payloads, "\n"), `"hasNext":false`))
	require.True(t, w.complete)
}

// TestDefer_NestedChildren_OneDeadOneLive: a parent with two nested children
// announces only the child whose anchor survived; the dead one is cancelled.
//
//	{ user { id ...@defer{ a { x ...@defer{ ax } } b { boom ...@defer{ bx } } } } }
//	  defer 1 anchor [user]; child defer 2 anchor [user,a]; child defer 3 anchor [user,b]
//	  b.boom: String! -> null -> nulls [user,b] -> defer 3 cancelled
func TestDefer_NestedChildren_OneDeadOneLive(t *testing.T) {
	t.Parallel()
	r := newResolver(t.Context())

	response := &GraphQLDeferResponse{
		DeferDescriptors: map[int]DeferDescriptor{
			1: {ID: 1, Path: []string{"user"}},
			2: {ID: 2, Path: []string{"user", "a"}, ParentID: 1},
			3: {ID: 3, Path: []string{"user", "b"}, ParentID: 1},
		},
		DeferTree: DeferSequence(
			DeferSingle(simpleGroup(1, `{}`)),
			DeferParallel(
				DeferSingle(simpleGroup(2, `{}`)),
				DeferSingle(simpleGroup(3, `{}`)),
			),
		),
		Response: &GraphQLResponse{
			Info:    deferQueryInfo(),
			Fetches: simpleFetch(`{"user":{"id":"u1","a":{"x":"xv","ax":"axv"},"b":{"boom":null,"bx":"bxv"}}}`),
			Data: &Object{
				Nullable: true,
				Fields: []*Field{
					{
						Name: []byte("user"),
						Value: &Object{
							Nullable: true,
							Path:     []string{"user"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &String{Path: []string{"id"}, Nullable: true}},
								{
									Name:  []byte("a"),
									Defer: &DeferField{DeferID: 1},
									Value: &Object{
										Nullable: true,
										Path:     []string{"a"},
										Fields: []*Field{
											{Name: []byte("x"), Defer: &DeferField{DeferID: 1}, Value: &String{Path: []string{"x"}, Nullable: true}},
											{Name: []byte("ax"), Defer: &DeferField{DeferID: 2}, Value: &String{Path: []string{"ax"}, Nullable: true}},
										},
									},
								},
								{
									Name:  []byte("b"),
									Defer: &DeferField{DeferID: 1},
									Value: &Object{
										Nullable: true,
										Path:     []string{"b"},
										Fields: []*Field{
											{Name: []byte("boom"), Defer: &DeferField{DeferID: 1}, Value: &String{Path: []string{"boom"}, Nullable: false}},
											{Name: []byte("bx"), Defer: &DeferField{DeferID: 3}, Value: &String{Path: []string{"bx"}, Nullable: true}},
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

	w := &testDeferWriter{}
	_, err := r.ResolveGraphQLDeferResponse(NewContext(context.Background()), response, w)
	require.NoError(t, err)

	all := strings.Join(w.payloads, "\n")
	// child id 2 (anchor a) survives and is announced + delivered.
	assert.Contains(t, all, `"id":"2","path":["user","a"]`)
	// child id 3 (anchor b) is cancelled (b null-propagated) — never announced.
	assert.NotContains(t, all, `"id":"3"`)
	// exactly one terminal frame.
	assert.Equal(t, 1, strings.Count(all, `"hasNext":false`))
	require.True(t, w.complete)
}
