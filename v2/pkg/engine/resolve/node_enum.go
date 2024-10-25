package resolve

import "slices"

type Enum struct {
	Path               []string
	Nullable           bool
	Export             *FieldExport `json:"export,omitempty"`
	TypeName           string
	Values             []string
	InaccessibleValues []string
}

func (_ *Enum) NodeKind() NodeKind {
	return NodeKindEnum
}

func (e *Enum) Copy() Node {
	return &Enum{
		Path:     e.Path,
		Nullable: e.Nullable,
		Export:   e.Export,
	}
}

func (e *Enum) NodePath() []string {
	return e.Path
}

func (e *Enum) NodeNullable() bool {
	return e.Nullable
}

func (e *Enum) Equals(n Node) bool {
	other, ok := n.(*Enum)
	if !ok {
		return false
	}

	if e.Nullable != other.Nullable {
		return false
	}

	if !slices.Equal(e.Path, other.Path) {
		return false
	}

	return true
}

func (e *Enum) isValidValue(returnedValue string) bool {
	for _, value := range e.Values {
		if value == returnedValue {
			return true
		}
	}
	return false
}

func (e *Enum) isAccessibleValue(returnedValue string) bool {
	for _, value := range e.InaccessibleValues {
		if value == returnedValue {
			return false
		}
	}
	return true
}
