# gRPC Datasource Ultra-Performance Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current interpreter-style gRPC datasource with a compiled Go execution engine that minimizes heap allocation, sharply reduces CPU cycles per request, and makes the hot path almost entirely precomputed.

**Architecture:** The datasource becomes a two-phase system. A cold-path compiler turns a GraphQL operation, protobuf schema, and mapping into a specialized execution program with fixed batches, direct request builders, direct context extractors, and direct response write instructions; the hot path only binds request values, runs precompiled stages, and emits the final GraphQL response. The protobuf layer becomes pluggable so the engine can run on generated/vtprotobuf fast paths for known schemas and a compiled dynamic runtime for unknown schemas, while `dynamicpb` remains only as a compatibility fallback.

**Tech Stack:** Go 1.25, `grpc-go`, `google.golang.org/protobuf`, `vtprotobuf`, optional `hyperpb`-style dynamic runtime, Go PGO, `pprof`, existing test/benchmark suite.

---

## Radical Thesis

The current datasource is architecturally wrong for extreme performance because it behaves like a generic runtime interpreter on every request:

- it rebuilds execution state in `Load`
- it re-discovers request/response structure at runtime
- it walks protobuf and JSON trees generically
- it materializes intermediate structures only to merge them later

The highest-impact Go-only redesign is:

1. Compile each datasource instance into an operation-specific execution kernel.
2. Replace reflection-heavy protobuf handling with a pluggable high-performance runtime.
3. Eliminate intermediate JSON subtree assembly and write directly into the final response shape.

Everything else is secondary.

---

## File Map

### Existing files to modify

- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/fetch.go`
- `v2/pkg/engine/datasource/grpc_datasource/json_builder.go`
- `v2/pkg/engine/datasource/grpc_datasource/execution_plan.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/fetch_test.go`

### New files to create

- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_program.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_program_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_compile.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_compile_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_request_builder.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_request_builder_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_context_extractor.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_context_extractor_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_response_writer.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_response_writer_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_memory.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_memory_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/proto_runtime.go`
- `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_dynamicpb.go`
- `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_vtproto.go`
- `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_compiled_dynamic.go`
- `v2/pkg/engine/datasource/grpc_datasource/perf_test.go`

### Optional later files

- `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_codegen.go`
- `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_hyperpb.go`
- `v2/pkg/engine/datasource/grpc_datasource/internal/generated/`

---

## Performance Targets

- Turn `Load` into an executor over a precompiled kernel, not a runtime planner/compiler.
- Remove per-request dependency graph creation and sorting entirely.
- Remove generic name-based field lookup from the hot path entirely.
- Remove `[]map[string]protoref.Value` and similar generic intermediate representations entirely.
- Replace intermediate response subtree creation with direct final-response writes.
- Reduce `Benchmark_DataSource_Load_WithFieldArguments` allocs/op by an order of magnitude.
- Reduce package-side CPU in request compilation, context extraction, and merge work by an order of magnitude.
- Leave gRPC transport and protobuf decode/encode as the dominant remaining cost.

---

## External Architecture Patterns Applied Here

- Apollo Router pushes planning into a native cold path and caches the result. We should do the same inside the datasource, per operation.
- Envoy uses shared-nothing workers and thread-local state. We should use sharded/request-local scratch and bounded execution, not request-byte-keyed global-ish reuse.
- NGINX keeps copies minimal. We should stop building transient JSON/protobuf object graphs that are immediately merged or discarded.
- gRPC recommends channel reuse and only using more exotic transport patterns when the transport itself becomes the bottleneck. Our current bottleneck is above transport.
- `vtprotobuf` proves that unrolled generated code is dramatically better than generic reflection in Go.
- `hyperpb` proves that even dynamic protobuf can be treated like a compiled runtime instead of a generic reflective interpreter.
- Go’s PGO and recent allocation work reinforce the same conclusion: fewer heap objects and more precompiled structure win.

---

## Chunk 1: Build The Kernel Boundary

### Task 1: Introduce The Kernel Abstraction

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`

- [ ] Define a `kernel` object that owns the fully compiled execution program for one datasource instance.
- [ ] Make `NewDataSource` compile the kernel once and store it.
- [ ] Make `Load` delegate to `kernel.Execute(...)`.
- [ ] Preserve existing public behavior and tests.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`

