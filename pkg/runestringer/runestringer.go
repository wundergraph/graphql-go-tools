package runestringer

// RuneStringer is capable of writing runes and return all bytes
type RuneStringer interface {
	Write(rune)
	Bytes() []byte
}
