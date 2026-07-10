package responsejsonschema

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

func fieldRefsByResponsePath(operation *ast.Document, selectionSetRef int, fieldPath []string) ([]int, error) {
	selectionSetRefs := []int{selectionSetRef}

	for index, responseName := range fieldPath {
		fieldGroup, ok, err := selectedFieldGroupByResponseName(operation, selectionSetRefs, responseName)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf(
				"response field %q not found at path %q",
				responseName,
				strings.Join(fieldPath[:index+1], "."),
			)
		}

		if index == len(fieldPath)-1 {
			fieldRefs := make([]int, 0, len(fieldGroup.fields))
			for _, field := range fieldGroup.fields {
				fieldRefs = append(fieldRefs, field.ref)
			}
			return fieldRefs, nil
		}

		selectionSetRefs = selectionSetRefs[:0]
		for _, selectedField := range fieldGroup.fields {
			fieldRef := selectedField.ref
			field := operation.Fields[fieldRef]
			if !field.HasSelections {
				return nil, fmt.Errorf(
					"response field %q has no selection set while resolving path %q",
					responseName,
					strings.Join(fieldPath, "."),
				)
			}
			selectionSetRefs = append(selectionSetRefs, field.SelectionSet)
		}
	}

	return nil, fmt.Errorf("response field path %q could not be resolved", strings.Join(fieldPath, "."))
}

type responseFieldCandidate struct {
	fieldDefinitionRef int
	fields             []selectedField
}

func fieldCandidatesByResponsePath(operation, definition *ast.Document, operationDefinition *ast.OperationDefinition, fieldPath []string) ([]responseFieldCandidate, error) {
	rootTypeName, err := rootTypeName(definition, operationDefinition.OperationType)
	if err != nil {
		return nil, err
	}

	parentNode, ok := definition.Index.FirstNodeByNameStr(rootTypeName)
	if !ok || parentNode.Kind != ast.NodeKindObjectTypeDefinition {
		return nil, fmt.Errorf("root object type %q is not defined", rootTypeName)
	}

	selectionSetRefs := []int{operationDefinition.SelectionSet}
	for index, responseName := range fieldPath {
		fieldGroup, ok, err := selectedFieldGroupByResponseName(operation, selectionSetRefs, responseName)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf(
				"response field %q not found at path %q",
				responseName,
				strings.Join(fieldPath[:index+1], "."),
			)
		}
		if err := validateRepeatedAliasDomains(operation, definition, parentNode, fieldGroup); err != nil {
			return nil, err
		}

		fieldDefinitionRefs := make([]int, len(fieldGroup.fields))
		for fieldIndex, field := range fieldGroup.fields {
			fieldDefinitionRef, err := selectedFieldDefinition(operation, definition, parentNode, field)
			if err != nil {
				return nil, err
			}
			fieldDefinitionRefs[fieldIndex] = fieldDefinitionRef
		}

		if index == len(fieldPath)-1 {
			fieldsByDefinitionRef := make(map[int][]selectedField)
			for fieldIndex, fieldDefinitionRef := range fieldDefinitionRefs {
				fieldsByDefinitionRef[fieldDefinitionRef] = append(fieldsByDefinitionRef[fieldDefinitionRef], fieldGroup.fields[fieldIndex])
			}
			definitionRefs := make([]int, 0, len(fieldsByDefinitionRef))
			for fieldDefinitionRef := range fieldsByDefinitionRef {
				definitionRefs = append(definitionRefs, fieldDefinitionRef)
			}
			sort.Ints(definitionRefs)
			candidates := make([]responseFieldCandidate, 0, len(definitionRefs))
			for _, fieldDefinitionRef := range definitionRefs {
				candidates = append(candidates, responseFieldCandidate{
					fieldDefinitionRef: fieldDefinitionRef,
					fields:             fieldsByDefinitionRef[fieldDefinitionRef],
				})
			}
			return candidates, nil
		}

		selectionSetRefs = selectionSetRefs[:0]
		for _, repeatedField := range fieldGroup.fields {
			repeatedFieldRef := repeatedField.ref
			field := operation.Fields[repeatedFieldRef]
			if !field.HasSelections {
				return nil, fmt.Errorf(
					"response field %q has no selection set while resolving path %q",
					responseName,
					strings.Join(fieldPath, "."),
				)
			}
			selectionSetRefs = append(selectionSetRefs, field.SelectionSet)
		}

		parentTypeName := definition.ResolveTypeNameString(definition.FieldDefinitions[fieldDefinitionRefs[0]].Type)
		for _, fieldDefinitionRef := range fieldDefinitionRefs[1:] {
			repeatedParentTypeName := definition.ResolveTypeNameString(definition.FieldDefinitions[fieldDefinitionRef].Type)
			if repeatedParentTypeName != parentTypeName {
				return nil, fmt.Errorf(
					"response field %q has incompatible nested return types %q and %q while resolving path %q",
					responseName,
					parentTypeName,
					repeatedParentTypeName,
					strings.Join(fieldPath, "."),
				)
			}
		}
		parentNode, ok = definition.Index.FirstNodeByNameStr(parentTypeName)
		if !ok || !isCompositeTypeNode(parentNode) {
			return nil, fmt.Errorf("composite type %q is not defined", parentTypeName)
		}
	}

	return nil, fmt.Errorf("response field path %q could not be resolved", strings.Join(fieldPath, "."))
}

