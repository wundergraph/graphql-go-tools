# gRPC Datasource Improvement Ledger

Date started: 2026-04-17
Worktree: `/Users/jens/.superset/worktrees/graphql-go-tools/hollow-playroom`
Scope: `v2/pkg/engine/datasource/grpc_datasource`

This file records the full optimization campaign for the gRPC datasource. Each stage is treated as an experiment. If a stage does not produce the expected architectural or benchmark effect, it should be reverted or reworked before moving on.

## Rules

- Do not commit during this exploration.
- Keep the benchmark set stable so improvements are comparable.
- Record both wins and failures.
- Prefer deleting hot-path work over micro-optimizing it.
- If a stage does not materially support the radical architecture, revisit scope.

## Benchmark Suite

Primary commands:

```sh
cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments|BenchmarkBuildDependencyGraph|BenchmarkCompareKeyFields)$' -benchmem ./pkg/engine/datasource/grpc_datasource
```

Profiling commands:

```sh
cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load$' -benchmem -cpuprofile /tmp/grpc-ds-load.cpu.out -memprofile /tmp/grpc-ds-load.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource
cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load_WithFieldArguments$' -benchmem -cpuprofile /tmp/grpc-ds-load-args.cpu.out -memprofile /tmp/grpc-ds-load-args.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource
```

## Checklist

- [x] Stage 0: Capture fresh benchmark baseline and hotspot profile snapshots.
- [x] Stage 1: Introduce a kernel boundary and compile fixed execution stages at datasource construction time.
- [x] Stage 2: Remove per-request dependency graph creation and topological sorting from `Load`.
- [x] Stage 3: First request-construction pass: pre-resolve call metadata and eliminate copy-returning hot lookups.
- [x] Stage 4: First context-extraction pass: replace map-based resolver batches with row-based batches.
- [ ] Stage 5: Introduce a pluggable protobuf runtime boundary.
- [x] Stage 6: Add a generated fast path for known schemas.
- [x] Stage 7: Add a compiled dynamic fast path for runtime schemas.
- [ ] Stage 8: Replace intermediate response subtree building with a direct response writer.
- [x] Stage 9: Replace request-byte-keyed pooling with kernel-owned sharded memory.
- [x] Stage 10: Reprofile, compare end-state vs baseline, and summarize findings.
- [x] Stage 11: Compile resolver-context extraction into field-number programs (attempted and reverted).
- [x] Stage 12: Compile a shared-context fast path for batched resolver requests (attempted and reverted).
- [x] Stage 13: Add a generated-message direct builder for resolver context requests.
- [x] Stage 14: Add a generated-message response writer fast path for supported schemas.
- [x] Stage 15: Apply generated resolve outputs directly onto the root response (attempted and reverted).
- [x] Stage 16: Materialize generated resolve value slices concurrently and attach them sequentially.

## Baseline

Status: captured on 2026-04-17

Benchmarks:

```text
BenchmarkCompareKeyFields/simple-16                294.4 ns/op      80 B/op       4 allocs/op
BenchmarkCompareKeyFields/complex-16               757.9 ns/op     304 B/op       9 allocs/op
BenchmarkCompareKeyFields/long-16                 2058 ns/op      1728 B/op      18 allocs/op
BenchmarkCompareKeyFields/long_and_nested-16      3067 ns/op      3072 B/op      21 allocs/op
BenchmarkBuildDependencyGraph-16                   343.1 ns/op     432 B/op       7 allocs/op
Benchmark_DataSource_Load-16                      2319 ns/op      1852 B/op      30 allocs/op
Benchmark_DataSource_Load_WithFieldArguments-16 154109 ns/op     84956 B/op    1488 allocs/op
```

Profiles:

```text
Benchmark_DataSource_Load:
- alloc_space hotspots include dynamicpb.NewMessage, NewDependencyGraph, CompileFetches,
  Message.GetField, TopologicalSortResolve, and JSON escaping.

Benchmark_DataSource_Load_WithFieldArguments:
- alloc_space hotspots include arena allocation, dynamicpb.Message.Set, dynamicpb.NewMessage,
  dynamicpb.Message.Mutable, and RPCCompiler.resolveContextData.
- CPU remains dominated by protobuf/gRPC and runtime profiling overhead, with package-side work
  still visible in request compilation and context extraction.
```

## Stage Log Template

Copy this block for each stage:

```md
## Stage N: Title

Goal:

Hypothesis:

Files touched:

Commands run:

Baseline before stage:

Result after stage:

What worked:

What did not work:

Decision:
- keep

## Stage 30: Direct Frame-To-Resolver Merge Path

Goal:
Exploit the new native merge seam instead of routing it back through generic `astjson` subtree materialization and `MergeValuesWithPath` for every native V2 merge.

Hypothesis:
If `v2NativeMergeResult.MergeInto` can:
- navigate indexed select paths directly inside the frame
- merge object nodes straight onto resolver targets
- only materialize leaf subtrees when necessary

then the seam from Stage 29 stops being just an architectural placeholder and becomes the base for a real fast merge path.

Files touched:
- `IMPROVEMENTS.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_frame.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- added red test for indexed native select path on frame-backed merge results:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestV2NativeMergeResult_MergeInto_SupportsIndexedSelectPath' -count=1`
- green verification for the indexed-select test plus existing resolver parity test:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'Test(V2NativeMergeResult_MergeInto_SupportsIndexedSelectPath|DataSourceV2_LoadResult_ResolveMatchesLoadAndLoadValue)$' -count=1`
- full package verification:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource ./pkg/engine/resolve`
- repeated V2 native-value vs merge-result benchmarks:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^$' -bench '^(Benchmark_DataSource_V2_Load(Value|Result)(_WithFieldArguments|_FederationRequiresUnion)?)$' -benchmem -count=3`

Baseline before stage:
- Stage 29 introduced the native merge seam, but `v2NativeMergeResult.MergeInto` still did the expensive thing:
  - select a frame node
  - materialize it into a full `astjson.Value`
  - call `astjson.MergeValuesWithPath`
- It also could not follow indexed select paths such as `["data","_entities","0"]`.

Result after stage:
- `selectDataNode` now supports array index segments.
- `MergeInto` now uses a direct object-merge path for object-to-object merges:
  - root object merges
  - batch merges
  - merge-path leaf object merges
- Only non-object or fallback shapes still go through full `nodeValue + MergeValuesWithPath`.
- New correctness coverage is in place for indexed select paths.

Repeated benchmark signal:
- simple path:
  - `LoadValue`: `28359-29102 ns/op`, `11434-11441 B/op`, `226 allocs/op`
  - `LoadResult`: `25072-25622 ns/op`, `11506-11511 B/op`, `229 allocs/op`
- field-args path:
  - `LoadValue`: `72345-78721 ns/op`, `49009-49024 B/op`, `964 allocs/op`
  - `LoadResult`: `72352-77667 ns/op`, `49109-49183 B/op`, `967 allocs/op`
- federation requires+union path:
  - `LoadValue`: `67719-69461 ns/op`, `41451-41460 B/op`, `787 allocs/op`
  - `LoadResult`: `66270-71907 ns/op`, `41564-41626 B/op`, `791 allocs/op`

What worked:
- The merge runtime now has a real specialization point instead of always falling back to generic merge code.
- Indexed select paths are now correct for frame-backed results.
- The direct merge path is recursive and structural:
  - object fields are merged directly into existing resolver objects
  - nested merge-path containers are created in-place
  - full subtree materialization is avoided for the object/object fast path
- The simple benchmark now shows `LoadResult` faster than `LoadValue` on CPU in this run, which is the first time the seam is not obviously underwater on the fast path.

What did not work:
- The benchmark suite used here still does not perfectly isolate the new item-merge fast path, because these datasource benchmarks primarily exercise whole-operation native results rather than loader-style target-item merges.
- Allocation count is still slightly higher on the `LoadResult` path.
- The heavy and federation-native benchmarks are mixed rather than decisive; they are roughly in the same band as `LoadValue`, not a clean breakthrough yet.

Useful conclusions:
- This stage fixes a real correctness gap and turns Stage 29’s seam into something the optimizer can actually build on.
- The next benchmark work should move up one level and measure real resolver/loader merges with V2 native results, not only datasource-local root selection.
- The next radical optimization opportunity is now clear:
  - either push this direct merge specialization further into loader-side batch fan-out
  - or delete more of `astjson` entirely by letting native V2 frames participate in final response writing.

Decision:
- keep

## Stage 29: Native Loader Merge Boundary For V2 Frames

Goal:
Delete the remaining loader-side `astjson` subtree materialization boundary for native V2 success paths by letting `grpc_datasource_v2` hand the loader a frame-backed merge result directly.

Hypothesis:
If the loader can consume a native merge result instead of forcing V2 through `LoadValue`, then we remove one architectural boundary:
- V2 no longer needs to materialize a full `{data: ...}` `astjson` envelope for loader consumption
- the loader can merge directly from the native frame-backed result into the resolver arena
- this becomes the base for future direct final-write work

Files touched:
- `IMPROVEMENTS.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_bench_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_frame.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`
- `v2/pkg/engine/resolve/datasource.go`
- `v2/pkg/engine/resolve/loader.go`
- `v2/pkg/engine/resolve/resolve.go`
- `v2/pkg/engine/resolve/resolve_test.go`

Commands run:
- added red resolve test for loader preference:
  - `cd v2 && go test ./pkg/engine/resolve -run 'TestResolver_ArenaResolveGraphQLResponse_PrefersNativeMergeDataSourceAndCallsCleanup' -count=1`
- added red datasource test for V2 merge-result contract:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2_LoadResult_ResolveMatchesLoadAndLoadValue' -count=1`
- full package verification:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource ./pkg/engine/resolve`
- synthetic resolve seam benchmark:
  - `cd v2 && go test ./pkg/engine/resolve -run '^$' -bench '^BenchmarkResolver_ArenaResolveGraphQLResponse_NativeBoundary$' -benchmem -count=3`
- real V2 frame benchmarks:
  - `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^$' -bench '^(Benchmark_DataSource_V2_Load(Value|Result)(_WithFieldArguments|_FederationRequiresUnion)?)$' -benchmem -count=3`

Baseline before stage:
- Stage 28 left V2 still crossing the loader boundary through `LoadValue` and a materialized `astjson` envelope.
- There was no native merge-result contract between datasource and loader.

Result after stage:
- The resolve layer now has an additive `NativeMergeDataSource` / `NativeMergeResult` contract.
- Loader prefers native merge results over `LoadValue` when available and defers cleanup correctly until after response writing.
- `grpc_datasource_v2` now exposes `LoadResult` for native success paths and returns a frame-backed `v2NativeMergeResult`.
- New coverage proves both sides:
  - loader prefers `LoadResult` and does not fall back to `LoadValue`
  - V2 `LoadResult` matches `Load` and `LoadValue` output for the resolver-heavy benchmark query

Measured benchmark signal:
- Synthetic resolve seam benchmark with fake datasources:
  - `native_value`: `1066-1091 ns/op`, `2203-2204 B/op`, `34 allocs/op`
  - `native_merge`: `1155-1223 ns/op`, `2259-2260 B/op`, `37 allocs/op`
