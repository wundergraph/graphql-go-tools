package plan

import (
	"errors"
	"fmt"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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

	skipFieldRefs []int
}

type RewriteResult struct {
	rewritten        bool
	changedFieldRefs map[int][]int // map[fieldRef][]fieldRef - for each original fieldRef list of new fieldRefs
}

var resultNotRewritten = RewriteResult{}

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

	fieldRefPaths, _, err := collectPath(r.operation, r.definition, fieldRef, true)
	if err != nil {
		return resultNotRewritten, err
	}

	err = r.rewriteUnionSelection(fieldRef, selectionSetInfo, unionTypeNames)
	if err != nil {
		return resultNotRewritten, err
	}

	changedRefs, err := r.collectChangedRefs(fieldRef, fieldRefPaths)
	if err != nil {
		return resultNotRewritten, err
	}

	return RewriteResult{
		rewritten:        true,
		changedFieldRefs: changedRefs,
	}, nil
}

func (r *fieldSelectionRewriter) unionFieldSelectionNeedsRewrite(selectionSetInfo selectionSetInfo, unionTypeNames, entityNames []string) (needRewrite bool) {
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

	r.preserveTypeNameSelection(fieldInfo, &newSelectionRefs)

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
		// we have to add __typename selection in case there is no other selections
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection()
		r.skipFieldRefs = append(r.skipFieldRefs, typeNameFieldRef)
		r.operation.AddSelectionRefToSelectionSet(fieldSelectionSetRef, typeNameSelectionRef)
	}

	op, _ := astprinter.PrintStringIndent(r.operation, "  ")
	fmt.Println("flattened operation:\n", op)

	normalizer := astnormalization.NewAbstractFieldNormalizer(r.operation, r.definition, fieldRef)
	if err := normalizer.Normalize(); err != nil {
		return err
	}

	return nil
}

func (r *fieldSelectionRewriter) processInterfaceSelection(fieldRef int, interfaceDefRef int, enclosingTypeName ast.ByteSlice) (res RewriteResult, err error) {
	/*
		1) extract selections which is not inline-fragments - e.g. shared selections
		2) extract selections for each inline fragment
		3) for types which do not have inline-fragment - add inline fragment with shared fields
		4) for types which have inline-fragment - add not selected shared fields to existing inline fragment
	*/

	interfaceTypeNames, isInterfaceObject, err := r.getAllowedInterfaceMemberTypeNames(fieldRef, interfaceDefRef, enclosingTypeName)
	if err != nil {
		return resultNotRewritten, err
	}
	entityNames, _ := r.datasourceHasEntitiesWithName(interfaceTypeNames)

	selectionSetInfo, err := r.collectFieldInformation(fieldRef)
	if err != nil {
		return resultNotRewritten, err
	}
	selectionSetInfo.isInterfaceObject = isInterfaceObject

	needRewrite := r.interfaceFieldSelectionNeedsRewrite(selectionSetInfo, interfaceTypeNames, entityNames)
	if !needRewrite {
		return resultNotRewritten, nil
	}

	fieldRefPaths, _, err := collectPath(r.operation, r.definition, fieldRef, true)
	if err != nil {
		return resultNotRewritten, err
	}

	err = r.rewriteInterfaceSelection(fieldRef, selectionSetInfo, interfaceTypeNames)
	if err != nil {
		return resultNotRewritten, err
	}

	changedRefs, err := r.collectChangedRefs(fieldRef, fieldRefPaths)
	if err != nil {
		return resultNotRewritten, err
	}

	return RewriteResult{
		rewritten:        true,
		changedFieldRefs: changedRefs,
	}, nil
}