func selectedFieldDefinition(operation, definition *ast.Document, parentNode ast.Node, field selectedField) (int, error) {
	fieldName := operation.FieldNameBytes(field.ref)
	for index := len(field.typeConditions) - 1; index >= 0; index-- {
		typeCondition := field.typeConditions[index]
		conditionNode, ok := definition.Index.FirstNodeByNameStr(typeCondition)
		if !ok {
			return ast.InvalidRef, fmt.Errorf("fragment type condition %q is not defined", typeCondition)
		}
		if fieldDefinitionRef, ok := fieldDefinitionOnNode(definition, conditionNode, fieldName); ok {
			return fieldDefinitionRef, nil
		}
	}

	if fieldDefinitionRef, ok := fieldDefinitionOnNode(definition, parentNode, fieldName); ok {
		return fieldDefinitionRef, nil
	}

	return ast.InvalidRef, fmt.Errorf(
		"field %q is not defined on response parent type %q",
		operation.FieldNameString(field.ref),
		definition.NodeNameString(parentNode),
	)
}

func fieldDefinitionOnNode(definition *ast.Document, node ast.Node, fieldName []byte) (int, bool) {
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		return definition.ObjectTypeDefinitionFieldWithName(node.Ref, fieldName)
	case ast.NodeKindInterfaceTypeDefinition:
		return definition.InterfaceTypeDefinitionFieldWithName(node.Ref, fieldName)
	default:
		return ast.InvalidRef, false
	}
}

func isCompositeTypeNode(node ast.Node) bool {
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
		return true
	default:
		return false
	}
}

type runtimeTypeDomain map[string]struct{}

func validateRepeatedAliasDomains(operation, definition *ast.Document, parentNode ast.Node, fieldGroup selectedFieldGroup) error {
	domains := make([]runtimeTypeDomain, len(fieldGroup.fields))
	for index, field := range fieldGroup.fields {
		domain, err := selectedFieldRuntimeDomain(definition, parentNode, field)
		if err != nil {
			return fmt.Errorf("resolve runtime types for response field %q: %w", fieldGroup.responseName, err)
		}
		domains[index] = domain
	}

	for left := 0; left < len(fieldGroup.fields); left++ {
		for right := left + 1; right < len(fieldGroup.fields); right++ {
			leftRef := fieldGroup.fields[left].ref
			rightRef := fieldGroup.fields[right].ref
			if operation.FieldNameString(leftRef) == operation.FieldNameString(rightRef) {
				continue
			}
			overlap := intersectRuntimeTypeDomains(domains[left], domains[right])
			if len(overlap) == 0 {
				continue
			}
			return fmt.Errorf(
				"response field %q combines incompatible fields %q and %q on overlapping runtime types %q",
				fieldGroup.responseName,
				operation.FieldNameString(leftRef),
				operation.FieldNameString(rightRef),
				sortedRuntimeTypeDomain(overlap),
			)
		}
	}
	return nil
}

