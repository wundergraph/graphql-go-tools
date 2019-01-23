package parser

import (
	"encoding/json"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"log"
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

func TestParser(t *testing.T) {

	type checkFunc func(parser *Parser, i int)

	run := func(input string, checks ...checkFunc) {
		parser := NewParser()
		if err := parser.l.SetInput([]byte(input)); err != nil {
			panic(err)
		}
		for i, checkFunc := range checks {
			checkFunc(parser, i)
		}
	}

	node := func(rules ...rule) ruleSet {
		return rules
	}

	nodes := func(sets ...ruleSet) []ruleSet {
		return sets
	}

	evalRules := func(node document.Node, parser *Parser, rules ruleSet, ruleIndex int) {
		rules.eval(node, parser, ruleIndex)
	}

	hasName := func(wantName string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			gotName := string(parser.ByteSlice(node.NodeName()))
			if wantName != gotName {
				panic(fmt.Errorf("hasName: want: %s, got: %s [rule: %d, node: %d]", wantName, gotName, ruleIndex, ruleSetIndex))
			}
		}
	}

	hasSchemaOperationTypeName := func(operationType document.OperationType, wantTypeName string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			schemaDefinition := node.(document.SchemaDefinition)

			gotQuery := string(schemaDefinition.Query)
			gotMutation := string(schemaDefinition.Mutation)
			gotSubscription := string(schemaDefinition.Subscription)

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

	hasPosition := func(position position.Position) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			gotPosition := node.NodePosition()
			if !reflect.DeepEqual(position, gotPosition) {
				panic(fmt.Errorf("hasPosition: want: %+v, got: %+v [rule: %d, node: %d]", position, gotPosition, ruleIndex, ruleSetIndex))
			}
		}
	}

	hasAlias := func(wantAlias string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			gotAlias := string(parser.ByteSlice(node.NodeAlias()))
			if wantAlias != gotAlias {
				panic(fmt.Errorf("hasAlias: want: %s, got: %s [rule: %d, node: %d]", wantAlias, gotAlias, ruleIndex, ruleSetIndex))
			}
		}
	}

	hasDescription := func(wantDescription string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			gotDescription := string(parser.ByteSlice(node.NodeDescription()))
			if wantDescription != gotDescription {
				panic(fmt.Errorf("hasName: want: %s, got: %s [rule: %d, node: %d]", wantDescription, gotDescription, ruleIndex, ruleSetIndex))
			}
		}
	}

	hasDirectiveLocations := func(locations ...document.DirectiveLocation) rule {
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

	unwrapObjectField := func(node document.Node, parser *Parser) document.Node {
		objectField, ok := node.(document.ObjectField)
		if ok {
			node = parser.ParsedDefinitions.Values[objectField.Value]
		}
		return node
	}

	expectIntegerValue := func(want int32) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, parser)
			got := parser.ParsedDefinitions.Integers[node.NodeValueReference()]
			if want != got {
				panic(fmt.Errorf("expectIntegerValue: want: %d, got: %d [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}

	expectFloatValue := func(want float32) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, parser)
			got := parser.ParsedDefinitions.Floats[node.NodeValueReference()]
			if want != got {
				panic(fmt.Errorf("expectIntegerValue: want: %.2f, got: %.2f [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}

	expectBooleanValue := func(want bool) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, parser)
			got := parser.ParsedDefinitions.Booleans[node.NodeValueReference()]
			if want != got {
				panic(fmt.Errorf("expectIntegerValue: want: %v, got: %v [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}

	expectByteSliceValue := func(want string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, parser)
			got := string(parser.ByteSlice(parser.ParsedDefinitions.ByteSliceReferences[node.NodeValueReference()]))
			if want != got {
				panic(fmt.Errorf("expectByteSliceValue: want: %s, got: %s [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}

	expectListValue := func(rules ...rule) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			list := parser.ParsedDefinitions.ListValues[node.NodeValueReference()]
			for j, rule := range rules {
				valueIndex := list[j]
				value := parser.ParsedDefinitions.Values[valueIndex]
				rule(value, parser, j, ruleSetIndex)
			}
		}
	}

	expectObjectValue := func(rules ...ruleSet) rule {
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

	hasOperationType := func(operationType document.OperationType) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			gotOperationType := node.NodeOperationType().String()
			wantOperationType := operationType.String()
			if wantOperationType != gotOperationType {
				panic(fmt.Errorf("hasOperationType: want: %s, got: %s [rule: %d, node: %d]", wantOperationType, gotOperationType, ruleIndex, ruleSetIndex))
			}
		}
	}

	hasTypeKind := func(wantTypeKind document.TypeKind) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			gotTypeKind := node.(document.Type).Kind
			if wantTypeKind != gotTypeKind {
				panic(fmt.Errorf("hasTypeKind: want(typeKind): %s, got: %s [rule: %d, node: %d]", wantTypeKind, gotTypeKind, ruleIndex, ruleSetIndex))
			}
		}
	}

	nodeType := func(rules ...rule) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			nodeType := parser.ParsedDefinitions.Types[node.NodeType()]
			for j, rule := range rules {
				rule(nodeType, parser, j, ruleSetIndex)
			}
		}
	}

	ofType := func(rules ...rule) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			ofType := parser.ParsedDefinitions.Types[node.(document.Type).OfType]
			for j, rule := range rules {
				rule(ofType, parser, j, ruleSetIndex)
			}
		}
	}

	hasTypeName := func(wantName string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			if fragment, ok := node.(document.FragmentDefinition); ok {
				node = parser.ParsedDefinitions.Types[fragment.TypeCondition]
			}

			if inlineFragment, ok := node.(document.InlineFragment); ok {
				node = parser.ParsedDefinitions.Types[inlineFragment.TypeCondition]
			}

			gotName := string(parser.ByteSlice(node.(document.Type).Name))
			if wantName != gotName {
				panic(fmt.Errorf("hasTypeName: want: %s, got: %s [rule: %d, node: %d]", wantName, gotName, ruleIndex, ruleSetIndex))
			}
		}
	}

	hasDefaultValue := func(rules ...rule) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			index := node.NodeDefaultValue()
			node = parser.ParsedDefinitions.Values[index]
			for k, rule := range rules {
				rule(node, parser, k, ruleSetIndex)
			}
		}
	}

	hasValueType := func(valueType document.ValueType) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			if node.NodeValueType() != valueType {
				panic(fmt.Errorf("hasValueType: want: %s, got: %s [check: %d]", valueType.String(), node.NodeValueType().String(), ruleIndex))
			}
		}
	}

	hasByteSliceValue := func(want string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {
			byteSliceRef := parser.ParsedDefinitions.ByteSliceReferences[node.NodeValueReference()]
			got := string(parser.ByteSlice(byteSliceRef))
			if want != got {
				panic(fmt.Errorf("hasByteSliceValue: want: %s, got: %s [check: %d]", want, got, ruleIndex))
			}
		}
	}

	hasEnumValuesDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeEnumValuesDefinition()

			for j, k := range index {
				ruleSet := rules[j]
				subNode := parser.ParsedDefinitions.EnumValuesDefinitions[k]
				ruleSet.eval(subNode, parser, k)
			}
		}
	}

	hasUnionTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeUnionTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := parser.ParsedDefinitions.UnionTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, parser, j)
			}
		}
	}

	hasScalarTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeScalarTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := parser.ParsedDefinitions.ScalarTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, parser, j)
			}
		}
	}

	hasObjectTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeObjectTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := parser.ParsedDefinitions.ObjectTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, parser, j)
			}
		}
	}

	hasInterfaceTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeInterfaceTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := parser.ParsedDefinitions.InterfaceTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, parser, j)
			}
		}
	}

	hasEnumTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeEnumTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := parser.ParsedDefinitions.EnumTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, parser, j)
			}
		}
	}

	hasInputObjectTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeInputObjectTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := parser.ParsedDefinitions.InputObjectTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, parser, j)
			}
		}
	}

	hasDirectiveDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeDirectiveDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := parser.ParsedDefinitions.DirectiveDefinitions[definitionIndex]
				ruleSet.eval(subNode, parser, j)
			}
		}
	}

	hasUnionMemberTypes := func(members ...string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeUnionMemberTypes()

			for j, want := range members {
				got := string(parser.ByteSlice(typeDefinitionIndex[j]))
				if want != got {
					panic(fmt.Errorf("hasUnionMemberTypes: want: %s, got: %s [check: %d]", want, got, ruleSetIndex))
				}
			}
		}
	}

	hasSchemaDefinition := func(rules ...rule) rule {
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

	hasVariableDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeVariableDefinitions()

			for j, k := range index {
				ruleSet := rules[j]
				subNode := parser.ParsedDefinitions.VariableDefinitions[k]
				ruleSet.eval(subNode, parser, k)
			}
		}
	}

	hasDirectives := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeDirectives()

			for i := range rules {
				ruleSet := rules[i]
				subNode := parser.ParsedDefinitions.Directives[index[i]]
				ruleSet.eval(subNode, parser, index[i])
			}
		}
	}

	hasImplementsInterfaces := func(interfaces ...string) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			actual := node.NodeImplementsInterfaces()
			for i, want := range interfaces {
				got := string(parser.ByteSlice(actual[i]))

				if want != got {
					panic(fmt.Errorf("hasImplementsInterfaces: want(at: %d): %s, got: %s [check: %d]", i, want, got, ruleSetIndex))
				}
			}
		}
	}

	hasFields := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeFields()

			for i := range rules {
				ruleSet := rules[i]
				subNode := parser.ParsedDefinitions.Fields[index[i]]
				ruleSet.eval(subNode, parser, index[i])
			}
		}
	}

	hasFieldsDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeFieldsDefinition()

			for i := range rules {
				ruleSet := rules[i]
				field := parser.ParsedDefinitions.FieldDefinitions[index[i]]
				ruleSet.eval(field, parser, index[i])
			}
		}
	}

	hasInputFieldsDefinition := func(rules ...rule) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeInputFieldsDefinition()
			node = parser.ParsedDefinitions.InputFieldsDefinitions[index]

			for i, rule := range rules {
				rule(node, parser, i, ruleSetIndex)
			}
		}
	}

	hasInputValueDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeInputValueDefinitions()

			for i := range rules {
				ruleSet := rules[i]
				subNode := parser.ParsedDefinitions.InputValueDefinitions[index[i]]
				ruleSet.eval(subNode, parser, index[i])
			}
		}
	}

	hasArguments := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeArguments()

			for i := range rules {
				ruleSet := rules[i]
				subNode := parser.ParsedDefinitions.Arguments[index[i]]
				ruleSet.eval(subNode, parser, index[i])
			}
		}
	}

	hasArgumentsDefinition := func(rules ...rule) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeArgumentsDefinition()
			node = parser.ParsedDefinitions.ArgumentsDefinitions[index]

			for k, rule := range rules {
				rule(node, parser, k, ruleSetIndex)
			}
		}
	}

	hasInlineFragments := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeInlineFragments()

			for i := range rules {
				ruleSet := rules[i]
				subNode := parser.ParsedDefinitions.InlineFragments[index[i]]
				ruleSet.eval(subNode, parser, index[i])
			}
		}
	}

	hasFragmentSpreads := func(rules ...ruleSet) rule {
		return func(node document.Node, parser *Parser, ruleIndex, ruleSetIndex int) {

			index := node.NodeFragmentSpreads()

			for i := range rules {
				ruleSet := rules[i]
				subNode := parser.ParsedDefinitions.FragmentSpreads[index[i]]
				ruleSet.eval(subNode, parser, index[i])
			}
		}
	}

	mustPanic := func(c checkFunc) checkFunc {
		return func(parser *Parser, i int) {

			defer func() {
				if recover() == nil {
					panic(fmt.Errorf("mustPanic: panic expected [check: %d]", i))
				}
			}()

			c(parser, i)
		}
	}

	mustParseArguments := func(wantArgumentNodes ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseArguments(&index); err != nil {
				panic(err)
			}

			for k, want := range wantArgumentNodes {
				argument := parser.ParsedDefinitions.Arguments[k]
				want.eval(argument, parser, k)
			}
		}
	}

	mustParseArgumentDefinition := func(rule ...ruleSet) checkFunc {
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

	mustParseDefaultValue := func(wantValueType document.ValueType) checkFunc {
		return func(parser *Parser, i int) {
			var index int
			err := parser.parseDefaultValue(&index)
			if err != nil {
				panic(err)
			}

			val := parser.ParsedDefinitions.Values[index]

			if val.ValueType != wantValueType {
				panic(fmt.Errorf("mustParseDefaultValue: want(valueType): %s, got: %s", wantValueType.String(), val.ValueType.String()))
			}
		}
	}

	mustParseDirectiveDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseDirectives := func(directives ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseDirectives(&index); err != nil {
				panic(err)
			}

			for k, rule := range directives {
				node := parser.ParsedDefinitions.Directives[index[k]]
				rule.eval(node, parser, k)
			}
		}
	}

	mustParseEnumTypeDefinition := func(rules ...rule) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseEnumTypeDefinition(nil, &index); err != nil {
				panic(err)
			}

			enum := parser.ParsedDefinitions.EnumTypeDefinitions[0]
			evalRules(enum, parser, rules, i)
		}
	}

	mustParseExecutableDefinition := func(fragments []ruleSet, operations []ruleSet) checkFunc {
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

	mustParseFields := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseField(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				reverseIndex := len(parser.ParsedDefinitions.Fields) - 1 - j
				field := parser.ParsedDefinitions.Fields[reverseIndex]
				evalRules(field, parser, rule, i)
			}
		}
	}

	mustParseFieldsDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseFragmentDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseFragmentSpread := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseFragmentSpread(position.Position{}, &index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				spread := parser.ParsedDefinitions.FragmentSpreads[j]
				evalRules(spread, parser, rule, i)
			}
		}
	}

	mustParseImplementsInterfaces := func(implements ...string) checkFunc {
		return func(parser *Parser, i int) {

			interfaces, err := parser.parseImplementsInterfaces()
			if err != nil {
				panic(err)
			}

			for j, want := range implements {
				got := string(parser.ByteSlice(interfaces[j]))
				if want != got {
					panic(fmt.Errorf("mustParseImplementsInterfaces: want: %s, got: %s [check: %d]", want, got, i))
				}
			}
		}
	}

	mustParseLiteral := func(wantKeyword keyword.Keyword, wantLiteral string) checkFunc {
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

	mustParseInlineFragments := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseInlineFragment(position.Position{}, &index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				reverseIndex := len(parser.ParsedDefinitions.InlineFragments) - 1 - j
				inlineFragment := parser.ParsedDefinitions.InlineFragments[reverseIndex]
				evalRules(inlineFragment, parser, rule, i)
			}
		}
	}

	mustParseInputFieldsDefinition := func(rules ...rule) checkFunc {
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

	mustParseInputObjectTypeDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseInputValueDefinitions := func(rules ...ruleSet) checkFunc {
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

	mustParseInterfaceTypeDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseObjectTypeDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseOperationDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseScalarTypeDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseTypeSystemDefinition := func(rules ruleSet) checkFunc {
		return func(parser *Parser, i int) {

			definition, err := parser.parseTypeSystemDefinition()
			if err != nil {
				panic(err)
			}

			evalRules(definition, parser, rules, i)
		}
	}

	mustParseSchemaDefinition := func(rules ...rule) checkFunc {
		return func(parser *Parser, i int) {
			var schemaDefinition document.SchemaDefinition
			err := parser.parseSchemaDefinition(&schemaDefinition)
			if err != nil {
				panic(err)
			}

			for k, rule := range rules {
				rule(schemaDefinition, parser, k, i)
			}
		}
	}

	mustParseSelectionSet := func(rules ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var selectionSet document.SelectionSet
			if err := parser.parseSelectionSet(&selectionSet); err != nil {
				panic(err)
			}

			rules.eval(selectionSet, parser, i)
		}
	}

	mustParseUnionTypeDefinition := func(rules ...ruleSet) checkFunc {
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

	mustParseVariableDefinitions := func(rules ...ruleSet) checkFunc {
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

	mustParseValue := func(valueType document.ValueType, rules ...rule) checkFunc {
		return func(parser *Parser, i int) {
			var index int
			if err := parser.parseValue(&index); err != nil {
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

	mustParseType := func(rules ...rule) checkFunc {
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

	mustParseFloatValue := func(t *testing.T, input string, want float32) checkFunc {
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

			var index int
			if err := parser.parsePeekedFloatValue(&index); err != nil {
				panic(err)
			}

			got := parser.ParsedDefinitions.Floats[index]

			if want != got {
				panic(fmt.Errorf("mustParseFloatValue: want: %.2f, got: %.2f [check: %d]", want, got, i))
			}
		}
	}

	// arguments

	t.Run("string argument", func(t *testing.T) {
		run(`(name: "Gophus")`,
			mustParseArguments(
				node(
					hasName("name"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   16,
					}),
				),
			),
		)
	})
	t.Run("string array argument", func(t *testing.T) {
		run(`(fooBars: ["foo","bar"])`,
			mustParseArguments(
				node(
					hasName("fooBars"),
				),
			),
		)
	})
	t.Run("int array argument", func(t *testing.T) {
		run(`(integers: [1,2,3])`,
			mustParseArguments(
				node(
					hasName("integers"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   19,
					}),
				),
			),
		)
	})
	t.Run("multiple string arguments", func(t *testing.T) {
		run(`(name: "Gophus", surname: "Gophersson")`,
			mustParseArguments(
				node(
					hasName("name"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   16,
					}),
				),
				node(
					hasName("surname"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 18,
						CharEnd:   39,
					}),
				),
			),
		)
	})
	t.Run("invalid argument must err", func(t *testing.T) {
		run(`(name: "Gophus", surname: "Gophersson"`,
			mustPanic(mustParseArguments()))
	})
	t.Run("invalid argument must err 2", func(t *testing.T) {
		run(`((name: "Gophus", surname: "Gophersson")`,
			mustPanic(mustParseArguments()))
	})
	t.Run("invalid argument must err 3", func(t *testing.T) {
		run(`(name: .)`,
			mustPanic(mustParseArguments()))
	})

	// arguments definition

	t.Run("single int value", func(t *testing.T) {
		run(`(inputValue: Int)`,
			mustParseArgumentDefinition(
				node(
					hasInputValueDefinitions(
						node(
							hasName("inputValue"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   18,
					}),
				),
			),
		)
	})
	t.Run("optional value", func(t *testing.T) {
		run(" ", mustParseArgumentDefinition())
	})
	t.Run("multiple values", func(t *testing.T) {
		run(`(inputValue: Int, outputValue: String)`,
			mustParseArgumentDefinition(
				node(
					hasInputValueDefinitions(
						node(
							hasName("inputValue"),
						),
						node(
							hasName("outputValue"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   39,
					}),
				),
			),
		)
	})
	t.Run("not read optional", func(t *testing.T) {
		run(`inputValue: Int)`,
			mustParseArgumentDefinition())
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`((inputValue: Int)`,
			mustPanic(mustParseArgumentDefinition()))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`(inputValue: Int`,
			mustPanic(mustParseArgumentDefinition()))
	})

	// parseDefaultValue

	t.Run("integer", func(t *testing.T) {
		run("= 2", mustParseDefaultValue(document.ValueTypeInt))
	})
	t.Run("bool", func(t *testing.T) {
		run("= true", mustParseDefaultValue(document.ValueTypeBoolean))
	})
	t.Run("invalid", func(t *testing.T) {
		run("true", mustPanic(mustParseDefaultValue(document.ValueTypeBoolean)))
	})

	// parseDirectiveDefinition

	t.Run("single directive with location", func(t *testing.T) {
		run("directive @ somewhere on QUERY",
			mustParseDirectiveDefinition(
				node(
					hasName("somewhere"),
					hasDirectiveLocations(document.DirectiveLocationQUERY),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   31,
					}),
				),
			),
		)
	})
	t.Run("trailing pipe", func(t *testing.T) {
		run("directive @ somewhere on | QUERY",
			mustParseDirectiveDefinition(
				node(
					hasName("somewhere"),
					hasDirectiveLocations(document.DirectiveLocationQUERY),
				),
			),
		)
	})
	t.Run("with input value", func(t *testing.T) {
		run("directive @ somewhere(inputValue: Int) on QUERY",
			mustParseDirectiveDefinition(
				node(
					hasName("somewhere"),
					hasDirectiveLocations(document.DirectiveLocationQUERY),
					hasArgumentsDefinition(
						hasInputValueDefinitions(
							node(
								hasName("inputValue"),
							),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   48,
					}),
				),
			),
		)
	})
	t.Run("multiple locations", func(t *testing.T) {
		run("directive @ somewhere on QUERY |\nMUTATION",
			mustParseDirectiveDefinition(
				node(
					hasName("somewhere"),
					hasDirectiveLocations(document.DirectiveLocationQUERY, document.DirectiveLocationMUTATION),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   2,
						CharStart: 1,
						CharEnd:   9,
					}),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("directive @ somewhere QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
						hasDirectiveLocations(document.DirectiveLocationQUERY),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("directive @ somewhere off QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
					),
				),
			),
		)
	})
	t.Run("missing at", func(t *testing.T) {
		run("directive somewhere off QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
					),
				),
			),
		)
	})
	t.Run("invalid args", func(t *testing.T) {
		run("directive @ somewhere(inputValue: .) on QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
						hasDirectiveLocations(document.DirectiveLocationQUERY),
					),
				),
			),
		)
	})
	t.Run("missing ident after at", func(t *testing.T) {
		run("directive @ \"somewhere\" off QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
					),
				),
			),
		)
	})
	t.Run("invalid location", func(t *testing.T) {
		run("directive @ somewhere on QUERY | thisshouldntwork",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
						hasDirectiveLocations(document.DirectiveLocationQUERY),
					),
				),
			),
		)
	})
	t.Run("invalid prefix", func(t *testing.T) {
		run("notdirective @ somewhere on QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
					),
				),
			),
		)
	})

	// parseDirectives

	t.Run(`simple directive`, func(t *testing.T) {
		run(`@rename(index: 3)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   18,
					}),
				),
			),
		)
	})
	t.Run("multiple directives", func(t *testing.T) {
		run(`@rename(index: 3)@moveto(index: 4)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
				),
				node(
					hasName("moveto"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 18,
						CharEnd:   35,
					}),
				),
			),
		)
	})
	t.Run("multiple arguments", func(t *testing.T) {
		run(`@rename(index: 3, count: 10)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
						node(
							hasName("count"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   29,
					}),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`@rename(index)`,
			mustPanic(mustParseDirectives()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`@1337(index)`,
			mustPanic(mustParseDirectives()),
		)
	})

	// parseEnumTypeDefinition

	t.Run("simple enum", func(t *testing.T) {
		run(`enum Direction {
						NORTH
						EAST
						SOUTH
						WEST
		}`,
			mustParseEnumTypeDefinition(
				hasName("Direction"),
				hasEnumValuesDefinitions(
					node(hasName("NORTH")),
					node(hasName("EAST")),
					node(hasName("SOUTH")),
					node(hasName("WEST")),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   6,
					CharEnd:   4,
				}),
			),
		)
	})
	t.Run("enum with descriptions", func(t *testing.T) {
		run(`enum Direction {
  						"describes north"
  						NORTH
  						"describes east"
  						EAST
  						"describes south"
  						SOUTH
  						"describes west"
  						WEST }`,
			mustParseEnumTypeDefinition(
				hasName("Direction"),
				hasEnumValuesDefinitions(
					node(hasName("NORTH"), hasDescription("describes north")),
					node(hasName("EAST"), hasDescription("describes east")),
					node(hasName("SOUTH"), hasDescription("describes south")),
					node(hasName("WEST"), hasDescription("describes west")),
				),
			))
	})
	t.Run("enum with space", func(t *testing.T) {
		run(`enum Direction {
  "describes north"
  NORTH

  "describes east"
  EAST

  "describes south"
  SOUTH

  "describes west"
  WEST
}`, mustParseEnumTypeDefinition(
			hasName("Direction"),
			hasEnumValuesDefinitions(
				node(hasName("NORTH"), hasDescription("describes north")),
				node(hasName("EAST"), hasDescription("describes east")),
				node(hasName("SOUTH"), hasDescription("describes south")),
				node(hasName("WEST"), hasDescription("describes west")),
			),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   13,
				CharEnd:   2,
			}),
		))
	})
	t.Run("enum with directives", func(t *testing.T) {
		run(`enum Direction @fromTop(to: "bottom") @fromBottom(to: "top"){ NORTH }`,
			mustParseEnumTypeDefinition(
				hasName("Direction"),
				hasDirectives(
					node(hasName("fromTop")),
					node(hasName("fromBottom")),
				),
				hasEnumValuesDefinitions(
					node(hasName("NORTH")),
				),
			))
	})
	t.Run("enum without values", func(t *testing.T) {
		run("enum Direction", mustParseEnumTypeDefinition(hasName("Direction")))
	})
	t.Run("invalid enum", func(t *testing.T) {
		run("enum Direction {", mustPanic(mustParseEnumTypeDefinition()))
	})
	t.Run("invalid enum 2", func(t *testing.T) {
		run("enum  \"Direction\" {}", mustPanic(mustParseEnumTypeDefinition()))
	})
	t.Run("invalid enum 2", func(t *testing.T) {
		run("enum  Direction @from(foo: .)", mustPanic(mustParseEnumTypeDefinition(hasName("Direction"))))
	})
	t.Run("invalid enum 3", func(t *testing.T) {
		run("enum Direction {FOO @bar(baz: .)}", mustPanic(mustParseEnumTypeDefinition(hasName("Direction"))))
	})
	t.Run("invalid enum 4", func(t *testing.T) {
		run("notenum Direction", mustPanic(mustParseEnumTypeDefinition()))
	})

	// parseExecutableDefinition

	t.Run("query with variable, directive and field", func(t *testing.T) {
		run(`query allGophers($color: String)@rename(index: 3) { name }`,
			mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
								nodeType(
									hasTypeKind(document.TypeKindNAMED),
									hasTypeName("String"),
								),
							),
						),
						hasDirectives(
							node(hasName("rename")),
						),
						hasFields(
							node(
								hasName("name"),
							),
						),
						hasPosition(position.Position{
							LineStart: 1,
							CharStart: 1,
							LineEnd:   1,
							CharEnd:   59,
						}),
					),
				),
			))
	})
	t.Run("mutation", func(t *testing.T) {
		run(`mutation allGophers`,
			mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeMutation),
						hasName("allGophers"),
					),
				),
			))
	})
	t.Run("subscription", func(t *testing.T) {
		run(`subscription allGophers`,
			mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeSubscription),
						hasName("allGophers"),
					),
				),
			))
	})
	t.Run("fragment with query", func(t *testing.T) {
		run(`
				fragment MyFragment on SomeType @rename(index: 3){
					name
				}
				query Q1 {
					foo
				}
				`,
			mustParseExecutableDefinition(
				nodes(
					node(
						hasName("MyFragment"),
						hasDirectives(
							node(
								hasName("rename"),
							),
						),
						hasFields(
							node(hasName("name")),
						),
					),
				),
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("Q1"),
						hasFields(
							node(hasName("foo")),
						),
					),
				),
			))
	})
	t.Run("multiple queries", func(t *testing.T) {
		run(`
				query allGophers($color: String) {
					name
				}

				query allGophinas($color: String) {
					name
				}

				`,
			mustParseExecutableDefinition(nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
								nodeType(
									hasTypeKind(document.TypeKindNAMED),
									hasTypeName("String"),
								),
							),
						),
						hasFields(
							node(hasName("name")),
						),
					),
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophinas"),
						hasVariableDefinitions(
							node(
								hasName("color"),
								nodeType(
									hasTypeKind(document.TypeKindNAMED),
									hasTypeName("String"),
								),
							),
						),
						hasFields(
							node(hasName("name")),
						),
					),
				),
			))
	})
	t.Run("invalid", func(t *testing.T) {
		run(`
				Barry allGophers($color: String)@rename(index: 3) {
					name
				}`, mustParseExecutableDefinition(nil, nil))
	})
	t.Run("large nested object", func(t *testing.T) {
		run(`
				query QueryWithFragments {
					hero {
						...heroFields
					}
				}

				fragment heroFields on SuperHero {
					name
					skill
					...on DrivingSuperHero {
						vehicles {
							...vehicleFields
						}
					}
				}

				fragment vehicleFields on Vehicle {
					name
					weapon
				}
				`,
			mustParseExecutableDefinition(
				nodes(
					node(
						hasName("heroFields"),
						hasPosition(position.Position{
							LineStart: 8,
							CharStart: 5,
							LineEnd:   16,
							CharEnd:   6,
						}),
					),
					node(
						hasName("vehicleFields"),
						hasPosition(position.Position{
							LineStart: 18,
							CharStart: 5,
							LineEnd:   21,
							CharEnd:   6,
						}),
					),
				),
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("QueryWithFragments"),
						hasPosition(position.Position{
							LineStart: 2,
							CharStart: 5,
							LineEnd:   6,
							CharEnd:   6,
						}),
					),
				)))
	})
	t.Run("unnamed operation", func(t *testing.T) {
		run("{\n  hero {\n    id\n    name\n  }\n}\n",
			mustParseExecutableDefinition(nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasFields(
							node(
								hasName("hero"),
								hasFields(
									node(
										hasName("id"),
									),
									node(
										hasName("name"),
									),
								),
							),
						),
					),
				),
			))
	})
	t.Run("invalid", func(t *testing.T) {
		run("{foo { bar(foo: .) }}",
			mustPanic(mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasFields(node(
							hasName("foo"),
							hasFields(node(
								hasName("\"bar\""),
							)),
						)),
					)),
			)))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("query SomeQuery {foo { bar(foo: .) }}",
			mustPanic(mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasFields(node(
							hasName("foo"),
							hasFields(node(
								hasName("\"bar\""),
							)),
						)),
					)),
			)))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("query SomeQuery {foo { bar }} fragment Fields on SomeQuery { foo(bar: .) }",
			mustPanic(mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasFields(node(
							hasName("foo"),
							hasFields(node(
								hasName("\"bar\""),
							)),
						)),
					)),
			)))
	})

	// parseField

	t.Run("parse field with name, arguments and directive", func(t *testing.T) {
		run("preferredName: originalName(isSet: true) @rename(index: 3)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasArguments(
						node(
							hasName("isSet"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   59,
					}),
				),
			),
		)
	})
	t.Run("without optional alias", func(t *testing.T) {
		run("originalName(isSet: true) @rename(index: 3)",
			mustParseFields(
				node(
					hasName("originalName"),
					hasArguments(
						node(hasName("isSet")),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
				),
			),
		)
	})
	t.Run("without optional arguments", func(t *testing.T) {
		run("preferredName: originalName @rename(index: 3)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
				),
			),
		)
	})
	t.Run("without optional directives", func(t *testing.T) {
		run("preferredName: originalName(isSet: true)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasArguments(
						node(
							hasName("isSet"),
						),
					),
				),
			),
		)
	})
	t.Run("with nested selection sets", func(t *testing.T) {
		run(`
				level1 {
					level2 {
						level3
					}
				}
				`,
			mustParseFields(
				node(
					hasName("level1"),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 5,
						LineEnd:   6,
						CharEnd:   6,
					}),
					hasFields(
						node(
							hasName("level2"),
							hasPosition(position.Position{
								LineStart: 3,
								CharStart: 6,
								LineEnd:   5,
								CharEnd:   7,
							}),
							hasFields(
								node(
									hasName("level3"),
									hasPosition(position.Position{
										LineStart: 4,
										CharStart: 7,
										LineEnd:   4,
										CharEnd:   13,
									}),
								),
							),
						),
					),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`
				level1 {
					alis: .
				}
				`,
			mustPanic(
				mustParseFields(
					node(
						hasName("level1"),
						hasFields(
							node(
								hasAlias("alias"),
								hasName("."),
							),
						),
					),
				)))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`
				level1 {
					alis: ok @foo(bar: .)
				}
				`,
			mustPanic(
				mustParseFields(
					node(
						hasName("level1"),
						hasFields(
							node(
								hasAlias("alias"),
								hasName("ok"),
							),
						),
					),
				)))
	})

	// parseFieldsDefinition

	t.Run("simple field definition", func(t *testing.T) {
		run(`{ name: String }`,
			mustParseFieldsDefinition(
				node(
					hasName("name"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 3,
						LineEnd:   1,
						CharEnd:   15,
					}),
				),
			))
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`{
					name: String
					age: Int
				}`,
			mustParseFieldsDefinition(
				node(
					hasName("name"),
					nodeType(
						hasTypeName("String"),
					),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 6,
						LineEnd:   2,
						CharEnd:   18,
					}),
				),
				node(
					hasName("age"),
					nodeType(
						hasTypeName("Int"),
					),
				),
			))
	})
	t.Run("with description", func(t *testing.T) {
		run(`{
					"describes the name"
					name: String
				}`,
			mustParseFieldsDefinition(
				node(
					hasDescription("describes the name"),
					hasName("name"),
				),
			))
	})
	t.Run("non null list", func(t *testing.T) {
		run(`{
					name: [ String ]!
					age: Int!
				}`,
			mustParseFieldsDefinition(
				node(
					hasName("name"),
					nodeType(
						hasTypeKind(document.TypeKindNON_NULL),
						ofType(
							hasTypeKind(document.TypeKindLIST),
						),
					),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 6,
						LineEnd:   2,
						CharEnd:   23,
					}),
				),
				node(
					hasName("age"),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`{ name(foo: .): String }`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`{ name. String }`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`{ name: [String! }`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`{ name: String @foo(bar: .)}`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
	t.Run("invalid 4", func(t *testing.T) {
		run(`{ name: String`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})

	// parseFragmentDefinition

	t.Run("simple fragment definition", func(t *testing.T) {
		run(`
				fragment MyFragment on SomeType @rename(index: 3){
					name
				}`,
			mustParseFragmentDefinition(
				node(
					hasName("MyFragment"),
					hasTypeName("SomeType"),
					hasFields(
						node(
							hasName("name"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 5,
						LineEnd:   4,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("fragment without optional directives", func(t *testing.T) {
		run(`
				fragment MyFragment on SomeType{
					name
				}`,
			mustParseFragmentDefinition(
				node(
					hasName("MyFragment"),
					hasTypeName("SomeType"),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			))
	})
	t.Run("invalid fragment 1", func(t *testing.T) {
		run(`
				fragment MyFragment SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 2", func(t *testing.T) {
		run(`
				fragment MyFragment un SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 3", func(t *testing.T) {
		run(`
				fragment 1337 on SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 4", func(t *testing.T) {
		run(`
				fragment Fields on [SomeType! {
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 4", func(t *testing.T) {
		run(`
				fragment Fields on SomeType @foo(bar: .) {
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})

	// parseFragmentSpread

	t.Run("with directive", func(t *testing.T) {
		run("firstFragment @rename(index: 3)",
			mustParseFragmentSpread(
				node(
					hasName("firstFragment"),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 0, // default, see mustParseFragmentSpread
						CharStart: 0, // default, see mustParseFragmentSpread
						LineEnd:   1,
						CharEnd:   32,
					}),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run("firstFragment",
			mustParseFragmentSpread(
				node(
					hasName("firstFragment"),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("on", mustPanic(mustParseFragmentSpread()))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("afragment @foo(bar: .)", mustPanic(mustParseFragmentSpread()))
	})

	// parseImplementsInterfaces

	t.Run("simple", func(t *testing.T) {
		run("implements Dogs",
			mustParseImplementsInterfaces("Dogs"),
		)
	})
	t.Run("multiple", func(t *testing.T) {
		run("implements Dogs & Cats & Mice",
			mustParseImplementsInterfaces("Dogs", "Cats", "Mice"),
		)
	})
	t.Run("multiple without &", func(t *testing.T) {
		run("implements Dogs & Cats Mice",
			mustParseImplementsInterfaces("Dogs", "Cats"),
			mustParseLiteral(keyword.IDENT, "Mice"),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("implement Dogs & Cats Mice",
			mustParseImplementsInterfaces(),
			mustParseLiteral(keyword.IDENT, "implement"),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("implements foo & .",
			mustPanic(mustParseImplementsInterfaces("foo", ".")),
		)
	})

	// parseInlineFragment

	t.Run("with nested selectionsets", func(t *testing.T) {
		run(`Goland {
					... on GoWater {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustParseInlineFragments(
				node(
					hasTypeName("Goland"),
					hasPosition(position.Position{
						LineStart: 0, // default, see mustParseFragmentSpread
						CharStart: 0, // default, see mustParseFragmentSpread
						LineEnd:   7,
						CharEnd:   6,
					}),
					hasInlineFragments(
						node(
							hasTypeName("GoWater"),
							hasPosition(position.Position{
								LineStart: 2,
								CharStart: 6,
								LineEnd:   6,
								CharEnd:   7,
							}),
							hasInlineFragments(
								node(
									hasTypeName("GoAir"),
									hasPosition(position.Position{
										LineStart: 3,
										CharStart: 7,
										LineEnd:   5,
										CharEnd:   8,
									}),
									hasFields(
										node(
											hasName("go"),
										),
									),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`Goland {
					... on 1337 {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustPanic(
				mustParseInlineFragments(
					node(
						hasTypeName("\"Goland\""),
						hasInlineFragments(
							node(
								hasTypeName("1337"),
								hasInlineFragments(
									node(
										hasTypeName("GoAir"),
										hasFields(
											node(
												hasName("go"),
											),
										),
									),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`Goland {
					... on GoWater @foo(bar: .) {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustPanic(
				mustParseInlineFragments(
					node(
						hasTypeName("Goland"),
						hasInlineFragments(
							node(
								hasTypeName("GoWater"),
								hasInlineFragments(
									node(
										hasTypeName("GoAir"),
										hasFields(
											node(
												hasName("go"),
											),
										),
									),
								),
							),
						),
					),
				),
			))
	})

	// parseInputFieldsDefinition

	t.Run("simple input fields definition", func(t *testing.T) {
		run("{inputValue: Int}",
			mustParseInputFieldsDefinition(
				hasInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   18,
				}),
			),
		)
	})
	t.Run("optional", func(t *testing.T) {
		run(" ", mustParseInputFieldsDefinition())
	})
	t.Run("multiple", func(t *testing.T) {
		run("{inputValue: Int, outputValue: String}",
			mustParseInputFieldsDefinition(
				hasInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
					node(
						hasName("outputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   39,
				}),
			),
		)
	})
	t.Run("optional", func(t *testing.T) {
		run("inputValue: Int}",
			mustParseInputFieldsDefinition(),
			mustParseLiteral(keyword.IDENT, "inputValue"),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("{{inputValue: Int}",
			mustPanic(mustParseInputFieldsDefinition()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("{inputValue: Int",
			mustPanic(mustParseInputFieldsDefinition()),
		)
	})

	// parseInputObjectTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run(`input Person {
					name: String
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFieldsDefinition(
						hasInputValueDefinitions(
							node(
								hasName("name"),
							),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   3,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`input Person {
					name: [String]!
					age: [ Int ]
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFieldsDefinition(
						hasInputValueDefinitions(
							node(hasName("name")),
							node(hasName("age")),
						),
					),
				),
			),
		)
	})
	t.Run("with default value", func(t *testing.T) {
		run(`input Person {
					name: String = "Gophina"
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFieldsDefinition(
						hasInputValueDefinitions(
							node(
								hasName("name"),
								nodeType(
									hasTypeKind(document.TypeKindNAMED),
									hasTypeName("String"),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run("input Person", mustParseInputObjectTypeDefinition(
			node(
				hasName("Person"),
			),
		))
	})
	t.Run("complex", func(t *testing.T) {
		run(`input Person @fromTop(to: "bottom") @fromBottom(to: "top"){
					name: String
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasInputFieldsDefinition(
						hasInputValueDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("input 1337 {}",
			mustPanic(
				mustParseInputObjectTypeDefinition(
					node(
						hasName("1337"),
					),
				)),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("input Person @foo(bar: .) {}",
			mustPanic(
				mustParseInputObjectTypeDefinition(
					node(
						hasName("1337"),
					),
				)),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("input Person { a: .}",
			mustPanic(
				mustParseInputObjectTypeDefinition(
					node(
						hasName("1337"),
					),
				)),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run("notinput Foo {}",
			mustPanic(
				mustParseInputObjectTypeDefinition(
					node(
						hasName("1337"),
					),
				)),
		)
	})

	// parseInputValueDefinitions

	t.Run("simple", func(t *testing.T) {
		run("inputValue: Int",
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   16,
					}),
				),
			),
		)
	})
	t.Run("with default", func(t *testing.T) {
		run("inputValue: Int = 2",
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
				),
			),
		)
	})
	t.Run("with description", func(t *testing.T) {
		run(`"useful description"inputValue: Int = 2`,
			mustParseInputValueDefinitions(
				node(
					hasDescription("useful description"),
					hasName("inputValue"),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   40,
					}),
				),
			),
		)
	})
	t.Run("multiple with descriptions and defaults", func(t *testing.T) {
		run(`"this is a inputValue"inputValue: Int = 2, "this is a outputValue"outputValue: String = "Out"`,
			mustParseInputValueDefinitions(
				node(
					hasDescription("this is a inputValue"),
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   42,
					}),
				),
				node(
					hasDescription("this is a outputValue"),
					hasName("outputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 44,
						LineEnd:   1,
						CharEnd:   94,
					}),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`inputValue: Int @fromTop(to: "bottom") @fromBottom(to: "top")`,
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   62,
					}),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("inputValue. foo",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("inputValue: foo @bar(baz: .)",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("inputValue: foo = [1!",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})

	// parseInterfaceTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run(`interface NameEntity {
					name: String
				}`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("NameEntity"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   3,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`interface Person {
					name: [String]!
					age: [ Int ]
				}`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("Person"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
						node(
							hasName("age"),
						),
					),
				),
			),
		)
	})
	t.Run("optional", func(t *testing.T) {
		run(`interface Person`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("Person"),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`interface NameEntity @fromTop(to: "bottom") @fromBottom(to: "top") {
					name: String
				}`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("NameEntity"),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`interface 1337 {
					name: String
				}`,
			mustPanic(
				mustParseInterfaceTypeDefinition(
					node(
						hasName("1337"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`interface Person @foo(bar: .) {
					name: String
				}`,
			mustPanic(
				mustParseInterfaceTypeDefinition(
					node(
						hasName("Person"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`interface Person {
					name: [String!
				}`,
			mustPanic(
				mustParseInterfaceTypeDefinition(
					node(
						hasName("Person"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run(`notinterface Person {
					name: [String!]
				}`,
			mustPanic(
				mustParseInterfaceTypeDefinition(),
			),
		)
	})

	// parseObjectTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run(`type Person {
					name: String
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   3,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`type Person {
					name: [String]!
					age: [ Int ]
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
						node(
							hasName("age"),
						),
					),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run(`type Person`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
				),
			),
		)
	})
	t.Run("implements interface", func(t *testing.T) {
		run(`type Person implements Human {
					name: String
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
					hasImplementsInterfaces("Human"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("implements multiple interfaces", func(t *testing.T) {
		run(`type Person implements Human & Mammal {
					name: String
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
					hasImplementsInterfaces("Human", "Mammal"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`type Person @fromTop(to: "bottom") @fromBottom(to: "top") {
					name: String
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`type 1337 {
					name: String
				}`,
			mustPanic(
				mustParseObjectTypeDefinition(
					node(
						hasName("1337"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`type Person implements 1337 {
					name: String
				}`,
			mustPanic(
				mustParseObjectTypeDefinition(
					node(
						hasName("Person"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`type Person @foo(bar: .) {
					name: String
				}`,
			mustPanic(
				mustParseObjectTypeDefinition(
					node(
						hasName("Person"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run(`type Person {
					name: [String!
				}`,
			mustPanic(
				mustParseObjectTypeDefinition(
					node(
						hasName("Person"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 5", func(t *testing.T) {
		run(`nottype Person {
					name: [String!]
				}`,
			mustPanic(mustParseObjectTypeDefinition()),
		)
	})

	// parseOperationDefinition

	t.Run("simple", func(t *testing.T) {
		run(`query allGophers($color: String)@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasVariableDefinitions(
						node(
							hasName("color"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   3,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("without directive", func(t *testing.T) {
		run(` query allGophers($color: String) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasVariableDefinitions(
						node(
							hasName("color"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("without variables", func(t *testing.T) {
		run(`query allGophers@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("unnamed", func(t *testing.T) {
		run(`query ($color: String)@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run(`{
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasFields(
						node(
							hasName("name"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   3,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("invalid ", func(t *testing.T) {
		run(` query allGophers($color: [String!) {
					name
				}`,
			mustPanic(
				mustParseOperationDefinition(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
							),
						),
						hasFields(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid ", func(t *testing.T) {
		run(` query allGophers($color: String!) @foo(bar: .) {
					name
				}`,
			mustPanic(
				mustParseOperationDefinition(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
							),
						),
						hasFields(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})

	// parseScalarTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run("scalar JSON", mustParseScalarTypeDefinition(
			node(
				hasName("JSON"),
			),
		))
	})
	t.Run("with directives", func(t *testing.T) {
		run(`scalar JSON @fromTop(to: "bottom") @fromBottom(to: "top")`, mustParseScalarTypeDefinition(
			node(
				hasName("JSON"),
				hasDirectives(
					node(
						hasName("fromTop"),
					),
					node(
						hasName("fromBottom"),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   58,
				}),
			),
		))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("scalar 1337",
			mustPanic(
				mustParseScalarTypeDefinition(
					node(
						hasName("1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("scalar JSON @foo(bar: .)",
			mustPanic(
				mustParseScalarTypeDefinition(
					node(
						hasName("1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("notscalar JSON",
			mustPanic(
				mustParseScalarTypeDefinition(),
			),
		)
	})

	// parseSchemaDefinition

	t.Run("simple", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
						subscription: Subscription 
					}`,
			mustParseSchemaDefinition(
				hasSchemaOperationTypeName(document.OperationTypeQuery, "Query"),
				hasSchemaOperationTypeName(document.OperationTypeMutation, "Mutation"),
				hasSchemaOperationTypeName(document.OperationTypeSubscription, "Subscription"),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`	schema {
						query : Query	
						mutation : Mutation
						subscription : Subscription
						query: Query2 
					}`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`	schema @fromTop(to: "bottom") @fromBottom(to: "top") {
						query: Query
						mutation: Mutation
						subscription: Subscription
					}`,
			mustParseSchemaDefinition(
				hasSchemaOperationTypeName(document.OperationTypeQuery, "Query"),
				hasSchemaOperationTypeName(document.OperationTypeMutation, "Mutation"),
				hasSchemaOperationTypeName(document.OperationTypeSubscription, "Subscription"),
				hasDirectives(
					node(
						hasName("fromTop"),
					),
					node(
						hasName("fromBottom"),
					),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`schema  @foo(bar: .) { query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`schema ( query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`schema { query. Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run(`schema { query: 1337 }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 5", func(t *testing.T) {
		run(`schema { query: Query )`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 6", func(t *testing.T) {
		run(`notschema { query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})

	// parseSelectionSet

	t.Run("simple", func(t *testing.T) {
		run(`{ foo }`, mustParseSelectionSet(
			node(
				hasFields(
					node(
						hasName("foo"),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   8,
				}),
			),
		))
	})
	t.Run("inline and fragment spreads", func(t *testing.T) {
		run(`{
					... on Goland
					...Air
					... on Water
				}`,
			mustParseSelectionSet(
				node(
					hasInlineFragments(
						node(
							hasTypeName("Goland"),
						),
						node(
							hasTypeName("Water"),
						),
					),
					hasFragmentSpreads(
						node(
							hasName("Air"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   5,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("mixed", func(t *testing.T) {
		run(`{
					... on Goland
					preferredName: originalName(isSet: true)
					... on Water
				}`, mustParseSelectionSet(
			node(
				hasFields(
					node(
						hasAlias("preferredName"),
						hasName("originalName"),
						hasArguments(
							node(
								hasName("isSet"),
							),
						),
					),
				),
				hasInlineFragments(
					node(
						hasTypeName("Goland"),
					),
					node(
						hasTypeName("Water"),
					),
				),
			),
		))
	})
	t.Run("field with directives", func(t *testing.T) {
		run(`{
					preferredName: originalName(isSet: true) @rename(index: 3)
				}`, mustParseSelectionSet(
			node(
				hasFields(
					node(
						hasAlias("preferredName"),
						hasName("originalName"),
						hasArguments(
							node(
								hasName("isSet"),
							),
						),
						hasDirectives(
							node(
								hasName("rename"),
							),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   3,
					CharEnd:   6,
				}),
			),
		))
	})
	t.Run("fragment with directive", func(t *testing.T) {
		run(`{
					...firstFragment @rename(index: 3)
				}`, mustParseSelectionSet(
			node(
				hasFragmentSpreads(
					node(
						hasName("firstFragment"),
						hasDirectives(
							node(
								hasName("rename"),
							),
						),
					),
				),
			),
		))
	})
	t.Run("invalid", func(t *testing.T) {
		run(`{
					...firstFragment @rename(index: .)
				}`,
			mustPanic(
				mustParseSelectionSet(
					node(
						hasFragmentSpreads(
							node(
								hasName("firstFragment"),
								hasDirectives(
									node(
										hasName("rename"),
									),
								),
							),
						),
					),
				),
			),
		)
	})

	// parseTypeSystemDefinition

	t.Run("unions", func(t *testing.T) {
		run(`
				"unifies SearchResult"
				union SearchResult = Photo | Person
				union thirdUnion 
				"second union"
				union secondUnion
				union firstUnion @fromTop(to: "bottom")
				"unifies UnionExample"
				union UnionExample = First | Second
				`,
			mustParseTypeSystemDefinition(
				node(
					hasUnionTypeSystemDefinitions(
						node(
							hasName("SearchResult"),
							hasPosition(position.Position{
								LineStart: 2,
								CharStart: 5,
								LineEnd:   3,
								CharEnd:   40,
							}),
						),
						node(
							hasName("thirdUnion"),
						),
						node(
							hasName("secondUnion"),
						),
						node(
							hasName("firstUnion"),
						),
						node(
							hasName("UnionExample"),
							hasPosition(position.Position{
								LineStart: 8,
								CharStart: 5,
								LineEnd:   9,
								CharEnd:   40,
							}),
						),
					),
				),
			),
		)
	})
	t.Run("schema", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
					}
					
					"this is a scalar"
					scalar JSON

					"this is a Person"
					type Person {
						name: String
					}


					"describes firstEntity"
					interface firstEntity {
						name: String
					}

					"describes direction"
					enum Direction {
						NORTH
					}

					"describes Person"
					input Person {
						name: String
					}

					"describes someway"
					directive @ someway on SUBSCRIPTION | MUTATION`,
			mustParseTypeSystemDefinition(
				node(
					hasSchemaDefinition(
						hasPosition(position.Position{
							LineStart: 1,
							CharStart: 2,
							LineEnd:   4,
							CharEnd:   7,
						}),
					),
					hasScalarTypeSystemDefinitions(
						node(
							hasName("JSON"),
							hasPosition(position.Position{
								LineStart: 6,
								CharStart: 6,
								LineEnd:   7,
								CharEnd:   17,
							}),
						),
					),
					hasObjectTypeSystemDefinitions(
						node(
							hasName("Person"),
							hasPosition(position.Position{
								LineStart: 9,
								CharStart: 6,
								LineEnd:   12,
								CharEnd:   7,
							}),
						),
					),
					hasInterfaceTypeSystemDefinitions(
						node(
							hasName("firstEntity"),
							hasPosition(position.Position{
								LineStart: 15,
								CharStart: 6,
								LineEnd:   18,
								CharEnd:   7,
							}),
						),
					),
					hasEnumTypeSystemDefinitions(
						node(
							hasName("Direction"),
							hasPosition(position.Position{
								LineStart: 20,
								CharStart: 6,
								LineEnd:   23,
								CharEnd:   7,
							}),
						),
					),
					hasInputObjectTypeSystemDefinitions(
						node(
							hasName("Person"),
							hasPosition(position.Position{
								LineStart: 25,
								CharStart: 6,
								LineEnd:   28,
								CharEnd:   7,
							}),
						),
					),
					hasDirectiveDefinitions(
						node(
							hasName("someway"),
							hasPosition(position.Position{
								LineStart: 30,
								CharStart: 6,
								LineEnd:   31,
								CharEnd:   52,
							}),
						),
					),
				),
			))
	})
	t.Run("set schema multiple times", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
					}

					schema {
						query: Query
						mutation: Mutation
					}`,
			mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid schema", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
					)`,
			mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid scalar", func(t *testing.T) {
		run(`scalar JSON @foo(bar: .)`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid object type definition", func(t *testing.T) {
		run(`type Foo implements Bar { foo(bar: .): Baz}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid interface type definition", func(t *testing.T) {
		run(`interface Bar { baz: [Bal!}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid union type definition", func(t *testing.T) {
		run(`union Foo = Bar | Baz | 1337`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid union type definition 2", func(t *testing.T) {
		run(`union Foo = Bar | Baz | "Bal"`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid enum type definition", func(t *testing.T) {
		run(`enum Foo { Bar @baz(bal: .)}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid input object type definition", func(t *testing.T) {
		run(`input Foo { foo(bar: .): Baz}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid directive definition", func(t *testing.T) {
		run(`directive @ foo ON InvalidLocation`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid directive definition 2", func(t *testing.T) {
		run(`directive @ foo(bar: .) ON QUERY`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid keyword", func(t *testing.T) {
		run(`unknown {}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})

	// parseUnionTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run("union SearchResult = Photo | Person",
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person"),
				),
			))
	})
	t.Run("multiple members", func(t *testing.T) {
		run("union SearchResult = Photo | Person | Car | Planet",
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person", "Car", "Planet"),
				),
			),
		)
	})
	t.Run("with linebreaks", func(t *testing.T) {
		run(`union SearchResult = Photo 
										| Person 
										| Car 
										| Planet`,
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person", "Car", "Planet"),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`union SearchResult @fromTop(to: "bottom") @fromBottom(to: "top") = Photo | Person`,
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasUnionMemberTypes("Photo", "Person"),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("union 1337 = Photo | Person",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("1337"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("union SearchResult @foo(bar: .) = Photo | Person",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("union SearchResult = Photo | Person | 1337",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person", "1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run("union SearchResult = Photo | Person | \"Video\"",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 5", func(t *testing.T) {
		run("notunion SearchResult = Photo | Person",
			mustPanic(mustParseUnionTypeDefinition()),
		)
	})

	// parseVariableDefinitions

	t.Run("simple", func(t *testing.T) {
		run("($foo : bar)",
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeName("bar"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 2,
						LineEnd:   1,
						CharEnd:   12,
					}),
				),
			),
		)
	})
	t.Run("multiple", func(t *testing.T) {
		run("($foo : bar $baz : bat)",
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeName("bar"),
					),
				),
				node(
					hasName("baz"),
					nodeType(
						hasTypeName("bat"),
					),
				),
			),
		)
	})
	t.Run("with default", func(t *testing.T) {
		run(`($foo : bar! = "me" $baz : bat)`,
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeKind(document.TypeKindNON_NULL),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("bar"),
						),
					),
					hasDefaultValue(
						hasValueType(document.ValueTypeString),
						hasByteSliceValue("me"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 2,
						LineEnd:   1,
						CharEnd:   20,
					}),
				),
				node(
					hasName("baz"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("bat"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 21,
						LineEnd:   1,
						CharEnd:   31,
					}),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("($foo : bar!",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("($foo . bar!)",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("($foo : bar! = . )",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 4", func(t *testing.T) {
		run("($foo : bar! = \"Baz! )",
			mustPanic(mustParseVariableDefinitions()))
	})

	// parseValue

	t.Run("int", func(t *testing.T) {
		run("1337", mustParseValue(
			document.ValueTypeInt,
			expectIntegerValue(1337),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
		))
	})
	t.Run("string", func(t *testing.T) {
		run(`"foo"`, mustParseValue(
			document.ValueTypeString,
			expectByteSliceValue("foo"),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("list", func(t *testing.T) {
		run("[1,3,3,7]", mustParseValue(
			document.ValueTypeList,
			expectListValue(
				expectIntegerValue(1),
				expectIntegerValue(3),
				expectIntegerValue(3),
				expectIntegerValue(7),
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   10,
			}),
		))
	})
	t.Run("mixed list", func(t *testing.T) {
		run(`[ 1	,"2" 3,,[	1	], { foo: 1337 } ]`,
			mustParseValue(
				document.ValueTypeList,
				expectListValue(
					expectIntegerValue(1),
					expectByteSliceValue("2"),
					expectIntegerValue(3),
					expectListValue(
						expectIntegerValue(1),
					),
					expectObjectValue(
						node(
							hasName("foo"),
							expectIntegerValue(1337),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					LineEnd:   1,
					CharStart: 1,
					CharEnd:   35,
				}),
			))
	})
	t.Run("object", func(t *testing.T) {
		run(`{foo: "bar"}`,
			mustParseValue(document.ValueTypeObject,
				expectObjectValue(
					node(
						hasName("foo"),
						expectByteSliceValue("bar"),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					LineEnd:   1,
					CharStart: 1,
					CharEnd:   13,
				}),
			))
	})
	t.Run("invalid object", func(t *testing.T) {
		run(`{foo. "bar"}`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid object 2", func(t *testing.T) {
		run(`{foo: [String!}`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid object 3", func(t *testing.T) {
		run(`{foo: "bar" )`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("nested object", func(t *testing.T) {
		run(`{foo: {bar: "baz"}, someEnum: SOME_ENUM }`, mustParseValue(document.ValueTypeObject,
			expectObjectValue(
				node(
					hasName("foo"),
					expectObjectValue(
						node(
							hasName("bar"),
							expectByteSliceValue("baz"),
						),
					),
				),
				node(
					hasName("someEnum"),
					expectByteSliceValue("SOME_ENUM"),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   42,
			}),
		))
	})
	t.Run("variable", func(t *testing.T) {
		run("$1337", mustParseValue(
			document.ValueTypeVariable,
			expectByteSliceValue("1337"),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("variable 2", func(t *testing.T) {
		run("$foo", mustParseValue(document.ValueTypeVariable, expectByteSliceValue("foo")))
	})
	t.Run("variable 3", func(t *testing.T) {
		run("$_foo", mustParseValue(document.ValueTypeVariable, expectByteSliceValue("_foo")))
	})
	t.Run("invalid variable", func(t *testing.T) {
		run("$ foo", mustPanic(mustParseValue(document.ValueTypeVariable, expectByteSliceValue(" foo"))))
	})
	t.Run("float", func(t *testing.T) {
		run("13.37", mustParseValue(
			document.ValueTypeFloat,
			expectFloatValue(13.37),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("invalid float", func(t *testing.T) {
		run("1.3.3.7", mustPanic(mustParseValue(document.ValueTypeFloat, expectFloatValue(13.37))))
	})
	t.Run("boolean", func(t *testing.T) {
		run("true", mustParseValue(
			document.ValueTypeBoolean,
			expectBooleanValue(true),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
		))
	})
	t.Run("boolean 2", func(t *testing.T) {
		run("false", mustParseValue(document.ValueTypeBoolean, expectBooleanValue(false)))
	})
	t.Run("string", func(t *testing.T) {
		run(`"foo"`, mustParseValue(document.ValueTypeString, expectByteSliceValue("foo")))
	})
	t.Run("string 2", func(t *testing.T) {
		run(`"""foo"""`, mustParseValue(document.ValueTypeString, expectByteSliceValue("foo")))
	})
	t.Run("null", func(t *testing.T) {
		run("null", mustParseValue(
			document.ValueTypeNull,
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
		))
	})

	// parseTypes

	t.Run("simple named", func(t *testing.T) {
		run("String", mustParseType(
			hasTypeKind(document.TypeKindNAMED),
			hasTypeName("String"),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   7,
			}),
		))
	})
	t.Run("named non null", func(t *testing.T) {
		run("String!", mustParseType(
			hasTypeKind(document.TypeKindNON_NULL),
			ofType(
				hasTypeKind(document.TypeKindNAMED),
				hasTypeName("String"),
			),
		))
	})
	t.Run("non null named list", func(t *testing.T) {
		run("[String!]", mustParseType(
			hasTypeKind(document.TypeKindLIST),
			ofType(
				hasTypeKind(document.TypeKindNON_NULL),
				ofType(
					hasTypeKind(document.TypeKindNAMED),
					hasTypeName("String"),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   10,
			}),
		))
	})
	t.Run("non null named non null list", func(t *testing.T) {
		run("[String!]!", mustParseType(
			hasTypeKind(document.TypeKindNON_NULL),
			ofType(
				hasTypeKind(document.TypeKindLIST),
				ofType(
					hasTypeKind(document.TypeKindNON_NULL),
					ofType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
				),
			),
		))
	})
	t.Run("nested list", func(t *testing.T) {
		run("[[[String]!]]", mustParseType(
			hasTypeKind(document.TypeKindLIST),
			ofType(
				hasTypeKind(document.TypeKindLIST),
				ofType(
					hasTypeKind(document.TypeKindNON_NULL),
					ofType(
						hasTypeKind(document.TypeKindLIST),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
							hasPosition(position.Position{
								LineStart: 1,
								CharStart: 4,
								LineEnd:   1,
								CharEnd:   10,
							}),
						),
					),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   14,
			}),
		))
	})
	t.Run("invalid", func(t *testing.T) {
		run("[\"String\"]",
			mustPanic(
				mustParseType(
					hasTypeKind(document.TypeKindLIST),
					ofType(
						hasTypeKind(document.TypeKindNON_NULL),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				),
			),
		)
	})

	// parsePeekedFloatValue
	t.Run("valid float", func(t *testing.T) {
		run("", mustParseFloatValue(t, "13.37", 13.37))
	})
	t.Run("invalid float", func(t *testing.T) {
		run("1.3.3.7", mustPanic(mustParseFloatValue(t, "1.3.3.7", 13.37)))
	})

	// newErrInvalidType

	t.Run("newErrInvalidType", func(t *testing.T) {
		want := "parser:a:invalidType - expected 'b', got 'c' @ 1:3-2:4"
		got := newErrInvalidType(position.Position{1, 2, 3, 4}, "a", "b", "c").Error()

		if want != got {
			t.Fatalf("newErrInvalidType: \nwant: %s\ngot: %s", want, got)
		}
	})
}

func TestParser_ParseExecutableDefinition(t *testing.T) {
	parser := NewParser()
	input := make([]byte, 65536)
	err := parser.ParseTypeSystemDefinition(input)
	if err == nil {
		t.Fatal("want err, got nil")
	}

	parser = NewParser()

	err = parser.ParseExecutableDefinition(input)
	if err == nil {
		t.Fatal("want err, got nil")
	}
}

func TestParser_Starwars(t *testing.T) {

	inputFileName := "../../starwars.schema.graphql"
	fixtureFileName := "type_system_definition_parsed_starwars"

	parser := NewParser(WithPoolSize(2), WithMinimumSliceSize(2))

	starwarsSchema, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	err = parser.ParseTypeSystemDefinition(starwarsSchema)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(parser.ParsedDefinitions.TypeSystemDefinition, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, fixtureFileName, jsonBytes)
	if t.Failed() {

		fixtureData, err := ioutil.ReadFile(fmt.Sprintf("./fixtures/%s.golden", fixtureFileName))
		if err != nil {
			log.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes(fixtureFileName, fixtureData, jsonBytes)
	}
}

func TestParser_IntrospectionQuery(t *testing.T) {

	inputFileName := "./testdata/introspectionquery.graphql"
	fixtureFileName := "type_system_definition_parsed_introspection"

	inputFileData, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	parser := NewParser()
	err = parser.ParseExecutableDefinition(inputFileData)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(parser.ParsedDefinitions.ExecutableDefinition, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes = append(jsonBytes, []byte("\n\n")...)

	parserData, err := json.MarshalIndent(parser, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes = append(jsonBytes, parserData...)

	goldie.Assert(t, fixtureFileName, jsonBytes)
	if t.Failed() {

		fixtureData, err := ioutil.ReadFile(fmt.Sprintf("./fixtures/%s.golden", fixtureFileName))
		if err != nil {
			log.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes(fixtureFileName, fixtureData, jsonBytes)
	}
}

func BenchmarkParser(b *testing.B) {

	b.ReportAllocs()

	parser := NewParser()

	testData, err := ioutil.ReadFile("./testdata/introspectionquery.graphql")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		err := parser.ParseExecutableDefinition(testData)
		if err != nil {
			b.Fatal(err)
		}

	}

}
