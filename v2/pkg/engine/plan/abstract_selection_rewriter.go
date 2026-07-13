package plan

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

var (
	ErrFieldHasNoSelectionSet          = errors.New("unexpected error: field does not have a selection set")
	ErrInlineFragmentHasNoSelectionSet = errors.New("unexpected error: inline fragment does not have a selection set")
	ErrInlineFragmentHasNoCondition    = errors.New("unexpected error: inline fragment type condition does not exist")

	ErrNoUpstreamSchema = errors.New("unexpected error: upstream schema is not defined in DataSource")
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

	skipFieldRefs []int
	alwaysRewrite bool
}

type RewriteResult struct {
	rewritten        bool
	changedFieldRefs map[int][]int // map[fieldRef][]fieldRef - for each original fieldRef list of new fieldRefs; identity mappings are omitted
	fieldRefOrigins  map[int][]int // map[fieldRef][]fieldRef - for each fieldRef present after the rewrite, all original fieldRefs occupying the same response position, including itself
}

var resultNotRewritten = RewriteResult{}

type rewriterOption func(*rewriterOptions)

type rewriterOptions struct {
	forceRewrite bool
}

func withForceRewrite() rewriterOption {
	return func(o *rewriterOptions) {
		o.forceRewrite = true
	}
}

func newFieldSelectionRewriter(operation *ast.Document, definition *ast.Document, dsConfiguration DataSource, options ...rewriterOption) (*fieldSelectionRewriter, error) {
	upstreamDefinition, ok := dsConfiguration.UpstreamSchema()
	if !ok {
		return nil, ErrNoUpstreamSchema
	}

	opts := &rewriterOptions{}
	for _, option := range options {
		option(opts)
	}

	return &fieldSelectionRewriter{
		operation:          operation,
		definition:         definition,
		upstreamDefinition: upstreamDefinition,
		dsConfiguration:    dsConfiguration,
		alwaysRewrite:      dsConfiguration.PlanningBehavior().AlwaysFlattenFragments || opts.forceRewrite,
	}, nil
}

func (r *fieldSelectionRewriter) RewriteFieldSelection(fieldRef int, enclosingNode ast.Node) (res RewriteResult, err error) {
	fieldName := r.operation.FieldNameBytes(fieldRef)
	fieldTypeNode, ok := r.definition.FieldTypeNode(fieldName, enclosingNode)
	if !ok {
		return resultNotRewritten, nil
	}

	enclosingTypeName := r.definition.NodeNameBytes(enclosingNode)

	switch fieldTypeNode.Kind {
	case ast.NodeKindInterfaceTypeDefinition:
		res, err = r.processInterfaceSelection(fieldRef, fieldTypeNode.Ref, enclosingTypeName)
		if err != nil {
			return resultNotRewritten, fmt.Errorf("failed to rewrite field %s.%s with the interface return type: %w", enclosingTypeName, fieldName, err)
		}
	case ast.NodeKindUnionTypeDefinition:
		res, err = r.processUnionSelection(fieldRef, fieldTypeNode.Ref, enclosingTypeName)
		if err != nil {
			return resultNotRewritten, fmt.Errorf("failed to rewrite field %s.%s with the union return type: %w", enclosingTypeName, fieldName, err)
		}
	case ast.NodeKindObjectTypeDefinition:
		res, err = r.processObjectSelection(fieldRef, fieldTypeNode.Ref)
		if err != nil {
			return resultNotRewritten, fmt.Errorf("failed to rewrite field %s.%s with the object return type: %w", enclosingTypeName, fieldName, err)
		}
	default:
		return resultNotRewritten, nil
	}

	return res, nil
}

