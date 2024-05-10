package resolve

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubscriptionFilter(t *testing.T) {
	t.Run("in allow", func(t *testing.T) {
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
			Variables: []byte(`{"var":"b"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"b"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("in skip", func(t *testing.T) {
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
			Variables: []byte(`{"var":"b"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in array skip", func(t *testing.T) {
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
			Variables: []byte(`{"var":["a","b"]}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("in array allow", func(t *testing.T) {
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
			Variables: []byte(`{"var":["a","b","c"]}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("not in skip", func(t *testing.T) {
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
			Variables: []byte(`{"var":"b"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"event":"b"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("and allow", func(t *testing.T) {
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
			Variables: []byte(`{"first":"b","second":"c"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("and allow static", func(t *testing.T) {
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
										Data:        []byte("b"),
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
										Data:        []byte("c"),
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
	t.Run("and skip", func(t *testing.T) {
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
			Variables: []byte(`{"first":"b","second":"d"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("and skip 2", func(t *testing.T) {
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
			Variables: []byte(`{"first":"b","third":"c"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
	t.Run("or allow", func(t *testing.T) {
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
			Variables: []byte(`{"first":"b","second":"c"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("or allow differing", func(t *testing.T) {
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
			Variables: []byte(`{"first":"b","third":"c"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, false, skip)
	})
	t.Run("or skip", func(t *testing.T) {
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
			Variables: []byte(`{"fourth":"b","third":"c"}`),
		}
		buf := &bytes.Buffer{}
		data := []byte(`{"eventX":"b","eventY":"c"}`)
		skip, err := filter.SkipEvent(c, data, buf)
		assert.NoError(t, err)
		assert.Equal(t, true, skip)
	})
}
