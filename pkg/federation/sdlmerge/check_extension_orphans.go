package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type checkUnresolvedExtensionOrphansVisitor struct {
	*astvisitor.Walker
	document      *ast.Document
	lastObjectRef int
}

func newCheckUnresolvedExtensionOrphansVisitor() *checkUnresolvedExtensionOrphansVisitor {
	return &checkUnresolvedExtensionOrphansVisitor{
		nil,
		nil,
		ast.InvalidRef,
	}
}

func (p *checkUnresolvedExtensionOrphansVisitor) Register(walker *astvisitor.Walker) {
	p.Walker = walker
	walker.RegisterEnterDocumentVisitor(p)
	walker.RegisterEnterEnumTypeExtensionVisitor(p)
	walker.RegisterEnterInputObjectTypeExtensionVisitor(p)
	walker.RegisterEnterInterfaceTypeExtensionVisitor(p)
	walker.RegisterEnterObjectTypeExtensionVisitor(p)
	walker.RegisterEnterScalarTypeExtensionVisitor(p)
	walker.RegisterEnterUnionTypeExtensionVisitor(p)
}

func (p *checkUnresolvedExtensionOrphansVisitor) EnterDocument(operation, _ *ast.Document) {
	p.document = operation
}

func (p *checkUnresolvedExtensionOrphansVisitor) EnterEnumTypeExtension(ref int) {
	p.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(p.document.EnumTypeExtensionNameBytes(ref)))
}

func (p *checkUnresolvedExtensionOrphansVisitor) EnterInputObjectTypeExtension(ref int) {
	p.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(p.document.InputObjectTypeExtensionNameBytes(ref)))
}

func (p *checkUnresolvedExtensionOrphansVisitor) EnterInterfaceTypeExtension(ref int) {
	p.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(p.document.InterfaceTypeExtensionNameBytes(ref)))
}

func (p *checkUnresolvedExtensionOrphansVisitor) EnterObjectTypeExtension(ref int) {
	p.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(p.document.ObjectTypeExtensionNameBytes(ref)))
}

func (p *checkUnresolvedExtensionOrphansVisitor) EnterScalarTypeExtension(ref int) {
	p.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(p.document.ScalarTypeExtensionNameBytes(ref)))
}

func (p *checkUnresolvedExtensionOrphansVisitor) EnterUnionTypeExtension(ref int) {
	p.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(p.document.UnionTypeExtensionNameBytes(ref)))
}
