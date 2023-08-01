package graphqljsonschema

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/buger/jsonparser"
	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
)

type options struct {
	overrides map[string]JsonSchema
	path      []string
}

type Option func(opts *options)

func WithOverrides(overrides map[string]JsonSchema) Option {
	return func(opts *options) {
		opts.overrides = overrides
	}
}

func WithPath(path []string) Option {
	return func(opts *options) {
		opts.path = path
	}
}

func FromTypeRef(operation, definition *ast.Document, typeRef int, opts ...Option) JsonSchema {
	appliedOptions := &options{}
	for _, opt := range opts {
		opt(appliedOptions)
	}

	var resolver *fromTypeRefResolver
	if len(appliedOptions.overrides) > 0 {
		resolver = &fromTypeRefResolver{
			overrides: appliedOptions.overrides,
		}
	} else {
		resolver = &fromTypeRefResolver{
			overrides: map[string]JsonSchema{},
		}
	}

	jsonSchema := resolver.fromTypeRef(operation, definition, typeRef)
	return resolveJsonSchemaPath(jsonSchema, appliedOptions.path)
}

func resolveJsonSchemaPath(jsonSchema JsonSchema, path []string) JsonSchema {
	switch typedJsonSchema := jsonSchema.(type) {
	case Object:
		for i := 0; i < len(path); i++ {
			propertyJsonSchema, exists := typedJsonSchema.Properties[path[i]]
			if !exists {
				return jsonSchema
			}
			jsonSchema = propertyJsonSchema
		}
	}

	return jsonSchema
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
			return NewAny()
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
	schema *jsonschema.Schema
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
	sch, err := jsonschema.CompileString("schema.json", schema)
	if err != nil {
		return nil, err
	}
	return &Validator{
		schema: sch,
	}, nil
}

func MustNewValidatorFromString(schema string) *Validator {
	validator, err := NewValidatorFromString(schema)
	if err != nil {
		panic(err)
	}
	return validator
}

func (v *Validator) Validate(ctx context.Context, inputJSON []byte) error {
	var value interface{}
	if err := json.Unmarshal(inputJSON, &value); err != nil {
		return err
	}
	if err := v.schema.Validate(value); err != nil {
		return err
	}
	return nil
}

func TopLevelType(schema string) (jsonparser.ValueType, error) {
	sch, err := jsonschema.CompileString("schema.json", schema)
	if err != nil {
		return jsonparser.Unknown, err
	}
	switch sch.Types[0] {
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

func (String) Kind() Kind {
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

func (ID) Kind() Kind {
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

func (Boolean) Kind() Kind {
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

func (Number) Kind() Kind {
	return NumberKind
}

type Integer struct {
	Type []string `json:"type"`
}

func (Integer) Kind() Kind {
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

func (Ref) Kind() Kind {
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

func (Object) Kind() Kind {
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

func (Array) Kind() Kind {
	return ArrayKind
}

func NewArray(itemSchema JsonSchema, nonNull bool) Array {
	return Array{
		Type:  maybeAppendNull(nonNull, "array"),
		Items: itemSchema,
	}
}
