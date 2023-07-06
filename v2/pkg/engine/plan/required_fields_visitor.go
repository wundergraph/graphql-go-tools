package plan

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

type skipFieldData struct {
	selectionSetRef int
	fieldConfig     *FieldConfiguration
	requiredField   string
	fieldPath       string
}

type requiredFieldsVisitor struct {
	operation, definition *ast.Document
	walker                *astvisitor.Walker
	config                *Configuration
	operationName         string
	skipFieldPaths        []string
	// selectedFieldPaths is a set of all explicitly selected field paths
	selectedFieldPaths map[string]struct{}
	// skipFieldDataPaths is to prevent appending duplicate skipFieldData to potentialSkipFieldDatas
	skipFieldDataPaths map[string]struct{}
	// potentialSkipFieldDatas is used in LeaveDocument to determine whether a required field should be skipped
	// Must be a slice to preserve field order, which is why duplicates are handled with a set
	potentialSkipFieldDatas []*skipFieldData
}

func (r *requiredFieldsVisitor) EnterDocument(_, _ *ast.Document) {
	r.skipFieldPaths = r.skipFieldPaths[:0]
	r.selectedFieldPaths = make(map[string]struct{})
	r.potentialSkipFieldDatas = make([]*skipFieldData, 0)
}

func (r *requiredFieldsVisitor) EnterField(ref int) {
	typeName := r.walker.EnclosingTypeDefinition.NameString(r.definition)
	fieldName := r.operation.FieldNameUnsafeString(ref)
	fieldConfig := r.config.Fields.ForTypeField(typeName, fieldName)
	path := r.walker.Path.DotDelimitedString()
	if fieldConfig == nil {
		// Record all explicitly selected fields
		// A field selected on an interface will have the same field path as a fragment on an object
		// LeaveDocument uses this record to ensure only required fields that were not explicitly selected are skipped
		r.selectedFieldPaths[fmt.Sprintf("%s.%s", path, fieldName)] = struct{}{}
		return
	}
	if len(fieldConfig.RequiresFields) == 0 {
		return
	}
	selectionSet := r.walker.Ancestors[len(r.walker.Ancestors)-1]
	if selectionSet.Kind != ast.NodeKindSelectionSet {
		return
	}
	for _, requiredField := range fieldConfig.RequiresFields {
		requiredFieldPath := fmt.Sprintf("%s.%s", path, requiredField)
		// Prevent adding duplicates to the slice (order is necessary; hence, a separate map)
		if _, ok := r.skipFieldDataPaths[requiredFieldPath]; ok {
			continue
		}
		// For each required field, collect the data required to handle (in LeaveDocument) whether we should skip it
		data := &skipFieldData{
			selectionSetRef: selectionSet.Ref,
			fieldConfig:     fieldConfig,
			requiredField:   requiredField,
			fieldPath:       requiredFieldPath,
		}
		r.potentialSkipFieldDatas = append(r.potentialSkipFieldDatas, data)
	}
}

func (r *requiredFieldsVisitor) handleRequiredField(selectionSet int, requiredFieldName, fullFieldPath string) {
	for _, ref := range r.operation.SelectionSets[selectionSet].SelectionRefs {
		selection := r.operation.Selections[ref]
		if selection.Kind != ast.SelectionKindField {
			continue
		}
		name := r.operation.FieldAliasOrNameString(selection.Ref)
		if name == requiredFieldName {
			// already exists
			return
		}
	}
	r.addRequiredField(requiredFieldName, selectionSet, fullFieldPath)
}

func (r *requiredFieldsVisitor) addRequiredField(fieldName string, selectionSet int, fullFieldPath string) {
	field := ast.Field{
		Name: r.operation.Input.AppendInputString(fieldName),
	}
	addedField := r.operation.AddField(field)
	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  addedField.Ref,
	}
	r.operation.AddSelection(selectionSet, selection)
	r.skipFieldPaths = append(r.skipFieldPaths, fullFieldPath)
}

func (r *requiredFieldsVisitor) EnterOperationDefinition(ref int) {
	operationName := r.operation.OperationDefinitionNameString(ref)
	if r.operationName != operationName {
		r.walker.SkipNode()
		return
	}
}

func (r *requiredFieldsVisitor) LeaveDocument(_, _ *ast.Document) {
	for _, data := range r.potentialSkipFieldDatas {
		path := data.fieldPath
		// If a field was not explicitly selected, skip it
		if _, ok := r.selectedFieldPaths[path]; !ok {
			r.handleRequiredField(data.selectionSetRef, data.requiredField, path)
		}
	}
}
