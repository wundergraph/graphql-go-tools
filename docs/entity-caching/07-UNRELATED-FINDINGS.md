# 07 — Out-of-Scope Findings Register

> Part of the entity-caching reimplementation document set.
> See [00-OVERVIEW.md](00-OVERVIEW.md) for the navigation map,
> [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md) for the integration seam,
> [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md) and
> [04-PR-PLAN-router.md](04-PR-PLAN-router.md) for the stacked-PR plans.

## What this document is

This is a **read-only register**.
It catalogues everything found inside (or near) the entity-caching branch that is **NOT entity caching**,
so a future reader knows what to ignore, what to peel into a separate PR, and what is pure noise.

**Nothing here is being fixed, changed, reverted, or re-implemented in this run.**
This is documentation only.
Every item below is described, classified, and given a recommended disposition —
but no code is touched as a result of this register.

The findings come from analyzing the in-flight PR
("feat: add caching to loader", referred to below as "the caching branch")
against its true upstream base.

## Why you must read the META finding first

The single most important fact in this whole register is a **measurement artifact**, not a code problem.
If you skip it, you will mischaracterize roughly half the branch.
Read finding 0 before anything else.

## How the findings are classified

Each finding records six things, as requested:

1. **Title** — short name.
2. **What it is** — plain description, assuming no prior knowledge.
3. **Why it is unrelated to entity caching** — the separation rationale.
4. **Affected files** — the concrete paths.
5. **Genuine bug fix?** — whether it looks like a real fix worth shipping on its own.
6. **Recommended disposition** — one of: **separate PR**, **discard**, **upstream / already upstreamed**.

---

## Finding 0 (META) — Local `master` is stale, so the raw diff overcounts by ~181 files

**What it is.**
This worktree's **local** `master` ref is frozen at an old commit.
It points at `1dcbd3bd` dated **2026-02-16**, which is the actual merge-base of the branch.
The **real upstream** `origin/master` has moved on to `6a5eb1a3` dated **2026-06-12** (release 2.4.6).
That is roughly four months of upstream work that local `master` does not know about.

The consequence shows up the moment you run a diff:

- `git diff --name-only master...HEAD` reports **397 files** (~135102 insertions) — *overcounted*.
- `git diff --shortstat origin/master...HEAD` reports **216 files, +78980 / -1628** — *the true PR size*.

Both numbers above were re-verified live while writing this register; they match exactly.

