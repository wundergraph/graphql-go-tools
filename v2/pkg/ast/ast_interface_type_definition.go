package ast

import (
	"bytes"
	"sort"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
)

// InterfaceTypeDefinition
// example:
//
//	interface NamedEntity {
//		name: String
//	}
type InterfaceTypeDefinition struct {
	Description                    Description        // optional, describes the interface
	InterfaceLiteral               position.Position  // interface
	Name                           ByteSliceReference // e.g. NamedEntity
	ImplementsInterfaces           TypeList           // e.g implements Bar & Baz
	HasDirectives                  bool
	Directives                     DirectiveList // optional, e.g. @foo
	HasFieldDefinitions            bool
	FieldsDefinition               FieldDefinitionList // optional, e.g. { name: String }
	ImplementedByObjectDefinitions []int               // list of ObjectTypeDefinition refs that implement this interface
}

func (d *Document) InterfaceTypeDefinitionNameBytes(ref int) ByteSlice {
	return d.Input.ByteSlice(d.InterfaceTypeDefinitions[ref].Name)
}

func (d *Document) InterfaceTypeDefinitionNameString(ref int) string {
	return unsafebytes.BytesToString(d.Input.ByteSlice(d.InterfaceTypeDefinitions[ref].Name))
}

func (d *Document) InterfaceTypeDefinitionDescriptionBytes(ref int) ByteSlice {
	if !d.InterfaceTypeDefinitions[ref].Description.IsDefined {
		return nil
	}
	return d.Input.ByteSlice(d.InterfaceTypeDefinitions[ref].Description.Content)
}

func (d *Document) InterfaceTypeDefinitionImplementsInterface(definitionRef int, interfaceName ByteSlice) bool {
	for _, iRef := range d.InterfaceTypeDefinitions[definitionRef].ImplementsInterfaces.Refs {
		implements := d.ResolveTypeNameBytes(iRef)
		if bytes.Equal(interfaceName, implements) {
			return true
		}
	}
	return false
}

func (d *Document) InterfaceTypeDefinitionDescriptionString(ref int) string {
	return unsafebytes.BytesToString(d.InterfaceTypeDefinitionDescriptionBytes(ref))
}

// InterfaceTypeDefinitionImplementedByRootNodes will return all RootNodes that implement the given interface type (by ref)
func (d *Document) InterfaceTypeDefinitionImplementedByRootNodes(ref int) []Node {
	interfaceTypeName := d.InterfaceTypeDefinitionNameBytes(ref)
	implementingRootNodes := make(map[Node]bool)
	for i := 0; i < len(d.RootNodes); i++ {
		if d.RootNodes[i].Kind == NodeKindInterfaceTypeDefinition && d.RootNodes[i].Ref == ref {
			continue
		}

		var rootNodeInterfaceRefs []int
		switch d.RootNodes[i].Kind {
		case NodeKindObjectTypeDefinition:
			if len(d.ObjectTypeDefinitions[d.RootNodes[i].Ref].ImplementsInterfaces.Refs) == 0 {
				continue
			}
			rootNodeInterfaceRefs = d.ObjectTypeDefinitions[d.RootNodes[i].Ref].ImplementsInterfaces.Refs
		case NodeKindInterfaceTypeDefinition:
			if len(d.InterfaceTypeDefinitions[d.RootNodes[i].Ref].ImplementsInterfaces.Refs) == 0 {
				continue
			}
			rootNodeInterfaceRefs = d.InterfaceTypeDefinitions[d.RootNodes[i].Ref].ImplementsInterfaces.Refs
		default:
			continue
		}

		for j := 0; j < len(rootNodeInterfaceRefs); j++ {
			implementedInterfaceTypeName := d.TypeNameBytes(rootNodeInterfaceRefs[j])
			if !interfaceTypeName.Equals(implementedInterfaceTypeName) {
				continue
			}

			var typeName ByteSlice
			switch d.RootNodes[i].Kind {
			case NodeKindObjectTypeDefinition:
				typeName = d.ObjectTypeDefinitionNameBytes(d.RootNodes[i].Ref)
			case NodeKindInterfaceTypeDefinition:
				typeName = d.InterfaceTypeDefinitionNameBytes(d.RootNodes[i].Ref)
			}

			node, exists := d.Index.FirstNodeByNameBytes(typeName)
			if !exists {
				continue
			}

			_, isAlreadyAdded := implementingRootNodes[node]
			if isAlreadyAdded {
				continue
			}

			implementingRootNodes[node] = true
		}
	}

	var nodes []Node
	for mapNode := range implementingRootNodes {
		nodes = append(nodes, mapNode)
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Ref < nodes[j].Ref
	})

	return nodes
}

