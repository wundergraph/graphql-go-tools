# Ordered Concurrent Subscription Updates Design

## Goal

Restore concurrent resolution for per-subscriber subscription updates without allowing updates for one subscriber, filter errors, or terminal frames to be delivered out of order.

The change must be local to `graphql-go-tools`, retain the synchronous completion semantics of `SubscriptionUpdater.UpdateSubscription`, and avoid reinstating one long-lived worker goroutine per subscription.

## Background

`subscriptionUpdater.UpdateSubscription` currently holds one trigger-wide mutex while it filters, resolves, fetches subgraph data, writes, and flushes a subscriber update. A stream hook fan-out invokes this method concurrently for different subscribers, but the mutex serializes the complete operation. Identical subgraph fetches therefore never overlap and cannot share an in-flight response.

Simply releasing the mutex before resolution is insufficient. If ordering is established later with a mutex on `subscriptionState`, concurrent calls can reach that mutex in a different order from the order in which the updater admitted them. Filter errors and lifecycle operations can also bypass that mutex.

## Required Behavior

1. Updates admitted for distinct `SubscriptionIdentifier` values may resolve concurrently.
2. Updates admitted for the same `SubscriptionIdentifier` resolve and write in updater admission order.
3. `UpdateSubscription` returns only after its update has resolved or has been skipped.
4. Operations admitted through `subscriptionUpdater`—broadcast updates, explicit heartbeats, completion, terminal errors, per-subscription close, and `Done`—do not overtake already admitted per-subscriber updates.
5. The first completion or terminal error wins. Once admitted, later data, heartbeat, completion, and error frames are suppressed so nothing is delivered after a terminal frame.
6. `subscriptionUpdater.Done` waits for admitted updates before detaching the trigger. Direct resolver unsubscribe and shutdown paths retain their existing removal-based behavior described below.
7. A panic in one admitted update releases its lane and in-flight accounting before propagating, so later lifecycle operations cannot deadlock.

## Architecture

### Admission and per-subscriber lanes

`subscriptionUpdater` remains the ordering boundary. While holding its existing lifecycle mutex, `UpdateSubscription` records a new lane entry for the target subscription and increments an in-flight wait group. Recording the entry before releasing the mutex gives each call a deterministic admission position relative to every other updater operation.

Each lane entry contains a completion channel and references the previous entry's completion channel. After admission, the call releases the lifecycle mutex, waits for only its predecessor on the same lane, and invokes `handleUpdateSubscription`. Calls for other identifiers have independent predecessors and can proceed immediately.

`UpdateSubscription` itself does not spawn a goroutine. Concurrency comes from callers that already invoke different subscriber updates concurrently, and each caller continues to block until its own resolution completes.

### Lane bookkeeping

A separate lane mutex protects the map of current lane tails. The lock order is lifecycle mutex followed by lane mutex. Completion only needs the lane mutex, so lifecycle operations may safely hold the lifecycle mutex while waiting for all in-flight entries.

When an entry finishes, it closes its completion channel, removes itself from the tail map if it is still the current tail, and decrements the in-flight wait group. Cleanup runs in a defer so it also executes on panic.

Removing completed tails prevents the map from growing when subscriptions churn on a long-lived trigger.

### Lifecycle barriers

The updater's existing mutex continues to serialize admission of all public operations. Operations that previously relied on holding that mutex across synchronous resolution first wait for the per-subscriber in-flight wait group while still holding the mutex:

- `Update` waits, then performs its synchronous broadcast fan-out.
- `Heartbeat` waits, then sends heartbeats.
- `Complete` and `Error` mark the updater terminal, wait, then deliver the terminal signal.
- `Done` marks the updater done, waits, then detaches the trigger.
- `CloseSubscription` waits before removing the subscription.

Holding the lifecycle mutex while waiting is safe because update completion never reacquires it. It also prevents a concurrent `UpdateSubscription` from incrementing the wait group while a lifecycle operation is waiting.

The wait-group invariant is: every positive `Add` occurs while holding the lifecycle mutex and before publishing the lane entry; every operation that calls `Wait` holds the same mutex. Consequently, no `Add` can race with a zero-count `Wait` or begin until that wait and its lifecycle operation have returned.

The terminal state is distinct from `done`: `Complete` and `Error` stop new data admissions but leave cleanup to the required final `Done` call.

`CloseSubscription` deliberately waits for all currently admitted per-subscriber updates rather than only the target lane. This preserves the existing trigger-wide ordering of public updater calls and keeps this short-term change small. Closing a subscription is exceptional, so an unrelated slow subscriber delaying close is accepted here; a keyed lifecycle barrier belongs in a larger rewrite.

