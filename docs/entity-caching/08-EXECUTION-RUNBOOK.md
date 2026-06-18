# 08 — Execution Runbook (Codex-driven implementation loop)

> Part of the entity-caching re-implementation document set.
> See [00-OVERVIEW.md](./00-OVERVIEW.md) for navigation,
> [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md) for the clean architecture and integration seam,
> [02-DIRECTIVE-INVENTORY.md](./02-DIRECTIVE-INVENTORY.md) for the directive table,
> [03-PR-PLAN-graphql-go-tools.md](./03-PR-PLAN-graphql-go-tools.md) for the gqtools PR stack (PR 0–21),
> [04-PR-PLAN-router.md](./04-PR-PLAN-router.md) for the router PR stack (R1–R11),
> [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md) for the astjson dependency spec,
> [06-TEST-AND-BENCH-PLAN.md](./06-TEST-AND-BENCH-PLAN.md) for the full test and benchmark plan,
> [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md) for out-of-scope topics,
> and the per-directive specs under [directives/](./directives/) plus the decision records under [adr/](./adr/).

This document is the **operational runbook**.
The other documents say *what* to build and *in what order*.
This one says *how to drive the build*, turn by turn, with two actors.

---

## 0. Who this is for, and the one-sentence model

You are about to execute a stacked-PR plan with two collaborators working as a team.
You need no prior knowledge of the feature to follow this loop;
the specs carry the knowledge, this runbook carries the procedure.

The one-sentence model:
**Claude plans, scopes, and reviews;
Codex CLI writes every line of production and test code;
a human approves before anything is pushed or opened as a PR.**

Nothing in this loop touches an existing branch.
Every PR starts from a fresh branch cut off the up-to-date default branch.

---

## 1. The two roles

There are exactly two automated actors, plus one human gate.

### 1.1 Claude — orchestrator, planner, reviewer

Claude (this assistant) never writes production code in this workflow.
Claude's job, per PR, is:

