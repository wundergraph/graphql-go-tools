package ast

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
)

// UnionTypeDefinition
// example:
// union SearchResult = Photo | Person
type UnionTypeDefinition struct {
	Description         Description        // optional, describes union
	UnionLiteral        position.Position  // union
	Name                ByteSliceReference // e.g. SearchResult
	HasDirectives       bool
	Directives          DirectiveList     // optional, e.g. @foo
	Equals              position.Position // =
	HasUnionMemberTypes bool
	UnionMemberTypes    TypeList // optional, e.g. Photo | Person
}

func (d *Document) UnionMemberTypeIsFirst(ref int, ancestor Node) bool {
	switch ancestor.Kind {
	case NodeKindUnionTypeDefinition:
		return len(d.UnionTypeDefinitions[ancestor.Ref].UnionMemberTypes.Refs) != 0 &&
			d.UnionTypeDefinitions[ancestor.Ref].UnionMemberTypes.Refs[0] == ref
	case NodeKindUnionTypeExtension:
		return len(d.UnionTypeExtensions[ancestor.Ref].UnionMemberTypes.Refs) != 0 &&
			d.UnionTypeExtensions[ancestor.Ref].UnionMemberTypes.Refs[0] == ref
	default:
		return false
	}
}

func (d *Document) UnionMemberTypeIsLast(ref int, ancestor Node) bool {
	switch ancestor.Kind {
	case NodeKindUnionTypeDefinition:
		return len(d.UnionTypeDefinitions[ancestor.Ref].UnionMemberTypes.Refs) != 0 &&
			d.UnionTypeDefinitions[ancestor.Ref].UnionMemberTypes.Refs[len(d.UnionTypeDefinitions[ancestor.Ref].UnionMemberTypes.Refs)-1] == ref
	case NodeKindUnionTypeExtension:
		return len(d.UnionTypeExtensions[ancestor.Ref].UnionMemberTypes.Refs) != 0 &&
			d.UnionTypeExtensions[ancestor.Ref].UnionMemberTypes.Refs[len(d.UnionTypeExtensions[ancestor.Ref].UnionMemberTypes.Refs)-1] == ref
	default:
		return false
	}
}

func (d *Document) UnionTypeDefinitionHasDirectives(ref int) bool {
	return d.UnionTypeDefinitions[ref].HasDirectives
}

func (d *Document) UnionTypeDefinitionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.UnionTypeDefinitions[ref].Name)
}

func (d *Document) UnionTypeDefinitionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.UnionTypeDefinitions[ref].Name))
}

func (d *Document) UnionTypeDefinitionDescriptionBytes(ref int) ByteSlice {
	if !d.UnionTypeDefinitions[ref].Description.IsDefined {
		return nil
	}
	return d.Input.ByteSlice(d.UnionTypeDefinitions[ref].Description.Content)
}

func (d *Document) UnionTypeDefinitionDescriptionString(ref int) string {
	return unsafebytes.BytesToString(d.UnionTypeDefinitionDescriptionBytes(ref))
}

type UnionTypeExtension struct {
	ExtendLiteral position.Position
	UnionTypeDefinition
}

func (d *Document) UnionTypeExtensionHasDirectives(ref int) bool {
	return d.UnionTypeExtensions[ref].HasDirectives
}

func (d *Document) UnionTypeExtensionHasUnionMemberTypes(ref int) bool {
	return d.UnionTypeExtensions[ref].HasUnionMemberTypes
}

func (d *Document) UnionTypeExtensionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.UnionTypeExtensions[ref].Name)
}

func (d *Document) UnionTypeExtensionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.UnionTypeExtensions[ref].Name))
}

func (d *Document) ExtendUnionTypeDefinitionByUnionTypeExtension(unionTypeDefinitionRef, unionTypeExtensionRef int) {
	if d.UnionTypeExtensionHasDirectives(unionTypeExtensionRef) {
		d.UnionTypeDefinitions[unionTypeDefinitionRef].Directives.Refs = append(d.UnionTypeDefinitions[unionTypeDefinitionRef].Directives.Refs, d.UnionTypeExtensions[unionTypeExtensionRef].Directives.Refs...)
		d.UnionTypeDefinitions[unionTypeDefinitionRef].HasDirectives = true
	}

	if d.UnionTypeExtensionHasUnionMemberTypes(unionTypeExtensionRef) {
		d.UnionTypeDefinitions[unionTypeDefinitionRef].UnionMemberTypes.Refs = append(d.UnionTypeDefinitions[unionTypeDefinitionRef].UnionMemberTypes.Refs, d.UnionTypeExtensions[unionTypeExtensionRef].UnionMemberTypes.Refs...)
		d.UnionTypeDefinitions[unionTypeDefinitionRef].HasUnionMemberTypes = true
	}

	d.Index.MergedTypeExtensions = append(d.Index.MergedTypeExtensions, Node{Ref: unionTypeExtensionRef, Kind: NodeKindUnionTypeExtension})
}
