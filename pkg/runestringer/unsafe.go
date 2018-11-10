package runestringer

import (
	"bytes"
	"reflect"
	"unsafe"
)

// UnsafeStringer is a runestringer using unsafe methods
type UnsafeStringer struct {
	pool   chan bytes.Buffer
	used   chan bytes.Buffer
	buff   bytes.Buffer
	maxCap int
}

// NewUnsafe returns an *UnsafeStringer
func NewUnsafe(maxCap int) *UnsafeStringer {

	pool := make(chan bytes.Buffer, maxCap)
	used := make(chan bytes.Buffer, maxCap)

	for i := 0; i < maxCap-1; i++ {
		pool <- bytes.Buffer{}
	}

	return &UnsafeStringer{pool, used, bytes.Buffer{}, maxCap}
}

// Write writes the next rune to the stringer
func (u *UnsafeStringer) Write(r rune) {
	u.buff.WriteRune(r)
}

// String returns the String
func (u *UnsafeStringer) String() string {
	b := u.buff.Bytes()
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := reflect.StringHeader{bh.Data, bh.Len}

	u.prepareNext()

	return *(*string)(unsafe.Pointer(&sh))
}

func (u *UnsafeStringer) prepareNext() {
	u.used <- u.buff
	u.buff = <-u.pool
}

// Reset resets the Stringer
func (u *UnsafeStringer) Reset() {
	for {
		select {
		case buff := <-u.used:
			buff.Reset()
			u.pool <- buff
		default:
			return
		}
	}
}
