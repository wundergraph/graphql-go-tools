package resolve

import (
	"bytes"
	"context"
	"testing"
)

var cachingOverheadBenchSink string

func BenchmarkCachingOverhead_Sequential(b *testing.B) {
	b.Run("Disabled", func(b *testing.B) {
		benchCachingOverheadSequential(b, &FetchCacheConfiguration{}, ResolverOptions{}, func(ctx *Context) {
			ctx.ExecutionOptions.Caching = CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: false,
			}
		})
	})
	b.Run("ConfiguredButDisabled", func(b *testing.B) {
		cache := batchUserNameCacheConfig(false)
		cache.UseL1Cache = true
		benchCachingOverheadSequential(b, cache, ResolverOptions{}, func(ctx *Context) {
			ctx.ExecutionOptions.Caching = CachingOptions{
				EnableL1Cache: false,
				EnableL2Cache: false,
			}
		})
	})
	b.Run("L1Only", func(b *testing.B) {
		cache := batchUserNameCacheConfig(false)
		cache.EnableL2Cache = false
		cache.UseL1Cache = true
		benchCachingOverheadSequential(b, cache, ResolverOptions{}, func(ctx *Context) {
			ctx.ExecutionOptions.Caching = CachingOptions{
				EnableL1Cache: true,
				EnableL2Cache: false,
			}
		})
	})
	b.Run("L1L2_Miss", func(b *testing.B) {
		cacheBackend := newBenchLoaderCache()
		cache := batchUserNameCacheConfig(false)
		cache.UseL1Cache = true
		benchCachingOverheadSequential(b, cache, ResolverOptions{
			Caches: map[string]LoaderCache{"default": cacheBackend},
		}, func(ctx *Context) {
			cacheBackend.Clear()
			ctx.ExecutionOptions.Caching = CachingOptions{
				EnableL1Cache: true,
				EnableL2Cache: true,
			}
		})
	})
	b.Run("L1L2_Hit", func(b *testing.B) {
		cacheBackend := newBenchLoaderCache()
		options := ResolverOptions{Caches: map[string]LoaderCache{"default": cacheBackend}}
		cache := batchUserNameCacheConfig(false)
		cache.UseL1Cache = true
		response, cancel := newCachingOverheadBenchResponse(cache)
		defer cancel()
		resolver, shutdown := newCachingOverheadBenchResolver(options)
		defer shutdown()
		benchResolveGraphQLResponse(b, resolver, response, func(ctx *Context) {
			ctx.ExecutionOptions.Caching = CachingOptions{
				EnableL1Cache: true,
				EnableL2Cache: true,
			}
		})
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			cachingOverheadBenchSink = benchResolveGraphQLResponse(b, resolver, response, func(ctx *Context) {
				ctx.ExecutionOptions.Caching = CachingOptions{
					EnableL1Cache: true,
					EnableL2Cache: true,
				}
			})
		}
	})
}

func BenchmarkCachingOverhead_Analytics(b *testing.B) {
	cacheBackend := newBenchLoaderCache()
	options := ResolverOptions{Caches: map[string]LoaderCache{"default": cacheBackend}}
	cache := batchUserNameCacheConfig(false)
	cache.UseL1Cache = true
	response, cancel := newCachingOverheadBenchResponse(cache)
	defer cancel()
	resolver, shutdown := newCachingOverheadBenchResolver(options)
	defer shutdown()
	benchResolveGraphQLResponse(b, resolver, response, func(ctx *Context) {
		ctx.ExecutionOptions.Caching = CachingOptions{
			EnableL1Cache: true,
			EnableL2Cache: true,
		}
	})

	b.Run("AnalyticsOff", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			cachingOverheadBenchSink = benchResolveGraphQLResponse(b, resolver, response, func(ctx *Context) {
				ctx.ExecutionOptions.Caching = CachingOptions{
					EnableL1Cache: true,
					EnableL2Cache: true,
				}
			})
		}
	})
	b.Run("AnalyticsOn", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			cachingOverheadBenchSink = benchResolveGraphQLResponse(b, resolver, response, func(ctx *Context) {
				ctx.ExecutionOptions.Caching = CachingOptions{
					EnableL1Cache:        true,
					EnableL2Cache:        true,
					EnableCacheAnalytics: true,
				}
			})
		}
	})
}

func benchCachingOverheadSequential(b *testing.B, cache *FetchCacheConfiguration, options ResolverOptions, configure func(*Context)) {
	b.Helper()

	response, cancel := newCachingOverheadBenchResponse(cache)
	defer cancel()
	resolver, shutdown := newCachingOverheadBenchResolver(options)
	defer shutdown()

	benchResolveGraphQLResponse(b, resolver, response, configure)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cachingOverheadBenchSink = benchResolveGraphQLResponse(b, resolver, response, configure)
	}
}

func newCachingOverheadBenchResponse(cache *FetchCacheConfiguration) (*GraphQLResponse, context.CancelFunc) {
	root := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"users":[{"__typename":"User","id":"1"},{"__typename":"User","id":"2"}]}}`),
		},
	}
	entities := &countingCacheTestDataSource{
		responses: [][]byte{
			[]byte(`{"data":{"_entities":[{"__typename":"User","id":"1","name":"Ada"},{"__typename":"User","id":"2","name":"Grace"}]}}`),
		},
	}
	return cacheTestBatchEntityResponse(root, entities, cache), func() {}
}

func newCachingOverheadBenchResolver(options ResolverOptions) (*Resolver, context.CancelFunc) {
	resolverCtx, cancel := context.WithCancel(context.Background())
	resolver := New(resolverCtx, options)
	return resolver, cancel
}

func benchResolveGraphQLResponse(b *testing.B, resolver *Resolver, response *GraphQLResponse, configure func(*Context)) string {
	b.Helper()

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	if configure != nil {
		configure(ctx)
	}

	var out bytes.Buffer
	_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, &out)
	if err != nil {
		b.Fatal(err)
	}
	return out.String()
}

type benchLoaderCache struct {
	entries map[string][]byte
}

func newBenchLoaderCache() *benchLoaderCache {
	return &benchLoaderCache{entries: map[string][]byte{}}
}

func (c *benchLoaderCache) Get(_ context.Context, keys []string) ([]*CacheEntry, error) {
	entries := make([]*CacheEntry, len(keys))
	for i, key := range keys {
		value, ok := c.entries[key]
		if !ok {
			continue
		}
		entries[i] = &CacheEntry{
			Key:   key,
			Value: append([]byte(nil), value...),
		}
	}
	return entries, nil
}

func (c *benchLoaderCache) Set(_ context.Context, entries []*CacheEntry) error {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		c.entries[entry.Key] = append([]byte(nil), entry.Value...)
	}
	return nil
}

func (c *benchLoaderCache) Delete(_ context.Context, keys []string) error {
	for _, key := range keys {
		delete(c.entries, key)
	}
	return nil
}

func (c *benchLoaderCache) Clear() {
	for key := range c.entries {
		delete(c.entries, key)
	}
}
