package pool

import (
	"bytes"
	"context"
	"runtime"
	"time"
)

// LimitBufferPool is a pool of buffers that is limited in size and is limiting the max size of buffers that should be recycled
// In addition, it runs a GC runtime that randomly resets a buffer every second to keep the memory usage low when usage is low
// This is an alternative to sync.Pool, which can grow unbounded rather quickly
type LimitBufferPool struct {
	buffers []*ResolvableBuffer
	index   chan int
	options LimitBufferPoolOptions
}

type ResolvableBuffer struct {
	Buf *bytes.Buffer
	idx int
}

type LimitBufferPoolOptions struct {
	// MaxBuffers limits the total amount of buffers that can be allocated for printing the response
	// It's recommended to set this to the number of CPU cores available, or a multiple of it
	// If set to 0, the number of CPU cores is used
	MaxBuffers int
	// MaxBufferSize limits the size of the buffer that can be recycled back into the pool
	// If set to 0, a limit of 10MB is applied
	// If the buffer cap exceeds this limit, a new buffer with the default size is created
	MaxBufferSize int
	// DefaultBufferSize is used to initialize the buffer with a default size
	// If set to 0, a default size of 8KB is used
	DefaultBufferSize int
}

func NewLimitBufferPool(ctx context.Context, options LimitBufferPoolOptions) *LimitBufferPool {
	if options.MaxBufferSize == 0 {
		options.MaxBufferSize = 1024 * 1024 * 10 // 10MB
	}
	if options.DefaultBufferSize < 1024*8 {
		options.DefaultBufferSize = 1024 * 8 // 8KB
	}
	if options.MaxBuffers == 0 {
		options.MaxBuffers = runtime.GOMAXPROCS(-1)
	}
	if options.MaxBuffers < 8 {
		options.MaxBuffers = 8
	}
	pool := &LimitBufferPool{
		buffers: make([]*ResolvableBuffer, options.MaxBuffers),
		index:   make(chan int, options.MaxBuffers),
		options: options,
	}
	for i := range pool.buffers {
		pool.buffers[i] = &ResolvableBuffer{
			idx: i,
		}
		pool.index <- i
	}
	go pool.runGC(ctx)
	return pool
}

func (p *LimitBufferPool) runGC(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b := p.Get()
			b.Buf = nil
			p.Put(b)
		}
	}
}

func (p *LimitBufferPool) Get() *ResolvableBuffer {
	i := <-p.index
	if p.buffers[i].Buf == nil {
		p.buffers[i].Buf = bytes.NewBuffer(make([]byte, 0, p.options.DefaultBufferSize))
	}
	return p.buffers[i]
}

func (p *LimitBufferPool) Put(buf *ResolvableBuffer) {
	if buf.Buf != nil {
		buf.Buf.Reset()
		if buf.Buf.Cap() > p.options.MaxBufferSize {
			buf.Buf = bytes.NewBuffer(make([]byte, 0, p.options.DefaultBufferSize))
		}
	}
	p.index <- buf.idx
}
