package astvalidation

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func ImplementTransitiveInterfaces() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := &implementTransitiveInterfacesVisitor{
			Walker: walker,
		}

		walker.RegisterDocumentVisitor(visitor)
		walker.RegisterEnterInterfaceTypeDefinitionVisitor(visitor)
		walker.RegisterEnterInterfaceTypeExtensionVisitor(visitor)
		walker.RegisterEnterObjectTypeDefinitionVisitor(visitor)
		walker.RegisterEnterObjectTypeExtensionVisitor(visitor)
	}
}

type implementTransitiveInterfacesVisitor struct {
	*astvisitor.Walker
	definition                  *ast.Document
	typesImplementingInterfaces map[string][]string
}

func (v *implementTransitiveInterfacesVisitor) EnterDocument(operation, definition *ast.Document) {
	v.definition = operation
	v.typesImplementingInterfaces = map[string][]string{}
}

// LeaveDocument will iterate over the types implementing interfaces lookup map
// and check if a types with interfaces has all the transitive interfaces in their slice.
//
// Valid (typeName contains interfaceBase from interfaceOne):
//
//	typeName -> [interfaceOne, interfaceBase]
//	interfaceOne -> [interfaceBase]
//
// Invalid (typeName does not contain interfaceBase from interfaceOne):
//
//	typeName -> [interfaceOne]
//	interfaceOne -> [interfaceBase]
func (v *implementTransitiveInterfacesVisitor) LeaveDocument(operation, definition *ast.Document) {
	for typeName, interfaceNames := range v.typesImplementingInterfaces {
		interfaceNamesLookupList := map[string]bool{}
		for i := 0; i < len(interfaceNames); i++ {
			interfaceNamesLookupList[interfaceNames[i]] = true
		}

		for i := 0; i < len(interfaceNames); i++ {
			implementedInterfaceName := interfaceNames[i]
			if _, ok := v.typesImplementingInterfaces[implementedInterfaceName]; !ok {
				continue
			}

			for j := 0; j < len(v.typesImplementingInterfaces[implementedInterfaceName]); j++ {
				transitiveInterfaceName := v.typesImplementingInterfaces[implementedInterfaceName][j]
				if _, ok := interfaceNamesLookupList[transitiveInterfaceName]; !ok {
					v.Report.AddExternalError(operationreport.ErrTransitiveInterfaceNotImplemented([]byte(typeName), []byte(transitiveInterfaceName)))
				}
			}
		}
	}
}

func (v *implementTransitiveInterfacesVisitor) EnterInterfaceTypeDefinition(ref int) {
	implementsInterfaces := len(v.definition.InterfaceTypeDefinitions[ref].ImplementsInterfaces.Refs) > 0
	if !implementsInterfaces {
		return
	}

	interfaceName := v.definition.InterfaceTypeDefinitionNameString(ref)
	v.collectImplementedInterfaces(interfaceName, v.definition.InterfaceTypeDefinitions[ref].ImplementsInterfaces.Refs)
}

func (v *implementTransitiveInterfacesVisitor) EnterInterfaceTypeExtension(ref int) {
	implementsInterfaces := len(v.definition.InterfaceTypeExtensions[ref].ImplementsInterfaces.Refs) > 0
	if !implementsInterfaces {
		return
	}

	interfaceName := v.definition.InterfaceTypeExtensionNameString(ref)
	fieldDefinitionRefs := v.definition.InterfaceTypeExtensions[ref].FieldsDefinition.Refs
	if len(fieldDefinitionRefs) == 0 {
		v.Report.AddExternalError(operationreport.ErrTransitiveInterfaceExtensionImplementingWithoutBody([]byte(interfaceName)))
	}
	v.collectImplementedInterfaces(interfaceName, v.definition.InterfaceTypeExtensions[ref].ImplementsInterfaces.Refs)
}

func (v *implementTransitiveInterfacesVisitor) EnterObjectTypeDefinition(ref int) {
	implementsInterfaces := len(v.definition.ObjectTypeDefinitions[ref].ImplementsInterfaces.Refs) > 0
	if !implementsInterfaces {
		return
	}

	objectTypeName := v.definition.ObjectTypeDefinitionNameString(ref)
	v.collectImplementedInterfaces(objectTypeName, v.definition.ObjectTypeDefinitions[ref].ImplementsInterfaces.Refs)
}

func (v *implementTransitiveInterfacesVisitor) EnterObjectTypeExtension(ref int) {
	implementsInterfaces := len(v.definition.ObjectTypeExtensions[ref].ImplementsInterfaces.Refs) > 0
	if !implementsInterfaces {
		return
	}

	objectTypeName := v.definition.ObjectTypeExtensionNameString(ref)
	v.collectImplementedInterfaces(objectTypeName, v.definition.ObjectTypeExtensions[ref].ImplementsInterfaces.Refs)
}

// collectImplementedInterfaces iterates over all implemented interfaces over a given type so that the
// names can be saved into the lookup map on the visitor
//
// Result:
//
//	typeName -> [interfaceOne, interfaceBase]
//	interfaceOne -> [interfaceBase]
func (v *implementTransitiveInterfacesVisitor) collectImplementedInterfaces(typeName string, implementedInterfacesRefs []int) {
	for i := 0; i < len(implementedInterfacesRefs); i++ {
		implementedInterfaceRef := implementedInterfacesRefs[i]
		implementedInterfaceName := v.definition.TypeNameString(implementedInterfaceRef)

		if _, ok := v.typesImplementingInterfaces[typeName]; !ok {
			v.typesImplementingInterfaces[typeName] = []string{implementedInterfaceName}
		}

		skipInterface := false
		for j := 0; j < len(v.typesImplementingInterfaces[typeName]); j++ {
			if v.typesImplementingInterfaces[typeName][j] == implementedInterfaceName {
				skipInterface = true
				break
			}
		}

		if !skipInterface {
			v.typesImplementingInterfaces[typeName] = append(v.typesImplementingInterfaces[typeName], implementedInterfaceName)
		}
	}
}
