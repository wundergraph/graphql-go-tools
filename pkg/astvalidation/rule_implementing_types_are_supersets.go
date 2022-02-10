package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func ImplementingTypesAreSupersets() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := &implementingTypesAreSupersetsVisitor{
			Walker: walker,
		}

		walker.RegisterDocumentVisitor(visitor)
		walker.RegisterEnterInterfaceTypeDefinitionVisitor(visitor)
		walker.RegisterEnterInterfaceTypeExtensionVisitor(visitor)
		walker.RegisterEnterObjectTypeDefinitionVisitor(visitor)
		walker.RegisterEnterObjectTypeExtensionVisitor(visitor)
	}
}

type implementingTypesAreSupersetsVisitor struct {
	*astvisitor.Walker
	definition                    *ast.Document
	implementingTypesWithFields   map[string][]string
	implementingTypesWithTypeRefs map[string][]int
}

func (v *implementingTypesAreSupersetsVisitor) EnterDocument(operation, definition *ast.Document) {
	v.definition = operation
	v.implementingTypesWithFields = make(map[string][]string)
	v.implementingTypesWithTypeRefs = make(map[string][]int)
}

func (v *implementingTypesAreSupersetsVisitor) LeaveDocument(operation, definition *ast.Document) {
	for typeName, interfacesTypeRefs := range v.implementingTypesWithTypeRefs {
		typeNameFields, ok := v.implementingTypesWithFields[typeName]
		if !ok {
			// error because has no fields?
		}

		typeNameFieldsLookupMap := map[string]bool{}
		for i := 0; i < len(typeNameFields); i++ {
			typeNameFieldsLookupMap[typeNameFields[i]] = true
		}

		for i := 0; i < len(interfacesTypeRefs); i++ {
			interfaceNameBytes := v.definition.TypeNameBytes(interfacesTypeRefs[i])
			nodes, exists := v.definition.Index.NodesByNameBytes(interfaceNameBytes)
			if !exists {
				continue
			}

			var fieldRefs []int
			for j := 0; j < len(nodes); j++ {
				switch nodes[j].Kind {
				case ast.NodeKindInterfaceTypeDefinition:
					fieldRefs = append(fieldRefs, v.definition.InterfaceTypeDefinitions[nodes[j].Ref].FieldsDefinition.Refs...)
				case ast.NodeKindInterfaceTypeExtension:
					fieldRefs = append(fieldRefs, v.definition.InterfaceTypeExtensions[nodes[j].Ref].FieldsDefinition.Refs...)
				default:
					continue
				}
			}

			for j := 0; j < len(fieldRefs); j++ {
				interfaceFieldName := v.definition.FieldDefinitionNameString(fieldRefs[j])
				if existsOnType := typeNameFieldsLookupMap[interfaceFieldName]; !existsOnType {
					v.Report.AddExternalError(operationreport.ErrTypeDoesNotImplementFieldFromInterface(
						[]byte(typeName),
						interfaceNameBytes,
						[]byte(interfaceFieldName),
					))
				}
			}
		}
	}
}

func (v *implementingTypesAreSupersetsVisitor) EnterInterfaceTypeDefinition(ref int) {
	interfacesRefs := v.definition.InterfaceTypeDefinitions[ref].ImplementsInterfaces.Refs
	if len(interfacesRefs) == 0 {
		return
	}

	typeName := v.definition.InterfaceTypeDefinitionNameString(ref)
	fieldDefinitionRefs := v.definition.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs
	v.collectFieldsForTypeName(typeName, fieldDefinitionRefs)
	v.collectTypeRefsForImplementedInterfacesByTypeName(typeName, interfacesRefs)
}

func (v *implementingTypesAreSupersetsVisitor) EnterInterfaceTypeExtension(ref int) {
	interfacesRefs := v.definition.InterfaceTypeExtensions[ref].ImplementsInterfaces.Refs
	if len(interfacesRefs) == 0 {
		return
	}

	typeName := v.definition.InterfaceTypeExtensionNameString(ref)
	fieldDefinitionRefs := v.definition.InterfaceTypeExtensions[ref].FieldsDefinition.Refs

	nodesWithTypeName, exists := v.definition.Index.NodesByNameStr(typeName)
	if !exists {
		return // if exists is false then something is really wrong
	}

	for i := 0; i < len(nodesWithTypeName); i++ {
		switch nodesWithTypeName[i].Kind {
		case ast.NodeKindInterfaceTypeDefinition:
			baseInterfaceRef := nodesWithTypeName[i].Ref
			baseInterfaceTypeFieldRefs := v.definition.InterfaceTypeDefinitions[baseInterfaceRef].FieldsDefinition.Refs
			for j := 0; j < len(baseInterfaceTypeFieldRefs); j++ {
				fieldDefinitionRefs = append(fieldDefinitionRefs, baseInterfaceTypeFieldRefs[j])
			}
		default:
			continue
		}
	}

	v.collectFieldsForTypeName(typeName, fieldDefinitionRefs)
	v.collectTypeRefsForImplementedInterfacesByTypeName(typeName, interfacesRefs)
}

