package runestringer

import "bytes"

// CachedRuneStringer is a runestringer with bytes.Buffer and a map as cache
type CachedRuneStringer struct {
	buff   bytes.Buffer
	strMap map[string]string
}

// NewCached returns a new *CachedRuneStringer
func NewCached() *CachedRuneStringer {
	return &CachedRuneStringer{
		buff:   bytes.Buffer{},
		strMap: map[string]string{},
	}
}

// Write writes the next rune
func (r *CachedRuneStringer) Write(ru rune) {
	r.buff.WriteRune(ru)
}

// String returns all Bytes as string and resets the buffer
func (r *CachedRuneStringer) String() string {
	str := r.strMap[r.buff.String()]
	r.buff.Reset()
	return str
}

// Train fills the cache
func (r *CachedRuneStringer) Train(str string) {
	r.strMap[str] = str
}
