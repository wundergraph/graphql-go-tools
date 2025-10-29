package resolve

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/go-arena"
)

func TestNewArenaPool(t *testing.T) {
	pool := NewArenaPool()

	require.NotNil(t, pool, "NewArenaPool returned nil")
	assert.Equal(t, 0, len(pool.pool), "expected empty pool")
	assert.Equal(t, 0, len(pool.sizes), "expected empty sizes map")
}

func TestArenaPool_Acquire_EmptyPool(t *testing.T) {
	pool := NewArenaPool()

	item := pool.Acquire(1)

	require.NotNil(t, item, "Acquire returned nil")
	assert.NotNil(t, item.Arena, "Arena is nil")

	// Verify we can use the arena
	buf := arena.NewArenaBuffer(item.Arena)
	_, err := buf.WriteString("test")
	assert.NoError(t, err)

	assert.Equal(t, 0, len(pool.pool), "pool should still be empty")
}

func TestArenaPool_ReleaseAndAcquire(t *testing.T) {
	pool := NewArenaPool()
	id := uint64(42)

	// Acquire first arena
	item1 := pool.Acquire(id)

	// Use the arena
	buf := arena.NewArenaBuffer(item1.Arena)
	_, err := buf.WriteString("test data")
	assert.NoError(t, err)

	// Release it
	pool.Release(id, item1)

	// Pool should have one item
	assert.Equal(t, 1, len(pool.pool), "expected pool to have 1 item")

	// Acquire from pool
	item2 := pool.Acquire(id)

	require.NotNil(t, item2, "Acquire returned nil")

	// Pool should be empty again
	assert.Equal(t, 0, len(pool.pool), "expected empty pool after acquire")

	// The acquired arena should be reset and usable
	buf2 := arena.NewArenaBuffer(item2.Arena)
	_, err = buf2.WriteString("new data")
	assert.NoError(t, err)

	assert.Equal(t, "new data", buf2.String())
}

func TestArenaPool_Acquire_ProvesBugFix(t *testing.T) {
	// This test specifically proves the bug fix works
	// Creates multiple items, clears some references, then acquires
	// to ensure all items are checked without skipping
	pool := NewArenaPool()
	id := uint64(800)

	numItems := 10
	items := make([]*ArenaPoolItem, numItems)

	// Acquire all items
	for i := 0; i < numItems; i++ {
		items[i] = pool.Acquire(id)
		buf := arena.NewArenaBuffer(items[i].Arena)
		_, err := buf.WriteString("item data")
		assert.NoError(t, err)
	}

	// Release all while keeping strong references
	for i := 0; i < numItems; i++ {
		pool.Release(id, items[i])
	}

	// Pool should have all items
	assert.Equal(t, numItems, len(pool.pool), "expected items in pool")

	// Clear every other item to simulate partial GC
	for i := 0; i < numItems; i += 2 {
		items[i] = nil
	}

	// Force GC
	runtime.GC()
	runtime.GC()

	// Acquire items - should process ALL items without skipping
	processed := 0
	acquired := 0

	for len(pool.pool) > 0 && processed < numItems*2 {
		poolSizeBefore := len(pool.pool)
		item := pool.Acquire(id)
		poolSizeAfter := len(pool.pool)
		processed++

		assert.Less(t, poolSizeAfter, poolSizeBefore, "Pool size did not decrease - item not removed properly!")

		if item != nil {
			acquired++
		}
	}

	// Pool should be empty
	assert.Equal(t, 0, len(pool.pool), "expected empty pool")
}

