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

func (c *failingCache) Set(_ context.Context, _ []*CacheEntry, _ time.Duration) error {
	c.setCalls.Add(1)
	return c.setErr
}

func (c *failingCache) Delete(_ context.Context, _ []string) error {
	c.delCalls.Add(1)
	return c.deleteErr
}

func TestCircuitBreaker(t *testing.T) {
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

		err = cb.Set(ctx, []*CacheEntry{{Key: "k1"}}, time.Minute)
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

		assert.Equal(t, int64(2), inner.getCalls.Load(), "both calls should pass through")
		assert.False(t, cb.state.isOpen(), "breaker should remain closed")

		// Third call still passes through (threshold is reached ON this call)
		_, _ = cb.Get(ctx, []string{"k1"})
		assert.Equal(t, int64(3), inner.getCalls.Load(), "threshold call should pass through")
		assert.True(t, cb.state.isOpen(), "breaker should be open after reaching threshold")
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

		// While open, Get returns nil/nil, inner is not called
		entries, err := cb.Get(ctx, []string{"k1"})
		assert.NoError(t, err, "open breaker returns nil error")
		assert.Nil(t, entries, "open breaker returns nil entries (all-miss)")
		assert.Equal(t, int64(2), inner.getCalls.Load(), "inner should not be called when open")
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
		err := cb.Set(ctx, []*CacheEntry{{Key: "k1"}}, time.Minute)
		assert.NoError(t, err, "open breaker Set returns nil")
		assert.Equal(t, int64(0), inner.setCalls.Load(), "inner Set not called when open")

		err = cb.Delete(ctx, []string{"k1"})
		assert.NoError(t, err, "open breaker Delete returns nil")
		assert.Equal(t, int64(0), inner.delCalls.Load(), "inner Delete not called when open")
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
		assert.Len(t, entries, 1, "probe should return data")
		assert.Equal(t, int64(1), inner.getCalls.Load(), "probe should call inner")
		assert.False(t, cb.state.isOpen(), "breaker should be closed after successful probe")
		assert.Equal(t, int64(0), cb.state.failures(), "failures should be reset")
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
		_, err := cb.Get(ctx, []string{"k1"})
		assert.Error(t, err, "probe failure should return error")
		assert.Equal(t, int64(1), inner.getCalls.Load(), "probe should call inner")
		assert.True(t, cb.state.isOpen(), "breaker should re-open after failed probe")
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
		_, err := cb.Get(ctx, []string{"k1"})
		require.NoError(t, err)
		assert.Equal(t, int64(0), state.failures(), "success should reset failures")
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

		assert.True(t, state.isOpen(), "breaker must be open after 100 concurrent failures with threshold=5")
		// Some calls may have been blocked by the open breaker, so inner calls <= 100
		assert.LessOrEqual(t, inner.getCalls.Load(), int64(100))
		assert.GreaterOrEqual(t, inner.getCalls.Load(), int64(5), "at least threshold calls must have reached inner before breaker opened")
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
		assert.Equal(t, int64(1), probeCount.Load(), "exactly one probe should be allowed in half-open state")
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
		assert.False(t, state.isOpen(), "breaker should stay closed with mixed success/failure below effective threshold")
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
				if err != nil {
					probeResults.Store(i, "probed-failed")
				} else {
					probeResults.Store(i, "blocked")
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

		assert.Equal(t, 1, probedCount, "exactly one goroutine should have probed and failed")
		// After probe failure, recordFailure re-opens with a fresh timestamp.
		// The new openedAt is ~now, so with 10ms cooldown it's still in the open window.
		assert.True(t, state.isOpen(), "breaker must be re-opened after probe failure")
	})

	t.Run("wrapCachesWithCircuitBreakers applies defaults", func(t *testing.T) {
		inner := &failingCache{}
		caches := map[string]LoaderCache{"default": inner}
		configs := map[string]CircuitBreakerConfig{
			"default": {Enabled: true}, // no threshold or cooldown set
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		wrapped, ok := result["default"].(*circuitBreakerCache)
		require.True(t, ok, "cache should be wrapped")
		assert.Equal(t, 5, wrapped.state.config.FailureThreshold, "default threshold should be 5")
		assert.Equal(t, 10*time.Second, wrapped.state.config.CooldownPeriod, "default cooldown should be 10s")
		// Original map should not be mutated
		_, originalWrapped := caches["default"].(*circuitBreakerCache)
		assert.False(t, originalWrapped, "original map should not be mutated")
	})

	t.Run("wrapCachesWithCircuitBreakers skips disabled", func(t *testing.T) {
		inner := &failingCache{}
		caches := map[string]LoaderCache{"default": inner}
		configs := map[string]CircuitBreakerConfig{
			"default": {Enabled: false},
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		_, ok := result["default"].(*circuitBreakerCache)
		assert.False(t, ok, "disabled breaker should not wrap the cache")
	})

	t.Run("wrapCachesWithCircuitBreakers ignores missing cache names", func(t *testing.T) {
		caches := map[string]LoaderCache{"default": &failingCache{}}
		configs := map[string]CircuitBreakerConfig{
			"nonexistent": {Enabled: true},
		}

		result := wrapCachesWithCircuitBreakers(caches, configs)

		_, ok := result["default"].(*circuitBreakerCache)
		assert.False(t, ok, "unrelated cache should not be wrapped")
	})
}
