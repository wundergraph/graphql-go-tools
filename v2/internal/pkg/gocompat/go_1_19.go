//go:build !go1.20

package gocompat

import (
	"reflect"
	"unsafe"
)

func GetUnsafeByteSliceByString(str *string) []byte {
	stringHdr := (*reflect.StringHeader)(unsafe.Pointer(str))
	return unsafe.Slice((*byte)(unsafe.Pointer(stringHdr.Data)), len(*str))
}
