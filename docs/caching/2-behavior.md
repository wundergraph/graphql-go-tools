# Caching behavior reference

This document describes caching behavior only:
what the engine sends to subgraphs, what it stores, when it serves from cache,
and when it does not.
It contains no architecture and no implementation detail.
Every literal in this document (request bodies, keys, envelopes, tags)
is pinned verbatim by a test in `execution/cachingtesting`.

Companion documents:
`1-readme.md` (components, configuration, testing map).

## 1. The running example

A federated graph with four subgraphs, as composed in `execution/cachingtesting`:

- `users` — `Query.me: User`
- `products` — `User.favoriteProduct: Product`, `Query.products(first: Int): [Product]`
- `inventory` — `Product.stock: Int`, `Product.stockHistory(days: Int): [Int]`, `Product.warehouse: Warehouse`
- `reviews` — `Product.reviews: [Review]`

`Product` is an entity with `@key(fields: "upc")`.
Caching is enabled for the `inventory` subgraph with `DefaultTTL: time.Minute`.

## 2. One request, end to end

Client query:

```graphql
{ me { username favoriteProduct { upc stock } } }
```

The plan produces three fetches, executed as a chain:

1. `users` root fetch — `{ me { username id } }` style root query. Uncached (no config).
2. `products` entity fetch for `User.favoriteProduct`. Uncached.
3. `inventory` entity fetch for `Product.stock`. Cached.

### 2.1 First request — miss, fetch, write

Before the inventory fetch goes out, the cache renders the item's key
from the exact representation the fetch is about to send, and looks it up.
The store receives one batched read:

```text
GetMany ["v1:1:4f796e3bbd360fce"]   -> miss
```

The fetch then goes to the subgraph unchanged.
This is the complete request body sent to `inventory`:

```json
{"method":"POST","url":"http://inventory.service","header":{},
 "body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename stock}}}",
         "variables":{"representations":[{"__typename":"Product","upc":"1"}]}}}
```

The subgraph answers:

```json
{"data":{"_entities":[{"__typename":"Product","stock":5}]}}
```

The entity value is queued for writing,
and at request end the store receives one batched write:

```text
SetMany [
  Key:   v1:1:4f796e3bbd360fce
  Value: {"data":{"__typename":"Product","stock":5},"cc":{"ttl":60,"created":<unix>,"scope":"public"}}
  TTL:   60s
  Tags:  subgraph:1, type:1:Product, entity:1:Product:d3cc039c7a9789e7
]
```

The client receives the full response; caching never changes it:

```json
{"data":{"me":{"username":"jens","favoriteProduct":{"upc":"1","stock":5}}}}
```

### 2.2 Second request — hit, no network

The same lookup now hits:

```text
GetMany ["v1:1:4f796e3bbd360fce"]   -> hit
```

The inventory subgraph receives **nothing** — zero requests.
The cached value is spliced into the response at the same position,
and the response is byte-identical to the first one.
A pure hit writes nothing: with one key per fetch, the read key IS the write key.

The uncached `users` and `products` fetches go to the network on every request.

## 3. Cache keys

### 3.1 Entity keys

```text
preimage = v1:{subgraph}[:h<headerHash>]:{"__typename":T,"representation":{...}[,"args":"D"][,"partition":"P"]}
L2 key   = v1:{subgraph}[:h<headerHash>]:{16-hex xxhash64 of the preimage}
L1 key   =              {subgraph}:{"__typename":T,"representation":{...}[,"args":"D"]}
```

Worked example — the inventory fetch above (datasource ID `1`):

```text
preimage: v1:1:{"__typename":"Product","representation":{"upc":"1"}}
L2 key:   v1:1:4f796e3bbd360fce
L1 key:   1:{"__typename":"Product","representation":{"upc":"1"}}
```

The `representation` object is exactly what the fetch sends on the wire:
the entity's `@key` fields **plus its `@requires` values**, in node order, objects recursing.
So a derived field cached under one `@requires` input
can never serve a request whose fresh input differs.

