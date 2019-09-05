package transform

import (
	"strconv"
)

// StringToFloat32 converts a string slice to a float32
func StringToFloat32(input []byte) (float32, error) {
	f64, err := strconv.ParseFloat(string(input), 32)
	return float32(f64), err
}

// StringToInt32 converts a string slice to a int32
func StringToInt32(input []byte) (int32, error) {
	i64, err := strconv.ParseInt(string(input), 10, 32)
	return int32(i64), err
}
