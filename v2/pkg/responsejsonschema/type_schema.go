package responsejsonschema

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation"
)

type typedSchema struct {
	Type []string `json:"type"`
}

type anyOfSchema struct {
	AnyOf []json.RawMessage `json:"anyOf"`
}

type arraySchema struct {
	Type  []string        `json:"type"`
	Items json.RawMessage `json:"items"`
}

type selectedObjectSchema struct {
	Type                 []string                   `json:"type"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Required             []string                   `json:"required"`
	AdditionalProperties bool                       `json:"additionalProperties"`
}

type enumSchema struct {
	Type []string `json:"type"`
	Enum []any    `json:"enum"`
}

func schemaForTypeRef(operation, definition *ast.Document, typeRef int, selectionSetRefs []int, options *options) (json.RawMessage, error) {
	if typeRef < 0 || typeRef >= len(definition.Types) {
		return nil, fmt.Errorf("type reference %d is out of bounds", typeRef)
	}

	nullable := true
	typeNode := definition.Types[typeRef]
	if typeNode.TypeKind == ast.TypeKindNonNull {
		nullable = false
		if typeNode.OfType < 0 || typeNode.OfType >= len(definition.Types) {
			return nil, fmt.Errorf("non-null type reference %d has invalid inner reference %d", typeRef, typeNode.OfType)
		}
		typeRef = typeNode.OfType
		typeNode = definition.Types[typeRef]
	}

	if typeNode.TypeKind == ast.TypeKindList {
		itemSchema, err := schemaForTypeRef(operation, definition, typeNode.OfType, selectionSetRefs, options)
		if err != nil {
			return nil, fmt.Errorf("build list item schema: %w", err)
		}
		types := []string{"array"}
		if nullable {
			types = append(types, "null")
		}
		return marshalSchema(arraySchema{Type: types, Items: itemSchema})
	}
	if typeNode.TypeKind != ast.TypeKindNamed {
		return nil, fmt.Errorf("unsupported GraphQL type kind %q", typeNode.TypeKind)
	}

	typeName := definition.TypeNameString(typeRef)
	if customSchema, ok := options.customScalarMappings[typeName]; ok {
		if !nullable {
			return append(json.RawMessage(nil), customSchema...), nil
		}

		return marshalSchema(anyOfSchema{AnyOf: []json.RawMessage{
			append(json.RawMessage(nil), customSchema...),
			json.RawMessage(`{"type":"null"}`),
		}})
	}

	var types []string
	switch typeName {
	case "String":
		types = []string{"string"}
	case "Boolean":
		types = []string{"boolean"}
	case "Int":
		types = []string{"integer"}
	case "Float":
		types = []string{"number"}
	case "ID":
		types = []string{"string", "integer"}
	}

	if len(types) != 0 {
		if nullable {
			types = append(types, "null")
		}
		return marshalSchema(typedSchema{Type: types})
	}

	typeDefinition, ok := definition.Index.FirstNodeByNameStr(typeName)
	if !ok {
		return json.RawMessage(`{}`), nil
	}
	switch typeDefinition.Kind {
	case ast.NodeKindObjectTypeDefinition:
		return selectedObjectTypeSchema(operation, definition, typeDefinition.Ref, selectionSetRefs, nullable, options)
	case ast.NodeKindEnumTypeDefinition:
		return accessibleEnumTypeSchema(definition, typeDefinition.Ref, nullable)
	default:
		return json.RawMessage(`{}`), nil
	}
}

func accessibleEnumTypeSchema(definition *ast.Document, enumTypeDefinitionRef int, nullable bool) (json.RawMessage, error) {
	values := make([]any, 0, len(definition.EnumTypeDefinitions[enumTypeDefinitionRef].EnumValuesDefinition.Refs)+1)
	for _, enumValueDefinitionRef := range definition.EnumTypeDefinitions[enumTypeDefinitionRef].EnumValuesDefinition.Refs {
		if _, inaccessible := definition.EnumValueDefinitionDirectiveByName(enumValueDefinitionRef, federation.InaccessibleDirectiveNameBytes); inaccessible {
			continue
		}
		values = append(values, definition.EnumValueDefinitionNameString(enumValueDefinitionRef))
	}

	types := []string{"string"}
	if nullable {
		types = append(types, "null")
		values = append(values, nil)
	}

	return marshalSchema(enumSchema{Type: types, Enum: values})
}

func selectedObjectTypeSchema(operation, definition *ast.Document, objectTypeDefinitionRef int, selectionSetRefs []int, nullable bool, options *options) (json.RawMessage, error) {
	if len(selectionSetRefs) == 0 {
		return nil, fmt.Errorf(
			"object type %q requires a selection set",
			definition.ObjectTypeDefinitionNameString(objectTypeDefinitionRef),
		)
	}

	properties := make(map[string]json.RawMessage)
	fieldGroups, err := selectedFieldGroups(operation, selectionSetRefs)
	if err != nil {
		return nil, err
	}
	required := make([]string, 0, len(fieldGroups))
	for _, fieldGroup := range fieldGroups {
		fieldRef := fieldGroup.fieldRefs[0]
		responseName := fieldGroup.responseName
		fieldDefinitionRef, ok := definition.ObjectTypeDefinitionFieldWithName(objectTypeDefinitionRef, operation.FieldNameBytes(fieldRef))
		if !ok {
			return nil, fmt.Errorf(
				"field %q is not defined on object type %q",
				operation.FieldNameString(fieldRef),
				definition.ObjectTypeDefinitionNameString(objectTypeDefinitionRef),
			)
		}

		childSelectionSetRefs := make([]int, 0, len(fieldGroup.fieldRefs))
		for _, repeatedFieldRef := range fieldGroup.fieldRefs {
			if operation.FieldNameString(repeatedFieldRef) != operation.FieldNameString(fieldRef) {
				return nil, fmt.Errorf(
					"response property %q combines incompatible fields %q and %q",
					responseName,
					operation.FieldNameString(fieldRef),
					operation.FieldNameString(repeatedFieldRef),
				)
			}
			if operation.Fields[repeatedFieldRef].HasSelections {
				childSelectionSetRefs = append(childSelectionSetRefs, operation.Fields[repeatedFieldRef].SelectionSet)
			}
		}
		propertySchema, err := schemaForTypeRef(
			operation,
			definition,
			definition.FieldDefinitions[fieldDefinitionRef].Type,
			childSelectionSetRefs,
			options,
		)
		if err != nil {
			return nil, fmt.Errorf("build schema for response property %q: %w", responseName, err)
		}

		properties[responseName] = propertySchema
		required = append(required, responseName)
	}
	// selectedFieldGroups is sorted, but sorting here keeps this invariant local
	// to the emitted schema.
	sort.Strings(required)

	types := []string{"object"}
	if nullable {
		types = append(types, "null")
	}

	return marshalSchema(selectedObjectSchema{
		Type:                 types,
		Properties:           properties,
		Required:             required,
		AdditionalProperties: false,
	})
}

func marshalSchema(schema any) (json.RawMessage, error) {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON Schema: %w", err)
	}
	return json.RawMessage(encoded), nil
}
