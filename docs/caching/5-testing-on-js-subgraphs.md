# Live-subgraph testing report

Findings from running the execution engine's response caching against real Apollo-federation JS subgraphs,
2026-08-11.
Suite written by an agent from a detailed guide; all headline findings re-verified by hand,
including an independent rerun reproducing the correctness bug.

## Environment

- Suite: `apollo-response-caching/gqlt-live/` (standalone Go module,
  `replace`-linked to the local graphql-go-tools checkout).
  Run from that directory: `gotestsum --format=short -- ./... -count=1`.
- Live origins: the two Apollo-federation subgraphs from the apollo-response-caching workspace,
  `products` on :4301 (`Cache-Control: max-age=60, public`) and `reviews` on :4302 (`max-age=30, public`),
  each with a request counter and body log.
  Both must be running (`node index.js`); the suite does not start them.
- Redis 7.2 container `apollo-cache-redis` on :6380 for the Redis-store subtests.
- Composition: wgc over SDL copies fetched from the live subgraphs.
  wgc rejects Apollo's `@cacheTag` directive;
  the copies drop it from the `@link` import and declare it locally,
  preserving all applications.
  Inert for what is tested: this engine's directive is `@cache`, so `@cacheTag` affects no key, TTL, or tag.
- Every datasource is reached through a transparent recording reverse proxy so tests pin exact wire bodies;
  the subgraphs' own counters are the independent cross-check.
  A control subtest runs one operation both proxied and direct
  and asserts identical responses, store ops, and counter deltas.

Result: 14 subtests, 13 pass, 1 deliberate known-failure pinning a real engine bug (finding 1).
Stable across `-race` reruns.

## Finding 1 — BUG: two calls of one cached root field in one operation share a key

```graphql
query($x: Int, $y: Int) { one: topProducts(first: $x) { upc } two: topProducts(first: $y) { upc } }
# variables {"x":1,"y":3}
```

The cold response is correct, and the two isolated fetches are genuinely different at the origin
(wire bodies carry `{"a":1}` and `{"b":3}` after variable remapping).
But both fetches render the SAME cache key, both read it, and both write it —
one silently overwrites the other.
The warm request serves the surviving entry to BOTH aliases:
observed `{"one":[1],"two":[1]}` and `{"one":[1,2,3],"two":[1,2,3]}` across runs,
i.e. which alias gets corrupted is nondeterministic (whichever isolated fetch finishes last wins).

Root cause: `rootFieldCacheKey` (`v2/pkg/engine/cache/cache_key_template.go`)
builds the preimage from the coordinate plus `canonicalVariables(ctx)`,
which serializes the ENTIRE request variable set rather than the arguments this fetch sends.
Two calls of one coordinate in one operation therefore have byte-identical preimages.

This breaks two stated invariants:
"read key == write key == fetch identity" and "caching never changes a response".
Entity keys are unaffected (they render from representation values).
Fix direction: derive the root-field key from the argument values the fetch itself sends
(its rendered input), mirroring how entity keys already work.
Pinned by the deliberately-failing subtest [11].

## Finding 2 — root-field entries fragment by client variable name

`topProducts(first: 1)` inline, `query($first: Int)` + `{"first":1}`, and `query($n: Int)` + `{"n":1}`
produce three different keys, three misses, and three copies of one value —
while the recorded wire bodies to the origin are byte-identical.
Same root cause as finding 1: the preimage serializes client-named request variables.
Impact is hit rate and store size, not correctness.
Pinned as a passing characterization subtest [10] (documents current behavior, not a fix).

Doc impact: 2-behavior.md §3.7 ("variable names never affect keys") was contradicted by §3.5
(root-field preimages include the name-sorted variable set);
the docs now carry the known-deviation note pointing here.

## Finding 3 — coverage genuinely prevents ping-pong (works as designed)

The selection set is not part of a root-field key, so narrow and wide queries share one entry.
Sequence narrow → wide → narrow → wide:
the narrow query misses and writes;
the wide query gets a store HIT but coverage rejects the narrow entry and overwrites it wider;
both later queries serve from the wide entry.
Two origin requests total, then stable —
entries converge on the widest selection, narrower selections are projected down on serve.

## Verified working (each with wire-body, op-log, and counter evidence)

- **Entity L2 lifecycle**: counter deltas `{products:1, reviews:1}` cold, `{1, 0}` warm;
  read key == write key.
- **Header beats static config**: with `DefaultTTL: 5m` at the global, subgraph AND root-field tiers,
  entries are written with `TTL: 60s` (products) / `30s` (reviews) and `cc.ttl` agrees —
  the origin's `max-age` is the runtime truth.
- **Root-field caching**: an alias-variant operation is served from the same entry with zero deltas;
  a different `first` argument misses into its own entry;
  root-field entries carry the two coarse tags only.
- **Batch entity fetch**: one `GetMany` with 3 keys, one `SetMany` with 3 entries,
  one wire request carrying all 3 representations; the warm run is a full hit.
- **Partial cache load**: with one product primed, the reviews origin received exactly
  `{"representations":[{"__typename":"Product","upc":"2"}]}`
  and only the missing entry was written back.
- **Client cache answer**: cold `MaxAge` is exactly `30s` (the minimum of 60/30, proving min-fold);
  the warm countdown lands in `(25s, 30s]`;
  one uncached fetch forces `NoStore` while the cacheable part is still stored.
- **Response invariance**: no-controller, cold-cached, and fully-warm runs are byte-identical.
- **Redis store (prototype)**: the envelope crosses into a real Redis verbatim with the header TTL;
  a fresh controller serves from it;
  delete-by-tag on the entity tag removes exactly one entry and forces a refetch.

## Not tested, and why

Both origins serve one hardcoded `Cache-Control` and never vary it,
so `no-store` / `no-cache` / `private` / malformed directives / the `MaxTTL` clamp are unreachable live.
Making the two `index.js` files vary the header per operation
is the highest-value extension to this suite.
Also uncovered: the negative sentinel (no empty-entity path in the fixtures),
privacy partitions (no identity source), shadow mode, `@defer` L1 reuse, store-error fallback —
all of these are covered by the in-repo `execution/cachingtesting` suites against doubles.
The reviews subgraph's `extensions.invalidation` hint on `addReview` is ignored by this engine;
mutation-driven invalidation is documented out of scope.

## Suite caveats

- `Mutation.addReview` permanently mutates the subgraph's in-memory data;
  running it once invalidates pinned bodies until the node process restarts.
- Pinned keys embed the composition-assigned datasource ordinal (`v1:0` / `v1:1`);
  adding a subgraph to `graph.yaml` re-pins them all.
- Subtests read shared origin counters and must stay sequential (no `t.Parallel`).

## Notes for the router's Redis adapter (defects found in the suite's own prototype)

All three were caught and fixed during the suite's self-review;
they are exactly the traps the production adapter must avoid
(consistent with `4-redis-adapter-report.md` §3–§5):

- An unconditional tag-set `PEXPIRE` lets a short-lived entry SHORTEN a tag index's lifetime,
  stranding longer-lived entries as permanently unpurgeable — use the `EXPIRE ... GT` form.
- Returning the tag set's cardinality from delete-by-tag over-reports;
  return the deletion command's own count.
- A read helper that maps every error to "absent" makes
  "the purge worked" satisfiable by "Redis is unreachable" — errors and misses must stay distinct.
