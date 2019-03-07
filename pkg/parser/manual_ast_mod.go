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

func (m *ManualAstMod) PutLiteralString(literal string) (ref int, err error) {
	err = m.p.l.AppendBytes([]byte(literal))
	if err != nil {
		return
	}

	ref = m.p.putByteSliceReference(m.p.l.Read().Literal)

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
