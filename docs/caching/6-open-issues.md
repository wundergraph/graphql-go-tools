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
Deliberately out of v1 (`docs/caching/specs/2026-08-06-header-driven-caching.md` §4.1): it needs a second freshness
number through the pipeline — entry TTL from `s-maxage`, plus a separate `client_ttl` in the value envelope's `cc`
metadata for the §10 client-header aggregation.
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
  Likely CORRECT (different subgraphs provide different data, and per-policy CacheName usually separates them anyway), but decide and document.
- The `OnTypeNames`-aware key renderer (merged representation nodes for abstract-type batch fetches) must be tested against interface entity fixtures,
  not only union/concrete ones.
- Coverage walk (`covers` / `skipFieldForTypeName`): verify type-conditioned fields behave for interface fetches where the cached value's `__typename`
  is a concrete implementer but ProvidesData fields are conditioned on the interface.
- Fixtures: current caching subgraphs have no interface entities; add one (e.g. `Personalized`-style interface with two implementers, one `@interfaceObject` subgraph)
  before claiming support either way.
