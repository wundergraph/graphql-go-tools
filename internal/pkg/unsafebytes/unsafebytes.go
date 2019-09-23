package unsafebytes

import (
	"reflect"
	"strconv"
	"unsafe"
)

func BytesToInt64(byteSlice []byte) int64 {
	out, _ := strconv.ParseInt(*(*string)(unsafe.Pointer(&byteSlice)), 10, 64)
	return out
}

func BytesToFloat64(byteSlice []byte) float64 {
	out, _ := strconv.ParseFloat(*(*string)(unsafe.Pointer(&byteSlice)), 64)
	return out
}

func BytesToString(bytes []byte) string {
	sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
	stringHeader := reflect.StringHeader{Data: sliceHeader.Data, Len: sliceHeader.Len}
	return *(*string)(unsafe.Pointer(&stringHeader))
}

func StringToBytes(str string) []byte {
	hdr := *(*reflect.StringHeader)(unsafe.Pointer(&str))
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: hdr.Data,
		Len:  hdr.Len,
		Cap:  hdr.Len,
	}))
}
