package pool

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestLimitBufferPool(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreCurrent(), // ignore the test itself
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	p := NewLimitBufferPool(ctx, LimitBufferPoolOptions{
		MaxBuffers:        4,
		DefaultBufferSize: 1024,
		MaxBufferSize:     1024,
		GCTime:            time.Millisecond * 10,
	})

	buffers := make([]*ResolvableBuffer, 4)

	for i := 0; i < 4; i++ {
		buf := p.Get()
		_, err := buf.Buf.Write(bytes.Repeat([]byte("a"), 64))
		assert.NoError(t, err)
		buffers[i] = buf
	}

	select {
	case <-p.index:
		t.Fatal("should not be able to get more buffers")
	default:
	}

	for i := 0; i < 4; i++ {
		p.Put(buffers[i])
	}

	b := p.Get()
	assert.NotNil(t, b)
	assert.Equal(t, 1024, b.Buf.Cap())
	assert.Equal(t, 0, b.Buf.Len())

	_, err := b.Buf.Write(bytes.Repeat([]byte("a"), 1025))
	assert.NoError(t, err)
	assert.Equal(t, 1025, b.Buf.Len()) // write over the limit
	assert.Equal(t, 2048, b.Buf.Cap()) // should have doubled the initial size
	p.Put(b)                           // should reset the buffer

	for i := 0; i < 4; i++ {
		buf := p.Get()
		assert.NotNil(t, buf.Buf)
		assert.Equal(t, 1024, buf.Buf.Cap()) // default size
		p.Put(buf)
	}

	time.Sleep(time.Millisecond * 100)
	for i := range p.buffers {
		assert.Nil(t, p.buffers[i].Buf) // should have been reset by the GC
	}
}
