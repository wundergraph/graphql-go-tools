# gRPC Datasource V2 Design

## Goal

Introduce a second gRPC datasource implementation that takes a fundamentally different route from the current interpreter-style datasource:

- preserve all current behavior
- handle generated and dynamic schemas from day one
- keep the current datasource intact as the compatibility baseline
- make direct v1 vs v2 benchmarking possible on the same operations

The new engine must be structurally capable of ultra-high performance, even if early iterations still rely on compatibility fallback for portions of behavior.

## Why A Second Datasource

The current datasource has been improved materially, but the remaining ceiling is architectural:

- request construction still depends on generic message semantics
- dynamic-schema handling still leans on protobuf generic runtime behavior
- response assembly still builds and merges `astjson` subtrees
- fallback behavior and performance experimentation are entangled in the same implementation

Keeping the existing datasource as-is and building `DataSourceV2` alongside it gives four advantages:

1. direct correctness comparison against the known-good path
2. direct benchmark comparison without archaeology
3. low-risk fallback for unsupported or not-yet-ported behavior
4. freedom to design a new runtime without contaminating the old one

## Useful Findings From The First Optimization Campaign

These findings should shape v2 from the start:

1. Compiling scheduling out of the hot path helped immediately.
2. Generated-type allocation was the largest single practical win.
3. Generic reflection-style resolver/context interpreters were not enough by themselves.
4. Generated response writing helped, but not enough, because the final response pipeline stayed generic.
5. Deleting intermediate resolve structures only worked when concurrency was preserved.
6. The true remaining ceiling is not a single hotspot. It is the interpreter model itself.

That means v2 must not be "v1 plus more fast paths". It needs its own runtime model.

## V2 Thesis

`DataSourceV2` is an operation-compiled engine with its own IR.

Cold path:

- use the existing planner to obtain a correct `RPCExecutionPlan`
- lower that plan into a compact runtime IR
- compile proto descriptors into schema tables and access programs
- precompute request builders, response writers, and fallback boundaries

Hot path:

- bind variables into slots
- run compiled stage programs
- build requests using IR, not recursive `RPCMessage` interpretation
- decode/access protobuf through a schema runtime, not ad hoc reflection
- write the final response through compiled response programs
- fall back to v1 for unsupported instructions while preserving exact behavior

## Core Architectural Choice

V2 will have a bytecode-like IR and interpreter runtime.

This is the most radical Go-only route that still preserves behavior:

- more radical than "more generated fast paths"
- more compatible than rewriting everything into generated Go code only
- more realistic than requiring all schemas to be compile-time known

The IR exists so both generated and dynamic schemas can share the same execution model.

## High-Level Structure

`DataSourceV2` will live in the same Go package as the current datasource, but as a separate type and constructor:

- `NewDataSource` remains the current baseline engine
- `NewDataSourceV2` constructs the new engine

Proposed file groups:

- `grpc_datasource_v2.go`
- `grpc_datasource_v2_ir.go`
- `grpc_datasource_v2_compile.go`
- `grpc_datasource_v2_schema.go`
- `grpc_datasource_v2_runtime.go`
- `grpc_datasource_v2_request.go`
- `grpc_datasource_v2_response.go`
- `grpc_datasource_v2_fallback.go`
- `grpc_datasource_v2_test.go`
- `grpc_datasource_v2_bench_test.go`

## Runtime Components

### 1. Planner Bridge

Input:

- `RPCExecutionPlan`
- compiled proto document
- mapping

Output:

- `v2Program`

The planner bridge is temporary but important. It lets v2 reuse current planning correctness while replacing execution.

### 2. Schema Runtime

The schema runtime compiles descriptors into stable tables for both generated and dynamic schemas.

Key structures:

- message table
- field table
- method table
- wire/decode metadata
- generated type handles when available
- dynamic access programs when generated types are unavailable

The goal is to move from name-driven runtime lookups to integer-indexed runtime access.

### 3. Request IR

Each fetch gets a request program:

- load variable slot
- load static literal
- load dependency field
- begin message
- set scalar field
- set enum field
- append repeated field
- begin/end oneof branch
- branch nullability

This is not codegen into Go source. It is a compact executable program.

### 4. Response IR

Each fetch gets a response program:

- decode root result field
- iterate repeated result items
- project scalar/message field
- emit into response slot
- attach to root path
- merge entity payload

The key design rule is that v2 should move toward direct final-response writes, not subtree-then-merge as the main architecture.

### 5. Execution Kernel

The kernel owns:

- compiled stages
- shard-local memory
- request scratch
- output slots
- temporary value vectors

The kernel should be oblivious to mapping semantics at runtime. It just executes IR.

### 6. Compatibility Fallback

Behavior preservation is non-negotiable.

So v2 must support:

- per-fetch fallback to v1 execution for unsupported IR
- optional whole-operation fallback when mixed-mode would be incorrect
- correctness-first routing until each feature is ported

Fallback is not failure. It is part of the design.

## Dynamic Schemas From Day One

V2 must not treat dynamic schemas as second-class.

That means:

- compile descriptor-backed message layouts into schema tables at datasource construction time
- execute request/response programs against those schema tables
- use generated type handles opportunistically, but never require them
- keep `dynamicpb` as a compatibility implementation detail only where the new runtime has not replaced behavior yet

Day-one support does not mean day-one peak performance for every dynamic path. It means the architecture and API surface support them natively.

## Compatibility Contract

`DataSourceV2` must preserve all current behavior:

- existing mapping semantics
- existing federation behavior
- resolver behavior
- aliases
- nullable and optional handling
- oneofs
- list wrappers
- enum mappings
- entity ordering
- required-field behavior

If any of those are not supported by a v2 fetch program, that fetch must route through fallback automatically.

## Rollout Strategy

Phase 1:

- revert the v1 package to baseline
- preserve all findings in docs
- introduce `DataSourceV2`
- compile IR and schema tables
- fallback to v1 broadly
- add direct v1 vs v2 tests and benchmarks

Phase 2:

- port standard fetch request build to IR runtime
- port standard response projection to IR runtime
- prove dynamic-schema path correctness

Phase 3:

- port resolve fetches
- port entity fetches
- port required-field fetches
- reduce fallback surface

Phase 4:

- introduce direct final-response writing
- eliminate generic subtree merge for supported shapes

## Comparison Strategy

We need explicit side-by-side comparisons:

- correctness tests: v1 output equals v2 output
- behavior tests: v2 fallback triggers where expected
- benchmarks:
  - `Benchmark_DataSource_Load`
  - `Benchmark_DataSource_Load_WithFieldArguments`
  - v1 vs v2 variants on identical operations

## Recommended First Breakthrough

The first real architectural milestone is not “make v2 faster than v1 everywhere”.

It is:

1. compile a genuine IR from day one
2. support dynamic schemas in that IR compiler
3. keep exact behavior through fallback
4. establish a stable side-by-side benchmark harness

Once that exists, the next breakthroughs can happen inside v2 without destabilizing v1.
