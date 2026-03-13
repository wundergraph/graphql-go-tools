package resolve

import (
	"context"
	"sync/atomic"
	"time"
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

// circuitBreakerState tracks the state of one circuit breaker instance.
// All fields use atomic operations for goroutine safety (L2 operations run in Phase 2 goroutines).
//
// States:
//   - Closed: openedAt == 0. All operations pass through.
//   - Open: openedAt != 0 && now < openedAt + cooldown. All operations are skipped.
//   - Half-Open: openedAt != 0 && now >= openedAt + cooldown. One probe request allowed.
type circuitBreakerState struct {
	consecutiveFailures atomic.Int64
	openedAt            atomic.Int64 // unix nano timestamp, 0 = closed
	probeInFlight       atomic.Bool
	config              CircuitBreakerConfig
}

func newCircuitBreakerState(config CircuitBreakerConfig) *circuitBreakerState {
	return &circuitBreakerState{config: config}
}

// shouldAllow returns true if the operation should proceed.
// In half-open state, uses CAS to allow exactly one probe without clearing the
// open state — openedAt and consecutiveFailures are only reset on probe success.
func (cb *circuitBreakerState) shouldAllow() bool {
	openedAt := cb.openedAt.Load()
	if openedAt == 0 {
		return true // closed
	}

	elapsed := time.Since(time.Unix(0, openedAt))
	if elapsed < cb.config.CooldownPeriod {
		return false // open, cooldown not elapsed
	}

	// Half-open: allow exactly one probe, but don't mark the breaker closed
	// until that probe succeeds.
	return cb.probeInFlight.CompareAndSwap(false, true)
}

// recordSuccess resets the breaker to closed state.
func (cb *circuitBreakerState) recordSuccess() {
	cb.consecutiveFailures.Store(0)
	cb.openedAt.Store(0)
	cb.probeInFlight.Store(false)
}

// recordFailure increments the failure counter and trips the breaker if threshold is reached.
func (cb *circuitBreakerState) recordFailure() {
	if cb.probeInFlight.Swap(false) {
		// Half-open probe failed — reopen immediately.
		cb.openedAt.Store(time.Now().UnixNano())
		return
	}
	failures := cb.consecutiveFailures.Add(1)
	if failures >= int64(cb.config.FailureThreshold) {
		cb.openedAt.Store(time.Now().UnixNano())
	}
}

// isOpen returns true if the breaker is currently open (not allowing operations).
func (cb *circuitBreakerState) isOpen() bool {
	openedAt := cb.openedAt.Load()
	if openedAt == 0 {
		return false
	}
	elapsed := time.Since(time.Unix(0, openedAt))
	return elapsed < cb.config.CooldownPeriod
}

// circuitBreakerCache wraps a LoaderCache with circuit breaker protection.
// When the breaker is open:
//   - Get returns (nil, nil) — treated as all cache misses by existing code
//   - Set returns nil — same as current non-fatal error handling
//   - Delete returns nil — same as current non-fatal error handling
type circuitBreakerCache struct {
	inner LoaderCache
	state *circuitBreakerState
}

func (c *circuitBreakerCache) Get(ctx context.Context, keys []string) ([]*CacheEntry, error) {
	if !c.state.shouldAllow() {
		return nil, nil
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
		return nil
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
		return nil
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
	for name, cache := range caches {
		wrapped[name] = cache
	}
	for name, cbConfig := range configs {
		cache, ok := wrapped[name]
		if !ok || !cbConfig.Enabled {
			continue
		}
		if cbConfig.FailureThreshold <= 0 {
			cbConfig.FailureThreshold = 5
		}
		if cbConfig.CooldownPeriod <= 0 {
			cbConfig.CooldownPeriod = 10 * time.Second
		}
		wrapped[name] = &circuitBreakerCache{
			inner: cache,
			state: newCircuitBreakerState(cbConfig),
		}
	}
	return wrapped
}