func (r *fieldSelectionRewriter) processUnionSelection(fieldRef int, unionDefRef int, enclosingTypeName ast.ByteSlice) (res RewriteResult, err error) {
	/*
		1) extract inline fragments selections with interface types
		2) extract inline fragments selections with members of the union
		3) intersect inline fragments selections with interface types and members of the union
		4) create new inline fragments with types from the intersection which do not have inline fragments
		5) append existing inline fragments with fields from the interfaces
	*/

	unionTypeNames, err := r.getAllowedUnionMemberTypeNames(fieldRef, unionDefRef, enclosingTypeName)
	if err != nil {
		return resultNotRewritten, err
	}

	entityNames, _ := r.datasourceHasEntitiesWithName(unionTypeNames)

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return resultNotRewritten, err
	}

	needRewrite := r.unionFieldSelectionNeedsRewrite(selectionSetInfo, unionTypeNames, entityNames)
	if !needRewrite {
		return resultNotRewritten, nil
	}

	fieldPaths, err := collectFieldPaths(r.operation, r.definition, fieldRef)
	if err != nil {
		return resultNotRewritten, err
	}

	err = r.rewriteUnionSelection(fieldRef, selectionSetInfo, unionTypeNames)
	if err != nil {
		return resultNotRewritten, err
	}

	changedRefs, originRefs, err := r.collectChangedRefs(fieldRef, fieldPaths)
	if err != nil {
		return resultNotRewritten, err
	}

	return RewriteResult{
		rewritten:        true,
		changedFieldRefs: changedRefs,
		fieldRefOrigins:  originRefs,
	}, nil
}

func (r *fieldSelectionRewriter) mustRewrite(s selectionSetInfo) bool {
	return r.alwaysRewrite &&
		(s.hasInlineFragmentsOnInterfaces ||
			s.hasInlineFragmentsOnUnions ||
			s.hasInlineFragmentsOnObjects)
}

func (r *fieldSelectionRewriter) unionFieldSelectionNeedsRewrite(selectionSetInfo selectionSetInfo, unionTypeNames, entityNames []string) (needRewrite bool) {
	if r.mustRewrite(selectionSetInfo) {
		return true
	}
	if selectionSetInfo.hasInlineFragmentsOnObjects {
		// when we have types not exists in the current datasource - we need to rewrite
		if r.objectFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnObjects, unionTypeNames) {
			return true
		}
	}

	// when we do not have fragments on interfaces or union, but only on objects - we do not need to rewrite
	if !selectionSetInfo.hasInlineFragmentsOnInterfaces && !selectionSetInfo.hasInlineFragmentsOnUnions {
		return false
	}

	if selectionSetInfo.hasInlineFragmentsOnInterfaces &&
		r.interfaceFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnInterfaces, unionTypeNames) {
		return true
	}

	if selectionSetInfo.hasInlineFragmentsOnUnions &&
		r.unionFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnUnions, unionTypeNames) {
		return true
	}

	if !selectionSetInfo.hasInlineFragmentsOnInterfaces {
		return false
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

func (r *fieldSelectionRewriter) rewriteUnionSelection(fieldRef int, fieldInfo selectionSetInfo, unionTypeNames []string) error {
	newSelectionRefs := make([]int, 0, len(unionTypeNames)+1) // 1 for __typename

	r.flattenFragmentOnUnion(fieldInfo, unionTypeNames, &newSelectionRefs)

	return r.replaceFieldSelections(fieldRef, newSelectionRefs)
}

func (r *fieldSelectionRewriter) replaceFieldSelections(fieldRef int, newSelectionRefs []int) error {
	fieldSelectionSetRef, _ := r.operation.FieldSelectionSet(fieldRef)
	r.operation.EmptySelectionSet(fieldSelectionSetRef)

	for _, newSelectionRef := range newSelectionRefs {
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, newSelectionRef)
	}

	if len(newSelectionRefs) == 0 {
		deferID, _ := r.operation.FieldInternalDeferID(fieldRef)
		// we have to add __typename selection in case there is no other selections
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection(deferID)
		r.skipFieldRefs = append(r.skipFieldRefs, typeNameFieldRef)
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, typeNameSelectionRef)

		// if there is no other selections we could skip normalization
		return nil
	}

	normalizer := astnormalization.NewAbstractFieldNormalizer(r.operation, r.definition, fieldRef)
	if err := normalizer.Normalize(); err != nil {
		return err
	}

	return nil
}

