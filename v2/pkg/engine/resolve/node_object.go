package resolve

type Object struct {
	Nullable             bool
	Path                 []string
	Fields               []*Field
	Fetch                Fetch
	UnescapeResponseJson bool `json:"unescape_response_json,omitempty"`
}

func (o *Object) HasChildFetches() bool {
	for i := range o.Fields {
		switch t := o.Fields[i].Value.(type) {
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
		default:
			continue
		}
	}
	return false
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
