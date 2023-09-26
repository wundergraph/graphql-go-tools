package plan

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type providesInput struct {
	key, definition *ast.Document
	report          *operationreport.Report
	parentPath      string
	DSHash          DSHash
}

func providesSuggestions(input *providesInput) []NodeSuggestion {
	walker := astvisitor.NewWalker(48)

	visitor := &providesVisitor{
		walker: &walker,
		input:  input,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterFragmentDefinitionVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)

	walker.Walk(input.key, input.definition, input.report)

	return visitor.suggestions
}

type providesVisitor struct {
	walker      *astvisitor.Walker
	input       *providesInput
	suggestions []NodeSuggestion
	pathPrefix  string
}

func (v *providesVisitor) EnterFragmentDefinition(ref int) {
	v.pathPrefix = v.input.key.FragmentDefinitionTypeNameString(ref)
}

func (v *providesVisitor) EnterDocument(_, _ *ast.Document) {
	v.suggestions = make([]NodeSuggestion, 0, 8)
}

func (v *providesVisitor) EnterField(ref int) {
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.input.definition)
	fieldName := v.input.key.FieldNameUnsafeString(ref)

	parentPath := v.input.parentPath + strings.TrimPrefix(v.walker.Path.DotDelimitedString(), v.pathPrefix)
	currentPath := parentPath + "." + fieldName

	v.suggestions = append(v.suggestions, NodeSuggestion{
		TypeName:       typeName,
		FieldName:      fieldName,
		DataSourceHash: v.input.DSHash,
		Path:           currentPath,
		ParentPath:     parentPath,
	})
}
