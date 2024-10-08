package plan

import (
	"errors"
	"slices"
	"sort"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
)

var (
	FieldDoesntHaveSelectionSetErr          = errors.New("unexpected error: field does not have a selection set")
	InlineFragmentDoesntHaveSelectionSetErr = errors.New("unexpected error: inline fragment does not have a selection set")
	InlineFragmentTypeIsNotExistsErr        = errors.New("unexpected error: inline fragment type condition does not exists")
)

/*
fieldSelectionRewriter - rewrites abstract types selection in the following cases:

1. We have selections on the field with the Interface return type
and some types do not have requested fields local to the current datasource

	interfaceField {
		some
		someOther // field is external to some types implementing interface Some
	}

2. We have inline fragment selection on the field with the Interface return type
and not all of these fragments are valid for the current datasource - e.g. in this subgraph this type is not implementing this interface

	interfaceField {
		... on Interface {
		}
	}

3. We have inline fragment selections on the field with the Union return type
and not all types of fragments exists in the current datasource or not a part of the union in this datasource

	unionField {
		... on A { // A - is exists in this datasource and part of a union
		}
		... on B { // B - is exists in this datasource but not part of a union
		}
		... on C { // C - do not exist in this datasource
		}
	}

4. We have inline fragment selection on the field with the Union return type
In this case if any of rules 1-3 are not satisfied we have to rewrite this fragment into concrete types

	unionField {
		... on Interface {
		}
	}
*/
type fieldSelectionRewriter struct {
	operation  *ast.Document
	definition *ast.Document

	upstreamDefinition *ast.Document
	dsConfiguration    DataSource
}

func newFieldSelectionRewriter(operation *ast.Document, definition *ast.Document) *fieldSelectionRewriter {
	return &fieldSelectionRewriter{
		operation:  operation,
		definition: definition,
	}
}

func (r *fieldSelectionRewriter) SetUpstreamDefinition(upstreamDefinition *ast.Document) {
	r.upstreamDefinition = upstreamDefinition
}

func (r *fieldSelectionRewriter) SetDatasourceConfiguration(dsConfiguration DataSource) {
	r.dsConfiguration = dsConfiguration
}

func (r *fieldSelectionRewriter) RewriteFieldSelection(fieldRef int, enclosingNode ast.Node) (rewritten bool, err error) {
	fieldTypeNode, ok := r.definition.FieldTypeNode(r.operation.FieldNameBytes(fieldRef), enclosingNode)
	if !ok {
		return false, nil
	}

	switch fieldTypeNode.Kind {
	case ast.NodeKindInterfaceTypeDefinition:
		return r.processInterfaceSelection(fieldRef, fieldTypeNode.Ref)
	case ast.NodeKindUnionTypeDefinition:
		return r.processUnionSelection(fieldRef, fieldTypeNode.Ref)
	default:
		return false, nil
	}
}

func (r *fieldSelectionRewriter) processUnionSelection(fieldRef int, unionDefRef int) (rewritten bool, err error) {
	/*
		1) extract inline fragments selections with interface types
		2) extract inline fragments selections with members of the union
		3) intersect inline fragments selections with interface types and members of the union
		4) create new inline fragments with types from the intersection which do not have inline fragments
		5) append existing inline fragments with fields from the interfaces
	*/

	unionTypeName := r.definition.UnionTypeDefinitionNameBytes(unionDefRef)
	node, hasNode := r.upstreamDefinition.NodeByName(unionTypeName)
	if !hasNode {
		return false, errors.New("unexpected error: union type definition not found in the upstream schema")
	}
	if node.Kind != ast.NodeKindUnionTypeDefinition {
		return false, errors.New("unexpected error: node kind is not union type definition in the upstream schema")
	}

	unionTypeNames, ok := r.upstreamDefinition.UnionTypeDefinitionMemberTypeNames(node.Ref)
	if !ok {
		return false, nil
	}

	sort.Strings(unionTypeNames)

	entityNames, _ := r.datasourceHasEntitiesWithName(unionTypeNames)

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return false, err
	}

	needRewrite := r.unionFieldSelectionNeedsRewrite(selectionSetInfo, unionTypeNames, entityNames)
	if !needRewrite {
		return false, nil
	}

	err = r.rewriteUnionSelection(fieldRef, selectionSetInfo, unionTypeNames, entityNames)
	if err != nil {
		return false, err
	}

	return needRewrite, nil
}

