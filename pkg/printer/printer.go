package printer

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
	"io"
)

type Printer struct {
	l   *lookup.Lookup
	w   *lookup.Walker
	p   *parser.Parser
	out io.Writer
	err error
}

func New() *Printer {
	return &Printer{}
}

func (p *Printer) SetInput(parser *parser.Parser, l *lookup.Lookup, w *lookup.Walker) {
	p.p = parser
	p.l = l
	p.w = w
	p.err = nil
}

func (p *Printer) write(bytes []byte) {
	if p.err != nil {
		return
	}
	_, p.err = p.out.Write(bytes)
}

func (p *Printer) PrintTypeSystemDefinition(out io.Writer) error {

	p.out = out

	rootNodes := p.w.TypeSystemDefinitionOrderedRootNodes()
	var addDoubleLineTerminator bool
	for rootNodes.Next() {

		if addDoubleLineTerminator {
			p.write(literal.LINETERMINATOR)
			p.write(literal.LINETERMINATOR)
		}

		ref, kind := rootNodes.Value()
		switch kind {
		case lookup.SCHEMA:
			p.PrintSchemaDefinition()
		case lookup.OBJECT_TYPE_DEFINITION:
			p.PrintObjectTypeDefinition(ref)
		case lookup.ENUM_TYPE_DEFINITION:
			p.PrintEnumTypeDefinition(ref)
		case lookup.DIRECTIVE_DEFINITION:
			p.PrintDirectiveDefinition(ref)
		case lookup.INTERFACE_TYPE_DEFINITION:
			p.PrintInterfaceTypeDefinition(ref)
		case lookup.SCALAR_TYPE_DEFINITION:
			p.PrintScalarTypeDefinition(ref)
		case lookup.UNION_TYPE_DEFINITION:
			p.PrintUnionTypeDefinition(ref)
		case lookup.INPUT_OBJECT_TYPE_DEFINITION:
			p.PrintInputObjectTypeDefinition(ref)
		}

		addDoubleLineTerminator = true
	}

	p.write(literal.LINETERMINATOR)
	return p.err
}

func (p *Printer) PrintSchemaDefinition() {
	definition := p.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition
	p.write(literal.SCHEMA)
	p.write(literal.SPACE)
	p.write(literal.CURLYBRACKETOPEN)
	if definition.Query.Length() != 0 {
		p.write(literal.LINETERMINATOR)
		p.write(literal.TAB)
		p.PrintSimpleField(literal.QUERY, definition.Query)
	}
	if definition.Mutation.Length() != 0 {
		p.write(literal.LINETERMINATOR)
		p.write(literal.TAB)
		p.PrintSimpleField(literal.MUTATION, definition.Mutation)
	}
	if definition.Subscription.Length() != 0 {
		p.write(literal.LINETERMINATOR)
		p.write(literal.TAB)
		p.PrintSimpleField(literal.SUBSCRIPTION, definition.Subscription)
	}
	p.write(literal.LINETERMINATOR)
	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) PrintSimpleField(name []byte, value document.ByteSliceReference) {
	p.write(name)
	p.write(literal.COLON)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(value))
}

func (p *Printer) PrintDescription(ref document.ByteSliceReference, linePrefix ...[]byte) {
	if ref.Length() == 0 {
		return
	}
	description := p.p.ByteSlice(ref)
	multiLine := bytes.Contains(description, literal.LINETERMINATOR)
	if !multiLine {
		for _, prefix := range linePrefix {
			p.write(prefix)
		}
		p.write(literal.QUOTE)
		p.write(description)
		p.write(literal.QUOTE)
		p.write(literal.LINETERMINATOR)
		return
	}
	description = transform.TrimWhitespace(description)
	for _, prefix := range linePrefix {
		p.write(prefix)
	}
	p.write(literal.QUOTE)
	p.write(literal.QUOTE)
	p.write(literal.QUOTE)
	p.write(literal.LINETERMINATOR)
	for _, prefix := range linePrefix {
		p.write(prefix)
	}
	p.write(description)
	p.write(literal.LINETERMINATOR)
	for _, prefix := range linePrefix {
		p.write(prefix)
	}
	p.write(literal.QUOTE)
	p.write(literal.QUOTE)
	p.write(literal.QUOTE)
	p.write(literal.LINETERMINATOR)
}

