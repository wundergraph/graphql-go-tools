package resolve

import (
	"context"
	"net/http"
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/pkg/fastbuffer"
)

func TestInputTemplate_Render(t *testing.T) {
	runTest := func(t *testing.T, initRenderer initTestVariableRenderer, variables string, sourcePath []string, jsonSchema string, expectErr bool, expected string) {
		t.Helper()

		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: sourcePath,
					Renderer:           initRenderer(jsonSchema),
				},
			},
		}
		ctx := &Context{
			Variables: []byte(variables),
		}
		buf := fastbuffer.New()
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
			runTest(t, renderer, `{"foo":"bar"}`, []string{"foo"}, `{"type":"string"}`, false, `bar`)
		})
		t.Run("boolean scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":true}`, []string{"foo"}, `{"type":"boolean"}`, false, "true")
		})
		t.Run("nested string", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":"value"}}`, []string{"foo", "bar"}, `{"type":"string"}`, false, `value`)
		})
		t.Run("json object pass through", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":"baz"}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"string"}}}`, false, `{"bar":"baz"}`)
		})
		t.Run("json object as graphql object", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":"baz"}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"string"}}}`, false, `{"bar":"baz"}`)
		})
		t.Run("json object as graphql object with null on required type", func(t *testing.T) {
			runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":["string"]}`, true, ``)
		})
		t.Run("json object as graphql object with null", func(t *testing.T) {
			runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":["string","null"]}`, false, `null`)
		})
		t.Run("json object as graphql object with number", func(t *testing.T) {
			runTest(t, renderer, `{"foo":123}`, []string{"foo"}, `{"type":"integer"}`, false, `123`)
		})
		t.Run("json object as graphql object with invalid number", func(t *testing.T) {
			runTest(t, renderer, `{"foo":123}`, []string{"foo"}, `{"type":"string"}`, true, "")
		})
		t.Run("json object as graphql object with boolean", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":true}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"boolean"}}}`, false, `{"bar":true}`)
		})
		t.Run("json object as graphql object with number", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":123}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"integer"}}}`, false, `{"bar":123}`)
		})
		t.Run("json object as graphql object with float", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":1.23}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"number"}}}`, false, `{"bar":1.23}`)
		})
		t.Run("json object as graphql object with nesting", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":{"baz":"bat"}}}`, []string{"foo"}, `{"type":"object","properties":{"bar":{"type":"object","properties":{"baz":{"type":"string"}}}}}`, false, `{"bar":{"baz":"bat"}}`)
		})
		t.Run("json object as graphql object with single array", func(t *testing.T) {
			runTest(t, renderer, `{"foo":["bar"]}`, []string{"foo"}, `{"type":"array","item":{"type":"string"}}`, false, `["bar"]`)
		})
		t.Run("json object as graphql object with array", func(t *testing.T) {
			runTest(t, renderer, `{"foo":["bar","baz"]}`, []string{"foo"}, `{"type":"array","item":{"type":"string"}}`, false, `["bar","baz"]`)
		})
		t.Run("json object as graphql object with object array", func(t *testing.T) {
			runTest(t, renderer, `{"foo":[{"bar":"baz"},{"bar":"bat"}]}`, []string{"foo"}, `{"type":"array","item":{"type":"object","properties":{"bar":{"type":"string"}}}}`, false, `[{"bar":"baz"},{"bar":"bat"}]`)
		})
	})

	t.Run("json renderer", func(t *testing.T) {
		renderer := useTestJSONVariableRenderer()
		t.Run("string scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":"bar"}`, []string{"foo"}, `{"type":"string"}`, false, `"bar"`)
		})
		t.Run("boolean scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":true}`, []string{"foo"}, `{"type":"boolean"}`, false, "true")
		})
		t.Run("number scalar", func(t *testing.T) {
			runTest(t, renderer, `{"foo":1}`, []string{"foo"}, `{"type":"number"}`, false, "1")
		})
		t.Run("nested string", func(t *testing.T) {
			runTest(t, renderer, `{"foo":{"bar":"value"}}`, []string{"foo", "bar"}, `{"type":"string"}`, false, `"value"`)
		})
		t.Run("on required scalars", func(t *testing.T) {
			t.Run("error on required string scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":"string"}`, true, ``)
			})
			t.Run("error on required int scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":"integer"}`, true, ``)
			})
			t.Run("error on required float scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":"number"}`, true, ``)
			})
			t.Run("error on required boolean scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":"boolean"}`, true, ``)
			})
		})
		t.Run("on non-required scalars", func(t *testing.T) {
			t.Run("null on non-required string scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":["string","null"]}`, false, `null`)
			})
			t.Run("null on non-required int scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":["integer","null"]}`, false, `null`)
			})
			t.Run("null on non-required float scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":["number","null"]}`, false, `null`)
			})
			t.Run("null on non-required boolean scalar", func(t *testing.T) {
				runTest(t, renderer, `{"foo":null}`, []string{"foo"}, `{"type":["boolean","null"]}`, false, `null`)
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
			Variables: []byte(`{"a":["foo","bar"]}`),
		}
		buf := fastbuffer.New()
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
			Variables: []byte(`{"a":[1,2,3]}`),
		}
		buf := fastbuffer.New()
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, "1,2,3", out)
	})
	t.Run("array with default render int", func(t *testing.T) {
		template := InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: []string{"a"},
					Renderer:           NewGraphQLVariableRenderer(`{"type":"array","items":{"type":"number"}}`),
				},
			},
		}
		ctx := &Context{
			Variables: []byte(`{"a":[1,2,3]}`),
		}
		buf := fastbuffer.New()
		err := template.Render(ctx, nil, buf)
		assert.NoError(t, err)
		out := buf.String()
		assert.Equal(t, "[1,2,3]", out)
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
				Variables: []byte(""),
			}
			buf := fastbuffer.New()
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
				Variables: []byte(""),
				Request: Request{
					Header: http.Header{"Auth": []string{"value"}},
				},
			}
			buf := fastbuffer.New()
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
				Variables: []byte(""),
				Request: Request{
					Header: http.Header{"Auth": []string{"value1", "value2"}},
				},
			}
			buf := fastbuffer.New()
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
						Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
					},
					{
						SegmentType: StaticSegmentType,
						Data:        []byte(`}`),
					},
				},
			}
			ctx := &Context{
				ctx:       context.Background(),
				Variables: []byte(""),
			}
			buf := fastbuffer.New()
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
							Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
						},
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`}`),
						},
					},
					SetTemplateOutputToNullOnVariableNull: true,
				}
				ctx := &Context{
					Variables: []byte(""),
				}
				buf := fastbuffer.New()
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
							Renderer:           NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
						},
						{
							SegmentType: StaticSegmentType,
							Data:        []byte(`}`),
						},
					},
					SetTemplateOutputToNullOnVariableNull: true,
				}
				ctx := &Context{
					Variables: []byte(`{"x":null}`),
				}
				buf := fastbuffer.New()
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
					Variables: []byte(""),
				}
				buf := fastbuffer.New()
				err := template.Render(ctx, nil, buf)
				assert.NoError(t, err)
				out := buf.String()
				assert.Equal(t, `{"key":""}`, out)
			})
		})
	})
}