Key rendering rules:

- `__typename` comes from the plan node when it is static
  (the `@interfaceObject` remap bakes the interface name in),
  else from the item itself.
- A field whose `OnTypeNames` exclude the item's concrete type is skipped.
- A `null` or absent key field makes the key unrenderable —
  the item is a plain miss and nothing is written.
- Numbers and strings of the same literal render identically (`1` and `"1"`),
  but `1` and `1.0` stay distinct keys (extra miss, never wrong data).
- An empty representation (`{}`) is unrenderable — it would collide across all entities.

### 3.2 The `args` segment

Present when the fetch's selection contains parameterized fields.

```graphql
query($days: Int!) { me { favoriteProduct { upc stockHistory(days: $days) } } }
```

With `days: 3` and `days: 1`, the two requests produce **two independent entries**:
the preimage gains `"args":"<16-hex digest>"`,
a digest over the sorted normalized field paths with each field's argument-value suffix.
Interleaved traffic with different argument values never replaces each other's entries.

Aliased multi-variant selections stay one entry:

```graphql
{ euro: price(currency: "EURO") usd: price(currency: "USD") }
```

is a single fetch, a single key, a single cacheable entry —
both variants live inside the stored value under argument-suffixed names (§5).

### 3.3 The `partition` segment (private fetches only)

A statically-private fetch appends `"partition":"<sha256>"` to the preimage.
The hashed value is source-tagged before hashing:

- provider identity: `sha256("i:" + identity)`
- forwarded-header fallback: `sha256("h:" + <16-hex header hash>)`

so one source's literal text can never forge the other's partition.
Two requesters therefore hold two disjoint entries for the same entity.

### 3.4 The `h` segment (public vary-by-headers)

On a **public** fetch with `IncludeSubgraphHeaders`,
the forwarded subgraph header hash joins the visible prefix:
`v1:products:h<16-hex>:<digest>`.
Requests forwarding different headers never share entries.
On a private fetch the same hash is an identity source instead (§3.3),
so one fetch is partitioned by exactly one mechanism.

### 3.5 Root-field keys

A cached root field keys the **whole fetch response** under:

```text
preimage = v1:{subgraph}[:h<hex>]:{TypeName}.{FieldName}:{name-sorted variables}[,"partition":"P"]
L2 key   = v1:{subgraph}[:h<hex>]:{16-hex xxhash64}
```

Example coordinate: `Query.topProducts` with variables `{"first":1}`.

- The query text is **not** part of the key:
  alias-variant operations over the same field and variables share one entry.
- Variables are name-sorted, so argument order never fragments entries.
- Operations are normalized with variable extraction before planning,
  so inline literals become variables and cannot collide under one key.
- Root fields are L2-only; they have no L1 key.

**Known deviation (bug, see [6-open-issues.md](6-open-issues.md) and [5-testing-on-js-subgraphs.md](5-testing-on-js-subgraphs.md)):**
the variables in the preimage are the request's WHOLE variable set under the CLIENT's names,
not the arguments the fetch sends.
Two calls of one cached root field in a single operation therefore collide on one key
(a correctness bug — the surviving entry serves both aliases),
and operations differing only in variable naming fragment into separate entries
(a hit-rate cost).
Entity keys are unaffected.

### 3.6 L1 keys

The L1 key is the raw preimage core — no version, no header hash, no partition, no digest.
Nothing persists (so no format version is needed),
and one request has one requester (so no partition is needed).

### 3.7 What never affects keys

Client query shape beyond the fetch identity:
field order, whitespace, aliases, operation names, fragment layout;
for ENTITY keys, variable names too.
(Root-field keys currently do vary by client variable names — the known deviation in §3.5.)

## 4. When a value is cached — and when not

A fetched value is written when ALL of the following hold:

