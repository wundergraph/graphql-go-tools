package cachingtesting

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// benchStore is the benchmark L2 store: a plain RWMutex map with NO op
// logging, so the measurements show the loader + cache hot paths and not the
// test double's bookkeeping.
type benchStore struct {
	mu   sync.RWMutex
	data map[string]benchEntry
}

type benchEntry struct {
	value     []byte
	expiresAt time.Time
}

func newBenchStore() *benchStore {
	return &benchStore{data: make(map[string]benchEntry)}
}

func (s *benchStore) GetMany(_ context.Context, keys []string) ([]cache.Entry, error) {
	entries := make([]cache.Entry, len(keys))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i, key := range keys {
		entry, ok := s.data[key]
		if !ok {
			continue
		}
		entries[i] = cache.Entry{
			Value:        entry.value,
			RemainingTTL: time.Until(entry.expiresAt),
			OK:           true,
		}
	}
	return entries, nil
}

func (s *benchStore) SetMany(_ context.Context, items []cache.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.data[item.Key] = benchEntry{value: item.Value, expiresAt: time.Now().Add(item.TTL)}
	}
	return nil
}

// discardWriter drops the rendered response (rendering cost stays included,
// buffer growth noise does not).
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// benchResolve plans once and resolves the SAME plan per iteration — the
// router's production mode (plans are cached and reused) — with ONE resolver
// and one controller across iterations and a fresh Context per request.
func benchResolve(b *testing.B, result PlanResult, controller resolve.CacheController) {
	b.Helper()
	r := resolve.New(context.Background(), resolve.ResolverOptions{MaxConcurrency: 1024})
	b.ReportAllocs()
	for b.Loop() {
		ctx := resolve.NewContext(context.Background())
		if controller != nil {
			ctx.SetCacheController(controller)
		}
		if _, err := r.ResolveGraphQLResponse(ctx, result.Response, nil, discardWriter{}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLoader measures the loader hot paths per fetch type, with and
// without caching. All caching variants are steady-state HITS (primed before
// the loop) unless named otherwise.
func BenchmarkLoader(b *testing.B) {
	entityQuery := `{ me { username favoriteProduct { upc stock } } }`
	entityResponses := map[string]string{
		"users":                        `{"data":{"me":{"__typename":"User","id":"u1","username":"jens"}}}`,
		"products:me":                  `{"data":{"_entities":[{"__typename":"User","favoriteProduct":{"__typename":"Product","upc":"1"}}]}}`,
		"inventory:me.favoriteProduct": `{"data":{"_entities":[{"__typename":"Product","stock":5}]}}`,
	}
	entityCaching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"inventory": {DefaultTTL: ptr(time.Hour)},
		},
	}

	batchQuery := `{ products(first: 2) { upc reviews { body } } }`
	batchResponses := map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`,
		"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]},{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`,
	}
	batchCaching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"reviews": {DefaultTTL: ptr(time.Hour)},
		},
	}
	partialCaching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"reviews": {
				DefaultTTL:             ptr(time.Hour),
				EnablePartialCacheLoad: ptr(true),
			},
		},
	}

	rootFieldQuery := `query($first: Int!) { products(first: $first) { upc name } }`
	rootFieldResponses := map[string]string{
		"products": `{"data":{"products":[{"__typename":"Product","upc":"1","name":"Table"}]}}`,
	}
	rootFieldCaching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"products": {
				RootFields: []cacheconfig.RootFieldCacheConfig{
					{TypeName: "Query", FieldName: "products", TTL: time.Hour},
				},
			},
		},
	}

	chainQuery := `{ deal(id: "d1") { product { name reviews { product { name } } } } }`
	chainResponses := map[string]string{
		"deals":                 `{"data":{"deal":{"__typename":"Deal","id":"d1","product":{"__typename":"Product","sku":"S1"}}}}`,
		"products:deal.product": `{"data":{"_entities":[{"__typename":"Product","name":"Table","upc":"1"}]}}`,
		"reviews":               `{"data":{"_entities":[{"__typename":"Product","reviews":[{"__typename":"Review","product":{"__typename":"Product","upc":"1"}}]}]}}`,
		"products:deal.product.reviews.@.product": `{"data":{"_entities":[{"__typename":"Product","name":"Table"}]}}`,
	}
	chainCaching := &cacheconfig.CachingConfiguration{
		Subgraphs: map[string]cacheconfig.SubgraphCacheConfig{
			"products": {DefaultTTL: ptr(time.Hour)},
		},
	}

	prime := func(b *testing.B, result PlanResult, controller resolve.CacheController) {
		b.Helper()
		r := resolve.New(context.Background(), resolve.ResolverOptions{MaxConcurrency: 1024})
		ctx := resolve.NewContext(context.Background())
		ctx.SetCacheController(controller)
		if _, err := r.ResolveGraphQLResponse(ctx, result.Response, nil, discardWriter{}); err != nil {
			b.Fatal(err)
		}
	}

	b.Run("entity/no-cache", func(b *testing.B) {
		benchResolve(b, Plan(b, entityQuery, nil, entityResponses), nil)
	})
	b.Run("entity/l2-hit", func(b *testing.B) {
		result := Plan(b, entityQuery, entityCaching, entityResponses)
		controller := cache.NewController(newBenchStore(), nil)
		prime(b, result, controller)
		benchResolve(b, result, controller)
	})
	b.Run("entity/l2-miss-write", func(b *testing.B) {
		// Every iteration misses and writes: missStore never returns hits and
		// swallows writes, keeping the steady-state miss+write path measurable
		// without unbounded map growth.
		result := Plan(b, entityQuery, entityCaching, entityResponses)
		controller := cache.NewController(missStore{}, nil)
		benchResolve(b, result, controller)
	})

	b.Run("batch/no-cache", func(b *testing.B) {
		benchResolve(b, Plan(b, batchQuery, nil, batchResponses), nil)
	})
	b.Run("batch/l2-hit", func(b *testing.B) {
		result := Plan(b, batchQuery, batchCaching, batchResponses)
		controller := cache.NewController(newBenchStore(), nil)
		prime(b, result, controller)
		benchResolve(b, result, controller)
	})
	b.Run("batch/partial-hit", func(b *testing.B) {
		// One of two entities primed: every iteration filters the batch and
		// fetches the other (the canned response only matches the REDUCED
		// request, so the partial path is proven exercised).
		primeResult := Plan(b, `{ products(first: 1) { upc reviews { body } } }`, partialCaching, map[string]string{
			"products": `{"data":{"products":[{"__typename":"Product","upc":"1"}]}}`,
			"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Solid"}]}]}}`,
		})
		store := newBenchStore()
		controller := cache.NewController(store, nil)
		prime(b, primeResult, controller)
		// partialShapeStore serves the primed upc 1 entry but drops the
		// iteration's upc 2 write-backs, so the one-of-two-primed partial
		// shape holds for every iteration.
		result := Plan(b, batchQuery, partialCaching, map[string]string{
			"products": `{"data":{"products":[{"__typename":"Product","upc":"1"},{"__typename":"Product","upc":"2"}]}}`,
			"reviews":  `{"data":{"_entities":[{"__typename":"Product","reviews":[{"body":"Wobbly"}]}]}}`,
		})
		partial := &partialShapeStore{inner: store}
		partialController := cache.NewController(partial, nil)
		benchResolve(b, result, partialController)
	})

	b.Run("rootfield/no-cache", func(b *testing.B) {
		result := Plan(b, rootFieldQuery, nil, rootFieldResponses)
		benchResolveWithVariables(b, result, `{"first":1}`, nil)
	})
	b.Run("rootfield/l2-hit", func(b *testing.B) {
		result := Plan(b, rootFieldQuery, rootFieldCaching, rootFieldResponses)
		controller := cache.NewController(newBenchStore(), nil)
		primeWithVariables(b, result, `{"first":1}`, controller)
		benchResolveWithVariables(b, result, `{"first":1}`, controller)
	})

	b.Run("chain/no-cache", func(b *testing.B) {
		benchResolve(b, Plan(b, chainQuery, nil, chainResponses), nil)
	})
	b.Run("chain/l2-hit", func(b *testing.B) {
		// Two entity fetches reach ONE Product through different
		// representations ({sku} then {upc}), so they own independent entries
		// and each hits L2 per request; the reviews hop between them stays
		// uncached.
		result := Plan(b, chainQuery, chainCaching, chainResponses)
		controller := cache.NewController(newBenchStore(), nil)
		prime(b, result, controller)
		benchResolve(b, result, controller)
	})
}

// missStore never hits and swallows writes: the steady-state miss+write path
// without unbounded map growth.
type missStore struct{}

func (missStore) GetMany(_ context.Context, keys []string) ([]cache.Entry, error) {
	return make([]cache.Entry, len(keys)), nil
}

func (missStore) SetMany(context.Context, []cache.Item) error { return nil }

// partialShapeStore serves reads from the primed inner store but drops writes,
// so the one-of-two-primed partial shape holds for every iteration.
type partialShapeStore struct {
	inner *benchStore
}

func (s *partialShapeStore) GetMany(ctx context.Context, keys []string) ([]cache.Entry, error) {
	return s.inner.GetMany(ctx, keys)
}

func (s *partialShapeStore) SetMany(context.Context, []cache.Item) error { return nil }

func benchResolveWithVariables(b *testing.B, result PlanResult, variables string, controller resolve.CacheController) {
	b.Helper()
	r := resolve.New(context.Background(), resolve.ResolverOptions{MaxConcurrency: 1024})
	b.ReportAllocs()
	for b.Loop() {
		ctx := resolve.NewContext(context.Background())
		ctx.Variables = astjson.MustParseBytes([]byte(variables))
		if controller != nil {
			ctx.SetCacheController(controller)
		}
		if _, err := r.ResolveGraphQLResponse(ctx, result.Response, nil, discardWriter{}); err != nil {
			b.Fatal(err)
		}
	}
}

func primeWithVariables(b *testing.B, result PlanResult, variables string, controller resolve.CacheController) {
	b.Helper()
	r := resolve.New(context.Background(), resolve.ResolverOptions{MaxConcurrency: 1024})
	ctx := resolve.NewContext(context.Background())
	ctx.Variables = astjson.MustParseBytes([]byte(variables))
	ctx.SetCacheController(controller)
	if _, err := r.ResolveGraphQLResponse(ctx, result.Response, nil, discardWriter{}); err != nil {
		b.Fatal(err)
	}
}