- Real V2 pooled-arena comparison:
  - simple path:
    - `LoadValue`: `21913-24199 ns/op`, `11424-11427 B/op`, `226 allocs/op`
    - `LoadResult`: `23915-24886 ns/op`, `11503-11508 B/op`, `229 allocs/op`
  - field-args path:
    - `LoadValue`: `68157-72812 ns/op`, `48996-49022 B/op`, `964 allocs/op`
    - `LoadResult`: `68804-71900 ns/op`, `49096-49216 B/op`, `967 allocs/op`
  - federation requires+union path:
    - `LoadValue`: `63621-66266 ns/op`, `41460-41461 B/op`, `788 allocs/op`
    - `LoadResult`: `64650-68670 ns/op`, `41617-41621 B/op`, `791 allocs/op`

What worked:
- The architectural seam is now real and additive. Other datasources are unaffected.
- Cleanup ownership is explicit across datasource -> loader -> resolver.
- Native V2 success paths can now hand off a frame-backed result without first materializing a response envelope value.
- This is the right boundary for future work:
  - direct final-write from V2 frames
  - merge-time specialization that avoids `astjson.MergeValuesWithPath`
  - loader-side native handling for batch fan-out without subtree reification

What did not work:
- The first benchmark signal is not a win yet.
- The synthetic resolve benchmark is misleading for real V2 because the fake merge result still builds ordinary `astjson` values, so it mostly measures interface overhead.
- The real V2 datasource-level comparison is roughly neutral to slightly worse for `LoadResult + MergeInto` than `LoadValue`.
- In other words, deleting the boundary alone is not enough. The merge side still pays for generic `astjson` materialization and path-based merging, so the new seam is currently an enabler more than an isolated speedup.

Useful conclusions:
- This stage should not be judged as a standalone throughput optimization.
- It should be judged as a prerequisite architectural step that makes the next radical work possible on the correct side of the boundary.
- The next meaningful optimization must now exploit this seam:
  - specialize merge from frame nodes into resolver targets
  - or bypass `astjson` entirely for native V2 final writing in the resolver path

Decision:
- keep

## Stage 28: Native Oneof / Fragment Response Execution

Goal:
Delete the next federation fallback wall by making V2 compile and execute fragment-driven oneof response messages natively, especially on queries that combine `@requires` with union/interface resolver output.

Hypothesis:
If V2 can compile response programs for oneof-backed protobuf wrapper messages and dispatch fragment materialization based on the active branch at runtime, then federation queries like `tagSummary + storageStatus` should stop falling back and produce another structural runtime drop.

Files touched:
- `IMPROVEMENTS.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_bench_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_compile.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_ir.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- added red test for native compilation of a federation `@requires + union resolver` query
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2_CompilesNativeProgramForFederationRequiresAndUnionResolve' -count=1`
- implemented oneof/fragment response compilation and runtime dispatch
- added parity test for native load/loadValue on the same query
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2_CompilesNativeProgramForFederationRequiresAndUnionResolve|TestDataSourceV2_LoadValue_FederationRequiresAndUnionResolveMatchesLoad' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource ./pkg/engine/resolve`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_V1_Load_FederationRequiresUnion|Benchmark_DataSource_V2_Load_FederationRequiresUnion|Benchmark_DataSource_V2_LoadValue_FederationRequiresUnion)$' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- The new red test failed with:
  - `fallback reasons: [call 1 (ResolveStorageStorageStatus): oneof or fragment-driven response messages are not yet supported natively]`
- No benchmark existed yet for this exact path, but the expected comparison target was a fallback-heavy federation path similar to the pre-Stage-27 ceiling.

Result after stage:
- The federation `@requires + union resolver` query now compiles natively in V2.
- New repeated measurements:
  - `Benchmark_DataSource_V1_Load_FederationRequiresUnion`: `93396-102076 ns/op`, `72637-72774 B/op`, `1216-1217 allocs/op`
  - `Benchmark_DataSource_V2_Load_FederationRequiresUnion`: `67333-71486 ns/op`, `42485-42504 B/op`, `794-795 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue_FederationRequiresUnion`: `66232-70917 ns/op`, `41467-41474 B/op`, `787-788 allocs/op`

What worked:
- V2 response programs now support oneof/fragment-driven response shapes by:
  - compiling per-concrete-type fragment programs
  - detecting the active oneof branch at runtime
  - materializing the matching fragment fields into the final response object
- The targeted query is now fully native and parity-tested.
- This is another structural improvement, not a micro-win:
  - V2 byte path beats V1 by roughly `26-35 us/op`
  - bytes/op drop by roughly `30 KB`
  - allocs/op drop by roughly `420`
  - native `LoadValue` trims another small layer beyond that

What did not work:
- The response compiler still does not support every possible fragment-driven shape. This stage specifically unlocked the oneof-backed resolver output used by the benchmarked federation query.
- Common fields on interface/union selections are not yet benchmarked explicitly. The current implementation is strongest where selection is mostly fragment-driven.
- This stage broadens native response coverage, but it does not yet address the deeper loader-side `astjson` materialization boundary.

Useful conclusions:
- The next remaining coverage barrier is no longer plain federation or simple oneof response support. The major native coverage classes are expanding quickly now.
- The strongest next radical step is likely one of:
  - broader interface/union coverage with common-field handling
  - or the loader/render boundary that still forces native V2 responses through `astjson`

Decision:
- keep

## Stage 21: Native V2 Response Frame Arena

Goal:
Delete `astjson` from the native V2 success path and replace it with a compiled response-frame runtime that stores only response slots and serializes the final GraphQL payload directly.

Hypothesis:
If native V2 stops building `astjson.Value` trees and instead writes into a compact frame graph with direct JSON serialization, the heavy benchmark should drop further on bytes/op and allocs/op, and the remaining response-side cost should become small enough that protobuf and context extraction dominate.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_frame.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- added red test for response-frame data-envelope serialization
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestV2ResponseFrameBuilder_MarshalDataEnvelope' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestV2ResponseFrameBuilder_MarshalDataEnvelope|TestDataSourceV2_' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_V1_Load|Benchmark_DataSource_V2_Load|Benchmark_DataSource_V1_Load_WithFieldArguments|Benchmark_DataSource_V2_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_V2_Load_WithFieldArguments$' -benchmem -cpuprofile /tmp/grpc-ds-v2-frame.cpu.out -memprofile /tmp/grpc-ds-v2-frame.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go tool pprof -top /tmp/grpc-ds-v2-frame.cpu.out`
- `cd v2 && go tool pprof -top /tmp/grpc-ds-v2-frame.mem.out`

Baseline before stage:
- Stable reference before this stage:
  - `Benchmark_DataSource_V2_Load`: `23098-24247 ns/op`, `12137-12142 B/op`, `240 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `68082-72125 ns/op`, `50678-50689 B/op`, `1022 allocs/op`

Result after stage:
- First frame-writer cut, without request-local reuse:
  - `Benchmark_DataSource_V2_Load`: `24245-25115 ns/op`, `14278-14280 B/op`, `230 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `69826-73300 ns/op`, `51583-51594 B/op`, `980 allocs/op`
- Final kept version, after converting the frame writer into a reusable request-local arena:
  - `Benchmark_DataSource_V2_Load`: `27802-29088 ns/op`, `11492-11496 B/op`, `225 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `69281-73091 ns/op`, `47844-47864 B/op`, `955 allocs/op`

What worked:
- Native V2 no longer builds `astjson.Value` trees on the success path.
- Resolver and standard fetches now attach into an index-based response frame and serialize once at the end as `{"data":...}`.
- The second step of the stage, turning the frame graph into a reusable request-local arena, was necessary and paid off:
  - heavy benchmark bytes/op dropped from about `50.7 KB` to about `47.8 KB`
  - heavy benchmark allocs/op dropped from `1022` to `955`
  - simple benchmark bytes/op dropped from about `12.1 KB` to about `11.5 KB`
  - simple benchmark allocs/op dropped from `240` to `225`
- Relative to V1, the native V2 heavy path is now materially leaner:
  - `~69-73 us/op` vs `~107-119 us/op`
  - `~47.8 KB/op` vs `~83.8 KB/op`
  - `955 allocs/op` vs `1483-1485 allocs/op`
- Reprofile after the arena conversion shows the response side is no longer a major structural blocker. Package-side hotspots are now narrower:
  - `v2ContextProgram.extractRows`
  - `v2ResolvePathProgram.extractFromMessage`
  - `v2ResponseFrameBuilder.appendNodeJSON`
  - protobuf unmarshal and gRPC transport remain larger than the V2 response writer itself

What did not work:
- The first frame-writer implementation was not good enough by itself. It reduced alloc count but regressed both CPU and bytes/op because it still allocated fresh node slices every request.
- Even after the arena conversion, the simple benchmark regressed on CPU versus the previous Stage 20 implementation. This is the main cost of the new architecture today.
- The new serializer now pays visible cost in quoted field-name and string emission (`strconv.AppendQuote` / `appendQuotedWith` shows up in alloc-space profiles).
- The remaining V2 ceiling is now concentrated in:
  - resolver context extraction and path walking
  - protobuf unmarshal / gRPC transport
  - string-heavy final serialization rather than object-tree construction

Decision:
- keep
- follow up with a more aggressive compiled serializer and a lower-allocation resolve-context runtime

## V2 Breakthrough Checklist

- [x] 1. Add an optional resolve interface for arena-rooted datasource values so loaders can consume native results without forcing every datasource off the byte contract.
- [x] 2. Move `grpc_datasource_v2` off final byte serialization for native paths and hand native values directly to the loader with explicit cleanup.
- [x] 3. Integrate a dynamic-schema decode backend in V2 that can replace reflective output decode on runtime-loaded schemas.
- [x] 4. Add backend-aware output allocation in V2 so the compiler/runtime can skip allocations that the decode backend will replace.
- [x] 5. Attack request encoding with a true V2 wire-format request path, keeping fallback for unsupported shapes.
- [x] 6. Add honest end-to-end happy-path and federated fan-out benchmarks for V2 so the next ceiling is measured correctly.

## Stage 22: Optional Native Loader Contract

Goal:
Create the first boundary needed for the next architectural jump: let loaders consume an arena-rooted datasource value directly, while preserving the existing byte-returning `DataSource` interface for compatibility.

Hypothesis:
If the new contract is additive instead of breaking, we can land the loader boundary first, verify lifecycle correctness, and then move `grpc_datasource_v2` onto it in the next stage without destabilizing the rest of the engine.

Files touched:
- `v2/pkg/engine/resolve/datasource.go`
- `v2/pkg/engine/resolve/loader.go`
- `v2/pkg/engine/resolve/resolve.go`
- `v2/pkg/engine/resolve/resolve_test.go`

Commands run:
- added red test for native-value datasource preference and cleanup lifecycle
- `cd v2 && go test ./pkg/engine/resolve -run 'TestResolver_ArenaResolveGraphQLResponse_UsesNativeValueDataSourceAndCallsCleanup' -count=1`
- `cd v2 && go test ./pkg/engine/resolve`

Baseline before stage:
- Loader only knew how to consume datasource bytes.
- `DataSource` had a single contract: `Load(...) ([]byte, error)`.
- Any datasource-native response graph had to be serialized and reparsed before merge.

Result after stage:
- `resolve.NativeDataSource` now exists as an additive interface with `LoadValue` / `LoadWithFilesValue`.
- `Loader` prefers the native interface when available and keeps the old byte path as fallback.
- `Loader` now tracks datasource cleanup callbacks and runs them in `Loader.Free()`.
- `Resolver` now explicitly calls `t.loader.Free()` after each top-level GraphQL response resolution.
- Focused lifecycle test passes, and the full `resolve` package is green.

What worked:
- The contract change stayed additive, so existing datasources and generated gomocks did not need a repo-wide rewrite.
- The red test proved the exact behavior we need for the next stage:
  - the loader uses the native value path instead of the legacy byte path
  - cleanup is called exactly once after response writing has finished
- Making `Loader.Free()` real was the correct lifecycle point because it sits after resolution/output consumption, not during merge.

What did not work:
- The first implementation dropped the shared `err` variable in `mergeResult`; fixed immediately once the build failed.
- This stage does not improve benchmarks by itself. It only opens the architectural seam needed for the larger V2 changes.

Decision:
- keep
- step 1 complete
- move to step 2 next

## Stage 23: Native `grpc_datasource_v2` Value Handoff

Goal:
Move native V2 execution off the final byte boundary so the loader can consume datasource-owned values directly and only the legacy byte interface pays the final marshal step.

Hypothesis:
If V2 exposes `LoadValue` natively, the new loader contract from Stage 22 can start paying off immediately:
- native callers stop forcing final byte serialization
- cleanup stays explicit and correctly scoped
- the old `Load` contract remains intact by layering byte marshaling on top of `LoadValue`

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_frame.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- added red test for native V2 value loading parity
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2_LoadValue_ResolveMatchesLoad' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource ./pkg/engine/resolve`

