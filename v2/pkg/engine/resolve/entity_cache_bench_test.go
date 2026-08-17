package resolve

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/entitycaching"
)

// benchCache is a full hit for every key it is asked for, so the benchmark
// measures the assembly in entityCacheLookup rather than a cache implementation.
type benchCache struct {
	items map[string]entitycaching.Item
}

func (c *benchCache) GetMany(_ context.Context, keys []string) (map[string]entitycaching.Item, error) {
	found := make(map[string]entitycaching.Item, len(keys))
	for _, key := range keys {
		if item, ok := c.items[key]; ok {
			found[key] = item
		}
	}
	return found, nil
}

func (c *benchCache) SetMany(_ context.Context, _ []entitycaching.Item) error { return nil }

func newEntityCacheLookupBench(entities, entitySize int) (*Loader, *preparedFetch) {
	keys := make([]string, entities)
	items := make(map[string]entitycaching.Item, entities)

	value := make([]byte, entitySize)
	value[0] = '{'
	for i := 1; i < entitySize-1; i++ {
		value[i] = 'a'
	}
	value[entitySize-1] = '}'

	for i := range keys {
		// The same shape entitycaching.Key produces: v1:<16 hex>:<16 hex>.
		keys[i] = fmt.Sprintf("v1:%016x:%016x", uint64(i), uint64(0xabcdef))
		items[keys[i]] = entitycaching.Item{Key: keys[i], Value: value, TTL: time.Minute}
	}

	ctx := &Context{ctx: context.Background()}
	ctx.SetEntityCache(&benchCache{items: items}, time.Minute, nil)

	l := &Loader{ctx: ctx}
	prepared := &preparedFetch{entityCacheKeys: keys, res: &result{}}
	return l, prepared
}

func BenchmarkEntityCacheLookup(b *testing.B) {
	cases := []struct {
		entities   int
		entitySize int
	}{
		{entities: 1, entitySize: 128},
		{entities: 10, entitySize: 128},
		{entities: 50, entitySize: 256},
		{entities: 200, entitySize: 512},
	}

	for _, tc := range cases {
		b.Run(fmt.Sprintf("entities=%d/size=%d", tc.entities, tc.entitySize), func(b *testing.B) {
			l, prepared := newEntityCacheLookupBench(tc.entities, tc.entitySize)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !l.entityCacheLookup(prepared) {
					b.Fatal("expected a full cache hit")
				}
			}
		})
	}
}