func selectedFieldRuntimeDomain(definition *ast.Document, parentNode ast.Node, field selectedField) (runtimeTypeDomain, error) {
	domain, err := possibleRuntimeTypes(definition, parentNode)
	if err != nil {
		return nil, err
	}
	for _, typeCondition := range field.typeConditions {
		conditionNode, ok := definition.Index.FirstNodeByNameStr(typeCondition)
		if !ok {
			return nil, fmt.Errorf("fragment type condition %q is not defined", typeCondition)
		}
		conditionDomain, err := possibleRuntimeTypes(definition, conditionNode)
		if err != nil {
			return nil, fmt.Errorf("fragment type condition %q: %w", typeCondition, err)
		}
		domain = intersectRuntimeTypeDomains(domain, conditionDomain)
	}
	return domain, nil
}

func possibleRuntimeTypes(definition *ast.Document, node ast.Node) (runtimeTypeDomain, error) {
	domain := make(runtimeTypeDomain)
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition:
		domain[definition.ObjectTypeDefinitionNameString(node.Ref)] = struct{}{}
	case ast.NodeKindInterfaceTypeDefinition:
		typeNames, _ := definition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
		for _, typeName := range typeNames {
			domain[typeName] = struct{}{}
		}
	case ast.NodeKindUnionTypeDefinition:
		typeNames, _ := definition.UnionTypeDefinitionMemberTypeNames(node.Ref)
		for _, typeName := range typeNames {
			domain[typeName] = struct{}{}
		}
	default:
		return nil, fmt.Errorf("type %q is not a composite response type", definition.NodeNameString(node))
	}
	return domain, nil
}

func intersectRuntimeTypeDomains(left, right runtimeTypeDomain) runtimeTypeDomain {
	intersection := make(runtimeTypeDomain)
	for typeName := range left {
		if _, ok := right[typeName]; ok {
			intersection[typeName] = struct{}{}
		}
	}
	return intersection
}

func sortedRuntimeTypeDomain(domain runtimeTypeDomain) []string {
	typeNames := make([]string, 0, len(domain))
	for typeName := range domain {
		typeNames = append(typeNames, typeName)
	}
	sort.Strings(typeNames)
	return typeNames
}

func selectedFieldGroupsForRuntimeType(definition *ast.Document, groups []selectedFieldGroup, runtimeTypeName string) ([]selectedFieldGroup, error) {
	filteredGroups := make([]selectedFieldGroup, 0, len(groups))
	for _, group := range groups {
		filteredFields := make([]selectedField, 0, len(group.fields))
		for _, field := range group.fields {
			applies, err := selectedFieldAppliesToRuntimeType(definition, field, runtimeTypeName)
			if err != nil {
				return nil, fmt.Errorf("response property %q: %w", group.responseName, err)
			}
			if applies {
				filteredFields = append(filteredFields, field)
			}
		}
		if len(filteredFields) != 0 {
			filteredGroups = append(filteredGroups, selectedFieldGroup{
				responseName: group.responseName,
				fields:       filteredFields,
			})
		}
	}
	return filteredGroups, nil
}

