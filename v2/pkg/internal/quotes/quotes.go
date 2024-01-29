package quotes

const (
	quoteByte = '"'
	quoteStr  = string(quoteByte)
)

// WrapBytes returns a new slice wrapping the given s
// in quotes (") by making a copy.
func WrapBytes(s []byte) []byte {
	cp := make([]byte, len(s)+2)
	cp[0] = quoteByte
	copy(cp[1:], s)
	cp[len(s)+1] = quoteByte
	return cp
}

func WrapString(str string) string {
	return quoteStr + str + quoteStr
}
