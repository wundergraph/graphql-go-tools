# Redis store adapter — design report

What the router-repository Redis adapter must implement to back the engine's response caching,
and what the invalidation endpoint on top of it needs.
Sources: the engine store contract (`v2/pkg/engine/cache/controller.go`),
the behavior spec (`2-behavior.md` §11, §14),
and the live-verified Apollo Router response-cache internals
(FINDINGS.md in the apollo-response-caching investigation workspace, Router 2.17.0).

## 1. The contract the adapter fills

```go
type Store interface {
    GetMany(ctx context.Context, keys []string) ([]Entry, error)
    SetMany(ctx context.Context, items []Item) error
}
// Item{Key string, Value []byte, TTL time.Duration, Tags []string}
// Entry{Value []byte, RemainingTTL time.Duration, OK bool}
```

Facts the design leans on:

- Exactly ONE `GetMany` per cache-participating fetch (N keys for an N-bucket batch),
  exactly ONE `SetMany` per request, flushed at request end — after the response is written,
  so `SetMany` latency is off the client path but still inside the request goroutine.
- `GetMany` must return exactly one `Entry` per key, in key order;
  a mis-sized answer is treated as an error by the engine.
- `Entry.RemainingTTL` may be 0 ("store cannot report one");
  the engine computes served freshness from the envelope's own `cc.ttl`/`cc.created`,
  and uses `RemainingTTL` for trace/analytics only.
- Errors never fail a request: a `GetMany` error degrades to an all-miss,
  a `SetMany` error drops the writes, both fire `CacheObserver.OnStoreError`.
  The adapter should therefore fail FAST, not retry into the request path.
- Values are opaque bytes to the engine (the envelope), replace-only, one TTL per entry.
- Every `Item` carries the stable tag vocabulary, coarsest first:
  `subgraph:{name}`, `type:{name}:{TypeName}`,
  and for entities `entity:{name}:{TypeName}:{16-hex @key digest}`.
- Entry keys are `v1:{subgraph}[:h<hex>]:{16-hex digest}` —
  the subgraph is a plaintext prefix of every key, which matters for §6.

## 2. Read path — `GetMany`

Options:

- `MGET k1..kN` — one round trip, O(N), values in request order, nil for absent keys.
  Cluster caveat: `MGET` is a single multi-key command;
  the server rejects it with `CROSSSLOT` when keys hash to different slots,
  and go-redis routes the whole command by one key rather than splitting it.
- Pipelined per-key `GET` — one logical round trip in standalone mode,
  and in cluster mode go-redis groups the pipeline's commands per node
  and runs the sub-pipelines concurrently, so it is cluster-safe by construction.

`RemainingTTL`: adding pipelined `PTTL` per key doubles the read command count
for a value the engine only surfaces in traces.
The envelope already carries the entry's own freshness record,
so the correctness-relevant remaining freshness never depends on the store's answer.

**Recommendation:**
implement `GetMany` as a pipelined `GET` per key (works identically standalone and cluster),
special-case `len(keys) == 1` to a plain `GET`,
and return `RemainingTTL: 0` for every entry.
Offer an opt-in `ReportRemainingTTL` knob that adds pipelined `PTTL` calls
for installations that want exact trace numbers, default off.
Treat a per-key error inside the pipeline as a miss for that key (`OK: false`) rather than
failing the whole batch, but return a batch-level error when the connection itself failed —
the engine turns that into an all-miss anyway.

## 3. Write path — `SetMany`

Each item is independent: entries are replace-only and self-consistent,
so there is no cross-item atomicity requirement.
A torn batch (some items written, some dropped) leaves every written entry valid;
the dropped ones are re-written by a later request.

**Recommendation:**
one pipeline per `SetMany` containing, per item:

```text
SET    {key} {value} PX {ttl_ms}
ZADD   {index(tag)} GT {expiry_unix_ms} {key}      # per indexed tag, see §4
EXPIRE {index(tag)} {tag_ttl_s} NX                  # first write: the ZADD-created key has NO expiry
EXPIRE {index(tag)} {tag_ttl_s} GT                  # later writes: only ever EXTEND the index lifetime
```

