package parser

import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"log"
	"testing"
)

type rule func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int)
type ruleSet []rule

func (r ruleSet) eval(node document.Node, definitions ParsedDefinitions, ruleIndex int) {
	for i, rule := range r {
		rule(node, definitions, ruleIndex, i)
	}
}

func TestParser(t *testing.T) {

	type checkFunc func(parser *Parser, i int)

	run := func(input string, checks ...checkFunc) {
		parser := NewParser()
		parser.l.SetInput([]byte(input))
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

	evalRules := func(node document.Node, definitions ParsedDefinitions, rules ruleSet, ruleIndex int) {
		rules.eval(node, definitions, ruleIndex)
	}

	hasName := func(name string) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			if name != node.NodeName() {
				panic(fmt.Errorf("hasName: want: %s, got: %s [rule: %d, node: %d]", name, node.NodeName(), ruleIndex, ruleSetIndex))
			}
		}
	}

	hasAlias := func(alias string) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			if alias != node.NodeAlias() {
				panic(fmt.Errorf("hasAlias: want: %s, got: %s [rule: %d, node: %d]", alias, node.NodeAlias(), ruleIndex, ruleSetIndex))
			}
		}
	}

	hasDescription := func(description string) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			if description != node.NodeDescription() {
				panic(fmt.Errorf("hasName: want: %s, got: %s [rule: %d, node: %d]", description, node.NodeDescription(), ruleIndex, ruleSetIndex))
			}
		}
	}

	unwrapObjectField := func(node document.Node, definitions ParsedDefinitions) document.Node {
		objectField, ok := node.(document.ObjectField)
		if ok {
			node = definitions.Values[objectField.Value]
		}
		return node
	}

	expectIntegerValue := func(want int32) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, definitions)
			got := definitions.Integers[node.NodeValueReference()]
			if want != got {
				panic(fmt.Errorf("expectIntegerValue: want: %d, got: %d [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}

	expectFloatValue := func(want float32) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, definitions)
			got := definitions.Floats[node.NodeValueReference()]
			if want != got {
				panic(fmt.Errorf("expectIntegerValue: want: %.2f, got: %.2f [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}

	expectBooleanValue := func(want bool) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, definitions)
			got := definitions.Booleans[node.NodeValueReference()]
			if want != got {
				panic(fmt.Errorf("expectIntegerValue: want: %v, got: %v [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}

	expectByteSliceValue := func(want string) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, definitions)
			got := string(definitions.ByteSlices[node.NodeValueReference()])
			if want != got {
				panic(fmt.Errorf("expectByteSliceValue: want: %s, got: %s [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}

	expectListValue := func(rules ...rule) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			list := definitions.ListValues[node.NodeValueReference()]
			for j, rule := range rules {
				valueIndex := list[j]
				value := definitions.Values[valueIndex]
				rule(value, definitions, j, ruleSetIndex)
			}
		}
	}

	expectObjectValue := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			node = unwrapObjectField(node, definitions)
			list := definitions.ObjectValues[node.NodeValueReference()]
			for j, rule := range rules {
				valueIndex := list[j]
				value := definitions.ObjectFields[valueIndex]
				rule.eval(value, definitions, j)
			}
		}
	}

	hasOperationType := func(operationType document.OperationType) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			gotOperationType := node.NodeOperationType().String()
			wantOperationType := operationType.String()
			if wantOperationType != gotOperationType {
				panic(fmt.Errorf("hasOperationType: want: %s, got: %s [rule: %d, node: %d]", wantOperationType, gotOperationType, ruleIndex, ruleSetIndex))
			}
		}
	}

	hasTypeKind := func(wantTypeKind document.TypeKind) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			gotTypeKind := node.(document.Type).Kind
			if wantTypeKind != gotTypeKind {
				panic(fmt.Errorf("hasTypeKind: want(typeKind): %s, got: %s [rule: %d, node: %d]", wantTypeKind, gotTypeKind, ruleIndex, ruleSetIndex))
			}
		}
	}

	nodeType := func(rules ...rule) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			nodeType := definitions.Types[node.NodeType()]
			for j, rule := range rules {
				rule(nodeType, definitions, j, ruleSetIndex)
			}
		}
	}

	ofType := func(rules ...rule) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {
			ofType := definitions.Types[node.(document.Type).OfType]
			for j, rule := range rules {
				rule(ofType, definitions, j, ruleSetIndex)
			}
		}
	}

	hasTypeName := func(wantName string) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			if fragment, ok := node.(document.FragmentDefinition); ok {
				node = definitions.Types[fragment.TypeCondition]
			}

			if inlineFragment, ok := node.(document.InlineFragment); ok {
				node = definitions.Types[inlineFragment.TypeCondition]
			}

			gotName := string(node.(document.Type).Name)
			if wantName != gotName {
				panic(fmt.Errorf("hasTypeName: want: %s, got: %s [rule: %d, node: %d]", wantName, gotName, ruleIndex, ruleSetIndex))
			}
		}
	}

	/*	hasDefaultValue := func(want document.Value) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			got := node.NodeDefaultValue()

			if reflect.DeepEqual(want, got) {
				panic(fmt.Errorf("hasDefaultValue: want: %+v, got: %+v [rule: %d, node: %d]", want, got, ruleIndex, ruleSetIndex))
			}
		}
	}*/

	hasEnumValuesDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeEnumValuesDefinition()

			for j, k := range index {
				ruleSet := rules[j]
				subNode := definitions.EnumValuesDefinitions[k]
				ruleSet.eval(subNode, definitions, k)
			}
		}
	}

	hasUnionTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeUnionTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := definitions.UnionTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, definitions, j)
			}
		}
	}

	hasScalarTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeScalarTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := definitions.ScalarTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, definitions, j)
			}
		}
	}

	hasObjectTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeObjectTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := definitions.ObjectTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, definitions, j)
			}
		}
	}

	hasInterfaceTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeInterfaceTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := definitions.InterfaceTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, definitions, j)
			}
		}
	}

	hasEnumTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeEnumTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := definitions.EnumTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, definitions, j)
			}
		}
	}

	hasInputObjectTypeSystemDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeInputObjectTypeDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := definitions.InputObjectTypeDefinitions[definitionIndex]
				ruleSet.eval(subNode, definitions, j)
			}
		}
	}

	hasDirectiveDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeDirectiveDefinitions()

			for j, ruleSet := range rules {
				definitionIndex := typeDefinitionIndex[j]
				subNode := definitions.DirectiveDefinitions[definitionIndex]
				ruleSet.eval(subNode, definitions, j)
			}
		}
	}

	hasUnionMemberTypes := func(members ...string) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			typeDefinitionIndex := node.NodeUnionMemberTypes()

			for j, want := range members {
				got := string(typeDefinitionIndex[j])
				if want != got {
					panic(fmt.Errorf("hasUnionMemberTypes: want: %s, got: %s [check: %d]", want, got, ruleSetIndex))
				}
			}
		}
	}

	hasSchemaDefinition := func() rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			schemaDefinition := node.NodeSchemaDefinition()
			if !schemaDefinition.IsDefined() {
				panic(fmt.Errorf("hasSchemaDefinition: schemaDefinition is undefined [check: %d]", ruleSetIndex))
			}
		}
	}

	hasVariableDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeVariableDefinitions()

			for j, k := range index {
				ruleSet := rules[j]
				subNode := definitions.VariableDefinitions[k]
				ruleSet.eval(subNode, definitions, k)
			}
		}
	}

	hasDirectives := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeDirectives()

			for i := range rules {
				ruleSet := rules[i]
				subNode := definitions.Directives[index[i]]
				ruleSet.eval(subNode, definitions, index[i])
			}
		}
	}

	hasImplementsInterfaces := func(interfaces ...string) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			actual := node.NodeImplementsInterfaces()
			for i, want := range interfaces {
				got := string(actual[i])

				if want != got {
					panic(fmt.Errorf("hasImplementsInterfaces: want(at: %d): %s, got: %s [check: %d]", i, want, got, ruleSetIndex))
				}
			}
		}
	}

	hasFields := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeFields()

			for i := range rules {
				ruleSet := rules[i]
				subNode := definitions.Fields[index[i]]
				ruleSet.eval(subNode, definitions, index[i])
			}
		}
	}

	hasFieldsDefinitions := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeFieldsDefinition()

			for i := range rules {
				ruleSet := rules[i]
				field := definitions.FieldDefinitions[index[i]]
				ruleSet.eval(field, definitions, index[i])
			}
		}
	}

	hasInputFields := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeFields()

			for i := range rules {
				ruleSet := rules[i]
				subNode := definitions.InputValueDefinitions[index[i]]
				ruleSet.eval(subNode, definitions, index[i])
			}
		}
	}

	hasArguments := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeArguments()

			for i := range rules {
				ruleSet := rules[i]
				subNode := definitions.Arguments[index[i]]
				ruleSet.eval(subNode, definitions, index[i])
			}
		}
	}

	hasInlineFragments := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeInlineFragments()

			for i := range rules {
				ruleSet := rules[i]
				subNode := definitions.InlineFragments[index[i]]
				ruleSet.eval(subNode, definitions, index[i])
			}
		}
	}

	hasFragmentSpreads := func(rules ...ruleSet) rule {
		return func(node document.Node, definitions ParsedDefinitions, ruleIndex, ruleSetIndex int) {

			index := node.NodeFragmentSpreads()

			for i := range rules {
				ruleSet := rules[i]
				subNode := definitions.FragmentSpreads[index[i]]
				ruleSet.eval(subNode, definitions, index[i])
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

	mustParseArguments := func(argumentNames ...string) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseArguments(&index); err != nil {
				panic(err)
			}

			for k, want := range argumentNames {
				got := string(parser.ParsedDefinitions.Arguments[k].Name)
				if want != got {
					panic(fmt.Errorf("mustParseArguments: want(i: %d): %s, got: %s [check: %d]", k, want, got, i))
				}
			}
		}
	}

	mustParseArgumentDefinitions := func(argumentNames ...string) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseArgumentsDefinition(&index); err != nil {
				panic(err)
			}

			for k, want := range argumentNames {
				got := string(parser.ParsedDefinitions.InputValueDefinitions[k].Name)
				if want != got {
					panic(fmt.Errorf("mustParseArguments: want(i: %d): %s, got: %s [check: %d]", k, want, got, i))
				}
			}
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

	mustParseDirectiveDefinition := func(name string, locations ...document.DirectiveLocation) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseDirectiveDefinition(&index); err != nil {
				panic(err)
			}

			got := parser.ParsedDefinitions.DirectiveDefinitions[0]
			if string(got.Name) != name {
				panic(fmt.Errorf("mustParseDirectiveDefinition: want(name): %s, got: %s", name, got.Name))
			}

			for k, wantLocation := range locations {
				gotLocation := got.DirectiveLocations[k]
				if wantLocation != gotLocation {
					panic(fmt.Errorf("mustParseDirectiveDefinition: want(location: %d): %s, got: %s", k, wantLocation.String(), gotLocation.String()))
				}
			}
		}
	}

	mustContainInputValueDefinition := func(index int, wantName string) checkFunc {
		return func(parser *Parser, i int) {
			gotName := string(parser.ParsedDefinitions.InputValueDefinitions[index].Name)
			if wantName != gotName {
				panic(fmt.Errorf("mustContainInputValueDefinition: want for index %d: %s,got: %s", index, wantName, gotName))
			}
		}
	}

	mustContainArguments := func(name ...string) checkFunc {
		return func(parser *Parser, i int) {

			for k, wantName := range name {
				gotName := string(parser.ParsedDefinitions.Arguments[k].Name)
				if wantName != gotName {
					panic(fmt.Errorf("mustContainArguments: want for index %d: %s,got: %s", k, wantName, gotName))
				}
			}
		}
	}

	mustParseDirectives := func(name ...string) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseDirectives(&index); err != nil {
				panic(err)
			}

			for i, k := range index {
				wantName := name[i]
				gotName := string(parser.ParsedDefinitions.Directives[k].Name)
				if gotName != wantName {
					panic(fmt.Errorf("mustParseDirectives: want: %s,got: %s [check: %d]", wantName, gotName, i))
				}
			}
		}
	}

	mustParseEnumTypeDefinition := func(rules ...rule) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseEnumTypeDefinition(&index); err != nil {
				panic(err)
			}

			enum := parser.ParsedDefinitions.EnumTypeDefinitions[0]
			evalRules(enum, parser.ParsedDefinitions, rules, i)
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
				set.eval(fragment, parser.ParsedDefinitions, i)
			}

			for i, set := range operations {
				opIndex := definition.OperationDefinitions[i]
				operation := parser.ParsedDefinitions.OperationDefinitions[opIndex]
				set.eval(operation, parser.ParsedDefinitions, i)
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
				evalRules(field, parser.ParsedDefinitions, rule, i)
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
				evalRules(field, parser.ParsedDefinitions, rule, i)
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
				evalRules(fragmentDefinition, parser.ParsedDefinitions, rule, i)
			}
		}
	}

	mustParseFragmentSpread := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseFragmentSpread(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				spread := parser.ParsedDefinitions.FragmentSpreads[j]
				evalRules(spread, parser.ParsedDefinitions, rule, i)
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
				got := string(interfaces[j])
				if want != got {
					panic(fmt.Errorf("mustParseImplementsInterfaces: want: %s, got: %s [check: %d]", want, got, i))
				}
			}
		}
	}

	mustParseLiteral := func(wantKeyword keyword.Keyword, wantLiteral string) checkFunc {
		return func(parser *Parser, i int) {
			next, err := parser.l.Read()
			if err != nil {
				panic(err)
			}

			gotKeyword := next.Keyword
			gotLiteral := string(next.Literal)

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
			if err := parser.parseInlineFragment(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				reverseIndex := len(parser.ParsedDefinitions.InlineFragments) - 1 - j
				inlineFragment := parser.ParsedDefinitions.InlineFragments[reverseIndex]
				evalRules(inlineFragment, parser.ParsedDefinitions, rule, i)
			}
		}
	}

	mustParseInputFieldsDefinition := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseInputFieldsDefinition(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				inputValueDefinition := parser.ParsedDefinitions.InputValueDefinitions[j]
				evalRules(inputValueDefinition, parser.ParsedDefinitions, rule, i)
			}
		}
	}

	mustParseInputObjectTypeDefinition := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseInputObjectTypeDefinition(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				inputObjectDefinition := parser.ParsedDefinitions.InputObjectTypeDefinitions[j]
				evalRules(inputObjectDefinition, parser.ParsedDefinitions, rule, i)
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
				evalRules(inputValueDefinition, parser.ParsedDefinitions, rule, i)
			}
		}
	}

	mustParseInterfaceTypeDefinition := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseInterfaceTypeDefinition(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				interfaceTypeDefinition := parser.ParsedDefinitions.InterfaceTypeDefinitions[j]
				evalRules(interfaceTypeDefinition, parser.ParsedDefinitions, rule, i)
			}
		}
	}

	mustParseObjectTypeDefinition := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseObjectTypeDefinition(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				objectTypeDefinition := parser.ParsedDefinitions.ObjectTypeDefinitions[j]
				evalRules(objectTypeDefinition, parser.ParsedDefinitions, rule, i)
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
				evalRules(operationDefinition, parser.ParsedDefinitions, rule, i)
			}
		}
	}

	mustParseScalarTypeDefinition := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseScalarTypeDefinition(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				scalarTypeDefinition := parser.ParsedDefinitions.ScalarTypeDefinitions[j]
				evalRules(scalarTypeDefinition, parser.ParsedDefinitions, rule, i)
			}
		}
	}

	mustParseTypeSystemDefinition := func(rules ruleSet) checkFunc {
		return func(parser *Parser, i int) {

			definition, err := parser.parseTypeSystemDefinition()
			if err != nil {
				panic(err)
			}

			evalRules(definition, parser.ParsedDefinitions, rules, i)
		}
	}

	mustParseSchemaDefinition := func(wantQuery, wantMutation, wantSubscription string, directives ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			schemaDefinition, err := parser.parseSchemaDefinition()
			if err != nil {
				panic(err)
			}

			gotQuery := string(schemaDefinition.Query)
			gotMutation := string(schemaDefinition.Mutation)
			gotSubscription := string(schemaDefinition.Subscription)

			if wantQuery != gotQuery {
				panic(fmt.Errorf("mustParseSchemaDefinition: want(query): %s, got: %s [check: %d]", wantQuery, gotQuery, i))
			}
			if wantMutation != gotMutation {
				panic(fmt.Errorf("mustParseSchemaDefinition: want(mutation): %s, got: %s [check: %d]", wantMutation, wantMutation, i))
			}
			if wantSubscription != gotSubscription {
				panic(fmt.Errorf("mustParseSchemaDefinition: want(subscription): %s, got: %s [check: %d]", wantSubscription, gotSubscription, i))
			}

			for j, rule := range directives {
				directive := parser.ParsedDefinitions.Directives[j]
				evalRules(directive, parser.ParsedDefinitions, rule, i)
			}
		}
	}

	mustParseSelectionSet := func(rules ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var selectionSet document.SelectionSet
			if err := parser.parseSelectionSet(&selectionSet); err != nil {
				panic(err)
			}

			rules.eval(selectionSet, parser.ParsedDefinitions, i)
		}
	}

	mustParseUnionTypeDefinition := func(rules ...ruleSet) checkFunc {
		return func(parser *Parser, i int) {
			var index []int
			if err := parser.parseUnionTypeDefinition(&index); err != nil {
				panic(err)
			}

			for j, rule := range rules {
				scalarTypeDefinition := parser.ParsedDefinitions.UnionTypeDefinitions[j]
				evalRules(scalarTypeDefinition, parser.ParsedDefinitions, rule, i)
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
				evalRules(scalarTypeDefinition, parser.ParsedDefinitions, rule, i)
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
				rule(value, parser.ParsedDefinitions, i, i)
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
				rule(node, parser.ParsedDefinitions, j, i)
			}
		}
	}

	// arguments

	t.Run("string argument", func(t *testing.T) {
		run(`(name: "Gophus")`, mustParseArguments("name"))
	})
	t.Run("string array argument", func(t *testing.T) {
		run(`(fooBars: ["foo","bar"])`, mustParseArguments("fooBars"))
	})
	t.Run("int array argument", func(t *testing.T) {
		run(`(integers: [1,2,3])`, mustParseArguments("integers"))
	})
	t.Run("multiple string arguments", func(t *testing.T) {
		run(`(name: "Gophus", surname: "Gophersson")`,
			mustParseArguments("name", "surname"))
	})
	t.Run("invalid argument must err", func(t *testing.T) {
		run(`(name: "Gophus", surname: "Gophersson"`,
			mustPanic(mustParseArguments()))
	})
	t.Run("invalid argument must err 2", func(t *testing.T) {
		run(`((name: "Gophus", surname: "Gophersson")`,
			mustPanic(mustParseArguments()))
	})

	// arguments definition

	t.Run("single int value", func(t *testing.T) {
		run(`(inputValue: Int)`,
			mustParseArgumentDefinitions("inputValue"))
	})
	t.Run("optional value", func(t *testing.T) {
		run(" ", mustParseArgumentDefinitions())
	})
	t.Run("multiple values", func(t *testing.T) {
		run(`(inputValue: Int, outputValue: String)`,
			mustParseArgumentDefinitions("inputValue", "outputValue"))
	})
	t.Run("not read optional", func(t *testing.T) {
		run(`inputValue: Int)`,
			mustParseArgumentDefinitions())
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`((inputValue: Int)`,
			mustPanic(mustParseArgumentDefinitions()))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`(inputValue: Int`,
			mustPanic(mustParseArgumentDefinitions()))
	})

	// parsePeekedBoolValue

	/*	t.Run("true", func(t *testing.T) {
			run("true", mustParsePeekedBoolValue(true))
		})
		t.Run("false", func(t *testing.T) {
			run("false", mustParsePeekedBoolValue(false))
		})
		t.Run("invalid", func(t *testing.T) {
			run("not_true", mustPanic(mustParsePeekedBoolValue(true)))
		})*/

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
		run("@ somewhere on QUERY",
			mustParseDirectiveDefinition("somewhere", document.DirectiveLocationQUERY))
	})
	t.Run("trailing pipe", func(t *testing.T) {
		run("@ somewhere on | QUERY",
			mustParseDirectiveDefinition("somewhere", document.DirectiveLocationQUERY))
	})
	t.Run("with input value", func(t *testing.T) {
		run("@ somewhere(inputValue: Int) on QUERY",
			mustParseDirectiveDefinition("somewhere", document.DirectiveLocationQUERY),
			mustContainInputValueDefinition(0, "inputValue"),
		)
	})
	t.Run("multiple locations", func(t *testing.T) {
		run("@ somewhere on QUERY | MUTATION",
			mustParseDirectiveDefinition("somewhere",
				document.DirectiveLocationQUERY, document.DirectiveLocationMUTATION))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("@ somewhere QUERY",
			mustPanic(mustParseDirectiveDefinition("somewhere", document.DirectiveLocationQUERY)),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("@ somewhere off QUERY",
			mustPanic(mustParseDirectiveDefinition("somewhere")))
	})
	t.Run("invalid location", func(t *testing.T) {
		run("@ somewhere on QUERY | thisshouldntwork",
			mustPanic(mustParseDirectiveDefinition("somewhere", document.DirectiveLocationQUERY)))
	})

	// parseDirectives

	t.Run(`simple directive`, func(t *testing.T) {
		run(`@rename(index: 3)`,
			mustParseDirectives("rename"),
			mustContainArguments("index"))
	})

	t.Run("multiple directives", func(t *testing.T) {
		run(`@rename(index: 3)@moveto(index: 4)`,
			mustParseDirectives("rename", "moveto"),
			mustContainArguments("index", "index"),
		)
	})

	t.Run("multiple arguments", func(t *testing.T) {
		run(`@rename(index: 3, count: 10)`,
			mustParseDirectives("rename"),
			mustContainArguments("index", "count"),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`@rename(index)`,
			mustPanic(mustParseDirectives("rename")),
		)
	})

	// parsePeekedEnumValue

	/*	t.Run("simple enum", func(t *testing.T) {
		run("MY_ENUM", mustParsePeekedEnumValue("MY_ENUM"))
	})*/

	// parseEnumTypeDefinition

	t.Run("simple enum", func(t *testing.T) {
		run(` Direction {
						NORTH
						EAST
						SOUTH
						WEST
		}`, mustParseEnumTypeDefinition(
			hasName("Direction"),
			hasEnumValuesDefinitions(
				node(hasName("NORTH")),
				node(hasName("EAST")),
				node(hasName("SOUTH")),
				node(hasName("WEST")),
			),
		))
	})
	t.Run("enum with descriptions", func(t *testing.T) {
		run(` Direction {
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
		run(` Direction {
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
		))
	})
	t.Run("enum with directives", func(t *testing.T) {
		run(` Direction @fromTop(to: "bottom") @fromBottom(to: "top"){ NORTH }`,
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
		run("Direction", mustParseEnumTypeDefinition(hasName("Direction")))
	})
	t.Run("invalid enum", func(t *testing.T) {
		run("Direction {", mustPanic(mustParseEnumTypeDefinition()))
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
				}`, mustPanic(mustParseExecutableDefinition(nil, nil)))
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
				`, mustParseExecutableDefinition(
			nodes(
				node(
					hasName("heroFields"),
				),
				node(
					hasName("vehicleFields"),
				),
			),
			nodes(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("QueryWithFragments"),
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
					hasFields(
						node(
							hasName("level2"),
							hasFields(
								node(
									hasName("level3"),
								),
							),
						),
					),
				),
			))
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
				),
				node(
					hasName("age"),
				),
			))
	})

	// parsePeekedFloatValue

	/*	t.Run("valid float", func(t *testing.T) {
		run("12.12", mustParsePeekedFloatValue(12.12))
	})*/

	// parseFragmentDefinition

	t.Run("simple fragment definition", func(t *testing.T) {
		run(`
				MyFragment on SomeType @rename(index: 3){
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
				),
			),
		)
	})
	t.Run("fragment without optional directives", func(t *testing.T) {
		run(`
				MyFragment on SomeType{
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
				MyFragment SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 2", func(t *testing.T) {
		run(`
				MyFragment un SomeType{
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
		)
	})

	// parseInputFieldsDefinition

	t.Run("simple input fields definition", func(t *testing.T) {
		run("{inputValue: Int}",
			mustParseInputFieldsDefinition(
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
	t.Run("optional", func(t *testing.T) {
		run(" ", mustParseInputFieldsDefinition())
	})
	t.Run("multiple", func(t *testing.T) {
		run("{inputValue: Int, outputValue: String}",
			mustParseInputFieldsDefinition(
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
		run(`Person {
					name: String
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`Person {
					name: [String]!
					age: [ Int ]
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFields(
						node(hasName("name")),
						node(hasName("age")),
					),
				),
			),
		)
	})
	t.Run("with default value", func(t *testing.T) {
		run(`Person {
					name: String = "Gophina"
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFields(
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
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run("Person", mustParseInputObjectTypeDefinition(
			node(
				hasName("Person"),
			),
		))
	})
	t.Run("", func(t *testing.T) {
		run(`Person @fromTop(to: "bottom") @fromBottom(to: "top"){
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
					hasInputFields(
						node(
							hasName("name"),
						),
					),
				),
			),
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
				),
				node(
					hasDescription("this is a outputValue"),
					hasName("outputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
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
				),
			),
		)
	})

	// parseInterfaceTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run(`NameEntity {
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
				),
			),
		)
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`Person {
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
		run(`Person`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("Person"),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`NameEntity @fromTop(to: "bottom") @fromBottom(to: "top") {
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

	// parsePeekedListValue
	/*	t.Run("simple 3", func(t *testing.T) {
			run("[1,2,3]", mustParsePeekedListValue(3))
		})
		t.Run("complex 4", func(t *testing.T) {
			run(`[ 1	,"2" 3,,[	1	]]`, mustParsePeekedListValue(4))
		})*/

	// parseObjectTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run(`Person {
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
				),
			),
		)
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`Person {
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
		run(`Person`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
				),
			),
		)
	})
	t.Run("implements interface", func(t *testing.T) {
		run(`Person implements Human {
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
		run(`Person implements Human & Mammal {
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
		run(`Person @fromTop(to: "bottom") @fromBottom(to: "top") {
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
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("fail without selection set", func(t *testing.T) {
		run(`query allGophers($color: String)@rename(index: 3) `,
			mustPanic(mustParseOperationDefinition()),
		)
	})

	// parseScalarTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run("JSON", mustParseScalarTypeDefinition(
			node(
				hasName("JSON"),
			),
		))
	})
	t.Run("with directives", func(t *testing.T) {
		run(`JSON @fromTop(to: "bottom") @fromBottom(to: "top")`, mustParseScalarTypeDefinition(
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
			),
		))
	})

	// parseSchemaDefinition

	t.Run("simple", func(t *testing.T) {
		run(`
{
	query: Query
	mutation: Mutation
	subscription: Subscription
}`, mustParseSchemaDefinition("Query", "Mutation", "Subscription"))
	})
	t.Run("invalid", func(t *testing.T) {
		run(` {
query : Query	
mutation : Mutation
subscription : Subscription
query: Query2 }`, mustPanic(mustParseSchemaDefinition("Query", "Mutation", "Subscription")),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(` @fromTop(to: "bottom") @fromBottom(to: "top") {
	query: Query
	mutation: Mutation
	subscription: Subscription
}`, mustParseSchemaDefinition("Query", "Mutation", "Subscription",
			node(
				hasName("fromTop"),
			),
			node(
				hasName("fromBottom"),
			),
		))
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
			),
		))
	})
	t.Run("inline and fragment spreads", func(t *testing.T) {
		run(`{
					... on Goland
					...Air
					... on Water
				}`, mustParseSelectionSet(
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
			),
		))
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

	// parsePeekedStringValue

	/*	t.Run("simple", func(t *testing.T) {
				run(`"lorem ipsum"`, mustParsePeekedStringValue("lorem ipsum"))
			})
			t.Run("multiline", func(t *testing.T) {
				run(`"""
		lorem ipsum
		"""`, mustParsePeekedStringValue("lorem ipsum"))
			})
			t.Run("multiline escaped", func(t *testing.T) {
				run(`"""
		foo \" bar
		"""`, mustParsePeekedStringValue("foo \\\" bar"))
			})
			t.Run("single line escaped", func(t *testing.T) {
				run(`"foo bar \" baz"`, mustParsePeekedStringValue("foo bar \\\" baz"))
			})*/

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
						),
					),
				),
			),
		)
	})
	t.Run("schema", func(t *testing.T) {
		run(`
schema {
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
					hasSchemaDefinition(),
					hasScalarTypeSystemDefinitions(
						node(
							hasName("JSON"),
						),
					),
					hasObjectTypeSystemDefinitions(
						node(
							hasName("Person"),
						),
					),
					hasInterfaceTypeSystemDefinitions(
						node(
							hasName("firstEntity"),
						),
					),
					hasEnumTypeSystemDefinitions(
						node(
							hasName("Direction"),
						),
					),
					hasInputObjectTypeSystemDefinitions(
						node(
							hasName("Person"),
						),
					),
					hasDirectiveDefinitions(
						node(
							hasName("someway"),
						),
					),
				),
			))
	})

	// parseUnionTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run("SearchResult = Photo | Person",
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person"),
				),
			))
	})
	t.Run("multiple members", func(t *testing.T) {
		run("SearchResult = Photo | Person | Car | Planet",
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person", "Car", "Planet"),
				),
			),
		)
	})
	t.Run("with linebreaks", func(t *testing.T) {
		run(` SearchResult = Photo 
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
		run(`SearchResult @fromTop(to: "bottom") @fromBottom(to: "top") = Photo | Person`,
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

	// parseVariableDefinitions

	t.Run("simple", func(t *testing.T) {
		run("($foo : bar)",
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeName("bar"),
					),
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
				),
				node(
					hasName("baz"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("bat"),
					),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("($foo : bar!",
			mustPanic(mustParseVariableDefinitions()))
	})

	// parseValue
	t.Run("int", func(t *testing.T) {
		run("1337", mustParseValue(
			document.ValueTypeInt,
			expectIntegerValue(1337),
		))
	})
	t.Run("string", func(t *testing.T) {
		run(`"foo"`, mustParseValue(
			document.ValueTypeString,
			expectByteSliceValue("foo"),
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
			))
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
		))
	})
	t.Run("variable", func(t *testing.T) {
		run("$1337", mustParseValue(document.ValueTypeVariable, expectByteSliceValue("1337")))
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
		run("13.37", mustParseValue(document.ValueTypeFloat, expectFloatValue(13.37)))
	})
	t.Run("invalid float", func(t *testing.T) {
		run("1.3.3.7", mustPanic(mustParseValue(document.ValueTypeFloat, expectFloatValue(13.37))))
	})
	t.Run("boolean", func(t *testing.T) {
		run("true", mustParseValue(document.ValueTypeBoolean, expectBooleanValue(true)))
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
		run("null", mustParseValue(document.ValueTypeNull))
	})

	// parseTypes

	t.Run("simple named", func(t *testing.T) {
		run("String", mustParseType(
			hasTypeKind(document.TypeKindNAMED),
			hasTypeName("String"),
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
						),
					),
				),
			),
		))
	})
}

func TestParser_Starwars(t *testing.T) {

	inputFileName := "../../starwars.schema.graphql"
	fixtureFileName := "type_system_definition_parsed_starwars"

	parser := NewParser()

	starwarsSchema, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	def, err := parser.ParseTypeSystemDefinition(starwarsSchema)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(def, "", "  ")
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
	executableDefinition, err := parser.ParseExecutableDefinition(inputFileData)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(executableDefinition, "", "  ")
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

		executableDefinition, err := parser.ParseExecutableDefinition(testData)
		if err != nil {
			b.Fatal(err)
		}

		_ = executableDefinition

	}

}
