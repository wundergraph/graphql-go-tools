ifeq ($(CI),true)
 FORMAT := github-actions
else
 FORMAT := pkgname
endif

GO_TOOL := go tool

.PHONY: test
test:
	$(GO_TOOL) gotestsum -f $(FORMAT) --format-hide-empty-pkg --hide-summary=skipped -- -race ./...

# First we test, then we benchmark. Otherwise we would get noisy output from testing logs.
.PHONY: test-quick
test-quick:
	$(GO_TOOL) gotestsum -f $(FORMAT) --format-hide-empty-pkg --hide-summary=skipped -- ./...
	go test -run=^$$ -bench=. -benchtime=1x ./...

# update-fixtures will update all! golden fixtures
.PHONY: update-fixtures
update-fixtures:
	GOLDIE_UPDATE=1 go test -count=1 ./pkg/...

# golangci-lint will find the config file in the root dir of the repo.
.PHONY: format
format:
	$(GO_TOOL) golangci-lint fmt

.PHONY: lint
lint:
	$(GO_TOOL) golangci-lint run
	cd ../execution && make lint

.PHONY: lint-fix
lint-fix:
	$(GO_TOOL) golangci-lint run --fix
	cd ../execution && make lint-fix

.PHONY: prepare-merge
prepare-merge: format test

.PHONY: generate
generate:
	go generate ./...
	go mod tidy
