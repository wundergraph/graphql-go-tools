package resolve

const (
	NodeKindObject NodeKind = iota + 1
	NodeKindEmptyObject
	NodeKindArray
	NodeKindEmptyArray
	NodeKindNull
	NodeKindString
	NodeKindBoolean
	NodeKindInteger
	NodeKindFloat
	NodeKindBigInt
	NodeKindCustom
	NodeKindScalar
)

type Node interface {
	NodeKind() NodeKind
}

type NodeKind int
