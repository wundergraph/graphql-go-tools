package ast

import (
	"bytes"
	"fmt"
	"log"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

type Node struct {
	Kind NodeKind
	Ref  int
}

var InvalidNode = Node{Kind: NodeKindUnknown, Ref: InvalidRef}

func (n *Node) IsExtensionKind() bool {
	switch n.Kind {
	case NodeKindSchemaExtension,
		NodeKindObjectTypeExtension,
		NodeKindInputObjectTypeExtension,
		NodeKindInterfaceTypeExtension,
		NodeKindEnumTypeExtension,
		NodeKindScalarTypeExtension,
		NodeKindUnionTypeExtension:
		return true
	}

	return false
}

func (d *Document) NodeNameBytes(node Node) ByteSlice {
	var ref ByteSliceReference

	switch node.Kind {
	case NodeKindObjectTypeDefinition:
		ref = d.ObjectTypeDefinitions[node.Ref].Name
	case NodeKindInterfaceTypeDefinition:
		ref = d.InterfaceTypeDefinitions[node.Ref].Name
	case NodeKindInputObjectTypeDefinition:
		ref = d.InputObjectTypeDefinitions[node.Ref].Name
	case NodeKindUnionTypeDefinition:
		ref = d.UnionTypeDefinitions[node.Ref].Name
	case NodeKindScalarTypeDefinition:
		ref = d.ScalarTypeDefinitions[node.Ref].Name
	case NodeKindDirectiveDefinition:
		ref = d.DirectiveDefinitions[node.Ref].Name
	case NodeKindEnumTypeDefinition:
		ref = d.EnumTypeDefinitions[node.Ref].Name
	case NodeKindField:
		ref = d.Fields[node.Ref].Name
	case NodeKindDirective:
		ref = d.Directives[node.Ref].Name
	case NodeKindObjectTypeExtension:
		ref = d.ObjectTypeExtensions[node.Ref].Name
	case NodeKindInterfaceTypeExtension:
		ref = d.InterfaceTypeExtensions[node.Ref].Name
	case NodeKindUnionTypeExtension:
		ref = d.UnionTypeExtensions[node.Ref].Name
	case NodeKindEnumTypeExtension:
		ref = d.EnumTypeExtensions[node.Ref].Name
	}

	return d.Input.ByteSlice(ref)
}

func (n Node) NameBytes(definition *Document) []byte {
	return definition.NodeNameBytes(n)
}

func (n Node) NameString(definition *Document) string {
	return unsafebytes.BytesToString(definition.NodeNameBytes(n))
}

func (d *Document) UpdateRootNode(ref int, newNodeRef int, newNodeKind NodeKind) {
	d.RootNodes[ref].Kind = newNodeKind
	d.RootNodes[ref].Ref = newNodeRef
}

// TODO: we could use node name directly
func (d *Document) NodeNameUnsafeString(node Node) string {
	return unsafebytes.BytesToString(d.NodeNameBytes(node))
}

func (d *Document) NodeNameString(node Node) string {
	return string(d.NodeNameBytes(node))
}

// Node directives

// NodeHasDirectiveByNameString returns whether the given node has a directive with the given name as string.
func (d *Document) NodeHasDirectiveByNameString(node Node, directiveName string) bool {
	for _, directiveRef := range d.NodeDirectives(node) {
		if d.DirectiveNameString(directiveRef) == directiveName {
			return true
		}
	}
	return false
}

func (d *Document) NodeDirectives(node Node) []int {
	switch node.Kind {
	case NodeKindField:
		return d.Fields[node.Ref].Directives.Refs
	case NodeKindInlineFragment:
		return d.InlineFragments[node.Ref].Directives.Refs
	case NodeKindFragmentSpread:
		return d.FragmentSpreads[node.Ref].Directives.Refs
	case NodeKindSchemaDefinition:
		return d.SchemaDefinitions[node.Ref].Directives.Refs
	case NodeKindSchemaExtension:
		return d.SchemaExtensions[node.Ref].Directives.Refs
	case NodeKindObjectTypeDefinition:
		return d.ObjectTypeDefinitions[node.Ref].Directives.Refs
	case NodeKindObjectTypeExtension:
		return d.ObjectTypeExtensions[node.Ref].Directives.Refs
	case NodeKindFieldDefinition:
		return d.FieldDefinitions[node.Ref].Directives.Refs
	case NodeKindInterfaceTypeDefinition:
		return d.InterfaceTypeDefinitions[node.Ref].Directives.Refs
	case NodeKindInterfaceTypeExtension:
		return d.InterfaceTypeExtensions[node.Ref].Directives.Refs
	case NodeKindInputObjectTypeDefinition:
		return d.InputObjectTypeDefinitions[node.Ref].Directives.Refs
	case NodeKindInputObjectTypeExtension:
		return d.InputObjectTypeExtensions[node.Ref].Directives.Refs
	case NodeKindScalarTypeDefinition:
		return d.ScalarTypeDefinitions[node.Ref].Directives.Refs
	case NodeKindScalarTypeExtension:
		return d.ScalarTypeExtensions[node.Ref].Directives.Refs
	case NodeKindUnionTypeDefinition:
		return d.UnionTypeDefinitions[node.Ref].Directives.Refs
	case NodeKindUnionTypeExtension:
		return d.UnionTypeExtensions[node.Ref].Directives.Refs
	case NodeKindEnumTypeDefinition:
		return d.EnumTypeDefinitions[node.Ref].Directives.Refs
	case NodeKindEnumTypeExtension:
		return d.EnumTypeExtensions[node.Ref].Directives.Refs
	case NodeKindFragmentDefinition:
		return d.FragmentDefinitions[node.Ref].Directives.Refs
	case NodeKindInputValueDefinition:
		return d.InputValueDefinitions[node.Ref].Directives.Refs
	case NodeKindEnumValueDefinition:
		return d.EnumValueDefinitions[node.Ref].Directives.Refs
	case NodeKindVariableDefinition:
		return d.VariableDefinitions[node.Ref].Directives.Refs
	case NodeKindOperationDefinition:
		return d.OperationDefinitions[node.Ref].Directives.Refs
	default:
		return nil
	}
}

func (d *Document) RemoveDirectivesFromNode(node Node, directiveRefs []int) {
	for _, ref := range directiveRefs {
		d.RemoveDirectiveFromNode(node, ref)
	}
}

func (d *Document) RemoveDirectiveFromNode(node Node, directiveRef int) {
	switch node.Kind {
	case NodeKindFragmentSpread:
		if i, ok := indexOf(d.FragmentSpreads[node.Ref].Directives.Refs, directiveRef); ok {
			deleteRef(&d.FragmentSpreads[node.Ref].Directives.Refs, i)
			d.FragmentSpreads[node.Ref].HasDirectives = len(d.FragmentSpreads[node.Ref].Directives.Refs) > 0
		}
	case NodeKindInlineFragment:
		if i, ok := indexOf(d.InlineFragments[node.Ref].Directives.Refs, directiveRef); ok {
			deleteRef(&d.InlineFragments[node.Ref].Directives.Refs, i)
			d.InlineFragments[node.Ref].HasDirectives = len(d.InlineFragments[node.Ref].Directives.Refs) > 0
		}
	case NodeKindField:
		if i, ok := indexOf(d.Fields[node.Ref].Directives.Refs, directiveRef); ok {
			deleteRef(&d.Fields[node.Ref].Directives.Refs, i)
			d.Fields[node.Ref].HasDirectives = len(d.Fields[node.Ref].Directives.Refs) > 0
		}
	case NodeKindFieldDefinition:
		if i, ok := indexOf(d.FieldDefinitions[node.Ref].Directives.Refs, directiveRef); ok {
			deleteRef(&d.FieldDefinitions[node.Ref].Directives.Refs, i)
			d.FieldDefinitions[node.Ref].HasDirectives = len(d.FieldDefinitions[node.Ref].Directives.Refs) > 0
		}
	case NodeKindInterfaceTypeDefinition:
		if i, ok := indexOf(d.InterfaceTypeDefinitions[node.Ref].Directives.Refs, directiveRef); ok {
			deleteRef(&d.InterfaceTypeDefinitions[node.Ref].Directives.Refs, i)
			d.InterfaceTypeDefinitions[node.Ref].HasDirectives = len(d.InterfaceTypeDefinitions[node.Ref].Directives.Refs) > 0
		}
	case NodeKindObjectTypeDefinition:
		if i, ok := indexOf(d.ObjectTypeDefinitions[node.Ref].Directives.Refs, directiveRef); ok {
			deleteRef(&d.ObjectTypeDefinitions[node.Ref].Directives.Refs, i)
			d.ObjectTypeDefinitions[node.Ref].HasDirectives = len(d.ObjectTypeDefinitions[node.Ref].Directives.Refs) > 0
		}
	default:
		log.Printf("RemoveDirectiveFromNode not implemented for node kind: %s", node.Kind)
	}
}

func (d *Document) NodeDirectiveLocation(node Node) (location DirectiveLocation, err error) {
	switch node.Kind {
	case NodeKindSchemaDefinition:
		location = TypeSystemDirectiveLocationSchema
	case NodeKindSchemaExtension:
		location = TypeSystemDirectiveLocationSchema
	case NodeKindObjectTypeDefinition:
		location = TypeSystemDirectiveLocationObject
	case NodeKindObjectTypeExtension:
		location = TypeSystemDirectiveLocationObject
	case NodeKindInterfaceTypeDefinition:
		location = TypeSystemDirectiveLocationInterface
	case NodeKindInterfaceTypeExtension:
		location = TypeSystemDirectiveLocationInterface
	case NodeKindUnionTypeDefinition:
		location = TypeSystemDirectiveLocationUnion
	case NodeKindUnionTypeExtension:
		location = TypeSystemDirectiveLocationUnion
	case NodeKindEnumTypeDefinition:
		location = TypeSystemDirectiveLocationEnum
	case NodeKindEnumTypeExtension:
		location = TypeSystemDirectiveLocationEnum
	case NodeKindInputObjectTypeDefinition:
		location = TypeSystemDirectiveLocationInputObject
	case NodeKindInputObjectTypeExtension:
		location = TypeSystemDirectiveLocationInputObject
	case NodeKindScalarTypeDefinition:
		location = TypeSystemDirectiveLocationScalar
	case NodeKindOperationDefinition:
		switch d.OperationDefinitions[node.Ref].OperationType {
		case OperationTypeQuery:
			location = ExecutableDirectiveLocationQuery
		case OperationTypeMutation:
			location = ExecutableDirectiveLocationMutation
		case OperationTypeSubscription:
			location = ExecutableDirectiveLocationSubscription
		}
	case NodeKindField:
		location = ExecutableDirectiveLocationField
	case NodeKindFragmentSpread:
		location = ExecutableDirectiveLocationFragmentSpread
	case NodeKindInlineFragment:
		location = ExecutableDirectiveLocationInlineFragment
	case NodeKindFragmentDefinition:
		location = ExecutableDirectiveLocationFragmentDefinition
	case NodeKindVariableDefinition:
		location = ExecutableDirectiveLocationVariableDefinition
	default:
		err = fmt.Errorf("node kind: %s is not allowed to have directives", node.Kind)
	}
	return
}

// Node resolvers

// NodeResolverTypeNameBytes returns lowercase query/mutation/subscription for Query/Mutation/Subscription
// for other type definitions it returns the default type name
func (d *Document) NodeResolverTypeNameBytes(node Node, path Path) ByteSlice {
	if len(path) == 1 && path[0].Kind == FieldName {
		return path[0].FieldName
	}
	switch node.Kind {
	case NodeKindObjectTypeDefinition:
		return d.ObjectTypeDefinitionNameBytes(node.Ref)
	case NodeKindInterfaceTypeDefinition:
		return d.InterfaceTypeDefinitionNameBytes(node.Ref)
	case NodeKindUnionTypeDefinition:
		return d.UnionTypeDefinitionNameBytes(node.Ref)
	}
	return nil
}

func (d *Document) NodeResolverTypeNameString(node Node, path Path) string {
	return unsafebytes.BytesToString(d.NodeResolverTypeNameBytes(node, path))
}

// Node field definitions

func (d *Document) NodeFieldDefinitions(node Node) []int {
	switch node.Kind {
	case NodeKindObjectTypeDefinition:
		return d.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs
	case NodeKindObjectTypeExtension:
		return d.ObjectTypeExtensions[node.Ref].FieldsDefinition.Refs
	case NodeKindInterfaceTypeDefinition:
		return d.InterfaceTypeDefinitions[node.Ref].FieldsDefinition.Refs
	case NodeKindInterfaceTypeExtension:
		return d.InterfaceTypeExtensions[node.Ref].FieldsDefinition.Refs
	case NodeKindUnionTypeDefinition:
		return d.UnionTypeDefinitions[node.Ref].FieldsDefinition.Refs
	default:
		return nil
	}
}

func (d *Document) NodeInputFieldDefinitions(node Node) []int {
	switch node.Kind {
	case NodeKindInputObjectTypeDefinition:
		return d.InputObjectTypeDefinitions[node.Ref].InputFieldsDefinition.Refs
	default:
		return nil
	}
}

func (d *Document) NodeInputFieldDefinitionByName(node Node, name ByteSlice) (int, bool) {
	switch node.Kind {
	case NodeKindInputObjectTypeDefinition:
		refs := d.InputObjectTypeDefinitions[node.Ref].InputFieldsDefinition.Refs
		for _, ref := range refs {
			if bytes.Equal(d.Input.ByteSlice(d.InputValueDefinitions[ref].Name), name) {
				return ref, true
			}
		}
	}
	return 0, false
}

func (d *Document) NodeFieldDefinitionByName(node Node, fieldName ByteSlice) (definition int, exists bool) {
	for _, i := range d.NodeFieldDefinitions(node) {
		if bytes.Equal(d.Input.ByteSlice(d.FieldDefinitions[i].Name), fieldName) {
			return i, true
		}
	}
	return InvalidRef, false
}

func (d *Document) NodeFieldDefinitionArgumentDefinitionByName(node Node, fieldName, argumentName ByteSlice) int {
	fieldDefinition, exists := d.NodeFieldDefinitionByName(node, fieldName)
	if !exists {
		return -1
	}
	argumentDefinitions := d.FieldDefinitionArgumentsDefinitions(fieldDefinition)
	for _, i := range argumentDefinitions {
		if bytes.Equal(argumentName, d.Input.ByteSlice(d.InputValueDefinitions[i].Name)) {
			return i
		}
	}
	return -1
}

func (d *Document) NodeFieldDefinitionArgumentsDefinitions(node Node, fieldName ByteSlice) []int {
	fieldDefinition, exists := d.NodeFieldDefinitionByName(node, fieldName)
	if !exists {
		return nil
	}
	return d.FieldDefinitionArgumentsDefinitions(fieldDefinition)
}

// Node input value definitions

// NodeInputValueDefinitions returns input value definition refs based on the node's kind.
func (d *Document) NodeInputValueDefinitions(node Node) []int {
	switch node.Kind {
	case NodeKindInputObjectTypeDefinition:
		return d.InputObjectTypeDefinitions[node.Ref].InputFieldsDefinition.Refs
	case NodeKindInputObjectTypeExtension:
		return d.InputObjectTypeExtensions[node.Ref].InputFieldsDefinition.Refs
	case NodeKindFieldDefinition:
		return d.FieldDefinitions[node.Ref].ArgumentsDefinition.Refs
	case NodeKindDirectiveDefinition:
		return d.DirectiveDefinitions[node.Ref].ArgumentsDefinition.Refs
	default:
		return nil
	}
}

func (d *Document) InputValueDefinitionIsFirst(inputValue int, ancestor Node) bool {
	inputValues := d.NodeInputValueDefinitions(ancestor)
	return inputValues != nil && inputValues[0] == inputValue
}

func (d *Document) InputValueDefinitionIsLast(inputValue int, ancestor Node) bool {
	inputValues := d.NodeInputValueDefinitions(ancestor)
	return inputValues != nil && inputValues[len(inputValues)-1] == inputValue
}

// NodeImplementsInterfaceFields checks that the node has all fields of the interfaceNode.
// This method should not be called from new code.
func (d *Document) NodeImplementsInterfaceFields(node Node, interfaceNode Node) bool {
	nodeFields := d.NodeFieldDefinitions(node)
	interfaceFields := d.NodeFieldDefinitions(interfaceNode)

	for _, i := range interfaceFields {
		interfaceFieldName := d.FieldDefinitionNameBytes(i)
		if !d.FieldDefinitionsContainField(nodeFields, interfaceFieldName) {
			return false
		}
	}

	return true
}

// InterfacesIntersect checks if two interfaces share at least one common implementing type.
func (d *Document) InterfacesIntersect(interfaceA, interfaceB int) bool {
	typeNamesImplementingInterfaceA, _ := d.InterfaceTypeDefinitionImplementedByObjectWithNames(interfaceA)
	typeNamesImplementingInterfaceB, _ := d.InterfaceTypeDefinitionImplementedByObjectWithNames(interfaceB)

	for _, typeNameB := range typeNamesImplementingInterfaceB {
		if slices.Contains(typeNamesImplementingInterfaceA, typeNameB) {
			return true
		}
	}

	return false
}

// NodeImplementsInterfaceNode checks that the node claims to implement an interfaceNode
// in the `implements` section. Node can be an object or interface kind.
//
// This check does not verify that the node has all the fields of the interfaceName.
func (d *Document) NodeImplementsInterfaceNode(node Node, interfaceNode Node) bool {
	interfaceName := d.InterfaceTypeDefinitionNameBytes(interfaceNode.Ref)
	return d.NodeImplementsInterface(node, interfaceName)
}

// NodeImplementsInterface performs the same check as NodeImplementsInterfaceNode, but
// uses the name of an interface.
func (d *Document) NodeImplementsInterface(node Node, interfaceName ByteSlice) bool {
	switch node.Kind {
	case NodeKindObjectTypeDefinition:
		return d.ObjectTypeDefinitionImplementsInterface(node.Ref, interfaceName)
	case NodeKindInterfaceTypeDefinition:
		return d.InterfaceTypeDefinitionImplementsInterface(node.Ref, interfaceName)
	default:
		return false
	}
}

// NodeIsUnionMember checks if the node is a member of the specified union.
func (d *Document) NodeIsUnionMember(node Node, union Node) bool {
	nodeTypeName := d.NodeNameBytes(node)
	for _, i := range d.UnionTypeDefinitions[union.Ref].UnionMemberTypes.Refs {
		memberName := d.ResolveTypeNameBytes(i)
		if bytes.Equal(nodeTypeName, memberName) {
			return true
		}
	}
	return false
}

func (d *Document) NodeIsLastRootNode(node Node) bool {
	if len(d.RootNodes) == 0 {
		return false
	}
	for i := len(d.RootNodes) - 1; i >= 0; i-- {
		if d.RootNodes[i].Kind == NodeKindUnknown {
			continue
		}
		return d.RootNodes[i] == node
	}
	return false
}

func (d *Document) RemoveNodeFromSelectionSetNode(remove, from Node) (removed bool) {
	if from.Kind != NodeKindSelectionSet {
		return false
	}

	return d.RemoveNodeFromSelectionSet(from.Ref, remove)
}

func (d *Document) RemoveNodeFromSelectionSet(set int, node Node) (removed bool) {
	var selectionKind SelectionKind

	switch node.Kind {
	case NodeKindFragmentSpread:
		selectionKind = SelectionKindFragmentSpread
	case NodeKindInlineFragment:
		selectionKind = SelectionKindInlineFragment
	case NodeKindField:
		selectionKind = SelectionKindField
	default:
		return false
	}

	for i, j := range d.SelectionSets[set].SelectionRefs {
		if d.Selections[j].Kind == selectionKind && d.Selections[j].Ref == node.Ref {
			d.SelectionSets[set].SelectionRefs = append(d.SelectionSets[set].SelectionRefs[:i], d.SelectionSets[set].SelectionRefs[i+1:]...)
			return true
		}
	}

	return false
}

// NodeInterfaceRefs returns the interfaces implemented by the given node (this is
// only applicable to object kinds).
// Returns nil if the node is not an object kind.
func (d *Document) NodeInterfaceRefs(node Node) (refs []int) {
	switch node.Kind {
	case NodeKindObjectTypeDefinition:
		return d.ObjectTypeDefinitions[node.Ref].ImplementsInterfaces.Refs
	case NodeKindObjectTypeExtension:
		return d.ObjectTypeExtensions[node.Ref].ImplementsInterfaces.Refs
	default:
		return nil
	}
}

// NodeUnionMemberRefs returns the union members of the given node (this is only
// applicable to union kinds).
// Returns nil if the node is not a union kind.
func (d *Document) NodeUnionMemberRefs(node Node) (refs []int) {
	switch node.Kind {
	case NodeKindUnionTypeDefinition:
		return d.UnionTypeDefinitions[node.Ref].UnionMemberTypes.Refs
	case NodeKindUnionTypeExtension:
		return d.UnionTypeExtensions[node.Ref].UnionMemberTypes.Refs
	default:
		return nil
	}
}

// Node fragments

// NodeFragmentIsAllowedOnNode determines if a fragment node is valid on a parent node.
func (d *Document) NodeFragmentIsAllowedOnNode(fragment, parent Node) bool {
	switch parent.Kind {
	case NodeKindObjectTypeDefinition:
		return d.NodeFragmentIsAllowedOnObjectTypeDefinition(fragment, parent)
	case NodeKindInterfaceTypeDefinition:
		return d.NodeFragmentIsAllowedOnInterfaceTypeDefinition(fragment, parent)
	case NodeKindUnionTypeDefinition:
		return d.NodeFragmentIsAllowedOnUnionTypeDefinition(fragment, parent)
	default:
		return false
	}
}

func (d *Document) NodeFragmentIsAllowedOnInterfaceTypeDefinition(fragment, interfaceType Node) bool {
	switch fragment.Kind {
	case NodeKindObjectTypeDefinition:
		return d.NodeImplementsInterfaceNode(fragment, interfaceType)
	case NodeKindInterfaceTypeDefinition:
		return d.InterfaceNodeIntersectsInterfaceNode(fragment, interfaceType)
	case NodeKindUnionTypeDefinition:
		return d.UnionNodeIntersectsInterfaceNode(fragment, interfaceType)
	default:
		return false
	}
}

func (d *Document) NodeFragmentIsAllowedOnUnionTypeDefinition(fragment, unionType Node) bool {
	switch fragment.Kind {
	case NodeKindObjectTypeDefinition:
		return d.NodeIsUnionMember(fragment, unionType)
	case NodeKindInterfaceTypeDefinition:
		return d.UnionNodeIntersectsInterfaceNode(unionType, fragment)
	case NodeKindUnionTypeDefinition:
		return d.UnionNodeIntersectsUnionNode(unionType, fragment)
	default:
		return false
	}
}

func (d *Document) NodeFragmentIsAllowedOnObjectTypeDefinition(fragment, objectType Node) bool {
	switch fragment.Kind {
	case NodeKindObjectTypeDefinition:
		return bytes.Equal(d.ObjectTypeDefinitionNameBytes(fragment.Ref), d.ObjectTypeDefinitionNameBytes(objectType.Ref))
	case NodeKindInterfaceTypeDefinition:
		return d.NodeImplementsInterfaceNode(objectType, fragment)
	case NodeKindUnionTypeDefinition:
		return d.NodeIsUnionMember(objectType, fragment)
	default:
		return false
	}
}

func (d *Document) UnionNodeIntersectsInterfaceNode(union, interfaceType Node) bool {
	for _, i := range d.UnionTypeDefinitions[union.Ref].UnionMemberTypes.Refs {
		memberName := d.ResolveTypeNameBytes(i)
		node, exists := d.Index.FirstNodeByNameBytes(memberName)
		if !exists {
			continue
		}
		if node.Kind != NodeKindObjectTypeDefinition {
			continue
		}
		if d.NodeImplementsInterfaceNode(node, interfaceType) {
			return true
		}
	}
	return false
}

func (d *Document) UnionNodeIntersectsUnionNode(parentUnion, nestedUnion Node) bool {
	for _, i := range d.UnionTypeDefinitions[parentUnion.Ref].UnionMemberTypes.Refs {
		memberName := d.ResolveTypeNameBytes(i)
		node, exists := d.Index.FirstNodeByNameBytes(memberName)
		if !exists {
			continue
		}
		if node.Kind != NodeKindObjectTypeDefinition {
			continue
		}
		if d.UnionHasMember(nestedUnion.Ref, memberName) {
			return true
		}
	}
	return false
}

// InterfaceNodeIntersectsInterfaceNode checks if two interface nodes share common implementers.
func (d *Document) InterfaceNodeIntersectsInterfaceNode(a, b Node) bool {
	nameA := d.InterfaceTypeDefinitionNameBytes(a.Ref)
	nameB := d.InterfaceTypeDefinitionNameBytes(b.Ref)
	if bytes.Equal(nameA, nameB) {
		return true
	}

	typeNamesImplementingInterfaceA, _ := d.InterfaceTypeDefinitionImplementedByObjectWithNames(a.Ref)
	typeNamesImplementingInterfaceB, _ := d.InterfaceTypeDefinitionImplementedByObjectWithNames(b.Ref)

	hasIntersection := false
	for _, typeNameB := range typeNamesImplementingInterfaceB {
		if slices.Contains(typeNamesImplementingInterfaceA, typeNameB) {
			hasIntersection = true
			break
		}
	}

	return hasIntersection
}
