package pool

import (
	"sync"

	"github.com/cespare/xxhash/v2"
)

var (
	Hash64 = hash64Pool{
		pool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
	}
)

type hash64Pool struct {
	pool sync.Pool
}

func (b *hash64Pool) Get() *xxhash.Digest {
	xxh := b.pool.Get().(*xxhash.Digest)
	xxh.Reset()
	return xxh
}

func (b *hash64Pool) Put(xxh *xxhash.Digest) {
	b.pool.Put(xxh)
}
