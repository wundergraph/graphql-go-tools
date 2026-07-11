package responsejsonschema

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func checkedReference(reference, length int, description string) error {
	if reference < 0 || reference >= length {
		return fmt.Errorf("%s %d is out of bounds", description, reference)
	}
	return nil
}

func checkedBytes(document *ast.Document, reference ast.ByteSliceReference, description string) ([]byte, error) {
	if reference.End < reference.Start || uint64(reference.End) > uint64(len(document.Input.RawBytes)) {
		return nil, fmt.Errorf(
			"%s byte range [%d:%d] is out of bounds for input length %d",
			description,
			reference.Start,
			reference.End,
			len(document.Input.RawBytes),
		)
	}
	return document.Input.RawBytes[reference.Start:reference.End], nil
}

func checkedOperationFieldNames(operation *ast.Document, fieldRef int) (name, responseName string, err error) {
	if err := checkedReference(fieldRef, len(operation.Fields), "field reference"); err != nil {
		return "", "", err
	}
	field := operation.Fields[fieldRef]
	nameBytes, err := checkedBytes(operation, field.Name, "field name")
	if err != nil {
		return "", "", err
	}
	responseNameBytes := nameBytes
	if field.Alias.IsDefined {
		responseNameBytes, err = checkedBytes(operation, field.Alias.Name, "field alias")
		if err != nil {
			return "", "", err
		}
	}
	return string(nameBytes), string(responseNameBytes), nil
}

func checkedOperationTypeName(operation *ast.Document, typeRef int, description string) (string, error) {
	if err := checkedReference(typeRef, len(operation.Types), description+" reference"); err != nil {
		return "", err
	}
	typeNode := operation.Types[typeRef]
	if typeNode.TypeKind != ast.TypeKindNamed {
		return "", fmt.Errorf("%s reference %d is not a named type", description, typeRef)
	}
	name, err := checkedBytes(operation, typeNode.Name, description+" name")
	if err != nil {
		return "", err
	}
	return string(name), nil
}

func checkedDefinitionTypeName(definition *ast.Document, typeRef int, description string) (string, error) {
	visited := make(map[int]struct{})
	for {
		if len(visited) >= responseSchemaTraversalDepthLimit {
			return "", fmt.Errorf(
				"response schema recursion depth limit %d exceeded while traversing %s nodes",
				responseSchemaTraversalDepthLimit,
				description,
			)
		}
		if err := checkedReference(typeRef, len(definition.Types), description+" reference"); err != nil {
			return "", err
		}
		if _, exists := visited[typeRef]; exists {
			return "", fmt.Errorf("%s contains a type node cycle at reference %d", description, typeRef)
		}
		visited[typeRef] = struct{}{}

		typeNode := definition.Types[typeRef]
		switch typeNode.TypeKind {
		case ast.TypeKindNamed:
			name, err := checkedBytes(definition, typeNode.Name, description+" name")
			if err != nil {
				return "", err
			}
			return string(name), nil
		case ast.TypeKindList, ast.TypeKindNonNull:
			if err := checkedReference(typeNode.OfType, len(definition.Types), description+" inner type reference"); err != nil {
				return "", err
			}
			typeRef = typeNode.OfType
		default:
			return "", fmt.Errorf("%s reference %d has unsupported kind %q", description, typeRef, typeNode.TypeKind)
		}
	}
}

func checkedDefinitionNodeName(definition *ast.Document, node ast.Node) (string, error) {
	var reference ast.ByteSliceReference
	var description string
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		if err := checkedReference(node.Ref, len(definition.ObjectTypeDefinitions), "object type reference"); err != nil {
			return "", err
		}
		reference = definition.ObjectTypeDefinitions[node.Ref].Name
		description = "object type name"
	case ast.NodeKindInterfaceTypeDefinition:
		if err := checkedReference(node.Ref, len(definition.InterfaceTypeDefinitions), "interface type reference"); err != nil {
			return "", err
		}
		reference = definition.InterfaceTypeDefinitions[node.Ref].Name
		description = "interface type name"
	case ast.NodeKindUnionTypeDefinition:
		if err := checkedReference(node.Ref, len(definition.UnionTypeDefinitions), "union type reference"); err != nil {
			return "", err
		}
		reference = definition.UnionTypeDefinitions[node.Ref].Name
		description = "union type name"
	case ast.NodeKindEnumTypeDefinition:
		if err := checkedReference(node.Ref, len(definition.EnumTypeDefinitions), "enum type reference"); err != nil {
			return "", err
		}
		reference = definition.EnumTypeDefinitions[node.Ref].Name
		description = "enum type name"
	case ast.NodeKindScalarTypeDefinition:
		if err := checkedReference(node.Ref, len(definition.ScalarTypeDefinitions), "scalar type reference"); err != nil {
			return "", err
		}
		reference = definition.ScalarTypeDefinitions[node.Ref].Name
		description = "scalar type name"
	default:
		return "", fmt.Errorf("unsupported response type node kind %q", node.Kind)
	}
	name, err := checkedBytes(definition, reference, description)
	if err != nil {
		return "", err
	}
	return string(name), nil
}

func checkedIndexNode(definition *ast.Document, typeName string) (ast.Node, bool, error) {
	node, exists := definition.Index.FirstNodeByNameStr(typeName)
	if !exists {
		return ast.InvalidNode, false, nil
	}
	actualName, err := checkedDefinitionNodeName(definition, node)
	if err != nil {
		return ast.InvalidNode, false, err
	}
	if actualName != typeName {
		return ast.InvalidNode, false, fmt.Errorf(
			"index lookup for type %q returned node named %q",
			typeName,
			actualName,
		)
	}
	return node, true, nil
}