func (r *fieldSelectionRewriter) unionFieldSelectionNeedsRewrite(selectionSetInfo selectionSetInfo, unionTypeNames, entityNames []string) (needRewrite bool) {
	// when we have types not exists in the current datasource - we need to rewrite
	if !r.allFragmentTypesExistsOnDatasource(selectionSetInfo.inlineFragmentsOnObjects) {
		return true
	}

	// when we do not have fragments on interfaces, but only on objects - we do not need to rewrite
	if !selectionSetInfo.hasInlineFragmentsOnInterfaces {
		return false
	}

	if r.interfaceFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnInterfaces, unionTypeNames) {
		return true
	}

	// when we do not have fragments on objects, but only on interfaces
	// we need to check that all entities implementing each interface have a root node with the requested fields
	// e.g. { ... on Interface { a } }

	if !selectionSetInfo.hasInlineFragmentsOnObjects {
		return !r.allEntitiesImplementsInterfaces(selectionSetInfo.inlineFragmentsOnInterfaces, entityNames)
	}

	// when we have fragments on both interfaces and objects
	// we need to check that all entities without fragments implementing each interface have a root node with the requested fields

	entitiesWithoutFragment := r.entityNamesWithoutFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)
	if len(entitiesWithoutFragment) > 0 {
		if !r.allEntitiesImplementsInterfaces(selectionSetInfo.inlineFragmentsOnInterfaces, entitiesWithoutFragment) {
			return true
		}
	}

	// for each existing fragment we need to check:
	// - is it entity
	// - is it implements each interface
	// - does it have all requested fields from this interface
	entityNamesWithFragments := r.entityNamesWithFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)
	if len(entityNamesWithFragments) > 0 {
		if !r.allEntityFragmentsSatisfyInterfaces(selectionSetInfo.inlineFragmentsOnInterfaces, selectionSetInfo.inlineFragmentsOnObjects, entityNamesWithFragments) {
			return true
		}
	}

	return false
}

func (r *fieldSelectionRewriter) rewriteUnionSelection(fieldRef int, fieldInfo selectionSetInfo, unionTypeNames, entityNames []string) error {
	newSelectionRefs := make([]int, 0, len(unionTypeNames)+1) // 1 for __typename
	if fieldInfo.hasTypeNameSelection {
		// we should preserve __typename if it was in the original query as it is explicitly requested
		typeNameSelectionRef, _ := r.typeNameSelection()
		newSelectionRefs = append(newSelectionRefs, typeNameSelectionRef)
	}

	for _, inlineFragmentOnInterface := range fieldInfo.inlineFragmentsOnInterfaces {
		// we need to recursively flatten nested fragments on interfaces
		r.flattenFragmentOnInterface(inlineFragmentOnInterface.selectionSetInfo, inlineFragmentOnInterface.typeNamesImplementingInterface, unionTypeNames, &newSelectionRefs)
	}

	// filter existing fragments by type names exists in the current datasource
	// TODO: do not need to iterate 2 times in filter and here
	filteredObjectFragments, _ := r.filterFragmentsByTypeNames(fieldInfo.inlineFragmentsOnObjects, unionTypeNames)
	// copy existing fragments on objects
	for _, existingObjectFragment := range filteredObjectFragments {
		fragmentSelectionRef := r.operation.CopySelection(existingObjectFragment.selectionRef)
		newSelectionRefs = append(newSelectionRefs, fragmentSelectionRef)
	}

	return r.replaceFieldSelections(fieldRef, newSelectionRefs)
}