`tag_ttl_s` is the item's TTL plus a small slack (Apollo uses entry TTL + 1s),
so an index never outlives its longest-lived member by more than the slack.
Use `PX` (relative) rather than `EXAT` for the value:
the engine hands the adapter a duration, not a deadline,
and relative expiry avoids clock-skew coupling between router and Redis.
The index score, by contrast, must be an absolute expiry timestamp (computed once, `now + TTL`),
because purging and pruning compare scores against wall-clock time.
`ZADD GT` (Redis ≥ 6.2) keeps a re-written entry from SHORTENING
an index's view of a member's lifetime when an older longer-lived write already registered it.
The `EXPIRE` pair (`NX`/`GT` options are Redis ≥ 7.0) is needed because
`EXPIRE ... GT` alone never fires on a key WITHOUT an expiry —
no-expiry counts as infinite — so the freshly ZADD-created index would live forever.
No MULTI/EXEC: a value written whose index member is lost only costs invalidation precision
for that one entry until it expires, which matches the engine's best-effort store stance.

## 4. Tag indexes

The invalidation endpoint needs tag → keys.
Two candidate structures per tag:

- `SET` of entry keys — cheap adds, but members never expire;
  a busy graph accumulates dead members forever without an external sweeper.
- `ZSET` with score = absolute expiry unix time — same add cost class,
  members carry their own expiry information,
  pruning is one `ZREMRANGEBYSCORE {index} -inf {now}`,
  and purge-time reads can skip already-expired members for free.

Apollo's live-verified choice is the ZSET:
members are full entry keys, score is the absolute expiry timestamp,
and the index key's own TTL is entry TTL + 1s.

**Recommendation:**
ZSET per indexed tag, key `v1:idx:{tag}` — the `v1:` prefix keeps index and entry keyspaces
under one format version, and `idx` becomes a RESERVED subgraph name
(a datasource actually named `idx` would collide with the index namespace;
reject or rename it at adapter construction).
Score = expiry unix milliseconds.
Prune lazily on every purge (`ZREMRANGEBYSCORE` first)
plus a low-frequency background maintenance loop (Apollo runs one) —
e.g. iterate known index keys via `SCAN MATCH v1:idx:*` every few minutes
and `ZREMRANGEBYSCORE` each; cheap, incremental, and safe to skip under load.
Memory: one member costs the key string (~40–80 bytes for our digest keys)
plus roughly 60–90 bytes of ZSET overhead;
at one million entries × 2 indexed tags that is order-of ~300 MB —
which is exactly why the subgraph tag should NOT be a ZSET (§6).

Mirror Apollo's documented caveat: indexes are additive-only.
Enabling an index later does not retro-index existing entries;
document that a flush (or waiting out the TTLs) is required.

## 5. Delete-by-tag — the invalidation endpoint's primitive

**Recommendation:** `InvalidateByTags(ctx, tags []string) (deleted int64, err error)`:

```text
for each tag:
  ZREMRANGEBYSCORE v1:idx:{tag} -inf {now_ms}          # prune expired members first
  loop:
    keys = ZRANGE v1:idx:{tag} 0 511                    # batches of ~512
    if empty: break
    UNLINK keys...                                      # count += reply
    ZREM v1:idx:{tag} keys...
  UNLINK v1:idx:{tag}
```

- `UNLINK` over `DEL`: same semantics, memory reclaim happens on a background thread,
  so purging a large tag cannot stall the event loop other traffic runs on.
- Batching bounds both the reply size and the multi-key command width
  (and in cluster mode go-redis fans the `UNLINK` batch out per node).
- Members deleted here stay ORPHANED in the OTHER tags' ZSETs
  (an entity purge leaves the key listed under its type and subgraph tags).
  That is harmless: purging an orphan is a no-op `UNLINK`,
  and the member ages out of the index by score/TTL.
  Apollo behaves the same way (endpoint deletes leave ZSET members to age out).
- Races with concurrent writes are acceptable:
  a write that lands after the purge read is a FRESH entry that deserves to survive,
  and a write that lands between `UNLINK` and `ZREM` merely re-orphans one member.
  Replace-only entries mean no partially-invalidated value can exist.
- Return the summed `UNLINK` count so the endpoint can answer Apollo-style `{"count":N}`.

## 6. The three granularities and their cardinalities

| Tag | Cardinality of its key set | Index? |
|---|---|---|
| `entity:{sg}:{T}:{digest}` | a handful (arg variants, requires variants, partitions of ONE entity) | ZSET — this is the workhorse |
| `type:{sg}:{T}` | all cached entries of one type in one subgraph — thousands to millions | ZSET, but see below |
| `subgraph:{sg}` | every entry of the subgraph — up to the whole keyspace slice | NO index — use prefix SCAN |

The entry-key layout makes the subgraph granularity index-free:
every key of subgraph `products` starts with `v1:products:`, header-hash variants included
(`v1:products:h<hex>:`), and partitioned keys too (the partition is inside the digest).

