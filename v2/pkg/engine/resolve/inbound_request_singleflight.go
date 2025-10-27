package resolve

import (
	"encoding/binary"
	"sync"

	"github.com/cespare/xxhash/v2"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// InboundRequestSingleFlight is a sharded goroutine safe single flight implementation to de-couple inbound requests
// It's taking into consideration the normalized operation hash, variables hash and headers hash
// making it robust against collisions
// for scalability, you can add more shards in case the mutexes are a bottleneck
type InboundRequestSingleFlight struct {
	shards []requestShard
}

type requestShard struct {
	mu sync.Mutex
	m  map[uint64]*InflightRequest
}

const defaultRequestSingleFlightShardCount = 4

// NewRequestSingleFlight creates a InboundRequestSingleFlight with the provided
// number of shards. If shardCount <= 0, the default of 4 is used.
func NewRequestSingleFlight(shardCount int) *InboundRequestSingleFlight {
	if shardCount <= 0 {
		shardCount = defaultRequestSingleFlightShardCount
	}
	r := &InboundRequestSingleFlight{
		shards: make([]requestShard, shardCount),
	}
	for i := range r.shards {
		r.shards[i] = requestShard{
			m: make(map[uint64]*InflightRequest),
		}
	}
	return r
}

type InflightRequest struct {
	Done         chan struct{}
	Data         []byte
	Err          error
	ID           uint64
	HasFollowers bool
}

// GetOrCreate creates a new InflightRequest or returns an existing (shared) one
// The first caller to create an InflightRequest for a given key is a leader, everyone else a follower
// GetOrCreate blocks until ctx.ctx.Done() returns or InflightRequest.Done is closed
// It returns an error if the leader returned an error
// It returns nil,nil if the inbound request is not eligible for request deduplication
func (r *InboundRequestSingleFlight) GetOrCreate(ctx *Context, response *GraphQLResponse) (*InflightRequest, error) {

	if ctx.ExecutionOptions.DisableRequestDeduplication {
		return nil, nil
	}

	if response != nil && response.Info != nil && response.Info.OperationType == ast.OperationTypeMutation {
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
	key := xxhash.Sum64(b[:])

	shard := r.shardFor(key)
	shard.mu.Lock()
	req, shared := shard.m[key]
	if shared {
		req.HasFollowers = true
		shard.mu.Unlock()
		select {
		case <-req.Done:
			if req.Err != nil {
				return nil, req.Err
			}
			return req, nil
		case <-ctx.ctx.Done():
			return nil, ctx.ctx.Err()
		}
	}

	req = &InflightRequest{
		Done: make(chan struct{}),
		ID:   key,
	}

	shard.m[key] = req
	shard.mu.Unlock()
	return req, nil
}

func (r *InboundRequestSingleFlight) FinishOk(req *InflightRequest, data []byte) {
	if req == nil {
		return
	}
	shard := r.shardFor(req.ID)
	shard.mu.Lock()
	delete(shard.m, req.ID)
	hasFollowers := req.HasFollowers
	shard.mu.Unlock()
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
	shard.mu.Lock()
	delete(shard.m, req.ID)
	shard.mu.Unlock()
	req.Err = err
	close(req.Done)
}

func (r *InboundRequestSingleFlight) shardFor(key uint64) *requestShard {
	// Fast modulo using power-of-two shard count if desired in the future.
	// For now, use standard modulo for clarity.
	idx := int(key % uint64(len(r.shards)))
	return &r.shards[idx]
}