func selectedFieldAppliesToRuntimeType(definition *ast.Document, field selectedField, runtimeTypeName string) (bool, error) {
	for _, typeCondition := range field.typeConditions {
		conditionNode, ok := definition.Index.FirstNodeByNameStr(typeCondition)
		if !ok {
			return false, fmt.Errorf("fragment type condition %q is not defined", typeCondition)
		}
		conditionDomain, err := possibleRuntimeTypes(definition, conditionNode)
		if err != nil {
			return false, fmt.Errorf("fragment type condition %q: %w", typeCondition, err)
		}
		if _, applies := conditionDomain[runtimeTypeName]; !applies {
			return false, nil
		}
	}
	return true, nil
}

func rootTypeName(definition *ast.Document, operationType ast.OperationType) (string, error) {
	var configuredName, defaultName []byte

	switch operationType {
	case ast.OperationTypeQuery:
		configuredName = definition.Index.QueryTypeName
		defaultName = ast.DefaultQueryTypeName
	case ast.OperationTypeMutation:
		configuredName = definition.Index.MutationTypeName
		defaultName = ast.DefaultMutationTypeName
	case ast.OperationTypeSubscription:
		configuredName = definition.Index.SubscriptionTypeName
		defaultName = ast.DefaultSubscriptionTypeName
	default:
		return "", fmt.Errorf("unsupported operation type %q", operationType)
	}

	if len(configuredName) != 0 {
		return string(configuredName), nil
	}

	return string(defaultName), nil
}

type selectedFieldGroup struct {
	responseName string
	fields       []selectedField
}

type selectedField struct {
	ref            int
	conditional    bool
	typeConditions []string
}

func selectedFieldGroups(operation *ast.Document, selectionSetRefs []int) ([]selectedFieldGroup, error) {
	groupsByResponseName := make(map[string][]selectedField)
	for _, selectionSetRef := range selectionSetRefs {
		var fields []selectedField
		if err := collectSelectedFields(operation, selectionSetRef, make(map[int]struct{}), false, nil, &fields); err != nil {
			return nil, err
		}
		for _, field := range fields {
			responseName := operation.FieldAliasOrNameString(field.ref)
			groupsByResponseName[responseName] = append(groupsByResponseName[responseName], field)
		}
	}

	responseNames := make([]string, 0, len(groupsByResponseName))
	for responseName := range groupsByResponseName {
		responseNames = append(responseNames, responseName)
	}
	sort.Strings(responseNames)

	groups := make([]selectedFieldGroup, 0, len(responseNames))
	for _, responseName := range responseNames {
		groups = append(groups, selectedFieldGroup{
			responseName: responseName,
			fields:       groupsByResponseName[responseName],
		})
	}
	return groups, nil
}

func selectedFieldGroupByResponseName(operation *ast.Document, selectionSetRefs []int, responseName string) (selectedFieldGroup, bool, error) {
	fieldGroups, err := selectedFieldGroups(operation, selectionSetRefs)
	if err != nil {
		return selectedFieldGroup{}, false, err
	}
	for _, fieldGroup := range fieldGroups {
		if fieldGroup.responseName == responseName {
			return fieldGroup, true, nil
		}
	}
	return selectedFieldGroup{}, false, nil
}

