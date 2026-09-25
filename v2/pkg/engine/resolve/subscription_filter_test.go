package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"
)

func TestSubscriptionFilter(t *testing.T) {
	t.Run("in: predicate is true (boolean)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":true}`)),
		}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: predicate is false (boolean)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":"false"}`)),
		}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: predicate is false due to type mismatch (boolean)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":"true"}`)),
		}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)

		c = &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":true}`)),
		}
		data = []byte(`{"event":"true"}`)
		skip, err = filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: predicate is true (float)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":1.13}`)),
		}
		data := []byte(`{"event":1.13}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: predicate is false due to type mismatch (float)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":"1.13"}`)),
		}
		data := []byte(`{"event":1.13}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)

		c = &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":1.13}`)),
		}
		data = []byte(`{"event":"1.13"}`)
		skip, err = filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: predicate is true (int)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":49}`)),
		}
		data := []byte(`{"event":49}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: predicate is false due to type mismatch (int)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":"49"}`)),
		}
		data := []byte(`{"event":49}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)

		c = &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":49}`)),
		}
		data = []byte(`{"event":"49"}`)
		skip, err = filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: predicate is false (float)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":"9.77"}`)),
		}
		data := []byte(`{"event":8.01}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: predicate is false (int)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":123}`)),
		}
		data := []byte(`{"event":321}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: predicate is true (boolean)", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":true}`)),
		}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: array predicate is false", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":["a","b"]}`)),
		}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: array predicate is false due to type mismatch", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":[1,"2"]}`)),
		}
		data := []byte(`{"event":2}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: array predicate is true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":["a","b","c"]}`)),
		}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("not in: predicate is true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			Not: &SubscriptionFilter{
				In: &SubscriptionFieldFilter{
					FieldPath: []string{"event"},
					Values: []InputTemplate{
						{
							Segments: []TemplateSegment{
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"var"},
									Renderer:           NewPlainVariableRenderer(),
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":"b"}`)),
		}
		data := []byte(`{"event":"b"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("not in: predicate is false", func(t *testing.T) {
		filter := &SubscriptionFilter{
			Not: &SubscriptionFilter{
				In: &SubscriptionFieldFilter{
					FieldPath: []string{"event"},
					Values: []InputTemplate{
						{
							Segments: []TemplateSegment{
								{
									SegmentType:        VariableSegmentType,
									VariableKind:       ContextVariableKind,
									VariableSourcePath: []string{"var"},
									Renderer:           NewPlainVariableRenderer(),
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":"b"}`)),
		}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("and: both in predicates are true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			And: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"first"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"second"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"first":"b","second":"c"}`)),
		}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("and: static predicates are true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			And: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`"b"`),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`"c"`),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("and: static predicate with bool as segment is true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			And: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`true`),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{}
		data := []byte(`{"eventX":true,"eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("static predicate with static segment type should not skip", func(t *testing.T) {
		filter := &SubscriptionFilter{
			And: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`{{ args.id }}`),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"id":1}`)),
		}
		data := []byte(`{"eventX":1,"eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("and: static predicate with NULL as segment is true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			And: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType: StaticSegmentType,
										Data:        []byte(`null`),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{}
		data := []byte(`{"eventX":null,"eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("and: first in predicate is false", func(t *testing.T) {
		filter := &SubscriptionFilter{
			And: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"first"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"second"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"first":"d","second":"c"}`)),
		}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("and: second in predicate is false", func(t *testing.T) {
		filter := &SubscriptionFilter{
			And: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"first"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"second"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"first":"b","unused":"c"}`)),
		}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("or: both in predicates are true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			Or: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"first"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"second"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"first":"b","second":"c"}`)),
		}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("or: first in predicate is true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			Or: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"first"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"second"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"first":"b","unused":"c"}`)),
		}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("or: second in predicate is true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			Or: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"first"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"second"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"third":"b","second":"c","fourth":1}`)),
		}
		data := []byte(`{"eventX":"b","eventY":"c","fourth":1}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("or: multiple predicates is true", func(t *testing.T) {
		filter := &SubscriptionFilter{
			Or: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"first"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"second"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"third":"b","second":"c"}`)),
		}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})

	t.Run("or: multiple segments with multiple is true and will always be compared byte-to-byte", func(t *testing.T) {
		filter := &SubscriptionFilter{
			Or: []SubscriptionFilter{
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventX"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"first"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
				{
					In: &SubscriptionFieldFilter{
						FieldPath: []string{"eventY"},
						Values: []InputTemplate{
							{
								Segments: []TemplateSegment{
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"second"},
										Renderer:           NewPlainVariableRenderer(),
									},
									{
										SegmentType:        VariableSegmentType,
										VariableKind:       ContextVariableKind,
										VariableSourcePath: []string{"fourth"},
										Renderer:           NewPlainVariableRenderer(),
									},
								},
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables: astjson.MustParseBytes([]byte(`{"third":"b","second":"c","fourth":1}`)),
		}
		data := []byte(`{"eventX":"b","eventY":"c1","fourth":1}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: remapped string variable matches", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"a"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"id":"abc"}`)),
			RemapVariables: map[string]string{"a": "id"},
		}
		data := []byte(`{"id":"abc"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: remapped string variable does not match", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"a"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"id":"abc"}`)),
			RemapVariables: map[string]string{"a": "id"},
		}
		data := []byte(`{"id":"xyz"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: remapped variable missing from variables", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"a"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{}`)),
			RemapVariables: map[string]string{"a": "id"},
		}
		data := []byte(`{"id":"abc"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: remapped variable type mismatch", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"a"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"id":"1"}`)),
			RemapVariables: map[string]string{"a": "id"},
		}
		data := []byte(`{"id":1}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: remapped float variable matches", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"a"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"var":1.13}`)),
			RemapVariables: map[string]string{"a": "var"},
		}
		data := []byte(`{"event":1.13}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: remapped boolean variable matches", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"event"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"a"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"var":true}`)),
			RemapVariables: map[string]string{"a": "var"},
		}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: no remap regression", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"var":"abc"}`)),
			RemapVariables: nil,
		}
		data := []byte(`{"id":"abc"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: remap present but path not in map", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"var"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"var":"abc"}`)),
			RemapVariables: map[string]string{"b": "other"},
		}
		data := []byte(`{"id":"abc"}`)
		skip, err := filter.SkipEvent(c, data)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in: remapped variable reads original name when new name collides", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"a"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"id":1,"a":"x"}`)),
			RemapVariables: map[string]string{"a": "id", "b": "a"},
		}
		skip, err := filter.SkipEvent(c, []byte(`{"id":1}`))
		assert.NoError(t, err)
		assert.Equal(t, false, skip)

		skip, err = filter.SkipEvent(c, []byte(`{"id":"x"}`))
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: remapped variable whose original name is another variable's new name", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"b"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"id":1,"a":"x"}`)),
			RemapVariables: map[string]string{"a": "id", "b": "a"},
		}
		skip, err := filter.SkipEvent(c, []byte(`{"id":"x"}`))
		assert.NoError(t, err)
		assert.Equal(t, false, skip)

		skip, err = filter.SkipEvent(c, []byte(`{"id":1}`))
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in: remapped nested variable path", func(t *testing.T) {
		filter := &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"id"},
				Values: []InputTemplate{
					{
						Segments: []TemplateSegment{
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ContextVariableKind,
								VariableSourcePath: []string{"b", "id"},
								Renderer:           NewPlainVariableRenderer(),
							},
						},
					},
				},
			},
		}
		c := &Context{
			Variables:      astjson.MustParseBytes([]byte(`{"id":1,"y":{"id":"x"}}`)),
			RemapVariables: map[string]string{"a": "id", "b": "y"},
		}
		skip, err := filter.SkipEvent(c, []byte(`{"id":"x"}`))
		assert.NoError(t, err)
		assert.Equal(t, false, skip)

		skip, err = filter.SkipEvent(c, []byte(`{"id":1}`))
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
}
