# Changelog

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
