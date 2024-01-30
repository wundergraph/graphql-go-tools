package unsafebytes

import (
	"strconv"
	"unsafe"
)

func BytesToInt64(byteSlice []byte) int64 {
	out, _ := strconv.ParseInt(*(*string)(unsafe.Pointer(&byteSlice)), 10, 64)
	return out
}

func BytesToInt32(byteSlice []byte) int32 {
	out, _ := strconv.ParseInt(*(*string)(unsafe.Pointer(&byteSlice)), 10, 32)
	return int32(out)
}

func BytesToFloat32(byteSlice []byte) float32 {
	out, _ := strconv.ParseFloat(*(*string)(unsafe.Pointer(&byteSlice)), 64)
	return float32(out)
}

func BytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func BytesToBool(byteSlice []byte) bool {
	out, _ := strconv.ParseBool(*(*string)(unsafe.Pointer(&byteSlice)))
	return out
}

func StringToBytes(str string) []byte {
	return unsafe.Slice(unsafe.StringData(str), len(str))
}

func BytesIsValidFloat32(byteSlice []byte) bool {
	_, err := strconv.ParseFloat(*(*string)(unsafe.Pointer(&byteSlice)), 64)
	return err == nil
}

func BytesIsValidInt64(byteSlice []byte) bool {
	_, err := strconv.ParseInt(*(*string)(unsafe.Pointer(&byteSlice)), 10, 64)
	return err == nil
}

func BytesIsValidInt32(byteSlice []byte) bool {
	_, err := strconv.ParseInt(*(*string)(unsafe.Pointer(&byteSlice)), 10, 32)
	return err == nil
}

func BytesIsValidBool(byteSlice []byte) bool {
	_, err := strconv.ParseBool(*(*string)(unsafe.Pointer(&byteSlice)))
	return err == nil
}
