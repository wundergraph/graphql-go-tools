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
