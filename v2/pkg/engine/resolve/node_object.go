package resolve

import (
	"slices"
)

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

func (o *Object) NodePath() []string {
	return o.Path
}

func (o *Object) NodeNullable() bool {
	return o.Nullable
}

type EmptyObject struct{}

func (_ *EmptyObject) NodeKind() NodeKind {
	return NodeKindEmptyObject
}

func (_ *EmptyObject) NodePath() []string {
	return nil
}

func (_ *EmptyObject) NodeNullable() bool {
	return false
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
	Name                string
	ExactParentTypeName string
	// ParentTypeNames is the list of possible parent types for this field.
	// E.g. for a root field, this will be Query, Mutation, Subscription.
	// For a field on an object type, this will be the name of that object type.
	// For a field on an interface type, this will be the name of that interface type and all of its possible implementations.
	ParentTypeNames []string
	// NamedType is the underlying node type of the field.
	// E.g. for a field of type Hobby! this will be Hobby.
	// For a field of type [Hobby] this will be Hobby.
	// For a field of type [Hobby!]! this will be Hobby.
	// For scalar fields, this will return string, int, float, boolean, ID.
	NamedType string
	Source    TypeFieldSource
	FetchID   int
	// HasAuthorizationRule needs to be set to true if the Authorizer should be called for this field
	HasAuthorizationRule bool
}

func (i *FieldInfo) Merge(other *FieldInfo) {
	for _, name := range other.ParentTypeNames {
		if !slices.Contains(i.ParentTypeNames, name) {
			i.ParentTypeNames = append(i.ParentTypeNames, name)
		}
	}

	for _, sourceID := range other.Source.IDs {
		if !slices.Contains(i.Source.IDs, sourceID) {
			i.Source.IDs = append(i.Source.IDs, sourceID)
		}
	}
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
