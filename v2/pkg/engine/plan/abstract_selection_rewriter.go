package plan

import (
	"errors"

	"golang.org/x/exp/slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
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
and not all types of fragments exists in the current datasource or part of a union in this datasource

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
	dsConfiguration    *DataSourceConfiguration

	skipTypeNameFieldRef int
}

func newFieldSelectionRewriter(operation *ast.Document, definition *ast.Document) *fieldSelectionRewriter {
	return &fieldSelectionRewriter{
		operation:            operation,
		definition:           definition,
		skipTypeNameFieldRef: ast.InvalidRef,
	}
}

func (r *fieldSelectionRewriter) SetUpstreamDefinition(upstreamDefinition *ast.Document) {
	r.upstreamDefinition = upstreamDefinition
}

func (r *fieldSelectionRewriter) SetDatasourceConfiguration(dsConfiguration *DataSourceConfiguration) {
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

	if !r.allInterfaceFragmentTypesExistsOnDatasource(selectionSetInfo.inlineFragmentsOnInterfaces) {
		return true
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

	unionTypeNamesToProcess := make([]string, 0, len(unionTypeNames))
	for _, typeName := range unionTypeNames {
		hasTypeOnDatasource := r.hasTypeOnDataSource(typeName)

		if !hasTypeOnDatasource {
			// remove/skip fragments with type not exists in the current datasource
			continue
		}

		unionTypeNamesToProcess = append(unionTypeNamesToProcess, typeName)
	}

	existingObjectFragments, missingFragmentTypeNames := r.filterFragmentsByTypeNames(fieldInfo.inlineFragmentsOnObjects, unionTypeNamesToProcess)

	addedFragments := 0

	// handle existing fragments
	for _, existingObjectFragment := range existingObjectFragments {
		// check if it implements interface
		// if yes - add fields from the interface
		// if no - just copy fragment

		fieldsToAdd := make([]fieldSelection, 0, len(existingObjectFragment.selectionSetInfo.fields))

		for _, fragmentSelectionOnInterface := range fieldInfo.inlineFragmentsOnInterfaces {
			if !fragmentSelectionOnInterface.hasTypeImplementingInterface(existingObjectFragment.typeName) {
				continue
			}

			fieldsToAdd = append(fieldsToAdd, fragmentSelectionOnInterface.selectionSetInfo.fields...)
		}

		fragmentSelectionRef, err := r.copyFragmentSelectionWithFieldsAppend(existingObjectFragment, fieldsToAdd)
		if err != nil {
			return err
		}

		newSelectionRefs = append(newSelectionRefs, fragmentSelectionRef)

		addedFragments++
	}

	// handle missing fragments
	for _, missingFragmentTypeName := range missingFragmentTypeNames {
		// check if it implements each interface
		// and add field from each interface fragment selection

		fieldsToAdd := make([]fieldSelection, 0, 2)

		for _, fragmentSelectionOnInterface := range fieldInfo.inlineFragmentsOnInterfaces {
			if !fragmentSelectionOnInterface.hasTypeImplementingInterface(missingFragmentTypeName) {
				continue
			}

			fieldsToAdd = append(fieldsToAdd, fragmentSelectionOnInterface.selectionSetInfo.fields...)
		}

		if len(fieldsToAdd) == 0 {
			continue
		}

		fragmentSelectionRef := r.createFragmentSelection(missingFragmentTypeName, fieldsToAdd)
		newSelectionRefs = append(newSelectionRefs, fragmentSelectionRef)
		addedFragments++
	}

	fieldSelectionSetRef, _ := r.operation.FieldSelectionSet(fieldRef)
	r.operation.EmptySelectionSet(fieldSelectionSetRef)

	for _, newSelectionRef := range newSelectionRefs {
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, newSelectionRef)
	}

	if addedFragments == 0 && !fieldInfo.hasTypeNameSelection {
		// we have to add __typename selection - but we should skip it in response
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection()
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, typeNameSelectionRef)
		r.skipTypeNameFieldRef = typeNameFieldRef
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
	if node.Kind != ast.NodeKindInterfaceTypeDefinition {
		return false, errors.New("unexpected error: node kind is not interface type definition in the upstream schema")
	}

	typeNames, ok := r.upstreamDefinition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
	if !ok {
		return false, nil
	}

	entityNames, _ := r.datasourceHasEntitiesWithName(typeNames)

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return false, err
	}

	entitiesWithoutFragment, needRewrite := r.interfaceFieldSelectionNeedsRewrite(selectionSetInfo, entityNames, typeNames)
	if !needRewrite {
		return false, nil
	}

	err = r.rewriteInterfaceSelection(fieldRef, selectionSetInfo, entitiesWithoutFragment, typeNames)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *fieldSelectionRewriter) interfaceFieldSelectionNeedsRewrite(selectionSetInfo selectionSetInfo, entityNames []string, interfaceTypeNames []string) (entitiesWithoutFragment []string, needRewrite bool) {
	entitiesWithoutFragment = r.entityNamesWithoutFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)

	// TODO: we are not checking inline fragments on interfaces - this is the case when we have on interface fragment within interface field selection

	/*

		case we do not have fragment on objects,
		but we do have fragment on interface and inside we have fragments on objects

		what we need to check?
	*/

	// case 1. we do not have fragments
	if len(selectionSetInfo.inlineFragmentsOnObjects) == 0 {
		// check that all types implementing the interface have a root node with the requested fields
		return entitiesWithoutFragment, !r.allEntitiesHaveFieldsAsRootNode(entityNames, selectionSetInfo.fields)
	}

	// check that all inline fragments types are implementing the interface in the current datasource
	if !r.allFragmentTypesImplementsInterfaceTypes(selectionSetInfo.inlineFragmentsOnObjects, interfaceTypeNames) {
		return entitiesWithoutFragment, true
	}

	// check that all inline fragments types are present in the current datasource
	if !r.allFragmentTypesExistsOnDatasource(selectionSetInfo.inlineFragmentsOnObjects) {
		return entitiesWithoutFragment, true
	}

	// case 2. we do not have shared fields, but only fragments
	if len(selectionSetInfo.fields) == 0 {
		// if we do not have shared fields but do have fragments - we do not need to rewrite
		return entitiesWithoutFragment, false
	}

	// case 3. we have both shared fields and inline fragments
	// 3.1 check first case for types for which we do not have inline fragments

	if !r.allEntitiesHaveFieldsAsRootNode(entitiesWithoutFragment, selectionSetInfo.fields) {
		return entitiesWithoutFragment, true
	}

	// 3.2 check that fragment types have all requested fields or all not selected fields are local for the datasource
	for _, inlineFragmentSelection := range selectionSetInfo.inlineFragmentsOnObjects {
		if !r.inlineFragmentHasAllFieldsLocalToDatasource(inlineFragmentSelection, selectionSetInfo.fields) {
			return entitiesWithoutFragment, true
		}
	}

	return entitiesWithoutFragment, false
}

