package plan

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type PlannedContextArgument struct {
	ContextArgument

	Sources []resolve.ContextSource
}

// SameInput reports whether one upstream variable can supply both arguments.
func (a PlannedContextArgument) SameInput(b PlannedContextArgument) bool {
	return a.ArgumentType == b.ArgumentType && slices.EqualFunc(a.Sources, b.Sources, resolve.ContextSource.Equal)
}

type plannedContext struct {
	PlannedContextArgument

	fieldRef int
}

type contextScope struct {
	typeName string
	depth    int
}

func (c *nodeSelectionVisitor) contextDepth() int {
	depth := 0
	for _, n := range c.walker.Ancestors {
		if n.Kind == ast.NodeKindField {
			depth++
		}
	}
	return depth
}

// Context selections are single field paths, optionally dispatched by top-level object fragments.
func contextFieldForType(definition, doc *ast.Document, set int, typeName string) int {
	for _, s := range doc.SelectionSets[set].SelectionRefs {
		node := doc.Selections[s]
		switch node.Kind {
		case ast.SelectionKindInlineFragment:
			// Normalized root projections place the concrete override before
			// the original abstract provider's fallback.
			condition := doc.InlineFragmentTypeConditionNameString(node.Ref)
			parent, _ := definition.NodeByNameStr(typeName)
			fragment, _ := definition.NodeByNameStr(condition)
			if condition == typeName || (parent.Kind == ast.NodeKindObjectTypeDefinition && definition.NodeFragmentIsAllowedOnNode(fragment, parent)) {
				return contextFieldForType(definition, doc, doc.InlineFragments[node.Ref].SelectionSet, typeName)
			}
		case ast.SelectionKindField:
			return node.Ref
		}
	}
	return ast.InvalidRef
}

func contextFieldsForType(definition, doc *ast.Document, set int, typeName string) (string, error) {
	ref := contextFieldForType(definition, doc, set, typeName)
	if ref == ast.InvalidRef {
		return "", nil
	}
	var selected strings.Builder
	selected.WriteString(doc.FieldNameString(ref))
	if doc.FieldHasArguments(ref) {
		if err := doc.PrintArguments(doc.FieldArguments(ref), &selected); err != nil {
			return "", err
		}
	}
	if doc.Fields[ref].HasSelections {
		child, err := contextFieldsForType(definition, doc, doc.Fields[ref].SelectionSet, "")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&selected, " { %s }", child)
	}
	return selected.String(), nil
}

// A mutation must only execute the client's write. A subscription has one event
// stream. In both cases, project context from the active root field's payload.
func rootContextUsesActiveField(definition *ast.Document, root string) bool {
	return root == string(definition.Index.MutationTypeName) || root == string(definition.Index.SubscriptionTypeName)
}

// Composition omits arguments equivalent to their coerced defaults on operation
// roots. Also match literal schema defaults in previously generated metadata.
func rootFieldCanProject(schema *ast.Document, root string, op *ast.Document, trigger int, projection *ast.Document, field int) bool {
	if field == ast.InvalidRef || projection.FieldNameString(field) == "__typename" {
		return true
	}
	name := op.FieldNameBytes(trigger)
	if !bytes.Equal(name, projection.FieldNameBytes(field)) {
		return false
	}
	if !op.FieldHasArguments(trigger) && !projection.FieldHasArguments(field) {
		return true
	}
	rootType, ok := schema.NodeByNameStr(root)
	if !ok {
		return false
	}
	// Check both sets. Unknown arguments must not silently match an omitted one.
	matches := func(left *ast.Document, leftField int, right *ast.Document, rightField int) bool {
		for _, arg := range left.FieldArguments(leftField) {
			argName := left.ArgumentNameBytes(arg)
			doc := right
			other, exists := right.FieldArgument(rightField, argName)
			var value ast.Value
			if exists {
				value = right.ArgumentValue(other)
			} else {
				def := schema.NodeFieldDefinitionArgumentDefinitionByName(rootType, name, argName)
				if def == ast.InvalidRef || !schema.InputValueDefinitionHasDefaultValue(def) {
					return false
				}
				doc, value = schema, schema.InputValueDefinitionDefaultValue(def)
			}
			a, errA := left.PrintValueBytes(left.ArgumentValue(arg), nil)
			b, errB := doc.PrintValueBytes(value, nil)
			if errA != nil || errB != nil || !bytes.Equal(a, b) {
				return false
			}
		}
		return true
	}
	return matches(op, trigger, projection, field) && matches(projection, field, op, trigger)
}

