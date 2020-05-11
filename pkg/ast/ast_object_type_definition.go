package ast

import (
	"bytes"

	"github.com/cespare/xxhash"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
)

type TypeList struct {
	Refs []int // Type
}

type ObjectTypeDefinition struct {
	Description          Description        // optional, e.g. "type Foo is ..."
	TypeLiteral          position.Position  // type
	Name                 ByteSliceReference // e.g. Foo
	ImplementsInterfaces TypeList           // e.g implements Bar & Baz
	HasDirectives        bool
	Directives           DirectiveList // e.g. @foo
	HasFieldDefinitions  bool
	FieldsDefinition     FieldDefinitionList // { foo:Bar bar(baz:String) }
}

func (d *Document) ObjectTypeDefinitionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.ObjectTypeDefinitions[ref].Name)
}

func (d *Document) ObjectTypeDefinitionNameRef(ref int) ByteSliceReference {
	return d.ObjectTypeDefinitions[ref].Name
}

func (d *Document) ObjectTypeDefinitionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.ObjectTypeDefinitions[ref].Name))
}

func (d *Document) ObjectTypeDescriptionNameBytes(ref int) ByteSlice {
	if !d.ObjectTypeDefinitions[ref].Description.IsDefined {
		return nil
	}
	return d.Input.ByteSlice(d.ObjectTypeDefinitions[ref].Description.Content)
}

func (d *Document) ObjectTypeDescriptionNameString(ref int) string {
	return unsafebytes.BytesToString(d.ObjectTypeDescriptionNameBytes(ref))
}

func (d *Document) ObjectTypeDefinitionHasField(ref int, fieldName []byte) bool {
	for _, fieldDefinitionRef := range d.ObjectTypeDefinitions[ref].FieldsDefinition.Refs {
		currentFieldName := d.FieldDefinitionNameBytes(fieldDefinitionRef)
		if currentFieldName.Equals(fieldName) {
			return true
		}
	}
	return false
}

// TODO: to be consistent consider renaming to ObjectTypeDefinitionContainsImplementsInterface
func (d *Document) TypeDefinitionContainsImplementsInterface(typeName, interfaceName ByteSlice) bool {
	typeDefinition, exists := d.Index.Nodes[xxhash.Sum64(typeName)]
	if !exists {
		return false
	}
	if typeDefinition.Kind != NodeKindObjectTypeDefinition {
		return false
	}
	for _, i := range d.ObjectTypeDefinitions[typeDefinition.Ref].ImplementsInterfaces.Refs {
		implements := d.ResolveTypeNameBytes(i)
		if bytes.Equal(interfaceName, implements) {
			return true
		}
	}
	return false
}

type ObjectTypeExtension struct {
	ExtendLiteral position.Position
	ObjectTypeDefinition
}

func (d *Document) ObjectTypeExtensionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.ObjectTypeExtensions[ref].Name)
}

func (d *Document) ObjectTypeExtensionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.ObjectTypeExtensions[ref].Name))
}

func (d *Document) ObjectTypeExtensionHasFieldDefinitions(ref int) bool {
	return d.ObjectTypeExtensions[ref].HasFieldDefinitions
}

func (d *Document) ObjectTypeExtensionHasDirectives(ref int) bool {
	return d.ObjectTypeExtensions[ref].HasDirectives
}

func (d *Document) ExtendObjectTypeDefinitionByObjectTypeExtension(objectTypeDefinitionRef, objectTypeExtensionRef int) {
	if d.ObjectTypeExtensionHasFieldDefinitions(objectTypeExtensionRef) {
		d.ObjectTypeDefinitions[objectTypeDefinitionRef].FieldsDefinition.Refs = append(d.ObjectTypeDefinitions[objectTypeDefinitionRef].FieldsDefinition.Refs, d.ObjectTypeExtensions[objectTypeExtensionRef].FieldsDefinition.Refs...)
		d.ObjectTypeDefinitions[objectTypeDefinitionRef].HasFieldDefinitions = true
	}

	if d.ObjectTypeExtensionHasDirectives(objectTypeExtensionRef) {
		d.ObjectTypeDefinitions[objectTypeDefinitionRef].Directives.Refs = append(d.ObjectTypeDefinitions[objectTypeDefinitionRef].Directives.Refs, d.ObjectTypeExtensions[objectTypeExtensionRef].Directives.Refs...)
		d.ObjectTypeDefinitions[objectTypeDefinitionRef].HasDirectives = true
	}

	d.Index.MergedTypeExtensions = append(d.Index.MergedTypeExtensions, Node{Ref: objectTypeExtensionRef, Kind: NodeKindObjectTypeExtension})
}