func collectSelectedFields(
	operation *ast.Document,
	selectionSetRef int,
	activeFragments map[int]struct{},
	conditional bool,
	typeConditions []string,
	fields *[]selectedField,
) error {
	if selectionSetRef < 0 || selectionSetRef >= len(operation.SelectionSets) {
		return fmt.Errorf("selection set reference %d is out of bounds", selectionSetRef)
	}

	for _, selectionRef := range operation.SelectionSets[selectionSetRef].SelectionRefs {
		selection := operation.Selections[selectionRef]
		switch selection.Kind {
		case ast.SelectionKindField:
			include, selectionConditional, err := selectionCondition(operation, operation.FieldDirectives(selection.Ref))
			if err != nil {
				return fmt.Errorf("field %q: %w", operation.FieldAliasOrNameString(selection.Ref), err)
			}
			if !include {
				continue
			}
			*fields = append(*fields, selectedField{
				ref:            selection.Ref,
				conditional:    conditional || selectionConditional,
				typeConditions: append([]string(nil), typeConditions...),
			})
		case ast.SelectionKindInlineFragment:
			inlineFragment := operation.InlineFragments[selection.Ref]
			include, selectionConditional, err := selectionCondition(operation, operation.InlineFragmentDirectives(selection.Ref))
			if err != nil {
				return fmt.Errorf("inline fragment on %q: %w", operation.InlineFragmentTypeConditionNameString(selection.Ref), err)
			}
			if !include {
				continue
			}
			childTypeConditions := typeConditions
			if operation.InlineFragmentHasTypeCondition(selection.Ref) {
				childTypeConditions = appendTypeCondition(typeConditions, operation.InlineFragmentTypeConditionNameString(selection.Ref))
			}
			if inlineFragment.HasSelections {
				if err := collectSelectedFields(operation, inlineFragment.SelectionSet, activeFragments, conditional || selectionConditional, childTypeConditions, fields); err != nil {
					return err
				}
			}
		case ast.SelectionKindFragmentSpread:
			include, selectionConditional, err := selectionCondition(operation, operation.FragmentSpreads[selection.Ref].Directives.Refs)
			if err != nil {
				return fmt.Errorf("fragment spread %q: %w", operation.FragmentSpreadNameString(selection.Ref), err)
			}
			if !include {
				continue
			}
			fragmentName := operation.FragmentSpreadNameBytes(selection.Ref)
			fragmentDefinitionRef, ok := operation.FragmentDefinitionRef(fragmentName)
			if !ok {
				return fmt.Errorf("fragment %q is not defined", fragmentName)
			}
			if _, active := activeFragments[fragmentDefinitionRef]; active {
				return fmt.Errorf("fragment %q contains a cyclic spread", fragmentName)
			}
			fragmentDefinition := operation.FragmentDefinitions[fragmentDefinitionRef]
			if !fragmentDefinition.HasSelections {
				continue
			}
			childTypeConditions := appendTypeCondition(typeConditions, operation.FragmentDefinitionTypeNameString(fragmentDefinitionRef))
			activeFragments[fragmentDefinitionRef] = struct{}{}
			if err := collectSelectedFields(operation, fragmentDefinition.SelectionSet, activeFragments, conditional || selectionConditional, childTypeConditions, fields); err != nil {
				return err
			}
			delete(activeFragments, fragmentDefinitionRef)
		}
	}
	return nil
}

func appendTypeCondition(typeConditions []string, typeCondition string) []string {
	appended := make([]string, len(typeConditions), len(typeConditions)+1)
	copy(appended, typeConditions)
	return append(appended, typeCondition)
}

func selectionCondition(operation *ast.Document, directiveRefs []int) (include, conditional bool, err error) {
	include = true
	for _, directiveRef := range directiveRefs {
		directiveName := operation.DirectiveNameBytes(directiveRef)
		if !bytes.Equal(directiveName, literal.INCLUDE) && !bytes.Equal(directiveName, literal.SKIP) {
			continue
		}

		condition, ok := operation.DirectiveArgumentValueByName(directiveRef, literal.IF)
		if !ok {
			return false, false, fmt.Errorf("@%s directive is missing its if condition", directiveName)
		}
		switch condition.Kind {
		case ast.ValueKindBoolean:
			conditionValue := bool(operation.BooleanValue(condition.Ref))
			if bytes.Equal(directiveName, literal.INCLUDE) && !conditionValue ||
				bytes.Equal(directiveName, literal.SKIP) && conditionValue {
				include = false
			}
		case ast.ValueKindVariable:
			conditional = true
		default:
			return false, false, fmt.Errorf("@%s directive has invalid if condition kind %q", directiveName, condition.Kind)
		}
	}
	return include, conditional, nil
}