func (v *implementingTypesAreSupersetsVisitor) EnterObjectTypeDefinition(ref int) {
	interfacesRefs := v.definition.ObjectTypeDefinitions[ref].ImplementsInterfaces.Refs
	if len(interfacesRefs) == 0 {
		return
	}

	typeName := v.definition.ObjectTypeDefinitionNameString(ref)
	fieldDefinitionRefs := v.definition.ObjectTypeDefinitions[ref].FieldsDefinition.Refs
	v.collectFieldsForTypeName(typeName, fieldDefinitionRefs)
	v.collectTypeRefsForImplementedInterfacesByTypeName(typeName, interfacesRefs)
}

func (v *implementingTypesAreSupersetsVisitor) EnterObjectTypeExtension(ref int) {
	interfacesRefs := v.definition.ObjectTypeExtensions[ref].ImplementsInterfaces.Refs
	if len(interfacesRefs) == 0 {
		return
	}

	typeName := v.definition.ObjectTypeExtensionNameString(ref)
	fieldDefinitionRefs := v.definition.ObjectTypeExtensions[ref].FieldsDefinition.Refs

	nodesWithTypeName, exists := v.definition.Index.NodesByNameStr(typeName)
	if !exists {
		return // if exists is false then something is really wrong
	}

	for i := 0; i < len(nodesWithTypeName); i++ {
		switch nodesWithTypeName[i].Kind {
		case ast.NodeKindObjectTypeDefinition:
			baseObjectTypeRef := nodesWithTypeName[i].Ref
			baseObjectTypeInterfaceRefs := v.definition.ObjectTypeDefinitions[baseObjectTypeRef].FieldsDefinition.Refs
			for j := 0; j < len(baseObjectTypeInterfaceRefs); j++ {
				fieldDefinitionRefs = append(fieldDefinitionRefs, baseObjectTypeInterfaceRefs[j])
			}
		default:
			continue
		}
	}

	v.collectFieldsForTypeName(typeName, fieldDefinitionRefs)
	v.collectTypeRefsForImplementedInterfacesByTypeName(typeName, interfacesRefs)
}

func (v *implementingTypesAreSupersetsVisitor) collectFieldsForTypeName(typeName string, fieldDefinitionRefs []int) {
	if _, ok := v.implementingTypesWithFields[typeName]; !ok {
		v.implementingTypesWithFields[typeName] = []string{}
	}

	for i := 0; i < len(fieldDefinitionRefs); i++ {
		fieldName := v.definition.FieldDefinitionNameString(fieldDefinitionRefs[i])

		skipFieldName := false
		for j := 0; j < len(v.implementingTypesWithFields[typeName]); j++ {
			if fieldName == v.implementingTypesWithFields[typeName][j] {
				skipFieldName = true
				break
			}
		}

		if skipFieldName {
			continue
		}

		v.implementingTypesWithFields[typeName] = append(v.implementingTypesWithFields[typeName], fieldName)
	}
}

func (v *implementingTypesAreSupersetsVisitor) collectTypeRefsForImplementedInterfacesByTypeName(typeName string, typeRefs []int) {
	if _, ok := v.implementingTypesWithTypeRefs[typeName]; !ok {
		v.implementingTypesWithTypeRefs[typeName] = []int{}
	}

	for i := 0; i < len(typeRefs); i++ {
		skipTypeRef := false
		for j := 0; j < len(v.implementingTypesWithTypeRefs[typeName]); j++ {
			if typeRefs[i] == v.implementingTypesWithTypeRefs[typeName][j] {
				skipTypeRef = true
				break
			}
		}

		if skipTypeRef {
			continue
		}

		v.implementingTypesWithTypeRefs[typeName] = append(v.implementingTypesWithTypeRefs[typeName], typeRefs[i])
	}
}