func (c *nodeSelectionVisitor) addContextSource(ref int, scope contextScope, matches []string, fragment *ast.Document, field fieldRequirementsContext) (source resolve.ContextSource, result AddRequiredFieldsResult, err error) {
	source.Depth = scope.depth
	config := addRequiredFieldsConfiguration{
		operation: c.operation, definition: c.definition, operationSelectionSetRef: ref,
		typeName: scope.typeName, allowTypename: true, deferInfo: field.deferInfo, parentFieldDeferID: field.parentFieldDeferID,
	}
	set := fragment.FragmentDefinitions[0].SelectionSet
	var triggerName string
	projected := ast.InvalidRef
	if scope.depth == 0 {
		projected = contextFieldForType(c.definition, fragment, set, scope.typeName)
		if projected == ast.InvalidRef {
			return source, result, nil
		}
		if fragment.FieldNameString(projected) == "__typename" {
			source.RootTypeName = scope.typeName
			return source, result, nil
		}
	}
	if scope.depth == 0 && rootContextUsesActiveField(c.definition, scope.typeName) {
		// Attach payload prerequisites to the existing trigger, including a trigger under a client alias or fragment.
		trigger := ast.InvalidRef
		for _, ancestor := range c.walker.Ancestors {
			if ancestor.Kind == ast.NodeKindField {
				trigger = ancestor.Ref
				break
			}
		}
		if trigger == ast.InvalidRef || !rootFieldCanProject(c.definition, scope.typeName, c.operation, trigger, fragment, projected) {
			return source, result, fmt.Errorf("context on %s cannot select another root field or different arguments", scope.typeName)
		}
		rootType, _ := c.definition.NodeByNameStr(scope.typeName)
		payloadType, ok := c.definition.FieldTypeNode(c.operation.FieldNameBytes(trigger), rootType)
		if !ok || !fragment.Fields[projected].HasSelections {
			return source, result, fmt.Errorf("context on %s requires a selection on the root field payload", scope.typeName)
		}
		config.operationSelectionSetRef = c.operation.Fields[trigger].SelectionSet
		config.typeName = payloadType.NameString(c.definition)
		config.allowTypename = false
		triggerName = c.operation.FieldAliasOrNameString(trigger)
		set = fragment.Fields[projected].SelectionSet
		matches = []string{config.typeName}
	}

	// A normalized interface fallback applies directly to that interface below
	// the operation root. Expanding it into every implementation can schedule
	// fields on types that the subgraph returning the interface cannot produce.
	scopeType, _ := c.definition.NodeByNameStr(scope.typeName)
	if scopeType.Kind == ast.NodeKindInterfaceTypeDefinition {
		for _, ref := range fragment.SelectionSets[set].SelectionRefs {
			s := fragment.Selections[ref]
			if s.Kind == ast.SelectionKindInlineFragment && fragment.InlineFragmentTypeConditionNameString(s.Ref) == scope.typeName {
				matches = []string{scope.typeName}
				break
			}
		}
	}

	var fields strings.Builder
	for _, name := range matches {
		path, err := contextFieldsForType(c.definition, fragment, set, name)
		if err != nil {
			return source, result, err
		}
		if path == "" {
			continue
		}
		if name == config.typeName {
			fields.WriteString(path)
		} else {
			fmt.Fprintf(&fields, " ... on %s { %s }", name, path)
		}
	}
	config.fieldSet = fields.String()
	result, report := c.addRequiredFields(&config)
	if report.HasErrors() {
		return source, result, report
	}
	if config.fieldSet == "" {
		return source, result, nil
	}
	// Only include the selected value in the binding. The added __typename is for dispatch.
	selected, report := RequiredFieldsFragment(config.typeName, config.fieldSet, false)
	if report.HasErrors() {
		return source, result, report
	}
	source.Selection = c.contextSelection(selected, selected.FragmentDefinitions[0].SelectionSet, config.typeName, result.remappedPaths)
	if triggerName != "" {
		source.Selection = []resolve.ContextSelection{{Field: triggerName, Children: source.Selection}}
	}
	return source, result, nil
}

