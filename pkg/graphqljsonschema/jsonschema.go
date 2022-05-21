package graphqljsonschema

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/qri-io/jsonschema"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
)

func FromTypeRef(operation, definition *ast.Document, typeRef int) JsonSchema {
	resolver := &fromTypeRefResolver{
		overrides: map[string]JsonSchema{},
	}
	return resolver.fromTypeRef(operation, definition, typeRef)
}

func FromTypeRefWithOverrides(operation, definition *ast.Document, typeRef int, overrides map[string]JsonSchema) JsonSchema {
	resolver := &fromTypeRefResolver{
		overrides: overrides,
	}
	return resolver.fromTypeRef(operation, definition, typeRef)
}

type fromTypeRefResolver struct {
	overrides map[string]JsonSchema
	defs      *map[string]JsonSchema
}

func (r *fromTypeRefResolver) fromTypeRef(operation, definition *ast.Document, typeRef int) JsonSchema {

	t := operation.Types[typeRef]

	nonNull := false
	if operation.TypeIsNonNull(typeRef) {
		t = operation.Types[t.OfType]
		nonNull = true
	}

	switch t.TypeKind {
	case ast.TypeKindList:
		var defs map[string]JsonSchema
		isRoot := false
		if r.defs == nil {
			defs = make(map[string]JsonSchema, 48)
			r.defs = &defs
			isRoot = true
		}
		itemSchema := r.fromTypeRef(operation, definition, t.OfType)
		arr := NewArray(itemSchema, nonNull)
		if isRoot {
			arr.Defs = defs
		}
		return arr
	case ast.TypeKindNonNull:
		panic("Should not be able to have multiple levels of non-null")
	case ast.TypeKindNamed:
		name := operation.Input.ByteSliceString(t.Name)
		if schema, ok := r.overrides[name]; ok {
			return schema
		}
		typeDefinitionNode, ok := definition.Index.FirstNodeByNameStr(name)
		if !ok {
			return nil
		}
		if typeDefinitionNode.Kind == ast.NodeKindEnumTypeDefinition {
			return NewString(nonNull)
		}
		if typeDefinitionNode.Kind == ast.NodeKindScalarTypeDefinition {
			switch name {
			case "Boolean":
				return NewBoolean(nonNull)
			case "String":
				return NewString(nonNull)
			case "ID":
				return NewID(nonNull)
			case "Int":
				return NewInteger(nonNull)
			case "Float":
				return NewNumber(nonNull)
			case "_Any":
				return NewObjectAny(nonNull)
			default:
				return NewAny()
			}
		}
		object := NewObject(nonNull)
		isRootObject := false
		if r.defs == nil {
			isRootObject = true
			object.Defs = make(map[string]JsonSchema, 48)
			r.defs = &object.Defs
		}
		if !isRootObject {
			if _, exists := (*r.defs)[name]; exists {
				return NewRef(name)
			}
			(*r.defs)[name] = object
		}
		if node, ok := definition.Index.FirstNodeByNameStr(name); ok {
			switch node.Kind {
			case ast.NodeKindInputObjectTypeDefinition:
				for _, ref := range definition.InputObjectTypeDefinitions[node.Ref].InputFieldsDefinition.Refs {
					fieldName := definition.Input.ByteSliceString(definition.InputValueDefinitions[ref].Name)
					fieldType := definition.InputValueDefinitions[ref].Type
					fieldSchema := r.fromTypeRef(definition, definition, fieldType)
					object.Properties[fieldName] = fieldSchema
					if definition.TypeIsNonNull(fieldType) {
						object.Required = append(object.Required, fieldName)
					}
				}
			case ast.NodeKindObjectTypeDefinition:
				for _, ref := range definition.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs {
					fieldName := definition.Input.ByteSliceString(definition.FieldDefinitions[ref].Name)
					fieldType := definition.FieldDefinitions[ref].Type
					fieldSchema := r.fromTypeRef(definition, definition, fieldType)
					object.Properties[fieldName] = fieldSchema
					if definition.TypeIsNonNull(fieldType) {
						object.Required = append(object.Required, fieldName)
					}
				}
			}
		}
		if !isRootObject {
			(*r.defs)[name] = object
			return NewRef(name)
		}
		return object
	}
	return NewObject(nonNull)
}

type Validator struct {
	schema jsonschema.Schema
}

func NewValidatorFromSchema(schema JsonSchema) (*Validator, error) {
	s, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	return NewValidatorFromString(string(s))
}

func MustNewValidatorFromSchema(schema JsonSchema) *Validator {
	s, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return MustNewValidatorFromString(string(s))
}

func NewValidatorFromString(schema string) (*Validator, error) {
	var validator Validator
	err := json.Unmarshal([]byte(schema), &validator.schema)
	if err != nil {
		return nil, err
	}
	return &validator, nil
}

