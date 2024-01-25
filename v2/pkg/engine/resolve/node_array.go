package resolve

type Array struct {
	Path                []string
	Nullable            bool
	ResolveAsynchronous bool
	Item                Node
	Items               []Node
}

func (a *Array) HasChildFetches() bool {
	switch t := a.Item.(type) {
	case *Object:
		if t.Fetch != nil {
			return true
		}
		if t.HasChildFetches() {
			return true
		}
	case *Array:
		if t.HasChildFetches() {
			return true
		}
	}
	return false
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