func (p *Printer) PrintFieldDefinition(ref int) {
	definition := p.p.ParsedDefinitions.FieldDefinitions[ref]
	p.PrintDescription(definition.Description, literal.TAB)
	p.write(literal.TAB)
	p.write(p.p.ByteSlice(definition.Name))
	if definition.ArgumentsDefinition != -1 {
		p.PrintArgumentsDefinitionInline(definition.ArgumentsDefinition)
	}
	p.write(literal.COLON)
	p.write(literal.SPACE)
	p.PrintType(definition.Type)
	if definition.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(definition.DirectiveSet)
	}
}

func (p *Printer) PrintArgumentsDefinition(ref int) {
	definition := p.p.ParsedDefinitions.ArgumentsDefinitions[ref]
	p.write(literal.BRACKETOPEN)
	for _, i := range definition.InputValueDefinitions {
		p.write(literal.LINETERMINATOR)
		p.PrintInputValueDefinition(i)
	}
	p.write(literal.LINETERMINATOR)
	p.write(literal.BRACKETCLOSE)
}

func (p *Printer) PrintArgumentsDefinitionInline(ref int) {
	definition := p.p.ParsedDefinitions.ArgumentsDefinitions[ref]
	p.write(literal.BRACKETOPEN)
	var addSpace bool
	for _, i := range definition.InputValueDefinitions {
		if addSpace {
			p.write(literal.SPACE)
		}
		p.PrintInputValueDefinitionInline(i)
		addSpace = true
	}
	p.write(literal.BRACKETCLOSE)
}

func (p *Printer) PrintInputValueDefinition(ref int) {
	definition := p.p.ParsedDefinitions.InputValueDefinitions[ref]
	p.PrintDescription(definition.Description, literal.TAB)
	p.write(literal.TAB)
	p.write(p.p.ByteSlice(definition.Name))
	p.write(literal.COLON)
	p.write(literal.SPACE)
	p.PrintType(definition.Type)
	if definition.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(definition.DirectiveSet)
	}
}

func (p *Printer) PrintInputValueDefinitionInline(ref int) {
	definition := p.p.ParsedDefinitions.InputValueDefinitions[ref]
	p.write(p.p.ByteSlice(definition.Name))
	p.write(literal.COLON)
	p.write(literal.SPACE)
	p.PrintType(definition.Type)
	if definition.DefaultValue != -1 {
		p.write(literal.SPACE)
		p.write(literal.EQUALS)
		p.write(literal.SPACE)
		p.PrintValue(definition.DefaultValue)
	}
	if definition.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(definition.DirectiveSet)
	}
}

func (p *Printer) PrintType(ref int) {
	definition := p.p.ParsedDefinitions.Types[ref]
	switch definition.Kind {
	case document.TypeKindNON_NULL:
		p.PrintType(definition.OfType)
		p.write(literal.BANG)
	case document.TypeKindLIST:
		p.write(literal.SQUAREBRACKETOPEN)
		p.PrintType(definition.OfType)
		p.write(literal.SQUAREBRACKETCLOSE)
	case document.TypeKindNAMED:
		p.write(p.p.ByteSlice(definition.Name))
	}
}