func (r *fieldSelectionRewriter) rewriteInterfaceSelection(fieldRef int, fieldInfo selectionSetInfo, entitiesWithoutFragment []string, interfaceTypeNames []string) error {
	newSelectionRefs := make([]int, 0, len(entitiesWithoutFragment)+len(fieldInfo.inlineFragmentsOnObjects)+1) // 1 for __typename

	fieldSelectionSetRef, _ := r.operation.FieldSelectionSet(fieldRef)
	r.operation.EmptySelectionSet(fieldSelectionSetRef)

	for _, newSelectionRef := range newSelectionRefs {
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, newSelectionRef)
	}

	if addedFragments == 0 && !fieldInfo.hasTypeNameSelection {
		// we have to add __typename selection - but we should skip it in response
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection()
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, typeNameSelectionRef)
		r.skipTypeNameFieldRef = typeNameFieldRef
	}
}

func (r *fieldSelectionRewriter) flattenFragmentOnInterface(fieldRef int, fieldInfo selectionSetInfo, entitiesWithoutFragment []string, interfaceTypeNames []string) error {
	newSelectionRefs := make([]int, 0, len(entitiesWithoutFragment)+len(fieldInfo.inlineFragmentsOnObjects)+1) // 1 for __typename

	if fieldInfo.hasTypeNameSelection {
		// we should preserve __typename if it was in the original query as it is explicitly requested
		typeNameSelectionRef, _ := r.typeNameSelection()
		newSelectionRefs = append(newSelectionRefs, typeNameSelectionRef)
	}

	addedFragments := 0

	if len(fieldInfo.fields) > 0 {
		for _, entityName := range entitiesWithoutFragment {
			newSelectionRefs = append(newSelectionRefs, r.createFragmentSelection(entityName, fieldInfo.fields))
			addedFragments++
		}
	}

	for _, inlineFragmentInfo := range fieldInfo.inlineFragmentsOnObjects {
		if !r.hasTypeOnDataSource(inlineFragmentInfo.typeName) {
			// remove fragments with type not exists in the current datasource
			continue
		}

		if !slices.Contains(interfaceTypeNames, inlineFragmentInfo.typeName) {
			// remove fragment which not implements interface in the current datasource
			continue
		}

		fragmentSelectionRef, err := r.copyFragmentSelectionWithFieldsAppend(inlineFragmentInfo, fieldInfo.fields)
		if err != nil {
			return err
		}

		newSelectionRefs = append(newSelectionRefs, fragmentSelectionRef)
		addedFragments++
	}

	return nil
}

/*
func (r *fieldSelectionRewriter) flattenFragmentOnInterface(fragmentSelection inlineFragmentSelection) {


		recursively traverse fragment
		for each nested fragment check does it contain other fragment

		when merging level up check that all type fragments are matching current implements interface types - remove not matching types

		fragments with directives what to do with them?
		we could not merge them


		after flattening we could merge this fragments with other fragments

		if there are inline fragments and shared fields:
		- we probably will merge them immediately

		if there are only shared fields, we could create new inline fragment with shared fields

		and merge them after words in case there is any other existing fragments

		could there be disruption between fragments for example on union and nested within interface fragment?
		probably yes, types from interface could be not present in the union
		and we could discard not matching types


		in case interface fragment contains nested fragments - we always rewrite
		- this should be also checked in needRewrite method



	if fragmentSelection.hasDirectives {
		// we have to propagate directives to nested fragments
	}

	if fragmentSelection.selectionSetInfo.hasInlineFragmentsOnInterfaces {
		// we need to recursively flatten nested fragments
	}

	if fragmentSelection.selectionSetInfo.hasInlineFragmentsOnObjects {
		// we need to check if it contains fragments on interface types
	}

	if fragmentSelection.selectionSetInfo.hasFields {

	}

}
*/
