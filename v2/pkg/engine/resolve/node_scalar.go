package resolve

import "slices"

type Scalar struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Scalar) NodeKind() NodeKind {
	return NodeKindScalar
}

func (s *Scalar) Copy() Node {
	return &Scalar{
		Path:     s.Path,
		Nullable: s.Nullable,
		Export:   s.Export,
	}
}

func (s *Scalar) NodePath() []string {
	return s.Path
}

func (s *Scalar) NodeNullable() bool {
	return s.Nullable
}

func (s *Scalar) Equals(n Node) bool {
	other, ok := n.(*Scalar)
	if !ok {
		return false
	}

	if s.Nullable != other.Nullable {
		return false
	}

	if !slices.Equal(s.Path, other.Path) {
		return false
	}

	return true
}

type String struct {
	Path                 []string
	Nullable             bool
	Export               *FieldExport `json:"export,omitempty"`
	UnescapeResponseJson bool         `json:"unescape_response_json,omitempty"`
	IsTypeName           bool         `json:"is_type_name,omitempty"`
}

func (s *String) Copy() Node {
	return &String{
		Path:                 s.Path,
		Nullable:             s.Nullable,
		Export:               s.Export,
		UnescapeResponseJson: s.UnescapeResponseJson,
		IsTypeName:           s.IsTypeName,
	}
}

func (s *String) Equals(n Node) bool {
	other, ok := n.(*String)
	if !ok {
		return false
	}

	if s.Nullable != other.Nullable {
		return false
	}

	if s.UnescapeResponseJson != other.UnescapeResponseJson {
		return false
	}

	if s.IsTypeName != other.IsTypeName {
		return false
	}

	if !slices.Equal(s.Path, other.Path) {
		return false
	}

	return true
}

func (_ *String) NodeKind() NodeKind {
	return NodeKindString
}

func (s *String) NodePath() []string {
	return s.Path
}

func (s *String) NodeNullable() bool {
	return s.Nullable
}

type StaticString struct {
	Path  []string
	Value string
}

func (_ *StaticString) NodeKind() NodeKind {
	return NodeKindStaticString
}

func (s *StaticString) NodePath() []string {
	return s.Path
}

func (s *StaticString) Copy() Node {
	return &StaticString{
		Path:  s.Path,
		Value: s.Value,
	}
}

func (s *StaticString) NodeNullable() bool {
	return false
}

func (s *StaticString) Equals(n Node) bool {
	other, ok := n.(*StaticString)
	if !ok {
		return false
	}

	if s.Value != other.Value {
		return false
	}

	if !slices.Equal(s.Path, other.Path) {
		return false
	}

	return true
}

type Boolean struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Boolean) NodeKind() NodeKind {
	return NodeKindBoolean
}

func (b *Boolean) Copy() Node {
	return &Boolean{
		Path:     b.Path,
		Nullable: b.Nullable,
		Export:   b.Export,
	}
}

func (b *Boolean) NodePath() []string {
	return b.Path
}

func (b *Boolean) NodeNullable() bool {
	return b.Nullable
}

func (b *Boolean) Equals(n Node) bool {
	other, ok := n.(*Boolean)
	if !ok {
		return false
	}

	if b.Nullable != other.Nullable {
		return false
	}

	if !slices.Equal(b.Path, other.Path) {
		return false
	}

	return true
}

type Float struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Float) NodeKind() NodeKind {
	return NodeKindFloat
}

func (f *Float) NodePath() []string {
	return f.Path
}

func (f *Float) NodeNullable() bool {
	return f.Nullable
}

func (f *Float) Copy() Node {
	return &Float{
		Path:     f.Path,
		Nullable: f.Nullable,
		Export:   f.Export,
	}
}

func (f *Float) Equals(n Node) bool {
	other, ok := n.(*Float)
	if !ok {
		return false
	}

	if f.Nullable != other.Nullable {
		return false
	}

	if !slices.Equal(f.Path, other.Path) {
		return false
	}

	return true
}

type Integer struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Integer) NodeKind() NodeKind {
	return NodeKindInteger
}

func (i *Integer) NodePath() []string {
	return i.Path
}

func (i *Integer) NodeNullable() bool {
	return i.Nullable
}

func (i *Integer) Copy() Node {
	return &Integer{
		Path:     i.Path,
		Nullable: i.Nullable,
		Export:   i.Export,
	}
}

func (i *Integer) Equals(n Node) bool {
	other, ok := n.(*Integer)
	if !ok {
		return false
	}

	if i.Nullable != other.Nullable {
		return false
	}

	if !slices.Equal(i.Path, other.Path) {
		return false
	}

	return true

}

type BigInt struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *BigInt) NodeKind() NodeKind {
	return NodeKindBigInt
}

func (b *BigInt) Copy() Node {
	return &BigInt{
		Path:     b.Path,
		Nullable: b.Nullable,
		Export:   b.Export,
	}
}

func (b *BigInt) NodePath() []string {
	return b.Path
}

func (b *BigInt) NodeNullable() bool {
	return b.Nullable
}

func (b *BigInt) Equals(n Node) bool {
	other, ok := n.(*BigInt)
	if !ok {
		return false
	}

	if b.Nullable != other.Nullable {
		return false
	}

	if !slices.Equal(b.Path, other.Path) {
		return false
	}

	return true

}

type Null struct {
}

func (_ *Null) NodeKind() NodeKind {
	return NodeKindNull
}

func (_ *Null) NodePath() []string {
	return nil
}

func (_ *Null) NodeNullable() bool {
	return true
}

func (_ *Null) Copy() Node {
	return &Null{}
}

func (_ *Null) Equals(n Node) bool {
	_, ok := n.(*Null)
	return ok
}

// FieldExport takes the value of the field during evaluation (rendering of the field)
// and stores it in the variables using the Path as JSON pointer.
type FieldExport struct {
	Path     []string
	AsString bool
}
