package ast

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/position"
)

type FragmentDefinitionRef int

// TypeCondition
// example:
// on User
type TypeCondition struct {
	On   position.Position // on
	Type int               // NamedType
}

// FragmentDefinition
// example:
//
//	fragment friendFields on User {
//	 id
//	 name
//	 profilePic(size: 50)
//	}
type FragmentDefinition struct {
	FragmentLiteral position.Position  // fragment
	Name            ByteSliceReference // Name but not on, e.g. friendFields
	TypeCondition   TypeCondition      // e.g. on User
	HasDirectives   bool
	Directives      DirectiveList // optional, e.g. @foo
	SelectionSet    int           // e.g. { id }
	HasSelections   bool
}

func (d *Document) FragmentDefinitionRef(byName ByteSlice) (ref FragmentDefinitionRef, exists bool) {
	for i := range d.FragmentDefinitions {
		if bytes.Equal(byName, d.Input.ByteSlice(d.FragmentDefinitions[i].Name)) {
			return FragmentDefinitionRef(i), true
		}
	}
	return -1, false
}

func (d *Document) FragmentDefinitionTypeName(ref FragmentDefinitionRef) ByteSlice {
	return d.ResolveTypeNameBytes(d.FragmentDefinitions[ref].TypeCondition.Type)
}

func (d *Document) FragmentDefinitionNameBytes(ref FragmentDefinitionRef) ByteSlice {
	return d.Input.ByteSlice(d.FragmentDefinitions[ref].Name)
}

func (d *Document) FragmentDefinitionNameString(ref FragmentDefinitionRef) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.FragmentDefinitions[ref].Name))
}

func (d *Document) FragmentDefinitionIsLastRootNode(ref FragmentDefinitionRef) bool {
	for i := range d.RootNodes {
		if d.RootNodes[i].Kind == NodeKindFragmentDefinition && d.RootNodes[i].Ref == int(ref) {
			return len(d.RootNodes)-1 == i
		}
	}
	return false
}

func (d *Document) FragmentDefinitionIsUsed(name ByteSlice) bool {
	for _, i := range d.Index.ReplacedFragmentSpreads {
		if bytes.Equal(name, d.FragmentSpreadNameBytes(i)) {
			return true
		}
	}
	return false
}
