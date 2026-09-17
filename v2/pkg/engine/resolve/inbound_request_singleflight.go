package resolve

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"sync/atomic"

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

// inboundRequestKey is the complete discriminator of one inbound request:
// operation, variables, headers and private identity. It is the map identity
// as is, so two requests share a flight only when every part matches.
type inboundRequestKey [24 + sha256.Size]byte

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
	// SharedData carries opaque state from the leader to followers (e.g. accumulated
	// response headers). Set by the leader via Context.GetDeduplicationData, read by
	// followers via Context.SetDeduplicationData. Typed as "any" because the resolve
	// package is data-agnostic — the caller decides the concrete type.
	SharedData any
	// SurrogateKeys are the leader's response cache surrogate keys, so a follower
	// serving the same body sends the same header.
	SurrogateKeys []string
	Err           error
	ID            inboundRequestKey

	followerCount atomic.Int32
}

func (r *InflightRequest) AddFollower() {
	r.followerCount.Add(1)
}

func (r *InflightRequest) HasFollowers() bool {
	return r.followerCount.Load() > 0
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

	// Derive a robust key from request ID, variables hash, (optional) headers hash
	// and the response cache user id (if present)
	var b inboundRequestKey
	binary.LittleEndian.PutUint64(b[0:8], ctx.Request.ID)
	binary.LittleEndian.PutUint64(b[8:16], ctx.VariablesHash)
	hh := uint64(0)
	if ctx.SubgraphHeadersBuilder != nil {
		hh = ctx.SubgraphHeadersBuilder.HashAll()
	}
	binary.LittleEndian.PutUint64(b[16:24], hh)
	if privateID, ok := ctx.responseCachePrivateID(); ok {
		copy(b[24:], privateID[:])
	}

	shard := r.shardFor(b)

	request := &InflightRequest{
		Done: make(chan struct{}),
		ID:   b,
	}

	inflight, shared := shard.m.LoadOrStore(b, request)
	if shared {
		request = inflight.(*InflightRequest)
		request.AddFollower()
		select {
		case <-request.Done:
			if request.Err != nil {
				return nil, request.Err
			}
			return request, nil
		case <-ctx.ctx.Done():
			return nil, ctx.ctx.Err()
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
	if req.HasFollowers() {
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

// shardFor hashes the key down to pick a shard only; the map is keyed by the
// full key, so a hash collision costs contention, never a shared flight.
func (r *InboundRequestSingleFlight) shardFor(key inboundRequestKey) *requestShard {
	h := pool.Hash64.Get()
	_, _ = h.Write(key[:])
	sum := h.Sum64()
	pool.Hash64.Put(h)
	idx := int(sum % uint64(len(r.shards)))
	return &r.shards[idx]
}
