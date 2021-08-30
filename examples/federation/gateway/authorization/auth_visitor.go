package authorization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

const (
	DirectiveName = "hasRole"
	DirectiveArgName = "role"
)

func GetRoles(operation, definition *ast.Document) (roles []string, err error) {
	walker := astvisitor.NewWalker(48)
	report := operationreport.Report{}
	visitor := newAuthVisitor()
	visitor.Register(&walker)
	walker.Walk(operation, definition, &report)
	if report.HasErrors() {
		return nil, report
	}

	return visitor.requiredRoles, nil
}

func newAuthVisitor() *authVisitor {
	return &authVisitor{}
}

type authVisitor struct {
	operation, definition *ast.Document
	*astvisitor.Walker
	requiredRoles []string
}

func (a *authVisitor) Register(walker *astvisitor.Walker) {
	a.Walker = walker
	walker.RegisterEnterDocumentVisitor(a)
	walker.RegisterEnterFieldVisitor(a)
}

func (a *authVisitor) EnterDocument(operation, definition *ast.Document) {
	a.operation, a.definition = operation, definition
}

func (a *authVisitor) EnterField(ref int) {
	definition, ok := a.FieldDefinition(ref)
	if !ok {
		return
	}

	authorizationDirectiveRef, ok := a.definition.FieldDefinitionDirectiveByName(definition, []byte(DirectiveName))
	if !ok {
		return
	}

	value, ok := a.definition.DirectiveArgumentValueByName(authorizationDirectiveRef, []byte(DirectiveArgName))
	if !ok {
		return
	}

	role := a.definition.ValueContentString(value)
	a.requiredRoles = append(a.requiredRoles, role)
}