| Condition | Detail |
|---|---|
| The fetch is cache-configured | resolved from the config cascade; `nil` config = the fetch is invisible to the cache |
| The fetch succeeded | `!FetchFailed && !HasErrors && data != null`; a transport failure, empty body, parse failure, GraphQL error, or null data writes nothing |
| The response is storable | no `no-store` / `no-cache` in the response `Cache-Control` |
| A TTL resolved > 0 | header `max-age`, else type declaration, else subgraph default, else global default; clamped by `MaxTTL` |
| The key rendered | every referenced representation field present and non-null |
| Privacy is satisfiable | a private fetch has a partition; a runtime-only `private` header on a public fetch drops the write |

A value is NOT cached when any of these fail. The specific cases:

- **No configuration**: the subgraph (or type, or root-field coordinate) is not configured.
  The fetch runs exactly as in a non-caching build.
- **Type veto**: a type declared `@cache(maxAge: 0)` gets no cache configuration at all,
  negative sentinel included, whatever the subgraph level says.
- **Subgraph veto**: `Enabled: false` disables the whole subgraph.
- **Mixed root fields**: a fetch resolving several root fields with differing cache settings
  declines caching entirely (rare — the planner isolates cached root fields into their own fetches).
- **`no-store` / `no-cache` response header**: nothing is written, in either layer.
- **Resolved TTL <= 0**: no L2 entry (L1 is unaffected — it is request-scoped).
- **Failed fetch / GraphQL errors**: nothing is written; existing entries are untouched.
- **Runtime-only `private`**: a statically-public fetch answered with `Cache-Control: private`
  skips both store writes (L1 keeps serving within the request);
  `CacheObserver.OnUncacheablePrivate("response-private")` fires.
- **Private without identity**: a statically-private fetch with no provider identity
  and no header fallback skips L2 entirely — reads and writes;
  `OnUncacheablePrivate("no-identity")` fires.
- **Unrenderable key**: the item is fetched and merged normally, just never cached.

## 5. What is stored

Every L2 entry is one envelope:

```json
{"data": {"__typename":"Product","stock":5},
 "cc":   {"ttl": 60, "created": 1785852117, "scope": "public"}}
```

- `cc.ttl` is the lifetime the entry was written with (whole seconds),
  `cc.created` the unix second of the write,
  `cc.scope` `"public"` or `"private"`.
- Storage is replace-only: one write moment per entry, one honest TTL.
  There is no merge-into-existing-entry; a newer write replaces the older entry whole.

`data` is stored **normalized**:

- Aliases are resolved to schema field names.
  A query selecting `productName: name` stores the value under `name`,
  and a later query selecting `label: name` is served from the same entry, under `label`.
- Parameterized fields are stored under argument-suffixed names:
  `stockHistory(days: 3)` stores as `stockHistory_<16-hex value hash>`,
  so `stockHistory(days: 1)` can never be served a `days: 3` value.
- Fields the plan did not select (key fields, `__typename`) are preserved as-is.

The **negative sentinel** is `{"data":null,"cc":{...}}`:
written when a successful entity fetch resolves an entity to nothing
(`_entities` returns null for it) and `NegativeCacheTTL > 0`.
On a later read the entity is served as null without a subgraph fetch.
Its lifetime is always `NegativeCacheTTL`, never the header `max-age` —
how long a value stays fresh says nothing about how long an entity stays nonexistent.

## 6. When a cached entry is served — and when it is a miss

An entry that exists in the store is served only when ALL of:

- **It decodes**: undecodable or foreign bytes are a miss the next write repairs — never an error.
- **The scope matches**: an entry recorded `"private"` read through a public key derivation
  (or the reverse) is configuration drift; it is discarded as a miss
  and counted via `OnScopeMismatch`, in both directions.
- **It covers the selection**: the entry contains every field the fetch provides,
  with null accepted only where the schema allows it.
  Wider entries serve narrower queries;
  a narrower entry is a miss and the refetch replaces it with the wider value.
  Argument-suffixed names make an argument-mismatched field a miss, not a wrong hit.
