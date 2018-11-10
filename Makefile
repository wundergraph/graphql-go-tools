.PHONY: test
test:
	go test ./pkg/...

# updateTestFixtures will update all! golden fixtures
.PHONY: updateTestFixtures
updateTestFixtures:
	go test ./pkg/... -update

.PHONY: lint
lint:
	gometalinter --config ./gometalinter.json ./pkg/**

.PHONY: format
format:
	go fmt ./...

.PHONY: prepare-merge
prepare-merge: format test lint

HAS_GOMETALINTER := $(shell command -v gometalinter;)
HAS_DEP          := $(shell command -v dep;)
HAS_GIT          := $(shell command -v git;)

.PHONY: generate
generate: $(GOPATH)/src/github.com/abice/go-enum
	go generate ./...

$(GOPATH)/src/github.com/abice/go-enum:
	go get -u github.com/abice/go-enum

.PHONY: bootstrap
bootstrap:
ifndef HAS_GIT
	$(error You must install git)
endif
ifndef HAS_DEP
	go get -u github.com/golang/dep/cmd/dep
endif
ifndef HAS_GOMETALINTER
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install
endif
	dep ensure