func (r *fieldSelectionRewriter) processObjectSelection(fieldRef int, objectDefRef int) (res RewriteResult, err error) {
	selectionSetRef, ok := r.operation.FieldSelectionSet(fieldRef)
	if !ok {
		return resultNotRewritten, ErrFieldHasNoSelectionSet
	}

	fieldTypeName := r.definition.ObjectTypeDefinitionNameBytes(objectDefRef)
	fieldTypeNameStr := r.definition.ObjectTypeDefinitionNameString(objectDefRef)

	if !r.dsConfiguration.HasRootNodeWithTypename(fieldTypeNameStr) {
		// if the object type is not an entity in the current datasource
		// we do not need to rewrite it
		return resultNotRewritten, nil
	}

	// Doing a full set of checks with collecting inline fragment information
	// is very expensive, so we are trying to avoid it.
	// We need to rewrite the object fragment only in case it has fragments
	// with type condition which is not matching the object type

	inlineFragmentSelectionRefs := r.operation.SelectionSetInlineFragmentSelections(selectionSetRef)
	if len(inlineFragmentSelectionRefs) == 0 {
		// no inline fragments on the field, so we do not need to rewrite it
		return resultNotRewritten, nil
	}

	hasFragmentsWithNotMatchingType := false
	for _, inlineFragmentSelectionRef := range inlineFragmentSelectionRefs {
		inlineFragmentRef := r.operation.Selections[inlineFragmentSelectionRef].Ref
		typeCondition := r.operation.InlineFragmentTypeConditionName(inlineFragmentRef)

		if !bytes.Equal(typeCondition, fieldTypeName) {
			hasFragmentsWithNotMatchingType = true
			break
		}
	}

	if !hasFragmentsWithNotMatchingType {
		return resultNotRewritten, nil
	}

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return resultNotRewritten, err
	}

	needRewrite := r.objectFieldSelectionNeedsRewrite(selectionSetInfo, fieldTypeNameStr)
	if !needRewrite {
		return resultNotRewritten, nil
	}

	fieldPaths, err := collectFieldPaths(r.operation, r.definition, fieldRef)
	if err != nil {
		return resultNotRewritten, err
	}

	err = r.rewriteObjectSelection(fieldRef, selectionSetInfo, fieldTypeNameStr)
	if err != nil {
		return resultNotRewritten, err
	}

	changedRefs, originRefs, err := r.collectChangedRefs(fieldRef, fieldPaths)
	if err != nil {
		return resultNotRewritten, err
	}

	return RewriteResult{
		rewritten:        true,
		changedFieldRefs: changedRefs,
		fieldRefOrigins:  originRefs,
	}, nil
}

func (r *fieldSelectionRewriter) rewriteObjectSelection(fieldRef int, fieldInfo selectionSetInfo, objectTypeName string) error {
	newSelectionRefs := make([]int, 0, 2)

	if fieldInfo.hasFields {
		newSelectionRefs = append(newSelectionRefs, r.createFragmentSelection(objectTypeName, fieldInfo.fields))
	}

	// handling of the object type is similar to the union type, so we reuse the same logic
	r.flattenFragmentOnUnion(fieldInfo, []string{objectTypeName}, &newSelectionRefs)

	return r.replaceFieldSelections(fieldRef, newSelectionRefs)
}

func (r *fieldSelectionRewriter) objectFieldSelectionNeedsRewrite(selectionSetInfo selectionSetInfo, objectTypeName string) (needRewrite bool) {
	if r.mustRewrite(selectionSetInfo) {
		return true
	}
	if selectionSetInfo.hasInlineFragmentsOnObjects {
		if r.objectFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnObjects, []string{objectTypeName}) {
			return true
		}
	}

	if selectionSetInfo.hasInlineFragmentsOnInterfaces &&
		r.interfaceFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnInterfaces, []string{objectTypeName}) {
		return true
	}

	if selectionSetInfo.hasInlineFragmentsOnUnions &&
		r.unionFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnUnions, []string{objectTypeName}) {
		return true
	}

	return false
}

