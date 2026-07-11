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

type allOfSchema struct {
	AllOf []json.RawMessage `json:"allOf"`
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

type constStringSchema struct {
	Type  string `json:"type"`
	Const string `json:"const"`
}

func schemaForTypeRef(operation, definition *ast.Document, typeRef int, selectionSetRefs []int, options *options) (json.RawMessage, error) {
	if options == nil || options.traversal == nil {
		return nil, fmt.Errorf("response schema traversal is unavailable")
	}
	leaveDepth, err := options.traversal.enterDepth("GraphQL types")
	if err != nil {
		return nil, err
	}
	defer leaveDepth()

	if _, err := checkedDefinitionTypeName(definition, typeRef, "type"); err != nil {
		return nil, err
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

	typeNameBytes, err := checkedBytes(definition, typeNode.Name, "type name")
	if err != nil {
		return nil, err
	}
	typeName := string(typeNameBytes)
	if customSchema, ok := options.customScalarMappings[typeName]; ok {
		if !nullable {
			return marshalSchema(allOfSchema{AllOf: []json.RawMessage{
				append(json.RawMessage(nil), customSchema...),
				nonNullJSONSchema(),
			}})
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

	typeDefinition, ok, err := checkedIndexNode(definition, typeName)
	if err != nil {
		return nil, err
	}
	if !ok {
		return json.RawMessage(`{}`), nil
	}
	if _, err := checkedDefinitionNodeName(definition, typeDefinition); err != nil {
		return nil, err
	}
	switch typeDefinition.Kind {
	case ast.NodeKindObjectTypeDefinition:
		return selectedObjectTypeSchema(operation, definition, typeDefinition.Ref, selectionSetRefs, nullable, options)
	case ast.NodeKindInterfaceTypeDefinition:
		return selectedInterfaceTypeSchema(operation, definition, typeDefinition.Ref, selectionSetRefs, nullable, options)
	case ast.NodeKindUnionTypeDefinition:
		return selectedUnionTypeSchema(operation, definition, typeDefinition.Ref, selectionSetRefs, nullable, options)
	case ast.NodeKindEnumTypeDefinition:
		return accessibleEnumTypeSchema(definition, typeDefinition.Ref, nullable)
	case ast.NodeKindScalarTypeDefinition:
		if nullable {
			return json.RawMessage(`{}`), nil
		}
		return nonNullJSONSchema(), nil
	default:
		return json.RawMessage(`{}`), nil
	}
}

func nonNullJSONSchema() json.RawMessage {
	return json.RawMessage(`{"not":{"type":"null"}}`)
}

func accessibleEnumTypeSchema(definition *ast.Document, enumTypeDefinitionRef int, nullable bool) (json.RawMessage, error) {
	if _, err := checkedDefinitionNodeName(definition, ast.Node{Kind: ast.NodeKindEnumTypeDefinition, Ref: enumTypeDefinitionRef}); err != nil {
		return nil, err
	}
	enumValueDefinitionRefs := definition.EnumTypeDefinitions[enumTypeDefinitionRef].EnumValuesDefinition.Refs
	values := make([]any, 0, len(enumValueDefinitionRefs)+1)
	for _, enumValueDefinitionRef := range enumValueDefinitionRefs {
		if err := checkedReference(enumValueDefinitionRef, len(definition.EnumValueDefinitions), "enum value definition reference"); err != nil {
			return nil, err
		}
		enumValueDefinition := definition.EnumValueDefinitions[enumValueDefinitionRef]
		inaccessible, err := checkedDefinitionDirectiveByName(
			definition,
			enumValueDefinition.Directives.Refs,
			federation.InaccessibleDirectiveNameBytes,
			"enum value directive",
		)
		if err != nil {
			return nil, err
		}
		if inaccessible {
			continue
		}
		name, err := checkedBytes(definition, enumValueDefinition.EnumValue, "enum value name")
		if err != nil {
			return nil, err
		}
		values = append(values, string(name))
	}

	types := []string{"string"}
	if nullable {
		types = append(types, "null")
		values = append(values, nil)
	}

	return marshalSchema(enumSchema{Type: types, Enum: values})
}

func selectedObjectTypeSchema(operation, definition *ast.Document, objectTypeDefinitionRef int, selectionSetRefs []int, nullable bool, options *options) (json.RawMessage, error) {
	objectTypeName, err := checkedDefinitionNodeName(definition, ast.Node{Kind: ast.NodeKindObjectTypeDefinition, Ref: objectTypeDefinitionRef})
	if err != nil {
		return nil, err
	}
	if len(selectionSetRefs) == 0 {
		return nil, fmt.Errorf(
			"object type %q requires a selection set",
			objectTypeName,
		)
	}
	leaveSelectionSets, err := options.traversal.enterSchemaSelectionSets(selectionSetRefs)
	if err != nil {
		return nil, err
	}
	defer leaveSelectionSets()

	properties := make(map[string]json.RawMessage)
	fieldGroups, err := selectedFieldGroups(operation, selectionSetRefs, options.traversal)
	if err != nil {
		return nil, err
	}
	fieldGroups, err = selectedFieldGroupsForRuntimeType(
		definition,
		fieldGroups,
		objectTypeName,
	)
	if err != nil {
		return nil, err
	}
	required := make([]string, 0, len(fieldGroups))
	for _, fieldGroup := range fieldGroups {
		fieldRef := fieldGroup.fields[0].ref
		responseName := fieldGroup.responseName
		fieldName, _, err := checkedOperationFieldNames(operation, fieldRef)
		if err != nil {
			return nil, err
		}
		if fieldName == "__typename" {
			for _, repeatedField := range fieldGroup.fields[1:] {
				repeatedFieldName, _, err := checkedOperationFieldNames(operation, repeatedField.ref)
				if err != nil {
					return nil, err
				}
				if repeatedFieldName != "__typename" {
					return nil, fmt.Errorf(
						"response property %q combines incompatible fields %q and %q",
						responseName,
						fieldName,
						repeatedFieldName,
					)
				}
			}
			propertySchema, err := marshalSchema(constStringSchema{
				Type:  "string",
				Const: objectTypeName,
			})
			if err != nil {
				return nil, fmt.Errorf("build schema for response property %q: %w", responseName, err)
			}
			properties[responseName] = propertySchema
			for _, field := range fieldGroup.fields {
				if !field.conditional {
					required = append(required, responseName)
					break
				}
			}
			continue
		}
		fieldDefinitionRef, ok, err := checkedFieldDefinitionOnNode(
			definition,
			ast.Node{Kind: ast.NodeKindObjectTypeDefinition, Ref: objectTypeDefinitionRef},
			[]byte(fieldName),
		)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf(
				"field %q is not defined on object type %q",
				fieldName,
				objectTypeName,
			)
		}

		childSelectionSetRefs := make([]int, 0, len(fieldGroup.fields))
		propertyRequired := false
		for _, repeatedField := range fieldGroup.fields {
			repeatedFieldRef := repeatedField.ref
			repeatedFieldName, _, err := checkedOperationFieldNames(operation, repeatedFieldRef)
			if err != nil {
				return nil, err
			}
			if repeatedFieldName != fieldName {
				return nil, fmt.Errorf(
					"response property %q combines incompatible fields %q and %q",
					responseName,
					fieldName,
					repeatedFieldName,
				)
			}
			if operation.Fields[repeatedFieldRef].HasSelections {
				childSelectionSetRefs = append(childSelectionSetRefs, operation.Fields[repeatedFieldRef].SelectionSet)
			}
			if !repeatedField.conditional {
				propertyRequired = true
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
		if propertyRequired {
			required = append(required, responseName)
		}
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
