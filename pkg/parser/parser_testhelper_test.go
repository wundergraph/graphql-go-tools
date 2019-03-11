package parser

import (
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"reflect"
	"testing"
)

type rule func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int)
type ruleSet []rule

func (r ruleSet) eval(node document.Node, parser *Parser, ruleIndex int) {
	for i, rule := range r {
		rule(node, parser, ruleIndex, i)
	}
}

type checkFunc func(parser *Parser, i int)

func run(input string, checks ...checkFunc) {
	parser := NewParser()
	if err := parser.l.SetTypeSystemInput([]byte(input)); err != nil {
		panic(err)
	}
	for i, checkFunc := range checks {
		checkFunc(parser, i)
	}
}

func node(rules ...rule) ruleSet {
	return rules
}

func nodes(sets ...ruleSet) []ruleSet {
	return sets
}

func evalRules(node document.Node, parser *Parser, rules ruleSet, ruleIndex int) {
	rules.eval(node, parser, ruleIndex)
}

func hasName(wantName string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		var gotName string
		if node.NodeName() != -1 {
			gotName = string(parser.CachedByteSlice(node.NodeName()))
		}
		if wantName != gotName {
			panic(fmt.Errorf("hasName: want: %s, got: %s [rule: %d, node: %d]", wantName, gotName, ruleIndex, ruleSetIndex))
		}
	}
}

func hasSchemaOperationTypeName(operationType document.OperationType, wantTypeName string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		schemaDefinition := node.(document.SchemaDefinition)

		gotQuery := string(parser.CachedByteSlice(schemaDefinition.Query))
		gotMutation := string(parser.CachedByteSlice(schemaDefinition.Mutation))
		gotSubscription := string(parser.CachedByteSlice(schemaDefinition.Subscription))

		if operationType == document.OperationTypeQuery && wantTypeName != gotQuery {
			panic(fmt.Errorf("hasOperationTypeName: want(query): %s, got: %s [check: %d]", wantTypeName, gotQuery, ruleIndex))
		}
		if operationType == document.OperationTypeMutation && wantTypeName != gotMutation {
			panic(fmt.Errorf("hasOperationTypeName: want(mutation): %s, got: %s [check: %d]", wantTypeName, gotMutation, ruleIndex))
		}
		if operationType == document.OperationTypeSubscription && wantTypeName != gotSubscription {
			panic(fmt.Errorf("hasOperationTypeName: want(subscription): %s, got: %s [check: %d]", wantTypeName, gotSubscription, ruleIndex))
		}
	}
}

func hasPosition(position position.Position) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		gotPosition := node.NodePosition()
		if !reflect.DeepEqual(position, gotPosition) {
			panic(fmt.Errorf("hasPosition: want: %+v, got: %+v [rule: %d, node: %d]", position, gotPosition, ruleIndex, ruleSetIndex))
		}
	}
}

func hasAlias(wantAlias string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		var gotAlias string
		if node.NodeAlias() != -1 {
			gotAlias = string(parser.CachedByteSlice(node.NodeAlias()))
		}
		if wantAlias != gotAlias {
			panic(fmt.Errorf("hasAlias: want: %s, got: %s [rule: %d, node: %d]", wantAlias, gotAlias, ruleIndex, ruleSetIndex))
		}
	}
}

func hasDescription(wantDescription string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		gotDescription := string(parser.ByteSlice(node.NodeDescription()))
		if wantDescription != gotDescription {
			panic(fmt.Errorf("hasName: want: %s, got: %s [rule: %d, node: %d]", wantDescription, gotDescription, ruleIndex, ruleSetIndex))
		}
	}
}

func hasDirectiveLocations(locations ...document.DirectiveLocation) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		got := node.(document.DirectiveDefinition).DirectiveLocations

		for k, wantLocation := range locations {
			gotLocation := got[k]
			if wantLocation != gotLocation {
				panic(fmt.Errorf("mustParseDirectiveDefinition: want(location: %d): %s, got: %s", k, wantLocation.String(), gotLocation.String()))
			}
		}
	}
}

func unwrapObjectField(node document.Node, parser *Parser) document.Node {
	objectField, ok := node.(document.ObjectField)
	if ok {
		node = parser.ParsedDefinitions.Values[objectField.Value]
	}
	return node
}