func (r *fieldSelectionRewriter) processInterfaceSelection(fieldRef int, interfaceDefRef int, enclosingTypeName ast.ByteSlice) (res RewriteResult, err error) {
	/*
		1) extract selections which is not inline-fragments - e.g. shared selections
		2) extract selections for each inline fragment
		3) for types which do not have inline-fragment - add inline fragment with shared fields
		4) for types which have inline-fragment - add not selected shared fields to existing inline fragment
	*/

	interfaceTypeName, interfaceTypeNames, isInterfaceObject, err := r.getAllowedInterfaceMemberTypeNames(fieldRef, interfaceDefRef, enclosingTypeName)
	if err != nil {
		return resultNotRewritten, err
	}
	entityNames, _ := r.datasourceHasEntitiesWithName(interfaceTypeNames)

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return resultNotRewritten, err
	}
	selectionSetInfo.isInterfaceObject = isInterfaceObject

	needRewrite := r.interfaceFieldSelectionNeedsRewrite(selectionSetInfo, interfaceTypeNames, entityNames, interfaceTypeName)
	if !needRewrite {
		return resultNotRewritten, nil
	}

	fieldPaths, err := collectFieldPaths(r.operation, r.definition, fieldRef)
	if err != nil {
		return resultNotRewritten, err
	}

	err = r.rewriteInterfaceSelection(fieldRef, selectionSetInfo, interfaceTypeNames)
	if err != nil {
		return resultNotRewritten, err
	}

	changedRefs, originRefs, err := r.collectChangedRefs(fieldRef, fieldPaths)
	if err != nil {
		return resultNotRewritten, err
	}

	return RewriteResult{
		rewritten:        true,
		changedFieldRefs: changedRefs,
		fieldRefOrigins:  originRefs,
	}, nil
}