The ~181-file gap between the two diffs is **upstream work that is already merged into the branch**
(via the branch's many `Merge branch 'master'` commits)
**but is not present in the stale local `master`.**
So those files light up as "changes" only because the comparison base is old.

**Why it is unrelated to entity caching.**
The overcounted files are not authored by the caching work at all.
They are already-merged upstream features.
The entire list of "suspect unrelated diffs" that a casual reviewer would flag falls into this bucket:

- the gRPC datasource feature line (multiple upstream PRs),
- the subscription-client transport rewrite (the base rewrite, not the two small fixes — see Finding 6),
- `jsonschema` fixes,
- the `astprinter` / `lexer` / `astparser` / `ast` description + variable-description spec feature,
- `grpctest` / `productv1` protobuf regeneration (a ~19770-line generated file),
- playground file deletions,
- starwars testdata.

Every one of those is **absent** from `git diff --name-only origin/master...HEAD`.
They are NOT part of the caching branch.
They are noise from diffing against a stale ref.

**Affected files (illustrative — these appear only in the stale diff, not the real one).**

- `v2/pkg/grpctest/productv1/product.pb.go` (generated protobuf, ~19770 lines)
- `v2/pkg/playground/files/playground.html` (and sibling playground deletions)
- `v2/pkg/lexer/lexer.go`
- `v2/pkg/astprinter/astprinter.go`
- `v2/pkg/engine/jsonschema/variables_schema.go`
- `v2/pkg/ast/ast_variable_definition.go`
- `v2/pkg/starwars/testdata/star_wars.graphql`

**Genuine bug fix?**
N/A — this is a tooling/measurement observation, not a code change.

**Recommended disposition: upstream / already upstreamed.**
Do nothing to these files.
They are already on `origin/master`.

**Action for every downstream reader and every PR-plan step:**
always diff against `origin/master`, never local `master`.
If you write up `grpctest` / playground / `lexer` / `astprinter` / `jsonschema` as
"caching-branch unrelated changes", you are wrong —
they belong to already-merged upstream PRs and must not be attributed to this branch.

---

## Finding 1 — `onError` / `ErrorBehavior` request-extension feature

**What it is.**
A complete, self-contained feature implementing the GraphQL `extensions.onError` request extension.
It controls how the engine handles an error on a non-nullable field —
whether to propagate the null up the tree, render `null`, or halt execution.
The three modes map to an integer enum.

New surface area:

- a new `ErrorBehavior` type with `String()` and a `ParseErrorBehavior(s)` helper,
- a `haltExecution` field and a `HaltExecution()` method on the resolvable,
  with the null-propagation path now switching on the configured behavior,
- an `ErrorBehavior` field added to the shared `ExecutionOptions` struct
  (gated by an `OnErrorEnabled` flag on the resolver options),
- an option-builder entry `WithErrorBehavior(behavior)` in the execution engine,
- a request-parsing path that reads an `Extensions` raw-JSON field
  and exposes `GetOnErrorBehavior()` parsing `{ "onError": "..." }`.

It ships with dedicated, sizable tests
(a unit test file for parsing, an E2E behavior file, and a request-parsing test file).

**Why it is unrelated to entity caching.**
This is net-new null-bubbling control.
It never reads or writes any cache, never participates in L1/L2,
and would behave identically with caching fully disabled.
It is its own feature that the author happened to bundle into the same branch.

**Affected files.**

- `v2/pkg/engine/resolve/error_behavior.go`
- `v2/pkg/engine/resolve/error_behavior_test.go`
- `v2/pkg/engine/resolve/resolvable.go`
- `v2/pkg/engine/resolve/context.go`
- `execution/engine/execution_engine.go`
- `execution/engine/error_behavior_test.go`
- `execution/graphql/request.go`
- `execution/graphql/request_onerror_test.go`

**Genuine bug fix?**
No — it is **net-new behavior**, not a fix.
It is well-tested and looks production-ready, but it is a feature.

**Recommended disposition: separate PR**, landed **before** the caching stack.

**Caveat (the entanglement that makes this hard).**
This feature **shares two files line-for-line with caching**:
the `ExecutionOptions` struct in `context.go`
and the option-builder file `execution_engine.go`.
The `resolvable.go` null-propagation edits also sit in a caching-heavy file.
So you cannot peel this out by dropping whole files —
the edits are interleaved and must be untangled by hand.
See [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md) for the shared-struct seam.

---

## Finding 2 — `service_datasource` package (`__service` capabilities introspection)

**What it is.**
An entirely new datasource package implementing a `__service { capabilities { ... } }`
introspection endpoint.
It advertises engine capabilities (for example `graphql.onError` and a default error behavior)
to clients via a spec-aligned introspection field.

The package adds the usual datasource layers
(config factory, factory, planner, source, types, schema)
plus a schema-extension helper that injects `_Service` / `_Capability` types
and a `__service` field onto the `Query` type,
parallel to how the base schema is normally merged.
It ships with its own test files.
The package is confirmed new on this branch — it does not exist on `origin/master`.

**Why it is unrelated to entity caching.**
This is the **schema/introspection half of the `onError` feature** (Finding 1) —
it exists to tell clients "this server supports onError".
It has **zero** dependency on entity caching:
no cache reads, no cache writes, no L1/L2 involvement.

**Affected files.**

- `v2/pkg/engine/datasource/service_datasource/config_factory.go`
- `v2/pkg/engine/datasource/service_datasource/factory.go`
- `v2/pkg/engine/datasource/service_datasource/planner.go`
- `v2/pkg/engine/datasource/service_datasource/schema.go`
- `v2/pkg/engine/datasource/service_datasource/source.go`
- `v2/pkg/engine/datasource/service_datasource/types.go`
- `v2/pkg/engine/datasource/service_datasource/schema_test.go`
- `v2/pkg/engine/datasource/service_datasource/service_datasource_test.go`

**Genuine bug fix?**
No — net-new feature.

**Recommended disposition: separate PR**, shipped **with or just before** the `onError` PR (Finding 1).
Logically these two findings are one feature pair:
`onError` behavior plus the capability advertisement that announces it.
Because it is a clean new package with no shared files,
it is the easiest of the bundled features to extract.

---

## Finding 3 — Embedded planner correctness changes to general `@requires` / `@provides` resolution

**What it is.**
Inside the request-scoped widening work, several edits change the **general federation planner** —
the code path that runs for every federated query, cached or not.
Three distinct edits stand out:

- **(a)** an early-return guard was removed from the "are required fields provided" check
  (the guard previously bailed out when no fields were provided).
  Removing it changes when `@requires` can be skipped even with no provided fields —
  a real behavior change to provides/requires planning.
- **(b)** the routine that adds field requirements to the operation now sources the type name
  from the field config's own `TypeName` instead of always using the enclosing type definition's name.
  This is a correctness fix for `@requires` type names under interface objects,
  packaged together with a pure-refactor extraction of the interface-object `@requires` lookup.
- **(c)** the required-fields visitor now adds aliases based on the field's alias-or-name bytes.

**Why it is unrelated to entity caching.**
These edits alter planning of **non-cached** federation queries.
They fire whether or not any cache is enabled.
They were made *in service of* request-scoped work,
but their effect is on the general `@requires` / `@provides` resolution path,
so they are logically separable from the cache machinery.

**Affected files.**

- `v2/pkg/engine/plan/required_fields_provided_visitor.go`
- `v2/pkg/engine/plan/node_selection_visitor.go`
- `v2/pkg/engine/plan/required_fields_visitor.go`
- `v2/pkg/engine/plan/path_builder_visitor.go`

**Genuine bug fix?**
**Likely yes** — they read as legitimate fixes
(especially the interface-object type-name correction).
But they are **entangled with request-scoped work** and currently have **no isolated regression coverage**
outside the request-scoped tests.

**Recommended disposition: separate PR**, with **dedicated regression tests**.
This is the highest-risk item in the register for silent breakage:
if these changes are blindly re-created "as part of caching",
they could change behavior for users who never enable caching at all.
They deserve their own scrutiny and their own tests proving the before/after planner behavior.
See the planner seam noted in [01-ARCHITECTURE-SPEC.md](01-ARCHITECTURE-SPEC.md).

---

## Finding 4 — Federation test-harness rewrite (gateway restructure, `http` → `httphandler` rename, options-based gateway)

**What it is.**
The federation test gateway and the example gateway are substantially rewritten.
The gateway constructor signature changed from a config-bytes form
to a functional-options form taking a handler factory, an HTTP client, a logger, a loader-cache map, and option funcs.
New functional-options plumbing, new handler-factory and datasource-observer/subject interfaces,
a new static-gateway file, a moved/expanded datasource poller, and a new gateway main were added.
The internal `http` package was mechanically renamed to `httphandler`
(touching the handler, http, and ws files in both the execution harness and the examples).

**Why it is unrelated to entity caching.**
Much of this plumbs the loader-cache map and subgraph caching configs,
so it is **caching-motivated** — but the rename, the options refactor, and the interface introduction
are **structural test-infrastructure changes** that are reviewable independently of any cache logic.
They inflate the diff substantially without being cache behavior.

**Affected files.**

- `execution/federationtesting/gateway/gateway.go`
- `execution/federationtesting/gateway/gateway_static.go`
- `execution/federationtesting/gateway/datasource_poller.go`
- `execution/federationtesting/gateway/main.go`
- `execution/federationtesting/gateway/httphandler/handler.go`
- `execution/federationtesting/gateway/httphandler/http.go`
- `execution/federationtesting/gateway/httphandler/ws.go`
- `examples/federation/gateway/gateway.go`
- `examples/federation/gateway/httphandler/handler.go`
- (rides along: `examples/federation/*` and `examples/engine/*` go.mod/go.sum churn, the poller move)

**Genuine bug fix?**
No — this is a refactor / restructure, not a fix.

**Recommended disposition: separate PR**, landed **first**,
so the caching test PRs only add cache wiring on top of an already-refactored harness.

**Caveat.**
The project test conventions discourage shared test helpers
(see the repo and `execution/engine` CLAUDE.md rules on self-contained subtests).
This is **harness / gateway code**, not per-test scaffolding,
so it is a different category — but flag it for explicit team sign-off
since it sits adjacent to the no-shared-helpers rule.
This is an open question, not a settled decision (see the open-questions note below).

---

## Finding 5 — Subscription-client transport bug fixes (connection-leak eviction + WS legacy-subprotocol compat)

**What it is.**
The subscription-client transport rewrite itself is already upstream (see Finding 0).
On top of it, the caching branch carries **two small, real bug fixes**:

- **(a)** an SSE close hook so that a naturally-completed stream
  (a complete event, an EOF, or a read error) evicts itself from the transport's connection map.
  Without it, streams that finish without the consumer calling cancel leak a connection / goroutine.
- **(b)** a WebSocket subprotocol-negotiation fallback:
  when the accepted subprotocol is empty and the requested one was the auto value,
  it falls back to the legacy `graphql-ws` subprotocol —
  restoring compatibility with older upstreams / intermediaries
  that strip the `Sec-WebSocket-Protocol` header.

Both fixes come with test additions.

**Why it is unrelated to entity caching.**
These are transport-layer correctness fixes.
They have nothing to do with L1/L2, entity resolution, or cache keys.
They simply ride along in the same branch.

**Affected files.**

- `v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport/sse_conn.go`
- `v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport/sse_transport.go`
- `v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport/ws_transport.go`
- `v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport/sse_conn_test.go`
- `v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/transport/ws_transport_test.go`

**Genuine bug fix?**
**Yes — both are genuine, testable fixes.**
They are small and easy to review in isolation.

**Recommended disposition: separate PR** —
a small standalone "subscription transport fixes" PR.
These are the cleanest extraction in the whole register.

---

## Finding 6 — Repo-meta and dependency-version churn (partly intentional, mostly stale-base artifact)

**What it is.**
The module manifest shows a mix of intentional and accidental changes.

**Intentional (the real external deps the caching work needs):**

- `github.com/wundergraph/astjson` bumped to a two-pass-parser pre-release version,
- `github.com/wundergraph/go-arena` bumped to its newer minor version.

These two are the genuine dependency requirements of the cache implementation —
see [05-ASTJSON-PRIMITIVES.md](05-ASTJSON-PRIMITIVES.md) for why the astjson pre-release is needed.

**Accidental (stale-base artifact):**
the same manifest also shows numerous **downgrades**
(for example `x/sync`, gRPC, protobuf, OpenTelemetry, `x/net`).
These are not deliberate.
They are a side effect of the 2026-02-16 base (Finding 0)
and will reconcile automatically when the work is rebased onto current `origin/master`.

**Other meta noise:**

- an `AGENTS.md` symlink to `CLAUDE.md`,
- `CLAUDE.md` additions,
- a one-line README example fix for the updated response-resolution signature,
- a `go.work` tweak removing a commented-out local astjson replace,
- a `.gitignore` addition,
- the caching docs set and the two caching-related package CLAUDE.md files.

**Why it is unrelated to entity caching.**
The downgrades and the go.work / .gitignore tweaks are incidental —
they carry no caching behavior.
The docs and the two CLAUDE.md files **are** caching content and stay with the caching PRs.

**Affected files.**

- `v2/go.mod`
- `v2/go.sum`
- `go.work`
- `.gitignore`
- `AGENTS.md`
- `CLAUDE.md`
- `README.md`
- `examples/engine/go.mod`
- `examples/federation/go.mod`

**Genuine bug fix?**
No.

**Recommended disposition: split.**

- Keep ONLY the astjson and go-arena bumps — these are real and required.
- **Discard** every downgrade — do not carry it into the clean re-implementation;
  it will resolve on rebase onto current `origin/master`.
- Keep the caching docs and the two package CLAUDE.md files with the caching PRs.
- Treat the rest (symlink, README one-liner, go.work, .gitignore) as incidental;
  fold or drop per the PR plan in [03-PR-PLAN-graphql-go-tools.md](03-PR-PLAN-graphql-go-tools.md).

---

## Disposition summary

| # | Finding | Genuine fix? | Disposition |
|---|---------|--------------|-------------|
| 0 | META: stale local `master` overcounts diff | N/A | Already upstreamed — diff against `origin/master`, change nothing |
| 1 | `onError` / `ErrorBehavior` feature | No (net-new) | Separate PR, before caching; shares files with caching |
| 2 | `service_datasource` `__service` capabilities | No (net-new) | Separate PR, with/before Finding 1; clean new package |
| 3 | Embedded planner `@requires`/`@provides` changes | Likely yes | Separate PR, needs isolated regression tests; highest silent-break risk |
| 4 | Federation test-harness rewrite | No (refactor) | Separate PR, landed first; needs team sign-off |
| 5 | Subscription transport fixes | Yes (both) | Separate PR; cleanest extraction |
| 6 | Repo-meta + dependency churn | No | Split: keep astjson/go-arena bumps + docs, discard downgrades |

## Risks to keep in mind

- Anyone diffing against **local** `master` will badly mischaracterize the branch:
  ~181 of the 397 files are already-merged upstream work.
  All of the "suspect" items a casual reviewer would flag are in that bucket.
  Always use `origin/master` as the base.
- Findings 1 and 2 **share** the `ExecutionOptions` struct and the execution-engine option-builder file with caching.
  They cannot be peeled out by dropping files alone — the edits are interleaved line by line.
- The Finding 3 planner changes alter **non-cached** query planning.
  Re-implemented blindly "as caching", they could silently change behavior for users who never enable caching,
  and they currently lack isolated regression coverage.
- The dependency **downgrades** in Finding 6 are stale-base artifacts.
  Copying them into a fresh branch off current `master` would regress the whole module's dependency tree.

## Open questions (for the team, not resolved here)

- Should Findings 1 + 2 (`onError` behavior + `__service` capability advertisement)
  be one stacked PR landed before the caching stack, given they share `ExecutionOptions` and the option-builder file?
- Are the Finding 3 planner edits standalone fixes with existing upstream issues,
  or are they only correct in the presence of request-scoped resolution?
  They need isolated regression tests either way.
- Is the Finding 4 harness rewrite acceptable as a standalone test-infra PR landed first,
  given the repo's no-shared-test-helpers convention (this is gateway code, not per-test scaffolding)?
- Confirm that local `master` being frozen at 2026-02-16 is intentional for this worktree.
  If so, every reviewer instruction should explicitly say to diff against `origin/master`.

---

**Reminder: this register fixes nothing.**
It is a documentation-only inventory of out-of-scope work.
Extraction into separate PRs, discarding of stale-base artifacts, and regression-test authoring
all happen later, under the PR plans, not as a side effect of writing this file.