**Recommendation:**
do not maintain the `subgraph:` ZSET at all —
implement subgraph purge as `SCAN MATCH v1:{sg}:* COUNT 1000` + batched `UNLINK`,
which also catches entries whose index members were lost.
`SCAN` is cursor-based and non-blocking; a subgraph purge is a rare, operator-initiated action
that can afford a full keyspace iteration.
Caveat: `MATCH` filters server-side but the cursor still walks the whole keyspace,
so on a shared Redis the cache should own its own logical DB (or run this rarely).
Skip `v1:idx:*` keys in the deletion (or purge the matching `v1:idx:*` too — cleaner).
Keep the ZSET for `type:` (bounded by a type's popularity, the schema-deploy purge unit)
and for `entity:` (tiny, the high-frequency business purge unit).
The engine still SENDS all three tags on every item;
which ones the adapter indexes is adapter configuration.

## 7. Private partitions

Partitioned entries differ only in the hashed key digest
(the `partition` segment is inside the hashed preimage) and carry the SAME tags as public ones —
by design, so one purge clears every requester's copy.
The ZSET design preserves this automatically:
each partitioned key is its own index member under the shared entity/type tag,
and the prefix SCAN for subgraph purge matches them all.
No identity material ever appears in tags or index keys — nothing to redact.

## 8. Negative sentinels

No special handling.
A sentinel is an ordinary item: envelope bytes `{"data":null,...}`, its own (shorter) TTL,
and the full tag set — the engine attaches tags to sentinels precisely so that
purging an entity also clears its "does not exist" record.
The adapter must not filter by value shape or TTL length; bytes are opaque.

## 9. Cluster mode

- Reads/writes: the pipelined per-key design (§2, §3) is already cluster-correct;
  go-redis groups pipeline commands per node and runs them concurrently.
  Apollo's storage layer solves the same problem from the other end:
  it uses `MGET` and falls back to concurrent per-key `GET`s in cluster mode.
  Starting with per-key pipelines skips the fallback complexity entirely.
- Do NOT hash-tag keys by subgraph (`v1:{products}:...`):
  it would pin a whole subgraph's cache to one slot — a hot-spot and a memory-skew machine.
  Losing slot co-location costs nothing here because no operation needs multi-key atomicity.
- Each tag's ZSET lives on one slot (single key), which is fine:
  purge reads the ZSET, then fans `UNLINK` batches out per node.
- Subgraph purge via SCAN must iterate every master in the cluster
  (go-redis `ForEachMaster` + per-node SCAN).

## 10. Failure semantics — what the adapter still owes

The engine already degrades gracefully; the adapter's job is to make failure CHEAP:

- Per-op timeouts, separately configurable for read / write / invalidate
  (Apollo exposes exactly this trio: `fetch_timeout`, `insert_timeout`, `invalidate_timeout`).
  Respect the incoming context — the engine's request context bounds `GetMany`.
- A circuit breaker (Apollo ships one):
  after M consecutive failures, open for T and answer `GetMany`/`SetMany` with an immediate error,
  so a dead Redis costs nanoseconds per request instead of a timeout per fetch.
  The engine's all-miss fallback turns this into "caching off" transparently.
- Bounded connection pool sized to the router's fetch parallelism;
  pool exhaustion should fail fast, not queue behind the request.
- A max-value-size guard on write (skip the item, count it, no error to the engine) —
  a single multi-megabyte entry should not be allowed to dominate memory and bandwidth.
- Never retry writes into the request path; drop and count.

## 11. Operational concerns

- **Expiry stampede**: the engine deliberately has no request coalescing,
  so N concurrent requests after a popular entry expires all hit the origin.
  Cheap mitigation in the adapter: optional DOWNWARD-only TTL jitter
  (`redisTTL = ttl - rand(0..j%)`, default off, j ≤ 5).
  Downward-only matters: the envelope records `cc.ttl`,
  and a Redis TTL longer than the envelope's leaves entries in Redis
  past the freshness the origin promised —
  stale serves or dead weight, depending on how the read path treats them,
  and neither is worth the jitter.
  Real stampede protection (singleflight, stale-while-revalidate) is an engine follow-up, not adapter scope.
- **Eviction policy**: every cache key carries a TTL, so `volatile-ttl` or `volatile-lru` fit.
  Avoid `allkeys-lru`: it can evict index ZSETs while their entries live,
  silently breaking invalidation for those entries.
- **Compression**: the JSON envelope compresses 3–5× typically.
  Worth having as an opt-in (LZ4/snappy above a size threshold, magic-byte prefix,
  transparent on read since the adapter owns the bytes) — but ship without it first and measure.
- **Metrics** the adapter should emit (the engine's observer covers hit/miss semantics already):
  op latency histograms per op kind, error and circuit-open counters,
  payload size distribution, purge counts per tag kind, index ZSET sizes (sampled),
  and skipped-oversize-write count.

## 12. Schema-deploy invalidation (open design note)

The engine has no schema-drift guard:
an entry written before a schema change is served as long as it decodes and covers the selection.
The agreed direction is composition-side diffing that purges `type:{sg}:{T}` for changed types.
The adapter needs nothing beyond §5 for this —
`InvalidateByTags` with a list of type tags is the whole API —
but it should accept MANY tags per call and return one summed count,
so a deploy purging dozens of types is one endpoint call and one audit line.
Until that lands, operators purge manually or size TTLs to deploy cadence.

## 13. Comparison with Apollo Router's implementation

| Aspect | This engine + proposed adapter | Apollo Router 2.17 (live-verified) |
|---|---|---|
| Entry key | `v1:{sg}[:h<hex>]:{16-hex xxhash64}` — short, digest-only | `version:1.2:subgraph:{sg}:type:{T}[:representation:{r}]:hash:{h}:data:{d}` — long, segmented, sha256 |
| Key inputs | fetch identity: representation + args + partition; NO query text | query-shape hash + supergraph SDL hash; every deploy cold-starts the cache |
| Cross-shape reuse | yes — coverage walk lets wider entries serve narrower queries | none — different shape ⇒ different hash ⇒ full miss |
| Value envelope | `{data, cc{ttl,created,scope}}`; tags NOT stored in the value | `{data, cache_control{created,maxAge,public}, cache_tags[...]}` |
| Tag index | ZSET `v1:idx:{tag}`, score = expiry ms; subgraph granularity via prefix SCAN, no index | ZSET `version:1.2:cache-tag:{...}`, score = expiry s, index TTL = entry TTL + 1s |
| Value expiry | `SET PX` relative | `EXAT` absolute |
| Index hygiene | lazy prune on purge + background `ZREMRANGEBYSCORE`; purge deletes the tag ZSET | background maintenance task; endpoint deletes leave ZSET members orphaned to age out |
| Invalidation API | router endpoint over `InvalidateByTags`; kinds map to tag strings | built-in endpoint, kinds subgraph/type/entity/cache_tag, 202 `{"count":N}`, shared-key auth |
| Batch reads | pipelined per-key GET (cluster-safe) | MGET with concurrent-GET fallback in cluster mode |
| Partial hits | per entity (reduced representations request) | per entity (`partial_hit` status) |
| Max-age vs config | header wins; static config is fallback; `MaxTTL` clamps | header wins outright; configured `ttl` is fallback, never a cap |

## 14. Implementation checklist (ordered)

1. Standalone client: pipelined `GetMany`/`SetMany` with PX TTLs, per-op timeouts, pool config. (S)
2. Unit tests against miniredis or a dockerized Redis: contract shape
   (one Entry per key in order, opaque bytes round-trip, error ⇒ engine-visible error). (S)
3. Tag ZSET writes for `type:`/`entity:` tags, `ZADD GT` + `EXPIRE GT`. (S)
4. `InvalidateByTags`: prune → batched ZRANGE/UNLINK/ZREM → delete index; count return. (M)
5. Subgraph purge via per-master `SCAN MATCH v1:{sg}:*` + batched `UNLINK`. (M)
6. Circuit breaker + fail-fast behavior; wire `OnStoreError` expectations into tests. (M)
7. Cluster support pass: verify pipeline fan-out, per-master SCAN, no hash tags. (M)
8. Background index maintenance loop (`SCAN v1:idx:*` + `ZREMRANGEBYSCORE`). (S)
9. Metrics, max-size guard, optional TTL jitter, optional `ReportRemainingTTL`. (S)
10. Invalidation HTTP endpoint in the router: kind → tags mapping, shared-key auth,
    `{"count":N}` responses, audit logging. (M)
11. Optional: compression behind a knob; measure first. (S)

## 15. Open questions

- User-defined cache tags (engine follow-up): once `Item.Tags` carries user values,
  index cardinality becomes user-controlled — the adapter will want a per-tag-count guard
  and possibly an allowlist, mirroring Apollo's `@cacheTag` format templates.
- `s-maxage` (engine follow-up, see notes): would lengthen entry TTLs relative to client TTLs —
  no adapter change, but sizing assumptions shift toward long-lived entries + active purge.
- Format-version rollovers: bumping `v1:` orphans the old generation;
  decide whether deploys should fire a `SCAN MATCH v0:*` cleanup or let TTLs drain it.
- Whether the type-tag ZSET should ALSO be optional (Apollo makes all indexes configurable);
  recommended default: entity + type on, subgraph off (SCAN path instead).
