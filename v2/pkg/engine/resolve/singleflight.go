package resolve

import (
	"context"
	"sync"

	"github.com/cespare/xxhash/v2"
)

type SingleFlightItem struct {
	loaded   chan struct{}
	response []byte
	err      error
}

type SingleFlight struct {
	mu      *sync.RWMutex
	items   map[uint64]*SingleFlightItem
	xxPool  *sync.Pool
	cleanup chan func()
}

func NewSingleFlight() *SingleFlight {
	return &SingleFlight{
		items: make(map[uint64]*SingleFlightItem),
		mu:    new(sync.RWMutex),
		xxPool: &sync.Pool{
			New: func() any {
				return xxhash.New()
			},
		},
		cleanup: make(chan func()),
	}
}

func (s *SingleFlight) GetOrCreateItem(ctx context.Context, fetchItem *FetchItem, input []byte) (key uint64, item *SingleFlightItem, shared bool) {
	key = s.key(fetchItem, input)

	// First, try to get the item with a read lock
	s.mu.RLock()
	item, exists := s.items[key]
	s.mu.RUnlock()
	if exists {
		return key, item, true
	}

	// If not exists, acquire a write lock to create the item
	s.mu.Lock()
	// Double-check if the item was created while acquiring the write lock
	item, exists = s.items[key]
	if exists {
		s.mu.Unlock()
		return key, item, true
	}

	// Create a new item
	item = &SingleFlightItem{
		loaded: make(chan struct{}),
	}
	s.items[key] = item
	s.mu.Unlock()
	return key, item, false
}

func (s *SingleFlight) key(fetchItem *FetchItem, input []byte) uint64 {
	h := s.xxPool.Get().(*xxhash.Digest)
	if fetchItem != nil && fetchItem.Fetch != nil {
		info := fetchItem.Fetch.FetchInfo()
		if info != nil {
			_, _ = h.WriteString(info.DataSourceID)
			_, _ = h.WriteString(":")
		}
	}
	_, _ = h.Write(input)
	key := h.Sum64()
	h.Reset()
	s.xxPool.Put(h)
	return key
}

func (s *SingleFlight) Finish(key uint64, item *SingleFlightItem) {
	close(item.loaded)
	s.mu.Lock()
	delete(s.items, key)
	s.mu.Unlock()
}
