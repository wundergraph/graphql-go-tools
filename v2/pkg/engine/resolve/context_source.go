package resolve

import "slices"

// ContextSource describes how to read a hidden argument from the ancestor at Depth.
type ContextSource struct {
	// An operation root has a statically known __typename, even when the subgraph never selects it.
	RootTypeName string
	Types        []string
	Depth        int
	Selection    []ContextSelection
}

type ContextSelection struct {
	Field    string
	Types    []string
	Children []ContextSelection
}

func (s ContextSource) Equal(other ContextSource) bool {
	return s.Depth == other.Depth && s.RootTypeName == other.RootTypeName &&
		slices.Equal(s.Types, other.Types) && slices.EqualFunc(s.Selection, other.Selection, ContextSelection.equal)
}

func (s ContextSelection) equal(other ContextSelection) bool {
	return s.Field == other.Field && slices.Equal(s.Types, other.Types) &&
		slices.EqualFunc(s.Children, other.Children, ContextSelection.equal)
}
