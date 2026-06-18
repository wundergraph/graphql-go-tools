package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

func TestLoaderCacheTransformStructuralCopy(t *testing.T) {
	t.Run("round trip aliases through normalized and denormalized copies", func(t *testing.T) {
		loader, release := newLoaderCacheTransformTestLoader()
		defer release()

		provides := &Object{
			Fields: []*Field{
				{
					Name:         []byte("fullName"),
					OriginalName: []byte("name"),
					Value:        &String{},
				},
				{
					Name:         []byte("years"),
					OriginalName: []byte("age"),
					Value:        &Integer{},
				},
			},
		}
		input := parseLoaderCacheTransformTestValue(t, loader, `{"fullName":"Bob","years":3}`)

		normalized := loader.structuralCopyNormalized(input, provides)
		assert.Equal(t, `{"name":"Bob","age":3}`, string(normalized.MarshalTo(nil)))

		denormalized := loader.structuralCopyDenormalized(normalized, provides)
		assert.Equal(t, `{"fullName":"Bob","years":3}`, string(denormalized.MarshalTo(nil)))
	})

	t.Run("projection drops unlisted fields", func(t *testing.T) {
		loader, release := newLoaderCacheTransformTestLoader()
		defer release()

		provides := nameAndAgeProvidesData()
		input := parseLoaderCacheTransformTestValue(t, loader, `{"fullName":"Bob","years":3,"secret":"x"}`)

		normalized := loader.structuralCopyNormalized(input, provides)
		assert.Equal(t, `{"name":"Bob","age":3}`, string(normalized.MarshalTo(nil)))
	})

	t.Run("passthrough keeps unlisted fields", func(t *testing.T) {
		loader, release := newLoaderCacheTransformTestLoader()
		defer release()

		provides := nameAndAgeProvidesData()
		input := parseLoaderCacheTransformTestValue(t, loader, `{"fullName":"Bob","years":3,"secret":"x"}`)

		normalized := loader.structuralCopyNormalizedPassthrough(input, provides)
		assert.Equal(t, `{"name":"Bob","age":3,"secret":"x"}`, string(normalized.MarshalTo(nil)))

		denormalized := loader.structuralCopyDenormalizedPassthrough(normalized, provides)
		assert.Equal(t, `{"fullName":"Bob","years":3,"secret":"x"}`, string(denormalized.MarshalTo(nil)))
	})

	t.Run("nested object rename", func(t *testing.T) {
		loader, release := newLoaderCacheTransformTestLoader()
		defer release()

		provides := &Object{
			Fields: []*Field{
				{
					Name: []byte("profile"),
					Value: &Object{
						Fields: []*Field{
							{
								Name:         []byte("displayName"),
								OriginalName: []byte("name"),
								Value:        &String{},
							},
						},
					},
				},
			},
		}
		input := parseLoaderCacheTransformTestValue(t, loader, `{"profile":{"displayName":"Bob"}}`)

		normalized := loader.structuralCopyNormalized(input, provides)
		assert.Equal(t, `{"profile":{"name":"Bob"}}`, string(normalized.MarshalTo(nil)))
	})

	t.Run("array of object rename", func(t *testing.T) {
		loader, release := newLoaderCacheTransformTestLoader()
		defer release()

		provides := &Object{
			Fields: []*Field{
				{
					Name: []byte("friends"),
					Value: &Array{
						Item: &Object{
							Fields: []*Field{
								{
									Name:         []byte("fullName"),
									OriginalName: []byte("name"),
									Value:        &String{},
								},
							},
						},
					},
				},
			},
		}
		input := parseLoaderCacheTransformTestValue(t, loader, `{"friends":[{"fullName":"Ada"},{"fullName":"Lin"}]}`)

		normalized := loader.structuralCopyNormalized(input, provides)
		assert.Equal(t, `{"friends":[{"name":"Ada"},{"name":"Lin"}]}`, string(normalized.MarshalTo(nil)))
	})

	t.Run("nil provides uses plain structural copy", func(t *testing.T) {
		loader, release := newLoaderCacheTransformTestLoader()
		defer release()

		input := parseLoaderCacheTransformTestValue(t, loader, `{"fullName":"Bob","years":3,"secret":"x"}`)

		copied := loader.structuralCopyNormalized(input, nil)
		assert.Equal(t, `{"fullName":"Bob","years":3,"secret":"x"}`, string(copied.MarshalTo(nil)))
	})
}

func newLoaderCacheTransformTestLoader() (*Loader, func()) {
	pool := arena.NewArenaPool()
	item := pool.Acquire(0)
	loader := &Loader{
		jsonArena: item.Arena,
	}
	return loader, func() {
		pool.Release(item)
	}
}

func parseLoaderCacheTransformTestValue(t *testing.T, loader *Loader, data string) *astjson.Value {
	t.Helper()

	value, err := astjson.ParseBytesWithArena(loader.jsonArena, []byte(data))
	assert.NoError(t, err)
	return value
}

func nameAndAgeProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{
				Name:         []byte("fullName"),
				OriginalName: []byte("name"),
				Value:        &String{},
			},
			{
				Name:         []byte("years"),
				OriginalName: []byte("age"),
				Value:        &Integer{},
			},
		},
	}
}
