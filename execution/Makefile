.PHONY: test
test:
	# The idiomatic way to disable test caching explicitly is to use -count=1
	go test -race -count=1 ./...

.PHONY: test-quick
test-quick:
	go test -count=1 ./...

.PHONY: test-no-race
test-no-race:
	go test -count=1 ./...

# updateTestFixtures will update all! golden fixtures
.PHONY: updateTestFixtures
updateTestFixtures:
	GOLDIE_UPDATE=1 go test -count=1 ./engine/...

.PHONY: format
format:
	go fmt ./...

.PHONY: prepare-merge
prepare-merge: format test

.PHONY: ci
ci: test

.PHONY: ci-quick
ci-full: test-quick

.PHONY: update-deps
update-deps:
	go get github.com/wundergraph/graphql-go-tools/v2@upgrade
	go get github.com/wundergraph/cosmo/composition-go@upgrade
	go get github.com/wundergraph/cosmo/router@upgrade
	go mod tidy
	cd .. && go work sync
