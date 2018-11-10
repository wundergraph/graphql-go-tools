package runestringer

import "bytes"

// BufferedRuneStringer is a runestringer using a bytes.Buffer
type BufferedRuneStringer struct {
	buff bytes.Buffer
}

// NewBuffered returns a new *BufferedRuneStringer
func NewBuffered() *BufferedRuneStringer {
	return &BufferedRuneStringer{
		buff: bytes.Buffer{},
	}
}

// Write writes the next rune
func (b *BufferedRuneStringer) Write(r rune) {
	b.buff.WriteRune(r)
}

// Bytes returns all bytes from the stringer and resets the buffer
func (b *BufferedRuneStringer) Bytes() []byte {
	out := b.buff.Bytes()
	b.buff.Reset()
	return out
}
