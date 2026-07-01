# Task 13 — Root-field L2 caching (plan + runtime)

Phase: B (L2 root fields).
Dependencies: tasks 07, 09.
References: RFC-2 §7.2; RFC-1 §3.5, §5.1(b); appendix rows D/F/I/J (root-field arms), H5.

## Problem

Root-field fetches carry no cache config, a merged fetch may mix policies, and root-field shadow must force-refetch WITHOUT comparing (the historical asymmetry).

## Scope

Two commits, plan then runtime (each independently reviewable).

Commit 1 — plan side (extends task 06's configurator):

- `fetchCacheConfigurator` root-field arm: `rootFieldPolicyForAllRootFields` — a policy only when EVERY root field in the fetch resolves to the SAME policy, else leave `Cache` nil (the conservative decline; task 14 makes mixed fetches rare by splitting them, and this rule stays as the residual safety net).
- Config shape: `L1 = false` (root fields act only as L2 providers; root→entity L1 promotion is a follow-up), `L2 = TTL > 0` (D3), `CacheScopeRootField` key spec (type/field; candidates empty for a plain root field).

Commit 2 — runtime (extends the task 07 controller):

- Render the root-field-scope key (canonical input + header hash + arg suffix per task 09); `Get`/coverage/deferred `Set`, reusing the entity path's primitives — no parallel implementation.
- Root-field shadow asymmetry: on `ShadowMode` + hit, force-refetch and overwrite L2 but DO NOT call `CompareShadow`.
- Normalization (task 09) applies to root-field values the same way (alias-independent reuse across operations).

## Tests

- Configurator rows: single cached root field → FULL `Cache` asserted; mixed-policy merge and cached+uncached merge → `Cache` nil (full-value asserts).
- Runtime rows: root-field arms of D (hit/miss/coverage), F (write gate), the J mode-matrix row over one query (NO-OP/L2 behave identically data-wise; loader branches only on `Decision`); H5 (shadow force-refetch, ZERO compares recorded); root-field flush.
- Plan-driven e2e: a cached root field served from L2 on the second request (zero network, COMPLETE response); an alias-variant operation served from the same entry (task 09 reuse).

## Acceptance criteria

- [ ] Mixed-policy fetches decline L2 (nil `Cache`) — never mis-cache.
- [ ] Root-field shadow records NO compare (H5).
- [ ] Runtime reuses the entity primitives (review the diff for duplication).
- [ ] Lint-clean in both modules.

## Reviewer guidance

- The root-field key is whole-response scoped per field coordinate — verify the key template includes type + field + args, and that read key == write key.
