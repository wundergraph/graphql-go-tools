package graphqljsonschema

import (
	"context"
	"encoding/json"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/qri-io/jsonschema"
)

func FromTypeRef(definition *ast.Document, typeRef int) JsonSchema {
	t := definition.Types[typeRef]
	switch t.TypeKind {
	case ast.TypeKindList:
		itemSchema := FromTypeRef(definition, t.OfType)
		if definition.TypeIsNonNull(typeRef) {
			min := 1
			return NewArray(itemSchema, &min)
		}
		return NewArray(itemSchema, nil)
	case ast.TypeKindNonNull:
		return FromTypeRef(definition, t.OfType)
	case ast.TypeKindNamed:
		name := definition.Input.ByteSliceString(t.Name)
		if definition.TypeIsEnum(typeRef, definition) {
			return NewString()
		}
		if definition.TypeIsScalar(typeRef, definition) {
			switch name {
			case "Boolean":
				return NewBoolean()
			case "String", "Date", "ID":
				return NewString()
			case "Int":
				return NewInteger()
			case "Float":
				return NewNumber()
			case "_Any":
				return NewObject()
			}
		}
		object := NewObject()
		if node, ok := definition.Index.FirstNodeByNameStr(name); ok {
			switch node.Kind {
			case ast.NodeKindInputObjectTypeDefinition:
				for _, ref := range definition.InputObjectTypeDefinitions[node.Ref].InputFieldsDefinition.Refs {
					fieldName := definition.Input.ByteSliceString(definition.InputValueDefinitions[ref].Name)
					fieldType := definition.InputValueDefinitions[ref].Type
					fieldSchema := FromTypeRef(definition, fieldType)
					object.Properties[fieldName] = fieldSchema
					if definition.TypeIsNonNull(fieldType) {
						object.Required = append(object.Required, fieldName)
					}
				}
			case ast.NodeKindObjectTypeDefinition:
				for _, ref := range definition.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs {
					fieldName := definition.Input.ByteSliceString(definition.FieldDefinitions[ref].Name)
					fieldType := definition.FieldDefinitions[ref].Type
					fieldSchema := FromTypeRef(definition, fieldType)
					object.Properties[fieldName] = fieldSchema
					if definition.TypeIsNonNull(fieldType) {
						object.Required = append(object.Required, fieldName)
					}
				}
			}
		}
		return object
	}
	return NewObject()
}

type Validator struct {
	schema jsonschema.Schema
}

func NewValidatorFromSchema(schema JsonSchema) (*Validator,error) {
	s,err := json.Marshal(schema)
	if err != nil {
		return nil,err
	}
	return NewValidatorFromString(string(s))
}

func MustNewValidatorFromSchema(schema JsonSchema) *Validator {
	s,err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return MustNewValidatorFromString(string(s))
}

func NewValidatorFromString(schema string) (*Validator,error) {
	var validator Validator
	err := json.Unmarshal([]byte(schema),&validator.schema)
	if err != nil {
		return nil,err
	}
	return &validator,nil
}

func MustNewValidatorFromString(schema string) *Validator {
	var validator Validator
	err := json.Unmarshal([]byte(schema),&validator.schema)
	if err != nil {
		panic(err)
	}
	return &validator
}

func (v *Validator) Validate(ctx context.Context, inputJSON []byte) bool {
	errs,err := v.schema.ValidateBytes(ctx, inputJSON)
	return err == nil && len(errs) == 0
}

type Kind int

const (
	StringKind Kind = iota + 1
	NumberKind
	BooleanKind
	IntegerKind
	ObjectKind
	ArrayKind
)

type JsonSchema interface {
	Kind() Kind
}

type String struct {
	Type string `json:"type"`
}

func (_ String) Kind() Kind {
	return StringKind
}

func NewString() String {
	return String{
		Type: "string",
	}
}

type Boolean struct {
	Type string `json:"type"`
}

func (_ Boolean) Kind() Kind {
	return BooleanKind
}

func NewBoolean() Boolean {
	return Boolean{
		Type: "boolean",
	}
}

type Number struct {
	Type string `json:"type"`
}

func NewNumber() Number {
	return Number{
		Type: "number",
	}
}

func (_ Number) Kind() Kind {
	return NumberKind
}

type Integer struct {
	Type string `json:"type"`
}

func (_ Integer) Kind() Kind {
	return IntegerKind
}

func NewInteger() Integer {
	return Integer{
		Type: "integer",
	}
}

type Object struct {
	Type                 string                `json:"type"`
	Properties           map[string]JsonSchema `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	AdditionalProperties bool                  `json:"additionalProperties,omitempty"`
}

func (_ Object) Kind() Kind {
	return ObjectKind
}

func NewObject() Object {
	return Object{
		Type:       "object",
		Properties: map[string]JsonSchema{},
	}
}

type Array struct {
	Type     string     `json:"type"`
	Items    JsonSchema `json:"item"`
	MinItems *int       `json:"minItems,omitempty"`
}

func (_ Array) Kind() Kind {
	return ArrayKind
}

func NewArray(itemSchema JsonSchema, minItems *int) Array {
	return Array{
		Type:     "array",
		Items:    itemSchema,
		MinItems: minItems,
	}
}
