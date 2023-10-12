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
			switch at := t.Item.(type) {
			case *Object:
				if at.Fetch != nil {
					return true
				}
				if at.HasChildFetches() {
					return true
				}
			case *Array:
				if at.HasChildFetches() {
					return true
				}
			}
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
	OnTypeNames             [][]byte
	SkipDirectiveDefined    bool
	SkipVariableName        string
	IncludeDirectiveDefined bool
	IncludeVariableName     string
	Info                    *FieldInfo
}

type FieldInfo struct {
	// Name is the name of the field.
	Name string
	// ParentTypeNames is the list of possible parent types for this field.
	// E.g. for a root field, this will be Query, Mutation, Subscription.
	// For a field on an object type, this will be the name of that object type.
	// For a field on an interface type, this will be the name of that interface type and all of its possible implementations.
	// For a field on a union type, this will be the name of that union type and all of its possible members.
	ParentTypeNames []string
	Source          TypeFieldSource
}

type TypeFieldSource struct {
	IDs []string
}

type Position struct {
	Line   uint32
	Column uint32
}

type StreamField struct {
	InitialBatchSize int
}

type DeferField struct{}
