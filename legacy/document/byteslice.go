package document

import "github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"

type ByteSlice []byte

func (b ByteSlice) MarshalJSON() ([]byte, error) {
	return append(append(literal.QUOTE, b...), literal.QUOTE...), nil
}

type ByteSliceReference struct {
	Start   uint32
	End     uint32
	NextRef int
}

func (b ByteSliceReference) Length() uint32 {
	return b.End - b.Start
}

type ByteSliceReferenceGetter interface {
	ByteSliceReference(ref int) ByteSliceReference
}

type ByteSliceReferences struct {
	nextRef    int
	currentRef int
	current    ByteSliceReference
}

func NewByteSliceReferences(nextRef int) ByteSliceReferences {
	return ByteSliceReferences{
		nextRef: nextRef,
	}
}

func (i *ByteSliceReferences) HasNext() bool {
	return i.nextRef != -1
}

func (i *ByteSliceReferences) Next(getter ByteSliceReferenceGetter) bool {
	if i.nextRef == -1 {
		return false
	}

	i.currentRef = i.nextRef
	i.current = getter.ByteSliceReference(i.nextRef)
	i.nextRef = i.current.NextRef
	return true
}

func (i *ByteSliceReferences) Value() (ByteSliceReference, int) {
	return i.current, i.currentRef
}
