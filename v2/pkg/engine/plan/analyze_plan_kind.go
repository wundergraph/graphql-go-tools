package plan

import (
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

func AnalyzePlanKind(operation, definition *ast.Document, operationName string) (operationType ast.OperationType, streaming bool, error error) {
	walker := astvisitor.NewWalker(48)
	visitor := &planKindVisitor{
		Walker:        &walker,
		operationName: operationName,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	walker.RegisterEnterDirectiveVisitor(visitor)

	var report operationreport.Report
	walker.Walk(operation, definition, &report)
	if report.HasErrors() {
		return ast.OperationTypeUnknown, false, report
	}
	operationType = visitor.operationType
	streaming = visitor.hasDeferDirective || visitor.hasStreamDirective
	return
}

type planKindVisitor struct {
	*astvisitor.Walker
	operation, definition                 *ast.Document
	operationName                         string
	hasStreamDirective, hasDeferDirective bool
	operationType                         ast.OperationType
}

func (p *planKindVisitor) EnterDirective(ref int) {
	directiveName := p.operation.DirectiveNameString(ref)
	ancestor := p.Ancestors[len(p.Ancestors)-1]
	switch ancestor.Kind {
	case ast.NodeKindField:
		switch directiveName {
		case "defer":
			p.hasDeferDirective = true
		case "stream":
			p.hasStreamDirective = true
		}
	}
}

func (p *planKindVisitor) EnterOperationDefinition(ref int) {
	name := p.operation.OperationDefinitionNameString(ref)
	if p.operationName != name {
		p.SkipNode()
		return
	}
	p.operationType = p.operation.OperationDefinitions[ref].OperationType
}

func (p *planKindVisitor) EnterDocument(operation, definition *ast.Document) {
	p.operation, p.definition = operation, definition
}
