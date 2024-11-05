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
	NodeKindStaticString
	NodeKindEnum
)

type Node interface {
	NodeKind() NodeKind
	NodePath() []string
	NodeNullable() bool
	Equals(Node) bool
	Copy() Node
}

type NodeKind int
