package resolve

type Array struct {
	Path                []string
	Nullable            bool
	ResolveAsynchronous bool
	Item                Node
	Stream              Stream
}

type Stream struct {
	Enabled          bool
	InitialBatchSize int
	PatchIndex       int
}

func (_ *Array) NodeKind() NodeKind {
	return NodeKindArray
}

type EmptyArray struct{}

func (_ *EmptyArray) NodeKind() NodeKind {
	return NodeKindEmptyArray
}