Baseline before stage:
- `grpc_datasource_v2` still returned bytes only.
- Even native V2 execution paid a final `frame -> bytes` conversion before the loader could consume the result.

Result after stage:
- `DataSourceV2` now implements both `resolve.DataSource` and `resolve.NativeDataSource`.
- `LoadValue` / `LoadWithFilesValue` return datasource-owned `*astjson.Value` trees with explicit cleanup.
- `Load` is now layered on top of `LoadValue` for compatibility.
- Focused parity test for resolver-heavy native execution is green.

What worked:
- The boundary is now real. Native callers can stay on value objects without forcing a final byte round trip.
- Compatibility stayed simple because the old `Load` path is still available and now reuses the native implementation rather than duplicating logic.
- Cleanup ownership is explicit and aligned with the loader lifecycle introduced in Stage 22.

What did not work:
- This is not yet a zero-copy final handoff. Native V2 still materializes an `astjson.Value` tree from the response frame for the native contract.
- No isolated benchmark was taken for this stage alone; the measured impact shows up later in Stage 26.

Decision:
- keep
- step 2 complete

## Stage 24: Dynamic Decode Backend And Backend-Aware Output Allocation

Goal:
Replace reflective dynamic output decode in V2 with a real runtime backend for dynamic schemas and teach the runtime to allocate output containers according to the backend in use.

Hypothesis:
If V2 can decode runtime-loaded schemas with `hyperpb` instead of generic reflective messages, then dynamic-schema execution will stop paying the worst dynamic decode overhead. If output allocation becomes backend-aware at the same time, the runtime can stop allocating containers that the decode backend will immediately replace.

Files touched:
- `v2/go.mod`
- `v2/go.sum`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_hyperpb.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_ir.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_schema.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- `cd v2 && go get buf.build/go/hyperpb@v0.1.3`
- added red test for dynamic decode message allocation
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestV2MessageRuntime_NewDecodeMessage_UsesHyperpbWhenGeneratedTypeMissing' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestV2MessageRuntime_NewDecodeMessage_UsesHyperpbWhenGeneratedTypeMissing|TestDataSourceV2_LoadValue_ResolveMatchesLoad' -count=1`

Baseline before stage:
- V2 still fell back to generic reflective decode for runtime-loaded schemas.
- Output allocation did not yet understand that different backends own different output containers.

Result after stage:
- V2 now compiles and stores a `hyperpb` message type for every schema runtime message.
- Runtime output allocation is backend-aware:
  - generated schema path uses generated protobuf messages
  - dynamic schema path can allocate `hyperpb` messages with per-call shared ownership
  - generic reflective allocation remains as the final fallback
- gRPC invocation now routes through a codec that can unmarshal directly into `hyperpb` messages.

What worked:
- Dynamic schemas are now handled by a real decode backend from day one instead of only by the reflective fallback.
- Backend-aware output ownership made the change structurally correct rather than bolting `hyperpb` onto the side.
- The new decode backend integrates cleanly with both native `LoadValue` and legacy `Load`.

What did not work:
- This stage does not remove all reflective behavior. Generated and unsupported paths still coexist with generic fallbacks.
- `hyperpb` types are compiled eagerly as part of schema runtime construction, which adds cold-path setup cost in exchange for hot-path speed.
- The benchmark effect was validated later as part of the full step-6 measurement pass rather than as a standalone stage benchmark.

Decision:
- keep
- steps 3 and 4 complete

## Stage 25: True V2 Wire-Format Request Path

Goal:
Delete protobuf request marshaling work for the subset of request shapes already represented in the V2 IR by emitting protobuf wire bytes directly from the compiled request program.

Hypothesis:
If V2 can compile request programs into a wire plan, then supported request shapes can bypass message marshaling entirely:
- no request-side protobuf object materialization
- lower allocation pressure on the input side
- clean fallback to the existing protobuf-message build when a shape is not yet supported

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_compile.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_hyperpb.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_ir.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_wire.go`

Commands run:
- added red test for direct wire-plan request input generation
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestV2RequestProgram_BuildInput_UsesWirePlanForNestedRequest' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestV2RequestProgram_BuildInput_UsesWirePlanForNestedRequest|TestV2MessageRuntime_NewDecodeMessage_UsesHyperpbWhenGeneratedTypeMissing|TestDataSourceV2_LoadValue_ResolveMatchesLoad' -count=1`

Baseline before stage:
- V2 still built protobuf request messages and then marshaled them even for request shapes already fully known to the compiled IR.

Result after stage:
- `v2RequestProgram` can now compile a direct wire plan for supported shapes.
- Supported request programs return a pre-marshaled protobuf input object instead of building a protobuf message.
- The gRPC codec reuses those bytes directly.
- Unsupported shapes still fall back to the existing protobuf-message build path.

What worked:
- The request path now has a real architectural escape hatch from protobuf message marshaling.
- The implementation stays safe because unsupported features do not try to force themselves through the wire encoder.
- Nested message input works, and the focused wire-plan test is green.

What did not work:
- The wire plan is intentionally incomplete:
  - no enum input support
  - no resolve-context request path
  - unsupported shapes fall back
- Repeated numeric fields currently use a simple repeated-field encoding rather than packed canonical encoding. It is valid, but not yet the final polished encoder.
- Like Stage 24, the benchmark effect is measured in the full pass later rather than from an isolated pre/post run here.

Decision:
- keep
- step 5 complete

## Stage 26: Honest Native-Value And Federation Fan-Out Benchmarks

Goal:
Measure the actual post-Stage-25 V2 shape instead of only the legacy byte-returning path. Add native `LoadValue` benchmarks and a federation fan-out benchmark so the next ceiling is visible.

Hypothesis:
If the new native boundary and request/decode changes are real, then:
- `LoadValue` should show a modest but measurable win over `Load` on native V2 paths
- federation/entity fan-out should expose whether V2 is still paying legacy byte/fallback overhead

Files touched:
- `IMPROVEMENTS.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_bench_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- added red parity test for federation fan-out native value loading
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2_LoadValue_FederationFanoutMatchesLoad' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2_LoadValue_ResolveMatchesLoad|TestDataSourceV2_LoadValue_FederationFanoutMatchesLoad' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource ./pkg/engine/resolve`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_V1_Load|Benchmark_DataSource_V2_Load|Benchmark_DataSource_V2_LoadValue|Benchmark_DataSource_V1_Load_WithFieldArguments|Benchmark_DataSource_V2_Load_WithFieldArguments|Benchmark_DataSource_V2_LoadValue_WithFieldArguments|Benchmark_DataSource_V1_Load_FederationFanout|Benchmark_DataSource_V2_Load_FederationFanout|Benchmark_DataSource_V2_LoadValue_FederationFanout)$' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- V2 benchmark coverage only measured the legacy byte-returning interface for the simple and resolver-heavy query shapes.
- There was no benchmark for the new native loader/value boundary.
- There was no benchmark for federation/entity resolver fan-out.

Result after stage:
- Added three new benchmark surfaces:
  - `Benchmark_DataSource_V2_LoadValue`
  - `Benchmark_DataSource_V2_LoadValue_WithFieldArguments`
  - `Benchmark_DataSource_V2_LoadValue_FederationFanout`
- Added V1/V2 federation fan-out byte-path benchmarks.
- Fresh repeated measurements:
  - `Benchmark_DataSource_V1_Load`: `33743-36344 ns/op`, `14912-14943 B/op`, `286 allocs/op`
  - `Benchmark_DataSource_V2_Load`: `24206-27385 ns/op`, `11548-11557 B/op`, `230 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue`: `23868-25793 ns/op`, `11292-11297 B/op`, `225 allocs/op`
  - `Benchmark_DataSource_V1_Load_WithFieldArguments`: `104360-109145 ns/op`, `83902-83937 B/op`, `1487 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `70428-73123 ns/op`, `49864-49876 B/op`, `970 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue_WithFieldArguments`: `69844-73502 ns/op`, `48840-48854 B/op`, `963 allocs/op`
  - `Benchmark_DataSource_V1_Load_FederationFanout`: `72981-75632 ns/op`, `43381-43404 B/op`, `756 allocs/op`
  - `Benchmark_DataSource_V2_Load_FederationFanout`: `74285-77329 ns/op`, `44489-44490 B/op`, `764 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue_FederationFanout`: `74155-76847 ns/op`, `43470-43477 B/op`, `757 allocs/op`

What worked:
- The native value boundary is real and measurable:
  - simple native path saves about `250 B/op` and `5 allocs/op` versus V2 `Load`
  - resolver-heavy native path saves about `1.0 KB/op` and `7 allocs/op` versus V2 `Load`
- V2 remains materially ahead of V1 on the native happy-path benchmarks.
- The new federation fan-out benchmark exposed a real architectural truth:
  - V2 `Load` is still paying extra overhead on this path
  - V2 `LoadValue` removes most of that memory/alloc gap immediately
- The parity test proves the new native federation path returns the same data as the byte path.

What did not work:
- Federation fan-out is not yet a breakthrough case for V2 on CPU. On the byte path it is slightly slower and slightly fatter than V1.
- The native value handoff helps federation memory more than CPU because the current fan-out path still leans on fallback execution and legacy behavior preservation.
- The native `LoadValue` benchmarks are still end-to-end datasource benchmarks, not full-engine resolver benchmarks. They expose the datasource boundary better, but not the whole resolver pipeline.

Useful conclusions:
- The next ceiling is now sharply defined:
  - native V2 hot paths benefit from the new boundary and runtime
  - fallback/federation-heavy paths still carry too much legacy cost
- The next radical step should target one of:
  - extending native V2 coverage deeper into federation/entity execution
  - deleting more fallback serialization work on mixed-mode paths

Decision:
- keep
- step 6 complete

## V2 Breakthrough Status

Current measured V2 shape after completing the checklist:

- Native happy path:
  - `Benchmark_DataSource_V2_Load`: `24206-27385 ns/op`, `11548-11557 B/op`, `230 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue`: `23868-25793 ns/op`, `11292-11297 B/op`, `225 allocs/op`