func expectIntegerValue(want int32) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		node = unwrapObjectField(node, parser)
		got := parser.ParsedDefinitions.Integers[node.NodeValueReference()]
		if want != got {
			panic(fmt.Errorf("expectIntegerValue: want: %d, got: %d [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
		}
	}
}

func expectFloatValue(want float32) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		node = unwrapObjectField(node, parser)
		got := parser.ParsedDefinitions.Floats[node.NodeValueReference()]
		if want != got {
			panic(fmt.Errorf("expectIntegerValue: want: %.2f, got: %.2f [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
		}
	}
}

func expectByteSliceRef(want document.ByteSliceReference) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		node = unwrapObjectField(node, parser)
		got := node.(document.Value).Raw
		if want != got {
			panic(fmt.Errorf("expectIntegerValue: want: %+v, got: %+v [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
		}
	}
}

func expectBooleanValue(want bool) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		node = unwrapObjectField(node, parser)
		got := parser.ParsedDefinitions.Booleans[node.NodeValueReference()]
		if want != got {
			panic(fmt.Errorf("expectIntegerValue: want: %v, got: %v [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
		}
	}
}

func expectByteSliceValue(want string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		node = unwrapObjectField(node, parser)
		got := string(parser.CachedByteSlice(node.NodeValueReference()))
		if want != got {
			panic(fmt.Errorf("expectByteSliceValue: want: %s, got: %s [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
		}
	}
}

func expectListValue(rules ...rule) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		list := parser.ParsedDefinitions.ListValues[node.NodeValueReference()]
		for j, rule := range rules {
			valueIndex := list[j]
			value := parser.ParsedDefinitions.Values[valueIndex]
			rule(value, parser, j, ruleSetIndex)
		}
	}
}

func expectObjectValue(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		node = unwrapObjectField(node, parser)
		list := parser.ParsedDefinitions.ObjectValues[node.NodeValueReference()]
		for j, rule := range rules {
			valueIndex := list[j]
			value := parser.ParsedDefinitions.ObjectFields[valueIndex]
			rule.eval(value, parser, j)
		}
	}
}

func hasOperationType(operationType document.OperationType) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		gotOperationType := node.NodeOperationType().String()
		wantOperationType := operationType.String()
		if wantOperationType != gotOperationType {
			panic(fmt.Errorf("hasOperationType: want: %s, got: %s [rule: %d, node: %d]", wantOperationType, gotOperationType, ruleIndex, ruleSetIndex))
		}
	}
}

func hasTypeKind(wantTypeKind document.TypeKind) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		gotTypeKind := node.(document.Type).Kind
		if wantTypeKind != gotTypeKind {
			panic(fmt.Errorf("hasTypeKind: want(typeKind): %s, got: %s [rule: %d, node: %d]", wantTypeKind, gotTypeKind, ruleIndex, ruleSetIndex))
		}
	}
}

func nodeType(rules ...rule) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		nodeType := parser.ParsedDefinitions.Types[node.NodeType()]
		for j, rule := range rules {
			rule(nodeType, parser, j, ruleSetIndex)
		}
	}
}

func ofType(rules ...rule) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		ofType := parser.ParsedDefinitions.Types[node.(document.Type).OfType]
		for j, rule := range rules {
			rule(ofType, parser, j, ruleSetIndex)
		}
	}
}

func hasTypeName(wantName string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		if fragment, ok := node.(document.FragmentDefinition); ok {
			node = parser.ParsedDefinitions.Types[fragment.TypeCondition]
		}

		if inlineFragment, ok := node.(document.InlineFragment); ok {
			node = parser.ParsedDefinitions.Types[inlineFragment.TypeCondition]
		}

		gotName := string(parser.CachedByteSlice(node.(document.Type).Name))
		if wantName != gotName {
			panic(fmt.Errorf("hasTypeName: want: %s, got: %s [rule: %d, node: %d]", wantName, gotName, ruleIndex, ruleSetIndex))
		}
	}
}

func hasDefaultValue(rules ...rule) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		index := node.NodeDefaultValue()
		node = parser.ParsedDefinitions.Values[index]
		for k, rule := range rules {
			rule(node, parser, k, ruleSetIndex)
		}
	}
}

