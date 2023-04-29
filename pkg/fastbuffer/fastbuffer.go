package fastbuffer

import (
	"reflect"
	"unsafe"
)

func New() *FastBuffer {
	return &FastBuffer{
		b: make([]byte, 0, 1024),
	}
}

type FastBuffer struct {
	b []byte
}

func (f *FastBuffer) Write(p []byte) (n int, err error) {
	f.b = append(f.b, p...)
	return len(p), nil
}

func (f *FastBuffer) Reset() {
	f.b = f.b[:0]
}

func (f *FastBuffer) WriteBytes(b []byte) {
	f.b = append(f.b, b...)
}

func (f *FastBuffer) WriteString(s string) {
	f.b = append(f.b, s...)
}

func (f *FastBuffer) Bytes() []byte {
	return f.b
}

func (f *FastBuffer) Len() int {
	return len(f.b)
}

// Grow increases the buffer capacity to be able to hold at least n more bytes
func (f *FastBuffer) Grow(n int) {
	required := cap(f.b) - len(f.b) + n
	if required > 0 {
		b := make([]byte, len(f.b), len(f.b)+n)
		copy(b, f.b)
		f.b = b
	}
}

func (f *FastBuffer) UnsafeString() string {
	sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&f.b))
	stringHeader := reflect.StringHeader{Data: sliceHeader.Data, Len: sliceHeader.Len}
	return *(*string)(unsafe.Pointer(&stringHeader)) // nolint: govet
}

func (f *FastBuffer) String() string {
	return string(f.b)
}
