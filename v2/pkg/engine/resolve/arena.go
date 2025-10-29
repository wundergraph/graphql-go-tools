package resolve

import (
	"sync"
	"weak"

	"github.com/wundergraph/go-arena"
)

// ArenaPool provides a thread-safe pool of arena.Arena instances for memory-efficient allocations.
// It uses weak pointers to allow garbage collection of unused arenas while maintaining
// a pool of reusable arenas for high-frequency allocation patterns.
//
// by storing ArenaPoolItem as weak pointers, the GC can collect them at any time
// before using an ArenaPoolItem, we try to get a strong pointer while removing it from the pool
// once we call Release, we turn the item back to the pool and make it a weak pointer again
// this means that at any time, GC can claim back the memory if required,
// allowing GC to automatically manage an appropriate pool size depending on available memory and GC pressure
type ArenaPool struct {
	// pool is a slice of weak pointers to the struct holding the arena.Arena
	pool  []weak.Pointer[ArenaPoolItem]
	sizes map[uint64]*arenaPoolItemSize
	mu    sync.Mutex
}

// arenaPoolItemSize is used to track the required memory across the last 50 arenas in the pool
type arenaPoolItemSize struct {
	count      int
	totalBytes int
}

// ArenaPoolItem wraps an arena.Arena for use in the pool
type ArenaPoolItem struct {
	Arena arena.Arena
}

// NewArenaPool creates a new ArenaPool instance
func NewArenaPool() *ArenaPool {
	return &ArenaPool{
		sizes: make(map[uint64]*arenaPoolItemSize),
	}
}

// Acquire gets an arena from the pool or creates a new one if none are available.
// The id parameter is used to track arena sizes per use case for optimization.
func (p *ArenaPool) Acquire(id uint64) *ArenaPoolItem {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try to find an available arena in the pool
	for len(p.pool) > 0 {
		// Pop the last item
		lastIdx := len(p.pool) - 1
		wp := p.pool[lastIdx]
		p.pool = p.pool[:lastIdx]

		v := wp.Value()
		if v != nil {
			return v
		}
		// If weak pointer was nil (GC collected), continue to next item
	}

	// No arena available, create a new one
	size := arena.WithMinBufferSize(p.getArenaSize(id))
	return &ArenaPoolItem{
		Arena: arena.NewMonotonicArena(size),
	}
}

// Release returns an arena to the pool for reuse.
// The peak memory usage is recorded to optimize future arena sizes for this use case.
func (p *ArenaPool) Release(id uint64, item *ArenaPoolItem) {
	peak := item.Arena.Peak()
	item.Arena.Reset()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Record the peak usage for this use case
	if size, ok := p.sizes[id]; ok {
		if size.count == 50 {
			size.count = 1
			size.totalBytes = size.totalBytes / 50
		}
		size.count++
		size.totalBytes += peak
	} else {
		p.sizes[id] = &arenaPoolItemSize{
			count:      1,
			totalBytes: peak,
		}
	}

	// Add the arena back to the pool using a weak pointer
	w := weak.Make(item)
	p.pool = append(p.pool, w)
}

// getArenaSize returns the optimal arena size for a given use case ID.
// If no size is recorded, it defaults to 1MB.
func (p *ArenaPool) getArenaSize(id uint64) int {
	if size, ok := p.sizes[id]; ok {
		return size.totalBytes / size.count
	}
	return 1024 * 1024 // Default 1MB
}