func (p *Printer) PrintObjectTypeDefinition(ref int) {
	definition := p.l.ObjectTypeDefinition(ref)
	p.PrintDescription(definition.Description)
	p.write(literal.TYPE)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(definition.Name))
	if definition.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(definition.DirectiveSet)

	}
	p.write(literal.SPACE)
	p.write(literal.CURLYBRACKETOPEN)
	for _, i := range definition.FieldsDefinition {
		p.write(literal.LINETERMINATOR)
		p.PrintFieldDefinition(i)
	}
	p.write(literal.LINETERMINATOR)
	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) PrintEnumTypeDefinition(ref int) {
	definition := p.p.ParsedDefinitions.EnumTypeDefinitions[ref]
	p.PrintDescription(definition.Description)
	p.write(literal.ENUM)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(definition.Name))
	if definition.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(definition.DirectiveSet)
	}
	p.write(literal.SPACE)
	p.write(literal.CURLYBRACKETOPEN)
	p.write(literal.LINETERMINATOR)
	var addLineTerminator bool
	for _, enumValue := range definition.EnumValuesDefinition {
		if addLineTerminator {
			p.write(literal.LINETERMINATOR)
		}
		p.PrintEnumValueDefinition(enumValue)
		addLineTerminator = true
	}
	p.write(literal.LINETERMINATOR)
	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) PrintEnumValueDefinition(ref int) {
	definition := p.p.ParsedDefinitions.EnumValuesDefinitions[ref]
	p.PrintDescription(definition.Description, literal.TAB)
	p.write(literal.TAB)
	p.write(p.p.ByteSlice(definition.EnumValue))
}

func (p *Printer) PrintDirectiveDefinition(ref int) {
	definition := p.p.ParsedDefinitions.DirectiveDefinitions[ref]
	p.PrintDescription(definition.Description)
	p.write(literal.DIRECTIVE)
	p.write(literal.SPACE)
	p.write(literal.AT)
	p.write(p.p.ByteSlice(definition.Name))
	p.write(literal.SPACE)
	if definition.ArgumentsDefinition != -1 {
		p.PrintArgumentsDefinition(definition.ArgumentsDefinition)
		p.write(literal.SPACE)
	}
	p.write(literal.ON)
	p.write(literal.SPACE)
	p.PrintDirectiveLocations(definition.DirectiveLocations)
}

func (p *Printer) PrintDirectiveLocations(locations []int) {
	var addPipe bool
	for _, location := range locations {

		if addPipe {
			p.write(literal.SPACE)
			p.write(literal.PIPE)
			p.write(literal.SPACE)
		}

		p.write([]byte(document.DirectiveLocation(location).String()))

		addPipe = true
	}
}

func (p *Printer) PrintInterfaceTypeDefinition(ref int) {
	definition := p.p.ParsedDefinitions.InterfaceTypeDefinitions[ref]
	p.PrintDescription(definition.Description)
	p.write(literal.INTERFACE)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(definition.Name))
	if definition.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(definition.DirectiveSet)
	}
	p.write(literal.SPACE)
	p.write(literal.CURLYBRACKETOPEN)
	for _, field := range definition.FieldsDefinition {
		p.write(literal.LINETERMINATOR)
		p.PrintFieldDefinition(field)
	}
	p.write(literal.LINETERMINATOR)
	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) PrintScalarTypeDefinition(ref int) {
	definition := p.p.ParsedDefinitions.ScalarTypeDefinitions[ref]
	p.PrintDescription(definition.Description)
	p.write(literal.SCALAR)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(definition.Name))
	if definition.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(definition.DirectiveSet)
	}
}

func (p *Printer) PrintUnionTypeDefinition(ref int) {
	definition := p.p.ParsedDefinitions.UnionTypeDefinitions[ref]
	p.PrintDescription(definition.Description)
	p.write(literal.UNION)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(definition.Name))
	if definition.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(definition.DirectiveSet)
	}
	p.write(literal.SPACE)
	p.write(literal.EQUALS)
	p.write(literal.SPACE)
	var addPipe bool
	for _, memberName := range definition.UnionMemberTypes {
		if addPipe {
			p.write(literal.SPACE)
			p.write(literal.PIPE)
			p.write(literal.SPACE)
		}
		p.write(p.p.CachedByteSlice(memberName))
		addPipe = true
	}
}