func checkedFieldDefinitionOnNode(definition *ast.Document, node ast.Node, fieldName []byte) (int, bool, error) {
	if _, err := checkedDefinitionNodeName(definition, node); err != nil {
		return ast.InvalidRef, false, err
	}

	var fieldDefinitionRefs []int
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		fieldDefinitionRefs = definition.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs
	case ast.NodeKindInterfaceTypeDefinition:
		fieldDefinitionRefs = definition.InterfaceTypeDefinitions[node.Ref].FieldsDefinition.Refs
	default:
		return ast.InvalidRef, false, nil
	}

	for _, fieldDefinitionRef := range fieldDefinitionRefs {
		if err := checkedReference(fieldDefinitionRef, len(definition.FieldDefinitions), "field definition reference"); err != nil {
			return ast.InvalidRef, false, err
		}
		candidateName, err := checkedBytes(definition, definition.FieldDefinitions[fieldDefinitionRef].Name, "field definition name")
		if err != nil {
			return ast.InvalidRef, false, err
		}
		if bytes.Equal(candidateName, fieldName) {
			return fieldDefinitionRef, true, nil
		}
	}
	return ast.InvalidRef, false, nil
}

func checkedPossibleRuntimeTypes(definition *ast.Document, node ast.Node) (runtimeTypeDomain, error) {
	domain := make(runtimeTypeDomain)
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		name, err := checkedDefinitionNodeName(definition, node)
		if err != nil {
			return nil, err
		}
		domain[name] = struct{}{}
	case ast.NodeKindInterfaceTypeDefinition:
		interfaceName, err := checkedDefinitionNodeName(definition, node)
		if err != nil {
			return nil, err
		}
		implementedBy := definition.InterfaceTypeDefinitions[node.Ref].ImplementedByObjectDefinitions
		if implementedBy != nil {
			for _, objectRef := range implementedBy {
				if err := checkedReference(objectRef, len(definition.ObjectTypeDefinitions), "implementing object reference"); err != nil {
					return nil, err
				}
				name, err := checkedDefinitionNodeName(definition, ast.Node{Kind: ast.NodeKindObjectTypeDefinition, Ref: objectRef})
				if err != nil {
					return nil, err
				}
				domain[name] = struct{}{}
			}
			break
		}

		for _, rootNode := range definition.RootNodes {
			var implementedInterfaces []int
			switch rootNode.Kind {
			case ast.NodeKindObjectTypeDefinition:
				if _, err := checkedDefinitionNodeName(definition, rootNode); err != nil {
					return nil, err
				}
				implementedInterfaces = definition.ObjectTypeDefinitions[rootNode.Ref].ImplementsInterfaces.Refs
			case ast.NodeKindInterfaceTypeDefinition:
				if _, err := checkedDefinitionNodeName(definition, rootNode); err != nil {
					return nil, err
				}
				implementedInterfaces = definition.InterfaceTypeDefinitions[rootNode.Ref].ImplementsInterfaces.Refs
			default:
				continue
			}
			for _, implementedInterfaceRef := range implementedInterfaces {
				implementedName, err := checkedDefinitionTypeName(definition, implementedInterfaceRef, "implemented interface type")
				if err != nil {
					return nil, err
				}
				if implementedName == interfaceName && rootNode.Kind == ast.NodeKindObjectTypeDefinition {
					name, err := checkedDefinitionNodeName(definition, rootNode)
					if err != nil {
						return nil, err
					}
					domain[name] = struct{}{}
				}
			}
		}
	case ast.NodeKindUnionTypeDefinition:
		if _, err := checkedDefinitionNodeName(definition, node); err != nil {
			return nil, err
		}
		union := definition.UnionTypeDefinitions[node.Ref]
		if union.HasUnionMemberTypes {
			for _, typeRef := range union.UnionMemberTypes.Refs {
				name, err := checkedDefinitionTypeName(definition, typeRef, "union member type")
				if err != nil {
					return nil, err
				}
				domain[name] = struct{}{}
			}
		}
	default:
		name, err := checkedDefinitionNodeName(definition, node)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("type %q is not a composite response type", name)
	}
	return domain, nil
}

func checkedDefinitionDirectiveByName(definition *ast.Document, directiveRefs []int, directiveName []byte, description string) (bool, error) {
	for _, directiveRef := range directiveRefs {
		if err := checkedReference(directiveRef, len(definition.Directives), description+" reference"); err != nil {
			return false, err
		}
		name, err := checkedBytes(definition, definition.Directives[directiveRef].Name, description+" name")
		if err != nil {
			return false, err
		}
		if bytes.Equal(name, directiveName) {
			return true, nil
		}
	}
	return false, nil
}

func checkedObjectTypeIsInaccessible(definition *ast.Document, objectTypeDefinitionRef int, directiveName []byte) (bool, error) {
	if err := checkedReference(objectTypeDefinitionRef, len(definition.ObjectTypeDefinitions), "object type reference"); err != nil {
		return false, err
	}
	object := definition.ObjectTypeDefinitions[objectTypeDefinitionRef]
	return checkedDefinitionDirectiveByName(definition, object.Directives.Refs, directiveName, "object directive")
}
