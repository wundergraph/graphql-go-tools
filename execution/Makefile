.PHONY: test
test:
	# The idiomatic way to disable test caching explicitly is to use -count=1
	go test -race -count=1 ./...

.PHONY: test-quick
test-quick:
	go test -count=1 ./...
	go test -run=^$$ -bench=. -benchtime=1x ./...

# update-fixtures will update all! golden fixtures
.PHONY: update-fixtures
update-fixtures:
	GOLDIE_UPDATE=1 go test -count=1 ./engine/...

# golangci-lint will find the config file in the root dir of the repo.
.PHONY: format
format:
	golangci-lint fmt 

lint:
	golangci-lint run 

.PHONY: prepare-merge
prepare-merge: format test

.PHONY: update-deps
update-deps:
	go get github.com/wundergraph/graphql-go-tools/v2@upgrade
	go get github.com/wundergraph/cosmo/composition-go@upgrade
	go get github.com/wundergraph/cosmo/router@upgrade
	go mod tidy
	cd .. && go work sync
