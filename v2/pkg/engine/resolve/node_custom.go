package resolve

type CustomResolve interface {
	Resolve(ctx *Context, value []byte) ([]byte, error)
}

type CustomNode struct {
	CustomResolve
	Nullable bool
	Path     []string
}

func (_ *CustomNode) NodeKind() NodeKind {
	return NodeKindCustom
}

func (c *CustomNode) NodePath() []string {
	return c.Path
}

func (c *CustomNode) NodeNullable() bool {
	return c.Nullable
}