### Task 2: Represent The Operation As A Compiled Program

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_program.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_program_test.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_compile.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_compile_test.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/fetch.go`

- [ ] Compile the current `RPCExecutionPlan` into fixed execution stages:
  - stage order
  - batch members
  - dependency links
  - precomputed method names
  - precomputed response merge routing
- [ ] Remove per-request `NewDependencyGraph(...)`.
- [ ] Remove per-request `TopologicalSortResolve(...)`.
- [ ] Keep a minimal mutable request-state array for stage outputs only.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
  - `cd v2 && go test -run '^$' -bench BenchmarkBuildDependencyGraph -benchmem ./pkg/engine/datasource/grpc_datasource`

---

## Chunk 2: Compile Request Construction

### Task 3: Replace Runtime Message Building With Compiled Request Builders

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_request_builder.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_request_builder_test.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel_compile.go`

- [ ] Compile each RPC request shape into a specialized builder program.
- [ ] Pre-resolve:
  - protobuf field descriptors
  - field numbers
  - nullability checks
  - oneof routing
  - repeated/list behavior
  - argument-slot reads
- [ ] The builder should operate on fixed slots and direct descriptor handles, not names.
- [ ] Split builders by call kind:
  - standard/entity
  - resolve
  - required
- [ ] Keep `dynamicpb` compatibility while removing generic per-request interpretation.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
  - `cd v2 && go test -run '^$' -bench Benchmark_DataSource_Load -benchmem ./pkg/engine/datasource/grpc_datasource`

### Task 4: Compile Resolver Context Extraction

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_context_extractor.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_context_extractor_test.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/compiler.go`

- [ ] Replace `resolveContextData`, `resolveContextDataForPath`, `resolveListDataForPath`, and `resolveDataForPath` as the main runtime path.
- [ ] Compile resolver context extraction into direct extraction programs that:
  - traverse known parent output shapes
  - write directly into the next request’s repeated `context` field
  - preserve response order alignment
- [ ] Eliminate `[]map[string]protoref.Value`.
- [ ] Eliminate per-field map growth in resolver batching.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
  - `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load_WithFieldArguments$' -benchmem ./pkg/engine/datasource/grpc_datasource`

---

## Chunk 3: Replace The Protobuf Runtime

### Task 5: Add A Pluggable Proto Runtime Layer

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/proto_runtime.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_dynamicpb.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_vtproto.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_compiled_dynamic.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel_request_builder.go`

- [ ] Define a runtime interface for:
  - request allocation/reset
  - response allocation/reset
  - unmarshal/marshal
  - descriptor-backed field access
  - optional pooled message reuse
- [ ] Keep `dynamicpb` only as fallback compatibility mode.
- [ ] Make the kernel depend on the runtime interface only.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`

### Task 6: Add The Generated Fast Path

**Files:**
- Modify: `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_vtproto.go`
- Optional create: `v2/pkg/engine/datasource/grpc_datasource/internal/generated/...`

- [ ] For known schemas, support generated message types with `vtprotobuf` marshal/unmarshal and pool helpers.
- [ ] Use the generated fast path wherever the schema is known at build time.
- [ ] Allow mixed mode so not all message types need generated support on day one.
- [ ] Benchmark the generated fast path against `dynamicpb`.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
  - `cd v2 && go test -run '^$' -bench '^Benchmark_DataSource_Load_WithFieldArguments$' -benchmem ./pkg/engine/datasource/grpc_datasource`

### Task 7: Add The Compiled Dynamic Fast Path

**Files:**
- Modify: `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_compiled_dynamic.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel_compile.go`

- [ ] Introduce a compiled dynamic protobuf backend for runtime-loaded schemas.
- [ ] The backend must:
  - compile message types once from descriptors
  - reuse parse/runtime state aggressively
  - avoid generic reflection-heavy field lookup during decode
- [ ] Model this after `hyperpb`’s compiled runtime approach, but keep the integration Go-native and repo-owned.
- [ ] Treat this as the long-term replacement for `dynamicpb` in the hot path.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
  - `cd v2 && go test -run '^$' -bench '^Benchmark_DataSource_Load_WithFieldArguments$' -benchmem ./pkg/engine/datasource/grpc_datasource`

---

## Chunk 4: Replace Response Tree Construction

### Task 8: Build A Direct Final-Response Writer

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_response_writer.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_response_writer_test.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/json_builder.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel.go`

- [ ] Stop creating one intermediate `astjson` subtree per service call as the primary runtime path.
- [ ] Compile response write instructions for each RPC result:
  - root merge writes
  - resolver path writes
  - entity ordering writes
  - nested list writes
  - optional/null handling
- [ ] Write directly into the final response object or final response buffer using the precomputed program.
- [ ] Keep the old builder only as a temporary parity oracle during migration.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
  - `cd v2 && go test -run '^$' -bench Benchmark_DataSource_Load -benchmem ./pkg/engine/datasource/grpc_datasource`

### Task 9: Remove Generic Merge Traversal

**Files:**
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel_response_writer.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/json_builder.go`

