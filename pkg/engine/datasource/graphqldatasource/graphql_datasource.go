package graphqldatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

type Planner struct {
	v                   *plan.Visitor
	fetch               *resolve.SingleFetch
	printer             astprinter.Printer
	operation           *ast.Document
	nodes               []ast.Node
	buf                 *bytes.Buffer
	operationNormalizer *astnormalization.OperationNormalizer
	URL                 []byte
	variables           []byte
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
	if p.operationNormalizer == nil {
		p.operationNormalizer = astnormalization.NewNormalizer(true)
	}
	p.nodes = p.nodes[:0]
	p.URL = nil
	p.variables = nil
}

func (p *Planner) EnterField(ref int) {

	isRootField, config := p.v.IsRootField(ref)

	if isRootField && !p.v.CurrentObjectHasFetch() {

		p.URL = config.Attributes.ValueForKey("url")

		bufferID := p.v.NextBufferID()
		p.v.SetBufferIDForCurrentFieldSet(bufferID)
		p.fetch = &resolve.SingleFetch{
			BufferId: bufferID,
		}
		p.v.SetCurrentObjectFetch(p.fetch, config)
		if len(p.operation.RootNodes) == 0 {
			set := p.operation.AddSelectionSet()
			definition := p.operation.AddOperationDefinitionToRootNodes(ast.OperationDefinition{
				OperationType: p.v.Operation.OperationDefinitions[p.v.Ancestors[0].Ref].OperationType,
				SelectionSet:  set.Ref,
				HasSelections: true,
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

	if config == nil {
		return
	}
	if arguments := config.Attributes.ValueForKey("arguments"); arguments != nil {
		p.configureFieldArguments(field.Ref, ref, arguments)
	}
}

func (p *Planner) configureFieldArguments(upstreamField, downstreamField int, arguments []byte) {
	var config ArgumentsConfig
	err := json.Unmarshal(arguments, &config)
	if err != nil {
		log.Fatal(err)
		return
	}
	fieldName := p.v.Operation.FieldNameString(downstreamField)
	for i := range config.Fields {
		if config.Fields[i].FieldName != fieldName {
			continue
		}
		for j := range config.Fields[i].Arguments {
			p.applyFieldArgument(upstreamField, downstreamField, config.Fields[i].Arguments[j])
		}
	}
}

func (p *Planner) applyFieldArgument(upstreamField, downstreamField int, arg Argument) {
	switch arg.Source {
	case FieldArgument:
		if fieldArgument, ok := p.v.Operation.FieldArgument(downstreamField, arg.Name); ok { // TODO: doesn't work with multi path args
			value := p.v.Operation.ArgumentValue(fieldArgument)
			if value.Kind != ast.ValueKindVariable {
				return
			}
			variableName := p.v.Operation.VariableValueNameBytes(value.Ref)
			variableNameStr := p.v.Operation.VariableValueNameString(value.Ref)

			contextVariableName := p.fetch.Variables.AddVariable(&resolve.ContextVariable{Path: append([]string{variableNameStr}, arg.SourcePath...)})
			p.variables, _ = sjson.SetRawBytes(p.variables, variableNameStr, contextVariableName)

			variableValueRef, argRef := p.operation.AddVariableValueArgument(arg.Name, variableName)
			p.operation.AddArgumentToField(upstreamField, argRef)

			for _, i := range p.v.Operation.OperationDefinitions[p.v.Ancestors[0].Ref].VariableDefinitions.Refs {
				ref := p.v.Operation.VariableDefinitions[i].VariableValue.Ref
				if !p.v.Operation.VariableValueNameBytes(ref).Equals(variableName) {
					continue
				}
				importedType := p.v.Importer.ImportType(p.v.Operation.VariableDefinitions[i].Type, p.v.Operation, p.operation)
				p.operation.AddVariableDefinitionToOperationDefinition(p.nodes[0].Ref, variableValueRef, importedType)
			}
		}
	case ObjectField:
	}
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
	p.operationNormalizer.NormalizeOperation(p.operation, definition, p.v.Report)
	buf := &bytes.Buffer{}
	err := p.printer.Print(p.operation, nil, buf)
	if err != nil {
		return
	}
	if p.variables != nil {
		p.fetch.Input, _ = sjson.SetRawBytes(p.fetch.Input, "body.variables", p.variables)
	}
	p.fetch.Input, _ = sjson.SetRawBytes(p.fetch.Input, "body.query", append([]byte{'"'}, append(buf.Bytes(), '"')...))
	p.fetch.Input, _ = sjson.SetRawBytes(p.fetch.Input, "url", append([]byte{'"'}, append(p.URL, '"')...))
	p.fetch.DataSource = &Source{
		Client: http.Client{
			Timeout: time.Second * 10,
		},
	}
}

type Source struct {
	Client http.Client
}

func (s *Source) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	var (
		url, body  []byte
		inputPaths = [][]string{
			{"url"},
			{"body"},
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
			body = bytes
		}
	}, inputPaths...)

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

func ArgumentsConfigJSON(config ArgumentsConfig) []byte {
	out, _ := json.Marshal(config)
	return out
}

type ArgumentsConfig struct {
	Fields []FieldConfig
}

type FieldConfig struct {
	FieldName string
	Arguments []Argument
}

type Argument struct {
	Name       []byte
	Source     ArgumentSource
	SourcePath []string
}

type ArgumentSource string

const (
	ObjectField   ArgumentSource = "objectField"
	FieldArgument ArgumentSource = "fieldArgument"
)
