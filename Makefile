GOLANG_CI_VERSION = "v1.21.0"
HAS_GOLANG_CI_LINT := $(shell command -v golangci-lint;)

.PHONY: test
test:
	go test ./...

# updateTestFixtures will update all! golden fixtures
.PHONY: updateTestFixtures
updateTestFixtures:
	go test ./pkg/... -update

.PHONY: lint
lint:
	golangci-lint run

.PHONY: format
format:
	go fmt ./...

.PHONY: prepare-merge
prepare-merge: format test lint

.PHONY: ci
ci: test lint

.PHONY: generate
generate: $(GOPATH)/bin/go-enum $(GOPATH)/bin/mockgen $(GOPATH)/bin/stringer
	go generate ./...

$(GOPATH)/bin/go-enum:
	go get -u github.com/abice/go-enum
	go install github.com/abice/go-enum

$(GOPATH)/bin/mockgen:
	go get -u github.com/golang/mock/gomock
	go install github.com/golang/mock/mockgen

$(GOPATH)/bin/stringer:
	go get -u -a golang.org/x/tools/cmd/stringer
	go install golang.org/x/tools/cmd/stringer

.PHONY: bootstrap
bootstrap:
ifndef HAS_GOLANG_CI_LINT
	go get github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANG_CI_VERSION}
	go mod tidy
endif
