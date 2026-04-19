package resolve

import (
	"context"
	"errors"
	"maps"
	"sync/atomic"
	"time"
)

// ErrCircuitBreakerOpen is returned by the circuit breaker cache wrappers
// (Get / Set / Delete) when the breaker is open. It lets callers distinguish
// a breaker short-circuit from either a true backend error or a genuine cache
// miss. Callers that do not care can continue to treat any non-nil error as a
// soft failure; callers that want to suppress analytics noise from a breaker
// skip should check it with errors.Is.
var ErrCircuitBreakerOpen = errors.New("circuit breaker open")

// Default circuit breaker parameters applied by wrapCachesWithCircuitBreakers
// when CircuitBreakerConfig values are zero or unset.
const (
	// DefaultFailureThreshold is the number of consecutive failures that trips
	// the breaker when CircuitBreakerConfig.FailureThreshold is not set.
	DefaultFailureThreshold = 5
	// DefaultCooldownPeriod is how long the breaker stays open before allowing
	// a probe request when CircuitBreakerConfig.CooldownPeriod is not set.
	DefaultCooldownPeriod = 10 * time.Second
)

// CircuitBreakerConfig configures the L2 cache circuit breaker for a named cache instance.
// When the circuit is open, all L2 operations (Get/Set/Delete) are skipped and the engine
// falls back to subgraph fetches. This prevents cascading latency when the cache backend
// (e.g., Redis) is slow or unavailable.
type CircuitBreakerConfig struct {
	// Enabled activates the circuit breaker for this cache instance.
	Enabled bool

	// FailureThreshold is the number of consecutive failures that trips the breaker.
	// Default: 5
	FailureThreshold int

	// CooldownPeriod is how long the breaker stays open before allowing a probe request.
	// Default: 10s
	CooldownPeriod time.Duration
}

// cbSnapshot is the immutable state of a circuit breaker, swapped atomically.
// A single atomic.Pointer load on the fast path (closed state) avoids multiple
// atomic loads and ensures readers always see a consistent state.
type cbSnapshot struct {
	consecutiveFailures int64
	openedAt            int64 // unix nano timestamp, 0 = closed
	probeInFlight       bool
}

// closed is the shared zero-value snapshot for the closed state.
// Since snapshots are immutable, all closed breakers can share this pointer.
var closedSnapshot = &cbSnapshot{}

// circuitBreakerState tracks the state of one circuit breaker instance.
// State is stored as an immutable snapshot behind an atomic pointer, so all
// reads see a consistent view and the fast path (breaker closed) is a single
// atomic load + nil-like check.
//
// States:
//   - Closed: openedAt == 0. All operations pass through.
//   - Open: openedAt != 0 && now < openedAt + cooldown. All operations are skipped.
//   - Half-Open: openedAt != 0 && now >= openedAt + cooldown. One probe request allowed.
type circuitBreakerState struct {
	snap   atomic.Pointer[cbSnapshot]
	config CircuitBreakerConfig
}

func newCircuitBreakerState(config CircuitBreakerConfig) *circuitBreakerState {
	s := &circuitBreakerState{config: config}
	s.snap.Store(closedSnapshot)
	return s
}

// shouldAllow returns true if the operation should proceed.
// Fast path: single atomic load, check openedAt == 0.
// In half-open state, uses CAS on the snapshot pointer to allow exactly one probe.
func (cb *circuitBreakerState) shouldAllow() bool {
	snap := cb.snap.Load()
	if snap.openedAt == 0 {
		return true // closed — single atomic load on hot path
	}

	elapsed := time.Since(time.Unix(0, snap.openedAt))
	if elapsed < cb.config.CooldownPeriod {
		return false // open, cooldown not elapsed
	}

	// Half-open: allow exactly one probe via CAS on the snapshot pointer.
	// Only the goroutine that wins the CAS gets to probe.
	if snap.probeInFlight {
		return false // another probe already in flight
	}
	probing := &cbSnapshot{
		consecutiveFailures: snap.consecutiveFailures,
		openedAt:            snap.openedAt,
		probeInFlight:       true,
	}
	return cb.snap.CompareAndSwap(snap, probing)
}

// recordSuccess resets the breaker to closed state with a single atomic store.
func (cb *circuitBreakerState) recordSuccess() {
	snap := cb.snap.Load()
	if snap.openedAt == 0 && snap.consecutiveFailures == 0 {
		return // already closed — single atomic load on fast path
	}
	cb.snap.Store(closedSnapshot)
}

