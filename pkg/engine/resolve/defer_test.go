package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestWithoutDefer(t *testing.T) {

	controller := gomock.NewController(t)

	userService := fakeService(t, controller,1, "user", "./testdata/users.json")
	postsService := fakeService(t, controller, 10,"posts", "./testdata/posts.json")

	res := &GraphQLResponse{
		Data: &Object{
			Fetch: &SingleFetch{
				DataSource: userService,
				BufferId:   0,
			},
			FieldSets: []FieldSet{
				{
					HasBuffer: true,
					BufferID:  0,
					Fields: []Field{
						{
							Name: []byte("users"),
							Value: &Array{
								Item: &Object{
									Fetch: &SingleFetch{
										BufferId:   1,
										DataSource: postsService,
									},
									FieldSets: []FieldSet{
										{
											Fields: []Field{
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
										{
											HasBuffer: true,
											BufferID:  1,
											Fields: []Field{
												{
													Name: []byte("posts"),
													Value: &Array{
														Item: &Object{
															FieldSets: []FieldSet{
																{
																	Fields: []Field{
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
							},
						},
					},
				},
			},
		},
	}

	resolver := New()

	ctx := Context{
		Context: context.Background(),
	}

	buf := &bytes.Buffer{}

	err := resolver.ResolveGraphQLResponse(ctx, res, nil, buf)
	assert.NoError(t, err)

	expectedBytes, err := ioutil.ReadFile("./testdata/response_without_defer.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expectedBytes), buf.String())
	if t.Failed() {
		fmt.Println(buf.String())
	}
}

func TestJsonPatch(t *testing.T){
	initialResponse,err := ioutil.ReadFile("./testdata/defer_1.json")
	assert.NoError(t,err)
	patch1,err := ioutil.ReadFile("./testdata/defer_2.json")
	assert.NoError(t,err)
	patch2,err := ioutil.ReadFile("./testdata/defer_3.json")
	assert.NoError(t,err)

	p1,err := jsonpatch.DecodePatch(patch1)
	assert.NoError(t,err)

	p2,err := jsonpatch.DecodePatch(patch2)
	assert.NoError(t,err)

	patched,err := p1.Apply(initialResponse)
	assert.NoError(t,err)

	patched,err = p2.Apply(patched)
	assert.NoError(t,err)

	expectedBytes, err := ioutil.ReadFile("./testdata/response_without_defer.json")
	assert.NoError(t, err)
	assert.JSONEq(t, string(expectedBytes), string(patched))
	if t.Failed() {
		fmt.Println(string(patched))
	}
}

func fakeService(t *testing.T, controller *gomock.Controller, expectedCalls int, name, responseFilePath string) DataSource {
	service := NewMockDataSource(controller)
	service.EXPECT().UniqueIdentifier().Return([]byte(name))
	for i := 0; i < expectedCalls; i++ {
		service.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&BufPair{})).
			Do(func(ctx context.Context, input []byte, pair *BufPair) (err error) {
				data, err := ioutil.ReadFile(responseFilePath)
				assert.NoError(t, err)
				pair.Data.WriteBytes(data)
				return
			}).
			Return(nil)
	}
	return service
}
