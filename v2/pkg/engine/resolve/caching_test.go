package resolve

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/astjson"
)

func TestCachingRenderRootQueryCacheKeyTemplate(t *testing.T) {
	t.Run("single field single argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			Fields: []CacheKeyQueryRootField{
				{
					Name: "droid",
					Args: []CacheKeyQueryRootFieldArgument{
						{
							Name: "id",
							Variables: InputTemplate{
								SetTemplateOutputToNullOnVariableNull: true,
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"id"},
										Renderer:           NewCacheKeyVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":1}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		out := &bytes.Buffer{}
		err := tmpl.RenderCacheKey(ctx, data, out)
		assert.NoError(t, err)
		assert.Equal(t, `Query::droid:id:1`, out.String())
	})

	t.Run("single field multiple arguments", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			Fields: []CacheKeyQueryRootField{
				{
					Name: "search",
					Args: []CacheKeyQueryRootFieldArgument{
						{
							Name: "term",
							Variables: InputTemplate{
								SetTemplateOutputToNullOnVariableNull: true,
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"term"},
										Renderer:           NewCacheKeyVariableRenderer(),
									},
								},
							},
						},
						{
							Name: "max",
							Variables: InputTemplate{
								SetTemplateOutputToNullOnVariableNull: true,
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"max"},
										Renderer:           NewCacheKeyVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"term":"C3PO","max":10}`),
			ctx:       context.Background(),
		}
		out := &bytes.Buffer{}
		data := astjson.MustParse(`{}`)
		err := tmpl.RenderCacheKey(ctx, data, out)
		assert.NoError(t, err)
		assert.Equal(t, `Query::search:term:C3PO:max:10`, out.String())
	})

	t.Run("multiple fields single argument each", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			Fields: []CacheKeyQueryRootField{
				{
					Name: "droid",
					Args: []CacheKeyQueryRootFieldArgument{
						{
							Name: "id",
							Variables: InputTemplate{
								SetTemplateOutputToNullOnVariableNull: true,
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"id"},
										Renderer:           NewCacheKeyVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					Name: "user",
					Args: []CacheKeyQueryRootFieldArgument{
						{
							Name: "name",
							Variables: InputTemplate{
								SetTemplateOutputToNullOnVariableNull: true,
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"name"},
										Renderer:           NewCacheKeyVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":1,"name":"john"}`),
			ctx:       context.Background(),
		}
		out := &bytes.Buffer{}
		data := astjson.MustParse(`{}`)
		err := tmpl.RenderCacheKey(ctx, data, out)
		assert.NoError(t, err)
		assert.Equal(t, `Query::droid:id:1::user:name:john`, out.String())
	})
}