func (c *nodeSelectionVisitor) handleContextArguments(field fieldRequirementsContext, key fieldIndexKey) {
	metadata := field.dsConfig.FederationConfiguration()
	for _, arg := range metadata.ContextArguments {
		if arg.TypeName != field.typeName || arg.FieldName != field.fieldName {
			continue
		}
		binding := &plannedContext{fieldRef: field.fieldRef, PlannedContextArgument: PlannedContextArgument{ContextArgument: arg}}
		set := strings.TrimSpace(arg.ContextSelection)
		if len(set) < 2 || set[0] != '{' || set[len(set)-1] != '}' {
			c.walker.StopWithInternalErr(fmt.Errorf("invalid selection for context %s", arg.ContextName))
			return
		}
		fragment, report := RequiredFieldsFragment(field.typeName, set[1:len(set)-1], false)
		if report.HasErrors() {
			c.walker.StopWithInternalErr(report)
			return
		}
		receiverDepth := c.contextScopes[c.selectionSetRefs[len(c.selectionSetRefs)-1]].depth
		for i := len(c.selectionSetRefs) - 2; i >= 0; i-- {
			ref := c.selectionSetRefs[i]
			scope := c.contextScopes[ref]
			// A fragment on the receiving object is still at the receiver depth.
			if scope.depth >= receiverDepth {
				continue
			}
			matches, all := metadata.providerTypes(c.definition, arg.ContextName, scope.typeName)
			if len(matches) == 0 {
				continue
			}
			source, result, err := c.addContextSource(ref, scope, matches, fragment, field)
			if err != nil {
				c.walker.StopWithInternalErr(err)
				return
			}
			c.addSkipFieldRefs(result.skipFieldRefs...)
			c.resetVisitedAbstractChecksForModifiedFields(result.modifiedFieldRefs)
			for _, dep := range result.requiredFieldRefs {
				c.addFieldDependency(key, dep)
				depKey := fieldDependencyKey{field: field.fieldRef, dependsOn: dep}
				if _, exists := c.fieldDependencyKind[depKey]; !exists {
					c.fieldDependencyKind[depKey] = fieldDependencyKindContext
				}
			}
			if !all {
				source.Types = matches
			}
			binding.Sources = append(binding.Sources, source)
			c.hasNewFields = true
			if all {
				break
			}
		}
		if c.contextFields == nil {
			c.contextFields = make(map[fieldIndexKey]struct{})
		}
		c.contextFields[key] = struct{}{}
		c.fieldRequirementsConfigs[key] = append(c.fieldRequirementsConfigs[key], FederationFieldConfiguration{TypeName: field.typeName, FieldName: field.fieldName, context: binding})
		c.handleKeyRequirementsForBackJumpOnSameDataSource(field)
	}
}

func (c *nodeSelectionVisitor) contextSelection(doc *ast.Document, ref int, prefix string, aliases map[string]string) (out []resolve.ContextSelection) {
	for _, s := range doc.SelectionSets[ref].SelectionRefs {
		node := doc.Selections[s]
		var step resolve.ContextSelection
		switch node.Kind {
		case ast.SelectionKindInlineFragment:
			condition := doc.InlineFragmentTypeConditionNameString(node.Ref)
			t, _ := c.definition.NodeByNameStr(condition)
			switch t.Kind {
			case ast.NodeKindInterfaceTypeDefinition:
				step.Types, _ = c.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(t.Ref)
			case ast.NodeKindUnionTypeDefinition:
				step.Types, _ = c.definition.UnionTypeDefinitionMemberTypeNames(t.Ref)
			default:
				step.Types = []string{condition}
			}
			step.Children = c.contextSelection(doc, doc.InlineFragments[node.Ref].SelectionSet, prefix+"."+ast.InlineFragmentPathPrefix+strconv.Itoa(node.Ref)+condition, aliases)
		case ast.SelectionKindField:
			name := doc.FieldNameString(node.Ref)
			path := prefix + "." + name
			step.Field = name
			if alias := aliases[path]; alias != "" {
				step.Field = alias
			}
			if doc.Fields[node.Ref].HasSelections {
				step.Children = c.contextSelection(doc, doc.Fields[node.Ref].SelectionSet, path, aliases)
			}
		}
		out = append(out, step)
	}
	return
}
