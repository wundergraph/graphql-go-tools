package ast

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
)

// InterfaceTypeDefinition
// example:
// interface NamedEntity {
// 	name: String
// }
type InterfaceTypeDefinition struct {
	Description         Description        // optional, describes the interface
	InterfaceLiteral    position.Position  // interface
	Name                ByteSliceReference // e.g. NamedEntity
	HasDirectives       bool
	Directives          DirectiveList // optional, e.g. @foo
	HasFieldDefinitions bool
	FieldsDefinition    FieldDefinitionList // optional, e.g. { name: String }
}

func (d *Document) InterfaceTypeExtensionHasDirectives(ref int) bool {
	return d.InterfaceTypeExtensions[ref].HasDirectives
}

func (d *Document) InterfaceTypeExtensionHasFieldDefinitions(ref int) bool {
	return d.InterfaceTypeExtensions[ref].HasFieldDefinitions
}

func (d *Document) InterfaceTypeDefinitionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.InterfaceTypeDefinitions[ref].Name)
}

func (d *Document) InterfaceTypeDefinitionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.InterfaceTypeDefinitions[ref].Name))
}

func (d *Document) InterfaceTypeDefinitionDescriptionBytes(ref int) ByteSlice {
	if !d.InterfaceTypeDefinitions[ref].Description.IsDefined {
		return nil
	}
	return d.Input.ByteSlice(d.InterfaceTypeDefinitions[ref].Description.Content)
}

func (d *Document) InterfaceTypeDefinitionDescriptionString(ref int) string {
	return unsafebytes.BytesToString(d.InterfaceTypeDefinitionDescriptionBytes(ref))
}

type InterfaceTypeExtension struct {
	ExtendLiteral position.Position
	InterfaceTypeDefinition
}

func (d *Document) InterfaceTypeExtensionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.InterfaceTypeExtensions[ref].Name)
}

func (d *Document) InterfaceTypeExtensionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.InterfaceTypeExtensions[ref].Name))
}

func (d *Document) ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(interfaceTypeDefinitionRef, interfaceTypeExtensionRef int) {
	if d.InterfaceTypeExtensionHasFieldDefinitions(interfaceTypeExtensionRef) {
		d.InterfaceTypeDefinitions[interfaceTypeDefinitionRef].FieldsDefinition.Refs = append(d.InterfaceTypeDefinitions[interfaceTypeDefinitionRef].FieldsDefinition.Refs, d.InterfaceTypeExtensions[interfaceTypeExtensionRef].FieldsDefinition.Refs...)
		d.InterfaceTypeDefinitions[interfaceTypeDefinitionRef].HasFieldDefinitions = true
	}

	if d.InterfaceTypeExtensionHasDirectives(interfaceTypeExtensionRef) {
		d.InterfaceTypeDefinitions[interfaceTypeDefinitionRef].Directives.Refs = append(d.InterfaceTypeDefinitions[interfaceTypeDefinitionRef].Directives.Refs, d.InterfaceTypeExtensions[interfaceTypeExtensionRef].Directives.Refs...)
		d.InterfaceTypeDefinitions[interfaceTypeDefinitionRef].HasDirectives = true
	}

	d.Index.MergedTypeExtensions = append(d.Index.MergedTypeExtensions, Node{Ref: interfaceTypeExtensionRef, Kind: NodeKindInterfaceTypeExtension})
}
