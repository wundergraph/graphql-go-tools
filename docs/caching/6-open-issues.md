# Open issues

## BUG: root-field cache keys derive from the request variable set, not the fetch's arguments

Found by live-subgraph testing 2026-08-11 (`5-testing-on-js-subgraphs.md`, findings 1 and 2).
`rootFieldCacheKey` hashes `{TypeName}.{FieldName}:{canonicalVariables(ctx)}` — the WHOLE request variable set.

Two consequences:

1. CORRECTNESS: two aliased calls of one cached root field in one operation
   (`one: topProducts(first:$x) two: topProducts(first:$y)`) render identical keys;
   one write overwrites the other and the warm response serves the survivor to both aliases,
   nondeterministically.
   Pinned by the deliberately-failing subtest [11] in the gqlt-live suite.
2. HIT RATE: client variable names are in the preimage,
   so `($first:Int){"first":1}` and `($n:Int){"n":1}` fragment into separate entries
   for byte-identical origin requests.

Fix direction: derive the key from the argument values the fetch itself sends
(its rendered input), mirroring entity keys.
Until fixed: do not configure root-field caching for fields a single operation may call twice.

## Entry thrash: disjoint selections over one entity share a key and replace each other

Selection sets are not part of entity keys,
so two operations selecting DISJOINT field sets of the same entity from the same subgraph
(`User {a b}` vs `User {address}`) compete for ONE entry slot.
Each read is a store HIT that the coverage walk rejects (the entry lacks the other shape's fields),
and each refetch REPLACES the entry wholesale (storage is replace-only, no entry merging).

Verified with a scratch engine test 2026-08-11:
alternating `{upc stock}` / `{upc warehouse{...}}` four times produced four origin requests,
four hit-but-rejected reads, and an entry ping-ponging between the two shapes —
hit rate zero for both operations.
Traffic converges only when some operation fetches a superset in a single fetch
(the wider-serves-narrower behavior verified live, `5-testing-on-js-subgraphs.md` finding 3).

Agreed direction: follow Apollo and add a selection-shape digest to the L2 key —
`,"fields":"<16-hex>"` over the sorted NORMALIZED field paths of the fetch's ProvidesData tree,
computable at plan time and frozen in the key spec.
Because the digest is over normalized schema names,
alias/order/fragment variants and argument-value variants still share entries
(better than Apollo's raw query-shape hash),
and the entity tag still covers all shape variants, so one purge clears them all.
Coverage stays on the read path as a corruption guard only.

Decided trade-offs to carry into the spec:

- L1 keeps the coverage-based model WITHOUT the digest:
  cross-defer-group superset reuse (initial `{stock, warehouse}` serving a deferred `{stock}`)
  and the `optimizeL1Cache` provider/consumer proof depend on it.
- Costs accepted: a narrow query after a wide one misses (per-shape warmup),
  negative sentinels fragment per shape,
  and the store holds one entry per (entity × shape × args × partition) — all Apollo-equivalent.

## TTL churn: the last writer's TTL caps the shared entry

Same root cause as the thrash issue, visible even for OVERLAPPING selections
when the origin's `Cache-Control: max-age` differs per operation
(the header is per HTTP response and beats static config).
With `{a b c}` answered `max-age=86400` and `{a b c d}` answered `max-age=600`:
whichever fetch wrote LAST owns the single entry and its TTL,
so the 10-minute superset write discards the remaining day of the narrow entry,
and after it expires the shapes alternate ownership indefinitely.
This is a consequence of the one-write-moment/one-honest-TTL invariant
(no field-level ages, no entry merging) — correct, but it makes a short-TTL operation
cap the effective cache lifetime of every operation sharing the entry.
The selection-shape digest above resolves this too:
per-shape entries each keep their own honest TTL.

## Open design: detecting the NEED for invalidation on schema deploys

The engine has NO schema-drift guard (the envelope `types` map was dropped 2026-08-10):
an entry written before a schema change is served as long as it decodes and covers the selection.
Undecided: whether/how a schema publish should purge affected entries — e.g. diff the composed schema at publish
time and fire `type:{subgraph}:{Type}` tag invalidations for types whose field shapes changed.
Belongs to the router/composition layer; the tag vocabulary already supports it.

## Follow-up: honor `s-maxage` in the header-driven caching model

The router cache is a SHARED cache, so the semantically correct reading of `Cache-Control: max-age=30, s-maxage=600`
is: router entry TTL = 600s (`s-maxage` wins for shared caches), client-facing aggregated header = 30s (`max-age`).
Useful pattern once tag invalidation exists: long router TTL + active purge, short client TTL.
Deliberately out of the initial model: it needs a second freshness number through the pipeline —
entry TTL from `s-maxage`, plus a separate client TTL in the value envelope's `cc`
metadata for the client-header aggregation.
Additive change; implement when the long-edge/short-client pattern is requested.

## Check: entity interface caching support

We need to verify whether caching works (and should be supported) for entity interfaces under the representation-derived single-key model
(implemented 2026-08-04; described in `1-readme.md`, spec text in git history of the 98dff2032..9f15c5c6c commit series).

Established (verified in `plan/representationvariable/representation_variable.go`):

- `BuildRepresentationVariableNode` bakes the interfaceObject/entity-interface `__typename` remap into the node:
  for an `@interfaceObject` target the node's `__typename` field is a `resolve.StaticString{Value: <InterfaceTypeName>}`, not an item read.
- Consequence for representation-derived keys: the key renderer must take `__typename` from the NODE
  (static value when present, item value otherwise), not unconditionally from the item —
  then interfaceObject fetches key under the interface name automatically, inheriting the planner's remap for free.

Still to answer:

- Cross-subgraph identity split: a concrete-type subgraph keys the entity as `Article` while the `@interfaceObject` subgraph keys it as `Personalized` —
  disjoint entries for the same logical entity.
  Likely CORRECT (different subgraphs provide different data and the subgraph segment separates their keyspaces anyway), but decide and document.
- The `OnTypeNames`-aware key renderer (merged representation nodes for abstract-type batch fetches) must be tested against interface entity fixtures,
  not only union/concrete ones.
- Coverage walk (`covers` / `skipFieldForTypeName`): verify type-conditioned fields behave for interface fetches where the cached value's `__typename`
  is a concrete implementer but ProvidesData fields are conditioned on the interface.
- Fixtures: current caching subgraphs have no interface entities; add one (e.g. `Personalized`-style interface with two implementers, one `@interfaceObject` subgraph)
  before claiming support either way.
