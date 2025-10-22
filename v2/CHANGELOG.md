# Changelog

## [2.0.0-rc.232](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.231...v2.0.0-rc.232) (2025-10-22)


### Bug Fixes

* correct SSE datasource complete behaviour ([#1311](https://github.com/wundergraph/graphql-go-tools/issues/1311)) ([18c39e7](https://github.com/wundergraph/graphql-go-tools/commit/18c39e7d7e42086bb710d7dc757e32c4eeed94f9))

## [2.0.0-rc.231](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.230...v2.0.0-rc.231) (2025-10-20)


### Features

* support the oneOf directive ([#1308](https://github.com/wundergraph/graphql-go-tools/issues/1308)) ([251cb02](https://github.com/wundergraph/graphql-go-tools/commit/251cb029a9e232f522ab3260db3d80942222ed2c))

## [2.0.0-rc.230](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.229...v2.0.0-rc.230) (2025-10-09)


### Bug Fixes

* avoid duplicate joins on errors ([#1314](https://github.com/wundergraph/graphql-go-tools/issues/1314)) ([a1f1f8c](https://github.com/wundergraph/graphql-go-tools/commit/a1f1f8c1e68e4fce79135423adb7f7ad27feb570))
* propagate fetch reasons for interface-related fields ([#1312](https://github.com/wundergraph/graphql-go-tools/issues/1312)) ([5ee3014](https://github.com/wundergraph/graphql-go-tools/commit/5ee3014edef13461fb1ef9e6297629f31ef6db7c))

## [2.0.0-rc.229](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.228...v2.0.0-rc.229) (2025-09-25)


### Bug Fixes

* remove index after _entities in the path ([#1306](https://github.com/wundergraph/graphql-go-tools/issues/1306)) ([7d0586e](https://github.com/wundergraph/graphql-go-tools/commit/7d0586effc154fdfe6eddb949b46a2e58943b801))

## [2.0.0-rc.228](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.227...v2.0.0-rc.228) (2025-09-24)


### Features

* validate presence of optional `@requires` dependencies ([#1297](https://github.com/wundergraph/graphql-go-tools/issues/1297)) ([ba75e25](https://github.com/wundergraph/graphql-go-tools/commit/ba75e25483165fa0172bad6c4504b0f48d94cd9b))

## [2.0.0-rc.227](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.226...v2.0.0-rc.227) (2025-09-23)


### Bug Fixes

* if blocked, defer async event insertion to prevent deadlocks ([#1298](https://github.com/wundergraph/graphql-go-tools/issues/1298)) ([df38c31](https://github.com/wundergraph/graphql-go-tools/commit/df38c3121216ac5695f7f00ba1a810ebb879651e))

## [2.0.0-rc.226](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.225...v2.0.0-rc.226) (2025-09-12)


### Bug Fixes

* detecting requires on interface members ([#1295](https://github.com/wundergraph/graphql-go-tools/issues/1295)) ([70bd5d5](https://github.com/wundergraph/graphql-go-tools/commit/70bd5d5b4f5d8442e488c0e1b4ed5e31f5113295))

## [2.0.0-rc.225](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.224...v2.0.0-rc.225) (2025-09-09)


### Features

* add support of field selection reasons extensions ([#1282](https://github.com/wundergraph/graphql-go-tools/issues/1282)) ([37c9582](https://github.com/wundergraph/graphql-go-tools/commit/37c95820a5892935315b59aea99b6efe646cccfb))
* upgrade all components to go 1.25 ([#1289](https://github.com/wundergraph/graphql-go-tools/issues/1289)) ([6bd2713](https://github.com/wundergraph/graphql-go-tools/commit/6bd27137a06e175f7987a1fed6debfe7c8f649af))


### Bug Fixes

* refactor CoordinateDependencies, FetchReasons ([#1293](https://github.com/wundergraph/graphql-go-tools/issues/1293)) ([cfebc16](https://github.com/wundergraph/graphql-go-tools/commit/cfebc16a2876fd94dbe50c08b5ede4688b0f2ec5))

## [2.0.0-rc.224](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.223...v2.0.0-rc.224) (2025-09-01)


### Bug Fixes

* add sorting before creating the hash ([#1286](https://github.com/wundergraph/graphql-go-tools/issues/1286)) ([7fa9c50](https://github.com/wundergraph/graphql-go-tools/commit/7fa9c50ca1bf9e726ac2a04060d98c4b4a22d404))

## [2.0.0-rc.223](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.222...v2.0.0-rc.223) (2025-08-28)


### Features

* improved subscription heartbeats ([#1269](https://github.com/wundergraph/graphql-go-tools/issues/1269)) ([4423d60](https://github.com/wundergraph/graphql-go-tools/commit/4423d60afd7bb8a58b193e31b61d7226d10dfd17))

## [2.0.0-rc.222](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.221...v2.0.0-rc.222) (2025-08-27)


### Bug Fixes

* improve handling for invalid responses ([#1283](https://github.com/wundergraph/graphql-go-tools/issues/1283)) ([d656e91](https://github.com/wundergraph/graphql-go-tools/commit/d656e9185a9d94874ae7b0c103176c10b6a4f352))

## [2.0.0-rc.221](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.220...v2.0.0-rc.221) (2025-08-21)


### Bug Fixes

* always close response body ([#1276](https://github.com/wundergraph/graphql-go-tools/issues/1276)) ([9069cc9](https://github.com/wundergraph/graphql-go-tools/commit/9069cc9b88bae6c64a9438bb08d6f5b4a5103c78))

## [2.0.0-rc.220](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.219...v2.0.0-rc.220) (2025-08-13)


### Features

* rewrite abstract fragments for grpc ([#1268](https://github.com/wundergraph/graphql-go-tools/issues/1268)) ([ebe1e53](https://github.com/wundergraph/graphql-go-tools/commit/ebe1e533aedc63b5970f476cb1bf37e88c6c21c4))

## [2.0.0-rc.219](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.218...v2.0.0-rc.219) (2025-08-08)


### Features

* add support for multiple key directives ([#1262](https://github.com/wundergraph/graphql-go-tools/issues/1262)) ([8535a92](https://github.com/wundergraph/graphql-go-tools/commit/8535a92f5b58e8f49330e9536ccdc39462a7142a))

## [2.0.0-rc.218](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.217...v2.0.0-rc.218) (2025-08-07)


### Bug Fixes

* fix rewriting an interface object implementing interface ([#1265](https://github.com/wundergraph/graphql-go-tools/issues/1265)) ([8c8c9de](https://github.com/wundergraph/graphql-go-tools/commit/8c8c9deaccfb9b9657a0d2cef9923d9caaf94a61))

## [2.0.0-rc.217](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.216...v2.0.0-rc.217) (2025-08-06)


### Bug Fixes

* add an option to ignore skip/include directives ([#1261](https://github.com/wundergraph/graphql-go-tools/issues/1261)) ([dfd6523](https://github.com/wundergraph/graphql-go-tools/commit/dfd65236170251c7e195015130da8b9eceeddf3a))

## [2.0.0-rc.216](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.215...v2.0.0-rc.216) (2025-08-06)


### Bug Fixes

* generate query plans for subscriptions ([#1260](https://github.com/wundergraph/graphql-go-tools/issues/1260)) ([560a89d](https://github.com/wundergraph/graphql-go-tools/commit/560a89d9f8d3797cd93c1fea136df5ab54b5bd0e))

## [2.0.0-rc.215](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.214...v2.0.0-rc.215) (2025-07-31)


### Bug Fixes

* make propagated operation names unique ([#1256](https://github.com/wundergraph/graphql-go-tools/issues/1256)) ([c2be87e](https://github.com/wundergraph/graphql-go-tools/commit/c2be87e80965697d51fb162d3614db7d135dbd8f))

## [2.0.0-rc.214](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.213...v2.0.0-rc.214) (2025-07-29)


### Bug Fixes

* handle nullable and nested argument lists properly ([#1254](https://github.com/wundergraph/graphql-go-tools/issues/1254)) ([67af556](https://github.com/wundergraph/graphql-go-tools/commit/67af5568bff4da601bbbb9082158087223581d6d))

## [2.0.0-rc.213](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.212...v2.0.0-rc.213) (2025-07-28)


### Bug Fixes

* fix parent node jump lookup ([#1252](https://github.com/wundergraph/graphql-go-tools/issues/1252)) ([9fb01be](https://github.com/wundergraph/graphql-go-tools/commit/9fb01be8188dcab52feb0877ab8f2a023143cb51))

## [2.0.0-rc.212](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.211...v2.0.0-rc.212) (2025-07-28)


### Bug Fixes

* handle null only for outer list ([#1250](https://github.com/wundergraph/graphql-go-tools/issues/1250)) ([0e055a4](https://github.com/wundergraph/graphql-go-tools/commit/0e055a447f4201f5b8c24e9786be71f6265457b6))

## [2.0.0-rc.211](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.210...v2.0.0-rc.211) (2025-07-28)


### Features

* redesign handling for lists in gRPC ([#1246](https://github.com/wundergraph/graphql-go-tools/issues/1246)) ([a06c9db](https://github.com/wundergraph/graphql-go-tools/commit/a06c9db0f2ac6558ef957885784e25e127ff40ae))


### Bug Fixes

* disable minifier for gRPC datasource ([#1249](https://github.com/wundergraph/graphql-go-tools/issues/1249)) ([9a26e5c](https://github.com/wundergraph/graphql-go-tools/commit/9a26e5cb4861b3a5f3adfa942970d4e42c05d718))
* test v2 benchmarks on ci ([#1238](https://github.com/wundergraph/graphql-go-tools/issues/1238)) ([d9cfb21](https://github.com/wundergraph/graphql-go-tools/commit/d9cfb2144387ff2e42e5b620ec93abcb11ff314b))

## [2.0.0-rc.210](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.209...v2.0.0-rc.210) (2025-07-22)


### Bug Fixes

* planner fixes for parent entity jumps and unique nodes selections ([#1230](https://github.com/wundergraph/graphql-go-tools/issues/1230)) ([1a7ed16](https://github.com/wundergraph/graphql-go-tools/commit/1a7ed16008de28adebdb0fb3485ba2ea5205d8e8))

## [2.0.0-rc.209](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.208...v2.0.0-rc.209) (2025-07-22)


### Bug Fixes

* merge inline fragment and field selections together ([#1240](https://github.com/wundergraph/graphql-go-tools/issues/1240)) ([99f2b32](https://github.com/wundergraph/graphql-go-tools/commit/99f2b321990591f51a1dd0f84e6b3696fb457d33))

## [2.0.0-rc.208](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.207...v2.0.0-rc.208) (2025-07-18)


### Features

* add depth limit to parser ([#1241](https://github.com/wundergraph/graphql-go-tools/issues/1241)) ([7de2a2e](https://github.com/wundergraph/graphql-go-tools/commit/7de2a2e6302e06f52e869d71546fe60811b89b50))


### Bug Fixes

* check that object claims to implement interface ([#1235](https://github.com/wundergraph/graphql-go-tools/issues/1235)) ([5afbc68](https://github.com/wundergraph/graphql-go-tools/commit/5afbc6821a858fac7ce2d3d62559aed196477bb3))
* use NodeFragmentIsAllowedOnNode to check fragmentSpread ([#1223](https://github.com/wundergraph/graphql-go-tools/issues/1223)) ([e448c81](https://github.com/wundergraph/graphql-go-tools/commit/e448c81e19a0b9955a449dbcdd207f60a7883994))

## [2.0.0-rc.207](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.206...v2.0.0-rc.207) (2025-07-15)


### Bug Fixes

* fix merging fetches and add dependencies update ([#1232](https://github.com/wundergraph/graphql-go-tools/issues/1232)) ([c91d59e](https://github.com/wundergraph/graphql-go-tools/commit/c91d59eeeb9ac09f84806b9d3af903f5d25f064d))

## [2.0.0-rc.206](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.205...v2.0.0-rc.206) (2025-07-15)


### Bug Fixes

* allow multiple aliases ([#1231](https://github.com/wundergraph/graphql-go-tools/issues/1231)) ([01d91e2](https://github.com/wundergraph/graphql-go-tools/commit/01d91e2b882b000d95a08ac4e96d97d95f1e3a9d))

## [2.0.0-rc.205](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.204...v2.0.0-rc.205) (2025-07-14)


### Bug Fixes

* missing field refs in union types ([#1228](https://github.com/wundergraph/graphql-go-tools/issues/1228)) ([57e1d38](https://github.com/wundergraph/graphql-go-tools/commit/57e1d38955f14df4e39f9f139a31a40a6d6e4659))
* return empty list for nullable lists ([#1225](https://github.com/wundergraph/graphql-go-tools/issues/1225)) ([1166a10](https://github.com/wundergraph/graphql-go-tools/commit/1166a10a5da4151af5a7abeea7bee45d63d71349))

## [2.0.0-rc.204](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.203...v2.0.0-rc.204) (2025-07-10)


### Features

* support nullable base types ([#1212](https://github.com/wundergraph/graphql-go-tools/issues/1212)) ([b45b92c](https://github.com/wundergraph/graphql-go-tools/commit/b45b92c37854778851740bcbd9d0562641b4593b))

## [2.0.0-rc.203](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.202...v2.0.0-rc.203) (2025-07-10)


### Features

* option to allow all error extensions ([#1217](https://github.com/wundergraph/graphql-go-tools/issues/1217)) ([b2e6575](https://github.com/wundergraph/graphql-go-tools/commit/b2e65752b043151c5a21f0dfbebe6823c4b96f0f))

## [2.0.0-rc.202](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.201...v2.0.0-rc.202) (2025-07-09)


### Bug Fixes

* return parsing error for empty selection sets ([#1220](https://github.com/wundergraph/graphql-go-tools/issues/1220)) ([726c0d2](https://github.com/wundergraph/graphql-go-tools/commit/726c0d203edba1a863444cbbe70ccec2092d8416))

## [2.0.0-rc.201](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.200...v2.0.0-rc.201) (2025-07-09)


### Features

* use a method to get response body details on demand ([#1218](https://github.com/wundergraph/graphql-go-tools/issues/1218)) ([3964286](https://github.com/wundergraph/graphql-go-tools/commit/39642862866d4a94691de10574834f64a66fb2a3))

## [2.0.0-rc.200](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.199...v2.0.0-rc.200) (2025-07-08)


### Features

* implement resolving fetch dependencies ([#1216](https://github.com/wundergraph/graphql-go-tools/issues/1216)) ([ca9ebaa](https://github.com/wundergraph/graphql-go-tools/commit/ca9ebaa7784b5da89c78239f83a1c3eba909b838))


### Bug Fixes

* use existing files in BenchmarkMinify ([#1214](https://github.com/wundergraph/graphql-go-tools/issues/1214)) ([0083b78](https://github.com/wundergraph/graphql-go-tools/commit/0083b7880bbcb20da4f26ca509e45cb48eedaaf1))
* use int in netpoll's BenchmarkSocketFdReflect ([#1213](https://github.com/wundergraph/graphql-go-tools/issues/1213)) ([35f3175](https://github.com/wundergraph/graphql-go-tools/commit/35f31751d5971c491c98fa3b223fba846729a3fa))

## [2.0.0-rc.199](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.198...v2.0.0-rc.199) (2025-07-07)


### Features

* add support for aliases ([#1209](https://github.com/wundergraph/graphql-go-tools/issues/1209)) ([9223351](https://github.com/wundergraph/graphql-go-tools/commit/9223351ca9530e3738bfac794de108bbbac134c0))


### Bug Fixes

* do not trim whitespaces around non-block strings ([#1211](https://github.com/wundergraph/graphql-go-tools/issues/1211)) ([6f5046b](https://github.com/wundergraph/graphql-go-tools/commit/6f5046b8b1cdbd3d54154d55cc049e45404905aa))

## [2.0.0-rc.198](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.197...v2.0.0-rc.198) (2025-07-04)


### Features

* add support for composite types ([#1197](https://github.com/wundergraph/graphql-go-tools/issues/1197)) ([e9b9f19](https://github.com/wundergraph/graphql-go-tools/commit/e9b9f193b749089eda7fa9126e93407c2a4dbd7f))


### Bug Fixes

* fix collecting representation for fetches scoped to concrete types ([#1200](https://github.com/wundergraph/graphql-go-tools/issues/1200)) ([bcf547d](https://github.com/wundergraph/graphql-go-tools/commit/bcf547d8c5f93fe6caf1c90b8f3049c94d1fed23))

## [2.0.0-rc.197](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.196...v2.0.0-rc.197) (2025-07-03)


### Features

* status code derived fallback errors ([#1198](https://github.com/wundergraph/graphql-go-tools/issues/1198)) ([aa1c7ef](https://github.com/wundergraph/graphql-go-tools/commit/aa1c7efcb22a92c63e22b5de71905fdc327c6875))

## [2.0.0-rc.196](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.195...v2.0.0-rc.196) (2025-07-02)


### Features

* pass in response body ([#1203](https://github.com/wundergraph/graphql-go-tools/issues/1203)) ([ef03374](https://github.com/wundergraph/graphql-go-tools/commit/ef03374352bff7715b430409d20a55c9f456405a))

## [2.0.0-rc.195](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.194...v2.0.0-rc.195) (2025-07-02)


### Bug Fixes

* fix checking presence of type mapping to interface object ([#1201](https://github.com/wundergraph/graphql-go-tools/issues/1201)) ([0a849ee](https://github.com/wundergraph/graphql-go-tools/commit/0a849ee561dd4abac38f870ca61299ca93672acd))

## [2.0.0-rc.194](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.193...v2.0.0-rc.194) (2025-06-28)


### Bug Fixes

* preserve fields selections during object selection rewrite ([#1194](https://github.com/wundergraph/graphql-go-tools/issues/1194)) ([1c9d4d2](https://github.com/wundergraph/graphql-go-tools/commit/1c9d4d2be548da6879f5b7b26d67b5d0ba52df93))

## [2.0.0-rc.193](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.192...v2.0.0-rc.193) (2025-06-27)


### Bug Fixes

* fix rewriting object selections with nested abstract fragments ([#1192](https://github.com/wundergraph/graphql-go-tools/issues/1192)) ([a22b89c](https://github.com/wundergraph/graphql-go-tools/commit/a22b89c4379db027e3e5be99670cc14d037c1894))

## [2.0.0-rc.192](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.191...v2.0.0-rc.192) (2025-06-19)


### Bug Fixes

* don't panic when calling QueryPlan on FetchTreeNode if `includeQueryPlans` is false ([#1189](https://github.com/wundergraph/graphql-go-tools/issues/1189)) ([f69a3a6](https://github.com/wundergraph/graphql-go-tools/commit/f69a3a6270c4043904d188800a35796dffd4ba43))

## [2.0.0-rc.191](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.190...v2.0.0-rc.191) (2025-06-18)


### Bug Fixes

* don't send complete when request/resolver context is done ([#1187](https://github.com/wundergraph/graphql-go-tools/issues/1187)) ([9b51ad6](https://github.com/wundergraph/graphql-go-tools/commit/9b51ad6632c28a985b1f67c97ab1e68647a5f93d))

## [2.0.0-rc.190](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.189...v2.0.0-rc.190) (2025-06-13)


### Bug Fixes

* handling nested abstract fragments in abstract fragments ([#1184](https://github.com/wundergraph/graphql-go-tools/issues/1184)) ([c3321d3](https://github.com/wundergraph/graphql-go-tools/commit/c3321d3cd72b63fbf5d7d949d70fd6dfeb73e2e1))

## [2.0.0-rc.189](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.188...v2.0.0-rc.189) (2025-06-12)


### Features

* expose parsed data in RenderFieldValue ([#1182](https://github.com/wundergraph/graphql-go-tools/issues/1182)) ([893c536](https://github.com/wundergraph/graphql-go-tools/commit/893c5366239362af68e0258c5987e7be24d286cb))


### Bug Fixes

* prefer websocket tws when negotiate graphql websocket protocol ([#1185](https://github.com/wundergraph/graphql-go-tools/issues/1185)) ([0787d7c](https://github.com/wundergraph/graphql-go-tools/commit/0787d7c8431fe840396e7b5b8e2aa47f1cd94ead))

## [2.0.0-rc.188](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.187...v2.0.0-rc.188) (2025-06-11)


### Features

* improve apollo gateway compatible field selection validation ([#1169](https://github.com/wundergraph/graphql-go-tools/issues/1169)) ([8c1a063](https://github.com/wundergraph/graphql-go-tools/commit/8c1a06302309b5c3ad36f908cc8acbbc0bfafda6))

## [2.0.0-rc.187](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.186...v2.0.0-rc.187) (2025-06-10)


### Bug Fixes

* support different kinds of close, correct client unsubscribe behaviour ([#1174](https://github.com/wundergraph/graphql-go-tools/issues/1174)) ([b6de322](https://github.com/wundergraph/graphql-go-tools/commit/b6de32263b69902c1f687b7b3fbf89e90df85cd2))

## [2.0.0-rc.186](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.185...v2.0.0-rc.186) (2025-06-05)


### Bug Fixes

* upgrade golang/net version and dependencies ([#1173](https://github.com/wundergraph/graphql-go-tools/issues/1173)) ([1e889dd](https://github.com/wundergraph/graphql-go-tools/commit/1e889dd3d79385db80f2a66fec70c9aef3423d76))

## [2.0.0-rc.185](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.184...v2.0.0-rc.185) (2025-06-04)


### Features

* differentiate between complete and close event types ([#1158](https://github.com/wundergraph/graphql-go-tools/issues/1158)) ([79f3f41](https://github.com/wundergraph/graphql-go-tools/commit/79f3f411b4101b0cdb29c2e5f075b8efe14fa6d8))

## [2.0.0-rc.184](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.183...v2.0.0-rc.184) (2025-06-03)


### Features

* add custom field renderer to resolvable ([#1166](https://github.com/wundergraph/graphql-go-tools/issues/1166)) ([eaa5e60](https://github.com/wundergraph/graphql-go-tools/commit/eaa5e60cd1687593d538f33a914d0a6c1acace9b))


### Bug Fixes

* usage of unions fragments in union/object selection set ([#1165](https://github.com/wundergraph/graphql-go-tools/issues/1165)) ([c78c7e8](https://github.com/wundergraph/graphql-go-tools/commit/c78c7e828bef8fd80dbd4a3b47bd7ec910a5eb0b))

## [2.0.0-rc.183](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.182...v2.0.0-rc.183) (2025-05-28)


### Features

* use new negation regex matching for matching connections ([#1161](https://github.com/wundergraph/graphql-go-tools/issues/1161)) ([4f2fe65](https://github.com/wundergraph/graphql-go-tools/commit/4f2fe65ffcf51346b7907626767e5472f56c1b11))

## [2.0.0-rc.182](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.181...v2.0.0-rc.182) (2025-05-27)


### Bug Fixes

* handle scalar values for lists ([#1155](https://github.com/wundergraph/graphql-go-tools/issues/1155)) ([94031e5](https://github.com/wundergraph/graphql-go-tools/commit/94031e5a1fa20a15b0d01a5a7f94c7dffec122f9))

## [2.0.0-rc.181](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.180...v2.0.0-rc.181) (2025-05-23)


### Bug Fixes

* do not add array field when node was skipped ([#1156](https://github.com/wundergraph/graphql-go-tools/issues/1156)) ([61dc0b1](https://github.com/wundergraph/graphql-go-tools/commit/61dc0b19b639b1d321c2bce12cad63ec70925a5f))

## [2.0.0-rc.180](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.179...v2.0.0-rc.180) (2025-05-20)


### Bug Fixes

* detach fetches from objects, serial mutations execution, remove fetch id from operation name ([#1150](https://github.com/wundergraph/graphql-go-tools/issues/1150)) ([d62026b](https://github.com/wundergraph/graphql-go-tools/commit/d62026b95029badee5cb68e24559201a7570c816))
* fix response path for aliased fields ([#1153](https://github.com/wundergraph/graphql-go-tools/issues/1153)) ([b6270bd](https://github.com/wundergraph/graphql-go-tools/commit/b6270bde84f599fd6dfc32358a9aaa11a7bcbdd7))

## [2.0.0-rc.179](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.178...v2.0.0-rc.179) (2025-05-19)


### Features

* implement gRPC datasource ([#1146](https://github.com/wundergraph/graphql-go-tools/issues/1146)) ([146a552](https://github.com/wundergraph/graphql-go-tools/commit/146a552419e83b350b769a5e37cceb6d3f4b59d3))


### Bug Fixes

* print indent once per level by default ([#1147](https://github.com/wundergraph/graphql-go-tools/issues/1147)) ([0f022e5](https://github.com/wundergraph/graphql-go-tools/commit/0f022e5a7443d71fa5c458485876dfaac4cf060b)), closes [#405](https://github.com/wundergraph/graphql-go-tools/issues/405)

## [2.0.0-rc.178](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.177...v2.0.0-rc.178) (2025-05-06)


### Features

* add deprecated arguments support to introspection ([#1142](https://github.com/wundergraph/graphql-go-tools/issues/1142)) ([1ac2908](https://github.com/wundergraph/graphql-go-tools/commit/1ac2908ec5ab5cfb5aed17c1fee127aef098c7fc))

## [2.0.0-rc.177](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.176...v2.0.0-rc.177) (2025-05-06)


### Bug Fixes

* use non aliased field name for graph coordinate ([#1143](https://github.com/wundergraph/graphql-go-tools/issues/1143)) ([a2ef742](https://github.com/wundergraph/graphql-go-tools/commit/a2ef742e9336f942702ec0cfd6d7fff32a270221))

## [2.0.0-rc.176](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.175...v2.0.0-rc.176) (2025-04-30)


### Bug Fixes

* evaluate keys using order of target subgraph ([#1139](https://github.com/wundergraph/graphql-go-tools/issues/1139)) ([f358e9e](https://github.com/wundergraph/graphql-go-tools/commit/f358e9e74c16f0c372170d9cde82decbf4991289))

## [2.0.0-rc.175](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.174...v2.0.0-rc.175) (2025-04-29)


### Features

* remove intermediate buffer from ResolveGraphQLResponse ([#1137](https://github.com/wundergraph/graphql-go-tools/issues/1137)) ([9f25e6f](https://github.com/wundergraph/graphql-go-tools/commit/9f25e6fccd15fa0a847d453ebd05276c2b250721))

## [2.0.0-rc.174](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.173...v2.0.0-rc.174) (2025-04-29)


### Features

* **subscriptions:** user proper frame timeout ([#1135](https://github.com/wundergraph/graphql-go-tools/issues/1135)) ([d1fbd62](https://github.com/wundergraph/graphql-go-tools/commit/d1fbd624f7af19802f736e191ba4079abbdd0a37))

## [2.0.0-rc.173](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.172...v2.0.0-rc.173) (2025-04-25)


### Bug Fixes

* use proper read timeout ([#1133](https://github.com/wundergraph/graphql-go-tools/issues/1133)) ([9a8f4aa](https://github.com/wundergraph/graphql-go-tools/commit/9a8f4aa99b007cfc69022992b8cf9aa150047a54))

## [2.0.0-rc.172](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.171...v2.0.0-rc.172) (2025-04-22)


### Features

* add static resolver of root operation types __typename fields ([#1128](https://github.com/wundergraph/graphql-go-tools/issues/1128)) ([3dc79f1](https://github.com/wundergraph/graphql-go-tools/commit/3dc79f18694ff83782a200f7f59541038a29655c))
* add support of fragments in the provides directive ([ca2e6c2](https://github.com/wundergraph/graphql-go-tools/commit/ca2e6c2e4fd6f601f9e72faefc922dba9a0fc01d))
* add support of inline fragments to representation variables builder; add typename selection to requires selection set in case it has fragments ([40e3fd9](https://github.com/wundergraph/graphql-go-tools/commit/40e3fd95c35fa2a0a46eee58c7ebdc75595e41f1))


### Bug Fixes

* add support of inline fragments in requires directive, add an alias for required fields in case of conflict ([496b7da](https://github.com/wundergraph/graphql-go-tools/commit/496b7dacb44e2e8b3c2405c70b311b325b1237ea))
* add support of remapping representation variables paths ([204e0b6](https://github.com/wundergraph/graphql-go-tools/commit/204e0b66889b8631196c3a6abd696f5213c2bc4b))
* allow to recheck abstract fields if they were modified by adding required fields ([b4d5fd5](https://github.com/wundergraph/graphql-go-tools/commit/b4d5fd51ce9eb13b892efb368a3b19cef2a5fc51))
* during abstract selection rewrite collect changed paths and after rewrite update field dependencies accordingly ([a6246fd](https://github.com/wundergraph/graphql-go-tools/commit/a6246fd1bb795a4c9092dea2b912193133479db1))
* exclude inaccesible types from possible types for interface/union ([ae1adc1](https://github.com/wundergraph/graphql-go-tools/commit/ae1adc187c0263792fe089184181416bc485fb7c))
* rework key matching logic to support chain leapfrogging jumps ([4f12691](https://github.com/wundergraph/graphql-go-tools/commit/4f1269166baf7ea4fcca8d67f87c38eab4ce179d))
* tune selections for root query fields and typename only selection sets ([ea3b276](https://github.com/wundergraph/graphql-go-tools/commit/ea3b276190252f798e672cf9ce46a8f611b15924))

## [2.0.0-rc.171](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.170...v2.0.0-rc.171) (2025-04-22)


### Bug Fixes

* dont use completed channel in sub updater ([#1127](https://github.com/wundergraph/graphql-go-tools/issues/1127)) ([be7db2a](https://github.com/wundergraph/graphql-go-tools/commit/be7db2ae9597003c546ee5ee87e155714a9f8495))

## [2.0.0-rc.170](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.169...v2.0.0-rc.170) (2025-04-17)


### Features

* operation input to MCP compatible json schema converter ([#1124](https://github.com/wundergraph/graphql-go-tools/issues/1124)) ([48cc99b](https://github.com/wundergraph/graphql-go-tools/commit/48cc99b441f851d1e0ecac2b5f4199a3e8eac94c))

## [2.0.0-rc.169](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.168...v2.0.0-rc.169) (2025-04-07)


### Bug Fixes

* **websocket:** handle ping/pong correctly ([#1122](https://github.com/wundergraph/graphql-go-tools/issues/1122)) ([8001f90](https://github.com/wundergraph/graphql-go-tools/commit/8001f90d29360e87e28450ad2c8af551efbecbff))

## [2.0.0-rc.168](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.167...v2.0.0-rc.168) (2025-04-01)


### Bug Fixes

* allow custom scalar values of any kind ([#1107](https://github.com/wundergraph/graphql-go-tools/issues/1107)) ([1a67689](https://github.com/wundergraph/graphql-go-tools/commit/1a67689322b86debee5aa9c8dd1dfc526a82a559))
* get query for plan when input is not valid JSON yet ([#1120](https://github.com/wundergraph/graphql-go-tools/issues/1120)) ([69485df](https://github.com/wundergraph/graphql-go-tools/commit/69485dfe7a76f77902595512f8ce6578cdc073f5))
* set proper write / read timeouts ([#1113](https://github.com/wundergraph/graphql-go-tools/issues/1113)) ([e717013](https://github.com/wundergraph/graphql-go-tools/commit/e717013750e65c4a8e513dfd6b5d4ae5e523dbf7))

## [2.0.0-rc.167](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.166...v2.0.0-rc.167) (2025-03-26)


### Features

* reset skipFieldsRefs ([#1117](https://github.com/wundergraph/graphql-go-tools/issues/1117)) ([7527cff](https://github.com/wundergraph/graphql-go-tools/commit/7527cff29f41755ea341faa235e8f92781cea936))

## [2.0.0-rc.166](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.165...v2.0.0-rc.166) (2025-03-25)


### Bug Fixes

* catch an error on provides with fragments ([#1115](https://github.com/wundergraph/graphql-go-tools/issues/1115)) ([f4bb0af](https://github.com/wundergraph/graphql-go-tools/commit/f4bb0afa4b0124eff279635f7a0dc7d65d2e8554))

## [2.0.0-rc.165](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.164...v2.0.0-rc.165) (2025-03-14)


### Features

* add max concurrency for data source collectors ([#1111](https://github.com/wundergraph/graphql-go-tools/issues/1111)) ([bae36b2](https://github.com/wundergraph/graphql-go-tools/commit/bae36b241bcb72619af8b76f8410c7796e018a72))

## [2.0.0-rc.164](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.163...v2.0.0-rc.164) (2025-03-13)


### Features

* add error code for authorization errors ([#1109](https://github.com/wundergraph/graphql-go-tools/issues/1109)) ([54e744e](https://github.com/wundergraph/graphql-go-tools/commit/54e744e9843b87e02c3251c8d5262e55c588e89a))

## [2.0.0-rc.163](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.162...v2.0.0-rc.163) (2025-03-04)


### Bug Fixes

* invalid enum values with value completion flag ([#1104](https://github.com/wundergraph/graphql-go-tools/issues/1104)) ([714fb3e](https://github.com/wundergraph/graphql-go-tools/commit/714fb3e098c795f23ee6273b33af3524de67c4b0))

## [2.0.0-rc.162](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.161...v2.0.0-rc.162) (2025-03-03)


### Bug Fixes

* **subscription:** never try to send on blocked channel when subscription was completed ([#1100](https://github.com/wundergraph/graphql-go-tools/issues/1100)) ([1a1bb20](https://github.com/wundergraph/graphql-go-tools/commit/1a1bb20e4cc02888ff9780f0e38c27b09774e9d7))

## [2.0.0-rc.161](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.160...v2.0.0-rc.161) (2025-02-28)


### Bug Fixes

* fix validation of variables used in nested fields of type list of an input object ([#1099](https://github.com/wundergraph/graphql-go-tools/issues/1099)) ([d74dc37](https://github.com/wundergraph/graphql-go-tools/commit/d74dc37ea452f8e1a2d63a57ce4cab52c1b7ec66)), closes [#1096](https://github.com/wundergraph/graphql-go-tools/issues/1096)

## [2.0.0-rc.160](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.159...v2.0.0-rc.160) (2025-02-27)


### Bug Fixes

* **subscriptions:** skip event after worker shutdown ([#1094](https://github.com/wundergraph/graphql-go-tools/issues/1094)) ([c30d9d9](https://github.com/wundergraph/graphql-go-tools/commit/c30d9d9f4718266ffbe41b44e6d6c86b269f6810))

## [2.0.0-rc.159](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.158...v2.0.0-rc.159) (2025-02-26)


### Features

* support files in nested variable input ([#1095](https://github.com/wundergraph/graphql-go-tools/issues/1095)) ([88c583a](https://github.com/wundergraph/graphql-go-tools/commit/88c583ac7447a6a204b67857823d847ce66550c8))

## [2.0.0-rc.158](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.157...v2.0.0-rc.158) (2025-02-22)


### Bug Fixes

* fix node selections do not select external parents of unique node ([#1087](https://github.com/wundergraph/graphql-go-tools/issues/1087)) ([6adc0f6](https://github.com/wundergraph/graphql-go-tools/commit/6adc0f69b40d1aa6bf8bd660cfdea6327b93ce1b))

## [2.0.0-rc.157](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.156...v2.0.0-rc.157) (2025-02-20)


### Features

* **engine:** mutex free subscription handling ([#1076](https://github.com/wundergraph/graphql-go-tools/issues/1076)) ([21be4ab](https://github.com/wundergraph/graphql-go-tools/commit/21be4ab2fff9962d6f56b2bcb6d51b70a2651381))


### Bug Fixes

* fix values validation list compatibility check ([#1082](https://github.com/wundergraph/graphql-go-tools/issues/1082)) ([541be0d](https://github.com/wundergraph/graphql-go-tools/commit/541be0d07d200c79235c1af9b9c6cdf2f4870d65))

## [2.0.0-rc.156](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.155...v2.0.0-rc.156) (2025-02-18)


### Features

* apollo-router-like non-ok http status errors ([#1072](https://github.com/wundergraph/graphql-go-tools/issues/1072)) ([e685c29](https://github.com/wundergraph/graphql-go-tools/commit/e685c29331c0d1879ff8e099d4441047fbddf054))

## [2.0.0-rc.155](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.154...v2.0.0-rc.155) (2025-02-15)


### Bug Fixes

* deadlock when waiting on inflight events of a trigger ([#1073](https://github.com/wundergraph/graphql-go-tools/issues/1073)) ([8a2b33c](https://github.com/wundergraph/graphql-go-tools/commit/8a2b33c289a921f53518e795a205fba9d4bd7058))

## [2.0.0-rc.154](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.153...v2.0.0-rc.154) (2025-02-14)


### Bug Fixes

* use correct compatibility spelling ([#1070](https://github.com/wundergraph/graphql-go-tools/issues/1070)) ([9b3d93b](https://github.com/wundergraph/graphql-go-tools/commit/9b3d93b072169f84e41977d9091b1415c33b150d))

## [2.0.0-rc.153](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.152...v2.0.0-rc.153) (2025-02-14)


### Features

* add apollo router compat flag for invalid variable rendering ([#1067](https://github.com/wundergraph/graphql-go-tools/issues/1067)) ([e87961f](https://github.com/wundergraph/graphql-go-tools/commit/e87961fcd13f4dde76432745c564950f56f5045d))

## [2.0.0-rc.152](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.151...v2.0.0-rc.152) (2025-02-13)


### Bug Fixes

* fix printing object value with optional fields ([#1065](https://github.com/wundergraph/graphql-go-tools/issues/1065)) ([5730d72](https://github.com/wundergraph/graphql-go-tools/commit/5730d728f78dc64a10c00eb1de1cd00292ce7dd2))

## [2.0.0-rc.151](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.150...v2.0.0-rc.151) (2025-02-12)


### Bug Fixes

* get typename from upstream schema in abstract field rewriter ([#1062](https://github.com/wundergraph/graphql-go-tools/issues/1062)) ([59f0a51](https://github.com/wundergraph/graphql-go-tools/commit/59f0a5151b1a63d19c4655f016ca8316e6a5d36f))

## [2.0.0-rc.150](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.149...v2.0.0-rc.150) (2025-02-11)


### Bug Fixes

* interface objects ([#1055](https://github.com/wundergraph/graphql-go-tools/issues/1055)) ([858d929](https://github.com/wundergraph/graphql-go-tools/commit/858d92992680bd8652dde2d3bdd036dbc40608c5))
* re-walking operation on stopped walker after rewriting abstract selection set; prevent adding __typename field suggestion for the datasource which do not have a union defined ([cf50a60](https://github.com/wundergraph/graphql-go-tools/commit/cf50a60e520beeabb91b2bd99912dbe983634696))
* use of arguments on interface object when jumping from type to interface object ([850bd6c](https://github.com/wundergraph/graphql-go-tools/commit/850bd6ceeef4d01a0780eb5ac309eba3604cc871))
* use of arguments on jump to interface object from concrete type ([#1061](https://github.com/wundergraph/graphql-go-tools/issues/1061)) ([9f7180e](https://github.com/wundergraph/graphql-go-tools/commit/9f7180e9193b4be1f5f72a388f5e1f37f120fc39))

## [2.0.0-rc.149](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.148...v2.0.0-rc.149) (2025-02-07)


### Bug Fixes

* extracting object input with optional variable values ([#1056](https://github.com/wundergraph/graphql-go-tools/issues/1056)) ([3325eac](https://github.com/wundergraph/graphql-go-tools/commit/3325eac3f1dc70069e8057972bf1da5f7324402a))

## [2.0.0-rc.148](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.147...v2.0.0-rc.148) (2025-02-07)


### Bug Fixes

* populating nodes selection for the external path used in a key ([#1053](https://github.com/wundergraph/graphql-go-tools/issues/1053)) ([1cfc6f5](https://github.com/wundergraph/graphql-go-tools/commit/1cfc6f58b6fbe352cd8cecfa8ebc2fd48b7caccd))

## [2.0.0-rc.147](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.146...v2.0.0-rc.147) (2025-02-03)


### Bug Fixes

* wait for updates in flight to be delivered before shutting down the trigger ([#1048](https://github.com/wundergraph/graphql-go-tools/issues/1048)) ([2b44f78](https://github.com/wundergraph/graphql-go-tools/commit/2b44f7868029062a8e92fcde90febcfcd285520b))

## [2.0.0-rc.146](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.145...v2.0.0-rc.146) (2025-01-29)


### Bug Fixes

* heartbeat go routine gone rogue ([#1030](https://github.com/wundergraph/graphql-go-tools/issues/1030)) ([b7e96dd](https://github.com/wundergraph/graphql-go-tools/commit/b7e96ddf45ee87aae91b3d88ffa6910dc7460718))

## [2.0.0-rc.145](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.144...v2.0.0-rc.145) (2025-01-27)


### Features

* add normalizedQuery to query plan and request info to trace ([#1045](https://github.com/wundergraph/graphql-go-tools/issues/1045)) ([e75a1dd](https://github.com/wundergraph/graphql-go-tools/commit/e75a1dd24d5255b6cc990269c5c7922f851f4fc1))

## [2.0.0-rc.144](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.143...v2.0.0-rc.144) (2025-01-23)


### Bug Fixes

* remove semaphore from ResolveGraphQLSubscription ([#1043](https://github.com/wundergraph/graphql-go-tools/issues/1043)) ([76d644e](https://github.com/wundergraph/graphql-go-tools/commit/76d644eb2316bfc71ae3a09cd4a5614998f26f43))

## [2.0.0-rc.143](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.142...v2.0.0-rc.143) (2025-01-23)


### Bug Fixes

* delete leftover heartbeat connections ([#1033](https://github.com/wundergraph/graphql-go-tools/issues/1033)) ([f7492d3](https://github.com/wundergraph/graphql-go-tools/commit/f7492d39b044f4901f695fb1e7718c9fe912504c))

## [2.0.0-rc.142](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.141...v2.0.0-rc.142) (2025-01-19)


### Bug Fixes

* do not remap variable with Upload type ([#1040](https://github.com/wundergraph/graphql-go-tools/issues/1040)) ([d184d17](https://github.com/wundergraph/graphql-go-tools/commit/d184d174622ec25464915a318122ec99ef53a20b))

## [2.0.0-rc.141](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.140...v2.0.0-rc.141) (2025-01-19)


### Bug Fixes

* fix files upload remap ([#1038](https://github.com/wundergraph/graphql-go-tools/issues/1038)) ([09a2235](https://github.com/wundergraph/graphql-go-tools/commit/09a223574869ded8123a2464bd99af15523eb68a))

## [2.0.0-rc.140](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.139...v2.0.0-rc.140) (2025-01-19)


### Features

* implement variables mapper ([#1034](https://github.com/wundergraph/graphql-go-tools/issues/1034)) ([b020295](https://github.com/wundergraph/graphql-go-tools/commit/b02029576746bf5459fa1f00d04146308852ad73))

## [2.0.0-rc.139](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.138...v2.0.0-rc.139) (2025-01-08)


### Features

* add extensions.code to rate limiting error ([#1027](https://github.com/wundergraph/graphql-go-tools/issues/1027)) ([9423458](https://github.com/wundergraph/graphql-go-tools/commit/9423458b476545e417d7606f6371ce621b725674))

## [2.0.0-rc.138](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.137...v2.0.0-rc.138) (2025-01-03)


### Features

* add an option to omit variables content in the variables validator error messages ([#934](https://github.com/wundergraph/graphql-go-tools/issues/934)) ([369e031](https://github.com/wundergraph/graphql-go-tools/commit/369e031037f9c09c66b98285686c2ecb7362da95))
* add error cases when subgraph response cannot be merged ([#1025](https://github.com/wundergraph/graphql-go-tools/issues/1025)) ([c4f2f44](https://github.com/wundergraph/graphql-go-tools/commit/c4f2f44fc25a62fb2e8b3e82575ecd568036b59c))

## [2.0.0-rc.137](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.136...v2.0.0-rc.137) (2024-12-30)


### Features

* upgrade go to 1.23 ([#1020](https://github.com/wundergraph/graphql-go-tools/issues/1020)) ([ba20971](https://github.com/wundergraph/graphql-go-tools/commit/ba209713de5a98bff3b2778090fac66a0d4ece1e))


### Bug Fixes

* **astprinter:** implement transitive interface output ([#1021](https://github.com/wundergraph/graphql-go-tools/issues/1021)) ([1b7bac3](https://github.com/wundergraph/graphql-go-tools/commit/1b7bac3e96e0a6ce4563b4a4fe671a4073338128)), closes [#1018](https://github.com/wundergraph/graphql-go-tools/issues/1018)

## [2.0.0-rc.136](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.135...v2.0.0-rc.136) (2024-12-16)


### Bug Fixes

* set request in hook context before send request, in case of error ([#1016](https://github.com/wundergraph/graphql-go-tools/issues/1016)) ([e41bdef](https://github.com/wundergraph/graphql-go-tools/commit/e41bdef9779aae6bba3ac52b39f7dd545241e4ce))

## [2.0.0-rc.135](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.134...v2.0.0-rc.135) (2024-12-13)


### Features

* add ConsumerInactiveThreshold to NatsStreamConfiguration ([#1014](https://github.com/wundergraph/graphql-go-tools/issues/1014)) ([7d66579](https://github.com/wundergraph/graphql-go-tools/commit/7d66579b53d831a1b543f9ad20239d1fced303ab))

## [2.0.0-rc.134](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.133...v2.0.0-rc.134) (2024-12-11)


### Bug Fixes

* array merging logic ([abaa939](https://github.com/wundergraph/graphql-go-tools/commit/abaa939544c5c9ef954b68c7c7ceae37b304c6bb))
* array merging logic ([6bb4cb5](https://github.com/wundergraph/graphql-go-tools/commit/6bb4cb5eff53a7591a81392ba764edc7a81032d5))
* upgrade astjson, add deprecated hint on old package ([#1012](https://github.com/wundergraph/graphql-go-tools/issues/1012)) ([1f7ad31](https://github.com/wundergraph/graphql-go-tools/commit/1f7ad3163a891216b28e780640f2b8044be6d4aa))

## [2.0.0-rc.133](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.132...v2.0.0-rc.133) (2024-12-11)


### Features

* query plan for subscriptions ([#1008](https://github.com/wundergraph/graphql-go-tools/issues/1008)) ([34cc4fa](https://github.com/wundergraph/graphql-go-tools/commit/34cc4fa864ec9dd8f99c4a1d79814062847ba45b))


### Bug Fixes

* array merging logic ([abaa939](https://github.com/wundergraph/graphql-go-tools/commit/abaa939544c5c9ef954b68c7c7ceae37b304c6bb))
* array merging logic ([6bb4cb5](https://github.com/wundergraph/graphql-go-tools/commit/6bb4cb5eff53a7591a81392ba764edc7a81032d5))
* upgrade astjson, add deprecated hint on old package ([#1012](https://github.com/wundergraph/graphql-go-tools/issues/1012)) ([1f7ad31](https://github.com/wundergraph/graphql-go-tools/commit/1f7ad3163a891216b28e780640f2b8044be6d4aa))

## [2.0.0-rc.132](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.131...v2.0.0-rc.132) (2024-12-05)


### Features

* make multipart heartbeat configurable ([#1006](https://github.com/wundergraph/graphql-go-tools/issues/1006)) ([7675b4b](https://github.com/wundergraph/graphql-go-tools/commit/7675b4bbc23f815affb870c831747a5144176f0d))

## [2.0.0-rc.131](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.130...v2.0.0-rc.131) (2024-12-02)


### Features

* add http datasource onfinished hook  ([#1001](https://github.com/wundergraph/graphql-go-tools/issues/1001)) ([5d14a22](https://github.com/wundergraph/graphql-go-tools/commit/5d14a2233445fe02bc4afe92a06ad1954fc6f33a))

## [2.0.0-rc.130](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.129...v2.0.0-rc.130) (2024-12-02)


### Features

* execute subscription writes on main goroutine in synchronous resolve subscriptions ([acdaf47](https://github.com/wundergraph/graphql-go-tools/commit/acdaf47598762aa20712abd4eb38250bb10cfd33))

## [2.0.0-rc.129](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.128...v2.0.0-rc.129) (2024-11-27)


### Bug Fixes

* sanitize operation name ([#999](https://github.com/wundergraph/graphql-go-tools/issues/999)) ([344902a](https://github.com/wundergraph/graphql-go-tools/commit/344902a8b66ddfdbd50b2afacbfd73d63aa1a1fb))

## [2.0.0-rc.128](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.127...v2.0.0-rc.128) (2024-11-25)


### Features

* propagate operation name ([#993](https://github.com/wundergraph/graphql-go-tools/issues/993)) ([fe24f2b](https://github.com/wundergraph/graphql-go-tools/commit/fe24f2bfd5de1af07d26665f25600738b9355c6e))

## [2.0.0-rc.127](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.126...v2.0.0-rc.127) (2024-11-19)


### Bug Fixes

* **netpoll:** obtain fd correctly when dealing with tls.Conn ([#991](https://github.com/wundergraph/graphql-go-tools/issues/991)) ([7aa57f2](https://github.com/wundergraph/graphql-go-tools/commit/7aa57f2b964720ca26e67605eaed61b29eb93560))

## [2.0.0-rc.126](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.125...v2.0.0-rc.126) (2024-11-18)


### Bug Fixes

* fix regression on removing null variables which was undefined ([#988](https://github.com/wundergraph/graphql-go-tools/issues/988)) ([06d9407](https://github.com/wundergraph/graphql-go-tools/commit/06d9407beee3cd1c210948c4ddbf2b8c0214fe75))

## [2.0.0-rc.125](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.124...v2.0.0-rc.125) (2024-11-18)


### Features

* better epoll detection, allow to disable epoll ([#984](https://github.com/wundergraph/graphql-go-tools/issues/984)) ([bf93cf9](https://github.com/wundergraph/graphql-go-tools/commit/bf93cf9adc8d5abb685c580029a73328a43a96ae))

## [2.0.0-rc.124](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.123...v2.0.0-rc.124) (2024-11-16)


### Bug Fixes

* **ws:** deadlock on unsubscribe when epoll disabled ([#982](https://github.com/wundergraph/graphql-go-tools/issues/982)) ([2fad683](https://github.com/wundergraph/graphql-go-tools/commit/2fad6830a3630aafbfaaedd710ae7c27e0f341ee))

## [2.0.0-rc.123](https://github.com/wundergraph/graphql-go-tools/compare/v2.0.0-rc.122...v2.0.0-rc.123) (2024-11-14)


### Bug Fixes

* fix merging of response nodes of enum type ([6ea332d](https://github.com/wundergraph/graphql-go-tools/commit/6ea332de897903e19d7750dd28256d864426f9f0))
* fix merging of response nodes of enum type ([#978](https://github.com/wundergraph/graphql-go-tools/issues/978)) ([230d188](https://github.com/wundergraph/graphql-go-tools/commit/230d1884d8f4354e091d362a3da05cc376050042))