func hasValueType(valueType document.ValueType) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		if node.NodeValueType() != valueType {
			panic(fmt.Errorf("hasValueType: want: %s, got: %s [check: %d]", valueType.String(), node.NodeValueType().String(), ruleIndex))
		}
	}
}

func hasByteSliceValue(want string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		got := string(parser.CachedByteSlice(node.NodeValueReference()))
		if want != got {
			panic(fmt.Errorf("hasByteSliceValue: want: %s, got: %s [check: %d]", want, got, ruleIndex))
		}
	}
}

func hasEnumValuesDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		index := node.NodeEnumValuesDefinition()

		for j, k := range index {
			ruleSet := rules[j]
			subNode := parser.ParsedDefinitions.EnumValuesDefinitions[k]
			ruleSet.eval(subNode, parser, k)
		}
	}
}

func hasUnionTypeSystemDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		typeDefinitionIndex := node.NodeUnionTypeDefinitions()

		for j, ruleSet := range rules {
			definitionIndex := typeDefinitionIndex[j]
			subNode := parser.ParsedDefinitions.UnionTypeDefinitions[definitionIndex]
			ruleSet.eval(subNode, parser, j)
		}
	}
}

func hasScalarTypeSystemDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		typeDefinitionIndex := node.NodeScalarTypeDefinitions()

		for j, ruleSet := range rules {
			definitionIndex := typeDefinitionIndex[j]
			subNode := parser.ParsedDefinitions.ScalarTypeDefinitions[definitionIndex]
			ruleSet.eval(subNode, parser, j)
		}
	}
}

func hasObjectTypeSystemDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		typeDefinitionIndex := node.NodeObjectTypeDefinitions()

		for j, ruleSet := range rules {
			definitionIndex := typeDefinitionIndex[j]
			subNode := parser.ParsedDefinitions.ObjectTypeDefinitions[definitionIndex]
			ruleSet.eval(subNode, parser, j)
		}
	}
}

func hasInterfaceTypeSystemDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		typeDefinitionIndex := node.NodeInterfaceTypeDefinitions()

		for j, ruleSet := range rules {
			definitionIndex := typeDefinitionIndex[j]
			subNode := parser.ParsedDefinitions.InterfaceTypeDefinitions[definitionIndex]
			ruleSet.eval(subNode, parser, j)
		}
	}
}

func hasEnumTypeSystemDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		typeDefinitionIndex := node.NodeEnumTypeDefinitions()

		for j, ruleSet := range rules {
			definitionIndex := typeDefinitionIndex[j]
			subNode := parser.ParsedDefinitions.EnumTypeDefinitions[definitionIndex]
			ruleSet.eval(subNode, parser, j)
		}
	}
}

func hasInputObjectTypeSystemDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		typeDefinitionIndex := node.NodeInputObjectTypeDefinitions()

		for j, ruleSet := range rules {
			definitionIndex := typeDefinitionIndex[j]
			subNode := parser.ParsedDefinitions.InputObjectTypeDefinitions[definitionIndex]
			ruleSet.eval(subNode, parser, j)
		}
	}
}

func hasDirectiveDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		typeDefinitionIndex := node.NodeDirectiveDefinitions()

		for j, ruleSet := range rules {
			definitionIndex := typeDefinitionIndex[j]
			subNode := parser.ParsedDefinitions.DirectiveDefinitions[definitionIndex]
			ruleSet.eval(subNode, parser, j)
		}
	}
}

func hasUnionMemberTypes(members ...string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		typeDefinitionIndex := node.NodeUnionMemberTypes()

		for j, want := range members {
			got := string(parser.CachedByteSlice(typeDefinitionIndex[j]))
			if want != got {
				panic(fmt.Errorf("hasUnionMemberTypes: want: %s, got: %s [check: %d]", want, got, ruleSetIndex))
			}
		}
	}
}

func hasSchemaDefinition(rules ...rule) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		schemaDefinition := node.NodeSchemaDefinition()
		if !schemaDefinition.IsDefined() {
			panic(fmt.Errorf("hasSchemaDefinition: schemaDefinition is undefined [check: %d]", ruleSetIndex))
		}

		for i, rule := range rules {
			rule(schemaDefinition, parser, i, ruleSetIndex)
		}
	}
}

func hasVariableDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		index := node.NodeVariableDefinitions()

		for j, k := range index {
			ruleSet := rules[j]
			subNode := parser.ParsedDefinitions.VariableDefinitions[k]
			ruleSet.eval(subNode, parser, k)
		}
	}
}

func hasDirectives(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		set := node.NodeDirectiveSet()
		index := parser.ParsedDefinitions.DirectiveSets[set]

		for i := range rules {
			ruleSet := rules[i]
			subNode := parser.ParsedDefinitions.Directives[index[i]]
			ruleSet.eval(subNode, parser, index[i])
		}
	}
}

func hasImplementsInterfaces(interfaces ...string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		actual := node.NodeImplementsInterfaces()
		for i, want := range interfaces {
			got := string(parser.CachedByteSlice(actual[i]))

			if want != got {
				panic(fmt.Errorf("hasImplementsInterfaces: want(at: %d): %s, got: %s [check: %d]", i, want, got, ruleSetIndex))
			}
		}
	}
}

func hasFields(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		if _, ok := node.(document.SelectionSet); !ok {
			node = parser.ParsedDefinitions.SelectionSets[node.NodeSelectionSet()]
		}
		index := node.NodeFields()

		for i := range rules {
			ruleSet := rules[i]
			subNode := parser.ParsedDefinitions.Fields[index[i]]
			ruleSet.eval(subNode, parser, index[i])
		}
	}
}

func hasFieldsDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		index := node.NodeFieldsDefinition()

		for i := range rules {
			ruleSet := rules[i]
			field := parser.ParsedDefinitions.FieldDefinitions[index[i]]
			ruleSet.eval(field, parser, index[i])
		}
	}
}

func hasInputFieldsDefinition(rules ...rule) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		index := node.NodeInputFieldsDefinition()
		node = parser.ParsedDefinitions.InputFieldsDefinitions[index]

		for i, rule := range rules {
			rule(node, parser, i, ruleSetIndex)
		}
	}
}

func hasInputValueDefinitions(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		index := node.NodeInputValueDefinitions()

		for i := range rules {
			ruleSet := rules[i]
			subNode := parser.ParsedDefinitions.InputValueDefinitions[index[i]]
			ruleSet.eval(subNode, parser, index[i])
		}
	}
}

func hasArguments(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		set := node.NodeArgumentSet()
		index := parser.ParsedDefinitions.ArgumentSets[set]

		for i := range rules {
			ruleSet := rules[i]
			subNode := parser.ParsedDefinitions.Arguments[index[i]]
			ruleSet.eval(subNode, parser, index[i])
		}
	}
}

func hasValue(rules ...rule) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		valueRef := node.NodeValue()
		value := parser.ParsedDefinitions.Values[valueRef]
		for i, rule := range rules {
			rule(value, parser, i, ruleSetIndex)
		}
	}
}

func hasRawValueContent(want string) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
		got := string(parser.ByteSlice(node.(document.Value).Raw))
		if want != got {
			panic(fmt.Errorf("hasRawValueContent: want: %s, got: %s", want, got))
		}
	}
}

func hasArgumentsDefinition(rules ...rule) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		index := node.NodeArgumentsDefinition()
		node = parser.ParsedDefinitions.ArgumentsDefinitions[index]

		for k, rule := range rules {
			rule(node, parser, k, ruleSetIndex)
		}
	}
}

func hasInlineFragments(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		if _, ok := node.(document.SelectionSet); !ok {
			node = parser.ParsedDefinitions.SelectionSets[node.NodeSelectionSet()]
		}
		index := node.NodeInlineFragments()

		for i := range rules {
			ruleSet := rules[i]
			subNode := parser.ParsedDefinitions.InlineFragments[index[i]]
			ruleSet.eval(subNode, parser, index[i])
		}
	}
}

func hasFragmentSpreads(rules ...ruleSet) rule {
	return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

		index := node.NodeFragmentSpreads()

		for i := range rules {
			ruleSet := rules[i]
			subNode := parser.ParsedDefinitions.FragmentSpreads[index[i]]
			ruleSet.eval(subNode, parser, index[i])
		}
	}
}

