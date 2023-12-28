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

type Null struct {
}

func (_ *Null) NodeKind() NodeKind {
	return NodeKindNull
}

func (_ *Null) NodePath() []string {
	return nil
}

// FieldExport takes the value of the field during evaluation (rendering of the field)
// and stores it in the variables using the Path as JSON pointer.
type FieldExport struct {
	Path     []string
	AsString bool
}
