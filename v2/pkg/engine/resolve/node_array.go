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

type EmptyArray struct{}

func (_ *EmptyArray) NodeKind() NodeKind {
	return NodeKindEmptyArray
}
