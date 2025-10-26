package resolve

import (
	"sync"

	"github.com/cespare/xxhash/v2"
)

type SingleFlight struct {
	mu      *sync.RWMutex
	items   map[uint64]*SingleFlightItem
	sizes   map[uint64]*fetchSize
	xxPool  *sync.Pool
	cleanup chan func()
}

// SingleFlightItem is used to communicate between leader and followers
// If an Item for a key doesn't exist, the leader creates and followers can join
type SingleFlightItem struct {
	// loaded will be closed by the leader to indicate to followers when the work is done
	loaded chan struct{}
	// response is the shared result, it must not be modified
	response []byte
	// err is non nil if the leader produced an error while doing the work
	err error
	// sizeHint keeps track of the last 50 responses per fetchKey to give an estimate on the size
	// this gives a leader a hint on how much space it should pre-allocate for buffers when fetching
	// this reduces memory usage
	sizeHint int
}

// fetchSize gives an estimate of required buffer size for a given fetchKey when dividing totalBytes / count
type fetchSize struct {
	// count is the number of fetches tracked
	count int
	// totalBytes is the cumulative bytes across tracked fetches
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

// GetOrCreateItem generates a single flight key (100% identical fetches) and a fetchKey (similar fetches, collisions possible but unproblematic)
// and return a SingleFlightItem as well as an indication if it's shared or not
// If shared == false, the caller is a leader
// If shared == true, the caller is a follower
// item.sizeHint can be used to create an optimal buffer for the fetch in case of a leader
// item.err must always be checked
// item.response must never be mutated
func (s *SingleFlight) GetOrCreateItem(fetchItem *FetchItem, input []byte, extraKey uint64) (sfKey, fetchKey uint64, item *SingleFlightItem, shared bool) {
	sfKey, fetchKey = s.keys(fetchItem, input, extraKey)

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
		// empty chan to indicate to all followers when we're done (close)
		loaded: make(chan struct{}),
	}
	if size, ok := s.sizes[fetchKey]; ok {
		item.sizeHint = size.totalBytes / size.count
	}
	s.items[sfKey] = item
	s.mu.Unlock()
	return sfKey, fetchKey, item, false
}

func (s *SingleFlight) keys(fetchItem *FetchItem, input []byte, extraKey uint64) (sfKey, fetchKey uint64) {
	h := s.xxPool.Get().(*xxhash.Digest)
	sfKey = s.sfKey(h, fetchItem, input, extraKey)
	h.Reset()
	fetchKey = s.fetchKey(h, fetchItem)
	h.Reset()
	s.xxPool.Put(h)
	return sfKey, fetchKey
}

// sfKey returns a key that 100% uniquely identifies a fetch with no collision
// two sfKey are only the same when the fetches are 100% equal
func (s *SingleFlight) sfKey(h *xxhash.Digest, fetchItem *FetchItem, input []byte, extraKey uint64) uint64 {
	if fetchItem != nil && fetchItem.Fetch != nil {
		info := fetchItem.Fetch.FetchInfo()
		if info != nil {
			_, _ = h.WriteString(info.DataSourceID)
			_, _ = h.WriteString(":")
		}
	}
	_, _ = h.Write(input)
	return h.Sum64() + extraKey // extraKey in this case is the pre-generated hash for the headers
}

// fetchKey is a less robust key compared to sfKey
// the purpose is to create a key from the DataSourceID and root fields to have less cardinality
// the goal is to get an estimate buffer size for similar fetches
// there's no point in hashing headers or the body for this purpose
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

// Finish is for the leader to mark the SingleFlightItem as "done"
// trigger all followers to look at the err & response of the item
// and to update the size estimates
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
