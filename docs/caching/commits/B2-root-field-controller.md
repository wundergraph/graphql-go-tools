# Commit B2 — root-field L2 controller + shadow asymmetry

Plan item: `docs/caching/PLAN.md` §4, B2 (completes Phase B).
RFC sections: RFC-1 §3.6 (`CacheScopeRootField`, `Input` key basis), §3.5 (root-field shadow force-refetch without compare), §5.
Phase: B (L2 root fields).

## Problem

Root-field config did nothing at runtime, and root-field shadow must force-refetch without comparing.

## Solution

Add the runtime root-field L2 path to the controller, reusing the entity primitives.

- `PrepareFetch` dispatches to `prepareRootFieldFetch` when `cfg.KeySpec.Scope == CacheScopeRootField`: render ONE key from the canonical `in.Input` (+ `CacheName`/header prefix), `Get`, coverage-validate against `cfg.ProvidesData`, reorder, and AND-reduce. The whole response is cached as one L2 unit (no `@key` candidates). The existing shadow branch still converts a covering hit to `DecisionFetchShadow` + stash for root-field scope.
- `OnFetchResult` (root-field): after the write gate, write the WHOLE `ResponseData` (StructuralCopy + heap bytes) under the Input-key with `cfg.TTL`.
- `OnFetchSkipped`: splice the cached whole-response value (reusing the entity splice).
- Key rendering refactored to a shared `renderCacheKey(prefix, payload)`; entity (representation preimage) and root-field (Input preimage) differ only in the preimage.

## Key decisions

- Root-field shadow asymmetry: `CompareShadow` is gated to `Scope == CacheScopeEntity` (from A4), so a root-field shadow fetch force-refetches and OVERWRITES L2 but does NOT compare (H5) — the OLD asymmetry, achieved by the existing gate.
- `cfg.L1 == false` for root fields (no L1 path in v1; root->entity L1 promotion is v2).
- The key derives from `Input` (canonical pre-injection), so read key == write key independent of post-prepare mutation.

## Tests

`controller_rootfield_test.go` (white-box, full-value `assert.Equal`): miss -> whole-response write under the Input-key; hit -> `SkipFullHit` + splice (no network); coverage fail -> Fetch; write gate (FetchFailed/HasErrors/null -> no write); **H5** shadow force-refetch + overwrite with `Compares()` empty (no compare); **J** mode matrix (NoOp vs L2 data-equal); key fidelity from canonical `Input` + header hash.

Verification:

- `cd v2 && go test ./pkg/engine/resolve/cache/... -count=1 -race` — PASS (root-field + entity rows).
- `cd v2 && go test ./pkg/engine/resolve/ -count=1` — PASS.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (goldens unchanged).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/resolve/cache/...` — clean.

## Reviewer guidance

- Root-field shadow records NO compare (entity-only); the root-field key is whole-response scoped, derived from `Input`.
- Root-field<->entity reuse (C2) and L1 (D2) remain out of scope.
- Deviation: the optional execution end-to-end root-field hit row was not added (the white-box hit/J rows + the B1 StageL2RootFields golden cover the path).