- Read the relevant spec section, the directive spec, and the ADR for the PR being built.
- Write a short **reviewer guide** for the PR (see [§4](#4-the-reviewer-guide-written-first)).
- Hand Codex a **tightly-scoped task** that references the spec and reviewer guide.
- **Review the resulting diff** against the spec, the reviewer guide, and the repo conventions.
- Drive the iteration with Codex until the diff is correct.
- Run the test and benchmark gates.
- Mark the PR ready and stop at the human gate.

Claude consults Codex for an independent second opinion via the `/codex` skill
(`codex review` for a diff gate, `codex challenge` to try to break the code,
`codex consult` for a design question).
That is a *review* use of Codex, distinct from the *authoring* use below.

### 1.2 Codex CLI — every coding task

Codex CLI is the implementer.
Every change to `.go`, `_test.go`, `go.mod`, YAML config, or docs that ship in a PR
is produced by Codex, driven by a task Claude writes.
Codex works test-first (TDD), applies modern Go idioms, and iterates on Claude's review notes.

Claude does not edit the source tree directly during a PR build.
The single exception is the planning docs under this directory
(`_entity-caching-reimpl/`), which are Claude's own artifacts and not part of any code PR.

### 1.3 The human — the approval gate

A human reviews and approves before:

- any `git push`,
- any `gh pr create`,
- any merge into the feature branch.

This is a hard rule.
The loop runs autonomously up to each gate, then waits.
Per the global instructions, neither actor ever comments, reviews, or merges
on GitHub on the human's behalf — the actors *prepare* PR bodies and review notes,
the human *posts* and *merges*.

---

## 2. One-time setup before the first PR

Do this once, before PR 0.

### 2.1 Confirm the base ref

In this worktree the **local** `master` ref is frozen at 2026-02-16 and is **not**
the same as `origin/master` (2026-06-12).
Diffing against the stale local ref overcounts the change by ~181 already-merged files.
See [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md) for the full explanation.

Always branch from and diff against the **remote** default branch:

```sh
git fetch origin
# gqtools default branch is `master`; the cosmo router default is `main`.
```

### 2.2 Create the long-lived feature branch (gqtools)

Per [03-PR-PLAN-graphql-go-tools.md §1.1](./03-PR-PLAN-graphql-go-tools.md):

```sh
git checkout -b feat/entity-caching origin/master
git push -u origin feat/entity-caching   # human-gated push
```

The router stack uses its own feature branch off `origin/main`
(see [04-PR-PLAN-router.md](./04-PR-PLAN-router.md)).

Every code PR targets its feature branch as the merge base.
The feature branch merges to the default branch exactly once, at the very end.

### 2.3 Load the Go guidelines once per session

Before Codex writes any Go, the `modern-go-guidelines` skill must be loaded
(`/use-modern-go`) so range-over-int, range-over-func, structured logging, and
the other current idioms are applied rather than legacy patterns.
This is restated in the per-PR loop because sessions reset.

---

## 3. Branch and worktree hygiene (NO existing branch is ever touched)

This is the most important safety property of the loop.

- **Every PR gets a brand-new branch**, named `feat/entity-caching-<nn>-<slug>`
  for gqtools PRs (e.g. `feat/entity-caching-03-cache-key-templates`)
  and `feat/entity-caching-r<nn>-<slug>` for router PRs.
- The branch is cut **fresh** off its parent:
  PR N stacks on PR N-1, so PR N branches off **PR N-1's branch tip**, never off an unrelated branch
  (see [03-PR-PLAN-graphql-go-tools.md §1.2](./03-PR-PLAN-graphql-go-tools.md)).
  PR 1 and the independent leaf PRs branch off the feature branch tip.
- **No existing branch is ever rebased, force-pushed, or reused** to carry new work.
  If a dependency merges and a child needs the new tip, the child is **rebased onto the new
  parent tip on its own branch** — the parent branch itself is left alone.
- Prefer a **separate git worktree per active PR** so parallel PRs never share a working
  directory or step on each other's `go.work` state.
  Create one with `git worktree add ../ec-<nn>-<slug> <new-branch>`;
  remove it with `git worktree remove` once the PR has merged.

The test, at the end of every PR: the only branch this PR's commits exist on is
the new branch created for it; no pre-existing branch has a single new commit.

---

## 4. The reviewer guide, written first

Before Codex touches code, Claude writes a **reviewer guide** for the PR.
Writing it first forces the scope to be pinned down before any code exists,
and it doubles as the spec Codex implements against and the rubric Claude reviews against.

A reviewer guide is **short** (a screen or two) and contains:

- **What this PR adds**, in two or three sentences, for a reader with no context.
- **The exact files** expected to change, and which must *not* change.
- **The contracts** introduced or consumed — tiny signatures only, e.g.
  `Get(ctx, keys []string) ([]*CacheEntry, error)`, never a pasted body.
- **The acceptance criteria**, copied from the PR's entry in the PR-plan doc.
- **The conventions that bite here** — for a test PR, the exact-assertion and
  full-struct rules from [06-TEST-AND-BENCH-PLAN.md §2](./06-TEST-AND-BENCH-PLAN.md);
  for a merge-site PR, the Copy-Budget triangle from [§7 of the test plan](./06-TEST-AND-BENCH-PLAN.md).
- **The link to the authoritative spec** — the PR's `Reviewer-guide doc` field already
  names it (architecture spec section, directive spec, or ADR); the reviewer guide
  points at it rather than restating it.

Most PRs already name their reviewer-guide doc in the PR-plan entry
(e.g. PR 3 → [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md), PR 1 → the architecture spec + ADR).
The per-PR reviewer guide Claude writes is the *thin task wrapper* around that doc,
not a duplicate of it.

The reviewer guide is one of Claude's planning artifacts; it is not committed to the code PR.

---

## 5. The per-PR loop

This is the core of the runbook.
Run it once per PR, in order, for both the gqtools stack (PR 0–21) and the router stack (R1–R11).

### 5.1 The seven steps

1. **Scope.**
   Claude opens the PR's entry in the PR-plan doc, reads its `Goal` / `Scope` / `Excludes` /
   `Dependencies` / `Acceptance criteria`, reads the named reviewer-guide doc and any directive
   spec + ADR, and confirms every dependency PR has already merged into the feature branch.
   If a dependency is unmerged, stop — this PR is not ready.

2. **Reviewer guide.**
   Claude writes the reviewer guide ([§4](#4-the-reviewer-guide-written-first)).

3. **Branch.**
   Claude (or the human) cuts a fresh branch off the correct parent and, preferably, a worktree
   ([§3](#3-branch-and-worktree-hygiene-no-existing-branch-is-ever-touched)).
   No push yet.

4. **Instruct Codex.**
   Claude hands Codex a single tightly-scoped task ([§6](#6-how-claude-instructs-codex)).
   The task says: build exactly what the reviewer guide describes, test-first,
   modern Go, touch only the listed files.

5. **Review and iterate.**
   Codex implements; Claude reviews the diff against the reviewer guide and the spec
   ([§7](#7-how-claude-reviews-the-diff)); Claude returns precise change requests;
   Codex revises.
   Repeat until the diff matches the guide and passes review.
   Optionally run an independent `codex review` pass as a second gate.

6. **Gate: tests and benchmarks.**
   Run the test and benchmark gates for this PR ([§8](#8-the-test-and-benchmark-gates)).
   All must pass, `-race` clean, and any benches in scope must hold their regression budget.

7. **Mark ready, stop at the human gate.**
   Claude prepares the PR body (drafted, not posted), summarizes the diff and the gate results,
   and **stops**.
   The human reviews, then pushes the branch and opens the PR.

### 5.2 What "tightly scoped" means

A good Codex task is one PR's worth of work and no more.
The PR-plan already sizes each PR to be reviewable in under ~30 minutes and lists its `Excludes`
(the adjacent work that belongs to *other* PRs).
The Codex task must restate those `Excludes` so Codex does not "helpfully" pull in the next PR's code.
Example: PR 2 (loader copy helpers) explicitly excludes any *caller* of the helpers — they are
dead code until PR 7.
The task must say so, or Codex will wire them up and balloon the diff.

---

## 6. How Claude instructs Codex

Each Codex task is a short, self-contained brief.
It contains, in order:

- **One-line goal** — copied from the PR's `Goal`.
- **The reviewer guide** — pasted or referenced by path.
- **Files to change / files to leave alone** — the explicit allow-list and the `Excludes`.
- **Contracts** — tiny signatures only, no bodies.
- **Test-first instruction** — write the failing tests named in
  [06-TEST-AND-BENCH-PLAN.md §5](./06-TEST-AND-BENCH-PLAN.md) for this directive first,
  then make them pass; mirror the reference test file named there.
- **Convention reminders** — for the resolve package, set
  `DisableSubgraphRequestDeduplication = true` and build the arena/loader per the canonical unit
  shape; for `execution/engine`, **no new shared helpers**, inline GraphQL, full normalized
  snapshot asserts, duplication-over-sharing.
  These are not optional and are restated every task because they are the most common miss.
- **Modern Go reminder** — apply current idioms (`/use-modern-go` loaded).
- **Stop condition** — the PR's `Acceptance criteria`.

A Codex task must never say "and also clean up nearby code" or "refactor while you're there".
Surgical changes only; every changed line traces to the PR's stated scope.
If Codex notices unrelated dead code or a real adjacent bug, it reports it back to Claude,
who records it for [07-UNRELATED-FINDINGS.md](./07-UNRELATED-FINDINGS.md) — it is not fixed in this PR.

---

## 7. How Claude reviews the diff

Claude reviews every Codex diff before the human ever sees it.
The review checks, in order:

- **Scope.** Only the allow-listed files changed; nothing from the `Excludes` leaked in;
  no unrelated formatting or "improvements" to adjacent code.
- **Contract fidelity.** The signatures match the spec exactly (e.g. the real
  `LoaderCache.Set(ctx, entries []*CacheEntry) error` with per-entry TTL — *not* the stale
  doc's `Set(ctx, entries, ttl)`; the real `RenderCacheKeys(a arena.Arena, ctx, items, prefix)`).
  The known doc-drift traps are catalogued in the public-API findings and called out in the specs —
  follow the code, not the stale `ENTITY_CACHING_INTEGRATION.md`.
- **Invariants.** For any cache read/write/merge: StructuralCopy isolation on every cache boundary,
  working-copy-and-swap on merge-into-existing (never mutate a live L1 entry in place),
  L1 main-thread-only, fail-closed on nil `ProvidesData`, and the L1-gating flag checks.
  These are the load-bearing rules from [01-ARCHITECTURE-SPEC.md](./01-ARCHITECTURE-SPEC.md)
  and the project `CLAUDE.md`.
- **Tests.** Exact assertions only (`assert.Equal` on full values — never `Contains`,
  `GreaterOrEqual`, or fuzzy comparisons), full-struct asserts, inline literals,
  one-item-per-line for multi-key literals, a "why" comment on every snapshot/log event line,
  and the `ClearLog → GetLog + assert` pairing with no orphan clears.
- **No code comments referencing PRs, issues, review threads, or reviewer names** —
  comments explain behavior, not history.
- **Acceptance criteria met**, verbatim from the PR-plan entry.

Where useful, Claude runs `codex review` for an independent pass and `codex challenge`
to try to break the new code — a second opinion before the human gate.

If the review finds problems, Claude writes precise, minimal change requests and hands them
back to Codex.
Claude does not silently fix the code itself.

---

## 8. The test and benchmark gates

Every PR must clear its gates before it is marked ready.
Commands are from [06-TEST-AND-BENCH-PLAN.md §6.7](./06-TEST-AND-BENCH-PLAN.md).

### 8.1 Build and unit tests

```sh
go build ./...
go test ./v2/pkg/engine/resolve/...
go test -race ./v2/pkg/engine/resolve/...
```

For E2E PRs, also:

```sh
go test ./execution/engine/...
```

Targeted runs while iterating (faster feedback):

```sh
go test -run TestL1Cache ./v2/pkg/engine/resolve/... -v
go test -run TestFederationCaching ./execution/engine/... -v
```

### 8.2 Benchmarks (only PRs in the benchmark scope)

A PR that touches a StructuralCopy merge site, the loader hot path, or the analytics collector
must run its benchmark and compare against the base.
The regression gates are the overhead ladder, the Copy-Budget merge benches, and the
non-caching floor (see [06-TEST-AND-BENCH-PLAN.md §6.8](./06-TEST-AND-BENCH-PLAN.md)).

```sh
# base branch
go test -run=^$ -bench 'BenchmarkCachingOverhead|BenchmarkMerge|BenchmarkNonCaching' \
  -benchmem -count=10 ./v2/pkg/engine/resolve/... | tee /tmp/before.txt
# change branch
go test -run=^$ -bench 'BenchmarkCachingOverhead|BenchmarkMerge|BenchmarkNonCaching' \
  -benchmem -count=10 ./v2/pkg/engine/resolve/... | tee /tmp/after.txt
benchstat /tmp/before.txt /tmp/after.txt
```

Two rungs to watch hardest:
`ConfiguredButDisabled` must stay at parity with `Disabled` (any gap is a guard leak),
and each `BenchmarkMerge*` must stay within **one** StructuralCopy of the matching
`BenchmarkNonCaching*` floor.

### 8.3 The acceptance-criteria sync

If the PR added or changed any caching test, it must update
`docs/entity-caching/ENTITY_CACHING_ACCEPTANCE_CRITERIA.md`:
every new/changed test linked from its AC with relative path + line + name
(see [06-TEST-AND-BENCH-PLAN.md §2.4](./06-TEST-AND-BENCH-PLAN.md)).
This is part of the gate, not an afterthought.

---

## 9. How PRs stack and stay mergeable

The stacking discipline is owned by the PR-plan docs; this is the operational summary.

- PRs stack **linearly**: PR N branches off PR N-1's branch
  (see [03-PR-PLAN-graphql-go-tools.md §1.2](./03-PR-PLAN-graphql-go-tools.md)).
- A PR merges into the feature branch (squash) **only after** the PR(s) it depends on have merged.
- When a dependency merges, **rebase the child branch onto the new feature-branch tip**
  before merging the child — on the child's own branch, never mutating the parent.
- Use `gh pr create --base <parent-branch>` so the review diff is scoped to just that PR's changes.
- **Every PR is independently mergeable into the feature branch** and leaves the feature branch
  green: data-only PRs ship structs/interfaces the runtime ignores until a later "wire it on" PR;
  the two behavior-flipping PRs in gqtools are the loader-integration PR (PR 7) and the
  visitor-wiring PR (PR 11) — see [03-PR-PLAN-graphql-go-tools.md §1.4](./03-PR-PLAN-graphql-go-tools.md).
- The hard external prerequisite is **PR 0** (cut + pin a real astjson release): nothing in the
  stack compiles until the astjson primitives are released and pinned.
  See [05-ASTJSON-PRIMITIVES.md](./05-ASTJSON-PRIMITIVES.md).
- The router stack (R1–R11) waits on the matching gqtools releases per the dependency table in
  [04-PR-PLAN-router.md](./04-PR-PLAN-router.md); R1 cannot start until the gqtools foundation is released.

Interleave test PRs with code PRs per the test plan's interleaving rule, so the feature branch is
never carrying behavior without coverage.

---

## 10. The human approval gate (where the loop pauses)

The loop runs autonomously, then **stops and waits for a human** at three points:

1. **Before `git push`** — the new branch and its commits exist only locally until approved.
2. **Before `gh pr create`** — Claude prepares the PR body (drafted); the human opens the PR.
3. **Before merge into the feature branch** — the human merges; the actors never merge.

Neither Claude nor Codex posts, comments, reviews, approves, or merges on GitHub on the human's
behalf — this is a hard global rule.
Reading from GitHub (e.g. `gh pr view`, `gh api` to fetch CI status) is allowed; writing is not.

At each gate Claude presents: the diff summary, the gate results (tests, race, benchmarks),
the acceptance-criteria checklist, and the drafted PR body — then ends its turn.

---

## 11. Final verification per feature

A feature (a directive end-to-end, or the foundation, or the router integration) is **done** only
after a full, clean run across both the unit and E2E tiers plus the benchmark gates.

Per directive / per feature, run and confirm green:

```sh
go build ./...
go test ./v2/pkg/engine/resolve/...
go test -race ./v2/pkg/engine/resolve/...
go test ./execution/engine/...
go test -run=^$ -bench 'BenchmarkCachingOverhead|BenchmarkMerge|BenchmarkNonCaching' \
  -benchmem -count=10 ./v2/pkg/engine/resolve/...
```

Then confirm:

- the benchmark comparison shows no regression beyond the budget
  (`ConfiguredButDisabled ≈ Disabled`; each `Merge*` within one copy of its `NonCaching*` floor);
- `ENTITY_CACHING_ACCEPTANCE_CRITERIA.md` lists every test for this feature with path + line + name;
- the per-directive test matrix in [06-TEST-AND-BENCH-PLAN.md §5](./06-TEST-AND-BENCH-PLAN.md)
  is fully covered for the directive;
- the @requestScoped E2E tests (AC-RS-01..07) are *un-skipped and exact* when the planner work has
  landed (never copy the placeholder fuzzy `if reviewsCalls == 0` smoke checks).

When the full gqtools stack is green and the router stack is integrated, the feature branch is
merged into the default branch — once, at the human gate.

---

## 12. Per-PR checklist template

Copy this block per PR.
Fill the header from the PR's entry in the PR-plan doc, then work top to bottom.

```text
PR: <nn> — <title>            Stack: gqtools | router
Branch: feat/entity-caching-<nn>-<slug>   (fresh, off <parent branch>)
Worktree: ../ec-<nn>-<slug>
Depends on (must be merged): PR <...>
Spec / reviewer-guide doc: <link from the PR-plan entry>
Directive spec: directives/<name>.md (if any)   ADR: adr/00NN-<name>.md (if any)

SCOPE
[ ] Read PR-plan entry: Goal / Scope / Excludes / Dependencies / Acceptance criteria
[ ] Read the named reviewer-guide doc (+ directive spec + ADR)
[ ] All dependency PRs confirmed merged into the feature branch

PREP
[ ] Reviewer guide written (what it adds, files touched, contracts, AC, conventions)
[ ] Fresh branch cut off the correct parent — NO existing branch touched
[ ] Worktree created
[ ] /use-modern-go loaded for this session

IMPLEMENT (Codex)
[ ] Codex task written: goal + reviewer guide + file allow-list + Excludes + contracts
[ ] Test-first: failing tests added mirroring the §5 reference file
[ ] Implementation makes them pass; only allow-listed files changed
[ ] Convention reminders included (exact asserts, no shared E2E helpers, inline GraphQL)

REVIEW (Claude)
[ ] Scope clean: no Excludes leaked, no unrelated edits/formatting
[ ] Contracts match spec exactly (watch the documented doc-drift signatures)
[ ] Invariants held: StructuralCopy isolation, working-copy-and-swap, L1 main-thread-only,
    fail-closed on nil ProvidesData, L1-gating flag checks
[ ] Tests: exact assertions, full-struct, inline literals, one-item-per-line,
    "why" comments, ClearLog→GetLog+assert pairing
[ ] No PR/issue/reviewer references in code comments
[ ] (optional) codex review + codex challenge passed

GATES
[ ] go build ./...  green
[ ] go test ./v2/pkg/engine/resolve/...  green
[ ] go test -race ./v2/pkg/engine/resolve/...  clean
[ ] go test ./execution/engine/...  green (if E2E in scope)
[ ] Benchmarks run + benchstat vs base within budget (if in bench scope)
[ ] ENTITY_CACHING_ACCEPTANCE_CRITERIA.md updated (path + line + name per test)
[ ] Acceptance criteria from the PR-plan entry all met

READY (human gate)
[ ] Diff summary + gate results + drafted PR body prepared
[ ] STOP — human pushes branch, opens PR, and merges (actors never do)
[ ] After merge: rebase dependent child branch onto new feature-branch tip; remove worktree
```

---

## 13. Quick reference — the loop in one screen

1. Scope from the PR-plan entry; confirm dependencies merged.
2. Claude writes the reviewer guide.
3. Cut a fresh branch + worktree off the correct parent (no existing branch touched).
4. Claude hands Codex a tightly-scoped, test-first, modern-Go task.
5. Codex implements; Claude reviews against the spec + conventions; iterate.
6. Run tests (`-race`) + benchmarks (benchstat vs base); update the AC doc.
7. Mark ready, draft the PR body, **stop at the human gate** for push / PR / merge.
8. After merge, rebase the next child onto the new tip; remove the worktree.
