package resolve

import (
	"slices"
)

type CustomResolve interface {
	Resolve(ctx *Context, value []byte) ([]byte, error)
}

type CustomNode struct {
	CustomResolve

	Nullable bool
	Path     []string
}

func (*CustomNode) NodeKind() NodeKind {
	return NodeKindCustom
}

func (c *CustomNode) Copy() Node {
	return &CustomNode{
		CustomResolve: c.CustomResolve,
		Nullable:      c.Nullable,
		Path:          c.Path,
	}
}

func (c *CustomNode) NodePath() []string {
	return c.Path
}

func (c *CustomNode) NodeNullable() bool {
	return c.Nullable
}

func (c *CustomNode) Equals(n Node) bool {
	other, ok := n.(*CustomNode)
	if !ok {
		return false
	}

	if !slices.Equal(c.Path, other.Path) {
		return false
	}

	if c.CustomResolve != other.CustomResolve {
		return false
	}

	return true
}
