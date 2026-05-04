package resolve

import (
	"bytes"
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)

		c = &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":true}`)),
		}
		buf = &bytes.Buffer{}
		data = []byte(`{"event":"true"}`)
		skip, err = filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":1.13}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":1.13}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)

		c = &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":1.13}`)),
		}
		buf = &bytes.Buffer{}
		data = []byte(`{"event":"1.13"}`)
		skip, err = filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":49}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":49}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)

		c = &Context{
			Variables: astjson.MustParseBytes([]byte(`{"var":49}`)),
		}
		buf = &bytes.Buffer{}
		data = []byte(`{"event":"49"}`)
		skip, err = filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":8.01}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":321}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":true}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":2}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"b"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":true,"eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":1,"eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":null,"eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c","fourth":1}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
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
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c1","fourth":1}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
}

func TestSubscriptionFieldFilterBypassIfValuesNull(t *testing.T) {
	variableValue := func(path ...string) InputTemplate {
		return InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: path,
					Renderer:           NewPlainVariableRenderer(),
				},
			},
		}
	}

	multiSegmentValue := func(path ...string) InputTemplate {
		return InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType:        VariableSegmentType,
					VariableKind:       ContextVariableKind,
					VariableSourcePath: path,
					Renderer:           NewPlainVariableRenderer(),
				},
				{
					SegmentType: StaticSegmentType,
					Data:        []byte(`-suffix`),
				},
			},
		}
	}

	staticValue := func(value string) InputTemplate {
		return InputTemplate{
			Segments: []TemplateSegment{
				{
					SegmentType: StaticSegmentType,
					Data:        []byte(value),
				},
			},
		}
	}

	tests := []struct {
		name               string
		bypassIfValuesNull bool
		variables          string
		data               string
		values             []InputTemplate
		expectedSkip       bool
	}{
		{
			name:         "flag false variable absent skips",
			variables:    `{}`,
			data:         `{"productName":"foo"}`,
			values:       []InputTemplate{variableValue("args", "x")},
			expectedSkip: true,
		},
		{
			name:         "flag false variable explicit null matches event null",
			variables:    `{"args":{"x":null}}`,
			data:         `{"productName":null}`,
			values:       []InputTemplate{variableValue("args", "x")},
			expectedSkip: false,
		},
		{
			name:         "flag false variable explicit null skips non-null event",
			variables:    `{"args":{"x":null}}`,
			data:         `{"productName":"foo"}`,
			values:       []InputTemplate{variableValue("args", "x")},
			expectedSkip: true,
		},
		{
			name:               "flag true variable absent bypasses when event field exists",
			bypassIfValuesNull: true,
			variables:          `{}`,
			data:               `{"productName":"foo"}`,
			values:             []InputTemplate{variableValue("args", "x")},
			expectedSkip:       false,
		},
		{
			name:               "flag true variable absent bypasses before event lookup",
			bypassIfValuesNull: true,
			variables:          `{}`,
			data:               `{"other":"foo"}`,
			values:             []InputTemplate{variableValue("args", "x")},
			expectedSkip:       false,
		},
		{
			name:               "flag true variable explicit null bypasses",
			bypassIfValuesNull: true,
			variables:          `{"args":{"x":null}}`,
			data:               `{"productName":"foo"}`,
			values:             []InputTemplate{variableValue("args", "x")},
			expectedSkip:       false,
		},
		{
			name:               "flag true variable value matches",
			bypassIfValuesNull: true,
			variables:          `{"args":{"x":"foo"}}`,
			data:               `{"productName":"foo"}`,
			values:             []InputTemplate{variableValue("args", "x")},
			expectedSkip:       false,
		},
		{
			name:               "flag true variable value mismatches",
			bypassIfValuesNull: true,
			variables:          `{"args":{"x":"foo"}}`,
			data:               `{"productName":"bar"}`,
			values:             []InputTemplate{variableValue("args", "x")},
			expectedSkip:       true,
		},
		{
			name:               "flag true multi-segment value with nil variable does not bypass",
			bypassIfValuesNull: true,
			variables:          `{}`,
			data:               `{"productName":"bar"}`,
			values:             []InputTemplate{multiSegmentValue("args", "a")},
			expectedSkip:       true,
		},
		{
			name:               "flag true empty array variable does not bypass",
			bypassIfValuesNull: true,
			variables:          `{"args":{"x":[]}}`,
			data:               `{"productName":"foo"}`,
			values:             []InputTemplate{variableValue("args", "x")},
			expectedSkip:       true,
		},
		{
			name:               "flag true array variable matches element",
			bypassIfValuesNull: true,
			variables:          `{"args":{"x":["foo","bar"]}}`,
			data:               `{"productName":"bar"}`,
			values:             []InputTemplate{variableValue("args", "x")},
			expectedSkip:       false,
		},
		{
			name:               "flag true array containing null does not bypass",
			bypassIfValuesNull: true,
			variables:          `{"args":{"x":[null]}}`,
			data:               `{"productName":"foo"}`,
			values:             []InputTemplate{variableValue("args", "x")},
			expectedSkip:       true,
		},
		{
			name:               "flag true mixed values bypass on null variable",
			bypassIfValuesNull: true,
			variables:          `{"args":{"x":null}}`,
			data:               `{"productName":"bar"}`,
			values:             []InputTemplate{staticValue(`"static"`), variableValue("args", "x")},
			expectedSkip:       false,
		},
		{
			name:               "flag true static-only value matches as before",
			bypassIfValuesNull: true,
			variables:          `{}`,
			data:               `{"productName":"foo"}`,
			values:             []InputTemplate{staticValue(`"foo"`)},
			expectedSkip:       false,
		},
		{
			name:               "flag true static-only value mismatches as before",
			bypassIfValuesNull: true,
			variables:          `{}`,
			data:               `{"productName":"bar"}`,
			values:             []InputTemplate{staticValue(`"foo"`)},
			expectedSkip:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &SubscriptionFieldFilter{
				FieldPath:          []string{"productName"},
				Values:             tt.values,
				BypassIfValuesNull: tt.bypassIfValuesNull,
			}
			c := &Context{
				Variables: astjson.MustParseBytes([]byte(tt.variables)),
			}
			buf := &bytes.Buffer{}

			skip, err := filter.SkipEvent(c, []byte(tt.data), buf)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedSkip, skip)
		})
	}
}