func mustPanic(c checkFunc) checkFunc {
	return func(parser *Parser, i int) {

		defer func() {
			if recover() == nil {
				panic(fmt.Errorf("mustPanic: panic expected [check: %d]", i))
			}
		}()

		c(parser, i)
	}
}

func mustParseArguments(wantArgumentNodes ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var set int
		if err := parser.parseArgumentSet(&set); err != nil {
			panic(err)
		}

		arguments := parser.ParsedDefinitions.ArgumentSets[set]

		for k, want := range wantArgumentNodes {
			argument := parser.ParsedDefinitions.Arguments[arguments[k]]
			want.eval(argument, parser, k)
		}
	}
}

func mustParseArgumentDefinition(rule ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index int
		if err := parser.parseArgumentsDefinition(&index); err != nil {
			panic(err)
		}

		if len(rule) == 0 {
			return
		} else if len(rule) != 1 {
			panic("must be only 1 node")
		}

		node := parser.ParsedDefinitions.ArgumentsDefinitions[index]
		rule[0].eval(node, parser, i)
	}
}

func mustParseDefaultValue(wantValueType document.ValueType) checkFunc {
	return func(parser *Parser, i int) {
		index, err := parser.parseDefaultValue()
		if err != nil {
			panic(err)
		}

		val := parser.ParsedDefinitions.Values[index]

		if val.ValueType != wantValueType {
			panic(fmt.Errorf("mustParseDefaultValue: want(valueType): %s, got: %s", wantValueType.String(), val.ValueType.String()))
		}
	}
}

func mustParseDirectiveDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseDirectiveDefinition(nil, &index); err != nil {
			panic(err)
		}

		for k, rule := range rules {
			node := parser.ParsedDefinitions.DirectiveDefinitions[index[k]]
			rule.eval(node, parser, k)
		}
	}
}

func mustParseDirectives(directives ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var set int
		if err := parser.parseDirectives(&set); err != nil {
			panic(err)
		}

		index := parser.ParsedDefinitions.DirectiveSets[set]

		for k, rule := range directives {
			node := parser.ParsedDefinitions.Directives[index[k]]
			rule.eval(node, parser, k)
		}
	}
}

func mustParseEnumTypeDefinition(rules ...rule) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseEnumTypeDefinition(nil, &index); err != nil {
			panic(err)
		}

		enum := parser.ParsedDefinitions.EnumTypeDefinitions[0]
		evalRules(enum, parser, rules, i)
	}
}

func mustParseExecutableDefinition(fragments []ruleSet, operations []ruleSet) checkFunc {
	return func(parser *Parser, i int) {

		definition, err := parser.parseExecutableDefinition()
		if err != nil {
			panic(err)
		}

		for i, set := range fragments {
			fragmentIndex := definition.FragmentDefinitions[i]
			fragment := parser.ParsedDefinitions.FragmentDefinitions[fragmentIndex]
			set.eval(fragment, parser, i)
		}

		for i, set := range operations {
			opIndex := definition.OperationDefinitions[i]
			operation := parser.ParsedDefinitions.OperationDefinitions[opIndex]
			set.eval(operation, parser, i)
		}
	}
}

func mustParseFields(rule ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		fieldRef, err := parser.parseField()
		if err != nil {
			panic(err)
		}

		field := parser.ParsedDefinitions.Fields[fieldRef]
		evalRules(field, parser, rule, i)
	}
}

func mustParseFieldsDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseFieldsDefinition(&index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			field := parser.ParsedDefinitions.FieldDefinitions[j]
			evalRules(field, parser, rule, i)
		}
	}
}

func mustParseFragmentDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {

		var index []int
		if err := parser.parseFragmentDefinition(&index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			fragmentDefinition := parser.ParsedDefinitions.FragmentDefinitions[j]
			evalRules(fragmentDefinition, parser, rule, i)
		}
	}
}

func mustParseFragmentSpread(rule ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		fragmentSpreadRef, err := parser.parseFragmentSpread(position.Position{})
		if err != nil {
			panic(err)
		}

		spread := parser.ParsedDefinitions.FragmentSpreads[fragmentSpreadRef]
		evalRules(spread, parser, rule, i)
	}
}