- **It is fresh**: the store reports expired entries as absent;
  remaining freshness also feeds the client cache answer (§10).

A negative sentinel short-circuits: the entity is known missing, no coverage walk runs.

## 7. Batch entity fetches

A batch fetch (`_entities` over N array items) renders **one key per unique representation**.
Duplicate representations collapse into one bucket.
The lookup is still one store call:

```text
GetMany [key1, key2, ... keyN]
```

Three outcomes:

- **Full hit** — every bucket covered: the network fetch is skipped entirely,
  each cached value spliced at its original positions.
- **Full miss / partial without the flag** — any bucket missing and
  `EnablePartialCacheLoad` off: the full batch goes to the subgraph,
  and every entity in the response is written back (one entry per unique representation).
- **Partial** — `EnablePartialCacheLoad` on: covered buckets serve from cache,
  and the subgraph receives a REDUCED representations list containing only the missing ones.

The reduced request is literal.
With product 1's reviews cached and product 2's missing,
the reviews subgraph receives exactly:

```json
{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename reviews {body}}}}",
 "variables":{"representations":[{"__typename":"Product","upc":"2"}]}}
```

The response realigns to the original batch positions:
cached and fetched entities appear exactly where they belong.
A failed partial fetch still serves the covered buckets;
only the fetched subset is lost to the normal error path.

One `Cache-Control` header governs a whole batch response —
every entity entry written from it shares the resolved TTL.

Mixed TTLs across subgraphs partition naturally:
when only the short-TTL subgraph's entry expires,
only that subgraph is re-fetched and the fresh entries keep serving.

## 8. TTL resolution

Per fetch result, first match wins:

```text
Cache-Control: no-store | no-cache   -> not cached, either layer
Cache-Control: max-age=N             -> entry TTL = N seconds (runtime truth)
else: type declaration MaxAge        -> static tier, most specific first
      else subgraph DefaultTTL
      else global DefaultTTL            (root fields: coordinate TTL first)
resolved TTL <= 0                    -> no L2 entry
MaxTTL (when set)                    -> clamps whichever source won
```

Header parsing behavior:

- Recognized: `max-age`, `no-store`, `no-cache`, `private`. Everything else is skipped,
  `s-maxage` deliberately included.
- Parsing never fails: a malformed directive is dropped and the static tier decides.
- The FIRST well-formed `max-age` wins; `no-store`/`no-cache`/`private` are sticky.
- `no-cache` is treated as `no-store`: this cache cannot revalidate,
  so the conservative reading is not to store.
- A `max-age` LOWER than the static TTL wins too — the header is truth, not a floor.

## 9. Privacy behavior

A fetch is **statically private** when its subgraph `Scope`,
type declaration scope, or root-field entry scope says PRIVATE.
Private widens down the cascade, never up.

| Situation | Behavior |
|---|---|
| Private fetch, provider names the requester | entries live under the requester's partition (§3.3); full read/write behavior per requester |
| Private fetch, no provider identity, `IncludeSubgraphHeaders` set | the forwarded-header hash is the identity; same partitioning |
| Private fetch, no identity at all | the fetch skips L2 entirely — no read, no write, sentinel included; L1 keeps working unpartitioned; `OnUncacheablePrivate("no-identity")` |
| Public fetch, response says `Cache-Control: private` | both store writes dropped, L1 keeps serving within the request; `OnUncacheablePrivate("response-private")` |
| Private fetch, response says `private` | the header confirms the declaration; entries written into the partition as usual |

Envelopes record their scope, and a scope mismatch on read is a counted miss (§6).
L1 is per-request and therefore inherently per-requester: it is never partitioned.
Tags and traces never carry identity material.

## 10. The client cache answer

After resolution the router reads `Context.CacheResponseInfo()` and emits headers.
The answer folds one contribution per fetch:

- A **served** entry contributes what is LEFT of its lifetime
  (a 60s entry served 20s after its write contributes 40s).
