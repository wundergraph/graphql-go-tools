package ast

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
)

type InputValueDefinitionList struct {
	LPAREN position.Position // (
	Refs   []int             // InputValueDefinition
	RPAREN position.Position // )
}

type DefaultValue struct {
	IsDefined bool
	Equals    position.Position // =
	Value     Value             // e.g. "Foo"
}

type InputValueDefinition struct {
	Description   Description        // optional, e.g. "input Foo is..."
	Name          ByteSliceReference // e.g. Foo
	Colon         position.Position  // :
	Type          int                // e.g. String
	DefaultValue  DefaultValue       // e.g. = "Bar"
	HasDirectives bool
	Directives    DirectiveList // e.g. @baz
}

func (d *Document) InputValueDefinitionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.InputValueDefinitions[ref].Name)
}

func (d *Document) InputValueDefinitionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.InputValueDefinitions[ref].Name))
}

func (d *Document) InputValueDefinitionDescriptionBytes(ref int) ByteSlice {
	if !d.InputValueDefinitions[ref].Description.IsDefined {
		return nil
	}
	return d.Input.ByteSlice(d.InputValueDefinitions[ref].Description.Content)
}

func (d *Document) InputValueDefinitionDescriptionString(ref int) string {
	return unsafebytes.BytesToString(d.InputValueDefinitionDescriptionBytes(ref))
}

func (d *Document) InputValueDefinitionType(ref int) int {
	return d.InputValueDefinitions[ref].Type
}

func (d *Document) InputValueDefinitionHasDirectives(ref int) bool {
	return d.InputValueDefinitions[ref].HasDirectives
}

func (d *Document) InputValueDefinitionHasDefaultValue(ref int) bool {
	return d.InputValueDefinitions[ref].DefaultValue.IsDefined
}

func (d *Document) InputValueDefinitionDefaultValue(ref int) Value {
	return d.InputValueDefinitions[ref].DefaultValue.Value
}

func (d *Document) InputValueDefinitionArgumentIsOptional(ref int) bool {
	nonNull := d.Types[d.InputValueDefinitions[ref].Type].TypeKind == TypeKindNonNull
	hasDefault := d.InputValueDefinitions[ref].DefaultValue.IsDefined
	return !nonNull || hasDefault
}

func (d *Document) InputValueDefinitionHasDirective(ref int, directiveName ByteSlice) bool {
	if !d.InputValueDefinitions[ref].HasDirectives {
		return false
	}
	for _, i := range d.InputValueDefinitions[ref].Directives.Refs {
		if bytes.Equal(directiveName, d.DirectiveNameBytes(i)) {
			return true
		}
	}
	return false
}

func (d *Document) InputValueDefinitionDirectiveByName(definitionRef int, directiveName ByteSlice) (ref int, exists bool) {
	for _, i := range d.InputValueDefinitions[definitionRef].Directives.Refs {
		if bytes.Equal(directiveName, d.DirectiveNameBytes(i)) {
			return i, true
		}
	}
	return
}

func (d *Document) AddInputValueDefinition(inputValueDefinition InputValueDefinition) (ref int) {
	d.InputValueDefinitions = append(d.InputValueDefinitions, inputValueDefinition)
	return len(d.InputValueDefinitions) - 1
}

func (d *Document) ImportInputValueDefinition(name, description string, typeRef int, defaultValue DefaultValue) (ref int) {
	inputValueDef := InputValueDefinition{
		Description:  d.ImportDescription(description),
		Name:         d.Input.AppendInputString(name),
		Type:         typeRef,
		DefaultValue: defaultValue,
	}

	return d.AddInputValueDefinition(inputValueDef)
}

func (d *Document) NamedTypeNodeByInputValueDefinitionRef(inputValueDefinitionRef int) (Node, bool) {
	inputValueTypeRef := d.InputValueDefinitions[inputValueDefinitionRef].Type
	typeNameBytes := d.ResolveTypeNameBytes(inputValueTypeRef)
	return d.Index.FirstNodeByNameBytes(typeNameBytes)
}

func (d *Document) InputValueDefinitionRefByFieldDefinitionRefAndArgumentNameBytes(fieldDefinitionRef int, argumentNameBytes []byte) (inputValueDefinitionRef int, ok bool) {
	for _, inputValueDefinitionRef = range d.FieldDefinitions[fieldDefinitionRef].ArgumentsDefinition.Refs {
		if d.InputValueDefinitionNameBytes(inputValueDefinitionRef).Equals(argumentNameBytes) {
			return inputValueDefinitionRef, true
		}
	}
	return InvalidRef, false
}

func (d *Document) InputValueDefinitionRefByInputObjectDefinitionRefAndFieldNameBytes(inputObjectDefinitionRef int, inputValueNameBytes []byte) (inputValueDefinitionRef int, ok bool) {
	for _, inputValueDefinitionRef = range d.InputObjectTypeDefinitions[inputObjectDefinitionRef].InputFieldsDefinition.Refs {
		if d.InputValueDefinitionNameBytes(inputValueDefinitionRef).Equals(inputValueNameBytes) {
			return inputValueDefinitionRef, true
		}
	}
	return InvalidRef, false
}
