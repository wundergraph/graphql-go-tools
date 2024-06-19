package resolve

import "slices"

type Array struct {
	Path     []string
	Nullable bool
	Item     Node
}

func (_ *Array) NodeKind() NodeKind {
	return NodeKindArray
}

func (a *Array) Copy() Node {
	return &Array{
		Path:     a.Path,
		Nullable: a.Nullable,
		Item:     a.Item.Copy(),
	}
}

func (a *Array) NodePath() []string {
	return a.Path
}

func (a *Array) NodeNullable() bool {
	return a.Nullable
}

func (a *Array) Equals(n Node) bool {
	other, ok := n.(*Array)
	if !ok {
		return false
	}

	if a.Nullable != other.Nullable {
		return false
	}

	if !slices.Equal(a.Path, other.Path) {
		return false
	}

	return a.Item.Equals(other.Item)
}

type EmptyArray struct{}

func (_ *EmptyArray) Copy() Node {
	return &EmptyArray{}
}

func (_ *EmptyArray) NodeKind() NodeKind {
	return NodeKindEmptyArray
}

func (_ *EmptyArray) NodePath() []string {
	return nil
}

func (_ *EmptyArray) NodeNullable() bool {
	return false
}

func (_ *EmptyArray) Equals(n Node) bool {
	_, ok := n.(*EmptyArray)
	return ok
}
