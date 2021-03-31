package federation

//import (
//	"github.com/jensneuse/graphql-go-tools/pkg/ast"
//	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
//)
//
//
//type ComposeVisitor struct {
//	*astvisitor.Walker
//	operation *ast.Document
//}
//
//func (c *ComposeVisitor) AttachToWalker(walker *astvisitor.Walker) {
//	c.Walker = walker
//	walker.RegisterOperationDefinitionVisitor(c)
//}
//
//func (c *ComposeVisitor) EnterDocument(operation, definition *ast.Document) {
//	c.operation = operation
//}
//
//func (c *ComposeVisitor) EnterOperationDefinition(ref int) {
//
//}