func (r *fieldSelectionRewriter) replaceFieldSelections(fieldRef int, newSelectionRefs []int) error {
	fieldSelectionSetRef, _ := r.operation.FieldSelectionSet(fieldRef)
	r.operation.EmptySelectionSet(fieldSelectionSetRef)

	for _, newSelectionRef := range newSelectionRefs {
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, newSelectionRef)
	}

	if len(newSelectionRefs) == 0 {
		// we have to add __typename selection in case there is no other selections
		typeNameSelectionRef, _ := r.typeNameSelection()
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, typeNameSelectionRef)
	}

	normalizer := astnormalization.NewAbstractFieldNormalizer(r.operation, r.definition, fieldRef)
	if err := normalizer.Normalize(); err != nil {
		return err
	}

	return nil
}

func (r *fieldSelectionRewriter) processInterfaceSelection(fieldRef int, interfaceDefRef int) (rewritten bool, err error) {
	/*
		1) extract selections which is not inline-fragments - e.g. shared selections
		2) extract selections for each inline fragment
		3) for types which do not have inline-fragment - add inline fragment with shared fields
		4) for types which have inline-fragment - add not selected shared fields to existing inline fragment
	*/

	interfaceTypeName := r.definition.InterfaceTypeDefinitionNameBytes(interfaceDefRef)
	node, hasNode := r.upstreamDefinition.NodeByName(interfaceTypeName)
	if !hasNode {
		return false, errors.New("unexpected error: interface type definition not found in the upstream schema")
	}

	var isInterfaceObject bool
	var interfaceTypeNames []string

	if node.Kind != ast.NodeKindInterfaceTypeDefinition {
		interfaceTypeNameStr := string(interfaceTypeName)
		for _, k := range r.dsConfiguration.FederationConfiguration().InterfaceObjects {
			if k.InterfaceTypeName == interfaceTypeNameStr {
				isInterfaceObject = true
				interfaceTypeNames = k.ConcreteTypeNames
				break
			}
		}

		if !isInterfaceObject {
			return false, errors.New("unexpected error: node kind is not interface type definition in the upstream schema")
		}
	}

	if !isInterfaceObject {
		var ok bool
		interfaceTypeNames, ok = r.upstreamDefinition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
		if !ok {
			return false, nil
		}
	}

	sort.Strings(interfaceTypeNames)

	entityNames, _ := r.datasourceHasEntitiesWithName(interfaceTypeNames)

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return false, err
	}

	needRewrite := r.interfaceFieldSelectionNeedsRewrite(selectionSetInfo, interfaceTypeNames, entityNames)
	if !needRewrite {
		return false, nil
	}

	err = r.rewriteInterfaceSelection(fieldRef, selectionSetInfo, interfaceTypeNames)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *fieldSelectionRewriter) interfaceFieldSelectionNeedsRewrite(selectionSetInfo selectionSetInfo, interfaceTypeNames []string, entityNames []string) (needRewrite bool) {
	// when we do not have fragments
	if !selectionSetInfo.hasInlineFragmentsOnInterfaces && !selectionSetInfo.hasInlineFragmentsOnObjects {
		// check that all types implementing the interface have a root node with the requested fields
		if !r.allEntitiesHaveFieldsAsRootNode(entityNames, selectionSetInfo.fields) {
			return true
		}

		return slices.ContainsFunc(entityNames, func(entityName string) bool {
			return r.hasRequiresConfigurationForField(entityName, selectionSetInfo.fields)
		})
	}

	if selectionSetInfo.hasInlineFragmentsOnObjects {
		// check that all inline fragments types are present in the current datasource
		if !r.allFragmentTypesExistsOnDatasource(selectionSetInfo.inlineFragmentsOnObjects) {
			return true
		}

		// check that all inline fragments types are implementing the interface in the current datasource
		if !r.allFragmentTypesImplementsInterfaceTypes(selectionSetInfo.inlineFragmentsOnObjects, interfaceTypeNames) {
			return true
		}
	}

	entitiesWithoutFragment := r.entityNamesWithoutFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)

	// check that all entities without fragments have a root node with the requested fields
	if selectionSetInfo.hasFields {
		if !r.allEntitiesHaveFieldsAsRootNode(entitiesWithoutFragment, selectionSetInfo.fields) {
			return true
		}
	}

	if selectionSetInfo.hasFields && selectionSetInfo.hasInlineFragmentsOnObjects {
		// check that fragment types have all requested fields or all not selected fields are local for the datasource
		for _, inlineFragmentSelection := range selectionSetInfo.inlineFragmentsOnObjects {
			if !r.inlineFragmentHasAllFieldsLocalToDatasource(inlineFragmentSelection, selectionSetInfo.fields) {
				return true
			}
		}
	}

	if selectionSetInfo.hasInlineFragmentsOnInterfaces {
		if r.interfaceFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnInterfaces, interfaceTypeNames) {
			return true
		}
	}

	if selectionSetInfo.hasInlineFragmentsOnInterfaces && selectionSetInfo.hasInlineFragmentsOnObjects {
		if len(entitiesWithoutFragment) > 0 {
			if !r.allEntitiesImplementsInterfaces(selectionSetInfo.inlineFragmentsOnInterfaces, entitiesWithoutFragment) {
				return true
			}
		}

		// for each existing fragment we need to check:
		// - is it entity
		// - is it implements each interface
		// - does it have all requested fields from this interface
		entityNamesWithFragments := r.entityNamesWithFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)
		if len(entityNamesWithFragments) > 0 {
			if !r.allEntityFragmentsSatisfyInterfaces(selectionSetInfo.inlineFragmentsOnInterfaces, selectionSetInfo.inlineFragmentsOnObjects, entityNamesWithFragments) {
				return true
			}
		}
	}

	return false
}

