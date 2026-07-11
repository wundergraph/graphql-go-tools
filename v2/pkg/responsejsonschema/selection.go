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
	traversal := newBuildTraversal()

	for index, responseName := range fieldPath {
		fieldGroup, ok, err := selectedFieldGroupByResponseName(operation, selectionSetRefs, responseName, traversal)
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

func fieldCandidatesByResponsePath(operation, definition *ast.Document, operationDefinition *ast.OperationDefinition, fieldPath []string, traversal *buildTraversal) ([]responseFieldCandidate, error) {
	rootTypeName, err := rootTypeName(definition, operationDefinition.OperationType)
	if err != nil {
		return nil, err
	}

	parentNode, ok, err := checkedIndexNode(definition, rootTypeName)
	if err != nil {
		return nil, err
	}
	if !ok || parentNode.Kind != ast.NodeKindObjectTypeDefinition {
		return nil, fmt.Errorf("root object type %q is not defined", rootTypeName)
	}
	if _, err := checkedDefinitionNodeName(definition, parentNode); err != nil {
		return nil, err
	}

	selectionSetRefs := []int{operationDefinition.SelectionSet}
	for index, responseName := range fieldPath {
		fieldGroup, ok, err := selectedFieldGroupByResponseName(operation, selectionSetRefs, responseName, traversal)
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

		parentTypeName, err := checkedDefinitionTypeName(
			definition,
			definition.FieldDefinitions[fieldDefinitionRefs[0]].Type,
			"field definition type",
		)
		if err != nil {
			return nil, err
		}
		for _, fieldDefinitionRef := range fieldDefinitionRefs[1:] {
			repeatedParentTypeName, err := checkedDefinitionTypeName(
				definition,
				definition.FieldDefinitions[fieldDefinitionRef].Type,
				"field definition type",
			)
			if err != nil {
				return nil, err
			}
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
		parentNode, ok, err = checkedIndexNode(definition, parentTypeName)
		if err != nil {
			return nil, err
		}
		if !ok || !isCompositeTypeNode(parentNode) {
			return nil, fmt.Errorf("composite type %q is not defined", parentTypeName)
		}
		if _, err := checkedDefinitionNodeName(definition, parentNode); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("response field path %q could not be resolved", strings.Join(fieldPath, "."))
}

func selectedFieldDefinition(operation, definition *ast.Document, parentNode ast.Node, field selectedField) (int, error) {
	fieldNameString, _, err := checkedOperationFieldNames(operation, field.ref)
	if err != nil {
		return ast.InvalidRef, err
	}
	fieldName := []byte(fieldNameString)
	for index := len(field.typeConditions) - 1; index >= 0; index-- {
		typeCondition := field.typeConditions[index]
		conditionNode, ok, err := checkedIndexNode(definition, typeCondition)
		if err != nil {
			return ast.InvalidRef, err
		}
		if !ok {
			return ast.InvalidRef, fmt.Errorf("fragment type condition %q is not defined", typeCondition)
		}
		fieldDefinitionRef, ok, err := checkedFieldDefinitionOnNode(definition, conditionNode, fieldName)
		if err != nil {
			return ast.InvalidRef, err
		}
		if ok {
			return fieldDefinitionRef, nil
		}
	}

	fieldDefinitionRef, ok, err := checkedFieldDefinitionOnNode(definition, parentNode, fieldName)
	if err != nil {
		return ast.InvalidRef, err
	}
	if ok {
		return fieldDefinitionRef, nil
	}
	parentTypeName, err := checkedDefinitionNodeName(definition, parentNode)
	if err != nil {
		return ast.InvalidRef, err
	}

	return ast.InvalidRef, fmt.Errorf(
		"field %q is not defined on response parent type %q",
		fieldNameString,
		parentTypeName,
	)
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
		conditionNode, ok, err := checkedIndexNode(definition, typeCondition)
		if err != nil {
			return nil, err
		}
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
	return checkedPossibleRuntimeTypes(definition, node)
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
		conditionNode, ok, err := checkedIndexNode(definition, typeCondition)
		if err != nil {
			return false, err
		}
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

func selectedFieldGroups(operation *ast.Document, selectionSetRefs []int, traversal *buildTraversal) ([]selectedFieldGroup, error) {
	groupsByResponseName := make(map[string][]selectedField)
	for _, selectionSetRef := range selectionSetRefs {
		var fields []selectedField
		if err := collectSelectedFields(operation, selectionSetRef, make(map[int]struct{}), false, nil, &fields, traversal); err != nil {
			return nil, err
		}
		for _, field := range fields {
			_, responseName, err := checkedOperationFieldNames(operation, field.ref)
			if err != nil {
				return nil, err
			}
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

func selectedFieldGroupByResponseName(operation *ast.Document, selectionSetRefs []int, responseName string, traversal *buildTraversal) (selectedFieldGroup, bool, error) {
	fieldGroups, err := selectedFieldGroups(operation, selectionSetRefs, traversal)
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
	traversal *buildTraversal,
) error {
	if selectionSetRef < 0 || selectionSetRef >= len(operation.SelectionSets) {
		return fmt.Errorf("selection set reference %d is out of bounds", selectionSetRef)
	}
	leaveSelection, err := traversal.enterSelectionWalk(selectionSetRef)
	if err != nil {
		return err
	}
	defer leaveSelection()

	for _, selectionRef := range operation.SelectionSets[selectionSetRef].SelectionRefs {
		if err := checkedReference(selectionRef, len(operation.Selections), "selection reference"); err != nil {
			return err
		}
		selection := operation.Selections[selectionRef]
		switch selection.Kind {
		case ast.SelectionKindField:
			_, responseName, err := checkedOperationFieldNames(operation, selection.Ref)
			if err != nil {
				return err
			}
			include, selectionConditional, err := selectionCondition(operation, operation.Fields[selection.Ref].Directives.Refs)
			if err != nil {
				return fmt.Errorf("field %q: %w", responseName, err)
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
			if err := checkedReference(selection.Ref, len(operation.InlineFragments), "inline fragment reference"); err != nil {
				return err
			}
			inlineFragment := operation.InlineFragments[selection.Ref]
			var typeConditionName string
			var err error
			if inlineFragment.TypeCondition.Type != ast.InvalidRef {
				typeConditionName, err = checkedOperationTypeName(operation, inlineFragment.TypeCondition.Type, "inline fragment type")
				if err != nil {
					return err
				}
			}
			include, selectionConditional, err := selectionCondition(operation, inlineFragment.Directives.Refs)
			if err != nil {
				return fmt.Errorf("inline fragment on %q: %w", typeConditionName, err)
			}
			if !include {
				continue
			}
			childTypeConditions := typeConditions
			if inlineFragment.TypeCondition.Type != ast.InvalidRef {
				childTypeConditions = appendTypeCondition(typeConditions, typeConditionName)
			}
			if inlineFragment.HasSelections {
				if err := collectSelectedFields(operation, inlineFragment.SelectionSet, activeFragments, conditional || selectionConditional, childTypeConditions, fields, traversal); err != nil {
					return err
				}
			}
		case ast.SelectionKindFragmentSpread:
			if err := checkedReference(selection.Ref, len(operation.FragmentSpreads), "fragment spread reference"); err != nil {
				return err
			}
			fragmentSpread := operation.FragmentSpreads[selection.Ref]
			fragmentName, err := checkedBytes(operation, fragmentSpread.FragmentName, "fragment spread name")
			if err != nil {
				return err
			}
			include, selectionConditional, err := selectionCondition(operation, fragmentSpread.Directives.Refs)
			if err != nil {
				return fmt.Errorf("fragment spread %q: %w", fragmentName, err)
			}
			if !include {
				continue
			}
			fragmentDefinitionRef, ok, err := checkedFragmentDefinitionRef(operation, fragmentName)
			if err != nil {
				return err
			}
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
			fragmentTypeName, err := checkedOperationTypeName(operation, fragmentDefinition.TypeCondition.Type, "fragment definition type")
			if err != nil {
				return err
			}
			childTypeConditions := appendTypeCondition(typeConditions, fragmentTypeName)
			activeFragments[fragmentDefinitionRef] = struct{}{}
			if err := collectSelectedFields(operation, fragmentDefinition.SelectionSet, activeFragments, conditional || selectionConditional, childTypeConditions, fields, traversal); err != nil {
				return err
			}
			delete(activeFragments, fragmentDefinitionRef)
		default:
			return fmt.Errorf("selection reference %d has unsupported kind %q", selectionRef, selection.Kind)
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
		if err := checkedReference(directiveRef, len(operation.Directives), "directive reference"); err != nil {
			return false, false, err
		}
		directive := operation.Directives[directiveRef]
		directiveName, err := checkedBytes(operation, directive.Name, "directive name")
		if err != nil {
			return false, false, err
		}
		if !bytes.Equal(directiveName, literal.INCLUDE) && !bytes.Equal(directiveName, literal.SKIP) {
			continue
		}

		condition, ok, err := checkedDirectiveCondition(operation, directive.Arguments.Refs)
		if err != nil {
			return false, false, err
		}
		if !ok {
			return false, false, fmt.Errorf("@%s directive is missing its if condition", directiveName)
		}
		switch condition.Kind {
		case ast.ValueKindBoolean:
			if err := checkedReference(condition.Ref, len(operation.BooleanValues), "boolean value reference"); err != nil {
				return false, false, err
			}
			conditionValue := bool(operation.BooleanValues[condition.Ref])
			if bytes.Equal(directiveName, literal.INCLUDE) && !conditionValue ||
				bytes.Equal(directiveName, literal.SKIP) && conditionValue {
				include = false
			}
		case ast.ValueKindVariable:
			if err := checkedReference(condition.Ref, len(operation.VariableValues), "variable value reference"); err != nil {
				return false, false, err
			}
			if _, err := checkedBytes(operation, operation.VariableValues[condition.Ref].Name, "variable name"); err != nil {
				return false, false, err
			}
			conditional = true
		default:
			return false, false, fmt.Errorf("@%s directive has invalid if condition kind %q", directiveName, condition.Kind)
		}
	}
	return include, conditional, nil
}

func checkedFragmentDefinitionRef(operation *ast.Document, fragmentName []byte) (int, bool, error) {
	for fragmentDefinitionRef := range operation.FragmentDefinitions {
		name, err := checkedBytes(operation, operation.FragmentDefinitions[fragmentDefinitionRef].Name, "fragment definition name")
		if err != nil {
			return ast.InvalidRef, false, err
		}
		if bytes.Equal(fragmentName, name) {
			return fragmentDefinitionRef, true, nil
		}
	}
	return ast.InvalidRef, false, nil
}

func checkedDirectiveCondition(operation *ast.Document, argumentRefs []int) (ast.Value, bool, error) {
	for _, argumentRef := range argumentRefs {
		if err := checkedReference(argumentRef, len(operation.Arguments), "argument reference"); err != nil {
			return ast.Value{}, false, err
		}
		argument := operation.Arguments[argumentRef]
		name, err := checkedBytes(operation, argument.Name, "argument name")
		if err != nil {
			return ast.Value{}, false, err
		}
		if bytes.Equal(name, literal.IF) {
			return argument.Value, true, nil
		}
	}
	return ast.Value{}, false, nil
}
