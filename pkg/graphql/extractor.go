package graphql

import (
	"bytes"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
)

type Extractor struct {
	walker  *astvisitor.Walker
	visitor *requestVisitor
}

func NewExtractor() *Extractor {
	walker := astvisitor.NewWalker(48)
	visitor := requestVisitor{
		Walker: &walker,
	}

	walker.RegisterEnterFieldVisitor(&visitor)
	walker.RegisterEnterOperationVisitor(&visitor)

	return &Extractor{
		walker:  &walker,
		visitor: &visitor,
	}
}

func (e *Extractor) ExtractFieldsFromRequest(request *Request, schema *Schema, report *operationreport.Report, data RequestTypes) {
	if !request.IsNormalized() {
		result, err := request.Normalize(schema)
		if err != nil {
			report.AddInternalError(err)
		}

		if !result.Successful {
			report.AddInternalError(result.Errors)
		}
	}

	e.visitor.data = data
	e.visitor.operation = &request.document
	e.visitor.definition = &schema.document
	e.walker.Walk(&request.document, &schema.document, report)
}

func (e *Extractor) ExtractFieldsFromRequestSingleOperation(request *Request, schema *Schema, report *operationreport.Report, data RequestTypes) {
	e.visitor.singleOperation = true
	e.visitor.operationName = request.OperationName

	e.ExtractFieldsFromRequest(request, schema, report, data)
}

type requestVisitor struct {
	singleOperation, shouldSkip bool
	operationName               string
	*astvisitor.Walker
	operation, definition *ast.Document
	data                  RequestTypes
}

func (p *requestVisitor) EnterOperationDefinition(ref int) {
	if !p.singleOperation {
		return
	}
	if p.operationName == "" && ref == 0 {
		p.shouldSkip = false
		return
	} else if p.operationName == "" && ref != 0 {
		p.shouldSkip = true
		return
	}

	opName := p.operation.OperationDefinitionNameBytes(ref)
	if bytes.Equal(opName, []byte(p.operationName)) {
		p.shouldSkip = false
	} else {
		p.shouldSkip = true
	}
}

func (p *requestVisitor) EnterField(ref int) {
	if p.shouldSkip {
		return
	}
	fieldName := p.operation.FieldNameString(ref)
	parentTypeName := p.definition.NodeNameString(p.EnclosingTypeDefinition)

	t, ok := p.data[parentTypeName]
	if !ok {
		t = make(RequestFields)
	}

	if _, ok := t[fieldName]; !ok {
		t[fieldName] = struct{}{}
	}

	p.data[parentTypeName] = t
}
