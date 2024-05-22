package resolve

type Array struct {
	Path     []string
	Nullable bool
	Item     Node
}

func (_ *Array) NodeKind() NodeKind {
	return NodeKindArray
}

func (a *Array) NodePath() []string {
	return a.Path
}

func (a *Array) NodeNullable() bool {
	return a.Nullable
}

type EmptyArray struct{}

func (_ *EmptyArray) NodeKind() NodeKind {
	return NodeKindEmptyArray
}

func (_ *EmptyArray) NodePath() []string {
	return nil
}

func (_ *EmptyArray) NodeNullable() bool {
	return false
}
