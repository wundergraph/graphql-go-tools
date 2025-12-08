package resolve

import (
	"encoding/binary"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// SubgraphRequestSingleFlight is a sharded, goroutine safe single flight implementation to de-duplicate subgraph requests
// It's hashing the input and adds the pre-computed subgraph headers hash to avoid collisions
// In addition to single flight, it provides size hints to create right-sized buffers for subgraph requests
type SubgraphRequestSingleFlight struct {
	shards []singleFlightShard
}

type singleFlightShard struct {
	items sync.Map // map[uint64]*SingleFlightItem
	sizes sync.Map // map[uint64]*fetchSize
}

const defaultSingleFlightShardCount = 4

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
	// SFKey uniquely identifies a single flight request
	SFKey uint64
	// FetchKey groups similar fetches for size hinting
	FetchKey uint64
}

// fetchSize gives an estimate of required buffer size for a given fetchKey when dividing totalBytes / count
type fetchSize struct {
	mu sync.Mutex
	// count is the number of fetches tracked
	count int
	// totalBytes is the cumulative bytes across tracked fetches
	totalBytes int
}

func NewSingleFlight(shardCount int) *SubgraphRequestSingleFlight {
	if shardCount <= 0 {
		shardCount = defaultSingleFlightShardCount
	}
	s := &SubgraphRequestSingleFlight{
		shards: make([]singleFlightShard, shardCount),
	}
	return s
}

// GetOrCreateItem returns a SingleFlightItem, which contains the single flight key (100% identical fetches),
// a fetchKey (similar fetches, collisions possible but unproblematic because it's only used for size hints),
// and an indication if it is shared or not.
// If not shared, the caller is a leader, otherwise it is a follower.
// item.sizeHint can be used to create an optimal buffer for the fetch in case of a leader.
// item.err must always be checked.
// item.response must never be mutated.
func (s *SubgraphRequestSingleFlight) GetOrCreateItem(fetchItem *FetchItem, input []byte, extraKey uint64) (item *SingleFlightItem, shared bool) {
	sfKey, fetchKey := s.computeKeys(fetchItem, input, extraKey)

	// Get shard based on sfKey for items
	shard := s.shardFor(sfKey)

	item = &SingleFlightItem{
		// empty chan to indicate to all followers when we're done (close)
		loaded:   make(chan struct{}),
		SFKey:    sfKey,
		FetchKey: fetchKey,
	}

	if existing, ok := shard.items.LoadOrStore(sfKey, item); ok {
		return existing.(*SingleFlightItem), true
	}
	// Read size hint from the same shard (both items and sizes use the same shard now)
	if sizeValue, ok := shard.sizes.Load(fetchKey); ok {
		size := sizeValue.(*fetchSize)
		size.mu.Lock()
		if size.count > 0 {
			item.sizeHint = size.totalBytes / size.count
		}
		size.mu.Unlock()
	}

	return item, false
}

// Finish is for the leader to mark the SingleFlightItem as "done"
// trigger all followers to look at the err & response of the item
// and to update the size estimates
func (s *SubgraphRequestSingleFlight) Finish(item *SingleFlightItem) {
	shard := s.shardFor(item.SFKey)
	shard.items.Delete(item.SFKey)
	close(item.loaded)

	sizeValue, ok := shard.sizes.Load(item.FetchKey)
	if !ok {
		newSize := &fetchSize{}
		sizeValue, _ = shard.sizes.LoadOrStore(item.FetchKey, newSize)
	}
	size := sizeValue.(*fetchSize)
	size.mu.Lock()
	if size.count == 0 {
		size.count = 1
		size.totalBytes = len(item.response)
		size.mu.Unlock()
		return
	}
	if size.count == 50 {
		size.count = 1
		size.totalBytes = size.totalBytes / 50
	}
	size.count++
	size.totalBytes += len(item.response)
	size.mu.Unlock()
}

func (s *SubgraphRequestSingleFlight) shardFor(key uint64) *singleFlightShard {
	idx := int(key % uint64(len(s.shards)))
	return &s.shards[idx]
}

func (s *SubgraphRequestSingleFlight) computeKeys(fetchItem *FetchItem, input []byte, extraKey uint64) (sfKey, fetchKey uint64) {
	h := pool.Hash64.Get()
	sfKey = s.computeSFKey(h, fetchItem, input, extraKey)
	h.Reset()
	fetchKey = s.computeFetchKey(h, fetchItem)
	pool.Hash64.Put(h)
	return sfKey, fetchKey
}

// computeSFKey returns a key that 100% uniquely identifies a fetch with no collision.
// Two sfKey values are only the same when the fetches are 100% equal.
func (s *SubgraphRequestSingleFlight) computeSFKey(h *xxhash.Digest, fetchItem *FetchItem, input []byte, extraKey uint64) uint64 {
	if fetchItem != nil && fetchItem.Fetch != nil {
		info := fetchItem.Fetch.FetchInfo()
		if info != nil {
			_, _ = h.WriteString(info.DataSourceID)
			_, _ = h.WriteString(":")
		}
	}
	_, _ = h.Write(input)
	if extraKey != 0 {
		// include pre-computed headers hash to avoid collisions
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[0:8], extraKey)
		_, _ = h.Write(buf[:])
	}
	return h.Sum64()
}

// computeFetchKey is a less robust key compared to sfKey.
// The purpose is to create a key from the DataSourceID and root fields to have less cardinality.
// The goal is to get an estimate buffer size for similar fetches; hashing headers or the body is not needed.
func (s *SubgraphRequestSingleFlight) computeFetchKey(h *xxhash.Digest, fetchItem *FetchItem) uint64 {
	if fetchItem == nil || fetchItem.Fetch == nil {
		return 0
	}
	info := fetchItem.Fetch.FetchInfo()
	if info == nil {
		return 0
	}
	_, _ = h.WriteString(info.DataSourceID)
	_, _ = h.Write(pipe)
	for i := range info.RootFields {
		if i != 0 {
			_, _ = h.Write(comma)
		}
		_, _ = h.WriteString(info.RootFields[i].TypeName)
		_, _ = h.Write(dot)
		_, _ = h.WriteString(info.RootFields[i].FieldName)
	}
	sum := h.Sum64()
	return sum
}