func (p *Printer) PrintInputObjectTypeDefinition(ref int) {
	definition := p.p.ParsedDefinitions.InputObjectTypeDefinitions[ref]
	p.PrintDescription(definition.Description)
	p.write(literal.INPUT)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(definition.Name))
	p.write(literal.SPACE)
	p.write(literal.CURLYBRACKETOPEN)
	for _, inputValueDefinition := range p.p.ParsedDefinitions.InputFieldsDefinitions[definition.InputFieldsDefinition].InputValueDefinitions {
		p.write(literal.LINETERMINATOR)
		p.PrintInputValueDefinition(inputValueDefinition)
	}
	p.write(literal.LINETERMINATOR)
	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) PrintExecutableSchema(out io.Writer) error {

	p.out = out

	operations := p.w.OperationDefinitionIterable()
	for operations.Next() {
		operation := operations.Value()
		p.printOperation(operation)
	}

	if p.l.HasOperationDefinitions() && p.l.HasFragmentDefinitions() {
		p.write(literal.LINETERMINATOR)
	}

	fragments := p.w.FragmentDefinitionIterable()
	var addNewLine bool
	for fragments.Next() {
		if addNewLine {
			p.write(literal.LINETERMINATOR)
		}
		fragment := fragments.Value()
		p.printFragmentDefinition(fragment)
		addNewLine = true
	}
	return p.err
}

func (p *Printer) printFragmentDefinition(fragment document.FragmentDefinition) {
	p.write(literal.FRAGMENT)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(fragment.FragmentName))
	p.write(literal.SPACE)
	p.write(literal.ON)
	p.write(literal.SPACE)
	p.write(p.p.ByteSlice(p.l.Type(fragment.TypeCondition).Name))
	p.write(literal.SPACE)
	if fragment.DirectiveSet != -1 {
		p.printDirectiveSet(fragment.DirectiveSet)
		p.write(literal.SPACE)
	}
	p.printSelectionSet(fragment.SelectionSet)
}

func (p *Printer) printOperation(operation document.OperationDefinition) {
	hasName := operation.Name.Length() != 0
	p.printOperationType(operation.OperationType, hasName)
	if hasName {
		p.write(p.p.ByteSlice(operation.Name))
		p.write(literal.SPACE)
	}
	if operation.DirectiveSet != -1 {
		p.printDirectiveSet(operation.DirectiveSet)
		p.write(literal.SPACE)
	}
	if operation.SelectionSet != -1 {
		p.printSelectionSet(operation.SelectionSet)
	}
}

func (p *Printer) printDirectiveSet(setRef int) {
	set := p.l.DirectiveSet(setRef)
	iter := p.l.DirectiveIterable(set)
	var addSpace bool
	for iter.Next() {
		if addSpace {
			p.write(literal.SPACE)
		}
		directive, _ := iter.Value()
		p.printDirective(directive)
		addSpace = true
	}
}

func (p *Printer) printDirective(directive document.Directive) {
	p.write(literal.AT)
	p.write(p.p.ByteSlice(directive.Name))
	if directive.ArgumentSet != -1 {
		p.printArgumentSet(directive.ArgumentSet)
	}
}

func (p *Printer) printOperationType(operationType document.OperationType, hasName bool) {
	switch operationType {
	case document.OperationTypeQuery:
		if hasName {
			p.write(literal.QUERY)
			p.write(literal.SPACE)
		}
	case document.OperationTypeMutation:
		p.write(literal.MUTATION)
		p.write(literal.SPACE)
	case document.OperationTypeSubscription:
		p.write(literal.SUBSCRIPTION)
		p.write(literal.SPACE)
	}
}

