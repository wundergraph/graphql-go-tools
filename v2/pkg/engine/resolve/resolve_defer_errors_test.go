package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// This file documents how @defer interacts with errors, mirroring the reference
// graphql-js incremental-delivery suite
// (graphql-js/src/execution/incremental/__tests__/defer-test.ts). Each test names
// both the equivalent GraphQL @defer operation it represents and the graphql-js
// analog it is derived from, and asserts the FULL multipart payload sequence
// (one string per flushed frame) so the exact wire shape is visible.
//
// Tests asserting CORRECT behavior the engine does NOT yet implement are marked
// t.Skip("KNOWN BUG: …") so the package suite stays green; their expected payloads
// encode the target wire shape. Removing the Skip turns each into the failing TDD
// driver for the fix.

// ---- test doubles ---------------------------------------------------------

// deferTestAuthorizer returns a hard error from AuthorizeObjectField for a chosen
// field, simulating an authorizer failure during the deferred render walk (F04).
type deferTestAuthorizer struct {
	errOnField string
}

func (a *deferTestAuthorizer) AuthorizePreFetch(_ *Context, _ string, _ json.RawMessage, _ GraphCoordinate) (*AuthorizationDeny, error) {
	return nil, nil
}

func (a *deferTestAuthorizer) AuthorizeObjectField(_ *Context, _ string, _ json.RawMessage, coordinate GraphCoordinate) (*AuthorizationDeny, error) {
	if coordinate.FieldName == a.errOnField {
		return nil, errors.New("authorizer hard error on " + coordinate.FieldName)
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

// TestDefer_MultipleErroringGroups_AllCompleted: two top-level defers that both
// fully null-bubble each report their error on their own completed entry.
//
// Operation:
//
//	{ ... @defer { f1 }  ... @defer { f2 } }   # f1: String!, f2: String!
//
// (A DeferSequence is used instead of DeferParallel only to make the frame order
// deterministic for a full-payload assertion; the error rendering is identical.)
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
		DeferTree: DeferSequence(DeferSingle(groupA), DeferSingle(groupB)),
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
	require.Equal(t, []string{
		`{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":[]}],"hasNext":true}`,
		`{"completed":[{"id":"1","errors":[{"message":"Cannot return null for non-nullable field 'Query.f1'.","path":["f1"]}]}],"hasNext":true}`,
		`{"completed":[{"id":"2","errors":[{"message":"Cannot return null for non-nullable field 'Query.f2'.","path":["f2"]}]}],"hasNext":false}`,
	}, w.payloads)
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
//
// KNOWN BUG: defer delivery is gated on the global resolvable.hasErrors()
// (resolvable.go printObject ~294 + resolve.go ResolveGraphQLDeferResponse ~530),
// so ANY initial error drops EVERY defer regardless of path. Today the engine
// emits only the initial frame below with hasNext:false and no pending.
func TestDefer_ErrorOnDifferentRootPath_DeferStillDelivered(t *testing.T) {
	t.Parallel()
	t.Skip("KNOWN BUG: a recoverable error on an unrelated path drops a surviving-anchor defer; should deliver per graphql-js 'Keeps deferred work outside nulled error paths'")
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

// ---- authorizer error during deferred render ------------------------------

// TestDefer_AuthErrorDuringDeferredRender_MustComplete: a hard authorizer error
// while rendering a deferred field must be delivered as that defer's error and the
// announced pending must still be completed + the stream terminated, not orphaned.
//
// Operation:
//
//	{ ... @defer { f1 } }   # f1 carries @requiresScopes; the authorizer hard-errors
//
// KNOWN BUG (F04): ResolveDeferBatch returns early on r.authorizationError
// (resolvable.go ~322) before writing completed/hasNext; the error then aborts
// ResolveGraphQLDeferResponse (resolve.go ~545), so only the initial frame is
// flushed and the pending (id "1") is never completed.
//
// NOTE: the exact error rendering (message/extensions, incremental vs completed)
// is the open design point of the fix; the expected payload below encodes the
// minimum: the pending is completed and the stream terminates.
func TestDefer_AuthErrorDuringDeferredRender_MustComplete(t *testing.T) {
	t.Parallel()
	t.Skip("KNOWN BUG (F04): authorizer error during deferred render orphans the pending; must complete it and terminate")
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

// ---- field-renderer error during deferred render --------------------------

// TestDefer_RenderErrorDuringDeferredRender_MustComplete: a custom field-value
// renderer error while rendering a deferred field must complete the pending and
// terminate the stream rather than swallowing the error.
//
// Operation:
//
//	{ ... @defer { f1 } }   # a custom field-value renderer errors on f1
//
// KNOWN BUG (F05): the renderer error sets r.printErr, which no-ops printHasNext
// (resolvable.go ~466) and makes ResolveDeferBatch return the printErr before the
// terminal frame is flushed; ResolveGraphQLDeferResponse then aborts.
//
// NOTE: exact error rendering is the open design point of the fix.
func TestDefer_RenderErrorDuringDeferredRender_MustComplete(t *testing.T) {
	t.Parallel()
	t.Skip("KNOWN BUG (F05): field-renderer error during deferred render orphans the pending; must complete it and terminate")
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
// deferred group's pre-fetch (e.g. rate limiter) must still complete the pending
// and terminate the stream, not orphan it.
//
// Operation:
//
//	{ ... @defer { f1 } }   # the deferred group's pre-fetch is rate-limited (hard error)
//
// KNOWN BUG: a hard fetch-phase error in resolveDeferSingle propagates out of
// ResolveGraphQLDeferResponse (resolve.go ~598/545) without completing the
// announced pending or writing a terminal frame.
//
// NOTE: exact error rendering is the open design point of the fix.
func TestDefer_RateLimitErrorDuringDeferredFetch_MustComplete(t *testing.T) {
	t.Parallel()
	t.Skip("KNOWN BUG: hard pre-fetch error on a deferred group orphans the pending; must complete it and terminate")
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
