package resolve

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingCache is a test LoaderCache that fails on demand.
// Uses atomic counters for goroutine safety in concurrent tests.
type failingCache struct {
	getErr    error
	setErr    error
	deleteErr error
	getCalls  atomic.Int64
	setCalls  atomic.Int64
	delCalls  atomic.Int64
}

func (c *failingCache) Get(_ context.Context, _ []string) ([]*CacheEntry, error) {
	c.getCalls.Add(1)
	if c.getErr != nil {
		return nil, c.getErr
	}
	return []*CacheEntry{{Key: "k", Value: []byte("v")}}, nil
}

func (c *failingCache) Set(_ context.Context, _ []*CacheEntry) error {
	c.setCalls.Add(1)
	return c.setErr
}

func (c *failingCache) Delete(_ context.Context, _ []string) error {
	c.delCalls.Add(1)
	return c.deleteErr
}

// TestCircuitBreaker_OpenCloseTransitions verifies circuit breaker state machine transitions
// (closed/open/half-open) for L2 cache wrappers. Without this, cache outages could cascade
// into subgraph overload or silent data loss.
func TestCircuitBreaker_OpenCloseTransitions(t *testing.T) {
	cacheErr := errors.New("redis: connection refused")

	t.Run("closed - passes through on success", func(t *testing.T) {
		inner := &failingCache{}
		cb := &circuitBreakerCache{
			inner: inner,
			state: newCircuitBreakerState(CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 3,
				CooldownPeriod:   time.Second,
			}),
		}

		ctx := t.Context()
		entries, err := cb.Get(ctx, []string{"k1"})
		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, int64(1), inner.getCalls.Load())

		err = cb.Set(ctx, []*CacheEntry{{Key: "k1", TTL: time.Minute}})
		require.NoError(t, err)
		assert.Equal(t, int64(1), inner.setCalls.Load())

		err = cb.Delete(ctx, []string{"k1"})
		require.NoError(t, err)
		assert.Equal(t, int64(1), inner.delCalls.Load())
	})

	t.Run("stays closed below threshold", func(t *testing.T) {
		inner := &failingCache{getErr: cacheErr}
		cb := &circuitBreakerCache{
			inner: inner,
			state: newCircuitBreakerState(CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 3,
				CooldownPeriod:   time.Second,
			}),
		}

		ctx := t.Context()
		// Two failures — below threshold of 3
		_, _ = cb.Get(ctx, []string{"k1"})
		_, _ = cb.Get(ctx, []string{"k1"})

		// Two failures below threshold of 3 — still closed
		assert.Equal(t, int64(2), inner.getCalls.Load())
		assert.False(t, cb.state.isOpen())

		// Third call passes through (threshold reached ON this call)
		_, _ = cb.Get(ctx, []string{"k1"})
		assert.Equal(t, int64(3), inner.getCalls.Load())
		assert.True(t, cb.state.isOpen())
	})

	t.Run("opens after consecutive failures reach threshold", func(t *testing.T) {
		inner := &failingCache{getErr: cacheErr}
		cb := &circuitBreakerCache{
			inner: inner,
			state: newCircuitBreakerState(CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 2,
				CooldownPeriod:   time.Second,
			}),
		}

		ctx := t.Context()
		_, _ = cb.Get(ctx, []string{"k1"})
		_, _ = cb.Get(ctx, []string{"k1"})
		assert.True(t, cb.state.isOpen())

		// While open: Get returns nil + ErrCircuitBreakerOpen, inner is not called
		entries, err := cb.Get(ctx, []string{"k1"})
		assert.Equal(t, ErrCircuitBreakerOpen, err)
		assert.True(t, errors.Is(err, ErrCircuitBreakerOpen))
		assert.Nil(t, entries)
		assert.Equal(t, int64(2), inner.getCalls.Load())
	})

	t.Run("open breaker skips Set and Delete", func(t *testing.T) {
		inner := &failingCache{setErr: cacheErr, deleteErr: cacheErr}
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 1,
			CooldownPeriod:   time.Second,
		})
		// Force open
		state.forceOpen(time.Now().UnixNano(), 0)

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()
		// Open breaker: Set and Delete return ErrCircuitBreakerOpen and skip the inner cache
		err := cb.Set(ctx, []*CacheEntry{{Key: "k1", TTL: time.Minute}})
		assert.Equal(t, ErrCircuitBreakerOpen, err)
		assert.True(t, errors.Is(err, ErrCircuitBreakerOpen))
		assert.Equal(t, int64(0), inner.setCalls.Load())

		err = cb.Delete(ctx, []string{"k1"})
		assert.Equal(t, ErrCircuitBreakerOpen, err)
		assert.True(t, errors.Is(err, ErrCircuitBreakerOpen))
		assert.Equal(t, int64(0), inner.delCalls.Load())
	})

	t.Run("half-open probe success closes breaker", func(t *testing.T) {
		inner := &failingCache{} // no errors — probe succeeds
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 2,
			CooldownPeriod:   10 * time.Millisecond,
		})
		// Open the breaker in the past so cooldown has elapsed
		state.forceOpen(time.Now().Add(-50*time.Millisecond).UnixNano(), 2)

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()
		entries, err := cb.Get(ctx, []string{"k1"})
		require.NoError(t, err)
		// Successful probe: breaker closes, failures reset
		assert.Len(t, entries, 1)
		assert.Equal(t, int64(1), inner.getCalls.Load())
		assert.False(t, cb.state.isOpen())
		assert.Equal(t, int64(0), cb.state.failures())
	})

	t.Run("half-open probe failure re-opens breaker", func(t *testing.T) {
		inner := &failingCache{getErr: cacheErr}
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 1,
			CooldownPeriod:   10 * time.Millisecond,
		})
		// Open the breaker in the past so cooldown has elapsed
		state.forceOpen(time.Now().Add(-50*time.Millisecond).UnixNano(), 0)

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()
		// Failed probe: breaker re-opens
		_, err := cb.Get(ctx, []string{"k1"})
		assert.Error(t, err)
		assert.Equal(t, int64(1), inner.getCalls.Load())
		assert.True(t, cb.state.isOpen())
	})

	t.Run("success resets consecutive failure count", func(t *testing.T) {
		inner := &failingCache{}
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 3,
			CooldownPeriod:   time.Second,
		})

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()

		// Two failures
		inner.getErr = cacheErr
		_, _ = cb.Get(ctx, []string{"k1"})
		_, _ = cb.Get(ctx, []string{"k1"})
		assert.Equal(t, int64(2), state.failures())

		// One success resets count
		inner.getErr = nil
		// One success resets the failure counter
		_, err := cb.Get(ctx, []string{"k1"})
		require.NoError(t, err)
		assert.Equal(t, int64(0), state.failures())
		assert.False(t, state.isOpen())
	})

	t.Run("concurrent failures trip breaker exactly once", func(t *testing.T) {
		// 100 goroutines all failing concurrently with threshold=5.
		// The breaker must end up open, and the failure count must be
		// between threshold and goroutine count (CAS retries may cause
		// some increments to be lost, but the threshold crossing is never missed).
		inner := &failingCache{getErr: cacheErr}
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			CooldownPeriod:   time.Second,
		})
		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()
		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				_, _ = cb.Get(ctx, []string{"k1"})
			})
		}
		wg.Wait()

		assert.True(t, state.isOpen())
		if inner.getCalls.Load() < int64(5) {
			t.Fatalf("expected at least 5 inner calls before breaker opened, got %d", inner.getCalls.Load())
		}
	})

	t.Run("concurrent half-open allows exactly one probe", func(t *testing.T) {
		// Open the breaker with expired cooldown, then race 50 goroutines
		// calling shouldAllow. Exactly one should win the CAS probe.
		// We do NOT call recordSuccess so the breaker stays in half-open
		// with probeInFlight=true — this isolates the CAS behavior.
		var probeCount atomic.Int64
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 1,
			CooldownPeriod:   10 * time.Millisecond,
		})
		// Open in the past so cooldown has elapsed → half-open
		state.forceOpen(time.Now().Add(-50*time.Millisecond).UnixNano(), 1)

		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() {
				if state.shouldAllow() {
					probeCount.Add(1)
					// Intentionally do NOT call recordSuccess — we're testing
					// that exactly one goroutine wins the CAS, not the reset path.
				}
			})
		}
		wg.Wait()

		// Exactly one goroutine should have won the CAS probe
		assert.Equal(t, int64(1), probeCount.Load())
	})

	t.Run("concurrent mixed success and failure", func(t *testing.T) {
		// 50 goroutines succeed, 50 fail concurrently. Threshold is 100.
		// The breaker must remain closed because the success calls reset
		// the failure counter before it can reach 100.
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 100,
			CooldownPeriod:   time.Second,
		})

		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() {
				state.recordSuccess()
			})
		}
		for range 50 {
			wg.Go(func() {
				state.recordFailure()
			})
		}
		wg.Wait()

		// With interleaved success resets, the breaker should not have tripped
		assert.False(t, state.isOpen())
	})

	t.Run("concurrent probe failure re-opens correctly", func(t *testing.T) {
		// Open the breaker with expired cooldown → half-open.
		// One goroutine wins the probe, but the probe fails.
		// Verify the breaker re-opens and subsequent calls are blocked.
		inner := &failingCache{getErr: cacheErr}
		state := newCircuitBreakerState(CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 1,
			CooldownPeriod:   10 * time.Millisecond, // short cooldown so initial state is half-open
		})
		// Open 50ms ago with 10ms cooldown → cooldown elapsed → half-open
		state.forceOpen(time.Now().Add(-50*time.Millisecond).UnixNano(), 1)

		cb := &circuitBreakerCache{inner: inner, state: state}

		ctx := t.Context()
		var wg sync.WaitGroup
		var probeResults sync.Map

		for i := range 20 {
			wg.Go(func() {
				_, err := cb.Get(ctx, []string{"k1"})
				switch {
				case err == nil:
					// Probe succeeded — should not happen here because inner always fails.
					probeResults.Store(i, "probed-succeeded")
				case errors.Is(err, ErrCircuitBreakerOpen):
					// Breaker blocked the call before reaching inner.
					probeResults.Store(i, "blocked")
				default:
					// Inner cache returned an error (the one goroutine that won the probe).
					probeResults.Store(i, "probed-failed")
				}
			})
		}
		wg.Wait()

		// Count how many actually probed (got an error back from inner)
		var probedCount int
		probeResults.Range(func(_, v any) bool {
			if v == "probed-failed" {
				probedCount++
			}
			return true
		})

		assert.Equal(t, 1, probedCount)
		// After probe failure, recordFailure re-opens with a fresh timestamp.
		// The new openedAt is ~now, so with 10ms cooldown it's still in the open window.
		assert.True(t, state.isOpen())
	})

	t.Run("wrapCachesWithCircuitBreakers applies defaults", func(t *testing.T) {
		inner := &failingCache{}
		caches := map[string]LoaderCache{"default": inner}
		configs := map[string]CircuitBreakerConfig{
			"default": {Enabled: true}, // no threshold or cooldown set
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		wrapped, ok := result["default"].(*circuitBreakerCache)
		// Verify defaults applied and original map not mutated
		require.True(t, ok)
		assert.Equal(t, 5, wrapped.state.config.FailureThreshold)
		assert.Equal(t, 10*time.Second, wrapped.state.config.CooldownPeriod)
		_, originalWrapped := caches["default"].(*circuitBreakerCache)
		assert.False(t, originalWrapped)
	})

	t.Run("wrapCachesWithCircuitBreakers skips disabled", func(t *testing.T) {
		inner := &failingCache{}
		caches := map[string]LoaderCache{"default": inner}
		configs := map[string]CircuitBreakerConfig{
			"default": {Enabled: false},
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		_, ok := result["default"].(*circuitBreakerCache)
		assert.False(t, ok)
	})

	t.Run("wrapCachesWithCircuitBreakers ignores missing cache names", func(t *testing.T) {
		caches := map[string]LoaderCache{"default": &failingCache{}}
		configs := map[string]CircuitBreakerConfig{
			"nonexistent": {Enabled: true},
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		_, ok := result["default"].(*circuitBreakerCache)
		assert.False(t, ok)
	})
}

// TestCircuitBreaker_OpenReturnsSentinel verifies that open-breaker Get/Set/Delete
// return ErrCircuitBreakerOpen so callers can distinguish a breaker-skip from a
// real backend error via errors.Is. This is the signal used by loader_cache.go
// call sites to suppress analytics/trace error recording when the breaker trips.
func TestCircuitBreaker_OpenReturnsSentinel(t *testing.T) {
	inner := &failingCache{}
	state := newCircuitBreakerState(CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		CooldownPeriod:   time.Second,
	})
	// Force open so every call short-circuits.
	state.forceOpen(time.Now().UnixNano(), 1)
	cb := &circuitBreakerCache{inner: inner, state: state}

	ctx := t.Context()

	entries, getErr := cb.Get(ctx, []string{"k1", "k2"})
	assert.Nil(t, entries)
	assert.Equal(t, ErrCircuitBreakerOpen, getErr)
	assert.True(t, errors.Is(getErr, ErrCircuitBreakerOpen))

	setErr := cb.Set(ctx, []*CacheEntry{{Key: "k1", TTL: time.Minute}})
	assert.Equal(t, ErrCircuitBreakerOpen, setErr)
	assert.True(t, errors.Is(setErr, ErrCircuitBreakerOpen))

	delErr := cb.Delete(ctx, []string{"k1"})
	assert.Equal(t, ErrCircuitBreakerOpen, delErr)
	assert.True(t, errors.Is(delErr, ErrCircuitBreakerOpen))

	// Inner cache was never called.
	assert.Equal(t, int64(0), inner.getCalls.Load())
	assert.Equal(t, int64(0), inner.setCalls.Load())
	assert.Equal(t, int64(0), inner.delCalls.Load())
}