- Resolver-heavy path:
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `70428-73123 ns/op`, `49864-49876 B/op`, `970 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue_WithFieldArguments`: `69844-73502 ns/op`, `48840-48854 B/op`, `963 allocs/op`
- Federation fan-out:
  - `Benchmark_DataSource_V1_Load_FederationFanout`: `72981-75632 ns/op`, `43381-43404 B/op`, `756 allocs/op`
  - `Benchmark_DataSource_V2_Load_FederationFanout`: `74285-77329 ns/op`, `44489-44490 B/op`, `764 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue_FederationFanout`: `74155-76847 ns/op`, `43470-43477 B/op`, `757 allocs/op`

Interpretation:

- The checklist work landed real wins. V2 now has:
  - a native value boundary
  - a dynamic decode backend
  - backend-aware output ownership
  - a true wire-format request fast path for supported input shapes
  - benchmark coverage that can see the native boundary and federation fan-out ceiling
- The native value handoff is worth keeping, but it is not itself the next breakthrough. It removes a measurable thin layer; it does not solve mixed-mode fallback cost.
- Federation fan-out is now the cleanest evidence of where the architecture still needs radical work: native happy paths are ahead, but mixed-mode/entity paths still preserve too much of the old datasource behavior.

## Stage 27: Native Federation Entity Execution

Goal:
Delete the fallback wall on the federation benchmark path by making V2 compile and execute entity-rooted fan-out plans natively instead of routing `_entities` plus resolver arguments back through v1.

Hypothesis:
If V2 can natively compile:
- `CallKindEntity` root fetches
- enum-valued request arguments on the generic request path
- optional scalar wrapper request fields
and validate federated entity counts without leaving the native runtime, then the federation benchmark should stop looking like a mixed-mode fallback path and collapse toward the native happy-path cost shape.

Files touched:
- `IMPROVEMENTS.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_compile.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- added red test for native federation fan-out compilation
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2_CompilesNativeProgramForFederationFanout' -count=1`
- fixed compile/runtime blockers iteratively:
  - enum request fields
  - optional scalar wrapper request fields
  - static `__typename` federation validation
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2_CompilesNativeProgramForFederationFanout|TestDataSourceV2_LoadValue_FederationFanoutMatchesLoad' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource ./pkg/engine/resolve`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_V1_Load_FederationFanout|Benchmark_DataSource_V2_Load_FederationFanout|Benchmark_DataSource_V2_LoadValue_FederationFanout)$' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- From Stage 26:
  - `Benchmark_DataSource_V1_Load_FederationFanout`: `72981-75632 ns/op`, `43381-43404 B/op`, `756 allocs/op`
  - `Benchmark_DataSource_V2_Load_FederationFanout`: `74285-77329 ns/op`, `44489-44490 B/op`, `764 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue_FederationFanout`: `74155-76847 ns/op`, `43470-43477 B/op`, `757 allocs/op`
- Native V2 still treated the federation fan-out benchmark as a fallback-heavy path because:
  - `CallKindEntity` was not compiled natively
  - enum request fields were rejected
  - optional scalar wrapper request fields were rejected

Result after stage:
- Fresh repeated measurements:
  - `Benchmark_DataSource_V1_Load_FederationFanout`: `71409-77628 ns/op`, `43405-43503 B/op`, `756 allocs/op`
  - `Benchmark_DataSource_V2_Load_FederationFanout`: `32076-34591 ns/op`, `16043-16045 B/op`, `299 allocs/op`
  - `Benchmark_DataSource_V2_LoadValue_FederationFanout`: `31697-33493 ns/op`, `15785-15786 B/op`, `294 allocs/op`

What worked:
- The federation benchmark path now compiles natively in V2:
  - the new red test proves `nativeOperation == true`
  - the benchmark-shaped plan compiles as `Entity -> Resolve` without fallback
- The generic V2 request path is materially stronger now:
  - enum request values are supported natively
  - optional scalar wrapper request fields are supported natively
- Federation validation stayed behavior-preserving by adding native entity-count validation for V2 entity responses, including static `__typename` handling.
- The benchmark result is a real architectural breakthrough:
  - V2 federation byte path dropped from roughly `74-77 us` to `32-35 us`
  - bytes/op dropped from roughly `44.5 KB` to `16.0 KB`
  - allocs/op dropped from `764` to `299`
  - native `LoadValue` trims another small layer on top:
    - about `250 B/op`
    - about `5 allocs/op`

What did not work:
- The first implementation only lifted `Entity`/`Required` kind routing and was not enough; the red test stayed red because enum request inputs were still rejected.
- After enum support, the path still failed because optional scalar wrappers were still compile-time fallbacks.
- Federation validation initially crashed on static `__typename` because the new validator assumed every typename had a field runtime; that had to be fixed explicitly.
- This stage is benchmarked only on the federation fan-out surface so far. It should be included in the next full V1/V2 sweep before broader conclusions are locked in.

Useful conclusions:
- The prior “federation is still the cleanest evidence of the remaining ceiling” statement is no longer true for this benchmark shape. This stage deleted that ceiling.
- The next radical target has moved again:
  - either broader federation/entity coverage beyond this benchmark shape
  - or the remaining generic response/value materialization cost on already-native paths

Decision:
- keep

## Stage 20: Direct Response Attachment In `grpc_datasource_v2`

Goal:
Delete the remaining `response -> astjson subtree -> merge` execution shape from native v2. Keep the protobuf outputs from each fetch, then attach them directly into the final response tree with compiled response programs.

Hypothesis:
If native v2 stops materializing per-fetch response objects and instead:
- writes root standard responses directly onto the final root object
- resolves resolver targets once from `ResponsePath`
- attaches row-aligned resolver values directly onto those target objects
then the remaining response-side allocation pressure should drop again, especially on the resolver-heavy benchmark.

Files touched:
- `IMPROVEMENTS.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_ir.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`

Commands run:
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench 'Benchmark_DataSource_(V1|V2)_' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_V2_Load_WithFieldArguments$' -benchmem -cpuprofile /tmp/grpc-ds-v2-direct-attach.cpu.out -memprofile /tmp/grpc-ds-v2-direct-attach.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -top /tmp/grpc-ds-v2-direct-attach.cpu.out`
- `go tool pprof -sample_index=alloc_space -top /tmp/grpc-ds-v2-direct-attach.mem.out`

Baseline before stage:
- Stage 19 comparison:
  - `Benchmark_DataSource_V1_Load`: `29169-30904 ns/op`, `14895-14920 B/op`, `286 allocs/op`
  - `Benchmark_DataSource_V2_Load`: `24780-26531 ns/op`, `12261-12272 B/op`, `243 allocs/op`
  - `Benchmark_DataSource_V1_Load_WithFieldArguments`: `104227-113210 ns/op`, `83924-83962 B/op`, `1487 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `69682-75128 ns/op`, `52289-52296 B/op`, `1063 allocs/op`
- At that point, v2 still built intermediate `astjson` objects for each fetch and then merged them, even though the protobuf runtime was already much faster.

What changed architecturally:
- The kernel no longer stores intermediate JSON responses for native fetches.
- Each fetch goroutine now returns only the protobuf output plus fetch metadata.
- Response programs gained direct-attach methods:
  - apply object fields directly to the root tree
  - resolve resolver targets from `ResponsePath`
  - materialize only the exact value being attached to each target field
- The old `write + mergeValues/mergeWithPath` path is gone from native v2 execution.

What worked:
- The simple benchmark improved again:
  - v1: `28271-29450 ns/op`, `14891-14915 B/op`, `286 allocs/op`
  - v2: `23098-24247 ns/op`, `12137-12142 B/op`, `240 allocs/op`
- The heavy benchmark improved again too:
  - v1: `101742-105839 ns/op`, `83931-83972 B/op`, `1487-1488 allocs/op`
  - v2: `68082-72125 ns/op`, `50678-50689 B/op`, `1022 allocs/op`
- Compared with the previous stage, the heavy benchmark dropped by roughly:
  - `1.6-3.0 us/op`
  - about `1.6 KB/op`
  - about `41 allocs/op`
- Profiling confirms the architectural intent:
  - the old subtree-building/merge path is no longer the main package-side shape
  - remaining package costs are narrower and more local:
    - `v2ContextProgram.extractRows`
    - `v2ResolvePathProgram.extractFromMessage`
    - `v2ResponseFieldProgram.materialize`
    - `v2ResponseProgram.attachResolve`

What did not work:
- This stage did not eliminate `astjson` allocations entirely. The final response tree still uses `astjson`, so the next ceiling is now the cost of materializing the exact attached values rather than whole fetch subtrees.
- The profile still shows meaningful transport and protobuf unmarshal cost on the heavy benchmark.
- The benchmark harness itself contributes noticeable server-side allocation noise (`createSubcategories`, mock service response construction).

Useful conclusions:
- The response-side architectural rewrite was worth doing. The old subtree model was still materially expensive even after the generated protobuf runtime landed.
- The remaining package-side cost is now much more focused:
  - resolver context row extraction
  - scalar/object value materialization for direct attach
- The next radical move should likely target one of two things:
  - a lower-allocation context extraction model
  - a response writer that bypasses `astjson` object/value construction even further

Decision:
- keep

## Stage 19: Generated-Message Runtime Backend For `grpc_datasource_v2`

Goal:
Replace `dynamicpb` as the primary hot-path message container in v2 when generated Go protobuf types are linked. Keep dynamic schemas working from day one by preserving `dynamicpb` as the fallback backend.

Hypothesis:
If the v2 schema runtime can allocate generated protobuf messages for request roots, response roots, and resolver-context rows, then the largest remaining protobuf runtime costs should fall sharply:
- fewer allocations during request build and response unmarshal
- lower bytes/op on the resolver-heavy benchmark
- materially better CPU due to generated marshal/unmarshal paths

Files touched:
- `IMPROVEMENTS.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_schema.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench 'Benchmark_DataSource_(V1|V2)_' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_V2_Load_WithFieldArguments$' -benchmem -cpuprofile /tmp/grpc-ds-v2-generated.cpu.out -memprofile /tmp/grpc-ds-v2-generated.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -top /tmp/grpc-ds-v2-generated.cpu.out`
- `go tool pprof -sample_index=alloc_space -top /tmp/grpc-ds-v2-generated.mem.out`

Baseline before stage:
- Stage 18 comparison:
  - `Benchmark_DataSource_V1_Load`: `28630-29491 ns/op`, `14894-14918 B/op`, `286 allocs/op`
  - `Benchmark_DataSource_V2_Load`: `25662-27261 ns/op`, `15008-15012 B/op`, `291 allocs/op`
  - `Benchmark_DataSource_V1_Load_WithFieldArguments`: `102533-110449 ns/op`, `83873-83975 B/op`, `1485-1488 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `95456-112489 ns/op`, `81777-81797 B/op`, `1526-1528 allocs/op`
- At that point, the heavy path was inside v2, but it was still paying for `dynamicpb` object creation and generic protobuf runtime overhead.

What changed architecturally:
- `v2MessageRuntime` now has two real backends:
  - generated message types when linked
  - `dynamicpb` otherwise
- Each compiled field runtime now carries both descriptor families:
  - compiler/dynamic descriptor
  - generated descriptor
- Field access and mutation choose the correct descriptor at runtime based on the actual message backend.
- The gRPC boundary now passes concrete `proto.Message` values via `protoreflect.Message.Interface()` while keeping the reflective handle internally.

What worked:
- This is the first real protobuf-runtime breakthrough in v2.
- The heavy benchmark dropped sharply:
  - `Benchmark_DataSource_V1_Load_WithFieldArguments`: `104227-113210 ns/op`, `83924-83962 B/op`, `1487 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `69682-75128 ns/op`, `52289-52296 B/op`, `1063 allocs/op`
