package fastbuffer

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFastBuffer(t *testing.T) {
	buf := New()
	buf.WriteBytes([]byte("Hello"))
	assert.Equal(t, "Hello", string(buf.Bytes()))
	buf.WriteBytes([]byte(", World!"))
	assert.Equal(t, "Hello, World!", string(buf.Bytes()))
	buf.Reset()
	assert.Equal(t, "", string(buf.Bytes()))
	buf.WriteBytes([]byte("Hello, World!"))
	assert.Equal(t, "Hello, World!", string(buf.Bytes()))
	buf.Reset()
	buf.WriteBytes([]byte("Goodbye!"))
	assert.Equal(t, "Goodbye!", string(buf.Bytes()))

	buf.b = make([]byte, 0)
	foobar := []byte("FooBar")
	buf.WriteBytes(foobar)
	foobar[0] = 'B'
	assert.Equal(t, "FooBar", string(buf.Bytes()))
}

func BenchmarkFastBuffer(b *testing.B) {
	data := []byte("Hello, World!")
	b.Run("bytes.Buffer", func(b *testing.B) {

		buf := bytes.NewBuffer(make([]byte, 0, 1024))

		b.ResetTimer()
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))

		for i := 0; i < b.N; i++ {
			buf.Reset()
			buf.Write(data)
			if !bytes.Equal(data, buf.Bytes()) {
				b.Error("!=")
			}
		}
	})
	b.Run("fastBuffer", func(b *testing.B) {

		buf := New()

		b.ResetTimer()
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))

		for i := 0; i < b.N; i++ {
			buf.Reset()
			buf.WriteBytes(data)
			if !bytes.Equal(data, buf.Bytes()) {
				b.Error("!=")
			}
		}
	})
}