- [ ] Replace `flattenObject`, `flattenList`, and generic resolver merge traversal with precompiled parent-child alignment programs.
- [ ] Preserve exact alias/null/list semantics.
- [ ] Add tests for:
  - sibling resolvers
  - nested resolvers
  - null parents
  - federation/entity ordering
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`

---

## Chunk 5: Memory Model And Execution Model

### Task 10: Introduce Kernel-Owned Memory Arenas And Sharded Scratch

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_memory.go`
- Create: `v2/pkg/engine/datasource/grpc_datasource/kernel_memory_test.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`

- [ ] Replace request-byte-keyed pool reuse with kernel-owned sharded scratch state.
- [ ] Pool:
  - request slot arrays
  - temporary decode/build buffers
  - response writer state
  - optional proto objects where runtime allows safe reset/reuse
- [ ] Keep pools bounded and shard-local.
- [ ] Optimize for stable high-throughput workloads, not byte-identical request reuse.
- [ ] Run:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
  - `cd v2 && go test -run '^$' -bench Benchmark_DataSource_Load -benchmem ./pkg/engine/datasource/grpc_datasource`

### Task 11: Make Execution Bounded And Worker-Like

**Files:**
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/kernel_memory.go`

- [ ] Replace unconditional per-stage goroutine fan-out with a bounded execution model.
- [ ] Inline tiny batches.
- [ ] Use worker-local scratch for larger batches.
- [ ] Add backpressure and clear concurrency limits so throughput does not turn into memory blow-up.
- [ ] Run concurrency benchmarks at multiple parallelism levels.

---

## Chunk 6: Validation And Build Optimization

### Task 12: Rebuild The Benchmark Suite Around The Kernel

**Files:**
- Create: `v2/pkg/engine/datasource/grpc_datasource/perf_test.go`
- Modify: `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_test.go`

- [ ] Add isolated benchmarks for:
  - kernel compile
  - request build
  - context extraction
  - proto runtime decode
  - direct response writing
  - end-to-end load
- [ ] Add benchmarks for:
  - single root fetch
  - one resolver stage
  - two sibling resolver stages
  - nested resolver chain
- [ ] Capture fresh CPU and allocation profiles after every major chunk.

### Task 13: Apply Go PGO To The New Hot Path

**Files:**
- Create when stable: `v2/default.pgo`

- [ ] Collect representative CPU profiles from the new kernel-based runtime.
- [ ] Build with Go PGO.
- [ ] Keep PGO only if it improves the final kernel, not intermediate experiments.
- [ ] Run:
  - `cd v2 && go test -run '^$' -bench '^Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments$' -benchmem ./pkg/engine/datasource/grpc_datasource`

---

## Non-Goals For The First Wave

- Do not start with small helper cleanups as the main project.
- Do not spend the first phase hand-tuning existing `dynamicpb` interpreter code.
- Do not treat `sync.Pool` or tiny hash-map fixes as the strategy.
- Do not introduce Rust, C++, or a sidecar. This remains Go-only.

Those may still happen as cleanup work, but they are explicitly not the center of the plan.

---

## Expected End State

- `DataSource.Load` is a thin wrapper around a compiled kernel executor.
- The operation DAG, batches, service names, request shapes, response write paths, and dependency routing are all precompiled.
- Request construction is specialized and slot-based.
- Resolver context propagation is direct and allocation-light.
- Response emission writes directly into the final result shape.
- The protobuf runtime is no longer synonymous with `dynamicpb`.
- The hot path is dominated by actual RPC I/O and decode/encode work, not internal orchestration overhead.

---

## Validation Checklist

- [ ] End-to-end behavior matches existing tests.
- [ ] Per-request graph/sort work is gone from profiles.
- [ ] Generic field-name lookup is gone from profiles.
- [ ] `resolveContextData`-style map building is gone from the hot path.
- [ ] Intermediate response subtree materialization is no longer dominant.
- [ ] `Benchmark_DataSource_Load_WithFieldArguments` shows order-of-magnitude improvement in allocs/op and material CPU reduction.
- [ ] The generated backend beats the fallback backend on representative workloads.

---

## Execution Guidance

Implement in this order:

1. Kernel boundary and compiled program
2. Compiled request builders
3. Compiled context extraction
4. Direct response writer
5. Proto runtime replacement
6. Memory/execution model tightening
7. PGO

If a task does not directly advance one of those seven items, it is probably not on the critical path.
