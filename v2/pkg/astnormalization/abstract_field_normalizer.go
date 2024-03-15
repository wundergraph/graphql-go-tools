package astnormalization

import (
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

type AbstractFieldNormalizer struct {
	*astvisitor.Walker

	operation  *ast.Document
	definition *ast.Document

	targetFieldRef int
	allow          bool
}

func NewAbstractFieldNormalizer(operation *ast.Document, definition *ast.Document, fieldRef int) *AbstractFieldNormalizer {
	walker := astvisitor.NewWalker(48)

	normalizer := &AbstractFieldNormalizer{
		Walker:         &walker,
		operation:      operation,
		definition:     definition,
		targetFieldRef: fieldRef,
	}

	walker.RegisterFieldVisitor(normalizer)
	walker.SetVisitorFilter(normalizer)

	mergeInlineFragmentSelections(&walker)
	mergeFieldSelections(&walker)
	deduplicateFields(&walker)

	return normalizer
}

func (v *AbstractFieldNormalizer) Normalize() error {
	v.allow = false

	report := &operationreport.Report{}
	v.Walk(v.operation, v.definition, report)
	if report.HasErrors() {
		return report
	}

	return nil
}

func (v *AbstractFieldNormalizer) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor interface{}, skipFor astvisitor.SkipVisitors) bool {
	if visitor == v {
		return true
	}

	switch kind {
	case astvisitor.EnterDocument, astvisitor.LeaveDocument:
		return true
	}

	return v.allow
}

func (v *AbstractFieldNormalizer) EnterField(ref int) {
	if ref == v.targetFieldRef {
		v.allow = true
	}
}

func (v *AbstractFieldNormalizer) LeaveField(ref int) {
	if ref == v.targetFieldRef {
		v.allow = false
		v.Stop()
	}
}
