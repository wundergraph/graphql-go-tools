//go:build go1.20

package gocompat

import (
	"unsafe"
)

func GetUnsafeByteSliceByString(str *string) []byte {
	return unsafe.Slice(unsafe.StringData(*str), len(*str))
}
