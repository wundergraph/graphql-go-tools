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
	sizeHint int
}

type SingleFlight struct {
	mu      *sync.RWMutex
	items   map[uint64]*SingleFlightItem
	sizes   map[uint64]*fetchSize
	xxPool  *sync.Pool
	cleanup chan func()
}

type fetchSize struct {
	count      int
	totalBytes int
}

func NewSingleFlight() *SingleFlight {
	return &SingleFlight{
		items: make(map[uint64]*SingleFlightItem),
		sizes: make(map[uint64]*fetchSize),
		mu:    new(sync.RWMutex),
		xxPool: &sync.Pool{
			New: func() any {
				return xxhash.New()
			},
		},
		cleanup: make(chan func()),
	}
}

func (s *SingleFlight) GetOrCreateItem(ctx context.Context, fetchItem *FetchItem, input []byte) (sfKey, fetchKey uint64, item *SingleFlightItem, shared bool) {
	sfKey, fetchKey = s.keys(fetchItem, input)

	// First, try to get the item with a read lock
	s.mu.RLock()
	item, exists := s.items[sfKey]
	s.mu.RUnlock()
	if exists {
		return sfKey, fetchKey, item, true
	}

	// If not exists, acquire a write lock to create the item
	s.mu.Lock()
	// Double-check if the item was created while acquiring the write lock
	item, exists = s.items[sfKey]
	if exists {
		s.mu.Unlock()
		return sfKey, fetchKey, item, true
	}

	// Create a new item
	item = &SingleFlightItem{
		loaded: make(chan struct{}),
	}
	if size, ok := s.sizes[fetchKey]; ok {
		item.sizeHint = size.totalBytes / size.count
	}
	s.items[sfKey] = item
	s.mu.Unlock()
	return sfKey, fetchKey, item, false
}

func (s *SingleFlight) keys(fetchItem *FetchItem, input []byte) (sfKey, fetchKey uint64) {
	h := s.xxPool.Get().(*xxhash.Digest)
	sfKey = s.sfKey(h, fetchItem, input)
	h.Reset()
	fetchKey = s.fetchKey(h, fetchItem)
	h.Reset()
	s.xxPool.Put(h)
	return sfKey, fetchKey
}

func (s *SingleFlight) sfKey(h *xxhash.Digest, fetchItem *FetchItem, input []byte) uint64 {
	if fetchItem != nil && fetchItem.Fetch != nil {
		info := fetchItem.Fetch.FetchInfo()
		if info != nil {
			_, _ = h.WriteString(info.DataSourceID)
			_, _ = h.WriteString(":")
		}
	}
	_, _ = h.Write(input)
	return h.Sum64()
}

func (s *SingleFlight) fetchKey(h *xxhash.Digest, fetchItem *FetchItem) uint64 {
	if fetchItem == nil || fetchItem.Fetch == nil {
		return 0
	}
	info := fetchItem.Fetch.FetchInfo()
	if info == nil {
		return 0
	}
	_, _ = h.WriteString(info.DataSourceID)
	_, _ = h.WriteString("|")
	for i := range info.RootFields {
		if i != 0 {
			_, _ = h.WriteString(",")
		}
		_, _ = h.WriteString(info.RootFields[i].TypeName)
		_, _ = h.WriteString(".")
		_, _ = h.WriteString(info.RootFields[i].FieldName)
	}
	return h.Sum64()
}

func (s *SingleFlight) Finish(sfKey, fetchKey uint64, item *SingleFlightItem) {
	close(item.loaded)
	s.mu.Lock()
	delete(s.items, sfKey)
	if size, ok := s.sizes[fetchKey]; ok {
		if size.count == 50 {
			size.count = 1
			size.totalBytes = size.totalBytes / 50
		}
		size.count++
		size.totalBytes += len(item.response)
	} else {
		s.sizes[fetchKey] = &fetchSize{
			count:      1,
			totalBytes: len(item.response),
		}
	}
	s.mu.Unlock()
}