func (d *Document) AddInterfaceTypeDefinition(definition InterfaceTypeDefinition) (ref int) {
	d.InterfaceTypeDefinitions = append(d.InterfaceTypeDefinitions, definition)
	return len(d.InterfaceTypeDefinitions) - 1
}

func (d *Document) ImportInterfaceTypeDefinition(name, description string, fieldRefs []int) (ref int) {
	return d.ImportInterfaceTypeDefinitionWithDirectives(name, description, fieldRefs, nil, nil)
}

func (d *Document) ImportInterfaceTypeDefinitionWithDirectives(name, description string, fieldRefs []int, iRefs []int, directiveRefs []int) (ref int) {
	definition := InterfaceTypeDefinition{
		Name:        d.Input.AppendInputString(name),
		Description: d.ImportDescription(description),
		FieldsDefinition: FieldDefinitionList{
			Refs: fieldRefs,
		},
		ImplementsInterfaces: TypeList{
			Refs: iRefs,
		},
		HasFieldDefinitions: len(fieldRefs) > 0,
		HasDirectives:       len(directiveRefs) > 0,
		Directives: DirectiveList{
			Refs: directiveRefs,
		},
	}

	ref = d.AddInterfaceTypeDefinition(definition)
	d.ImportRootNode(ref, NodeKindInterfaceTypeDefinition)

	return
}

func (d *Document) InterfaceTypeDefinitionFieldWithName(ref int, fieldName []byte) (fieldDefRef int, ok bool) {
	for _, fieldDefinitionRef := range d.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs {
		currentFieldName := d.FieldDefinitionNameBytes(fieldDefinitionRef)
		if currentFieldName.Equals(fieldName) {
			return fieldDefinitionRef, true
		}
	}
	return InvalidRef, false
}

// InterfaceTypeDefinitionImplementedByObjectWithNames returns object type names implementing the interface.
func (d *Document) InterfaceTypeDefinitionImplementedByObjectWithNames(interfaceDefRef int) (typeNames []string, ok bool) {
	implementedByObjectRefs := d.InterfaceTypeDefinitions[interfaceDefRef].ImplementedByObjectDefinitions
	if len(implementedByObjectRefs) == 0 {
		// fallback for documents not precalculated (e.g., built programmatically)
		for _, n := range d.InterfaceTypeDefinitionImplementedByRootNodes(interfaceDefRef) {
			if n.Kind == NodeKindObjectTypeDefinition {
				typeNames = append(typeNames, d.ObjectTypeDefinitionNameString(n.Ref))
			}
		}
	} else {
		for _, implementedByObjectRef := range d.InterfaceTypeDefinitions[interfaceDefRef].ImplementedByObjectDefinitions {
			typeNames = append(typeNames, d.ObjectTypeDefinitionNameString(implementedByObjectRef))
		}
	}

	if len(typeNames) > 0 {
		return typeNames, true

	}

	return nil, false
}

func (d *Document) InterfaceTypeDefinitionImplementedByObjectWithNamesAsBytes(interfaceDefRef int) (typeNames [][]byte, ok bool) {
	implementedByObjectRefs := d.InterfaceTypeDefinitions[interfaceDefRef].ImplementedByObjectDefinitions
	if len(implementedByObjectRefs) == 0 {
		// fallback for documents not precalculated (e.g., built programmatically)
		for _, n := range d.InterfaceTypeDefinitionImplementedByRootNodes(interfaceDefRef) {
			if n.Kind == NodeKindObjectTypeDefinition {
				typeNames = append(typeNames, d.ObjectTypeDefinitionNameBytes(n.Ref))
			}
		}
	} else {
		for _, implementedByObjectRef := range d.InterfaceTypeDefinitions[interfaceDefRef].ImplementedByObjectDefinitions {
			typeNames = append(typeNames, d.ObjectTypeDefinitionNameBytes(implementedByObjectRef))
		}
	}

	if len(typeNames) > 0 {
		return typeNames, true

	}

	return nil, false
}

func (d *Document) PopulateInterfaceTypeDefinitionImplementedByObjects() {
	for ref := 0; ref < len(d.InterfaceTypeDefinitions); ref++ {
		implementedByNodes := d.InterfaceTypeDefinitionImplementedByRootNodes(ref)
		implementedByObjectRefs := make([]int, 0, len(implementedByNodes))
		for _, implementedByNode := range implementedByNodes {
			if implementedByNode.Kind != NodeKindObjectTypeDefinition {
				continue
			}

			implementedByObjectRefs = append(implementedByObjectRefs, implementedByNode.Ref)
		}
		d.InterfaceTypeDefinitions[ref].ImplementedByObjectDefinitions = implementedByObjectRefs
	}
}
