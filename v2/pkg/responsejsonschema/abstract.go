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
	interfaceNode := ast.Node{Kind: ast.NodeKindInterfaceTypeDefinition, Ref: interfaceTypeDefinitionRef}
	interfaceName, err := checkedDefinitionNodeName(definition, interfaceNode)
	if err != nil {
		return nil, err
	}
	domain, err := checkedPossibleRuntimeTypes(definition, interfaceNode)
	if err != nil {
		return nil, err
	}
	return selectedAbstractTypeSchema(
		operation,
		definition,
		interfaceName,
		sortedRuntimeTypeDomain(domain),
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
	unionNode := ast.Node{Kind: ast.NodeKindUnionTypeDefinition, Ref: unionTypeDefinitionRef}
	unionName, err := checkedDefinitionNodeName(definition, unionNode)
	if err != nil {
		return nil, err
	}
	domain, err := checkedPossibleRuntimeTypes(definition, unionNode)
	if err != nil {
		return nil, err
	}
	return selectedAbstractTypeSchema(
		operation,
		definition,
		unionName,
		sortedRuntimeTypeDomain(domain),
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
		typeNode, ok, err := checkedIndexNode(definition, typeName)
		if err != nil {
			return nil, err
		}
		if !ok || typeNode.Kind != ast.NodeKindObjectTypeDefinition {
			return nil, fmt.Errorf("possible type %q of abstract type %q is not an object type", typeName, abstractTypeName)
		}
		checkedTypeName, err := checkedDefinitionNodeName(definition, typeNode)
		if err != nil {
			return nil, err
		}
		if checkedTypeName != typeName {
			return nil, fmt.Errorf("possible type %q has inconsistent object type name %q", typeName, checkedTypeName)
		}
		inaccessible, err := checkedObjectTypeIsInaccessible(definition, typeNode.Ref, federation.InaccessibleDirectiveNameBytes)
		if err != nil {
			return nil, err
		}
		if inaccessible {
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
