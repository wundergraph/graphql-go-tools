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

func (*Enum) NodeKind() NodeKind {
	return NodeKindEnum
}

func (e *Enum) Copy() Node {
	return &Enum{
		Path:               e.Path,
		Nullable:           e.Nullable,
		Export:             e.Export,
		TypeName:           e.TypeName,
		Values:             e.Values,
		InaccessibleValues: e.InaccessibleValues,
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
	return slices.Contains(e.Values, returnedValue)
}

func (e *Enum) isAccessibleValue(returnedValue string) bool {
	return !slices.Contains(e.InaccessibleValues, returnedValue)
}
