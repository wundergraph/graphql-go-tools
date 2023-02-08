GOLANG_CI_VERSION = "v1.51.1"
GOLANG_CI_VERSION_SHORT = "1.51.1"
HAS_GOLANG_CI_LINT := $(shell command -v /tmp/ci/golangci-lint;)
INSTALLED_VERSION := $(shell command -v /tmp/ci/golangci-lint version;)
HAS_CORRECT_VERSION := $(shell command -v if [[ $(INSTALLED_VERSION) == *$(GOLANG_CI_VERSION_SHORT)* ]]; echo "OK" fi)

.PHONY: bootstrap

.PHONY: test
test:
	go test --short -count=1 ./...

.PHONY: test-full
test-full:
	go test -count=1 ./...

# updateTestFixtures will update all! golden fixtures
.PHONY: updateTestFixtures
updateTestFixtures:
	go test ./pkg/... -update

.PHONY: lint
lint:
	/tmp/ci/golangci-lint run

.PHONY: format
format:
	go fmt ./...

.PHONY: prepare-merge
prepare-merge: format test lint

.PHONY: ci
ci: bootstrap test lint

.PHONY: ci-full
ci-full: bootstrap test-full lint

.PHONY: generate
generate: $(GOPATH)/bin/go-enum $(GOPATH)/bin/mockgen $(GOPATH)/bin/stringer
	go generate ./...
	go mod tidy

$(GOPATH)/bin/go-enum:
	go get -u github.com/abice/go-enum
	go install github.com/abice/go-enum

$(GOPATH)/bin/mockgen:
	go get -u github.com/golang/mock/gomock
	go install github.com/golang/mock/mockgen

$(GOPATH)/bin/stringer:
	go get -u -a golang.org/x/tools/cmd/stringer
	go install golang.org/x/tools/cmd/stringer

bootstrap:
ifndef HAS_GOLANG_CI_LINT
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b /tmp/ci ${GOLANG_CI_VERSION}
endif

updateci:
	rm /tmp/ci/golangci-lint
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b /tmp/ci ${GOLANG_CI_VERSION}
