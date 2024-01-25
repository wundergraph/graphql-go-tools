package resolve

type Scalar struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Scalar) NodeKind() NodeKind {
	return NodeKindScalar
}

func (s *Scalar) NodePath() []string {
	return s.Path
}

func (s *Scalar) NodeNullable() bool {
	return s.Nullable
}

type String struct {
	Path                 []string
	Nullable             bool
	Export               *FieldExport `json:"export,omitempty"`
	UnescapeResponseJson bool         `json:"unescape_response_json,omitempty"`
	IsTypeName           bool         `json:"is_type_name,omitempty"`
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

func (s *StaticString) NodeNullable() bool {
	return false
}

type Boolean struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Boolean) NodeKind() NodeKind {
	return NodeKindBoolean
}

func (b *Boolean) NodePath() []string {
	return b.Path
}

func (b *Boolean) NodeNullable() bool {
	return b.Nullable
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

type BigInt struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *BigInt) NodeKind() NodeKind {
	return NodeKindBigInt
}

func (b *BigInt) NodePath() []string {
	return b.Path
}

func (b *BigInt) NodeNullable() bool {
	return b.Nullable
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

// FieldExport takes the value of the field during evaluation (rendering of the field)
// and stores it in the variables using the Path as JSON pointer.
type FieldExport struct {
	Path     []string
	AsString bool
}
