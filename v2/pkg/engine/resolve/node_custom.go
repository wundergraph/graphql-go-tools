package resolve

type CustomResolve interface {
	Resolve(value []byte) ([]byte, error)
}

type CustomNode struct {
	CustomResolve
	Nullable bool
	Path     []string
}

func (_ *CustomNode) NodeKind() NodeKind {
	return NodeKindCustom
}