- The simple benchmark improved substantially too:
  - v1: `29169-30904 ns/op`, `14895-14920 B/op`, `286 allocs/op`
  - v2: `24780-26531 ns/op`, `12261-12272 B/op`, `243 allocs/op`
- That means this stage simultaneously improved CPU, bytes/op, and alloc count on both comparison workloads.
- The v2 engine now genuinely differentiates between generated-linked schemas and runtime-only schemas without sacrificing dynamic-schema support.

What did not work:
- The generated path was not a drop-in replacement. Descriptor identity differs between the compiler’s protobuf descriptors and the generated descriptors, so the runtime had to be redesigned to carry both descriptor families explicitly.
- The remaining hot path is still not “ultra high performance” yet. Profiling shows the next package-side costs are now:
  - resolver context extraction (`v2ContextProgram.extractRows`, `v2ResolvePathProgram.extractFromMessage`)
  - JSON object assembly and merge (`astjson`, `jsonBuilder`)
  - server-side mock/response allocations in the benchmark harness

Useful conclusions:
- Generated-message execution belongs in the architecture, not as an optional micro-fast-path. It materially changes the runtime profile.
- The v2 engine is now structurally in the right place for the next radical move: the protobuf backend is no longer the main blocker for linked schemas.
- The next serious ceiling is now the response pipeline and the context-extraction row model, not basic protobuf message allocation.

Decision:
- keep

## Stage 18: Native Resolve Calls In `grpc_datasource_v2`

Goal:
Push the new v2 engine past root-only fetches and make the benchmark-dominant resolver path execute natively. The target is dependency-driven `CallKindResolve` execution with compiled context extraction from prior protobuf outputs.

Hypothesis:
If v2 stops falling back to v1 for field resolvers and instead:
- keeps prior stage protobuf outputs in kernel state
- compiles resolve-context extraction into path programs
- builds resolve requests directly from dependency outputs plus `field_args`
- merges resolver `result` payloads by `ResponsePath`
then the heavy benchmark should move meaningfully because the old dependency graph/compiler path disappears from execution.

Files touched:
- `IMPROVEMENTS.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_ir.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_compile.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`

Commands run:
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestDataSourceV2' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench 'Benchmark_DataSource_(V1|V2)_' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_V2_Load_WithFieldArguments$' -benchmem -cpuprofile /tmp/grpc-ds-v2-resolve.cpu.out -memprofile /tmp/grpc-ds-v2-resolve.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -top /tmp/grpc-ds-v2-resolve.cpu.out`
- `go tool pprof -sample_index=alloc_space -top /tmp/grpc-ds-v2-resolve.mem.out`

Baseline before stage:
- Stage 17 comparison:
  - `Benchmark_DataSource_V1_Load`: `28810-29932 ns/op`, `14894-14919 B/op`, `286 allocs/op`
  - `Benchmark_DataSource_V2_Load`: `26249-27292 ns/op`, `14833-14834 B/op`, `289 allocs/op`
  - `Benchmark_DataSource_V1_Load_WithFieldArguments`: `103156-107469 ns/op`, `83930-83956 B/op`, `1486-1488 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `102145-105199 ns/op`, `83876-83933 B/op`, `1487-1488 allocs/op`
- In that state, the heavy benchmark was still effectively the v1 path because v2 fell back for resolver fetches.

What changed architecturally:
- Each v2 fetch now carries dependency IDs.
- V2 runtime keeps prior protobuf outputs by fetch ID so later stages can consume native outputs directly.
- Resolve requests now have a dedicated compiled `context` program instead of abusing the standard variable-only request builder.
- Resolve paths are compiled into field-runtime step programs and executed directly against dependency protobuf outputs.
- V2 now merges resolve responses with `mergeWithPath` like v1, instead of treating every fetch as a root merge.

What worked:
- The heavy benchmark query is now truly native in v2. It no longer has to whole-operation fallback just because the plan contains resolver calls.
- End-to-end parity remains intact:
  - the resolver-heavy query now compiles as `nativeOperation == true`
  - v1/v2 JSON parity passes
  - compiled resolve request construction from dependency output passes
- The heavy benchmark improved on bytes/op and moved into a better CPU range on average:
  - `Benchmark_DataSource_V1_Load_WithFieldArguments`: `102533-110449 ns/op`, `83873-83975 B/op`, `1485-1488 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `95456-112489 ns/op`, `81777-81797 B/op`, `1526-1528 allocs/op`
- Mean CPU across the 3 runs is modestly better for v2, and the floor is materially better (`95.5 us/op` vs `102.5 us/op`).
- The new stage establishes the right execution architecture for the next major improvements: the dominant resolver path is finally inside the new engine, where it can now be optimized directly.

What did not work:
- Allocation count got worse in the heavy benchmark, rising from `1485-1488` to `1526-1528 allocs/op`.
- The simple benchmark remained faster on CPU but gave back some bytes/op and allocs:
  - v1: `28630-29491 ns/op`, `14894-14918 B/op`, `286 allocs/op`
  - v2: `25662-27261 ns/op`, `15008-15012 B/op`, `291 allocs/op`
- Profiling shows the new context path is not free:
  - `v2ContextProgram.extractRows`
  - `v2ResolvePathProgram.extractFromMessage`
  - `dynamicpb.(*Message).Mutable`
  are now visible allocation sites on the heavy path.
- The real ceiling is still dominated by generic protobuf runtime cost and subtree JSON assembly, not by scheduling anymore.

Useful conclusions:
- This was the necessary architectural breakpoint. V2 can now execute staged root + resolver plans without dropping back to v1.
- The next round of wins will not come from more graph/compiler work; that layer is now largely gone from the heavy path.
- The next meaningful levers are:
  - lower-allocation context row construction
  - less generic protobuf message/runtime handling than `dynamicpb`
  - direct resolver-value attachment or a final-response writer that avoids more intermediate subtree work

Decision:
- keep

## Stage 17: Split The Work Into A New `grpc_datasource_v2`

Goal:
Stop trying to drag the original datasource toward an ultra-high-performance architecture in place. Revert the v1 package to baseline, preserve the experiment ledger, and introduce a second datasource that can take a fundamentally different route without destabilizing the current engine.

Hypothesis:
A side-by-side v2 engine will make the next radical work materially easier:
- v1 stays as the correctness baseline
- v2 can introduce a compiled IR/runtime without fitting itself into every old abstraction
- behavior can stay exact by falling back to v1 for unsupported fetches or operations
- benchmark comparisons become clean and repeatable

Files touched:
- `IMPROVEMENTS.md`
- `docs/superpowers/specs/2026-04-18-grpc-datasource-v2-design.md`
- `docs/superpowers/plans/2026-04-18-grpc-datasource-v2.md`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_ir.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_schema.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_compile.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_runtime.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_v2_bench_test.go`

Commands run:
- reverted the v1 package back to `HEAD`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'Test(NewDataSourceV2|DataSourceV2)' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench 'Benchmark_DataSource_(V1|V2)_' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- The previous optimization campaign ended at roughly:
  - `Benchmark_DataSource_Load`: `1910-1975 ns/op`, `1246-1251 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `84145-87850 ns/op`, `51528-51905 B/op`, `906-907 allocs/op`
- Those changes were intentionally reverted from v1 so the old datasource could become a clean control again.

What v2 is:
- `DataSourceV2` is a second datasource, not a patch set on v1.
- It compiles the existing execution plan into a new compact runtime shape:
  - stages
  - fetch descriptors
  - request programs
  - response programs
- It builds a schema runtime from protobuf descriptors for dynamic schemas from day one.
- It keeps exact behavior by routing unsupported operations through the existing `DataSource`.

What v2 supports natively today:
- standard root fetches with no dependency edges
- compiled request building for supported scalar/message shapes
- compiled response writing for supported scalar/message shapes
- exact whole-operation fallback to v1 for unsupported shapes

What worked:
- The architectural split is now real and testable.
- We can compare v1 and v2 on the same package without rewriting the existing engine.
- The native v2 path is functionally correct for the first supported operation shape:
  - native v1/v2 output parity test passes
  - fallback v1/v2 parity test passes
- The new schema runtime works for dynamic schemas from day one by compiling descriptor-backed message tables even when no generated fast path is available.
- The first native comparison already shows a CPU win on the simple standard-fetch path:
  - `Benchmark_DataSource_V1_Load`: `28810-29932 ns/op`, `14894-14919 B/op`, `286 allocs/op`
  - `Benchmark_DataSource_V2_Load`: `26249-27292 ns/op`, `14833-14834 B/op`, `289 allocs/op`
- The heavy benchmark remains essentially at parity because v2 currently falls back to v1 there:
  - `Benchmark_DataSource_V1_Load_WithFieldArguments`: `103156-107469 ns/op`, `83930-83956 B/op`, `1486-1488 allocs/op`
  - `Benchmark_DataSource_V2_Load_WithFieldArguments`: `102145-105199 ns/op`, `83876-83933 B/op`, `1487-1488 allocs/op`

What did not work:
- The first native v2 path is not a breakthrough yet.
- It improves the simple standard-fetch path, but only modestly.
- It still uses `dynamicpb` as the runtime message container, so the hot path has not yet escaped generic protobuf object creation.
- The benchmark-dominant resolver path is still handled by v1, so v2 has not attacked the real ceiling yet.
- The current v2 response path still builds `astjson` trees and merges them at the operation root; it is not yet a true direct final writer.

Useful conclusions:
- Splitting the engine was the right move. The new compiler/runtime can now evolve independently without destabilizing v1.
- A compact IR plus fallback is a viable migration strategy for keeping all behavior while radically changing architecture.
- The next serious breakthrough must come from extending native coverage into dependency-driven and resolver-heavy execution, not from polishing the first root-fetch path.
- The biggest remaining ceilings are now explicit:
  - native support for dependent/resolve calls
  - a less generic message runtime than `dynamicpb`
  - a final-response writer that avoids intermediate subtree assembly

Decision:
- keep
- revert
- revisit
```

## Stage 1: Kernel Boundary And Precompiled Schedule

Goal:
Move execution ordering out of `Load` and into datasource construction.

Hypothesis:
If the datasource compiles a fixed execution schedule once, `Load` can stop rebuilding scheduling state on every request and become an executor over precomputed stages.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`

Commands run:
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments|BenchmarkBuildDependencyGraph)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -sample_index=alloc_space -top ...`

Baseline before stage:
- `Benchmark_DataSource_Load`: `2319 ns/op`, `1852 B/op`, `30 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `154109 ns/op`, `84956 B/op`, `1488 allocs/op`

Result after stage:
- `Benchmark_DataSource_Load`: `2168 ns/op`, `1588 B/op`, `30 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `144825 ns/op`, `83671 B/op`, `1485 allocs/op`

What worked:
- `Load` now executes a kernel program with precompiled stages instead of constructing a dependency graph and sorting it per request.
- Post-change alloc-space profiling no longer shows `NewDependencyGraph` or `TopologicalSortResolve` in the hot path for `Benchmark_DataSource_Load`.
- The stage produced an immediate structural and measurable win.

