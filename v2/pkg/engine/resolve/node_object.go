package resolve

import (
	"bytes"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

// KeyField represents a field in an @key directive. Supports nested keys:
// @key(fields: "id")           → [{Name:"id"}]
// @key(fields: "id address { city }") → [{Name:"id"}, {Name:"address", Children:[{Name:"city"}]}]
type KeyField struct {
	Name     string
	Children []KeyField // non-nil for nested object key fields
}

// ObjectCacheAnalytics holds entity analytics configuration set at plan time.
// Nil for non-entity types. For polymorphic types (interface/union), ByTypeName
// maps concrete type names to their analytics config.
type ObjectCacheAnalytics struct {
	// Concrete entity type (ByTypeName == nil): use KeyFields/HashKeys directly
	KeyFields []KeyField // full @key structure (without __typename)
	HashKeys  bool       // true = hash entity keys, false = raw (default)

	// Polymorphic type (ByTypeName != nil): resolve __typename at runtime, then look up
	// Only populated for interface/union types where at least one implementor is an entity
	ByTypeName map[string]*ObjectCacheAnalytics // concreteName → analytics (nil = not entity)
}

// IsKeyField returns true if fieldName is a top-level @key field.
func (a *ObjectCacheAnalytics) IsKeyField(name string) bool {
	for _, kf := range a.KeyFields {
		if kf.Name == name {
			return true
		}
	}
	return false
}

type Object struct {
	Nullable   bool
	Path       []string
	Fields     []*Field
	HasAliases bool // True if any field in this object or descendants has an alias (OriginalName set)

	PossibleTypes  map[string]struct{}   `json:"-"`
	SourceName     string                `json:"-"`
	TypeName       string                `json:"-"`
	CacheAnalytics *ObjectCacheAnalytics `json:"-"` // nil for non-entity types
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
	OriginalName      []byte // Schema field name when Name is an alias; nil if Name IS the original
	Value             Node
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
		Position:     f.Position,
		Defer:        f.Defer,
		Stream:       f.Stream,
		OnTypeNames:  f.OnTypeNames,
		Info:         f.Info,
	}
}

// SchemaFieldName returns the original schema field name.
// If OriginalName is set (field has an alias), returns OriginalName.
// Otherwise returns Name (which IS the original name).
func (f *Field) SchemaFieldName() string {
	if f.OriginalName != nil {
		return unsafebytes.BytesToString(f.OriginalName)
	}
	return unsafebytes.BytesToString(f.Name)
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
	// CacheAnalyticsHash is true if this field should be hashed for cache analytics.
	// Set at plan time for non-key scalar fields on concrete entity types.
	// At runtime, replaces both IsEntityType() and IsKeyField() checks with a single bool.
	CacheAnalyticsHash bool
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

// ComputeHasAliases recursively checks whether any field in the object tree has an alias
// and sets HasAliases on each Object accordingly. Returns true if any alias was found.
func ComputeHasAliases(obj *Object) bool {
	if obj == nil {
		return false
	}
	hasAliases := false
	for _, field := range obj.Fields {
		if field.OriginalName != nil {
			hasAliases = true
		}
		if computeNodeHasAliases(field.Value) {
			hasAliases = true
		}
	}
	obj.HasAliases = hasAliases
	return hasAliases
}

func computeNodeHasAliases(node Node) bool {
	switch n := node.(type) {
	case *Object:
		return ComputeHasAliases(n)
	case *Array:
		if n != nil && n.Item != nil {
			return computeNodeHasAliases(n.Item)
		}
	}
	return false
}