func TestArenaPool_Release_PeakTracking(t *testing.T) {
	pool := NewArenaPool()
	id := uint64(200)

	// First arena
	item1 := pool.Acquire(id)
	buf1 := arena.NewArenaBuffer(item1.Arena)
	_, err := buf1.WriteString("small")
	assert.NoError(t, err)

	peak1 := item1.Arena.Peak()
	assert.Equal(t, peak1, 5)

	pool.Release(id, item1)

	// Check that size was tracked
	size, exists := pool.sizes[id]
	require.True(t, exists, "size tracking not created")
	assert.Equal(t, 1, size.count, "expected count 1")

	// Second arena
	item2 := pool.Acquire(id)
	buf2 := arena.NewArenaBuffer(item2.Arena)
	_, err = buf2.WriteString("larger data")
	assert.NoError(t, err)

	pool.Release(id, item2)

	// Check updated tracking
	assert.Equal(t, 2, size.count, "expected count 2")
}

func TestArenaPool_GetArenaSize(t *testing.T) {
	pool := NewArenaPool()

	// Test default size for unknown ID
	size1 := pool.getArenaSize(999)
	expectedDefault := 1024 * 1024
	assert.Equal(t, expectedDefault, size1, "expected default size")

	// Test calculated size after usage
	id := uint64(400)
	item := pool.Acquire(id)
	buf := arena.NewArenaBuffer(item.Arena)
	_, err := buf.WriteString("some data")
	assert.NoError(t, err)
	pool.Release(id, item)

	size2 := pool.getArenaSize(id)
	assert.NotEqual(t, 0, size2, "expected non-zero size after usage")
}

func TestArenaPool_MultipleItemsInPool(t *testing.T) {
	pool := NewArenaPool()
	id := uint64(500)

	// Acquire multiple distinct items
	numItems := 3
	items := make([]*ArenaPoolItem, numItems)

	for i := 0; i < numItems; i++ {
		items[i] = pool.Acquire(id)
		buf := arena.NewArenaBuffer(items[i].Arena)
		_, err := buf.WriteString("data")
		assert.NoError(t, err)
	}

	// Release all while keeping references
	for i := 0; i < numItems; i++ {
		pool.Release(id, items[i])
	}

	// Should have all items in pool
	assert.Equal(t, numItems, len(pool.pool), "expected items in pool")

	// Acquire all back
	acquired := 0
	for len(pool.pool) > 0 {
		item := pool.Acquire(id)
		if item != nil {
			acquired++
		}
	}

	assert.Equal(t, numItems, acquired, "expected to acquire all items")
}

func TestArenaPool_Release_MovingWindow(t *testing.T) {
	pool := NewArenaPool()
	id := uint64(600)

	// Release exactly 50 items
	for i := 0; i < 50; i++ {
		item := pool.Acquire(id)
		buf := arena.NewArenaBuffer(item.Arena)
		_, err := buf.WriteString("test data")
		assert.NoError(t, err)
		pool.Release(id, item)
	}

	// After 50 releases, verify count and total
	size := pool.sizes[id]
	require.NotNil(t, size, "size tracking should exist")
	assert.Equal(t, 50, size.count, "expected count to be 50")

	totalBytesAfter50 := size.totalBytes

	// Release one more item to trigger the window reset
	item51 := pool.Acquire(id)
	buf51 := arena.NewArenaBuffer(item51.Arena)
	_, err := buf51.WriteString("test data")
	assert.NoError(t, err)
	peak51 := item51.Arena.Peak()
	pool.Release(id, item51)

	// After 51st release, verify the window was reset
	// count should be 2 (reset to 1, then incremented)
	// totalBytes should be (totalBytesAfter50 / 50) + peak51
	assert.Equal(t, 2, size.count, "expected count to be 2 after window reset")

	expectedTotalBytes := (totalBytesAfter50 / 50) + peak51
	assert.Equal(t, expectedTotalBytes, size.totalBytes, "expected totalBytes to be divided by 50 and new peak added")

	// Verify we can continue releasing and counting works correctly
	for i := 0; i < 10; i++ {
		item := pool.Acquire(id)
		buf := arena.NewArenaBuffer(item.Arena)
		_, err := buf.WriteString("more data")
		assert.NoError(t, err)
		pool.Release(id, item)
	}

	// After 10 more releases, count should be 12 (2 + 10)
	assert.Equal(t, 12, size.count, "expected count to continue incrementing after window reset")
}
