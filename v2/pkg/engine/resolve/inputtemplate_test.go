package resolve

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/astjson"
)

func TestInputTemplate_VariablesRemapping(t *testing.T) {
	runTest := func(t *testing.T, variables string, sourcePath []string, remap map[string]string, expectErr bool, expected string) {
		t.Helper()

		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: sourcePath,
					Renderer:           NewJSONVariableRenderer(),
				},
			},
		}
		ctx := &Context{
			Variables:      astjson.MustParseBytes([]byte(variables)),
			RemapVariables: remap,
		}
		buf := &bytes.Buffer{}
		err := template.Render(ctx, nil, buf)
		if expectErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		out := buf.String()
		assert.Equal(t, expected, out)
	}

	t.Run("maping", func(t *testing.T) {
		t.Run("a to foo", func(t *testing.T) {
			runTest(t, `{"foo":"bar"}`, []string{"a"}, map[string]string{"a": "foo"}, false, `"bar"`)
		})
		t.Run("a to a", func(t *testing.T) {
			runTest(t, `{"a":true}`, []string{"a"}, map[string]string{"a": "a"}, false, "true")
		})
		t.Run("no mapping", func(t *testing.T) {
			runTest(t, `{"a":true}`, []string{"a"}, map[string]string{}, false, "true")
		})
		t.Run("no variable value", func(t *testing.T) {
			runTest(t, `{}`, []string{"a"}, map[string]string{"a": "x"}, false, `{"undefined":["a"]}`)
		})
	})
}

