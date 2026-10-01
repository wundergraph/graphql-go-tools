BENCH ?= Scenarios
COUNT ?= 10
BENCHTIME ?= 200ms

# Run the benchmarks of this package once.
.PHONY: bench
bench:
	go test -run='^$$' -bench='$(BENCH)' -benchmem -benchtime=$(BENCHTIME) .

# Compare the benchmarks against a base ref (default: previous v2.* tag).
# Example: make bench-compare BASE=origin/master COUNT=6
.PHONY: bench-compare
bench-compare:
	BASE=$(BASE) BENCH='$(BENCH)' COUNT=$(COUNT) BENCHTIME=$(BENCHTIME) OUT=$(OUT) ./bench-compare.sh
