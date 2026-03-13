# E2E Test Conventions for `execution/engine`

## Inline everything

No `const` blocks, no named variables for expected values. Put all literal values (cache keys, hashes, byte sizes, query strings, expected responses) directly inline in assertions and setup code. Duplicate values across subtests rather than sharing — each subtest must be fully self-contained and readable without scrolling up.

```go
// CORRECT: literals inline in assertions
assert.Equal(t, normalizeSnapshot(resolve.CacheAnalyticsSnapshot{
    L2Reads: []resolve.CacheKeyEvent{
        {CacheKey: `{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", Kind: resolve.CacheKeyMiss, DataSource: "reviews"},
    },
    L2Writes: []resolve.CacheWriteEvent{
        {CacheKey: `11945571715631340836:{"__typename":"Product","key":{"upc":"top-1"}}`, EntityType: "Product", ByteSize: 177, DataSource: "reviews", CacheLevel: resolve.CacheLevelL2, TTL: 30 * time.Second},
    },
}), snap)

// WRONG: named constants defined above the test logic
const (
    keyProductTop1      = `{"__typename":"Product","key":{"upc":"top-1"}}`
    byteSizeProductTop1 = 177
)
```

## Inline setup too

Config structs (e.g. `SubgraphCachingConfigs`) should be defined inline in the setup call, not as named variables. Only keep variables for state that is mutated or referenced multiple times at runtime (e.g. `tracker`, `mockHeaders`, `setup`).

```go
// CORRECT: config inline
setup := federationtesting.NewFederationSetup(addCachingGateway(
    withCachingLoaderCache(map[string]resolve.LoaderCache{"default": NewFakeLoaderCache()}),
    withHTTPClient(&http.Client{Transport: tracker}),
    withSubgraphEntityCachingConfigs(engine.SubgraphCachingConfigs{
        {SubgraphName: "products", RootFieldCaching: plan.RootFieldCacheConfigurations{
            {TypeName: "Query", FieldName: "topProducts", CacheName: "default", TTL: 30 * time.Second},
        }},
    }),
))

// WRONG: named variable for config used only once
configs := engine.SubgraphCachingConfigs{...}
setup := federationtesting.NewFederationSetup(addCachingGateway(
    withSubgraphEntityCachingConfigs(configs),
))
```

## Self-contained subtests

Each `t.Run` subtest must be independently readable. No shared constants, variables, or helpers defined in the parent test function. Duplication across subtests is preferred over sharing.

## Inline queries

Use `QueryStringWithHeaders` with inline GraphQL query strings. Do not load queries from files.

```go
// CORRECT
resp, headers := gqlClient.QueryStringWithHeaders(ctx, setup.GatewayServer.URL,
    `query { topProducts { name reviews { body } } }`, nil, t)

// WRONG
resp := gqlClient.QueryWithHeaders(ctx, setup.GatewayServer.URL,
    cachingTestQueryPath("queries/my_query.query"), nil, t)
```

## Full snapshot assertions

Assert complete `CacheAnalyticsSnapshot` structs — not just the fields you care about. This catches unexpected events.

## Snapshot comments

Every event line in a snapshot assertion MUST have a brief comment explaining **why** that event occurred.

```go
// CORRECT: explains causation
{CacheKey: `...`, Kind: resolve.CacheKeyMiss, Shadow: true},  // Shadow L2 miss: cache empty
{CacheKey: `...`, Kind: resolve.CacheKeyMiss, Shadow: false}, // L2 miss: shadow mode not implemented for root fields

// WRONG: restates the field value
{CacheKey: `...`, Kind: resolve.CacheKeyMiss}, // this is a miss
```

## Always check every cache log

Every `defaultCache.ClearLog()` MUST be followed by `defaultCache.GetLog()` with full assertions BEFORE the next `ClearLog()` or end of test. Never clear a log without verifying its contents — skipped checks hide regressions.

## http.Header is a reference type

When returning `http.Header` from mocks, always `.Clone()` before returning. The HTTP client mutates the header map in-place (adds `Accept`, `Content-Type`, `Accept-Encoding`), which corrupts the mock's stored state and causes different hashes on subsequent calls.

```go
// CORRECT: clone before returning
func (m *mock) HeadersForSubgraph(name string) (http.Header, uint64) {
    h := m.headers[name]
    return h.Clone(), hashHeaders(h)
}

// WRONG: returns the same map reference — will be mutated by HTTP client
func (m *mock) HeadersForSubgraph(name string) (http.Header, uint64) {
    h := m.headers[name]
    return h, hashHeaders(h)
}
```