func (r *fieldSelectionRewriter) rewriteInterfaceSelection(fieldRef int, fieldInfo selectionSetInfo, interfaceTypeNames []string) error {
	newSelectionRefs := make([]int, 0, len(interfaceTypeNames)+1) // 1 for __typename

	r.flattenFragmentOnInterface(
		fieldInfo,
		interfaceTypeNames,
		interfaceTypeNames,
		&newSelectionRefs,
	)

	return r.replaceFieldSelections(fieldRef, newSelectionRefs)
}

func (r *fieldSelectionRewriter) flattenFragmentOnInterface(selectionSetInfo selectionSetInfo, typeNamesImplementingInterfaceInCurrentDS []string, allowedTypeNames []string, selectionRefs *[]int) {
	if len(typeNamesImplementingInterfaceInCurrentDS) == 0 {
		return
	}

	filteredImplementingTypes := make([]string, 0, len(typeNamesImplementingInterfaceInCurrentDS))
	for _, typeName := range typeNamesImplementingInterfaceInCurrentDS {
		if slices.Contains(allowedTypeNames, typeName) {
			filteredImplementingTypes = append(filteredImplementingTypes, typeName)
		}
	}

	if selectionSetInfo.hasFields {
		for _, typeName := range filteredImplementingTypes {
			*selectionRefs = append(*selectionRefs, r.createFragmentSelection(typeName, selectionSetInfo.fields))
		}
	}

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnObjects {
		if !slices.Contains(filteredImplementingTypes, inlineFragmentInfo.typeName) {
			// remove fragment which not allowed
			continue
		}

		fragmentSelectionRef := r.operation.CopySelection(inlineFragmentInfo.selectionRef)

		*selectionRefs = append(*selectionRefs, fragmentSelectionRef)
	}

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnInterfaces {
		// We do not check if interface fragment type not exists in the current datasource
		// in case of interfaces the only thing which is matter is an interception of implementing types
		// and parent allowed types

		r.flattenFragmentOnInterface(inlineFragmentInfo.selectionSetInfo, inlineFragmentInfo.typeNamesImplementingInterface, filteredImplementingTypes, selectionRefs)
	}
}
