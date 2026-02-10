package plan

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type areRequiredFieldsProvidedInput struct {
	TypeName       string
	FieldName      string
	RequiredFields string
	Definition     *ast.Document
	ProvidedFields map[string]struct{}
	ParentPath     string
}

func areRequiredFieldsProvided(input areRequiredFieldsProvidedInput) (bool, *operationreport.Report) {
	if len(input.ProvidedFields) == 0 {
		return false, nil
	}

	key, report := RequiredFieldsFragment(input.TypeName, input.RequiredFields, false)
	if report.HasErrors() {
		return false, report
	}

	walker := astvisitor.NewWalkerWithID(4, "RequiredFieldsProvidedVisitor")

	visitor := &requiredFieldsProvidedVisitor{
		walker:      &walker,
		input:       input,
		key:         key,
		allProvided: true,
	}

	walker.RegisterEnterFieldVisitor(visitor)
	walker.Walk(key, input.Definition, report)

	return visitor.allProvided, report
}

type requiredFieldsProvidedVisitor struct {
	walker      *astvisitor.Walker
	input       areRequiredFieldsProvidedInput
	key         *ast.Document
	allProvided bool
}

func (v *requiredFieldsProvidedVisitor) EnterField(ref int) {
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.input.Definition)
	currentFieldName := v.key.FieldNameUnsafeString(ref)

	currentPathWithoutFragments := v.walker.Path.WithoutInlineFragmentNames().DotDelimitedString()
	// remove the parent type name from the path because we are walking a fragment with the required fields
	parentPath := v.input.ParentPath + strings.TrimPrefix(currentPathWithoutFragments, v.input.TypeName)
	currentPath := parentPath + "." + currentFieldName

	key := providedFieldKey(typeName, currentFieldName, currentPath)

	_, provided := v.input.ProvidedFields[key]
	if !provided {
		v.allProvided = false
		v.walker.Stop()
	}
}
