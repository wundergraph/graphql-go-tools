package ast

import (
	"bytes"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
)

// EnumTypeDefinition
// example:
// enum Direction {
//  NORTH
//  EAST
//  SOUTH
//  WEST
//}
type EnumTypeDefinition struct {
	Description             Description        // optional, describes enum
	EnumLiteral             position.Position  // enum
	Name                    ByteSliceReference // e.g. Direction
	HasDirectives           bool
	Directives              DirectiveList // optional, e.g. @foo
	HasEnumValuesDefinition bool
	EnumValuesDefinition    EnumValueDefinitionList // optional, e.g. { NORTH EAST }
}

func (d *Document) EnumTypeDefinitionHasDirectives(ref int) bool {
	return d.EnumTypeDefinitions[ref].HasDirectives
}

func (d *Document) EnumTypeDefinitionHasEnumValueDefinition(ref int) bool {
	return d.EnumTypeDefinitions[ref].HasEnumValuesDefinition
}

func (d *Document) EnumTypeDefinitionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.EnumTypeDefinitions[ref].Name)
}

func (d *Document) EnumTypeDefinitionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.EnumTypeDefinitions[ref].Name))
}

func (d *Document) EnumTypeDefinitionDescriptionBytes(ref int) ByteSlice {
	if !d.EnumTypeDefinitions[ref].Description.IsDefined {
		return nil
	}
	return d.Input.ByteSlice(d.EnumTypeDefinitions[ref].Description.Content)
}

func (d *Document) EnumTypeDefinitionDescriptionString(ref int) string {
	return unsafebytes.BytesToString(d.EnumTypeDefinitionDescriptionBytes(ref))
}

func (d *Document) EnumTypeDefinitionContainsEnumValue(enumTypeDef int, valueName ByteSlice) bool {
	for _, i := range d.EnumTypeDefinitions[enumTypeDef].EnumValuesDefinition.Refs {
		if bytes.Equal(valueName, d.EnumValueDefinitionNameBytes(i)) {
			return true
		}
	}
	return false
}

type EnumTypeExtension struct {
	ExtendLiteral position.Position
	EnumTypeDefinition
}

func (d *Document) EnumTypeExtensionHasDirectives(ref int) bool {
	return d.EnumTypeExtensions[ref].HasDirectives
}

func (d *Document) EnumTypeExtensionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.EnumTypeExtensions[ref].Name)
}

func (d *Document) EnumTypeExtensionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.EnumTypeExtensions[ref].Name))
}

func (d *Document) ExtendEnumTypeDefinitionByEnumTypeExtension(enumTypeDefinitionRef, enumTypeExtensionRef int) {
	if d.EnumTypeExtensionHasDirectives(enumTypeExtensionRef) {
		d.EnumTypeDefinitions[enumTypeDefinitionRef].Directives.Refs = append(d.EnumTypeDefinitions[enumTypeDefinitionRef].Directives.Refs, d.EnumTypeExtensions[enumTypeExtensionRef].Directives.Refs...)
		d.EnumTypeDefinitions[enumTypeDefinitionRef].HasDirectives = true
	}

	if d.EnumTypeDefinitionHasEnumValueDefinition(enumTypeExtensionRef) {
		d.EnumTypeDefinitions[enumTypeDefinitionRef].EnumValuesDefinition.Refs = append(d.EnumTypeDefinitions[enumTypeDefinitionRef].EnumValuesDefinition.Refs, d.EnumTypeExtensions[enumTypeExtensionRef].EnumValuesDefinition.Refs...)
		d.EnumTypeDefinitions[enumTypeDefinitionRef].HasEnumValuesDefinition = true
	}

	d.Index.MergedTypeExtensions = append(d.Index.MergedTypeExtensions, Node{Ref: enumTypeExtensionRef, Kind: NodeKindEnumTypeExtension})
}
