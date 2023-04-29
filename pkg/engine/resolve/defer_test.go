//go:build !windows

package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
)

func TestWithoutDefer(t *testing.T) {

	controller := gomock.NewController(t)

	userService := fakeService(t, controller, "user", "./testdata/users.json",
		"")
	postsService := fakeService(t, controller, "posts", "./testdata/posts.json",
		"1", "2",
	)

	res := &GraphQLResponse{
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
						Item: &Object{
							Fetch: &SingleFetch{
								BufferId:   1,
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

									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("posts"),
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

	buf := &bytes.Buffer{}

	err := resolver.ResolveGraphQLResponse(ctx, res, nil, buf)
	assert.NoError(t, err)

	expectedBytes, err := os.ReadFile("./testdata/response_without_defer.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expectedBytes), buf.String())
	if t.Failed() {
		fmt.Println(buf.String())
	}
}

func TestJsonPatch(t *testing.T) {
	initialResponse, err := os.ReadFile("./testdata/defer_1.json")
	assert.NoError(t, err)
	patch1, err := os.ReadFile("./testdata/defer_2.json")
	assert.NoError(t, err)
	patch2, err := os.ReadFile("./testdata/defer_3.json")
	assert.NoError(t, err)

	p1, err := jsonpatch.DecodePatch(patch1)
	assert.NoError(t, err)

	p2, err := jsonpatch.DecodePatch(patch2)
	assert.NoError(t, err)

	patched, err := p1.Apply(initialResponse)
	assert.NoError(t, err)

	patched, err = p2.Apply(patched)
	assert.NoError(t, err)

	expectedBytes, err := os.ReadFile("./testdata/response_without_defer.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expectedBytes), string(patched))
	if t.Failed() {
		fmt.Println(string(patched))
	}
}

type TestWriter struct {
	flushed []string
	buf     bytes.Buffer
}

func (t *TestWriter) Write(p []byte) (n int, err error) {
	return t.buf.Write(p)
}

func (t *TestWriter) Flush() {
	t.flushed = append(t.flushed, t.buf.String())
	t.buf.Reset()
}

func TestDefer(t *testing.T) {

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
									{
										Name: []byte("posts"),
										Value: &Null{
											Defer: Defer{
												Enabled:    true,
												PatchIndex: 0,
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
		Patches: []*GraphQLResponsePatch{
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

	writer := &TestWriter{}

	err := resolver.ResolveGraphQLStreamingResponse(ctx, res, nil, writer)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(writer.flushed))

	expectedBytes, err := os.ReadFile("./testdata/defer_1.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expectedBytes), writer.flushed[0])
	if t.Failed() {
		fmt.Println(writer.flushed[0])
	}

	expectedBytes, err = os.ReadFile("./testdata/defer_2.json")
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedBytes), writer.flushed[1])
	if t.Failed() {
		fmt.Println(writer.flushed[1])
	}

	expectedBytes, err = os.ReadFile("./testdata/defer_3.json")
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedBytes), writer.flushed[2])
	if t.Failed() {
		fmt.Println(writer.flushed[2])
	}
}

type DiscardFlushWriter struct {
}

func (d *DiscardFlushWriter) Write(p []byte) (n int, err error) {
	return
}

func (d *DiscardFlushWriter) Flush() {

}

func BenchmarkDefer(b *testing.B) {

	userData, err := os.ReadFile("./testdata/users.json")
	assert.NoError(b, err)
	postsData, err := os.ReadFile("./testdata/posts.json")
	assert.NoError(b, err)

	userService := FakeDataSource(string(userData))
	postsService := FakeDataSource(string(postsData))

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
									{
										Name: []byte("posts"),
										Value: &Null{
											Defer: Defer{
												Enabled:    true,
												PatchIndex: 0,
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
		Patches: []*GraphQLResponsePatch{
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

	bgCtx := context.Background()
	ctx := NewContext(bgCtx)

	writer := &DiscardFlushWriter{}
	// writer := &TestFlushWriter{}

	expect1, err := os.ReadFile("./testdata/defer_1.json")
	assert.NoError(b, err)
	expect2, err := os.ReadFile("./testdata/defer_2.json")
	assert.NoError(b, err)
	expect3, err := os.ReadFile("./testdata/defer_3.json")
	assert.NoError(b, err)

	_, _, _ = expect1, expect2, expect3

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		/*err = resolver.ResolveGraphQLStreamingResponse(ctx, res, nil, writer)
		assert.NoError(b,err)
		assert.Equal(b,3,len(writer.flushed))
		assert.JSONEq(b,string(expect1),writer.flushed[0])
		assert.JSONEq(b,string(expect2),writer.flushed[1])
		assert.JSONEq(b,string(expect3),writer.flushed[2])*/

		_ = resolver.ResolveGraphQLStreamingResponse(ctx, res, nil, writer)

		ctx.Free()
		ctx.ctx = bgCtx
		// writer.flushed = writer.flushed[:0]
	}
}

func fakeService(t *testing.T, controller *gomock.Controller, serviceName, responseFilePath string, expectedInput ...string) DataSource {
	data, err := os.ReadFile(responseFilePath)
	assert.NoError(t, err)
	service := NewMockDataSource(controller)
	for i := 0; i < len(expectedInput); i++ {
		i := i
		service.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			DoAndReturn(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				assert.Equal(t, expectedInput[i], string(input))
				_, err = w.Write(data)
				return
			})
	}
	return service
}
