package resolve

import (
	"bytes"
	"slices"
)

type Object struct {
	Nullable bool
	Path     []string
	Fields   []*Field
	Fetches  []Fetch
	Fetch    Fetch

	PossibleTypes map[string]struct{} `json:"-"`
	SourceName    string              `json:"-"`
	TypeName      string              `json:"-"`
}

func (o *Object) Copy() Node {
	fields := make([]*Field, len(o.Fields))
	for i, f := range o.Fields {
		fields[i] = f.Copy()
	}
	return &Object{
		Nullable: o.Nullable,
		Path:     o.Path,
		Fields:   fields,
		Fetches:  o.Fetches,
	}
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

func (o *Object) Equals(n Node) bool {
	other, ok := n.(*Object)
	if !ok {
		return false
	}
	if o.Nullable != other.Nullable {
		return false
	}

	if !slices.Equal(o.Path, other.Path) {
		return false
	}

	if !slices.EqualFunc(o.Fields, other.Fields, func(a, b *Field) bool {
		return a.Equals(b)
	}) {
		return false
	}

	// We ignore fetches in comparison, because we compare shape of the response nodes

	return true
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

func (_ *EmptyObject) Equals(n Node) bool {
	_, ok := n.(*EmptyObject)
	return ok
}

func (_ *EmptyObject) Copy() Node {
	return &EmptyObject{}
}

type Field struct {
	Name                    []byte
	Value                   Node
	Position                Position
	Defer                   *DeferField
	Stream                  *StreamField
	OnTypeNames             [][]byte
	ParentOnTypeNames       []ParentOnTypeNames
	SkipDirectiveDefined    bool
	SkipVariableName        string
	IncludeDirectiveDefined bool
	IncludeVariableName     string
	Info                    *FieldInfo
}

type ParentOnTypeNames struct {
	Depth int
	Names [][]byte
}

func (f *Field) Copy() *Field {
	return &Field{
		Name:                    f.Name,
		Value:                   f.Value.Copy(),
		Position:                f.Position,
		Defer:                   f.Defer,
		Stream:                  f.Stream,
		OnTypeNames:             f.OnTypeNames,
		SkipDirectiveDefined:    f.SkipDirectiveDefined,
		SkipVariableName:        f.SkipVariableName,
		IncludeDirectiveDefined: f.IncludeDirectiveDefined,
		IncludeVariableName:     f.IncludeVariableName,
		Info:                    f.Info,
	}
}

func (f *Field) Equals(n *Field) bool {
	// NOTE: a lot of struct fields are not compared here
	// because they are not relevant for the value comparison of response nodes

	if !bytes.Equal(f.Name, n.Name) {
		return false
	}
	if !f.Value.Equals(n.Value) {
		return false
	}
	return true
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
	// IndirectInterfaceNames is set to the interfaces name if the field is on a concrete type that implements an interface which wraps it
	// It's plural because interfaces and be overlapping with types that implement multiple interfaces
	IndirectInterfaceNames []string
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
	IDs   []string
	Names []string
}

type Position struct {
	Line   uint32
	Column uint32
}

type StreamField struct {
	InitialBatchSize int
}

type DeferField struct{}
