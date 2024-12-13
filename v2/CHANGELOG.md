# Changelog

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
