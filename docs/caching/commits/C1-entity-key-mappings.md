# Commit C1 — freeze EntityKeyMappings (root-arg ↔ @key)

Plan item: `docs/caching/PLAN.md` §5, C1.
RFC sections: RFC-2 §6, §6.3 (mappings are additional candidates), R4 (v1 freezes structurally-derivable mappings).
Phase: C (root fields that re-use the entity cache).

## Problem

A by-key root field (`product(upc:)`, `user(id:)`) cannot reuse an entity-cache entry because its root args are not linked to the entity `@key`. The OLD branch took these mappings from external config; on this branch they must be derived structurally.

## Solution

Implement `freezeEntityKeyMappings(definition, fed, rootTypeName, rootFieldName)` and populate `CacheKeySpec.EntityKeyMappings` for ROOT-FIELD fetches:

1. Resolve the root field's return type from the definition (`NodeFieldDefinitionByName` -> `FieldDefinitionTypeNameString`). If it is not a federation entity (`!fed.HasEntity`), no mappings.
2. Collect the root field's argument names (`NodeInputValueDefinitions` -> `InputValueDefinitionNameString`).
3. For each `@key` set (`RequiredFieldsByKey`, sorted by `SelectionSet`), parse it into field names (`keySelectionSetFieldNames` over `ast.SelectionSetFieldNames`/`RequiredFieldsFragment`). Map the `@key` ONLY when EVERY key field has a matching root-arg name, producing `EntityKeyMapping{EntityTypeName, [{EntityKeyField, ArgumentPath:[field], ArgumentIsEntityKey:true}]}`. Dedup identical mappings.

So `product(upc:)` -> `{Product,[{upc,[upc],true}]}` (the `@key(sku)` set is NOT mapped — `sku` is not an arg), and `user(id:)` -> `{User,[{id,[id],true}]}`.

## Key decisions

- Structurally-derivable only (RFC-2 R4): a `@key` maps only when its field names match root-arg names; operator-declared name-mismatch overrides are staged to a future provider method.
- Mappings are frozen onto the ROOT-FIELD spec (the entities are RENDERED at runtime from them in C2); entity-scope `EntityKeyMappings` stay nil (entity fetches already key by representation).
- No federation pointer escapes the freezer (value strings/slices; the post-freeze mutation test pins it).

## Tests

- `postprocess/cache_key_spec_freezer_test.go`: `product(upc:)` -> full `EntityKeyMappings` `[{Product,[{upc,[upc],true}]}]`; `user(id:)` -> `[{User,[{id,[id],true}]}]`; a root field whose args don't match any `@key` -> empty; post-freeze `FederationMetaData` mutation re-asserted (no aliasing).
- Execution `caching_rootreuse_golden_test.go` `TestCaching_StageL2RootReusesEntity_Golden` over `{ product(upc:"1"){ name } }`: the `product` root fetch's KeySpec shows `mappings:[Product:1]`. `cacheProvidersForStage(StageL2RootReusesEntity)` adds `RootFieldPolicy` for `(Query, product)`/`(Query, user)`.

Verification:

- `cd v2 && go test ./pkg/engine/postprocess/... ./pkg/engine/plan/... ./pkg/engine/resolve/... -count=1` — PASS; StageNoop no-op golden byte-identical.
- `cd execution && go test ./engine/ -run 'Caching' -count=1` — PASS (StageNoop/StageL2Entities/StageL2RootFields goldens unchanged; new StageL2RootReusesEntity golden).
- `cd v2 && go build ./pkg/... && go vet ./pkg/engine/postprocess/...` — clean.

## Reviewer guidance

- Mappings are frozen from federation + definition, never from the policy struct; no federation pointer retained.
- A `@key` is mapped only when all its fields match root args (best-effort; the runtime renders the entity candidate from these in C2).
- The runtime reuse at lookup is C2.