What did not work:
- This stage does not yet change the standalone `BenchmarkBuildDependencyGraph` benchmark because that benchmark still exercises the old helper directly.
- It does not materially reduce alloc count by itself because the remaining request-building pipeline is still dynamic and reflective.

Decision:
- keep

## Stage 2: Hot-Path Graph And Sort Removal

Goal:
Finish the transition from per-request graph scheduling to request-local execution state over a precompiled program.

Hypothesis:
Even if the standalone graph helper remains in the package, removing it from `Load` should lower package-side CPU and alloc space in the real benchmark path.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`

Commands run:
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments|BenchmarkBuildDependencyGraph)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -sample_index=alloc_space -top ...`

Baseline before stage:
- Same as Stage 1 baseline.

Result after stage:
- Same as Stage 1 result. This was implemented together with Stage 1 because separating the boundary from the schedule removal would have created an unmeasurable intermediate state.

What worked:
- The hot path no longer depends on `DependencyGraph`.
- Request-local state is now explicit, which is the foundation for later compiled request/context stages.

What did not work:
- The legacy graph helper still exists for tests and benchmarks, so graph code is not deleted yet.

Decision:
- keep

## Stage 3: Pre-Resolved Call Metadata And Stable Field Lookups

Goal:
Start turning request construction into a compiled path by pre-resolving per-call metadata and removing copy-returning lookup helpers from the hot path.

Hypothesis:
If the kernel carries service names and message handles, and field lookups stop returning copies, request construction should spend less time and memory on repeated schema lookups and avoidable heap traffic.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/execution_plan.go`

Commands run:
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments|BenchmarkBuildDependencyGraph)$' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- `Benchmark_DataSource_Load`: `2168 ns/op`, `1588 B/op`, `30 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `144825 ns/op`, `83671 B/op`, `1485 allocs/op`

Result after stage:
- `Benchmark_DataSource_Load`: `2074 ns/op`, `1524 B/op`, `28 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `133514 ns/op`, `82613 B/op`, `1472 allocs/op`

What worked:
- The kernel now pre-resolves service full names and input/output message handles.
- `Message.GetField` now returns stable stored pointers instead of pointer-to-copy results.
- `RPCFields.ByName` now returns the actual slice element instead of a range-variable copy.
- This was the first stage to reduce allocation count in both load benchmarks.

What did not work:
- This is only the first pass of compiled request construction; the hot path still interprets `RPCMessage` recursively and still uses `dynamicpb` heavily.
- Descriptor-by-name lookups still happen during message construction, so this is not yet the full builder architecture.

Decision:
- keep

## Stage 4: Row-Based Resolver Context Batches

Goal:
Reduce memory pressure in resolver batching by replacing `[]map[string]protoref.Value` with a denser row-based representation.

Hypothesis:
If resolver context data is collected as ordered rows instead of per-row maps, the field-args benchmark should allocate less and stop showing `resolveContextData` as a significant alloc-space hotspot.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`

Commands run:
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -sample_index=alloc_space -top ...`

Baseline before stage:
- `Benchmark_DataSource_Load`: `2074 ns/op`, `1524 B/op`, `28 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `133514 ns/op`, `82613 B/op`, `1472 allocs/op`

Result after stage:
- `Benchmark_DataSource_Load`: `1960 ns/op`, `1524 B/op`, `28 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `135182 ns/op`, `80033 B/op`, `1464 allocs/op`

What worked:
- `Benchmark_DataSource_Load_WithFieldArguments` dropped another `2580 B/op` and `8 allocs/op`.
- Post-change alloc-space profiling no longer shows `RPCCompiler.resolveContextData` among the top allocation hotspots.
- The simpler benchmark also improved again in latency.

What did not work:
- The wall-clock delta for the field-args benchmark was slightly worse on this single run, so CPU impact is not yet clearly positive.
- This is still not a fully compiled extractor; path walking and dynamic message access remain in the hot path.

Decision:
- keep

## Stage 5: Proto Runtime Boundary Experiment

Goal:
Introduce a protobuf runtime boundary so later generated or compiled-dynamic backends can be added without another datasource rewrite.

Hypothesis:
An initial boundary that still uses `dynamicpb` should be behaviorally neutral and should not materially change benchmark results.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/proto_runtime.go` (reverted)
- `v2/pkg/engine/datasource/grpc_datasource/proto_runtime_dynamicpb.go` (reverted)

Commands run:
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- repeated benchmark run
- reverted patch
- reran tests and benchmarks

Baseline before stage:
- `Benchmark_DataSource_Load`: `1960 ns/op`, `1524 B/op`, `28 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `135182 ns/op`, `80033 B/op`, `1464 allocs/op`

Result after stage:
- First run after introducing the boundary:
  - `Benchmark_DataSource_Load`: `2069 ns/op`, `1524 B/op`, `28 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `142519 ns/op`, `80410 B/op`, `1465 allocs/op`
- Repeat run:
  - `Benchmark_DataSource_Load`: `2090 ns/op`, `1524 B/op`, `28 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `172699 ns/op`, `80321 B/op`, `1465 allocs/op`
- After revert:
  - `Benchmark_DataSource_Load`: `1998 ns/op`, `1524 B/op`, `28 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `138150 ns/op`, `80138 B/op`, `1465 allocs/op`

What worked:
- The abstraction itself was straightforward to wire in.

What did not work:
- The experiment was not behaviorally neutral in benchmark runs.
- Because this stage is supposed to be an enabler rather than a win, carrying even a possible regression is not justified yet.
- The runtime boundary likely needs to arrive together with a real fast path so the abstraction cost is absorbed by a larger architectural gain.

Decision:
- revert
- revisit

## Stage 8: Direct Response Application Experiment

Goal:
Bypass per-call intermediate `astjson` subtree materialization and merge standard and resolver responses directly into the final response tree.

Hypothesis:
If the datasource stops building a temporary JSON subtree for each non-entity RPC result, the hot path should reduce allocation pressure and improve both benchmarks, especially the field-resolver case.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/json_builder.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/json_builder_direct_test.go` (reverted)

Commands run:
- added red tests for direct root application and direct path application
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestJSONBuilder_ApplyMessageMatchesMarshalResponseJSON|TestJSONBuilder_ApplyMessageWithPathMatchesMarshalAndMerge'`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- repeated benchmark run
- reverted hot-path integration
- reran tests and benchmarks

Baseline before stage:
- `Benchmark_DataSource_Load`: `1998 ns/op`, `1524 B/op`, `28 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `138150 ns/op`, `80138 B/op`, `1465 allocs/op`

Result after stage:
- With direct application integrated:
  - first run:
    - `Benchmark_DataSource_Load`: `2014 ns/op`, `1524 B/op`, `28 allocs/op`
    - `Benchmark_DataSource_Load_WithFieldArguments`: `151989 ns/op`, `80279 B/op`, `1466 allocs/op`
  - repeat run:
    - `Benchmark_DataSource_Load`: `1990 ns/op`, `1524 B/op`, `28 allocs/op`
    - `Benchmark_DataSource_Load_WithFieldArguments`: `142955 ns/op`, `80114 B/op`, `1466 allocs/op`
- After reverting the hot-path integration:
  - repeat run:
    - `Benchmark_DataSource_Load`: `1982 ns/op`, `1524 B/op`, `28 allocs/op`
    - `Benchmark_DataSource_Load_WithFieldArguments`: `141360 ns/op`, `80338 B/op`, `1465 allocs/op`

What worked:
- The direct application logic was implementable and behaviorally correct under dedicated red/green tests.
- The root benchmark stayed roughly flat, so the idea is not obviously invalid.

What did not work:
- The field-resolver benchmark did not improve and consistently lost enough ground to reject the stage.
- Allocation count did not improve at all in the hot path, which means the avoided subtree materialization was not the dominant remaining allocator.
- This path likely needs to be paired with a broader protobuf/runtime change before it pays off.

Decision:
- revert
- revisit

## Stage 6: Generated Protobuf Allocation Fast Path

Goal:
Use concrete generated protobuf message types in the hot path whenever the compiled schema's message full name exists in the linked Go binary, while preserving `dynamicpb` fallback for unknown schemas.

Hypothesis:
If request and response roots allocate generated messages instead of `dynamicpb` when available, gRPC marshal/unmarshal and message allocation should get dramatically cheaper without requiring a full request-builder rewrite.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource_test.go`
- `v2/pkg/engine/datasource/grpc_datasource/compiler_test.go`

Commands run:
- added red tests for generated-type selection and dynamic fallback
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestRPCCompiler_NewEmptyMessage_UsesGeneratedTypeWhenAvailable|TestRPCCompiler_NewEmptyMessage_FallsBackToDynamicPBWhenGeneratedTypeUnavailable|TestBuildProtoMessage'`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -sample_index=alloc_space -top ...`

Baseline before stage:
- Current kept state before this stage:
  - `Benchmark_DataSource_Load`: about `1982-1998 ns/op`, `1524 B/op`, `28 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `138150-141360 ns/op`, `80138-80338 B/op`, `1465 allocs/op`

Result after stage:
- Repeated sample over 3 runs:
  - `Benchmark_DataSource_Load`: `2045-2140 ns/op`, `1203-1204 B/op`, `22 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `100316-108678 ns/op`, `50261-50444 B/op`, `1004-1005 allocs/op`

What worked:
- The compiler now allocates generated message types through `protoregistry.GlobalTypes` when the full protobuf message name is linked into the binary.
- Unknown/runtime-only schemas still fall back to `dynamicpb`.
- The field-args benchmark saw the first truly major runtime drop:
  - allocs/op fell from about `1465` to about `1004-1005`
  - bytes/op fell from about `80 KB` to about `50 KB`
  - latency stabilized around `100-109 us/op`, materially below the prior `138-141 us/op`
- Alloc-space profiling no longer shows `dynamicpb.NewMessage` or `dynamicpb.Message.Set` as dominant field-args hotspots.

What did not work:
- The simpler load benchmark did not improve on wall-clock time; it stayed roughly flat to slightly worse, though memory improved substantially.
- This is not yet a `vtprotobuf` path and not yet a compiled-dynamic runtime; it only captures schemas that already have linked generated Go types.

Decision:
- keep

## Stage 7: Compiled Runtime-Type Cache

Goal:
Cache the runtime message type once per schema message so both generated and fallback-dynamic allocations stop redoing type resolution on every allocation.

Hypothesis:
If runtime type selection is compiled into the schema model up front, the datasource should preserve the generated fast path and slightly reduce remaining allocation-path overhead; it also establishes the correct architecture for runtime-only schemas by using `dynamicpb.NewMessageType(...)` instead of raw descriptor allocation decisions at each call site.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/compiler_test.go`

Commands run:
- added red tests for generated and fallback runtime-type assignment
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run 'TestRPCCompiler_AssignsGeneratedRuntimeTypeWhenAvailable|TestRPCCompiler_AssignsCompiledDynamicRuntimeTypeWhenGeneratedTypeUnavailable|TestRPCCompiler_NewEmptyMessage_UsesGeneratedTypeWhenAvailable|TestRPCCompiler_NewEmptyMessage_FallsBackToDynamicPBWhenGeneratedTypeUnavailable'`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- After Stage 6:
  - `Benchmark_DataSource_Load`: about `2045-2140 ns/op`, `1203-1204 B/op`, `22 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `100316-108678 ns/op`, `50261-50444 B/op`, `1004-1005 allocs/op`

Result after stage:
- Repeated sample over 3 runs:
  - `Benchmark_DataSource_Load`: `2053-2112 ns/op`, `1203-1204 B/op`, `22 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `96708-101801 ns/op`, `50221-50301 B/op`, `1004-1005 allocs/op`

