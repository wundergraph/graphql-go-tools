# gRPC Datasource V2 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce `DataSourceV2` as a second gRPC datasource with a new IR-driven execution architecture while preserving the current datasource as the baseline for correctness and comparison.

**Architecture:** Revert the current datasource package to baseline, preserve the campaign findings in docs, then add `DataSourceV2` in the same package with a compiled IR, schema runtime tables for generated and dynamic schemas, and compatibility fallback to the existing datasource for unsupported fetches. Establish explicit v1 vs v2 tests and benchmarks immediately.

**Tech Stack:** Go, `grpc-go`, `google.golang.org/protobuf`, existing `RPCExecutionPlan`, existing planner/compiler package internals, `pprof`, existing benchmark suite.

---

## Chunk 1: Reset And Preserve

### Task 1: Preserve design and campaign knowledge

**Files:**
- Create: `docs/superpowers/specs/2026-04-18-grpc-datasource-v2-design.md`
- Modify: `IMPROVEMENTS.md`

- [ ] Write the design doc describing why v2 exists, the new IR/runtime, dynamic schema support, and fallback strategy.
- [ ] Keep the improvement ledger intact as the historical record for v1 experiments.

### Task 2: Revert the existing datasource package to baseline

**Files:**
- Restore tracked files under `v2/pkg/engine/datasource/grpc_datasource/`
- Delete experimental files introduced for the v1 campaign under that directory

- [ ] Revert tracked modifications in `compiler.go`, `execution_plan.go`, `grpc_datasource.go`, `json_builder.go`, related tests, and any other v1 experiment files.
- [ ] Delete untracked experimental code files such as `kernel.go` and `kernel_test.go`.
- [ ] Run: `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`

## Chunk 2: Introduce V2 Skeleton

### Task 3: Add the second datasource type and constructor

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

- [ ] Add `DataSourceV2` implementing `resolve.DataSource`.
- [ ] Add `NewDataSourceV2(...)`.
- [ ] Make `DataSourceV2` own:
  - the v2 compiled program
  - the schema runtime
  - a v1 fallback datasource
- [ ] Write a failing test asserting v2 can be constructed and loaded for a simple query.
- [ ] Run the test to watch it fail.
- [ ] Implement the minimal constructor and fallback-backed `Load`.
- [ ] Run: `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2'`

### Task 4: Add the v2 IR and schema-runtime core types

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_ir.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_schema.go`

- [ ] Define `v2Program`, `v2Stage`, `v2Fetch`, opcode enums, operand structs, and support/fallback markers.
- [ ] Define schema runtime tables for messages, fields, and methods that work for both generated and dynamic schemas.
- [ ] Keep the initial IR small but real; do not fake it with plain `RPCCall`.

## Chunk 3: Compile The IR

### Task 5: Lower the existing execution plan into v2 IR

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_compile.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- Test: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

- [ ] Write a failing test that compiles a known query into a non-empty v2 program with stages and fetch records.
- [ ] Run it to confirm failure.
- [ ] Implement plan lowering:
  - stage layout
  - per-fetch request/response metadata
  - fallback flags
  - response path records
- [ ] Run the focused test again.

### Task 6: Compile descriptor-backed schema tables from day one

**Files:**
- Modify: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_schema.go`
- Test: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

- [ ] Write a failing test that compiles runtime schema tables for a proto message without requiring generated Go structs.
- [ ] Run it to confirm failure.
- [ ] Implement descriptor lowering into stable runtime tables.
- [ ] Add generated-type handles opportunistically when available.
- [ ] Run the focused tests again.

## Chunk 4: Build The Runtime

### Task 7: Add a v2 execution kernel with broad fallback

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_fallback.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`

- [ ] Write a failing test asserting v2 routes unsupported fetches through the v1 fallback and preserves output exactly.
- [ ] Run it to confirm failure.
- [ ] Implement runtime stage execution with broad fallback as the default.
- [ ] Preserve exact output and error behavior.
- [ ] Run: `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`

### Task 8: Add the first native v2 fetch path

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_request.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_response.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- Test: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

- [ ] Pick one narrow but real fetch shape and implement native v2 execution for it.
- [ ] Prefer a standard fetch with no resolver dependencies as the first native path.
- [ ] Compile request ops and response projection ops for that fetch.
- [ ] Keep dynamic-schema support by using schema tables, not generated-only logic.
- [ ] Fallback automatically for everything else.
- [ ] Run focused tests and full package tests.

## Chunk 5: Comparison Harness

### Task 9: Add v1 vs v2 comparison tests

**Files:**
- Modify: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

- [ ] Add tests that execute the same query through v1 and v2 and compare JSON output byte-for-byte where stable.
- [ ] Cover:
  - simple standard fetch
  - benchmark-dominant field-resolver query
  - at least one query that forces fallback

### Task 10: Add comparison benchmarks

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_bench_test.go`

- [ ] Add side-by-side benchmarks:
  - `Benchmark_DataSource_V1_Load`
  - `Benchmark_DataSource_V2_Load`
  - `Benchmark_DataSource_V1_Load_WithFieldArguments`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`
- [ ] Ensure both benchmarks use the same setup and query shapes.
- [ ] Run: `cd v2 && go test -run '^$' -bench 'Benchmark_DataSource_(V1|V2)_' -benchmem ./pkg/engine/datasource/grpc_datasource`

## Chunk 6: Record The New Phase

### Task 11: Update the improvement ledger for the v2 reset

**Files:**
- Modify: `IMPROVEMENTS.md`

- [ ] Add a section documenting that v1 was intentionally reset to baseline and why.
- [ ] Add a section documenting the start of the v2 comparison phase.
- [ ] Record benchmark results for v1 baseline vs v2 initial engine.
