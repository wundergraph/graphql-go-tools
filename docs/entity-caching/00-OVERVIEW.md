# 00 — Overview & Navigation

> The entry point for this document set.
> Read this first.
> It assumes you have never seen the entity-caching feature and explains why this folder exists, what is in it, and the order to read it in.

---

## 1. The problem

Entity caching for the GraphQL router was built across **three pull requests** that grew too large to review.
The diffs touch the most sensitive files in the engine — the resolver's `loader` and `resolvable` — and they mix many concerns in one change:
new astjson primitives,
planner passes,
seven distinct caching behaviors,
analytics events,
and router wiring.
A reviewer cannot hold all of that in their head at once,
cannot tell which line traces to which feature,
and cannot approve any single piece in isolation.

The result is that the work is effectively unmergeable in its current shape,
not because it is wrong,
but because it is unreviewable.

## 2. The goal

Re-implement the same feature cleanly as **two fresh feature branches** — one in `graphql-go-tools`, one in the router (cosmo) — each landed as a **stack of small, additive PRs**.

The guiding principles:

- **One concern per PR.**
  Each directive or caching behavior lands on its own,
  against contracts that earlier PRs already made stable.
- **The hot path stays untouched.**
  Caching attaches through a small **integration seam** (a handful of interfaces and hook points),
  so the existing `loader` and `resolvable` code is left essentially as-is.
  This is a hard requirement, not a preference.
- **Foundation first.**
  The architecture and the seams land before any directive,
  so every later PR is a small leaf change rather than a rewrite.
- **A hard external prerequisite is made explicit.**
  The caching layer depends on astjson APIs that exist only on an **unreleased** astjson branch.
  Landing and releasing those primitives is the first step of the whole plan,
  not an afterthought.

## 3. The deliverables in this folder

Read in this order.
Each entry says what the document gives you and when you need it.

| # | Document | What it gives you |
|---|----------|-------------------|
| 00 | **00-OVERVIEW.md** (this file) | Executive summary and the map of everything else. |
| 01 | [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) | The clean architecture: where caching attaches, the two-level model, the memory invariants, and the **integration seam** (the interfaces and hook points that keep `loader`/`resolvable` untouched). |
| 02 | [02-DIRECTIVE-INVENTORY.md](./02-DIRECTIVE-INVENTORY.md) | A table of every directive and caching-config concept the feature reads or introduces — what each means, who consumes it, and which PR rebuilds it. |
| — | [directives/&lt;name&gt;.md](./directives/) | One detailed contract spec per directive (linked from the inventory table). |
| — | [adr/0001-foundation.md](./adr/0001-foundation.md) | The foundation decision record: architecture and seams, decided once so later PRs are additive. Plus one ADR per directive at `adr/00NN-<name>.md`. |
| 03 | [03-PR-PLAN-graphql-go-tools.md](./03-PR-PLAN-graphql-go-tools.md) | The stacked-PR plan for the engine repo (`graphql-go-tools`). |
| 04 | [04-PR-PLAN-router.md](./04-PR-PLAN-router.md) | The stacked-PR plan for the router repo (cosmo). |
| 05 | [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md) | The astjson dependency contract — the unreleased primitives the foundation needs, and which are required versus ship-along. |
| 06 | [06-TEST-AND-BENCH-PLAN.md](./06-TEST-AND-BENCH-PLAN.md) | The test and benchmark plan that gates each PR. |
| 07 | [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md) | Out-of-scope issues found while reading the original code — documented, deliberately **not** fixed here. |
| 08 | [08-EXECUTION-RUNBOOK.md](./08-EXECUTION-RUNBOOK.md) | The Codex-driven implementation loop that turns the plan into landed PRs. |

## 4. The architecture in five sentences

Entity caching is a **two-level cache**: an L1 per-request `map[string]*astjson.Value` on the resolver's Loader that deduplicates identical entity lookups inside one request, plus an external L2 (for example Redis) behind a narrow interface that deduplicates across requests.
Caching attaches at exactly two stages of the `parse → normalize → validate → plan → resolve → response` pipeline — the **planner** annotates each fetch with a cache-key template, a provides-shape, and per-fetch flags, and the **resolver** acts on those annotations to read and write the caches — while everything else stays unaware.
The whole layer plugs into the engine through a small **integration seam**: extracted `LoaderCache` and cache-key interfaces plus a thin `entityCache` collaborator and a single `cacheSkipFetch`/`cacheMustBeUpdated` flag, so the hot `loader` and `resolvable` files are left essentially untouched.
Correctness rests on one memory primitive, astjson's **`StructuralCopy`**, which clones container nodes onto the per-request arena while aliasing scalar leaves — cheap, safe only within the same request, and the basis for keeping cache entries and the live response tree from corrupting one another.
The one true new schema directive is `@requestScoped`, a symmetric per-request coordinate L1 where any field with a shared key can populate the entry and later fields skip their fetch; everything else (`@key`, `@requires`, `@provides`) is existing federation metadata the cache merely *reads*.

## 5. How to read this package

If you are reviewing the plan, follow this path:

1. **Start here (00),** then read [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) end to end.
   The architecture spec is load-bearing — every other document assumes its vocabulary (L1, L2, the seam, `StructuralCopy`, `ProvidesData`).
2. **Read [adr/0001-foundation.md](./adr/0001-foundation.md)** to understand *why* the architecture is shaped this way and why the foundation must land before any directive.
3. **Skim [02-DIRECTIVE-INVENTORY.md](./02-DIRECTIVE-INVENTORY.md)** for the lay of the land,
   then dip into individual [directives/&lt;name&gt;.md](./directives/) specs and their matching ADRs only when a given behavior matters to you.
4. **Read [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md)** before either PR plan — the astjson release is the literal first step, and the plans depend on it.
5. **Read the two PR plans** ([03](./03-PR-PLAN-graphql-go-tools.md) for the engine, [04](./04-PR-PLAN-router.md) for the router) to see the concrete stack ordering.
6. **Use [06-TEST-AND-BENCH-PLAN.md](./06-TEST-AND-BENCH-PLAN.md)** as the acceptance bar for each PR, and **[08-EXECUTION-RUNBOOK.md](./08-EXECUTION-RUNBOOK.md)** as the operating procedure for actually doing the work.
7. **Treat [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md)** as a parking lot — issues to be aware of, but explicitly out of scope for this re-implementation.

A few terms that recur everywhere, defined once so the rest reads smoothly:

- **L1** — per-request in-memory cache on the Loader, main-thread only, entity fetches only.
- **L2** — external cross-request cache behind the `LoaderCache` interface, root-field and entity fetches.
- **The seam** — the small set of interfaces and hook points through which caching attaches without rewriting the resolver.
- **`StructuralCopy`** — astjson primitive that clones containers on the arena while aliasing leaves; the safety basis for the cache.
- **`ProvidesData`** — the alias-aware field shape a fetch yields, used for cache projection and the field-widening check.
- **`@requestScoped`** — the one new schema directive; a symmetric per-request coordinate L1.

---

> Two repos, two stacks of small PRs, one astjson release that must land first.
> The architecture spec (01) and the foundation ADR (0001) are the two documents to read closely before anything else.
