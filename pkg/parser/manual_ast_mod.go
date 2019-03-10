package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
)

// ManualAstMod keeps functions to manually modify the parsed ast
type ManualAstMod struct {
	p *Parser
}

func NewManualAstMod(p *Parser) *ManualAstMod {
	return &ManualAstMod{
		p: p,
	}
}

func (m *ManualAstMod) SetQueryTypeName(name int) {
	m.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Query = name
}

func (m *ManualAstMod) SetMutationTypeName(name int) {
	m.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Mutation = name
}

func (m *ManualAstMod) SetSubscriptionTypeName(name int) {
	m.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Subscription = name
}

func (m *ManualAstMod) PutLiteralString(literal string) (nameRef int, byteSliceRef document.ByteSliceReference, err error) {
	return m.PutLiteralBytes([]byte(literal))
}

func (m *ManualAstMod) PutLiteralBytes(literal []byte) (nameRef int, byteSliceRef document.ByteSliceReference, err error) {

	err = m.p.l.AppendBytes(literal)
	if err != nil {
		return
	}

	tok := m.p.l.Read()

	byteSliceRef = tok.Literal
	nameRef = m.p.putByteSliceReference(byteSliceRef)

	return
}

func (m *ManualAstMod) PutField(field document.Field) int {
	m.p.ParsedDefinitions.Fields = append(m.p.ParsedDefinitions.Fields, field)
	return len(m.p.ParsedDefinitions.Fields) - 1
}

func (m *ManualAstMod) DeleteFieldFromSelectionSet(fieldRef, setRef int) {
	for i, j := range m.p.ParsedDefinitions.SelectionSets[setRef].Fields {
		if fieldRef == j {
			m.p.ParsedDefinitions.SelectionSets[setRef].Fields = append(m.p.ParsedDefinitions.SelectionSets[setRef].Fields[:i], m.p.ParsedDefinitions.SelectionSets[setRef].Fields[i+1:]...)
			return
		}
	}
}

func (m *ManualAstMod) AppendFieldToSelectionSet(fieldRef, setRef int) {
	m.p.ParsedDefinitions.SelectionSets[setRef].Fields = append(m.p.ParsedDefinitions.SelectionSets[setRef].Fields, fieldRef)
}

func (m *ManualAstMod) PutValue(value document.Value) int {
	m.p.ParsedDefinitions.Values = append(m.p.ParsedDefinitions.Values, value)
	return len(m.p.ParsedDefinitions.Values) - 1
}

func (m *ManualAstMod) PutArgument(argument document.Argument) int {
	m.p.ParsedDefinitions.Arguments = append(m.p.ParsedDefinitions.Arguments, argument)
	return len(m.p.ParsedDefinitions.Arguments) - 1
}

func (m *ManualAstMod) MergeArgIntoFieldArguments(argRef, fieldRef int) {

	arg := m.p.ParsedDefinitions.Arguments[argRef]
	field := m.p.ParsedDefinitions.Fields[fieldRef]

	if field.ArgumentSet == -1 {
		set := m.p.indexPoolGet()
		set = append(set, argRef)
		field.ArgumentSet = m.PutArgumentSet(set)
	} else {
		var didUpdate bool
		for i, j := range m.p.ParsedDefinitions.ArgumentSets[field.ArgumentSet] {
			current := m.p.ParsedDefinitions.Arguments[j]
			if current.Name == arg.Name {
				m.p.ParsedDefinitions.ArgumentSets[field.ArgumentSet][i] = argRef // update reference in place
				didUpdate = true
				break
			}
		}

		if !didUpdate {
			m.p.ParsedDefinitions.ArgumentSets[field.ArgumentSet] = append(m.p.ParsedDefinitions.ArgumentSets[field.ArgumentSet], argRef) // add argument
		}
	}

	m.p.ParsedDefinitions.Fields[fieldRef] = field
}

func (m *ManualAstMod) PutArgumentSet(set document.ArgumentSet) int {
	m.p.ParsedDefinitions.ArgumentSets = append(m.p.ParsedDefinitions.ArgumentSets, set)
	return len(m.p.ParsedDefinitions.ArgumentSets) - 1
}
