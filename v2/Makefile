.PHONY: test
test:
	go test -race ./...

# First we test, then we benchmark. Otherwise we would get noisy output from testing logs.
.PHONY: test-quick
test-quick:
	go test ./...
	go test -run=^$$ -bench=. -benchtime=1x ./...

# update-fixtures will update all! golden fixtures
.PHONY: update-fixtures
update-fixtures:
	GOLDIE_UPDATE=1 go test -count=1 ./pkg/...

# golangci-lint will find the config file in the root dir of the repo.
.PHONY: format
format:
	golangci-lint fmt 

.PHONY: lint
lint:
	golangci-lint run 
	cd ../execution && make lint

.PHONY: lint-fix
lint-fix:
	golangci-lint run --fix
	cd ../execution && make lint-fix

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
