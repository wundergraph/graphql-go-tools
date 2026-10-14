package resolve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefer_AllDefersCancelledBeforeInitialResult covers initial null propagation that
// removes every defer anchor. The resolver writes an ordinary execution result.
func TestDefer_AllDefersCancelledBeforeInitialResult(t *testing.T) {
	t.Parallel()

	t.Run("[C1] null propagation removes the anchor of the only defer", func(t *testing.T) {
		t.Parallel()
		r := newResolver(t.Context())

		// { user { name ... @defer { articles } } }
		// user: User (nullable); name: String! returns null and nulls user.
		response := &GraphQLDeferResponse{
			DeferDescriptors: map[int]DeferDescriptor{
				1: {ID: 1, Path: []string{"user"}},
			},
			DeferTree: DeferSingle(simpleGroup(1, `{"articles":"x"}`)),
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
		_, err := r.ResolveGraphQLDeferResponse(NewContext(t.Context()), response, w)
		require.NoError(t, err)
		require.Equal(t, []string{
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":{"user":null}}`,
		}, w.payloads)
		require.True(t, w.complete)
	})

	t.Run("[C2] null propagation removes a parent defer and its nested child", func(t *testing.T) {
		t.Parallel()
		r := newResolver(t.Context())

		// { user { boom ... @defer { p { ... @defer { c } } } } }
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
		_, err := r.ResolveGraphQLDeferResponse(NewContext(t.Context()), response, w)
		require.NoError(t, err)
		require.Equal(t, []string{
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.boom'.","path":["user","boom"]}],"data":{"user":null}}`,
		}, w.payloads)
		require.True(t, w.complete)
	})

	t.Run("[C3] null propagation reaches the root and removes a nested anchor", func(t *testing.T) {
		t.Parallel()
		r := newResolver(t.Context())

		// { user { name ... @defer { articles } } }
		// user: User! (non-null); name: String! returns null and nulls data.
		response := &GraphQLDeferResponse{
			DeferDescriptors: map[int]DeferDescriptor{
				1: {ID: 1, Path: []string{"user"}},
			},
			DeferTree: DeferSingle(simpleGroup(1, `{"articles":"x"}`)),
			Response: &GraphQLResponse{
				Info:    deferQueryInfo(),
				Fetches: simpleFetch(`{"user":{"name":null}}`),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Nullable: false,
								Path:     []string{"user"},
								Fields: []*Field{
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
		_, err := r.ResolveGraphQLDeferResponse(NewContext(t.Context()), response, w)
		require.NoError(t, err)
		require.Equal(t, []string{
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":null}`,
		}, w.payloads)
		require.True(t, w.complete)
	})

	t.Run("[C4] null propagation reaches the root and removes a root defer", func(t *testing.T) {
		t.Parallel()
		r := newResolver(t.Context())

		// { boom ... @defer { f1 } }
		// boom: String! returns null and nulls data; the defer anchor is the root.
		response := &GraphQLDeferResponse{
			DeferDescriptors: map[int]DeferDescriptor{
				1: {ID: 1},
			},
			DeferTree: DeferSingle(simpleGroup(1, `{"f1":"x"}`)),
			Response: &GraphQLResponse{
				Info:    deferQueryInfo(),
				Fetches: simpleFetch(`{"boom":null}`),
				Data: &Object{
					Nullable: true,
					Fields: []*Field{
						{Name: []byte("boom"), Value: &String{Path: []string{"boom"}, Nullable: false}},
						deferredField("f1", 1, &String{Path: []string{"f1"}, Nullable: true}, nil),
					},
				},
			},
		}

		w := &testDeferWriter{}
		_, err := r.ResolveGraphQLDeferResponse(NewContext(t.Context()), response, w)
		require.NoError(t, err)
		require.Equal(t, []string{
			`{"errors":[{"message":"Cannot return null for non-nullable field 'Query.boom'.","path":["boom"]}],"data":null}`,
		}, w.payloads)
		require.True(t, w.complete)
	})
}
