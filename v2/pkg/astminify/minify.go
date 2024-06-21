package astminify

import (
	"bytes"
	"errors"
	"hash"
	"slices"
	"strings"

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
	Pretty  bool
	SortAST bool
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
	if m.opts.SortAST {
		m.sortAst(m.temp)
		m.sortAst(m.out)
	}
}

func (m *Minifier) sortAst(doc *ast.Document) {
	for i := range doc.SelectionSets {
		slices.SortFunc(doc.SelectionSets[i].SelectionRefs, func(a, b int) int {
			left := doc.Selections[a]
			right := doc.Selections[b]
			if left.Kind == ast.SelectionKindInlineFragment && right.Kind == ast.SelectionKindField {
				return 1
			}
			if left.Kind == ast.SelectionKindField && right.Kind == ast.SelectionKindInlineFragment {
				return -1
			}
			if left.Kind == ast.SelectionKindField && right.Kind == ast.SelectionKindField {
				return strings.Compare(doc.FieldNameString(left.Ref), doc.FieldNameString(right.Ref))
			}
			if left.Kind == ast.SelectionKindInlineFragment && right.Kind == ast.SelectionKindInlineFragment {
				return strings.Compare(doc.InlineFragmentTypeConditionNameString(left.Ref), doc.InlineFragmentTypeConditionNameString(right.Ref))
			}
			return 0
		})
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
	slices.SortStableFunc(replacements, func(a, b *stats) int {
		if a.depth == b.depth {
			if b.size == a.size {
				return strings.Compare(b.enclosingTypeName, a.enclosingTypeName)
			}
			return b.size - a.size
		}
		return b.depth - a.depth
	})
	for _, s := range replacements {
		m.replaceItems(s)
	}
}

func (m *Minifier) replaceItems(s *stats) {

	fragmentName := m.out.Input.AppendInputString(m.fragmentName())
	typeDef := ast.Type{
		TypeKind: ast.TypeKindNamed,
		Name:     m.out.Input.AppendInputString(s.enclosingTypeName),
	}
	m.out.Types = append(m.out.Types, typeDef)
	typeRef := len(m.out.Types) - 1

	frag := ast.FragmentDefinition{
		Name:          fragmentName,
		Directives:    m.out.CopyDirectiveList(s.items[0].directives),
		HasDirectives: len(s.items[0].directives.Refs) > 0,
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

func (m *minifyVisitor) EnterFragmentDefinition(_ int) {
	m.w.SkipNode()
}

type stats struct {
	count             int
	size              int
	items             []item
	depth             int
	enclosingTypeName string
}

type item struct {
	selectionSet int
	directives   ast.DirectiveList
	ancestor     ast.Node
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

	i := item{
		selectionSet: ref,
		ancestor:     ancestor,
	}

	switch ancestor.Kind {
	case ast.NodeKindField:
		i.directives = m.out.Fields[ancestor.Ref].Directives
	case ast.NodeKindInlineFragment:
		i.directives = m.out.InlineFragments[ancestor.Ref].Directives
	}

	m.h.Reset()
	// write data to hash
	_, _ = m.h.Write(data)
	// print directives to hash
	// this ensures that selection sets with different directives are not merged
	for _, j := range i.directives.Refs {
		_ = m.out.PrintDirective(j, m.h)
	}
	key := m.h.Sum64()

	if s, ok := m.s[key]; ok {
		s.count++
		s.items = append(s.items, i)
		if m.w.Depth > s.depth {
			s.depth = m.w.Depth
		}
		return
	}
	s := &stats{
		count:             1,
		size:              len(data),
		items:             []item{i},
		depth:             m.w.Depth,
		enclosingTypeName: string(enclosingTypeName),
	}
	m.s[key] = s
}