func mustParseImplementsInterfaces(implements ...string) checkFunc {
	return func(parser *Parser, i int) {

		interfaces, err := parser.parseImplementsInterfaces()
		if err != nil {
			panic(err)
		}

		for j, want := range implements {
			got := string(parser.CachedByteSlice(interfaces[j]))
			if want != got {
				panic(fmt.Errorf("mustParseImplementsInterfaces: want: %s, got: %s [check: %d]", want, got, i))
			}
		}
	}
}

func mustParseLiteral(wantKeyword keyword.Keyword, wantLiteral string) checkFunc {
	return func(parser *Parser, i int) {
		next := parser.l.Read()

		gotKeyword := next.Keyword
		gotLiteral := string(parser.ByteSlice(next.Literal))

		if wantKeyword != gotKeyword {
			panic(fmt.Errorf("mustParseLiteral: want(keyword): %s, got: %s, [check: %d]", wantKeyword.String(), gotKeyword.String(), i))
		}

		if wantLiteral != gotLiteral {
			panic(fmt.Errorf("mustParseLiteral: want(literal): %s, got: %s, [check: %d]", wantLiteral, gotLiteral, i))
		}
	}
}

func mustParseInlineFragments(rule ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		inlineFragmentRef, err := parser.parseInlineFragment(position.Position{})
		if err != nil {
			panic(err)
		}

		inlineFragment := parser.ParsedDefinitions.InlineFragments[inlineFragmentRef]
		evalRules(inlineFragment, parser, rule, i)
	}
}

func mustParseInputFieldsDefinition(rules ...rule) checkFunc {
	return func(parser *Parser, i int) {

		var index int
		if err := parser.parseInputFieldsDefinition(&index); err != nil {
			panic(err)
		}

		if len(rules) == 0 {
			return
		}

		node := parser.ParsedDefinitions.InputFieldsDefinitions[index]

		for k, rule := range rules {
			rule(node, parser, k, i)
		}
	}
}

func mustParseInputObjectTypeDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseInputObjectTypeDefinition(nil, &index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			inputObjectDefinition := parser.ParsedDefinitions.InputObjectTypeDefinitions[j]
			evalRules(inputObjectDefinition, parser, rule, i)
		}
	}
}

func mustParseInputValueDefinitions(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseInputValueDefinitions(&index, keyword.UNDEFINED); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			inputValueDefinition := parser.ParsedDefinitions.InputValueDefinitions[j]
			evalRules(inputValueDefinition, parser, rule, i)
		}
	}
}

func mustParseInterfaceTypeDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseInterfaceTypeDefinition(nil, &index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			interfaceTypeDefinition := parser.ParsedDefinitions.InterfaceTypeDefinitions[j]
			evalRules(interfaceTypeDefinition, parser, rule, i)
		}
	}
}

func mustParseObjectTypeDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseObjectTypeDefinition(nil, &index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			objectTypeDefinition := parser.ParsedDefinitions.ObjectTypeDefinitions[j]
			evalRules(objectTypeDefinition, parser, rule, i)
		}
	}
}

func mustParseOperationDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseOperationDefinition(&index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			operationDefinition := parser.ParsedDefinitions.OperationDefinitions[j]
			evalRules(operationDefinition, parser, rule, i)
		}
	}
}

func mustContainOperationDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		for j, rule := range rules {
			operationDefinition := parser.ParsedDefinitions.OperationDefinitions[j]
			evalRules(operationDefinition, parser, rule, i)
		}
	}
}

func mustParseScalarTypeDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseScalarTypeDefinition(nil, &index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			scalarTypeDefinition := parser.ParsedDefinitions.ScalarTypeDefinitions[j]
			evalRules(scalarTypeDefinition, parser, rule, i)
		}
	}
}

func mustParseTypeSystemDefinition(rules ruleSet) checkFunc {
	return func(parser *Parser, i int) {

		definition, err := parser.parseTypeSystemDefinition()
		if err != nil {
			panic(err)
		}

		evalRules(definition, parser, rules, i)
	}
}

