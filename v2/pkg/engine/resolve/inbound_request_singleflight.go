package resolve

import (
	"encoding/binary"
	"sync"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

// InboundRequestSingleFlight is a sharded goroutine safe single flight implementation to de-couple inbound requests
// to the GraphQL engine. Contrary to SubgraphRequestSingleFlight, this is not per-subgraph
// but global for all inbound requests.
// It's taking into consideration the normalized operation hash, variables hash and headers hash
// making it robust against collisions
// for scalability, you can add more shards in case the mutexes are a bottleneck
type InboundRequestSingleFlight struct {
	shards []requestShard
}

type requestShard struct {
	m sync.Map
}

const defaultRequestSingleFlightShardCount = 8

// NewRequestSingleFlight creates a InboundRequestSingleFlight with the provided
// number of shards. If shardCount <= 0, the default of 4 is used.
func NewRequestSingleFlight(shardCount int) *InboundRequestSingleFlight {
	if shardCount <= 0 {
		shardCount = defaultRequestSingleFlightShardCount
	}
	r := &InboundRequestSingleFlight{
		shards: make([]requestShard, shardCount),
	}
	return r
}

type InflightRequest struct {
	Done chan struct{}
	Data []byte
	Err  error
	ID   uint64

	HasFollowers bool
	Mu           sync.Mutex
}

// GetOrCreate creates a new InflightRequest or returns an existing (shared) one
// The first caller to create an InflightRequest for a given key is a leader, everyone else a follower
// GetOrCreate blocks until ctx.ctx.Done() returns or InflightRequest.Done is closed
// It returns an error if the leader returned an error
// It returns nil,nil if the inbound request is not eligible for request deduplication
// or if DisableInboundRequestDeduplication is set to true on Context
func (r *InboundRequestSingleFlight) GetOrCreate(ctx *Context, response *GraphQLResponse) (*InflightRequest, error) {

	if ctx.ExecutionOptions.DisableInboundRequestDeduplication {
		return nil, nil
	}

	if !response.SingleFlightAllowed() {
		return nil, nil
	}

	// Derive a robust key from request ID, variables hash and (optional) headers hash
	var b [24]byte
	binary.LittleEndian.PutUint64(b[0:8], ctx.Request.ID)
	binary.LittleEndian.PutUint64(b[8:16], ctx.VariablesHash)
	hh := uint64(0)
	if ctx.SubgraphHeadersBuilder != nil {
		hh = ctx.SubgraphHeadersBuilder.HashAll()
	}
	binary.LittleEndian.PutUint64(b[16:24], hh)
	h := pool.Hash64.Get()
	_, _ = h.Write(b[:])
	key := h.Sum64()
	pool.Hash64.Put(h)

	shard := r.shardFor(key)

	request := &InflightRequest{
		Done: make(chan struct{}),
		ID:   key,
	}

	inflight, shared := shard.m.LoadOrStore(key, request)
	if shared {
		request = inflight.(*InflightRequest)
		request.Mu.Lock()
		request.HasFollowers = true
		request.Mu.Unlock()
		select {
		case <-request.Done:
			if request.Err != nil {
				return nil, request.Err
			}
			return request, nil
		case <-ctx.ctx.Done():
			request.Err = ctx.ctx.Err()
			return nil, request.Err
		}
	}

	return request, nil
}

func (r *InboundRequestSingleFlight) FinishOk(req *InflightRequest, data []byte) {
	if req == nil {
		return
	}
	shard := r.shardFor(req.ID)
	shard.m.Delete(req.ID)
	req.Mu.Lock()
	hasFollowers := req.HasFollowers
	req.Mu.Unlock()
	if hasFollowers {
		// optimization to only copy when we actually have to
		req.Data = make([]byte, len(data))
		copy(req.Data, data)
	}
	close(req.Done)
}

func (r *InboundRequestSingleFlight) FinishErr(req *InflightRequest, err error) {
	if req == nil {
		return
	}
	shard := r.shardFor(req.ID)
	shard.m.Delete(req.ID)
	req.Err = err
	close(req.Done)
}

func (r *InboundRequestSingleFlight) shardFor(key uint64) *requestShard {
	// Fast modulo using power-of-two shard count if desired in the future.
	// For now, use standard modulo for clarity.
	idx := int(key % uint64(len(r.shards)))
	return &r.shards[idx]
}