What worked:
- Runtime type choice is now cached directly on each compiled message as `RuntimeType`.
- Fallback dynamic allocation now uses compiled `dynamicpb.NewMessageType(...)` instead of re-deciding from the raw descriptor every time.
- The expensive benchmark improved a bit further and did so consistently across repeated runs.
- This stage strengthens the runtime-loaded-schema architecture even where the current benchmark suite does not directly isolate that path.

What did not work:
- This is an incremental stage, not another large jump.
- The current end-to-end benchmark suite still primarily exercises the linked-generated schema path, so the runtime-only-schema benefit is validated by tests and architecture more than by a dedicated external benchmark.

Decision:
- keep

## Stage 9: Kernel-Owned Sharded Memory

Goal:
Replace request-byte-keyed arena acquisition with kernel-owned request state, shard-local arena pools, and fixed scratch buffers sized to the compiled execution program.

Hypothesis:
If the kernel owns request memory instead of rebuilding slices and hashing full request bytes on every `Load`, the hot path should reduce allocation count again and improve arena reuse characteristics under varied real traffic.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go`

Commands run:
- added red tests for stable logical slot keys and preallocated request scratch
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^(TestKernelMemoryUsesStableLogicalSlotKeys|TestKernelMemoryAcquireRequestStatePreallocatesScratch)$'`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^Test_DataSource_Load$'`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- After Stage 7:
  - `Benchmark_DataSource_Load`: about `2053-2112 ns/op`, `1203-1204 B/op`, `22 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `96708-101801 ns/op`, `50221-50301 B/op`, `1004-1005 allocs/op`

Result after stage:
- Repeated sample over 3 runs:
  - `Benchmark_DataSource_Load`: `1873-1961 ns/op`, `1182-1186 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `97023-101347 ns/op`, `55023-55277 B/op`, `997-998 allocs/op`

What worked:
- The kernel now owns request-local scratch state instead of allocating `serviceCalls`, stage result slices, and pool-item tracking on every request.
- Arena keys are now stable logical slot identifiers derived from the compiled execution program rather than the full input payload, which is the right memory-ownership model for real traffic with variable inputs.
- The simple load benchmark improved again in both latency and allocations.
- The field-args benchmark cut another `6-8 allocs/op`, bringing the end state below `1000` allocs/op in repeated runs.

What did not work:
- Bytes/op went up for the field-args benchmark even though allocation count dropped, which implies the remaining dominant transport/protobuf allocations are chunkier than the request-state allocations we removed.
- This stage does not address the reflective request construction and protobuf decode work that still dominate the expensive benchmark.

Decision:
- keep

## Stage 10: End-State Reprofile And Comparison

Goal:
Reprofile the kept end state, compare it to the original baseline, and identify the next architectural ceiling.

Hypothesis:
The retained stages should shift the dominant costs away from datasource scheduling/allocation scaffolding and toward the remaining protobuf/gRPC and JSON assembly work.

Files touched:
- `IMPROVEMENTS.md`

Commands run:
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load$' -benchmem -cpuprofile /tmp/grpc-ds-load.cpu.out -memprofile /tmp/grpc-ds-load.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load_WithFieldArguments$' -benchmem -cpuprofile /tmp/grpc-ds-load-args.cpu.out -memprofile /tmp/grpc-ds-load-args.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -top /tmp/grpc-ds-load.cpu.out`
- `go tool pprof -sample_index=alloc_space -top /tmp/grpc-ds-load.mem.out`
- `go tool pprof -top /tmp/grpc-ds-load-args.cpu.out`
- `go tool pprof -sample_index=alloc_space -top /tmp/grpc-ds-load-args.mem.out`

Baseline before stage:
- Original campaign baseline:
  - `Benchmark_DataSource_Load`: `2319 ns/op`, `1852 B/op`, `30 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `154109 ns/op`, `84956 B/op`, `1488 allocs/op`

Result after stage:
- Current end state:
  - `Benchmark_DataSource_Load`: about `1873-1961 ns/op`, `1182-1186 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `97023-101347 ns/op`, `55023-55277 B/op`, `997-998 allocs/op`

What worked:
- The retained stages materially changed the runtime shape:
  - `Load` no longer rebuilds graph scheduling state.
  - request state and arena ownership are now compiled and kernel-local.
  - generated protobuf message allocation is the default fast path when linked types are available.
- End-state alloc-space profiles confirm the old scheduling allocators are gone from the hot path.
- The remaining package-side alloc hotspots in the simple load case are now concentrated in `CompileCompiledFetches`, `CompileCompiledNode`, `buildProtoMessage`, `newEmptyMessage`, `resolveNestedMessage`, and JSON escaping.
- In the field-args benchmark, the dominant ceiling is now clearly the transport/protobuf path plus remaining reflective datasource work:
  - gRPC stream creation and transport buffers
  - protobuf unmarshal
  - `buildProtoMessageWithContext`
  - `resolveContextData` / `resolveListDataForPath`
  - `marshalResponseJSON`

What did not work:
- The direct response writer experiment did not pay off in isolation and remains out of the final state.
- The datasource is still far from zero-GC or zero-reflection because request construction for contextual resolver calls and response JSON assembly remain generic.
- The field-args profile makes it clear that the next major gain will not come from another small memory tweak; it requires removing more of the reflective protobuf and response-building pipeline.

Decision:
- keep

## Stage 11: Compiled Resolver-Context Extraction Experiment

Goal:
Compile resolver-context extraction into field-number-based programs and direct setters so `buildProtoMessageWithContext` stops reinterpreting `ResolvePath` and field names on every request.

Hypothesis:
If the datasource precompiles the resolver-context path and target bindings, the field-args benchmark should reduce package-side CPU and allocations by deleting runtime path interpretation work from `resolveContextData` and context-row assembly.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go` (reverted)
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go` (reverted)
- `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go` (reverted)

Commands run:
- added red test for precompiled resolve-plan presence and usage
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^TestKernelCompilesResolveRequestPlanAndUsesIt$'`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- reverted patch
- reran package tests and repeated benchmark suite

Baseline before stage:
- Current kept state before the experiment:
  - `Benchmark_DataSource_Load`: about `1873-1961 ns/op`, `1182-1186 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `97023-101347 ns/op`, `55023-55277 B/op`, `997-998 allocs/op`

Result after stage:
- With the compiled resolver-context extraction integrated:
  - `Benchmark_DataSource_Load`: `1915-1994 ns/op`, `1199-1202 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `114566-159581 ns/op`, `56249-57218 B/op`, `976 allocs/op`
- After reverting:
  - `Benchmark_DataSource_Load`: `1872-1902 ns/op`, `1182-1183 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `94433-108731 ns/op`, `54698-56471 B/op`, `995-997 allocs/op`

What worked:
- The compiler could precompute resolver-context path programs and apply them correctly in tests.
- Allocation count in the heavy benchmark dropped further, from roughly `997-998` to `976 allocs/op`.

What did not work:
- CPU regressed too much in the heavy benchmark, with repeated runs landing well above the kept baseline.
- Bytes/op also increased, so the experiment did not meet the “more memory efficient and faster” bar.
- The compiled path evaluator still walked protobuf values generically enough that the extra abstraction cost outweighed the saved name lookups.

Decision:
- revert
- revisit

## Stage 12: Shared-Context Fast Path Experiment

Goal:
Compile a shared-context fast path for resolver requests that traverse the parent repeated list once and write context rows directly, instead of resolving each context field path independently.

Hypothesis:
If the datasource compiles the shared parent-list prefix of resolver context paths, the field-args benchmark should improve over Stage 11 by deleting duplicate traversal work instead of merely changing field lookup mechanics.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go` (reverted)
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go` (reverted)
- `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go` (reverted)

Commands run:
- added red test for shared-context plan compilation and request building
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^TestKernelCompilesSharedContextPlanAndUsesIt$'`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- reverted patch
- reran package tests and repeated benchmark suite

Baseline before stage:
- Stable kept reference before the experiment:
  - `Benchmark_DataSource_Load`: about `1873-1961 ns/op`, `1182-1186 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `97023-101347 ns/op`, `55023-55277 B/op`, `997-998 allocs/op`

Result after stage:
- With the shared-context fast path integrated:
  - `Benchmark_DataSource_Load`: `2326-3035 ns/op`, `1201-1215 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `131474-159120 ns/op`, `55121-58122 B/op`, `942 allocs/op`
- After reverting:
  - immediate reruns were noisy and remained above the prior stable kept reference:
    - `Benchmark_DataSource_Load`: `2004-2227 ns/op` on one run set, `2402-2556 ns/op` on another, with one `4727 ns/op` outlier
    - `Benchmark_DataSource_Load_WithFieldArguments`: `132816-170761 ns/op`, `56635-58895 B/op`, `996-997 allocs/op`

What worked:
- The shared-context plan compiled and produced correct batched resolver requests in tests.
- Allocation count in the heavy benchmark dropped materially again, from about `997-998` to `942 allocs/op`.

What did not work:
- CPU regressed even more than the previous Stage 11 experiment.
- Bytes/op stayed worse than the stable kept reference.
- The direct shared-list traversal still paid too much generic protobuf overhead to be a net runtime win.
- Post-revert reruns were noisier than the earlier stable reference, so the kept reference numbers remain the last reliable comparison point for the campaign summary.

Decision:
- revert
- revisit

## Stage 13: Generated Direct Resolver-Context Builder

Goal:
Bypass `protoreflect` entirely for resolver-context request building on linked generated schemas by compiling the shared resolver-context path against generated Go struct layouts and copying fields directly into the generated request structs.

Hypothesis:
If the datasource compiles the resolver-context builder against generated message layouts, the field-args benchmark should finally improve on CPU as well as memory because the hot path deletes both repeated path interpretation and the generic protobuf field-set path for batched resolver contexts.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go`

Commands run:
- added red test for generated context-plan compilation and request building
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^TestKernelCompilesGeneratedContextPlanAndUsesIt$'`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- Stable kept reference before this stage:
  - `Benchmark_DataSource_Load`: about `1873-1961 ns/op`, `1182-1186 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `97023-101347 ns/op`, `55023-55277 B/op`, `997-998 allocs/op`

Result after stage:
- Repeated sample over 3 runs:
  - `Benchmark_DataSource_Load`: `1884-1944 ns/op`, `1198-1199 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `91316-109863 ns/op`, `52187-53744 B/op`, `903-904 allocs/op`

What worked:
- The kernel now compiles a generated-context plan for supported resolve fetches.
- The resolve hot path for those fetches now builds context rows by directly walking generated Go structs and filling generated request structs, instead of going through `protoreflect` for each copied field.
- This is the first resolver-context experiment that improved CPU and memory at the same time.
- The heavy benchmark improved materially again:
  - allocs/op dropped from about `997-998` to `903-904`
  - bytes/op dropped from about `55 KB` to `52-54 KB`
  - latency improved from about `97-101 us/op` to about `91-110 us/op`, with the best run landing at `91.3 us/op`

