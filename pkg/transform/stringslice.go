package transform

import (
	"strconv"
)

// StringToFloat32 converts a string slice to a float32
func StringToFloat32(input string) (float32, error) {
	f64, err := strconv.ParseFloat(input, 32)
	return float32(f64), err
}

// StringToInt32 converts a string slice to a int32
func StringToInt32(input string) (int32, error) {
	i64, err := strconv.ParseInt(input, 10, 32)
	return int32(i64), err
}
