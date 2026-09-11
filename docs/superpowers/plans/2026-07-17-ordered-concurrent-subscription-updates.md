# Ordered Concurrent Subscription Updates Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve per-subscriber updates concurrently across subscribers while preserving updater admission order within each subscriber and preventing writes after terminal frames.

**Architecture:** Add keyed predecessor/completion lanes at the `subscriptionUpdater` admission boundary. Existing callers remain synchronous, but concurrent callers for different subscription identifiers no longer share the trigger-wide mutex during resolution. Lifecycle methods use an in-flight barrier, while `subscriptionState` gains terminal-frame protection for write paths that bypass the updater.

**Tech Stack:** Go, `sync.Mutex`, `sync.WaitGroup`, channels, existing resolver subscription tests, Go race detector.

**Design:** `docs/superpowers/specs/2026-07-17-ordered-concurrent-subscription-updates-design.md`

---

## File Structure

- Create `v2/pkg/engine/resolve/subscription_updater_test.go`: focused anonymous regression tests and small test-only harnesses for updater admission, ordering, lifecycle, cancellation, and panic behavior.
- Modify `v2/pkg/engine/resolve/resolve.go`: keyed update lanes, updater lifecycle barriers, terminal updater state, and per-subscription terminal write protection.

No public interface, router code, configuration, or customer-specific fixture changes are required.

Command convention: run `go test` and final verification commands from `v2/`. Run `git`, implementation `gofmt`, and commit commands that use `v2/...` paths from the repository root.

## Chunk 1: Concurrent and Ordered Admission

### Task 1: Add the anonymous cross-subscriber regression test

**Files:**
- Create: `v2/pkg/engine/resolve/subscription_updater_test.go`

- [ ] **Step 1: Add a blocking test data source and resolver harness**

Create a `concurrentSubscriptionDataSource` that counts entries into `Load`, closes `allStarted` after the configured number of calls, and blocks on `release`. Implement both `Load` and `LoadWithFiles` using the same helper. Its `Release` method must use `sync.Once`, making explicit release, deferred failure cleanup, and harness cleanup safely idempotent.

Create `newSubscriptionUpdaterHarness(t, count, dataSource)` which:

1. Creates a real resolver with a long heartbeat interval and gives each subscription context `ExecutionOptions.DisableSubgraphRequestDeduplication = true`, ensuring the test observes two physical loads rather than one leader and one single-flight follower.
2. Builds one trigger and one `subscriptionUpdater`.
3. Registers `count` anonymous subscription states under distinct identifiers.
4. Gives every state a response whose event post-processing selects `data`, whose `SingleFetch` has static input `{}`, query operation metadata, data-source post-processing that selects `data`, and whose rendered object projects both event field `value` and fetched field `resolved`.
5. Registers the trigger and subscriptions in the resolver indexes.
6. Returns the updater, identifiers, recorders, and a cleanup function.

Use generic payloads such as `{"data":{"value":"event"}}` and `{"data":{"resolved":"value"}}`.

The cleanup function must release every blocking primitive, cancel the resolver, and wait with a timeout for all started updater calls. Recorder assertions must require the rendered output `{"data":{"value":"event","resolved":"value"}}`, proving the real fetch tree and render path ran.

- [ ] **Step 2: Write the failing concurrency test**

Add:

```go
func TestSubscriptionUpdater_UpdateSubscription_ResolvesDistinctSubscribersConcurrently(t *testing.T) {
    dataSource := newConcurrentSubscriptionDataSource(2)
    updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 2, dataSource)
    defer cleanup()
    defer dataSource.Release()

    var wg sync.WaitGroup
    for _, id := range ids {
        wg.Go(func() {
            updater.UpdateSubscription(id, []byte(`{"data":{"value":"event"}}`))
        })
    }

    select {
    case <-dataSource.AllStarted():
    case <-time.After(time.Second):
        t.Fatal("distinct subscribers did not enter their fetches concurrently")
    }

    dataSource.Release()
    wg.Wait()
}
```

- [ ] **Step 3: Run the test and verify RED**

Run:

```bash
go test ./pkg/engine/resolve -run TestSubscriptionUpdater_UpdateSubscription_ResolvesDistinctSubscribersConcurrently -count=1
```

from `v2/`.

Expected: FAIL with `distinct subscribers did not enter their fetches concurrently`. The defer must release the blocked first fetch so no goroutine leaks.

### Task 2: Implement keyed admission lanes

