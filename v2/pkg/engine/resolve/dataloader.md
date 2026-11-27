# This document describes current batching solution

## Why do we need dataLoader ?

DataLoader provides solutions for next problems:

### 1. Batching 
   
Batching highly decrease number of network request to data sources.
   
Resolver tries to resolve array of Products ```[Product1, Product2, Product3]```:
   
**Before:**
   
_fetch 1:_
```json
 {
   "method":"POST",
   "url":"http://localhost:4003",
   "body":{
     "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}",
     "variables":{"representations":[{"upc":"top-1","__typename":"Product"}]
   }
 }
```
   
_fetch 2:_
```json
{
  "method":"POST",
  "url":"http://localhost:4003",
  "body":{
    "query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}",
    "variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}
  }
}
```
   
_fetch 3:_
```json
{
  "method":"POST",
  "url":"http://localhost:4003",
  "body":{
    "query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}",
    "variables":{"representations":[{"upc":"top-3","__typename":"Product"}]}
  }
}
```

**After:**
   
_fetch 1:_
```json
{
  "method":"POST",
  "url":"http://localhost:4003",
  "body":{
    "query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}",
    "variables":{
      "representations":[
        {"upc":"top-1","__typename":"Product"},
        {"upc":"top-2","__typename":"Product"},
        {"upc":"top-3","__typename":"Product"}
      ]
    }
  }
}
```

### 2. Request deduplication
    
It allows requesting data for only uniq arguments set 

Resolver tries to resolve array of Products ```[Product1, Product2, Product1, Product3, Product2]```:
    
**Batch without deduplication:**
   
_fetch:_
```json
{
  "method":"POST",
  "url":"http://localhost:4003",
  "body":{
    "query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}",
    "variables": {
      "representations":[
        {"upc":"top-1","__typename":"Product"},
        {"upc":"top-2","__typename":"Product"},
        {"upc":"top-1","__typename":"Product"},
        {"upc":"top-3","__typename":"Product"},
        {"upc":"top-2","__typename":"Product"}
      ]
    }
  }
}
```

**Batch with deduplication:**

fetch:
```json
{
  "method":"POST",
  "url":"http://localhost:4003",
  "body":{
    "query": "query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}",
    "variables": {
      "representations":[
        {"upc":"top-1","__typename":"Product"},
        {"upc":"top-2","__typename":"Product"},
        {"upc":"top-3","__typename":"Product"}
      ]
    }
  }
}
```

## How does dataLoader work ?

There is no separate `DataLoader` type in the engine. Batching and deduplication are done by the
`Loader` in [loader.go](./loader.go), using information that the planner and the
[postprocess](../postprocess) package have already prepared at planning time.

### Overview

- The planner turns the operation into a **fetch tree** (`FetchTreeNode`, see [fetchtree.go](./fetchtree.go)).
  Each node is one of:
  - `Single` – one fetch to a data source (`FetchItem`),
  - `Sequence` – children are resolved one after another (a child depends on data loaded by a previous one),
  - `Parallel` – children are independent and are resolved concurrently.
- Every `FetchItem` has a `FetchPath`: a list of `FetchItemPathElement`s (`ObjectPath(...)` / `ArrayPath(...)`)
  that points from the root of the response to the objects the fetch has to enrich.
  A path element can also carry `TypeNames`, so only objects with a matching `__typename` are selected
  (useful for interfaces and unions).
- During postprocessing, a nested entity fetch becomes a concrete fetch type:
  - `EntityFetch` – the entity lives on a single object; the request has one representation.
  - `BatchEntityFetch` – the entity lives inside a list; **all** matching objects are sent in one request.
  - `MultiEntityFetch` – several entity fetches to the same subgraph are merged into one request with
    aliased `_entities` fields (see [fetch_multi.go](./fetch_multi.go)).
- A `Loader` is created for each request (and for each subscription event or defer group). It writes every
  response into one shared JSON document (the `DataBuffer`). After all fetches finish, `Resolvable` renders the
  client response from that document.

The key point: the engine does **not** walk the response tree node by node and fetch per object.
Each fetch in the tree runs exactly once. It collects *all* objects that it has to enrich at that moment,
no matter how deeply they are nested in lists. This is what gives batching.

### Resolving a fetch

`Loader.ResolveFetchNode` walks the fetch tree (`resolveSerial` for `Sequence`, `resolveParallel` for `Parallel`).
Each `Single` node is resolved by `resolveSingle` in three phases:

1. **Prepare** (`preparePhase`, holds the data lock)
   - If a fetch this one depends on (`DependsOnFetchIDs`) has failed, the fetch is skipped.
   - `selectItemsForPath` follows the `FetchPath` from the root of the current response data.
     Arrays found along the way are flattened. The result is a flat list of every object
     the fetch must enrich (the "siblings" across all list items).
   - The request body is rendered from those items:
     - `SingleFetch` / `EntityFetch` render their `InputTemplate` once.
     - `BatchEntityFetch` renders `Header`, then one `Item` template per selected object
       (joined by `Separator`), then `Footer`. This builds the `representations` array.
       Items that render to `null` or `{}`, or that fail to render, can be skipped
       (`SkipNullItems`, `SkipEmptyObjectItems`, `SkipErrItems`).
       If every item is skipped, the request is not sent.
   - Authorization, rate limiting and other pre-fetch checks run on the rendered input.
2. **Load** (`loadPhase`, no lock held, so parallel fetches really run at the same time)
   - The response cache is checked (if enabled), then the request is sent to the `DataSource`.
   - For queries, identical in-flight subgraph requests are shared via `SubgraphRequestSingleFlight`.
     This can be turned off with `ExecutionOptions.DisableSubgraphRequestDeduplication`.
3. **Merge** (`mergePhase`, holds the data lock)
   - The response is parsed. Subgraph errors are collected and their paths are rewritten to the client's path.
   - Data from `PostProcessing.SelectResponseDataPath` (e.g. `data._entities`) is merged back into the
     selected items, at `PostProcessing.MergePath` if one is set.
     For a batch, element `i` of the response goes to the objects that produced representation `i`.

### Deduplication inside a batch

While building a `BatchEntityFetch` request (`prepareBatchEntityFetch`), each rendered representation is hashed.
If the same representation was already added, it is not added again. The current object is
remembered as one more merge target for that representation (`result.batchStats`).

So for `[Product1, Product2, Product1, Product3, Product2]` the request contains three representations.
In the merge phase, entity `0` is merged into both `Product1` objects, entity `1` into both `Product2` objects,
and entity `2` into `Product3`.
This is the "Batch with deduplication" request shown above.

**Example:**

_Query :_
```graphql
query {
  topProducts {
    reviews {
      body
      author {
        username
      }
    }
  }
}
```

Assume `topProducts` comes from a products subgraph, `reviews` from a reviews subgraph,
and `author.username` from an accounts subgraph. The fetch tree is roughly:

```
Sequence(
  Single(fetch 0: SingleFetch        -> products  { topProducts { upc } }),
  Single(fetch 1: BatchEntityFetch   -> reviews   _entities(Product)   path: [ArrayPath("topProducts")]),
  Single(fetch 2: BatchEntityFetch   -> accounts  _entities(User)      path: [ArrayPath("topProducts"), ArrayPath("reviews"), ObjectPath("author")]),
)
```

```
                                                    |topProducts|                                               fetch 0 (root)
                                                          |
                       |---------------------------|Array of Products|--------------------|
                       |                                  |                               |
                  |Product A|                        |Product B|                      |Product C|                 fetch 1 selects 3 items
                       |                                  |                               |
     |----------|Array of Reviews|--------|        |Array of Reviews|       |-----|Array of Reviews|-----|
     |                 |                  |               |                 |                            |
 |Review A1|      |Review A2|      |Review A3|         |Review B1|     |Review C1|                  |Review C2|
     |                 |                  |               |                 |                            |
  |Author 1|       |Author 2|         |Author 3|       |Author 4|       |Author 5|                   |Author 6|   fetch 2 selects 6 items
```

1. **fetch 0** has an empty `FetchPath`, so it runs against the root. The response
   `{"topProducts":[{"upc":"top-1"},{"upc":"top-2"},{"upc":"top-3"}]}` is merged into the root of the response data.
2. **fetch 1** selects `topProducts`. The value is an array, so it is flattened into
   `[Product A, Product B, Product C]`. It sends **one** request with three representations and merges
   `_entities[0..2]` back into Product A, B and C. Each product now has its `reviews`.
3. **fetch 2** selects `topProducts` → `reviews` (flattened again, so all six reviews of all products) → `author`.
   It sends **one** request with the representations of Author 1..6 and merges the results back.
   If two reviews have the same author, that author is sent only once and the result is merged into both reviews.

Without batching, this query would send 1 + 3 + 6 = 10 subgraph requests.
With the `Loader` it sends 3, one per fetch in the tree, however many products and reviews there are.
