# Changelog

## [1.1.0](https://github.com/wundergraph/graphql-go-tools/compare/v1.0.9...v1.1.0) (2024-11-13)


### Features

* add data source ID to trace ([#870](https://github.com/wundergraph/graphql-go-tools/issues/870)) ([beb8720](https://github.com/wundergraph/graphql-go-tools/commit/beb8720b423de3907c012e7c6ccfc12c03c26506))
* add further apollo-compatible error support ([#939](https://github.com/wundergraph/graphql-go-tools/issues/939)) ([2d08eb6](https://github.com/wundergraph/graphql-go-tools/commit/2d08eb6602571e9c12878be4f6bb82ecb2379d03))
* add query plans ([#871](https://github.com/wundergraph/graphql-go-tools/issues/871)) ([da79d7e](https://github.com/wundergraph/graphql-go-tools/commit/da79d7e8df4dc79506a901a6a0691c27b7b173b2))
* expose acquire resolver wait time in loader hooks ([#854](https://github.com/wundergraph/graphql-go-tools/issues/854)) ([b85148d](https://github.com/wundergraph/graphql-go-tools/commit/b85148dcb109b4bc1089ed6b27a7af8fce811494))
* expose compose method of engine federation config factory ([#878](https://github.com/wundergraph/graphql-go-tools/issues/878)) ([95e943e](https://github.com/wundergraph/graphql-go-tools/commit/95e943e83634482cc0d94b4c7f0a117d5f70dd82))
* improve performance and memory usage of loader & resolbable ([#851](https://github.com/wundergraph/graphql-go-tools/issues/851)) ([27670b7](https://github.com/wundergraph/graphql-go-tools/commit/27670b7fd55cb3a377c6bb7a89780b9b43d0bebb))
* improve resolve performance by solving merge abstract nodes in postprocessing ([#826](https://github.com/wundergraph/graphql-go-tools/issues/826)) ([6566e02](https://github.com/wundergraph/graphql-go-tools/commit/6566e023a0cc11833a21a2057259caeba69cacdc))
* include subgraph name in ART ([#929](https://github.com/wundergraph/graphql-go-tools/issues/929)) ([fc0993d](https://github.com/wundergraph/graphql-go-tools/commit/fc0993d6d757e395b95934794093ba1181609d04))
* rewrite variable renderer to use astjson ([#946](https://github.com/wundergraph/graphql-go-tools/issues/946)) ([0d2d642](https://github.com/wundergraph/graphql-go-tools/commit/0d2d64265c23f2286eb1b8562e68ad7c9491ed53))
* subgraph error propagation improvements ([#883](https://github.com/wundergraph/graphql-go-tools/issues/883)) ([13cb695](https://github.com/wundergraph/graphql-go-tools/commit/13cb69507d32a10203068d505bfa20afba7e3316))
* support multiple pubsub providers ([#788](https://github.com/wundergraph/graphql-go-tools/issues/788)) ([ea8b3d3](https://github.com/wundergraph/graphql-go-tools/commit/ea8b3d3e6447b2939980568b62a657b0c56926e5))
* validate returned enum values ([#936](https://github.com/wundergraph/graphql-go-tools/issues/936)) ([7aa4add](https://github.com/wundergraph/graphql-go-tools/commit/7aa4add94ea6033d1391ad1fa11bace9b670ae59))


### Bug Fixes

* argument and variable validation during execution ([#902](https://github.com/wundergraph/graphql-go-tools/issues/902)) ([895e332](https://github.com/wundergraph/graphql-go-tools/commit/895e3322c81b759176d44e58f6dbca06e8e5897c))
* correctly render trace and query plan together ([#874](https://github.com/wundergraph/graphql-go-tools/issues/874)) ([2fc364f](https://github.com/wundergraph/graphql-go-tools/commit/2fc364fd977ec21ee2a961a2f6d7c4eda7d65f89))
* execution validation order, do not reuse planner ([#925](https://github.com/wundergraph/graphql-go-tools/issues/925)) ([3ffce8b](https://github.com/wundergraph/graphql-go-tools/commit/3ffce8bfbff5b03ee052e5fd21d836ec075b0031))
* ignore empty errors ([#890](https://github.com/wundergraph/graphql-go-tools/issues/890)) ([4c5556f](https://github.com/wundergraph/graphql-go-tools/commit/4c5556f30c476dafc0a3ff34bba6bfdd93664c9f))
* improve ws subprotocol selection ([1fc0dd9](https://github.com/wundergraph/graphql-go-tools/commit/1fc0dd9b9a78e69c4831e379e3db548ece140d71))
* improve ws subprotocol selection ([#795](https://github.com/wundergraph/graphql-go-tools/issues/795)) ([ad67dbb](https://github.com/wundergraph/graphql-go-tools/commit/ad67dbb75b536fc628414584925c463c2f77405e))
* keep scalar order when merging fields in post processing ([#835](https://github.com/wundergraph/graphql-go-tools/issues/835)) ([d27fb6e](https://github.com/wundergraph/graphql-go-tools/commit/d27fb6ea477306a54d360cb5187de1c25de74824))
* keep unused variables during normalization ([#802](https://github.com/wundergraph/graphql-go-tools/issues/802)) ([15ae7b3](https://github.com/wundergraph/graphql-go-tools/commit/15ae7b30a58e4a66063f71e4992a19a5e6cf8fca))
* level of null data propagation ([#810](https://github.com/wundergraph/graphql-go-tools/issues/810)) ([537f4d6](https://github.com/wundergraph/graphql-go-tools/commit/537f4d6503a627a29691870dede91cb4b3d07124))
* merging fields correctly ([0dfb6a2](https://github.com/wundergraph/graphql-go-tools/commit/0dfb6a20f3c9af3866badf3f31aa3ff955e6b62b))
* merging fields correctly ([#836](https://github.com/wundergraph/graphql-go-tools/issues/836)) ([3c4cb17](https://github.com/wundergraph/graphql-go-tools/commit/3c4cb175dafb214644c3eee89960808e03924d54))
* merging response nodes ([#772](https://github.com/wundergraph/graphql-go-tools/issues/772)) ([5e89693](https://github.com/wundergraph/graphql-go-tools/commit/5e89693a57dd40b3cc58e2b0c35b02dd6099ee01))
* planning of provides, parent entity jump, conditional implicit keys, handling of external fields ([#818](https://github.com/wundergraph/graphql-go-tools/issues/818)) ([fe6ffd6](https://github.com/wundergraph/graphql-go-tools/commit/fe6ffd6b65949d6a4b9672ea06ca37c1c7e41f74)), closes [#830](https://github.com/wundergraph/graphql-go-tools/issues/830) [#847](https://github.com/wundergraph/graphql-go-tools/issues/847)
* return empty data when all root fields was skipped ([#910](https://github.com/wundergraph/graphql-go-tools/issues/910)) ([4607dc0](https://github.com/wundergraph/graphql-go-tools/commit/4607dc09a4633a8b577a1aca5e1d59f3378003f0))
* variables normalization for the anonymous operations ([#965](https://github.com/wundergraph/graphql-go-tools/issues/965)) ([267aef8](https://github.com/wundergraph/graphql-go-tools/commit/267aef8f74dcfcef8f01a3d64f883ce0d809f9de))