func TestInputTemplate_Render(t *testing.T) {
	runTest := func(t *testing.T, initRenderer initTestVariableRenderer, variables string, sourcePath []string, expectErr bool, expected string) {
		t.Helper()

		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: sourcePath,
					Renderer:           initRenderer(),
				},
			},
		}
		ctx := &Context{
			Variables: astjson.MustParseBytes([]byte(variables)),
		}
		buf := &bytes.Buffer{}
		err := template.Render(ctx, nil, buf)
		if expectErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		out := buf.String()
		assert.Equal(t, expected, out)
	}

	t.Run("plain renderer", func(t *testing.T) {
		renderer := useTestPlainVariableRenderer()
		t.Run("string scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":"bar"}`, []string{"foo"}, false, `bar`)
		})
		t.Run("boolean scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":true}`, []string{"foo"}, false, "true")
		})
		t.Run("nested string", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":"value"}}`, []string{"foo", "bar"}, false, `value`)
		})
		t.Run("json object pass through", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":"baz"}}`, []string{"foo"}, false, `{"bar":"baz"}`)
		})
		t.Run("json object as graphql object", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":"baz"}}`, []string{"foo"}, false, `{"bar":"baz"}`)
		})
		t.Run("json object as graphql object with null", func(t *testing.T) {
			runTest(t, renderer, `{"foo":null}`, []string{"foo"}, false, `null`)
		})
		t.Run("json object as graphql object with number", func(t *testing.T) {
			runTest(t, renderer, `{"foo":123}`, []string{"foo"}, false, `123`)
		})
		t.Run("json object as graphql object with boolean", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":true}}`, []string{"foo"}, false, `{"bar":true}`)
		})
		t.Run("json object as graphql object with number", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":123}}`, []string{"foo"}, false, `{"bar":123}`)
		})
		t.Run("json object as graphql object with float", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":1.23}}`, []string{"foo"}, false, `{"bar":1.23}`)
		})
		t.Run("json object as graphql object with nesting", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":{"baz":"bat"}}}`, []string{"foo"}, false, `{"bar":{"baz":"bat"}}`)
		})
		t.Run("json object as graphql object with single array", func(t *testing.T) {
			runTest(t, renderer, `{"foo":["bar"]}`, []string{"foo"}, false, `["bar"]`)
		})
		t.Run("json object as graphql object with array", func(t *testing.T) {
			runTest(t, renderer, `{"foo":["bar","baz"]}`, []string{"foo"}, false, `["bar","baz"]`)
		})
		t.Run("json object as graphql object with object array", func(t *testing.T) {
			runTest(t, renderer, `{"foo":[{"bar":"baz"},{"bar":"bat"}]}`, []string{"foo"}, false, `[{"bar":"baz"},{"bar":"bat"}]`)
		})
	})

	t.Run("json renderer", func(t *testing.T) {
		renderer := useTestJSONVariableRenderer()
		t.Run("string scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":"bar"}`, []string{"foo"}, false, `"bar"`)
		})
		t.Run("boolean scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":true}`, []string{"foo"}, false, "true")
		})
		t.Run("number scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":1}`, []string{"foo"}, false, "1")
		})
		t.Run("nested string", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":"value"}}`, []string{"foo", "bar"}, false, `"value"`)
		})
		t.Run("on non-required scalars", func(t *testing.T) {
			t.Run("null on non-required string scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, false, `null`)
			})
			t.Run("null on non-required int scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, false, `null`)
			})
			t.Run("null on non-required float scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, false, `null`)
			})
			t.Run("null on non-required boolean scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, false, `null`)
			})
		})
	})

	t.Run("array with csv render string", func(t *testing.T) {
		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: []string{"a"},
					Renderer:           NewCSVVariableRenderer(JsonRootType{Value: jsonparser.String, Kind: JsonRootTypeKindSingle}),
				},
			},
		}
		ctx := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"a":["foo","bar"]}`)),
		}
		buf := &bytes.Buffer{}
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, "foo,bar", out)
	})
	t.Run("array with csv render int", func(t *testing.T) {
		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: []string{"a"},
					Renderer:           NewCSVVariableRenderer(JsonRootType{Value: jsonparser.Number}),
				},
			},
		}
		ctx := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"a":[1,2,3]}`)),
		}
		buf := &bytes.Buffer{}
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, "1,2,3", out)
	})

	t.Run("header variable", func(t *testing.T) {
		t.Run("missing value for header variable - results into empty segment", func(t *testing.T) {
			template := InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`{"key":"`),
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       HeaderVariableKind,
						VariableSourcePath: []string{"Auth"},
					},
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`"}`),
					},
				},
			}
			ctx := &Context{
				Variables: astjson.MustParseBytes([]byte(`{}`)),
			}
			buf := &bytes.Buffer{}
			err := template.Render(ctx, nil, buf)
			assert.NoError(t, err)
			out := buf.String()
			assert.Equal(t, `{"key":""}`, out)
		})

		t.Run("renders single value", func(t *testing.T) {
			template := InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`{"key":"`),
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       HeaderVariableKind,
						VariableSourcePath: []string{"Auth"},
					},
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`"}`),
					},
				},
			}
			ctx := &Context{
				Variables: astjson.MustParseBytes([]byte(`{}`)),
				Request: Request{
					Header: http.Header{"Auth": []string{"value"}},
				},
			}
			buf := &bytes.Buffer{}
			err := template.Render(ctx, nil, buf)
			assert.NoError(t, err)
			out := buf.String()
			assert.Equal(t, `{"key":"value"}`, out)
		})

		t.Run("renders multi value", func(t *testing.T) {
			template := InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`{"key":"`),
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       HeaderVariableKind,
						VariableSourcePath: []string{"Auth"},
					},
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`"}`),
					},
				},
			}
			ctx := &Context{
				Variables: astjson.MustParseBytes([]byte(`{}`)),
				Request: Request{
					Header: http.Header{"Auth": []string{"value1", "value2"}},
				},
			}
			buf := &bytes.Buffer{}
			err := template.Render(ctx, nil, buf)
			assert.NoError(t, err)
			out := buf.String()
			assert.Equal(t, `{"key":"value1,value2"}`, out)
		})
	})

	t.Run("JSONVariableRenderer", func(t *testing.T) {
		t.Run("missing value for context variable - renders segment to null", func(t *testing.T) {
			template := InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`{"key":`),
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       ContextVariableKind,
						VariableSourcePath: []string{"a"},
						Renderer:           NewJSONVariableRenderer(),
					},
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`}`),
					},
				},
			}
			ctx := &Context{
				ctx:       context.Background(),
				Variables: astjson.MustParseBytes([]byte(`{}`)),
			}
			buf := &bytes.Buffer{}
			err := template.Render(ctx, nil, buf)
			assert.NoError(t, err)
			out := buf.String()
			assert.Equal(t, `{"undefined":["a"],"key":null}`, out)
		})

		t.Run("when SetTemplateOutputToNullOnVariableNull: true", func(t *testing.T) {
			t.Run("null value for object variable - renders whole template as null", func(t *testing.T) {
				template := InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"key":`),
						},
						{
							SegmentType:        VariableSegmentType,
							VariableKind:       ObjectVariableKind,
							VariableSourcePath: []string{"id"},
							Renderer:           NewJSONVariableRenderer(),
						},
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`}`),
						},
					},
					SetTemplateOutputToNullOnVariableNull: true,
				}
				ctx := &Context{
					Variables: astjson.MustParseBytes([]byte(`{}`)),
				}
				buf := &bytes.Buffer{}
				err := template.Render(ctx, nil, buf)
				assert.NoError(t, err)
				out := buf.String()
				assert.Equal(t, `null`, out)
			})

			t.Run("null value for context variable - renders segment as null", func(t *testing.T) {
				template := InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"key":`),
						},
						{
							SegmentType:        VariableSegmentType,
							VariableKind:       ContextVariableKind,
							VariableSourcePath: []string{"x"},
							Renderer:           NewJSONVariableRenderer(),
						},
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`}`),
						},
					},
					SetTemplateOutputToNullOnVariableNull: true,
				}
				ctx := &Context{
					Variables: astjson.MustParseBytes([]byte(`{"x":null}`)),
				}
				buf := &bytes.Buffer{}
				err := template.Render(ctx, nil, buf)
				assert.NoError(t, err)
				out := buf.String()
				assert.Equal(t, `{"key":null}`, out)
			})

			t.Run("missing value for header variable - results into empty segment", func(t *testing.T) {
				template := InputTemplate{
					Segments: []TemplateSegment{
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`{"key":"`),
						},
						{
							SegmentType:        VariableSegmentType,
							VariableKind:       HeaderVariableKind,
							VariableSourcePath: []string{"Auth"},
						},
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`"}`),
						},
					},
					SetTemplateOutputToNullOnVariableNull: true,
				}
				ctx := &Context{
					Variables: astjson.MustParseBytes([]byte(`{}`)),
				}
				buf := &bytes.Buffer{}
				err := template.Render(ctx, nil, buf)
				assert.NoError(t, err)
				out := buf.String()
				assert.Equal(t, `{"key":""}`, out)
			})
		})
	})

	t.Run("GraphQLVariableResolveRenderer", func(t *testing.T) {
		t.Run("optional fields", func(t *testing.T) {
			template := InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType:  VariableSegmentType,
						VariableKind: ResolvableObjectVariableKind,
						Renderer: NewGraphQLVariableResolveRenderer(&Object{
							Nullable: false,
							Fields: []*Field{
								{
									Name: []byte("name"),
									Value: &String{
										Path:     []string{"name"},
										Nullable: true,
									},
								},
							},
						}),
					},
				},
			}

			data := astjson.MustParseBytes([]byte(`{"name":"foo"}`))
			ctx := &Context{
				ctx: context.Background(),
			}
			buf := &bytes.Buffer{}

			err := template.Render(ctx, data, buf)
			assert.NoError(t, err)
			out := buf.String()
			assert.Equal(t, `{"name":"foo"}`, out)

			data = astjson.MustParseBytes([]byte(`{}`))
			buf.Reset()
			err = template.Render(ctx, data, buf)
			assert.NoError(t, err)
			out = buf.String()
			assert.Equal(t, `{"name":null}`, out)

			data = astjson.MustParseBytes([]byte(`{"name":null}`))
			buf.Reset()
			err = template.Render(ctx, data, buf)
			assert.NoError(t, err)
			out = buf.String()
			assert.Equal(t, `{"name":null}`, out)

			data = astjson.MustParseBytes([]byte(`{"name":123}`))
			buf.Reset()
			err = template.Render(ctx, data, buf)
			assert.Error(t, err)
		})
		t.Run("nested objects", func(t *testing.T) {
			template := InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType:  VariableSegmentType,
						VariableKind: ResolvableObjectVariableKind,
						Renderer: NewGraphQLVariableResolveRenderer(&Object{
							Nullable: true,
							Fields: []*Field{
								{
									Name: []byte("address"),
									Value: &Object{
										Path:     []string{"address"},
										Nullable: false,
										Fields: []*Field{
											{
												Name: []byte("zip"),
												Value: &String{
													Path:     []string{"zip"},
													Nullable: false,
												},
											},
											{
												Name: []byte("items"),
												Value: &Array{
													Path:     []string{"items"},
													Nullable: false,
													Item: &Object{
														Nullable: false,
														Fields: []*Field{
															{
																Name: []byte("active"),
																Value: &Boolean{
																	Path: []string{"active"},
																},
															},
														},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("static"),
									Value: &StaticString{
										Path:  []string{"static"},
										Value: "static_string",
									},
								},
							},
						}),
					},
				},
			}
			ctx := &Context{
				ctx:       context.Background(),
				Variables: astjson.MustParseBytes([]byte(`{}`)),
			}

			cases := []struct {
				name      string
				input     string
				expected  string
				expectErr bool
			}{
				{
					name:     "data is present",
					input:    `{"name":"home","address":{"zip":"00000","items":[{"name":"home","active":true}]}}`,
					expected: `{"address":{"zip":"00000","items":[{"active":true}]},"static":"static_string"}`,
				},
				{
					name:      "data is missing",
					input:     `{"name":"home"}`,
					expectErr: true,
				},
				{
					name:      "partial data",
					input:     `{"name":"home","address":{},"static":"static_string"}`,
					expectErr: true,
				},
			}

			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					buf := &bytes.Buffer{}
					err := template.Render(ctx, astjson.MustParseBytes([]byte(c.input)), buf)
					if c.expectErr {
						assert.Error(t, err)
					} else {
						assert.NoError(t, err)
					}
					out := buf.String()
					assert.Equal(t, c.expected, out)
				})
			}
		})
	})

	t.Run("ListSegment", func(t *testing.T) {
		t.Run("nested objects", func(t *testing.T) {
			template := InputTemplate{
				Segments: []TemplateSegment{
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`{"representations":[`),
					},
					{
						SegmentType:  VariableSegmentType,
						VariableKind: ResolvableObjectVariableKind,
						Renderer: NewGraphQLVariableResolveRenderer(&Object{
							Nullable: false,
							Fields: []*Field{
								{
									Name: []byte("__typename"),
									Value: &String{
										Path:     []string{"__typename"},
										Nullable: false,
									},
								},
								{
									Name: []byte("address"),
									Value: &Object{
										Path:     []string{"address"},
										Nullable: false,
										Fields: []*Field{
											{
												Name: []byte("zip"),
												Value: &String{
													Path:     []string{"zip"},
													Nullable: false,
												},
											},
										},
									},
								},
							},
						}),
					},
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`]}`),
					},
				},
			}
			ctx := &Context{
				ctx:       context.Background(),
				Variables: astjson.MustParseBytes([]byte(`{}`)),
			}
			buf := &bytes.Buffer{}
			err := template.Render(ctx, astjson.MustParseBytes([]byte(`{"__typename":"Address","address":{"zip":"00000"}}`)), buf)
			assert.NoError(t, err)
			out := buf.String()
			assert.Equal(t, `{"representations":[{"__typename":"Address","address":{"zip":"00000"}}]}`, out)
		})
	})
}

type initTestVariableRenderer func() VariableRenderer

func useTestPlainVariableRenderer() initTestVariableRenderer {
	return func() VariableRenderer {
		return NewPlainVariableRenderer()
	}
}

func useTestJSONVariableRenderer() initTestVariableRenderer {
	return func() VariableRenderer {
		return NewJSONVariableRenderer()
	}
}
