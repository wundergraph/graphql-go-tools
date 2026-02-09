# Caching Test Rules

## Always check every cache log

Every `defaultCache.ClearLog()` MUST be followed by `defaultCache.GetLog()` with full assertions BEFORE the next `ClearLog()` or end of test. Never clear a log without verifying its contents — skipped checks hide regressions.

```go
// CORRECT: every ClearLog has a corresponding GetLog + assertion
defaultCache.ClearLog()
resp := gqlClient.Query(...)
assert.Equal(t, expectedResp, string(resp))

logAfterFirst := defaultCache.GetLog()
wantLog := []CacheLogEntry{
    {Operation: "get", Keys: []string{`...`}, Hits: []bool{false}},
    {Operation: "set", Keys: []string{`...`}},
}
assert.Equal(t, sortCacheLogKeys(wantLog), sortCacheLogKeys(logAfterFirst), "descriptive message")

// WRONG: ClearLog without checking — hides bugs
defaultCache.ClearLog()
resp := gqlClient.Query(...)
assert.Equal(t, expectedResp, string(resp))
defaultCache.ClearLog() // previous log lost!
```