package astvalidation

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
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
	definition                           *ast.Document
	implementingTypesWithFields          map[string][]string
	implementingTypesWithInterfacesNames map[string][]string
}

func (v *implementingTypesAreSupersetsVisitor) EnterDocument(operation, definition *ast.Document) {
	v.definition = operation
	v.implementingTypesWithFields = make(map[string][]string)
	v.implementingTypesWithInterfacesNames = make(map[string][]string)
}

// LeaveDocument will iterate over all types which implement an interface by using the interface name. If a
// field does exist on the implemented interface but not on the implementing type, then the rule will consider it
// as invalid.
//
// Valid:
//
//	( interfaceBase -> [fieldA] )
//	interfaceOneImplementingInterfaceBase -> [fieldA, fieldB]
//	objectTypeImplementingInterfaceOne -> [fieldA, fieldB, fieldC]
//
// Invalid:
//
//	( interfaceBase -> [fieldA] )
//	interfaceOneImplementingInterfaceBase -> [fieldA, fieldB]
//	objectTypeImplementingInterfaceOne -> [fieldA, fieldC]
func (v *implementingTypesAreSupersetsVisitor) LeaveDocument(operation, definition *ast.Document) {
	for typeName, interfacesNames := range v.implementingTypesWithInterfacesNames {
		typeNameHasFields := true
		typeNameFields, exists := v.implementingTypesWithFields[typeName]
		if !exists || len(typeNameFields) == 0 {
			typeNameHasFields = false
		}

		typeNameFieldsLookupMap := map[string]bool{}
		for i := 0; i < len(typeNameFields); i++ {
			typeNameFieldsLookupMap[typeNameFields[i]] = true
		}

		for i := 0; i < len(interfacesNames); i++ {
			nodes, exists := v.definition.Index.NodesByNameStr(interfacesNames[i])
			if !exists {
				continue
			}

			var interfaceFieldRefs []int
			for j := 0; j < len(nodes); j++ {
				switch nodes[j].Kind {
				case ast.NodeKindInterfaceTypeDefinition:
					interfaceFieldRefs = append(interfaceFieldRefs, v.definition.InterfaceTypeDefinitions[nodes[j].Ref].FieldsDefinition.Refs...)
				case ast.NodeKindInterfaceTypeExtension:
					interfaceFieldRefs = append(interfaceFieldRefs, v.definition.InterfaceTypeExtensions[nodes[j].Ref].FieldsDefinition.Refs...)
				default:
					continue
				}
			}

			if !typeNameHasFields && len(interfaceFieldRefs) > 0 {
				v.Report.AddExternalError(operationreport.ErrImplementingTypeDoesNotHaveFields([]byte(typeName)))
				continue
			}

			for j := 0; j < len(interfaceFieldRefs); j++ {
				interfaceFieldName := v.definition.FieldDefinitionNameString(interfaceFieldRefs[j])
				if existsOnType := typeNameFieldsLookupMap[interfaceFieldName]; !existsOnType {
					v.Report.AddExternalError(operationreport.ErrTypeDoesNotImplementFieldFromInterface(
						[]byte(typeName),
						[]byte(interfacesNames[i]),
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
	v.collectInterfaceNamesForImplementedInterfacesByTypeName(typeName, interfacesRefs)
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
	v.collectInterfaceNamesForImplementedInterfacesByTypeName(typeName, interfacesRefs)
}

func (v *implementingTypesAreSupersetsVisitor) EnterObjectTypeDefinition(ref int) {
	interfacesRefs := v.definition.ObjectTypeDefinitions[ref].ImplementsInterfaces.Refs
	if len(interfacesRefs) == 0 {
		return
	}

	typeName := v.definition.ObjectTypeDefinitionNameString(ref)
	fieldDefinitionRefs := v.definition.ObjectTypeDefinitions[ref].FieldsDefinition.Refs
	v.collectFieldsForTypeName(typeName, fieldDefinitionRefs)
	v.collectInterfaceNamesForImplementedInterfacesByTypeName(typeName, interfacesRefs)
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
	v.collectInterfaceNamesForImplementedInterfacesByTypeName(typeName, interfacesRefs)
}

// collectFieldsForTypeName will add all field names of a type which implements an interface to a slice in a
// map entry, so that it can be used as a lookup table later on.
//
// Example:
//
//	interfaceOne -> [fieldA, fieldB]
//	objectType -> [fieldA, fieldB, fieldC]
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

// collectInterfaceNamesForImplementedInterfacesByTypeName will add all interface names implemented by the given type,
// so it can be used to iterate over them when leaving the document.
//
// Example:
//
//	interfaceOne -> [interfaceBase]
//	objectType -> [interfaceOne, interfaceBase]
func (v *implementingTypesAreSupersetsVisitor) collectInterfaceNamesForImplementedInterfacesByTypeName(typeName string, typeRefs []int) {
	if _, ok := v.implementingTypesWithInterfacesNames[typeName]; !ok {
		v.implementingTypesWithInterfacesNames[typeName] = []string{}
	}

	for i := 0; i < len(typeRefs); i++ {
		interfaceName := v.definition.TypeNameString(typeRefs[i])
		skipInterfaceName := false
		for j := 0; j < len(v.implementingTypesWithInterfacesNames[typeName]); j++ {
			if interfaceName == v.implementingTypesWithInterfacesNames[typeName][j] {
				skipInterfaceName = true
				break
			}
		}

		if skipInterfaceName {
			continue
		}

		v.implementingTypesWithInterfacesNames[typeName] = append(v.implementingTypesWithInterfacesNames[typeName], interfaceName)
	}
}
