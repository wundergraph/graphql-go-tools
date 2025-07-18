.PHONY: test
test:
	go test -race ./...

test-fresh: clean-testcache test

test-stability:
	@while $(MAKE) test-fresh; do :; done

# First we run tests, then benchmarks.
# If we run them together then we get noisy output from testing logs in data
.PHONY: test-quick
test-quick:
	go test ./...
	go test -run=xxxx -bench=. -benchtime=1x ./...

clean-testcache:
	go clean -testcache

# updateTestFixtures will update all! golden fixtures
.PHONY: updateTestFixtures
updateTestFixtures:
	go test ./pkg/... -update

.PHONY: format
format:
	go fmt ./...

.PHONY: prepare-merge
prepare-merge: format test

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