func (r *fieldSelectionRewriter) interfaceFieldSelectionNeedsRewrite(selectionSetInfo selectionSetInfo, interfaceTypeNames []string, entityNames []string, interfaceTypeName string) (needRewrite bool) {
	if r.mustRewrite(selectionSetInfo) {
		return true
	}

	if selectionSetInfo.hasFields {
		// We check that all selected fields are defined on the interface type.
		// If all implementing types have the field local to the datasource, but interface does not define it - we won't be able to plan such fields.
		// example:
		//
		// current datasource schema:
		// interface Node {
		//   id: ID!
		// }
		//
		// type User implements Node {
		//   id: ID!          <-- local to the current datasource
		//   name: String!    <-- local to the current datasource, but not defined on the interface
		// }
		//
		// other datasource schema:
		// interface Node {
		//   id: ID!
		//   name: String! <-- defined on the interface and concrete type
		// }
		//
		// type User implements Node {
		//   id: ID!
		//   name: String!
		// }
		//

		if !r.typeHasAllFieldLocal(interfaceTypeName, selectionSetInfo.fields) {
			return true
		}
	}

	// when we do not have fragments
	if !selectionSetInfo.hasInlineFragmentsOnInterfaces &&
		!selectionSetInfo.hasInlineFragmentsOnUnions &&
		!selectionSetInfo.hasInlineFragmentsOnObjects {
		// check that all types implementing the interface have a root node with the requested fields
		if !r.allEntitiesHaveFieldsAsRootNode(entityNames, selectionSetInfo.fields) {
			return true
		}

		return slices.ContainsFunc(entityNames, func(entityName string) bool {
			return r.hasRequiresConfigurationForField(entityName, selectionSetInfo.fields)
		})
	}

	if selectionSetInfo.hasInlineFragmentsOnObjects {
		if r.objectFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnObjects, interfaceTypeNames) {
			return true
		}

		// check that all inline fragments types are implementing the interface in the current datasource
		if !r.allFragmentTypesImplementsInterfaceTypes(selectionSetInfo.inlineFragmentsOnObjects, interfaceTypeNames) {
			return true
		}

		// In case it is an interface object, and we have fragments on concrete types - we have to add shared __typename selection.
		// It will mean that we will rewrite a query to separate concrete type fragments,
		// but due to the nature of the interface object, they eventually will be flattened by datasource
		// into a single fragment or just a flattened query.
		// So it should be safe to rewrite a field.
		if selectionSetInfo.isInterfaceObject {
			return !selectionSetInfo.hasTypeNameSelection
		}
	}

	entitiesWithoutFragment := r.entityNamesWithoutFragments(selectionSetInfo.inlineFragmentsOnObjects, entityNames)

	// check that all entities without fragments have a root node with the requested fields
	if selectionSetInfo.hasFields {
		if !r.allEntitiesHaveFieldsAsRootNode(entitiesWithoutFragment, selectionSetInfo.fields) {
			return true
		}

		// check if any implementing type has requiresConfiguration for one of the requested fields
		if slices.ContainsFunc(entityNames, func(entityName string) bool {
			return r.hasRequiresConfigurationForField(entityName, selectionSetInfo.fields)
		}) {
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

	if selectionSetInfo.hasInlineFragmentsOnInterfaces &&
		r.interfaceFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnInterfaces, interfaceTypeNames) {
		return true
	}

	if selectionSetInfo.hasInlineFragmentsOnUnions &&
		r.unionFragmentsRequiresCleanup(selectionSetInfo.inlineFragmentsOnUnions, interfaceTypeNames) {
		return true
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

	// When interface is an interface object
	// When we have fragments on concrete types,
	// And we do not have __typename selection - we are adding it
	if fieldInfo.isInterfaceObject && !fieldInfo.hasTypeNameSelection && fieldInfo.hasInlineFragmentsOnObjects {
		deferID, _ := r.operation.FieldInternalDeferID(fieldRef)
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection(deferID)
		r.skipFieldRefs = append(r.skipFieldRefs, typeNameFieldRef)
		newSelectionRefs = append(newSelectionRefs, typeNameSelectionRef)
	}

	r.flattenFragmentOnInterface(
		fieldInfo,
		interfaceTypeNames,
		interfaceTypeNames,
		&newSelectionRefs,
	)

	return r.replaceFieldSelections(fieldRef, newSelectionRefs)
}

func (r *fieldSelectionRewriter) flattenFragmentOnInterface(selectionSetInfo selectionSetInfo, implementingTypes []string, allowedTypeNames []string, selectionRefs *[]int) {
	allowedImplementingTypes := make([]string, 0, len(implementingTypes))
	for _, typeName := range implementingTypes {
		if slices.Contains(allowedTypeNames, typeName) {
			allowedImplementingTypes = append(allowedImplementingTypes, typeName)
		}
	}

	if selectionSetInfo.hasFields {
		for _, typeName := range allowedImplementingTypes {
			*selectionRefs = append(*selectionRefs, r.createFragmentSelection(typeName, selectionSetInfo.fields))
		}
	}

	r.flattenFragments(selectionSetInfo, allowedImplementingTypes, selectionRefs)
}

func (r *fieldSelectionRewriter) flattenFragmentOnUnion(selectionSetInfo selectionSetInfo, allowedTypeNames []string, selectionRefs *[]int) {
	r.preserveTypeNameSelection(selectionSetInfo, selectionRefs)
	r.flattenFragments(selectionSetInfo, allowedTypeNames, selectionRefs)
}

func (r *fieldSelectionRewriter) flattenFragments(selectionSetInfo selectionSetInfo, allowedTypeNames []string, selectionRefs *[]int) {
	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnObjects {
		// for object fragments it is necessary to check if inline fragment type is allowed
		if !slices.Contains(allowedTypeNames, inlineFragmentInfo.typeName) {
			// remove fragment which not allowed
			continue
		}

		r.flattenFragmentOnObject(inlineFragmentInfo.selectionSetInfo, inlineFragmentInfo.typeName, selectionRefs)
	}

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnInterfaces {
		// We do not check if interface fragment type not exists in the current datasource
		// in case of interfaces the only thing which is matter is an interception of implementing types
		// and parent allowed types

		r.flattenFragmentOnInterface(inlineFragmentInfo.selectionSetInfo, inlineFragmentInfo.typeNamesImplementingInterface, allowedTypeNames, selectionRefs)
	}

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnUnions {
		// We do not check if union fragment type not exists in the current datasource
		// in case of unions the only thing which is matter is an interception of implementing types
		// and parent allowed types
		r.flattenFragmentOnUnion(inlineFragmentInfo.selectionSetInfo, allowedTypeNames, selectionRefs)
	}
}

func (r *fieldSelectionRewriter) flattenFragmentOnObject(selectionSetInfo selectionSetInfo, typeName string, selectionRefs *[]int) {
	if selectionSetInfo.hasFields {
		*selectionRefs = append(*selectionRefs, r.createFragmentSelection(typeName, selectionSetInfo.fields))
	}

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnInterfaces {
		// We do not check if interface fragment type not exists in the current datasource
		// in case of interfaces the only thing which is matter is an interception of implementing types
		// and parent allowed types

		r.flattenFragmentOnInterface(inlineFragmentInfo.selectionSetInfo, inlineFragmentInfo.typeNamesImplementingInterface, []string{typeName}, selectionRefs)
	}

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnUnions {
		// We do not check if union fragment type not exists in the current datasource
		// in case of unions the only thing which is matter is an interception of implementing types
		// and parent allowed types
		r.flattenFragmentOnUnion(inlineFragmentInfo.selectionSetInfo, []string{typeName}, selectionRefs)
	}
}

func (r *fieldSelectionRewriter) collectChangedRefs(fieldRef int, oldFieldPaths []collectedFieldPath) (changedFieldRefs map[int][]int, fieldRefOrigins map[int][]int, err error) {
	newFieldPaths, err := collectFieldPaths(r.operation, r.definition, fieldRef)
	if err != nil {
		return nil, nil, err
	}

	// group new entries by path to compare only the fields which could occupy the same response position
	newEntriesByPath := make(map[string][]int, len(newFieldPaths))
	for i, entry := range newFieldPaths {
		newEntriesByPath[entry.path] = append(newEntriesByPath[entry.path], i)
	}

	changedFieldRefs = make(map[int][]int, len(oldFieldPaths))
	fieldRefOrigins = make(map[int][]int, len(newFieldPaths))

	for _, oldEntry := range oldFieldPaths {
		newEntryIndexes, ok := newEntriesByPath[oldEntry.path]
		if !ok {
			// TODO: some paths could actually disappear due to rewrite
			continue
		}

		newRefs := make([]int, 0, len(newEntryIndexes))
		for _, i := range newEntryIndexes {
			newEntry := newFieldPaths[i]
			if !scopeChainsIntersect(oldEntry.scopes, newEntry.scopes) {
				// fields with the same path but non-intersecting type condition scopes
				// could not occupy the same response position - e.g. fragments on different concrete types
				continue
			}
			newRefs = append(newRefs, newEntry.ref)
			fieldRefOrigins[newEntry.ref] = append(fieldRefOrigins[newEntry.ref], oldEntry.ref)
		}

		if len(newRefs) == 0 {
			continue
		}

		if len(newRefs) == 1 && newRefs[0] == oldEntry.ref {
			continue
		}

		changedFieldRefs[oldEntry.ref] = newRefs
	}

	return changedFieldRefs, fieldRefOrigins, nil
}

// collectedFieldPath describes the response position of a field within the rewritten field subtree:
// a dot-delimited path of field aliases with inline fragment names excluded,
// and a chain of type condition scopes - one per field nesting level.
type collectedFieldPath struct {
	ref    int
	path   string
	scopes scopeChain
}

// scopeChain holds one scope per field nesting level.
// Each scope is a list of concrete type names allowed by the inline fragment type conditions
// enclosing the field at that level; a nil scope means the level is unconstrained.
type scopeChain [][]string

// scopeChainsIntersect reports whether two fields with an equal path could occupy
// the same response position - e.g. whether at every nesting level
// their type condition scopes have at least one concrete type in common.
func scopeChainsIntersect(a, b scopeChain) bool {
	levels := min(len(a), len(b))
	for i := range levels {
		if !scopesIntersect(a[i], b[i]) {
			return false
		}
	}
	return true
}

func scopesIntersect(a, b []string) bool {
	if a == nil || b == nil {
		// nil scope is unconstrained and intersects with everything
		return true
	}
	return slices.ContainsFunc(a, func(typeName string) bool {
		return slices.Contains(b, typeName)
	})
}

func intersectScopes(a, b []string) []string {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := make([]string, 0, len(a))
	for _, typeName := range a {
		if slices.Contains(b, typeName) {
			out = append(out, typeName)
		}
	}
	return out
}

type AbstractFieldPathCollector struct {
	*astvisitor.Walker

	operation  *ast.Document
	definition *ast.Document

	entries []collectedFieldPath

	levelScopes [][]string // current type condition scope per field nesting level
	savedScopes [][]string // restore stack for the enclosing inline fragments
}

func (v *AbstractFieldPathCollector) EnterField(ref int) {
	parentPath := v.Walker.Path.WithoutInlineFragmentNames().DotDelimitedString()
	currentPath := parentPath + "." + v.operation.FieldAliasOrNameString(ref)

	scopes := make(scopeChain, len(v.levelScopes))
	copy(scopes, v.levelScopes)

	v.entries = append(v.entries, collectedFieldPath{
		ref:    ref,
		path:   currentPath,
		scopes: scopes,
	})

	// selections of the current field start a new unconstrained nesting level
	v.levelScopes = append(v.levelScopes, nil)
}

func (v *AbstractFieldPathCollector) LeaveField(ref int) {
	v.levelScopes = v.levelScopes[:len(v.levelScopes)-1]
}

func (v *AbstractFieldPathCollector) EnterInlineFragment(ref int) {
	current := v.levelScopes[len(v.levelScopes)-1]
	v.savedScopes = append(v.savedScopes, current)
	v.levelScopes[len(v.levelScopes)-1] = intersectScopes(current, v.resolveTypeCondition(ref))
}

func (v *AbstractFieldPathCollector) LeaveInlineFragment(ref int) {
	v.levelScopes[len(v.levelScopes)-1] = v.savedScopes[len(v.savedScopes)-1]
	v.savedScopes = v.savedScopes[:len(v.savedScopes)-1]
}

// resolveTypeCondition resolves an inline fragment type condition into a list of concrete type names.
// Returns nil for a fragment without a type condition or with an unresolvable abstract type condition -
// such fragments do not constrain the scope.
func (v *AbstractFieldPathCollector) resolveTypeCondition(inlineFragmentRef int) []string {
	typeConditionName := v.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
	if typeConditionName == "" {
		return nil
	}

	node, exists := v.definition.Index.FirstNodeByNameStr(typeConditionName)
	if !exists {
		return []string{typeConditionName}
	}

	switch node.Kind {
	case ast.NodeKindInterfaceTypeDefinition:
		typeNames, _ := v.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
		return typeNames
	case ast.NodeKindUnionTypeDefinition:
		typeNames, _ := v.definition.UnionTypeDefinitionMemberTypeNames(node.Ref)
		return typeNames
	default:
		return []string{typeConditionName}
	}
}

func collectFieldPaths(operation *ast.Document, definition *ast.Document, fieldRef int) ([]collectedFieldPath, error) {
	walker := astvisitor.NewWalkerWithID(4, "AbstractFieldPathCollector")

	c := &AbstractFieldPathCollector{
		Walker:      &walker,
		operation:   operation,
		definition:  definition,
		levelScopes: [][]string{nil},
	}

	filter := &FieldLimitedVisitor{
		Walker:         &walker,
		targetFieldRef: fieldRef,
	}

	walker.RegisterFieldVisitor(filter)
	walker.SetVisitorFilter(filter)
	walker.RegisterFieldVisitor(c)
	walker.RegisterInlineFragmentVisitor(c)

	report := &operationreport.Report{}
	walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil, report
	}

	return c.entries, nil
}

type FieldLimitedVisitor struct {
	*astvisitor.Walker

	targetFieldRef int
	allow          bool
}

func (v *FieldLimitedVisitor) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor any, skipFor astvisitor.SkipVisitors) bool {
	if visitor == v {
		return true
	}

	return v.allow
}

func (v *FieldLimitedVisitor) EnterField(ref int) {
	if ref == v.targetFieldRef {
		v.allow = true
		return
	}
}

func (v *FieldLimitedVisitor) LeaveField(ref int) {
	if ref == v.targetFieldRef {
		v.allow = false
		v.Stop()
	}
}
