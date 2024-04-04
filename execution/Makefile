.PHONY: test
test:
	go test ./...

.PHONY: test-quick
test-quick:
	go test -count=1 ./...

.PHONY: test-race
test-race:
	go test -race ./...

# updateTestFixtures will update all! golden fixtures
.PHONY: updateTestFixtures
updateTestFixtures:
	go test ./pkg/... -update

.PHONY: format
format:
	go fmt ./...

.PHONY: prepare-merge
prepare-merge: format test

.PHONY: ci
ci: test

.PHONY: ci-quick
ci-full: test-quick
