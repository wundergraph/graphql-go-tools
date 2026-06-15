package resolve

import (
	"bytes"
	"slices"
)

type Object struct {
	Nullable   bool
	Path       []string
	Fields     []*Field
	HasAliases bool `json:",omitempty"`

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
		Nullable:   o.Nullable,
		Path:       o.Path,
		Fields:     fields,
		HasAliases: o.HasAliases,
	}
}

func (*Object) NodeKind() NodeKind {
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

// isAbstract returns whether the object resolves an interface or union field.
// An abstract object's PossibleTypes holds the concrete implementers/members
// (an entity interface additionally includes the interface name itself).
func (o *Object) isAbstract() bool {
	if len(o.PossibleTypes) > 1 {
		return true
	}
	if len(o.PossibleTypes) == 1 {
		_, self := o.PossibleTypes[o.TypeName]
		return !self
	}
	return false
}

type EmptyObject struct{}

func (*EmptyObject) NodeKind() NodeKind {
	return NodeKindEmptyObject
}

func (*EmptyObject) NodePath() []string {
	return nil
}

func (*EmptyObject) NodeNullable() bool {
	return false
}

func (*EmptyObject) Equals(n Node) bool {
	_, ok := n.(*EmptyObject)
	return ok
}

func (*EmptyObject) Copy() Node {
	return &EmptyObject{}
}

type Field struct {
	Name              []byte
	OriginalName      []byte `json:",omitempty"`
	Value             Node
	CacheArgs         []CacheFieldArg `json:",omitempty"`
	Position          Position
	Defer             *DeferField
	Stream            *StreamField
	OnTypeNames       [][]byte
	ParentOnTypeNames []ParentOnTypeNames
	Info              *FieldInfo
}

type ParentOnTypeNames struct {
	Depth int
	Names [][]byte
}

func (f *Field) Copy() *Field {
	return &Field{
		Name:         f.Name,
		OriginalName: f.OriginalName,
		Value:        f.Value.Copy(),
		CacheArgs:    f.CacheArgs,
		Position:     f.Position,
		Defer:        f.Defer,
		Stream:       f.Stream,
		OnTypeNames:  f.OnTypeNames,
		Info:         f.Info,
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

type CacheFieldArg struct {
	ArgName      string
	VariableName string
}

type StreamField struct {
	InitialBatchSize int
}

type DeferField struct{}

func ComputeHasAliases(o *Object) bool {
	if o == nil {
		return false
	}

	for _, field := range o.Fields {
		if field == nil {
			continue
		}
		if len(field.CacheArgs) > 0 {
			return true
		}
		if len(field.OriginalName) > 0 && !bytes.Equal(field.OriginalName, field.Name) {
			return true
		}
		if nodeHasAliases(field.Value) {
			return true
		}
	}

	return false
}

func nodeHasAliases(node Node) bool {
	switch n := node.(type) {
	case *Object:
		return ComputeHasAliases(n)
	case *Array:
		if n == nil {
			return false
		}
		return nodeHasAliases(n.Item)
	default:
		return false
	}
}
