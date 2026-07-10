package responsejsonschema

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation"
)

type oneOfSchema struct {
	OneOf []json.RawMessage `json:"oneOf"`
}

func selectedInterfaceTypeSchema(
	operation, definition *ast.Document,
	interfaceTypeDefinitionRef int,
	selectionSetRefs []int,
	nullable bool,
	options *options,
) (json.RawMessage, error) {
	typeNames, _ := definition.InterfaceTypeDefinitionImplementedByObjectWithNames(interfaceTypeDefinitionRef)
	return selectedAbstractTypeSchema(
		operation,
		definition,
		definition.InterfaceTypeDefinitionNameString(interfaceTypeDefinitionRef),
		typeNames,
		selectionSetRefs,
		nullable,
		options,
	)
}

func selectedUnionTypeSchema(
	operation, definition *ast.Document,
	unionTypeDefinitionRef int,
	selectionSetRefs []int,
	nullable bool,
	options *options,
) (json.RawMessage, error) {
	typeNames, _ := definition.UnionTypeDefinitionMemberTypeNames(unionTypeDefinitionRef)
	return selectedAbstractTypeSchema(
		operation,
		definition,
		definition.UnionTypeDefinitionNameString(unionTypeDefinitionRef),
		typeNames,
		selectionSetRefs,
		nullable,
		options,
	)
}

func selectedAbstractTypeSchema(
	operation, definition *ast.Document,
	abstractTypeName string,
	possibleTypeNames []string,
	selectionSetRefs []int,
	nullable bool,
	options *options,
) (json.RawMessage, error) {
	if len(selectionSetRefs) == 0 {
		return nil, fmt.Errorf("abstract type %q requires a selection set", abstractTypeName)
	}

	sort.Strings(possibleTypeNames)
	variants := make([]json.RawMessage, 0, len(possibleTypeNames))
	for _, typeName := range possibleTypeNames {
		typeNode, ok := definition.Index.FirstNodeByNameStr(typeName)
		if !ok || typeNode.Kind != ast.NodeKindObjectTypeDefinition {
			return nil, fmt.Errorf("possible type %q of abstract type %q is not an object type", typeName, abstractTypeName)
		}
		if objectTypeIsInaccessible(definition, typeNode.Ref) {
			continue
		}

		variant, err := selectedObjectTypeSchema(operation, definition, typeNode.Ref, selectionSetRefs, false, options)
		if err != nil {
			return nil, fmt.Errorf("build variant %q for abstract type %q: %w", typeName, abstractTypeName, err)
		}
		variants = append(variants, variant)
	}
	if len(variants) == 0 {
		return nil, fmt.Errorf("abstract type %q has no accessible concrete variants", abstractTypeName)
	}

	variantSchema, err := marshalSchema(oneOfSchema{OneOf: variants})
	if err != nil {
		return nil, fmt.Errorf("build variants for abstract type %q: %w", abstractTypeName, err)
	}
	if !nullable {
		return variantSchema, nil
	}

	return marshalSchema(anyOfSchema{AnyOf: []json.RawMessage{
		variantSchema,
		json.RawMessage(`{"type":"null"}`),
	}})
}

func objectTypeIsInaccessible(definition *ast.Document, objectTypeDefinitionRef int) bool {
	objectType := definition.ObjectTypeDefinitions[objectTypeDefinitionRef]
	return objectType.HasDirectives && objectType.Directives.HasDirectiveByName(definition, string(federation.InaccessibleDirectiveNameBytes))
}
