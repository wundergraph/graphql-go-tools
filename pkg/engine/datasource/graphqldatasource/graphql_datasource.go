package graphqldatasource

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

type Planner struct {
	v         *plan.Visitor
	fetch     *resolve.SingleFetch
	printer   astprinter.Printer
	operation *ast.Document
	nodes     []ast.Node
	buf       *bytes.Buffer
	URL       []byte
}

func (p *Planner) Register(visitor *plan.Visitor) {
	p.v = visitor
	visitor.RegisterFieldVisitor(p)
	visitor.RegisterDocumentVisitor(p)
	visitor.RegisterSelectionSetVisitor(p)
}

func (p *Planner) EnterDocument(operation, definition *ast.Document) {
	if p.operation == nil {
		p.operation = ast.NewDocument()
	} else {
		p.operation.Reset()
	}
	if p.buf == nil {
		p.buf = &bytes.Buffer{}
	} else {
		p.buf.Reset()
	}
	p.nodes = p.nodes[:0]
	p.URL = nil
}

func (p *Planner) EnterField(ref int) {
	isRootField, config := p.v.IsRootField(ref)
	fieldName := p.v.Operation.FieldNameString(ref)
	fmt.Printf("Planner - field: %s, path: %s, isRootField: %t\n", fieldName, p.v.Path.String(), isRootField)

	if isRootField && p.v.CurrentObject.Fetch == nil {

		p.URL = config.Attributes.ValueForKey("url")

		p.fetch = &resolve.SingleFetch{}
		p.v.CurrentObject.Fetch = p.fetch
		if len(p.operation.RootNodes) == 0 {
			set := p.operation.AddSelectionSet()
			definition := p.operation.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
				OperationType:          p.v.Operation.OperationDefinitions[p.v.Ancestors[0].Ref].OperationType,
				HasVariableDefinitions: false,
				VariableDefinitions:    ast.VariableDefinitionList{},
				HasDirectives:          false,
				Directives:             ast.DirectiveList{},
				SelectionSet:           set.Ref,
				HasSelections:          true,
			})
			p.nodes = append(p.nodes, definition, set)
		}
	}
	field := p.operation.AddField(ast.Field{
		Name: p.operation.Input.AppendInputBytes(p.v.Operation.FieldNameBytes(ref)),
	})
	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  field.Ref,
	}
	p.operation.AddSelection(p.nodes[len(p.nodes)-1].Ref, selection)
	p.nodes = append(p.nodes, field)
}

func (p *Planner) LeaveField(ref int) {
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) EnterSelectionSet(ref int) {
	parent := p.nodes[len(p.nodes)-1]
	set := p.operation.AddSelectionSet()
	switch parent.Kind {
	case ast.NodeKindField:
		p.operation.Fields[parent.Ref].HasSelections = true
		p.operation.Fields[parent.Ref].SelectionSet = set.Ref
	case ast.NodeKindInlineFragment:
		p.operation.InlineFragments[parent.Ref].HasSelections = true
		p.operation.InlineFragments[parent.Ref].SelectionSet = set.Ref
	}
	p.nodes = append(p.nodes, set)
}

func (p *Planner) LeaveSelectionSet(ref int) {
	p.nodes = p.nodes[:len(p.nodes)-1]
}

func (p *Planner) LeaveDocument(operation, definition *ast.Document) {
	buf := &bytes.Buffer{}
	err := p.printer.Print(p.operation, nil, buf)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	p.fetch.Input, err = sjson.SetRawBytes(nil, "query", buf.Bytes())
	p.fetch.Input, err = sjson.SetRawBytes(p.fetch.Input, "url", append([]byte{'"'}, append(p.URL, '"')...))
}

type Source struct {
	Client http.Client
}

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	var (
		url, query, variables []byte
		inputPaths            = [][]string{
			{"url"},
			{"operation"},
			{"variables"},
		}
		responsePaths = [][]string{
			{"error"},
			{"data"},
		}
	)
	jsonparser.EachKey(input, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			url = bytes
		case 1:
			query = append([]byte{'"'}, append(bytes, '"')...)
		case 2:
			variables = bytes
		}
	}, inputPaths...)

	var body []byte
	if len(variables) != 0 {
		body, err = sjson.SetRawBytes(body, "variables", variables)
		if err != nil {
			return err
		}
	}
	body, err = sjson.SetRawBytes(body, "operation", query)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, string(url), bytes.NewReader(body))
	if err != nil {
		return err
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")

	res, err := s.Client.Do(request)
	if err != nil {
		return err
	}
	responseData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	responseData = bytes.ReplaceAll(responseData, literal.BACKSLASH, nil)
	jsonparser.EachKey(responseData, func(i int, bytes []byte, valueType jsonparser.ValueType, err error) {
		switch i {
		case 0:
			bufPair.Errors.Write(bytes)
		case 1:
			bufPair.Data.Write(bytes)
		}
	}, responsePaths...)

	return
}
