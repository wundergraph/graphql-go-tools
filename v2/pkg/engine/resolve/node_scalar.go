package resolve

type Scalar struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Scalar) NodeKind() NodeKind {
	return NodeKindScalar
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

type Boolean struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Boolean) NodeKind() NodeKind {
	return NodeKindBoolean
}

type Float struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Float) NodeKind() NodeKind {
	return NodeKindFloat
}

type Integer struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (_ *Integer) NodeKind() NodeKind {
	return NodeKindInteger
}

type BigInt struct {
	Path     []string
	Nullable bool
	Export   *FieldExport `json:"export,omitempty"`
}

func (BigInt) NodeKind() NodeKind {
	return NodeKindBigInt
}

type Null struct {
	Defer Defer
}

type Defer struct {
	Enabled    bool
	PatchIndex int
}

func (_ *Null) NodeKind() NodeKind {
	return NodeKindNull
}

// FieldExport takes the value of the field during evaluation (rendering of the field)
// and stores it in the variables using the Path as JSON pointer.
type FieldExport struct {
	Path     []string
	AsString bool
}