// recordFailure increments the failure counter and trips the breaker if threshold is reached.
func (cb *circuitBreakerState) recordFailure() {
	for {
		snap := cb.snap.Load()
		if snap.probeInFlight {
			// Half-open probe failed — reopen immediately with fresh timestamp.
			reopened := &cbSnapshot{
				consecutiveFailures: snap.consecutiveFailures,
				openedAt:            time.Now().UnixNano(),
			}
			if cb.snap.CompareAndSwap(snap, reopened) {
				return
			}
			continue // snapshot changed, retry
		}
		newFailures := snap.consecutiveFailures + 1
		next := &cbSnapshot{
			consecutiveFailures: newFailures,
			openedAt:            snap.openedAt,
		}
		if newFailures >= int64(cb.config.FailureThreshold) {
			next.openedAt = time.Now().UnixNano()
		}
		if cb.snap.CompareAndSwap(snap, next) {
			return
		}
		// snapshot changed concurrently, retry
	}
}

// isOpen returns true if the breaker is currently open (not allowing operations).
func (cb *circuitBreakerState) isOpen() bool {
	snap := cb.snap.Load()
	if snap.openedAt == 0 {
		return false
	}
	elapsed := time.Since(time.Unix(0, snap.openedAt))
	return elapsed < cb.config.CooldownPeriod
}

// forceOpen sets the breaker to open state with the given timestamp.
// Used only in tests to set up initial conditions.
func (cb *circuitBreakerState) forceOpen(openedAt int64, failures int64) {
	cb.snap.Store(&cbSnapshot{
		consecutiveFailures: failures,
		openedAt:            openedAt,
	})
}

// failures returns the current consecutive failure count. Used in tests.
func (cb *circuitBreakerState) failures() int64 {
	return cb.snap.Load().consecutiveFailures
}

// circuitBreakerCache wraps a LoaderCache with circuit breaker protection.
// When the breaker is open:
//   - Get returns (nil, ErrCircuitBreakerOpen) — callers treat via errors.Is as a clean skip
//   - Set returns ErrCircuitBreakerOpen — same, analytics should not record as a backend error
//   - Delete returns ErrCircuitBreakerOpen — same
//
// Returning the sentinel (instead of nil) preserves the "fall back to subgraph"
// behavior for callers that only check for a non-nil value/error, while letting
// callers that care distinguish a breaker-skip from a real backend failure.
// The sentinel is a package-level singleton so the open path stays allocation-free.
type circuitBreakerCache struct {
	inner LoaderCache
	state *circuitBreakerState
}

func (c *circuitBreakerCache) Get(ctx context.Context, keys []string) ([]*CacheEntry, error) {
	if !c.state.shouldAllow() {
		return nil, ErrCircuitBreakerOpen
	}
	entries, err := c.inner.Get(ctx, keys)
	if err != nil {
		c.state.recordFailure()
		return nil, err
	}
	c.state.recordSuccess()
	return entries, nil
}

func (c *circuitBreakerCache) Set(ctx context.Context, entries []*CacheEntry, ttl time.Duration) error {
	if !c.state.shouldAllow() {
		return ErrCircuitBreakerOpen
	}
	err := c.inner.Set(ctx, entries, ttl)
	if err != nil {
		c.state.recordFailure()
		return err
	}
	c.state.recordSuccess()
	return nil
}

func (c *circuitBreakerCache) Delete(ctx context.Context, keys []string) error {
	if !c.state.shouldAllow() {
		return ErrCircuitBreakerOpen
	}
	err := c.inner.Delete(ctx, keys)
	if err != nil {
		c.state.recordFailure()
		return err
	}
	c.state.recordSuccess()
	return nil
}

// wrapCachesWithCircuitBreakers returns a shallow copy of caches with circuit breaker
// wrappers applied where configured. The original map is not mutated.
// Called once during Resolver.New().
func wrapCachesWithCircuitBreakers(caches map[string]LoaderCache, configs map[string]CircuitBreakerConfig) map[string]LoaderCache {
	if caches == nil || configs == nil {
		return caches
	}
	wrapped := make(map[string]LoaderCache, len(caches))
	maps.Copy(wrapped, caches)
	for name, cbConfig := range configs {
		cache, ok := wrapped[name]
		if !ok || !cbConfig.Enabled {
			continue
		}
		if cbConfig.FailureThreshold <= 0 {
			cbConfig.FailureThreshold = DefaultFailureThreshold
		}
		if cbConfig.CooldownPeriod <= 0 {
			cbConfig.CooldownPeriod = DefaultCooldownPeriod
		}
		wrapped[name] = &circuitBreakerCache{
			inner: cache,
			state: newCircuitBreakerState(cbConfig),
		}
	}
	return wrapped
}
