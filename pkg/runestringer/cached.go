package runestringer

import "bytes"

// LazyCachedRuneStringer is a runestringer using a bytes.Buffer and a map as cache
type LazyCachedRuneStringer struct {
	buff   bytes.Buffer
	strMap map[string]string
}

// NewLazyCached returns a new *LazyCachedRuneStringer
func NewLazyCached() *LazyCachedRuneStringer {
	return &LazyCachedRuneStringer{
		buff:   bytes.Buffer{},
		strMap: map[string]string{},
	}
}

// Write writes the next rune
func (r *LazyCachedRuneStringer) Write(ru rune) {
	r.buff.WriteRune(ru)
}

// String returns all bytes as String and resets the buffer
func (r *LazyCachedRuneStringer) String() string {
	b := r.buff.Bytes()
	str, ok := r.strMap[string(b)]
	if !ok {
		str := string(b)
		r.strMap[str] = str
	}

	r.buff.Reset()
	return str
}
