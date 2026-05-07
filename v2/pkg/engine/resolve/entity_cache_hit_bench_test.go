package resolve

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func BenchmarkEntityCacheHitPath(b *testing.B) {
	providesData := benchArticleProvidesData(2)

	for _, entityCount := range []int{1, 32} {
		b.Run("entities="+strconv.Itoa(entityCount), func(b *testing.B) {
			for _, tracing := range []bool{false, true} {
				tracingLabel := "tracing=off"
				if tracing {
					tracingLabel = "tracing=on"
				}

				b.Run("L1/"+tracingLabel, func(b *testing.B) {
					benchTryL1CacheLoadHitPath(b, entityCount, tracing, providesData)
				})
				b.Run("L2/"+tracingLabel, func(b *testing.B) {
					benchTryL2CacheLoadHitPath(b, entityCount, tracing, providesData)
				})
			}
		})
	}
}

func benchTryL1CacheLoadHitPath(b *testing.B, entityCount int, tracing bool, providesData *Object) {
	requestArena := arena.NewMonotonicArena(arena.WithMinBufferSize(128 * 1024))
	// Cache-backing arena: holds cached *astjson.Value across benchmark
	// iterations. We never Reset it so stored pointers stay valid.
	cacheArena := arena.NewMonotonicArena(arena.WithMinBufferSize(128 * 1024))

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.TracingOptions.Enable = tracing

	loader := &Loader{
		jsonArena: requestArena,
		ctx:       ctx,
		l1Cache:   map[string]*astjson.Value{},
	}

	cacheKeys := make([]*CacheKey, 0, entityCount)
	for i := range entityCount {
		id := "article-" + strconv.Itoa(i)
		cacheKey := "Article:" + id
		parsed, err := astjson.ParseBytesWithArena(cacheArena, benchArticleJSON(id))
		if err != nil {
			b.Fatalf("parse bench article: %v", err)
		}
		loader.l1Cache[cacheKey] = parsed
		cacheKeys = append(cacheKeys, &CacheKey{
			Keys: []string{cacheKey},
		})
	}

	info := &FetchInfo{
		OperationType:  ast.OperationTypeQuery,
		DataSourceName: "bench-subgraph",
		RootFields: []GraphCoordinate{
			{TypeName: "Article", FieldName: "_entities"},
		},
		ProvidesData: providesData,
	}

	res := &result{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		requestArena.Reset()
		resetCacheKeyState(cacheKeys)
		resetCacheResult(res)
		if !loader.tryL1CacheLoad(info, cacheKeys, res) {
			b.Fatal("expected complete L1 cache hit")
		}
	}
}

func benchTryL2CacheLoadHitPath(b *testing.B, entityCount int, tracing bool, providesData *Object) {
	requestArena := arena.NewMonotonicArena(arena.WithMinBufferSize(128 * 1024))
	cache := newBenchCache()

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.TracingOptions.Enable = tracing

	loader := &Loader{
		jsonArena: requestArena,
		ctx:       ctx,
	}

	l1Keys := make([]*CacheKey, 0, entityCount)
	l2Keys := make([]*CacheKey, 0, entityCount)
	for i := range entityCount {
		id := "article-" + strconv.Itoa(i)
		cacheKey := "Article:" + id
		cache.storage[cacheKey] = benchArticleJSON(id)
		l1Keys = append(l1Keys, &CacheKey{
			Keys: []string{cacheKey},
		})
		l2Keys = append(l2Keys, &CacheKey{
			Keys: []string{cacheKey},
		})
	}

	info := &FetchInfo{
		OperationType:  ast.OperationTypeQuery,
		DataSourceName: "bench-subgraph",
		RootFields: []GraphCoordinate{
			{TypeName: "Article", FieldName: "_entities"},
		},
		ProvidesData: providesData,
	}

	res := &result{
		cache:       cache,
		cacheConfig: FetchCacheConfiguration{TTL: time.Minute},
		l1CacheKeys: l1Keys,
		l2CacheKeys: l2Keys,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		requestArena.Reset()
		resetCacheKeyState(l1Keys)
		resetCacheKeyState(l2Keys)
		resetCacheResult(res)
		res.cache = cache
		res.cacheConfig = FetchCacheConfiguration{TTL: time.Minute}
		res.l1CacheKeys = l1Keys
		res.l2CacheKeys = l2Keys
		skipFetch, err := loader.tryL2CacheLoad(context.Background(), info, res)
		if err != nil {
			b.Fatal(err)
		}
		if !skipFetch {
			b.Fatal("expected complete L2 cache hit")
		}
	}
}

func resetCacheKeyState(keys []*CacheKey) {
	for _, key := range keys {
		if key == nil {
			continue
		}
		key.FromCache = nil
		key.missingKeys = nil
		key.cachedData = cachedData{}
	}
}