func MustNewValidatorFromString(schema string) *Validator {
	var validator Validator
	err := json.Unmarshal([]byte(schema), &validator.schema)
	if err != nil {
		panic(err)
	}
	return &validator
}

func TopLevelType(schema string) (jsonparser.ValueType, error) {
	var jsonSchema jsonschema.Schema
	err := json.Unmarshal([]byte(schema), &jsonSchema)
	if err != nil {
		return jsonparser.Unknown, err
	}
	switch jsonSchema.TopLevelType() {
	case "boolean":
		return jsonparser.Boolean, nil
	case "string":
		return jsonparser.String, nil
	case "object":
		return jsonparser.Object, nil
	case "number":
		return jsonparser.Number, nil
	case "integer":
		return jsonparser.Number, nil
	case "null":
		return jsonparser.Null, nil
	case "array":
		return jsonparser.Array, nil
	default:
		return jsonparser.NotExist, nil
	}
}

func (v *Validator) Validate(ctx context.Context, inputJSON []byte) error {
	errs, err := v.schema.ValidateBytes(ctx, inputJSON)
	if err != nil {
		// There was an issue performing the validation itself. Return a
		// generic error so the input isn't exposed.
		return fmt.Errorf("could not perform validation")
	}
	if len(errs) > 0 {
		messages := make([]string, len(errs))
		for i := range errs {
			messages[i] = errs[i].Error()
		}
		return fmt.Errorf("validation failed: %v", strings.Join(messages, "; "))
	}
	return nil
}

type Kind int

const (
	StringKind Kind = iota + 1
	NumberKind
	BooleanKind
	IntegerKind
	ObjectKind
	ArrayKind
	AnyKind
	IDKind
	RefKind
)

func maybeAppendNull(nonNull bool, types ...string) []string {
	if nonNull {
		return types
	}
	return append(types, "null")
}

type JsonSchema interface {
	Kind() Kind
}

type Any struct{}

func NewAny() Any {
	return Any{}
}

func (a Any) Kind() Kind {
	return AnyKind
}

type String struct {
	Type []string `json:"type"`
}

func (_ String) Kind() Kind {
	return StringKind
}

func NewString(nonNull bool) String {
	return String{
		Type: maybeAppendNull(nonNull, "string"),
	}
}

type ID struct {
	Type []string `json:"type"`
}

func (_ ID) Kind() Kind {
	return IDKind
}

func NewID(nonNull bool) ID {
	return ID{
		Type: maybeAppendNull(nonNull, "string", "integer"),
	}
}

type Boolean struct {
	Type []string `json:"type"`
}

func (_ Boolean) Kind() Kind {
	return BooleanKind
}

func NewBoolean(nonNull bool) Boolean {
	return Boolean{
		Type: maybeAppendNull(nonNull, "boolean"),
	}
}

type Number struct {
	Type []string `json:"type"`
}

func NewNumber(nonNull bool) Number {
	return Number{
		Type: maybeAppendNull(nonNull, "number"),
	}
}

func (_ Number) Kind() Kind {
	return NumberKind
}

type Integer struct {
	Type []string `json:"type"`
}

func (_ Integer) Kind() Kind {
	return IntegerKind
}

func NewInteger(nonNull bool) Integer {
	return Integer{
		Type: maybeAppendNull(nonNull, "integer"),
	}
}

type Ref struct {
	Ref string `json:"$ref"`
}

func (_ Ref) Kind() Kind {
	return RefKind
}

func NewRef(definitionName string) Ref {
	return Ref{
		Ref: fmt.Sprintf("#/$defs/%s", definitionName),
	}
}

type Object struct {
	Type                 []string              `json:"type"`
	Properties           map[string]JsonSchema `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	AdditionalProperties bool                  `json:"additionalProperties"`
	Defs                 map[string]JsonSchema `json:"$defs,omitempty"`
}

func (_ Object) Kind() Kind {
	return ObjectKind
}

func NewObject(nonNull bool) Object {
	return Object{
		Type:                 maybeAppendNull(nonNull, "object"),
		Properties:           map[string]JsonSchema{},
		AdditionalProperties: false,
	}
}

func NewObjectAny(nonNull bool) Object {
	return Object{
		Type:                 maybeAppendNull(nonNull, "object"),
		Properties:           map[string]JsonSchema{},
		AdditionalProperties: true,
	}
}

type Array struct {
	Type     []string              `json:"type"`
	Items    JsonSchema            `json:"items"`
	MinItems *int                  `json:"minItems,omitempty"`
	Defs     map[string]JsonSchema `json:"$defs,omitempty"`
}

func (_ Array) Kind() Kind {
	return ArrayKind
}

func NewArray(itemSchema JsonSchema, nonNull bool) Array {
	return Array{
		Type:  maybeAppendNull(nonNull, "array"),
		Items: itemSchema,
	}
}