func (p *Printer) printSelectionSet(ref int) {

	p.write(literal.CURLYBRACKETOPEN)

	set := p.l.SelectionSetContentsIterator(ref)
	var addSpace bool
	for set.Next() {

		if addSpace {
			p.write(literal.SPACE)
		}

		kind, ref := set.Value()
		switch kind {
		case lookup.FIELD:
			p.printField(ref)
		case lookup.FRAGMENT_SPREAD:
			p.printFragmentSpread(ref)
		case lookup.INLINE_FRAGMENT:
			p.printInlineFragment(ref)
		}

		addSpace = true
	}

	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) printField(ref int) {

	field := p.l.Field(ref)
	p.write(p.p.ByteSlice(field.Name))

	if field.ArgumentSet != -1 {
		p.printArgumentSet(field.ArgumentSet)
	}

	if field.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(field.DirectiveSet)
	}

	if field.SelectionSet != -1 {
		p.write(literal.SPACE)
		p.printSelectionSet(field.SelectionSet)
	}
}

func (p *Printer) printFragmentSpread(ref int) {
	spread := p.l.FragmentSpread(ref)
	p.write(literal.SPREAD)
	p.write(p.p.ByteSlice(spread.FragmentName))
	if spread.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(spread.DirectiveSet)
	}
}

func (p *Printer) printInlineFragment(ref int) {

	inline := p.l.InlineFragment(ref)
	p.write(literal.SPREAD)

	if inline.TypeCondition != -1 {
		typeCondition := p.l.Type(inline.TypeCondition)
		p.write(literal.ON)
		p.write(literal.SPACE)
		p.write(p.p.ByteSlice(typeCondition.Name))
	}

	if inline.DirectiveSet != -1 {
		p.write(literal.SPACE)
		p.printDirectiveSet(inline.DirectiveSet)
	}

	p.printSelectionSet(inline.SelectionSet)
}

func (p *Printer) printArgumentSet(ref int) {

	p.write(literal.BRACKETOPEN)

	set := p.l.ArgumentSet(ref)
	iter := p.l.ArgumentsIterable(set)
	var addSpace bool
	for iter.Next() {
		if addSpace {
			p.write(literal.SPACE)
		}
		argument, _ := iter.Value()
		p.printArgument(argument)
		addSpace = true
	}

	p.write(literal.BRACKETCLOSE)
}

func (p *Printer) printArgument(arg document.Argument) {
	p.write(p.p.ByteSlice(arg.Name))
	p.write(literal.COLON)
	p.PrintValue(arg.Value)
}

func (p *Printer) PrintValue(ref int) {

	value := p.l.Value(ref)

	switch value.ValueType {
	case document.ValueTypeBoolean, document.ValueTypeInt, document.ValueTypeFloat, document.ValueTypeEnum:
		p.write(p.p.ByteSlice(value.Raw))
	case document.ValueTypeNull:
		p.write(literal.NULL)
	case document.ValueTypeString:
		p.write(literal.QUOTE)
		p.write(p.p.ByteSlice(value.Raw))
		p.write(literal.QUOTE)
	case document.ValueTypeVariable:
		p.write(literal.DOLLAR)
		p.write(p.p.ByteSlice(value.Raw))
	case document.ValueTypeObject:
		p.printObjectValue(value.Reference)
	case document.ValueTypeList:
		p.printListValue(value.Reference)
	}
}

func (p *Printer) printObjectValue(ref int) {

	p.write(literal.CURLYBRACKETOPEN)

	objectValue := p.l.ObjectValue(ref)
	fields := p.l.ObjectFieldsIterator(objectValue)
	var addComma bool
	for fields.Next() {
		if addComma {
			p.write(literal.COMMA)
		}

		field, _ := fields.Value()
		p.printObjectField(field)

		addComma = true
	}

	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) printObjectField(field document.ObjectField) {
	p.write(p.p.ByteSlice(field.Name))
	p.write(literal.COLON)
	p.PrintValue(field.Value)
}

func (p *Printer) printListValue(ref int) {

	p.write(literal.SQUAREBRACKETOPEN)

	list := p.l.ListValue(ref)
	var addComma bool
	for _, valueRef := range list {

		if addComma {
			p.write(literal.COMMA)
		}

		p.PrintValue(valueRef)
		addComma = true
	}

	p.write(literal.SQUAREBRACKETCLOSE)
}