**Files:**
- Modify: `v2/pkg/engine/resolve/resolve.go` near `subscriptionUpdater`
- Test: `v2/pkg/engine/resolve/subscription_updater_test.go`

- [ ] **Step 1: Add lane state to `subscriptionUpdater`**

Add the following private state:

```go
updateMu    sync.Mutex
updateTails map[SubscriptionIdentifier]chan struct{}
updateWG    sync.WaitGroup
terminal    bool
```

`mu` remains the lifecycle/admission mutex. `updateMu` only protects tail publication and completion cleanup.

- [ ] **Step 2: Add admission and completion helpers**

Implement helpers equivalent to:

```go
func (s *subscriptionUpdater) admitSubscriptionUpdate(id SubscriptionIdentifier) (previous <-chan struct{}, current chan struct{}) {
    s.updateMu.Lock()
    defer s.updateMu.Unlock()
    if s.updateTails == nil {
        s.updateTails = make(map[SubscriptionIdentifier]chan struct{})
    }
    previous = s.updateTails[id]
    current = make(chan struct{})
    s.updateTails[id] = current
    s.updateWG.Add(1)
    return previous, current
}

func (s *subscriptionUpdater) finishSubscriptionUpdate(id SubscriptionIdentifier, current chan struct{}) {
    s.updateMu.Lock()
    close(current)
    if s.updateTails[id] == current {
        delete(s.updateTails, id)
    }
    s.updateMu.Unlock()
    s.updateWG.Done()
}
```

Document the lock order `mu -> updateMu`, and that `finishSubscriptionUpdate` never acquires `mu`.

- [ ] **Step 3: Change `UpdateSubscription` to admit under `mu` and resolve outside it**

The method must:

1. Lock `mu`.
2. Return if done, terminal, or the updater context is cancelled.
3. Admit the keyed entry and unlock `mu`.
4. Defer lane completion immediately.
5. Wait for the same-subscription predecessor, also selecting on updater-context cancellation.
6. Recheck updater-context cancellation.
7. Call `handleUpdateSubscription` synchronously.

Do not create a goroutine in the engine and do not add a mutex to `subscriptionState` for resolution.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run the Task 1 command again.

Expected: PASS; both data-source calls enter before release.

### Task 3: Lock down same-subscriber order and lane cleanup

**Files:**
- Modify: `v2/pkg/engine/resolve/subscription_updater_test.go`
- Modify if needed: `v2/pkg/engine/resolve/resolve.go`

- [ ] **Step 1: Add a same-subscriber FIFO test**

Use a first fetch that blocks and signals entry. Once it is active, lock `updateMu` and capture its current `updateTails[id]` channel. Admit a second update for the same identifier, then poll under `updateMu` with a one-second deadline until `updateTails[id]` differs from the captured first tail; this proves the second call was admitted rather than merely unscheduled. Assert the second fetch does not begin before the first is released, then assert recorder messages are exactly `first` followed by `second`.

Configure the response so `value` is projected from each event payload while the fetched `resolved` field is constant. Use payloads `{"data":{"value":"first"}}` and `{"data":{"value":"second"}}` so FIFO is visible at the writer.

- [ ] **Step 2: Add lane-tail cleanup assertions**

After one update and after two overlapping same-subscriber updates complete, lock `updateMu` in the test and require `updateTails` to be empty.

- [ ] **Step 3: Add cancellation and panic cleanup tests**

For cancellation, queue a second same-subscriber update behind a blocked first update, prove it was admitted by observing the replaced lane tail, cancel the updater context, and require the queued call to return within one second. Retain a deferred idempotent release for failure paths, but explicitly call `Release()` before joining the first call with a one-second deadline. Only after that join may the test verify final lane and in-flight cleanup.

For panic, use a test data source that panics from `Load`. Start a goroutine that invokes `UpdateSubscription` inside a closure with a deferred recover and sends the recovered value over a buffered channel; require the unchanged value within one second. Start a separate waiter goroutine for `updateWG.Wait()` and require it to return within one second before asserting the lane-tail map is empty. No test may call `Wait` directly on the test goroutine without a timeout.

- [ ] **Step 4: Add filter-error ordering coverage**

Add a synchronized test-only `VariableRenderer` that emits a matching static JSON value on its first invocation and returns a sentinel error on its second invocation. Filter templates receive the subscription context rather than event data, so invocation count—not payload inspection—selects the error. Use a chronological subscription writer whose single log records flushed data, formatted errors, terminal frames, and heartbeats in actual call order.

