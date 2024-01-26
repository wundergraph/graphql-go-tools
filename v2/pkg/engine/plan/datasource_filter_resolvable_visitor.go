package plan

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

type nodesResolvableVisitor struct {
	operation  *ast.Document
	definition *ast.Document
	walker     *astvisitor.Walker

	nodes *NodeSuggestions
}

func (f *nodesResolvableVisitor) EnterField(ref int) {
	typeName := f.walker.EnclosingTypeDefinition.NameString(f.definition)
	fieldName := f.operation.FieldNameUnsafeString(ref)
	fieldAliasOrName := f.operation.FieldAliasOrNameString(ref)

	isTypeName := fieldName == typeNameField
	isUnionParent := f.walker.EnclosingTypeDefinition.Kind == ast.NodeKindUnionTypeDefinition

	if isUnionParent && isTypeName {
		// typename field on union parent is always resolvable
		return
	}

	parentPath := f.walker.Path.DotDelimitedString()
	currentPath := parentPath + "." + fieldAliasOrName

	_, found := f.nodes.HasSuggestionForPath(typeName, fieldName, currentPath)
	if !found {
		f.walker.StopWithInternalErr(errors.Wrap(&errOperationFieldNotResolved{TypeName: typeName, FieldName: fieldName, Path: currentPath}, "nodesResolvableVisitor"))
	}
}

type errOperationFieldNotResolved struct {
	TypeName  string
	FieldName string
	Path      string
}

func (e *errOperationFieldNotResolved) Error() string {
	return fmt.Sprintf("could not select the datasource to resolve %s.%s on a path %s", e.TypeName, e.FieldName, e.Path)
}
