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
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

type Planner struct {
	v             *plan.Visitor
	fetch         *resolve.SingleFetch
	upstreamQuery *ast.Document
	upstreamNodes []ast.Node
}

func (p *Planner) Register(visitor *plan.Visitor) {
	p.v = visitor
	visitor.RegisterFieldVisitor(p)
	visitor.RegisterDocumentVisitor(p)
}

func (p *Planner) EnterDocument(operation, definition *ast.Document) {
	if p.upstreamQuery == nil {
		p.upstreamQuery = ast.NewDocument()
	} else {
		p.upstreamQuery.Input.Reset()
	}
	p.upstreamNodes = p.upstreamNodes[:0]
}

func (p *Planner) isRootField() bool {
	for i := range p.v.Ancestors {
		if p.v.Ancestors[i].Kind == ast.NodeKindField {
			return false
		}
	}
	return true
}

func (p *Planner) EnterField(ref int) {
	isRootField := p.isRootField()
	fieldName := p.v.Operation.FieldNameString(ref)
	fmt.Printf("Planner - field: %s, path: %s, isRootField: %t\n", fieldName, p.v.Path.String(), isRootField)

	if isRootField {
		if p.v.CurrentObject.Fetch == nil {
			p.fetch = &resolve.SingleFetch{}
			p.v.CurrentObject.Fetch = p.fetch
		}
	}
}

func (p *Planner) LeaveField(ref int) {

}

func (p *Planner) LeaveDocument(operation, definition *ast.Document) {
	p.fetch.Input = []byte("dummy")
}

type Source struct {
	Client http.Client
}

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	var (
		url, query, variables []byte
		inputPaths            = [][]string{
			{"url"},
			{"query"},
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
	body, err = sjson.SetRawBytes(body, "query", query)
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
