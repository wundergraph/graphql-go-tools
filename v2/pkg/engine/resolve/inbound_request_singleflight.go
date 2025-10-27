package resolve

import (
	"sync"

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

	// ctx.Request.ID is the unique ID of the normalized GraphQL document +1 (offset)
	key := ctx.Request.ID + 1
	// ctx.VariablesHash is the hash of the normalized variables from the client request
	// this makes the key unique across different variables
	key += ctx.VariablesHash + 1
	if ctx.SubgraphHeadersBuilder != nil {
		// ctx.SubgraphHeadersBuilder.HashAll() returns the hash of all headers that will be forwarded to all subgraphs
		// this makes the key unique across different client request headers, given that we forward them
		// we pre-compute all headers that will be forwarded to each subgraph
		// if we combine all the subgraph header hashes, the key will be stable across all headers
		key += ctx.SubgraphHeadersBuilder.HashAll()
	}

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