Admit a blocked successful update first, prove a filter-error update for the same subscriber has replaced the lane tail, and require the log to remain empty before releasing the first update. After release, require the log to contain the successful data frame followed by the formatted filter error.

- [ ] **Step 5: Run the admission test group**

Run:

```bash
go test ./pkg/engine/resolve -run 'TestSubscriptionUpdater_UpdateSubscription_' -count=1
```

Expected: PASS.

Name the FIFO, cancellation, panic, cleanup, and filter-error tests with the exact `TestSubscriptionUpdater_UpdateSubscription_` prefix so this command selects all of them.

- [ ] **Step 6: Commit the admission behavior**

```bash
gofmt -w v2/pkg/engine/resolve/resolve.go v2/pkg/engine/resolve/subscription_updater_test.go
git add v2/pkg/engine/resolve/resolve.go v2/pkg/engine/resolve/subscription_updater_test.go
git commit -m "fix(resolve): resolve subscriber updates concurrently"
```

## Chunk 2: Lifecycle and Terminal Ordering

### Task 4: Add updater lifecycle barriers

**Files:**
- Modify: `v2/pkg/engine/resolve/resolve.go`
- Modify: `v2/pkg/engine/resolve/subscription_updater_test.go`

- [ ] **Step 1: Add failing lifecycle-order tests**

Create table-driven subtests with isolated harnesses. Each subtest admits and blocks a per-subscriber update, defers the data source's idempotent release for failure cleanup, and exercises one public updater operation from another goroutine:

- `Update`
- `Heartbeat`
- `Complete`
- `Error`
- `CloseSubscription`
- `Done`

Before releasing the update, assert both that the lifecycle call has not returned and that its operation-specific effect has not occurred:

- `Update`: the broadcast's second fetch has not started.
- `Heartbeat`: no heartbeat is present in the chronological writer log.
- `Complete` and `Error`: no terminal frame is present.
- `CloseSubscription`: the target remains registered.
- `Done`: the trigger remains registered and the subscription completion channel remains open.

Explicitly release the blocked update, then require both the update goroutine and lifecycle goroutine to finish using bounded selects with a shared multi-second test deadline. After release, require the operation-specific effect. A lifecycle test must never use an unbounded `Wait` or rely only on a goroutine-return assertion.

For the heartbeat subtest, set `subscriptionState.heartbeat = true` and set the test resolver's `heartbeatInterval` to zero after construction. This makes the subscription eligible even though the completed update refreshes `lastWriteTime`, so the post-release assertion must observe exactly one heartbeat.

Add table-driven first-terminal-wins cases for both `Complete`-first and `Error`-first, each with at least two subscriptions. After the winning terminal call, calls to `Error`, `Complete`, `Update`, `UpdateSubscription`, and `Heartbeat` must not change either writer. `CloseSubscription` must remove one target after terminal state while leaving the trigger and second subscription registered. `Done` must then detach the trigger and complete the remaining subscription. After `Done`, invoke every mutating updater method and require all resolver and writer state to remain unchanged; `Subscriptions` remains read-only and callable.

Name these tests with the exact prefixes `TestSubscriptionUpdater_Lifecycle_` and `TestSubscriptionUpdater_Terminal_`.

- [ ] **Step 2: Verify the lifecycle tests fail against the partial implementation**

Run:

```bash
go test ./pkg/engine/resolve -run 'TestSubscriptionUpdater_(Lifecycle|Terminal)' -count=1
```

Expected: FAIL because methods other than `UpdateSubscription` do not yet wait on `updateWG` or enforce terminal state.

- [ ] **Step 3: Add lifecycle barriers**

While holding `subscriptionUpdater.mu`, call `updateWG.Wait()` before executing `Update`, `Heartbeat`, `Complete`, `Error`, `CloseSubscription`, or `Done`.

Apply these state rules:

- `Update`, `UpdateSubscription`, and `Heartbeat` return when `done`, `terminal`, or context-cancelled.
- `Complete` and `Error` return when `done`, `terminal`, or context-cancelled; the first accepted call sets `terminal = true` before waiting.
- `CloseSubscription` is allowed while terminal but returns when done or context-cancelled.
- `Done` sets `done = true` before waiting and always remains available after terminal.

Add a comment recording the `WaitGroup` invariant: every `Add` occurs under `mu`, and every `Wait` holds `mu`, so `Add` cannot race a zero-count `Wait`.

- [ ] **Step 4: Run the lifecycle tests and verify GREEN**

Run the Step 2 command again.

Expected: PASS.

### Task 5: Prevent writes after terminal frames