func (r *fieldSelectionRewriter) interfaceFieldSelectionNeedsRewrite(selectionSetInfo selectionSetInfo, interfaceTypeNames []string, entityNames []string) (needRewrite bool) {
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

		// in case it is an interface object, and we have fragments on concrete types - we have to add shared __typename selection
		// it will mean that we will rewrite a query to separate concrete type fragments, but due to nature of the interface object
		// they eventually will be flattened by datasource into a single fragment or just a flatten query.
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
		typeNameSelectionRef, typeNameFieldRef := r.typeNameSelection()
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

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnObjects {
		// for object fragments it is necessary to check if inline fragment type is allowed
		if !slices.Contains(allowedImplementingTypes, inlineFragmentInfo.typeName) {
			// remove fragment which not allowed
			continue
		}

		r.flattenFragmentOnObject(inlineFragmentInfo.selectionSetInfo, inlineFragmentInfo.typeName, selectionRefs)
	}

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnInterfaces {
		// We do not check if interface fragment type not exists in the current datasource
		// in case of interfaces the only thing which is matter is an interception of implementing types
		// and parent allowed types

		r.flattenFragmentOnInterface(inlineFragmentInfo.selectionSetInfo, inlineFragmentInfo.typeNamesImplementingInterface, allowedImplementingTypes, selectionRefs)
	}

	for _, inlineFragmentInfo := range selectionSetInfo.inlineFragmentsOnUnions {
		// We do not check if union fragment type not exists in the current datasource
		// in case of unions the only thing which is matter is an interception of implementing types
		// and parent allowed types
		r.flattenFragmentOnUnion(inlineFragmentInfo.selectionSetInfo, allowedImplementingTypes, selectionRefs)
	}
}

func (r *fieldSelectionRewriter) flattenFragmentOnUnion(selectionSetInfo selectionSetInfo, allowedTypeNames []string, selectionRefs *[]int) {
	r.preserveTypeNameSelection(selectionSetInfo, selectionRefs)

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

func (r *fieldSelectionRewriter) collectChangedRefs(fieldRef int, fieldRefsPaths map[int]string) (map[int][]int, error) {
	_, pathsToRefs, err := collectPath(r.operation, r.definition, fieldRef, false)
	if err != nil {
		return nil, err
	}

	out := make(map[int][]int, len(fieldRefsPaths))

	for fieldRef, path := range fieldRefsPaths {
		newRefs, ok := pathsToRefs[path]
		if !ok {
			// TODO: some path actually could dissapear due to rewrite
			continue
		}

		if len(newRefs) == 0 {
			continue
		}

		if len(newRefs) == 1 && newRefs[0] == fieldRef {
			continue
		}

		out[fieldRef] = newRefs
	}

	return out, nil
}

type AbstractFieldPathCollector struct {
	*astvisitor.Walker

	operation  *ast.Document
	definition *ast.Document

	targetFieldRef int
	allow          bool
	fieldRefPaths  map[int]string
	pathFieldRefs  map[string][]int
	fieldToPath    bool
}

func (v *AbstractFieldPathCollector) EnterField(ref int) {
	parentPath := v.Walker.Path.WithoutInlineFragmentNames().DotDelimitedString()
	currentFieldName := v.operation.FieldNameString(ref)
	currentPath := parentPath + "." + currentFieldName

	if v.fieldToPath {
		v.fieldRefPaths[ref] = currentPath
		return
	}

	if _, ok := v.pathFieldRefs[currentPath]; !ok {
		v.pathFieldRefs[currentPath] = make([]int, 0, 1)
	}
	v.pathFieldRefs[currentPath] = append(v.pathFieldRefs[currentPath], ref)
}

func collectPath(operation *ast.Document, definition *ast.Document, fieldRef int, fieldToPath bool) (fieldRefPaths map[int]string, pathFieldRefs map[string][]int, err error) {
	walker := astvisitor.NewWalker(4)

	c := &AbstractFieldPathCollector{
		Walker:         &walker,
		operation:      operation,
		definition:     definition,
		targetFieldRef: fieldRef,
		fieldRefPaths:  make(map[int]string),
		pathFieldRefs:  make(map[string][]int),
		fieldToPath:    fieldToPath,
	}

	filter := &FieldLimitedVisitor{
		Walker:         &walker,
		targetFieldRef: fieldRef,
	}

	walker.RegisterFieldVisitor(filter)
	walker.SetVisitorFilter(filter)
	walker.RegisterEnterFieldVisitor(c)

	report := &operationreport.Report{}
	walker.Walk(c.operation, c.definition, report)
	if report.HasErrors() {
		return nil, nil, report
	}

	return c.fieldRefPaths, c.pathFieldRefs, nil
}

type FieldLimitedVisitor struct {
	*astvisitor.Walker

	targetFieldRef int
	allow          bool
}

func (v *FieldLimitedVisitor) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor interface{}, skipFor astvisitor.SkipVisitors) bool {
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
