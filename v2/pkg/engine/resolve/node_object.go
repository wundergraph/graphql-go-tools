package resolve

type Object struct {
	Nullable             bool
	Path                 []string
	Fields               []*Field
	Fetch                Fetch
	UnescapeResponseJson bool `json:"unescape_response_json,omitempty"`
}

func (_ *Object) NodeKind() NodeKind {
	return NodeKindObject
}

type EmptyObject struct{}

func (_ *EmptyObject) NodeKind() NodeKind {
	return NodeKindEmptyObject
}

type Field struct {
	Name                    []byte
	Value                   Node
	Position                Position
	Defer                   *DeferField
	Stream                  *StreamField
	HasBuffer               bool
	BufferID                int
	OnTypeNames             [][]byte
	SkipDirectiveDefined    bool
	SkipVariableName        string
	IncludeDirectiveDefined bool
	IncludeVariableName     string
}

type Position struct {
	Line   uint32
	Column uint32
}

type StreamField struct {
	InitialBatchSize int
}

type DeferField struct{}