func resetCacheResult(res *result) {
	res.cachedItemIndices = nil
	res.fetchItemIndices = nil
	res.cacheSkipFetch = false
	res.cacheMustBeUpdated = false
	res.cacheTraceDurationSinceStartNano = 0
	res.cacheTraceDurationNano = 0
	res.cacheTraceEntityCount = 0
	res.cacheTraceL2GetAttempted = false
	res.cacheTraceL2SetAttempted = false
	res.cacheTraceL2SetNegAttempted = false
	res.cacheTraceL2GetDuration = 0
	res.cacheTraceL2SetDuration = 0
	res.cacheTraceL2SetNegDuration = 0
	res.cacheTraceL2GetError = ""
	res.cacheTraceL2SetError = ""
	res.cacheTraceL2SetNegError = ""
	res.cacheTraceL1Hits = 0
	res.cacheTraceL1Misses = 0
	res.cacheTraceRequestScopedHits = 0
	res.cacheTraceL2Hits = 0
	res.cacheTraceL2Misses = 0
	res.cacheTraceNegativeHits = 0
	res.cacheTraceShadowHit = false
	res.cacheTraceEntityDetails = nil
}

func benchArticleProvidesData(relatedDepth int) *Object {
	viewer := &Object{
		Nullable: true,
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Nullable: true}},
			{Name: []byte("name"), Value: &Scalar{Nullable: true}},
			{Name: []byte("email"), Value: &Scalar{Nullable: true}},
		},
	}

	article := &Object{
		Nullable: true,
		Fields: []*Field{
			{Name: []byte("__typename"), Value: &Scalar{Nullable: true}},
			{Name: []byte("id"), Value: &Scalar{Nullable: true}},
			{Name: []byte("title"), Value: &Scalar{Nullable: true}},
			{Name: []byte("body"), Value: &Scalar{Nullable: true}},
			{Name: []byte("tags"), Value: &Array{Nullable: true, Item: &Scalar{Nullable: true}}},
			{Name: []byte("viewCount"), Value: &Scalar{Nullable: true}},
			{Name: []byte("rating"), Value: &Scalar{Nullable: true}},
			{Name: []byte("reviewSummary"), Value: &Scalar{Nullable: true}},
			{Name: []byte("personalizedRecommendation"), Value: &Scalar{Nullable: true}},
			{Name: []byte("currentViewer"), Value: viewer},
		},
	}

	if relatedDepth > 0 {
		article.Fields = append(article.Fields, &Field{
			Name: []byte("relatedArticles"),
			Value: &Array{
				Nullable: true,
				Item:     benchArticleProvidesData(relatedDepth - 1),
			},
		})
	}

	ComputeHasAliases(article)
	return article
}

func benchArticleJSON(id string) []byte {
	return []byte(`{
		"__typename":"Article",
		"id":"` + id + `",
		"title":"Title ` + id + `",
		"body":"Body for ` + id + `",
		"tags":["graphql","cache","router"],
		"viewCount":12345,
		"rating":4.7,
		"reviewSummary":"Strong engagement and stable recommendation quality.",
		"personalizedRecommendation":"Recommended because the current viewer follows router performance topics.",
		"currentViewer":{
			"id":"viewer-1",
			"name":"Alice",
			"email":"alice@example.com"
		},
		"relatedArticles":[
			{
				"__typename":"Article",
				"id":"` + id + `-rel-1",
				"title":"Related 1",
				"body":"Nested body 1",
				"tags":["perf"],
				"viewCount":7,
				"rating":4.2,
				"reviewSummary":"Nested review 1",
				"personalizedRecommendation":"Nested recommendation 1",
				"currentViewer":{
					"id":"viewer-1",
					"name":"Alice",
					"email":"alice@example.com"
				},
				"relatedArticles":[
					{
						"__typename":"Article",
						"id":"` + id + `-rel-1a",
						"title":"Nested 1A",
						"body":"Deep body 1A",
						"tags":["deep"],
						"viewCount":3,
						"rating":4.0,
						"reviewSummary":"Deep review 1A",
						"personalizedRecommendation":"Deep recommendation 1A",
						"currentViewer":{
							"id":"viewer-1",
							"name":"Alice",
							"email":"alice@example.com"
						}
					}
				]
			},
			{
				"__typename":"Article",
				"id":"` + id + `-rel-2",
				"title":"Related 2",
				"body":"Nested body 2",
				"tags":["entity"],
				"viewCount":9,
				"rating":4.4,
				"reviewSummary":"Nested review 2",
				"personalizedRecommendation":"Nested recommendation 2",
				"currentViewer":{
					"id":"viewer-1",
					"name":"Alice",
					"email":"alice@example.com"
				},
				"relatedArticles":[
					{
						"__typename":"Article",
						"id":"` + id + `-rel-2a",
						"title":"Nested 2A",
						"body":"Deep body 2A",
						"tags":["deep"],
						"viewCount":4,
						"rating":4.1,
						"reviewSummary":"Deep review 2A",
						"personalizedRecommendation":"Deep recommendation 2A",
						"currentViewer":{
							"id":"viewer-1",
							"name":"Alice",
							"email":"alice@example.com"
						}
					}
				]
			}
		]
	}`)
}
