# Changelog

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
