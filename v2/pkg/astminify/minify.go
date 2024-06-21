package astminify

import (
	"bytes"
	"errors"
	"fmt"
	"hash"
	"slices"

	"github.com/cespare/xxhash/v2"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type Minifier struct {
	out  *ast.Document
	temp *ast.Document
	def  *ast.Document
	hs   xxhash.Digest

	opts MinifyOptions

	fragmentDefinitionCount int
}

func NewMinifier(operation, definition string) (*Minifier, error) {
	out, rep := astparser.ParseGraphqlDocumentString(operation)
	if rep.HasErrors() {
		return nil, rep
	}
	temp, _ := astparser.ParseGraphqlDocumentString(operation)
	def, rep := astparser.ParseGraphqlDocumentString(definition)
	if rep.HasErrors() {
		return nil, rep
	}
	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	if err != nil {
		return nil, err
	}
	return &Minifier{
			out:                     &out,
			temp:                    &temp,
			def:                     &def,
			fragmentDefinitionCount: -1},
		nil
}

type MinifyOptions struct {
	Pretty bool
}

func (m *Minifier) Minify(options MinifyOptions) (string, error) {

	m.opts = options

	err := m.validate()
	if err != nil {
		return "", err
	}
	m.setupAst()

	walker := astvisitor.Walker{}
	v := &minifyVisitor{
		w:    &walker,
		out:  m.out,
		temp: m.temp,
		def:  m.def,
		s:    make(map[uint64]*stats),
		buf:  &bytes.Buffer{},
		h:    xxhash.New(),
	}
	walker.RegisterEnterSelectionSetVisitor(v)
	walker.RegisterEnterFragmentDefinitionVisitor(v)
	report := &operationreport.Report{}
	walker.Walk(m.out, m.def, report)
	if report.HasErrors() {
		return "", report
	}
	m.apply(v)
	if options.Pretty {
		return astprinter.PrintStringIndent(m.out, nil, "  ")
	}
	return astprinter.PrintString(m.out, nil)
}

func (m *Minifier) validate() error {
	if len(m.temp.OperationDefinitions) != 1 {
		return errors.New("AST must have exactly one operation definition")
	}
	return nil

}

func (m *Minifier) setupAst() {
	m.temp.OperationDefinitions[0].VariableDefinitions.Refs = nil
	m.temp.OperationDefinitions[0].HasVariableDefinitions = false
	m.temp.OperationDefinitions[0].VariableDefinitions.Refs = nil
	m.temp.OperationDefinitions[0].HasVariableDefinitions = false
	m.temp.OperationDefinitions[0].Directives.Refs = nil
	m.temp.OperationDefinitions[0].HasDirectives = false
	m.temp.OperationDefinitions[0].Name = ast.ByteSliceReference{
		Start: 0,
		End:   0,
	}
}

func (m *Minifier) apply(vis *minifyVisitor) {
	replacements := make([]*stats, 0, len(vis.s))
	for _, s := range vis.s {
		if s.count > 1 {
			replacements = append(replacements, s)
		}
	}
	// sort by depth
	slices.SortFunc(replacements, func(a, b *stats) int {
		return b.depth - a.depth
	})
	for _, s := range replacements {
		m.replaceItems(s)
	}
}

func (m *Minifier) replaceItems(s *stats) {
	fragmentName := m.out.Input.AppendInputString(m.fragmentName())

	typeName := s.items[0].enclosingType.NameString(m.def)
	typeDef := ast.Type{
		TypeKind: ast.TypeKindNamed,
		Name:     m.out.Input.AppendInputString(typeName),
	}
	m.out.Types = append(m.out.Types, typeDef)
	typeRef := len(m.out.Types) - 1

	frag := ast.FragmentDefinition{
		Name: fragmentName,
		//HasDirectives:   false,
		//Directives:      ast.DirectiveList{},
		SelectionSet:  m.out.CopySelectionSet(s.items[0].selectionSet),
		HasSelections: true,
		TypeCondition: ast.TypeCondition{
			Type: typeRef,
		},
	}

	m.out.FragmentDefinitions = append(m.out.FragmentDefinitions, frag)
	fragRef := len(m.out.FragmentDefinitions) - 1
	m.out.RootNodes = append(m.out.RootNodes, ast.Node{
		Kind: ast.NodeKindFragmentDefinition,
		Ref:  fragRef,
	})
	for x, i := range s.items {
		switch i.ancestor.Kind {
		case ast.NodeKindInlineFragment:
			for j := range m.out.Selections {
				if m.out.Selections[j].Kind == ast.SelectionKindInlineFragment && m.out.Selections[j].Ref == s.items[x].ancestor.Ref {
					m.out.Selections[j].Kind = ast.SelectionKindFragmentSpread

					spread := ast.FragmentSpread{
						FragmentName: fragmentName,
					}
					m.out.FragmentSpreads = append(m.out.FragmentSpreads, spread)
					spreadRef := len(m.out.FragmentSpreads) - 1
					m.out.Selections[j].Ref = spreadRef
					break
				}
			}

			/*
				set := m.out.InlineFragments[i.ancestor.Ref].SelectionSet
				spread := ast.FragmentSpread{
					FragmentName: fragmentName,
				}
				m.out.FragmentSpreads = append(m.out.FragmentSpreads, spread)
				spreadRef := len(m.out.FragmentSpreads) - 1

				m.out.Selections = append(m.out.Selections, ast.Selection{
					Kind: ast.SelectionKindFragmentSpread,
					Ref:  spreadRef,
				})
				selection := len(m.out.Selections) - 1
				m.out.SelectionSets[set].SelectionRefs = []int{selection}
			*/

		case ast.NodeKindField:
			set := m.out.Fields[i.ancestor.Ref].SelectionSet

			spread := ast.FragmentSpread{
				FragmentName: fragmentName,
			}
			m.out.FragmentSpreads = append(m.out.FragmentSpreads, spread)
			spreadRef := len(m.out.FragmentSpreads) - 1

			m.out.Selections = append(m.out.Selections, ast.Selection{
				Kind: ast.SelectionKindFragmentSpread,
				Ref:  spreadRef,
			})
			selection := len(m.out.Selections) - 1
			m.out.SelectionSets[set].SelectionRefs = []int{selection}
		default:
			fmt.Printf("Unknown ancestor kind: %s\n", i.ancestor.Kind.String())
		}
	}
}

const (
	alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

func (m *Minifier) fragmentName() string {
	m.fragmentDefinitionCount++
	if m.fragmentDefinitionCount < 26 {
		return string(alphabet[m.fragmentDefinitionCount])
	}
	chars := make([]byte, 2)
	chars[0] = alphabet[m.fragmentDefinitionCount/26]
	chars[1] = alphabet[m.fragmentDefinitionCount%26]
	name := string(chars)
	_, exists := m.out.FragmentDefinitionRef([]byte(name))
	if exists {
		return m.fragmentName()
	}
	return name
}

type minifyVisitor struct {
	w    *astvisitor.Walker
	out  *ast.Document
	temp *ast.Document
	def  *ast.Document

	s map[uint64]*stats

	buf *bytes.Buffer
	h   hash.Hash64
}

func (m *minifyVisitor) EnterFragmentDefinition(ref int) {
	m.w.SkipNode()
}

type stats struct {
	count int
	size  int
	items []item
	depth int
}

type item struct {
	selectionSet  int
	ancestor      ast.Node
	enclosingType ast.Node
}

func (m *minifyVisitor) EnterSelectionSet(ref int) {

	ancestor := m.w.Ancestor()

	m.temp.OperationDefinitions[0].SelectionSet = ref

	tempName := m.w.EnclosingTypeDefinition.NameBytes(m.def)
	enclosingTypeName := make([]byte, len(tempName))
	copy(enclosingTypeName, tempName)

	printer := astprinter.NewPrinter(nil)
	m.buf.Reset()
	err := printer.Print(m.temp, nil, m.buf)
	if err != nil {
		return
	}
	data := append(enclosingTypeName, m.buf.Bytes()...)

	m.h.Reset()
	_, _ = m.h.Write(data)
	key := m.h.Sum64()

	i := item{
		selectionSet:  ref,
		ancestor:      ancestor,
		enclosingType: m.w.EnclosingTypeDefinition,
	}

	if s, ok := m.s[key]; ok {
		s.count++
		s.items = append(s.items, i)
		if m.w.Depth > s.depth {
			s.depth = m.w.Depth
		}
		return
	}
	s := &stats{
		count: 1,
		size:  len(data),
		items: []item{i},
		depth: m.w.Depth,
	}
	m.s[key] = s
}
