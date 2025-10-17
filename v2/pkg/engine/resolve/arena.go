package resolve

import (
	"sync"
	"weak"

	"github.com/wundergraph/go-arena"
)

// ArenaPool provides a thread-safe pool of arena.Arena instances for memory-efficient allocations.
// It uses weak pointers to allow garbage collection of unused arenas while maintaining
// a pool of reusable arenas for high-frequency allocation patterns.
type ArenaPool struct {
	pool  []weak.Pointer[ArenaPoolItem]
	sizes map[uint64]int
	mu    sync.Mutex
}

// ArenaPoolItem wraps an arena.Arena for use in the pool
type ArenaPoolItem struct {
	Arena arena.Arena
}

// NewArenaPool creates a new ArenaPool instance
func NewArenaPool() *ArenaPool {
	return &ArenaPool{
		sizes: make(map[uint64]int),
	}
}

// Acquire gets an arena from the pool or creates a new one if none are available.
// The id parameter is used to track arena sizes per use case for optimization.
func (p *ArenaPool) Acquire(id uint64) *ArenaPoolItem {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try to find an available arena in the pool
	for i := 0; i < len(p.pool); i++ {
		v := p.pool[i].Value()
		p.pool = append(p.pool[:i], p.pool[i+1:]...)
		if v == nil {
			continue
		}
		return v
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
	p.sizes[id] = peak

	// Add the arena back to the pool using a weak pointer
	w := weak.Make(item)
	p.pool = append(p.pool, w)
}

// getArenaSize returns the optimal arena size for a given use case ID.
// If no size is recorded, it defaults to 1MB.
func (p *ArenaPool) getArenaSize(id uint64) int {
	if size, ok := p.sizes[id]; ok {
		return size
	}
	return 1024 * 1024 // Default 1MB
}