**Files:**
- Modify: `v2/pkg/engine/resolve/resolve.go` near `subscriptionState` and its write helpers
- Modify: `v2/pkg/engine/resolve/subscription_updater_test.go`

- [ ] **Step 1: Add failing terminal-writer tests**

Test `subscriptionState` directly with a recording writer:

1. Call `complete`, then require `sendHeartbeat` and `writeError` to produce no frames.
2. Call `error`, then require the same suppression.
3. Call completion/error repeatedly and require exactly one terminal frame.
4. Exercise a real late data path: start `executeSubscriptionUpdate` with its data source blocked, deliver `complete` or `error` while the load is blocked, release the load, join it with a bounded select, and require no data frame after the terminal frame. This directly tests the terminal check in the resolver's final write section.
5. Mark a heartbeat-enabled subscription terminal, then invoke `resolver.heartbeatTriggerSubscriptions` directly and require no heartbeat. This covers the periodic resolver path that bypasses `subscriptionUpdater`.
6. Start direct unsubscribe during an in-flight resolve and require that removal returns without waiting for the updater barrier and that the late resolve does not write. Use deferred idempotent release for failure cleanup, explicit release on the success path, and bounded joins.

Name state tests with `TestSubscriptionState_Terminal_`, the periodic path `TestResolver_TerminalHeartbeat_`, and unsubscribe `TestSubscriptionUpdater_DirectUnsubscribe_`.

- [ ] **Step 2: Verify RED**

Run:

```bash
go test ./pkg/engine/resolve -run 'TestSubscriptionState_Terminal_|TestResolver_TerminalHeartbeat_|TestSubscriptionUpdater_DirectUnsubscribe_' -count=1
```

Expected: FAIL because `subscriptionState` currently tracks only removal, not terminal delivery.

- [ ] **Step 3: Add per-subscription terminal state**

Add `terminal bool` to `subscriptionState`, guarded by `writeMu`.

- `complete` and `error` check `removed || terminal`, set terminal, then write the terminal frame.
- `writeError` and `sendHeartbeat` return without writing when removed or terminal.
- The final write section of `executeSubscriptionUpdate` returns when removed or terminal.

Keep all terminal reads and writes under `writeMu`; no new atomic or lock is needed.

- [ ] **Step 4: Run terminal and lifecycle tests**

Run the Step 2 command, followed by the Task 4 lifecycle command.

Expected: PASS.

- [ ] **Step 5: Commit lifecycle and terminal ordering**

```bash
gofmt -w v2/pkg/engine/resolve/resolve.go v2/pkg/engine/resolve/subscription_updater_test.go
git add v2/pkg/engine/resolve/resolve.go v2/pkg/engine/resolve/subscription_updater_test.go
git commit -m "fix(resolve): preserve subscription update ordering"
```

### Task 6: Full verification

**Files:**
- Verify: `v2/pkg/engine/resolve/resolve.go`
- Verify: `v2/pkg/engine/resolve/subscription_updater_test.go`

- [ ] **Step 1: Check formatting and inspect all task changes**

Run:

```bash
test -z "$(gofmt -d pkg/engine/resolve/resolve.go pkg/engine/resolve/subscription_updater_test.go)"
task_base=$(git merge-base HEAD origin/master)
git diff --check "$task_base"..HEAD
git diff --stat "$task_base"..HEAD
```

Expected: no formatting or whitespace errors; only the planned resolver, test, spec, and plan files differ from the original branch base.

- [ ] **Step 2: Run focused tests repeatedly**

```bash
go test ./pkg/engine/resolve -run 'TestSubscriptionUpdater_|TestSubscriptionState_Terminal_|TestResolver_TerminalHeartbeat_' -count=20
```

Expected: PASS across all repetitions.

- [ ] **Step 3: Run the focused race suite**

```bash
go test -race ./pkg/engine/resolve -run 'TestSubscriptionUpdater_|TestSubscriptionState_Terminal_|TestResolver_TerminalHeartbeat_' -count=1
```

Expected: PASS with no race reports.

- [ ] **Step 4: Run the complete resolve package**

```bash
go test ./pkg/engine/resolve -count=1
```

Expected: PASS.

- [ ] **Step 5: Run the complete resolve package under the race detector**

```bash
go test -race ./pkg/engine/resolve -count=1
```

Expected: PASS with no race reports.

- [ ] **Step 6: Review repository status and commits**

```bash
git status --short
git log --oneline -4
```

Expected: a clean worktree with the design commit, plan commit, and two implementation commits. Do not push or mutate any remote service without separate explicit approval.
