package quotes

const (
	quoteByte = '"'
	quoteStr  = string(quoteByte)
)

func WrapBytes(bytes []byte) []byte {
	cp := make([]byte, len(bytes)+2)
	cp[0] = quoteByte
	copy(cp[1:], bytes)
	cp[len(bytes)+1] = quoteByte
	return cp
}

func WrapString(str string) string {
	return quoteStr + str + quoteStr
}