- A **freshly written** entry contributes the TTL it was written with.
- An **uncacheable** part — `no-store`, zero TTL, failed fetch, private-without-identity,
  or any fetch executed without cache configuration — contributes zero,
  and one zero makes the whole response `NoStore`.
- An **L1-served** part contributes nothing (its originating fetch already folded).

Resulting client policy:

- `HasPolicy == false` — the operation touched no cache-configured fetch,
  or the response used `@defer` (headers ship with the first frame,
  so no answer can describe the whole response). Emit nothing.
- `NoStore == true` — emit `Cache-Control: no-store`.
- Otherwise — emit `Cache-Control: max-age=<MaxAge>[, private]`,
  where `MaxAge` is the MINIMUM across contributions and counts down between requests.
- `Tags` (only with `EmitCdnTags`) is the sorted union of the contributing entries' tags.

## 11. Invalidation tags

Every written entry carries, in this order:

```text
subgraph:{name}
type:{name}:{TypeName}
entity:{name}:{TypeName}:{16-hex digest of the @key fields only}   // entity entries
```

Example (inventory datasource `1`, Product upc "1"):
`subgraph:1`, `type:1:Product`, `entity:1:Product:d3cc039c7a9789e7`.

The entity digest covers the `@key` subset alone —
no `@requires` values, no argument digest, no partition —
so every variant of one entity (argument variants, requires variants, private partitions)
shares the tag, and a single purge clears them all, sentinels included.
Root-field entries carry the two coarse tags with the coordinate's type (`type:{name}:Query`).
Tags ride on the store item, never inside the stored value.

## 12. Shadow mode

`ShadowMode` reads but never serves:

1. The lookup runs normally and stashes what WOULD have been served.
2. The fetch always goes to the network, in full.
3. The fresh `data` is byte-compared against the stashed value
   (`CacheObserver.CompareShadow`; envelope metadata excluded — only staleness of data counts).
4. The fresh value overwrites the entry.

The response is always the origin's.
This is the safe-rollout mode: it measures staleness without risking a stale serve.
Root fields shadow asymmetrically: read, then force-refetch, no compare
(their whole-response value has no per-item compare surface).

## 13. The request-lifetime layer (L1)

- Holds decoded, normalized values under raw preimage keys; no envelopes, no bytes.
- Scope: ONE request, including all of its `@defer` groups.
  An initial-response fetch that stores `{stock, warehouse}` serves a later
  defer-group fetch that needs only `{stock}` — with zero store operations.
- Reads apply the same coverage and negative-sentinel rules as L2.
- Only `no-store` disables L1 writes; a zero TTL does not (L1 has no TTL).
- Values are isolated by structural copy at both boundaries,
  so later merges can never corrupt a stored value.
- Private data lands in L1 unpartitioned by design:
  one request is one requester.

## 14. Store interaction contract

- Exactly ONE `GetMany` per cache-participating fetch, however many items it batches.
- Exactly ONE `SetMany` per request, at request end, carrying every deferred write.
- A `GetMany` error (or a mis-sized answer) degrades to an all-miss; the fetch falls back to the origin.
- A `SetMany` error drops the writes.
- Neither ever fails the request; both fire `CacheObserver.OnStoreError(op, subgraph, keyCount, err)`.
- Store outage behavior is deliberate origin fallback —
  pair caching with subgraph rate limiting operationally.

## 15. Observability signals

| Signal | Fires when |
|---|---|
| `OnFetchObserved` | once per cache-touched fetch, at request end (or at trace flush): decision, served-from layer, per-item key / hit / negative / remaining TTL |
| `CompareShadow` | once per shadow-mode fetch result, before the write-back |
| `OnStoreError` | once per failed store round trip |
| `OnUncacheablePrivate` | at most once per fetch, reason `"response-private"` or `"no-identity"` |
| `OnScopeMismatch` | once per discarded wrong-scope entry |

The ART trace carries a per-fetch cache section with the same data.
`HashAnalyticsKeys` hashes key material in all trace output.