### Terminal operation matrix

- The first `Complete` or `Error` marks the updater terminal, waits for admitted updates, and emits its terminal frame.
- Later `Update`, `UpdateSubscription`, `Heartbeat`, `Complete`, and `Error` calls are no-ops.
- `CloseSubscription` remains allowed before `Done`; it performs subscription cleanup but cannot emit data.
- `Done` remains required and performs trigger cleanup exactly once.
- After `Done`, all updater methods except the read-only `Subscriptions` call are no-ops.

Each `subscriptionState` also records whether its downstream terminal frame has been written. `complete` and `error` set this state while holding `writeMu`; data, formatted errors, and heartbeats check it while holding the same mutex before writing. This closes the window between an updater terminal call and `Done`, including writes originating from resolver paths that do not pass through the updater.

### Resolver paths outside the updater

The resolver heartbeat loop calls `heartbeatTriggerSubscriptions` directly. It does not participate in updater admission and may run while an update is fetching, as it does today. `subscriptionState.writeMu` continues to make the eventual heartbeat or data write atomic, and the per-subscription terminal state prevents a heartbeat after a terminal frame. A heartbeat may still precede an update whose fetch is in flight; heartbeat-versus-in-flight-data ordering before termination is not part of this fix's data-ordering guarantee.

Direct unsubscribe, resolver shutdown, startup failure, and unsubscribe-on-flush-failure also bypass the updater barrier. They retain the existing removal protocol: removal marks `subscriptionState.removed`, detaches it from resolver indexes, and prevents an in-flight resolution from writing after removal. They must not wait on the updater in-flight group because unsubscribe-on-flush-failure can originate inside an admitted update and would deadlock by waiting for itself.

## Ordering Guarantee and Limitation

The engine guarantees updater admission order. It cannot reconstruct original broker order if an upstream hook runner invokes a newer event before an older event. In particular, a router timeout that abandons an older hook invocation can allow the next event's hook to call `UpdateSubscription` first.

Guaranteeing broker order across such timeouts requires the router to assign event sequence information before hook execution and carry it through to delivery, or to prevent overlapping event batches. That router-level change is outside this short-term fix.

## Error Handling

Filtering, resolution errors, authorization errors, and writes all execute inside the admitted lane entry, so they retain the same per-subscriber order as successful data frames. Existing error formatting and unsubscribe-on-flush-failure behavior remain unchanged.

While a queued entry waits for its lane predecessor, it also observes the updater context. If that context is cancelled first, the entry skips resolution and completes its lane bookkeeping, unblocking its successor. Once an entry starts resolving, cancellation continues to be handled by the existing resolution timeout and subscription contexts.

An admitted entry always completes its lane bookkeeping if resolution returns early or panics. A panic is not swallowed: cleanup runs and the panic continues to the caller, preserving existing panic behavior while preventing a stuck lane or lifecycle deadlock.

## Anonymous Regression Coverage

Tests will use generic subscription identifiers, event payloads, and a blocking in-memory data source. No customer names, schemas, headers, message-provider details, or measured production values will be included.

The tests will verify:

1. Two subscribers on one trigger both enter their subgraph loads before either load is released. This is the red regression test against the current trigger-wide serialization.
2. Two updates admitted for one subscriber cannot resolve or write in reverse order.
3. A filter error and a successful update for one subscriber retain their admission order.
4. The first terminal operation waits for admitted updates, later competing terminal operations are ignored, and later data is not admitted.
5. Periodic and explicit heartbeats cannot write after a terminal frame.
6. `Update`, `Heartbeat`, `Complete`, `Error`, `Done`, and `CloseSubscription` each wait for already admitted per-subscriber updates; dedicated assertions verify `Done` detachment and `CloseSubscription` removal.
7. A queued update cleans up promptly when the updater context is cancelled.
8. A panic releases its lane and in-flight accounting before propagating.
9. Direct unsubscribe during an in-flight update prevents the late write without waiting on the updater barrier.
10. Completed lane tails are removed after both single and overlapping same-subscriber updates.
11. Focused tests pass under the Go race detector.

## Non-Goals

- Rewriting the router subscription pipeline.
- Restoring per-subscription worker goroutines.
- Deduplicating non-identical subgraph requests.
- Guaranteeing broker order after the router has already reordered hook invocations.
- Changing public interfaces or configuration.