func mustParseSchemaDefinition(rules ...rule) checkFunc {
	return func(parser *Parser, i int) {
		typeSystemDefinition := parser.makeTypeSystemDefinition()
		err := parser.parseSchemaDefinition(&typeSystemDefinition.SchemaDefinition)
		if err != nil {
			panic(err)
		}

		for k, rule := range rules {
			rule(typeSystemDefinition.SchemaDefinition, parser, k, i)
		}
	}
}

func mustParseSelectionSet(rules ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var ref int
		if err := parser.parseSelectionSet(&ref); err != nil {
			panic(err)
		}

		selectionSet := parser.ParsedDefinitions.SelectionSets[ref]

		rules.eval(selectionSet, parser, i)
	}
}

func mustParseUnionTypeDefinition(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseUnionTypeDefinition(nil, &index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			scalarTypeDefinition := parser.ParsedDefinitions.UnionTypeDefinitions[j]
			evalRules(scalarTypeDefinition, parser, rule, i)
		}
	}
}

func mustParseVariableDefinitions(rules ...ruleSet) checkFunc {
	return func(parser *Parser, i int) {
		var index []int
		if err := parser.parseVariableDefinitions(&index); err != nil {
			panic(err)
		}

		for j, rule := range rules {
			scalarTypeDefinition := parser.ParsedDefinitions.VariableDefinitions[j]
			evalRules(scalarTypeDefinition, parser, rule, i)
		}
	}
}

func mustParseValue(valueType document.ValueType, rules ...rule) checkFunc {
	return func(parser *Parser, i int) {
		var index int
		var err error
		if index, err = parser.parseValue(); err != nil {
			panic(err)
		}

		value := parser.ParsedDefinitions.Values[index]

		if valueType != value.ValueType {
			panic(fmt.Errorf("mustParseValue: want: %s, got: %s [check: %d]", valueType, value.ValueType, i))
		}

		for _, rule := range rules {
			rule(value, parser, i, i)
		}
	}
}

func mustParseType(rules ...rule) checkFunc {
	return func(parser *Parser, i int) {
		var index int
		if err := parser.parseType(&index); err != nil {
			panic(err)
		}

		node := parser.ParsedDefinitions.Types[index]

		for j, rule := range rules {
			rule(node, parser, j, i)
		}
	}
}

func mustParseFloatValue(t *testing.T, input string, want float32) checkFunc {
	return func(parser *Parser, i int) {

		controller := gomock.NewController(t)
		lexer := NewMockLexer(controller)

		parser.l = lexer

		ref := document.ByteSliceReference{
			Start: 0,
			End:   0,
		}

		lexer.EXPECT().Read().Return(token.Token{
			Literal: ref,
		})

		lexer.EXPECT().ByteSlice(ref).Return([]byte(input))

		var value document.Value
		if err := parser.parsePeekedFloatValue(&value); err != nil {
			panic(err)
		}

		got := parser.ParsedDefinitions.Floats[value.Reference]

		if want != got {
			panic(fmt.Errorf("mustParseFloatValue: want: %.2f, got: %.2f [check: %d]", want, got, i))
		}
	}
}

func mustMergeArgumentOnField(fieldName, argumentName, valueContent string) checkFunc {
	return func(parser *Parser, i int) {
		mod := NewManualAstMod(parser)

		sensitiveInformationRef, _, err := mod.PutLiteralString(fieldName)
		if err != nil {
			panic(err)
		}

		argumentNameRef, _, err := mod.PutLiteralString(argumentName)
		if err != nil {
			panic(err)
		}

		valueContentRef, valueContentByteSliceRef, err := mod.PutLiteralString(valueContent)
		if err != nil {
			panic(err)
		}

		val := document.Value{
			ValueType: document.ValueTypeString,
			Reference: valueContentRef,
			Raw:       valueContentByteSliceRef,
		}

		valueRef := mod.PutValue(val)

		for fieldRef, field := range parser.ParsedDefinitions.Fields {
			if field.Name == sensitiveInformationRef {

				arg := document.Argument{
					Name:  argumentNameRef,
					Value: valueRef,
				}

				argumentRef := mod.PutArgument(arg)

				mod.MergeArgIntoFieldArguments(argumentRef, fieldRef)
				break
			}
		}
	}
}
