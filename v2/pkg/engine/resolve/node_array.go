package resolve

import (
	"slices"

	"github.com/wundergraph/astjson"
)

type Array struct {
	Path     []string
	Nullable bool
	Item     Node

	SkipItem SkipArrayItem
}

type SkipArrayItem func(ctx *Context, arrayItem *astjson.Value) bool

type IntrospectionData struct {
	IncludeDeprecatedVariableName string
}

func (*Array) NodeKind() NodeKind {
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

func (*EmptyArray) Copy() Node {
	return &EmptyArray{}
}

func (*EmptyArray) NodeKind() NodeKind {
	return NodeKindEmptyArray
}

func (*EmptyArray) NodePath() []string {
	return nil
}

func (*EmptyArray) NodeNullable() bool {
	return false
}

func (*EmptyArray) Equals(n Node) bool {
	_, ok := n.(*EmptyArray)
	return ok
}
