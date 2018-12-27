package document

import "bytes"

// ByteSlice is an alias for []byte which implements MarshalJSON to pretty print string byte slices
type ByteSlice []byte

// MarshalJSON is implemented to make the default json encoder work
func (b ByteSlice) MarshalJSON() ([]byte, error) {
	b = bytes.Replace(b, []byte("\n"), []byte("\\n"), -1)
	return append([]byte(`"`), append(b, []byte(`"`)...)...), nil
}
