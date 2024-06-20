package astminify

import (
	"bytes"
	"errors"
	"fmt"
	"hash"

	"github.com/cespare/xxhash/v2"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
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
	return &Minifier{out: &out, temp: &temp, def: &def}, nil
}

type MinifyOptions struct {
	Pretty    bool
	Threshold int
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
		w:         &walker,
		out:       m.out,
		temp:      m.temp,
		s:         make(map[uint64]*stats),
		buf:       &bytes.Buffer{},
		h:         xxhash.New(),
		threshold: options.Threshold,
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
}

func (m *Minifier) apply(vis *minifyVisitor) {
	for _, s := range vis.s {
		if s.count > 1 {
			content := string(s.content)
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			//fmt.Printf("SelectionSet with %d occurences, size: %d, content: %s\n\n", s.count, s.size, content)
			m.replaceItems(s)
		}
	}
}

func (m *Minifier) replaceItems(s *stats) {
	fragmentName := m.out.Input.AppendInputString(m.fragmentName())
	m.fragmentDefinitionCount++

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
		SelectionSet:  s.items[0].selectionSet,
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
	spread := ast.FragmentSpread{
		FragmentName: fragmentName,
	}
	m.out.FragmentSpreads = append(m.out.FragmentSpreads, spread)
	spreadRef := len(m.out.FragmentSpreads) - 1
	for x, i := range s.items {
		switch i.ancestor.Kind {
		case ast.NodeKindInlineFragment:
			for j := range m.out.Selections {
				if m.out.Selections[j].Kind == ast.SelectionKindInlineFragment && m.out.Selections[j].Ref == s.items[x].selectionSet {
					m.out.Selections[j].Kind = ast.SelectionKindFragmentSpread
					m.out.Selections[j].Ref = spreadRef
					break
				}
			}
		case ast.NodeKindField:
			for j := range m.out.Selections {
				if m.out.Selections[j].Kind == ast.SelectionKindField && m.out.Selections[j].Ref == s.items[x].selectionSet {
					m.out.Selections[j].Kind = ast.SelectionKindFragmentSpread
					m.out.Selections[j].Ref = spreadRef
					break
				}
			}
		default:
			fmt.Printf("Unknown ancestor kind: %s\n", i.ancestor.Kind.String())
		}
		state := unsafeprinter.PrettyPrint(m.out, m.def)
		fmt.Printf("State: %s\n", state)
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

	s         map[uint64]*stats
	threshold int

	buf *bytes.Buffer
	h   hash.Hash64
}

func (m *minifyVisitor) EnterFragmentDefinition(ref int) {
	m.w.SkipNode()
}

type stats struct {
	count   int
	size    int
	items   []item
	content []byte
}

type item struct {
	selectionSet  int
	ancestor      ast.Node
	enclosingType ast.Node
}

func (m *minifyVisitor) EnterSelectionSet(ref int) {

	ancestor := m.w.Ancestor()
	if ancestor.Kind == ast.NodeKindFragmentDefinition {
		return
	}

	/*hasNestedSelections := false

	for _, i := range m.out.SelectionSets[ref].SelectionRefs {
		if m.out.Selections[i].Kind == ast.SelectionKindField {
			if m.out.Fields[m.out.Selections[i].Ref].HasSelections {
				hasNestedSelections = true
				break
			}
		}
	}

	if !hasNestedSelections {
		return
	}
	*/

	m.temp.OperationDefinitions[0].SelectionSet = ref

	printer := astprinter.NewPrinter(nil)
	m.buf.Reset()
	err := printer.Print(m.temp, nil, m.buf)
	if err != nil {
		return
	}
	data := m.buf.Bytes()
	if len(data) < m.threshold {
		return
	}

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
		return
	}
	m.s[key] = &stats{
		count:   1,
		size:    len(data),
		content: data,
		items:   []item{i},
	}
}