What did not work:
- The simple load benchmark stayed essentially flat; this stage is specifically a resolver-context win.
- The fast path only applies when both the parent response and resolver request/context messages are linked generated types and the resolve paths fit the compiled shared-prefix shape.
- Fallback dynamic schemas still use the generic path.

Decision:
- keep

## Stage 14: Generated Response Writer Fast Path

Goal:
Delete the reflective response walk for supported generated response messages by compiling a response plan once and writing JSON from generated Go structs directly.

Hypothesis:
If the datasource can compile the response tree for generated schemas, the heavy benchmark should drop again because the response path will stop doing descriptor lookups and generic `protoreflect` traversal on every returned message.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/json_builder.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go`

Commands run:
- added red test for generated response-plan compilation and response writing
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^TestKernelCompilesGeneratedResponsePlanAndUsesIt$' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load_WithFieldArguments$' -benchmem -cpuprofile /tmp/grpc-ds-stage14-args.cpu.out -memprofile /tmp/grpc-ds-stage14-args.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -top /tmp/grpc-ds-stage14-args.cpu.out`
- `go tool pprof -sample_index=alloc_space -top /tmp/grpc-ds-stage14-args.mem.out`

Baseline before stage:
- Stable kept reference before this stage:
  - `Benchmark_DataSource_Load`: about `1884-1944 ns/op`, `1198-1199 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `91316-109863 ns/op`, `52187-53744 B/op`, `903-904 allocs/op`

Result after stage:
- Repeated sample over 3 runs:
  - `Benchmark_DataSource_Load`: `1869-1959 ns/op`, `1230-1234 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `86220-88353 ns/op`, `51720-52100 B/op`, `904 allocs/op`

What worked:
- The kernel now compiles a generated response plan for supported generated response messages.
- `Load` now selects a generated response writer fast path before falling back to the existing reflective marshaller.
- The heavy benchmark improved again on CPU and bytes/op:
  - latency moved from about `91-110 us/op` down to about `86-88 us/op`
  - bytes/op moved from about `52-54 KB` down to about `51.7-52.1 KB`
  - alloc count stayed flat at `904 allocs/op`
- Post-change profiling shows the old reflective response builder is no longer the package-side response hotspot on the generated path.

What did not work:
- This does not yet reduce alloc count because the fast path still materializes the same `astjson` tree shape.
- The simple load benchmark stayed effectively flat and gave back a small amount of bytes/op versus Stage 13.
- The fast path intentionally rejects oneofs, fragment-driven response selection, optional scalar wrappers, list-wrapper flattening, enum mapping, and field-flattening (`JSONPath == ""`), so unsupported shapes still use the old marshaller.
- This is not the final “direct final response writer” architecture; it is a generated-subtree writer that still feeds the existing merge pipeline.

Decision:
- keep

## Stage 15: Generated Resolve Direct-Apply Fast Path

Goal:
Delete the intermediate resolve response subtree on supported generated resolver outputs by applying generated results directly onto the already-built root response object.

Hypothesis:
If the datasource skips materializing `{"result":[...]}` for generated resolve outputs and writes the resolved values straight onto the target objects, the heavy benchmark should improve again because it deletes both subtree creation and `mergeWithPath` work for resolver calls.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/json_builder.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go`

Commands run:
- added red test for generated resolve-plan compilation and direct application
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^TestKernelCompilesGeneratedResolveApplyPlanAndUsesIt$' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- reverted the stage
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`

Baseline before stage:
- Stable kept reference before this stage:
  - `Benchmark_DataSource_Load`: about `1869-1959 ns/op`, `1230-1234 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `86220-88353 ns/op`, `51720-52100 B/op`, `904 allocs/op`

Result after stage:
- With the experiment enabled:
  - `Benchmark_DataSource_Load`: `1935-2011 ns/op`, `1247-1251 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `159375-282128 ns/op`, `57751-60102 B/op`, `907-909 allocs/op`
- After revert:
  - `Benchmark_DataSource_Load`: `1896-1960 ns/op`, `1231-1236 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `86988-89208 ns/op`, `51919-52099 B/op`, `904-905 allocs/op`

What worked:
- The stage correctly compiled a narrow generated resolve-apply plan for benchmark-shaped resolver outputs.
- Functionality was correct in the focused test and the full package test suite.
- The experiment confirmed a useful architectural fact: deleting the intermediate resolve subtree is only valuable if it does not also serialize response construction behind the stage barrier.

What did not work:
- The heavy benchmark regressed badly on CPU, bytes/op, and alloc count.
- The direct-apply path moved generated resolve response materialization out of the concurrent goroutine path and into the sequential merge phase, which erased the intended benefit and made the hot path slower overall.
- In other words, this version removed object creation but also removed too much parallelism.

Decision:
- revert
- revisit

## Stage 16: Concurrent Generated Resolve-Value Slices

Goal:
Keep resolver response construction on the concurrent goroutine path while deleting the intermediate `{"result":[...]}` wrapper object for supported generated resolver outputs.

Hypothesis:
If the datasource materializes only the final resolved value slice in parallel and then does a minimal sequential attach step, the heavy benchmark should improve over Stage 14 because it removes wrapper-object creation without repeating the Stage 15 mistake of serializing response construction.

Files touched:
- `v2/pkg/engine/datasource/grpc_datasource/compiler.go`
- `v2/pkg/engine/datasource/grpc_datasource/grpc_datasource.go`
- `v2/pkg/engine/datasource/grpc_datasource/json_builder.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel.go`
- `v2/pkg/engine/datasource/grpc_datasource/kernel_test.go`

Commands run:
- added red test for generated resolve-values-plan compilation and value-slice attach
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource -run '^TestKernelCompilesGeneratedResolveValuesPlanAndUsesIt$' -count=1`
- `cd v2 && go test ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=3 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load_WithFieldArguments$' -benchmem -cpuprofile /tmp/grpc-ds-stage16-args.cpu.out -memprofile /tmp/grpc-ds-stage16-args.mem.out -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -top /tmp/grpc-ds-stage16-args.cpu.out`
- `go tool pprof -sample_index=alloc_space -top /tmp/grpc-ds-stage16-args.mem.out`

Baseline before stage:
- Stable kept reference before this stage:
  - `Benchmark_DataSource_Load`: about `1869-1959 ns/op`, `1230-1234 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: about `86220-88353 ns/op`, `51720-52100 B/op`, `904 allocs/op`

Result after stage:
- Repeated sample over 3 runs:
  - `Benchmark_DataSource_Load`: `1910-1975 ns/op`, `1246-1251 B/op`, `19 allocs/op`
  - `Benchmark_DataSource_Load_WithFieldArguments`: `84145-87850 ns/op`, `51528-51905 B/op`, `906-907 allocs/op`

What worked:
- The kernel now compiles a generated resolve-values plan for the supported benchmark-shaped resolver output.
- The concurrent goroutine path now materializes only the resolved value slice for those resolver calls instead of a full `{"result":[...]}` subtree.
- The heavy benchmark improved again on CPU and bytes/op:
  - latency moved from about `86-88 us/op` down to about `84-88 us/op`
  - bytes/op moved from about `51.7-52.1 KB` down to about `51.5-51.9 KB`
- Profiling confirms the new work stayed on the concurrent path: `marshalGeneratedResolveValues` shows up as a small package-side allocation site, while the Stage 15-style sequential direct-apply regression did not reappear.
- This stage validates the architectural constraint uncovered by Stage 15: subtree deletion is only a win when parallel response materialization is preserved.

What did not work:
- Allocation count increased slightly in the heavy benchmark, from `904` to `906-907 allocs/op`.
- The simple load benchmark stayed essentially flat and gave back a small amount of bytes/op.
- The fast path still only supports a narrow resolver-output shape: repeated top-level `result` with exactly one scalar or message field per item, using generated linked schemas.

Decision:
- keep

## Current Status

From the original baseline to the current state:

- `Benchmark_DataSource_Load`: `2319 ns/op` -> about `1910-1975 ns/op`, `1852 B/op` -> `1246-1251 B/op`, `30 allocs/op` -> `19 allocs/op`
- `Benchmark_DataSource_Load_WithFieldArguments`: `154109 ns/op` -> about `84145-87850 ns/op`, `84956 B/op` -> `51528-51905 B/op`, `1488 allocs/op` -> `906-907 allocs/op`

Interpretation:

- The biggest structural gain came from compiling scheduling and request ownership out of the hot path.
- The next gains came from reducing schema-lookup churn and map-heavy context construction.
- The largest single runtime drop came from replacing `dynamicpb` allocation with generated message allocation when linked types are available.
- Kernel-owned sharded memory closed out another chunk of request-local allocation overhead and fixed the wrong pool-key model.
- The compiled runtime-type cache cleaned up the remaining allocation path and formalized the right architecture for runtime-only schemas.
- A standalone runtime abstraction is not worth keeping until it lands with a real faster backend.
- Direct response application is not worth keeping by itself; it needs a larger surrounding architecture change to matter.
- A field-number-based resolver-path interpreter is not enough by itself; it saves allocations but still loses on CPU.
- A shared-list context fast path is also not enough by itself; it removes row-building allocations but still loses on CPU because the underlying protobuf machinery remains too generic.
- The first real post-Stage-10 win came from deleting generic protobuf work entirely on the generated resolver path rather than compiling another generic interpreter.
- The resolve-side wrapper subtree can be reduced profitably, but only if the work stays on the concurrent goroutine path.
- The next real ceiling is now even narrower: the remaining generic fallback path and the still-generic final response assembly for unsupported shapes.

## Stage 0: Baseline And Profiles

Goal:
Capture a clean baseline before any code changes in this campaign.

Hypothesis:
The current baseline should confirm that `Benchmark_DataSource_Load_WithFieldArguments` is the dominant cost center and that dependency graph work is measurable but secondary.

Files touched:
- `IMPROVEMENTS.md`

Commands run:
- `cd v2 && go test -count=1 -run '^$' -bench '^(Benchmark_DataSource_Load|Benchmark_DataSource_Load_WithFieldArguments|BenchmarkBuildDependencyGraph|BenchmarkCompareKeyFields)$' -benchmem ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load$' -cpuprofile <tmp> -memprofile <tmp> -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `cd v2 && go test -count=1 -run '^$' -bench '^Benchmark_DataSource_Load_WithFieldArguments$' -cpuprofile <tmp> -memprofile <tmp> -memprofilerate=1 -cpu=1 ./pkg/engine/datasource/grpc_datasource`
- `go tool pprof -top ...`
- `go tool pprof -sample_index=alloc_space -top ...`

Baseline before stage:
- not applicable

Result after stage:
- `Benchmark_DataSource_Load_WithFieldArguments` is still the dominant runtime target at
  `154109 ns/op`, `84956 B/op`, `1488 allocs/op`.
- `BenchmarkBuildDependencyGraph` is small in absolute time but still pure structural overhead
  at `343.1 ns/op`, `432 B/op`, `7 allocs/op`.
- `Benchmark_DataSource_Load` still shows package-side alloc pressure in graph construction,
  fetch compilation, and field lookup before transport cost dominates.

What worked:
- The current benchmark set is sufficient to compare structural changes stage by stage.
- Profiling still clearly separates interpreter overhead from transport/protobuf overhead.

What did not work:
- CPU profiles for the field-args benchmark are noisy because profiling overhead is large relative
  to the benchmark duration; alloc-space data is more actionable for early stages.

Decision:
- keep
