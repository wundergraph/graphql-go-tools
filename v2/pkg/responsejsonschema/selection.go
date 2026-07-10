package responsejsonschema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
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
			return fieldGroup.fieldRefs, nil
		}

		selectionSetRefs = selectionSetRefs[:0]
		for _, fieldRef := range fieldGroup.fieldRefs {
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

func fieldDefinitionByResponsePath(operation, definition *ast.Document, operationDefinition *ast.OperationDefinition, fieldPath []string) (int, error) {
	rootTypeName, err := rootTypeName(definition, operationDefinition.OperationType)
	if err != nil {
		return ast.InvalidRef, err
	}

	parentNode, ok := definition.Index.FirstNodeByNameStr(rootTypeName)
	if !ok || parentNode.Kind != ast.NodeKindObjectTypeDefinition {
		return ast.InvalidRef, fmt.Errorf("root object type %q is not defined", rootTypeName)
	}

	selectionSetRefs := []int{operationDefinition.SelectionSet}
	for index, responseName := range fieldPath {
		fieldGroup, ok, err := selectedFieldGroupByResponseName(operation, selectionSetRefs, responseName)
		if err != nil {
			return ast.InvalidRef, err
		}
		if !ok {
			return ast.InvalidRef, fmt.Errorf(
				"response field %q not found at path %q",
				responseName,
				strings.Join(fieldPath[:index+1], "."),
			)
		}
		fieldRef := fieldGroup.fieldRefs[0]
		for _, repeatedFieldRef := range fieldGroup.fieldRefs[1:] {
			if operation.FieldNameString(repeatedFieldRef) != operation.FieldNameString(fieldRef) {
				return ast.InvalidRef, fmt.Errorf(
					"response field %q combines incompatible fields %q and %q",
					responseName,
					operation.FieldNameString(fieldRef),
					operation.FieldNameString(repeatedFieldRef),
				)
			}
		}

		fieldDefinitionRef, ok := definition.ObjectTypeDefinitionFieldWithName(parentNode.Ref, operation.FieldNameBytes(fieldRef))
		if !ok {
			return ast.InvalidRef, fmt.Errorf(
				"field %q is not defined on object type %q",
				operation.FieldNameString(fieldRef),
				definition.NodeNameString(parentNode),
			)
		}

		if index == len(fieldPath)-1 {
			return fieldDefinitionRef, nil
		}

		selectionSetRefs = selectionSetRefs[:0]
		for _, repeatedFieldRef := range fieldGroup.fieldRefs {
			field := operation.Fields[repeatedFieldRef]
			if !field.HasSelections {
				return ast.InvalidRef, fmt.Errorf(
					"response field %q has no selection set while resolving path %q",
					responseName,
					strings.Join(fieldPath, "."),
				)
			}
			selectionSetRefs = append(selectionSetRefs, field.SelectionSet)
		}

		parentTypeName := definition.ResolveTypeNameString(definition.FieldDefinitions[fieldDefinitionRef].Type)
		parentNode, ok = definition.Index.FirstNodeByNameStr(parentTypeName)
		if !ok || parentNode.Kind != ast.NodeKindObjectTypeDefinition {
			return ast.InvalidRef, fmt.Errorf("object type %q is not defined", parentTypeName)
		}
	}

	return ast.InvalidRef, fmt.Errorf("response field path %q could not be resolved", strings.Join(fieldPath, "."))
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
	fieldRefs    []int
}

func selectedFieldGroups(operation *ast.Document, selectionSetRefs []int) ([]selectedFieldGroup, error) {
	groupsByResponseName := make(map[string][]int)
	for _, selectionSetRef := range selectionSetRefs {
		var fieldRefs []int
		if err := collectSelectedFieldRefs(operation, selectionSetRef, make(map[int]struct{}), &fieldRefs); err != nil {
			return nil, err
		}
		for _, fieldRef := range fieldRefs {
			responseName := operation.FieldAliasOrNameString(fieldRef)
			groupsByResponseName[responseName] = append(groupsByResponseName[responseName], fieldRef)
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
			fieldRefs:    groupsByResponseName[responseName],
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

func collectSelectedFieldRefs(operation *ast.Document, selectionSetRef int, activeFragments map[int]struct{}, fieldRefs *[]int) error {
	if selectionSetRef < 0 || selectionSetRef >= len(operation.SelectionSets) {
		return fmt.Errorf("selection set reference %d is out of bounds", selectionSetRef)
	}

	for _, selectionRef := range operation.SelectionSets[selectionSetRef].SelectionRefs {
		selection := operation.Selections[selectionRef]
		switch selection.Kind {
		case ast.SelectionKindField:
			*fieldRefs = append(*fieldRefs, selection.Ref)
		case ast.SelectionKindInlineFragment:
			inlineFragment := operation.InlineFragments[selection.Ref]
			if inlineFragment.HasSelections {
				if err := collectSelectedFieldRefs(operation, inlineFragment.SelectionSet, activeFragments, fieldRefs); err != nil {
					return err
				}
			}
		case ast.SelectionKindFragmentSpread:
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
			activeFragments[fragmentDefinitionRef] = struct{}{}
			if err := collectSelectedFieldRefs(operation, fragmentDefinition.SelectionSet, activeFragments, fieldRefs); err != nil {
				return err
			}
			delete(activeFragments, fragmentDefinitionRef)
		}
	}
	return nil
}
