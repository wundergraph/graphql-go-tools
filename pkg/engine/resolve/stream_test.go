//go:build !windows

package resolve

import (
	"context"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
)

func TestArrayStream(t *testing.T) {

	controller := gomock.NewController(t)

	userService := fakeService(t, controller, "user", "./testdata/users.json",
		"")

	res := &GraphQLStreamingResponse{
		InitialResponse: &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					DataSource: userService,
					BufferId:   0,
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("users"),
						Value: &Array{
							Stream: Stream{
								Enabled:          true,
								InitialBatchSize: 0,
								PatchIndex:       0,
							},
						},
					},
				},
			},
		},
		Patches: []*GraphQLResponsePatch{
			{
				Operation: literal.ADD,
				Value: &Object{
					Fields: []*Field{
						{
							Name: []byte("id"),
							Value: &Integer{
								Path: []string{"id"},
							},
						},
						{
							Name: []byte("name"),
							Value: &String{
								Path: []string{"name"},
							},
						},
					},
				},
			},
		},
	}

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := New(rCtx, NewFetcher(false), false)

	ctx := NewContext(context.Background())

	writer := &TestFlushWriter{}

	err := resolver.ResolveGraphQLStreamingResponse(ctx, res, nil, writer)
	assert.NoError(t, err)

	assert.Equal(t, 3, len(writer.flushed))

	expected, err := os.ReadFile("./testdata/stream_1.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[0])

	expected, err = os.ReadFile("./testdata/stream_2.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[1])

	expected, err = os.ReadFile("./testdata/stream_3.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[2])
}

func TestArrayStream_InitialBatch_1(t *testing.T) {

	controller := gomock.NewController(t)

	userService := fakeService(t, controller, "user", "./testdata/users.json",
		"")

	res := &GraphQLStreamingResponse{
		InitialResponse: &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					DataSource: userService,
					BufferId:   0,
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("users"),
						Value: &Array{
							Stream: Stream{
								Enabled:          true,
								InitialBatchSize: 1,
								PatchIndex:       0,
							},
							Item: &Object{
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Integer{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Patches: []*GraphQLResponsePatch{
			{
				Operation: literal.ADD,
				Value: &Object{
					Fields: []*Field{
						{
							Name: []byte("id"),
							Value: &Integer{
								Path: []string{"id"},
							},
						},
						{
							Name: []byte("name"),
							Value: &String{
								Path: []string{"name"},
							},
						},
					},
				},
			},
		},
	}

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := New(rCtx, NewFetcher(false), false)

	ctx := NewContext(context.Background())

	writer := &TestFlushWriter{}

	err := resolver.ResolveGraphQLStreamingResponse(ctx, res, nil, writer)
	assert.NoError(t, err)

	assert.Equal(t, 2, len(writer.flushed))

	expected, err := os.ReadFile("./testdata/stream_4.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[0])

	expected, err = os.ReadFile("./testdata/stream_3.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[1])
}

func TestArrayStream_InitialBatch_2(t *testing.T) {

	controller := gomock.NewController(t)

	userService := fakeService(t, controller, "user", "./testdata/users.json",
		"")

	res := &GraphQLStreamingResponse{
		InitialResponse: &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					DataSource: userService,
					BufferId:   0,
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("users"),
						Value: &Array{
							Stream: Stream{
								Enabled:          true,
								InitialBatchSize: 2,
								PatchIndex:       0,
							},
							Item: &Object{
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Integer{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Patches: []*GraphQLResponsePatch{
			{
				Operation: literal.ADD,
				Value: &Object{
					Fields: []*Field{
						{
							Name: []byte("id"),
							Value: &Integer{
								Path: []string{"id"},
							},
						},
						{
							Name: []byte("name"),
							Value: &String{
								Path: []string{"name"},
							},
						},
					},
				},
			},
		},
	}

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := New(rCtx, NewFetcher(false), false)

	ctx := NewContext(context.Background())

	writer := &TestFlushWriter{}

	err := resolver.ResolveGraphQLStreamingResponse(ctx, res, nil, writer)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(writer.flushed))

	expected, err := os.ReadFile("./testdata/stream_5.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[0])
}

func TestStreamAndDefer(t *testing.T) {
	t.Skip("temporary disabled")

	controller := gomock.NewController(t)

	userService := fakeService(t, controller, "user", "./testdata/users.json",
		"")

	postsService := fakeService(t, controller, "posts", "./testdata/posts.json",
		"1", "2",
	)

	res := &GraphQLStreamingResponse{
		InitialResponse: &GraphQLResponse{
			Data: &Object{
				Fetch: &SingleFetch{
					DataSource: userService,
					BufferId:   0,
				},
				Fields: []*Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("users"),
						Value: &Array{
							Stream: Stream{
								Enabled:          true,
								InitialBatchSize: 0,
								PatchIndex:       0,
							},
						},
					},
				},
			},
		},
		Patches: []*GraphQLResponsePatch{
			{
				Operation: literal.ADD,
				Value: &Object{
					Fields: []*Field{
						{
							Name: []byte("id"),
							Value: &Integer{
								Path: []string{"id"},
							},
						},
						{
							Name: []byte("name"),
							Value: &String{
								Path: []string{"name"},
							},
						},
						{

							Name: []byte("posts"),
							Value: &Null{
								Defer: Defer{
									Enabled:    true,
									PatchIndex: 1,
								},
							},
						},
					},
				},
			},
			{
				Operation: literal.REPLACE,
				Fetch: &SingleFetch{
					DataSource: postsService,
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ObjectVariableKind,
								VariableSourcePath: []string{"id"},
								Renderer:           NewGraphQLVariableRenderer(`{"type":"number"}`),
							},
						},
					},
				},
				Value: &Array{
					Item: &Object{
						Fields: []*Field{
							{
								Name: []byte("title"),
								Value: &String{
									Path: []string{"title"},
								},
							},
							{
								Name: []byte("body"),
								Value: &String{
									Path: []string{"body"},
								},
							},
						},
					},
				},
			},
		},
	}

	rCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := New(rCtx, NewFetcher(false), false)

	ctx := NewContext(context.Background())

	writer := &TestFlushWriter{}

	err := resolver.ResolveGraphQLStreamingResponse(ctx, res, nil, writer)
	assert.NoError(t, err)

	assert.Equal(t, 5, len(writer.flushed))

	expected, err := os.ReadFile("./testdata/stream_1.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[0])

	expected, err = os.ReadFile("./testdata/stream_6.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[1])

	expected, err = os.ReadFile("./testdata/stream_7.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[2])

	expected, err = os.ReadFile("./testdata/stream_8.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[3])

	expected, err = os.ReadFile("./testdata/stream_9.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expected), writer.flushed[4])
}
