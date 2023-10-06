package resolve

import "github.com/wundergraph/graphql-go-tools/v2/pkg/fastbuffer"

type BufPair struct {
	Data   *fastbuffer.FastBuffer
	Errors *fastbuffer.FastBuffer
}

func NewBufPair() *BufPair {
	return &BufPair{
		Data:   fastbuffer.New(),
		Errors: fastbuffer.New(),
	}
}

func (b *BufPair) HasData() bool {
	return b.Data.Len() != 0
}

func (b *BufPair) HasErrors() bool {
	return b.Errors.Len() != 0
}

func (b *BufPair) Reset() {
	b.Data.Reset()
	b.Errors.Reset()
}

func (b *BufPair) writeErrors(data []byte) {
	b.Errors.WriteBytes(data)
}

func (b *BufPair) WriteErr(message, locations, path, extensions []byte) {
	if b.HasErrors() {
		b.writeErrors(comma)
	}
	b.writeErrors(lBrace)
	b.writeErrors(quote)
	b.writeErrors(literalMessage)
	b.writeErrors(quote)
	b.writeErrors(colon)
	b.writeErrors(quote)
	b.writeErrors(message)
	b.writeErrors(quote)

	if locations != nil {
		b.writeErrors(comma)
		b.writeErrors(quote)
		b.writeErrors(literalLocations)
		b.writeErrors(quote)
		b.writeErrors(colon)
		b.writeErrors(locations)
	}

	if path != nil {
		b.writeErrors(comma)
		b.writeErrors(quote)
		b.writeErrors(literalPath)
		b.writeErrors(quote)
		b.writeErrors(colon)
		b.writeErrors(path)
	}

	if extensions != nil {
		b.writeErrors(comma)
		b.writeErrors(quote)
		b.writeErrors(literalExtensions)
		b.writeErrors(quote)
		b.writeErrors(colon)
		b.writeErrors(extensions)
	}

	b.writeErrors(rBrace)
}
