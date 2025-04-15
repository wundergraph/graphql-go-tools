package plan

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const (
	requiredFieldsFragmentTemplate             = `fragment Key on %s {%s}`
	requiredFieldsFragmentTemplateWithTypeName = `fragment Key on %s { __typename %s}`
)

func RequiredFieldsFragment(typeName, requiredFields string, includeTypename bool) (*ast.Document, *operationreport.Report) {
	template := requiredFieldsFragmentTemplate
	if includeTypename {
		template = requiredFieldsFragmentTemplateWithTypeName
	}

	key, report := astparser.ParseGraphqlDocumentString(fmt.Sprintf(template, typeName, requiredFields))
	return &key, &report
}

func QueryPlanRequiredFieldsFragment(fieldName, typeName, requiredFields string) (*ast.Document, *operationreport.Report) {
	var fragment string
	if fieldName == "" {
		fragment = fmt.Sprintf("fragment Key on %s { __typename %s }", typeName, requiredFields)
	} else {
		fragment = fmt.Sprintf("fragment Requires_for_%s on %s { %s }", fieldName, typeName, requiredFields)
	}
	key, report := astparser.ParseGraphqlDocumentString(fragment)
	return &key, &report
}

type addRequiredFieldsInput struct {
	key, operation, definition       *ast.Document
	report                           *operationreport.Report
	operationSelectionSet            int
	isTypeNameForEntityInterface     bool
	recordOnlyTopLevelRequiredFields bool
}

type AddRequiredFieldsResult struct {
	skipFieldRefs     []int
	requiredFieldRefs []int
	modifiedFieldRefs []int
}

func addRequiredFields(input *addRequiredFieldsInput) (out AddRequiredFieldsResult) {
	walker := astvisitor.WalkerFromPool()
	defer walker.Release()

	importer := &astimport.Importer{}

	visitor := &requiredFieldsVisitor{
		Walker:            walker,
		input:             input,
		importer:          importer,
		skipFieldRefs:     make([]int, 0, 2),
		requiredFieldRefs: make([]int, 0, 2),
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)

	walker.Walk(input.key, input.definition, input.report)

	return AddRequiredFieldsResult{
		skipFieldRefs:     visitor.skipFieldRefs,
		requiredFieldRefs: visitor.requiredFieldRefs,
		modifiedFieldRefs: visitor.modifiedFieldRefs,
	}
}

func testRequiredFields(input *addRequiredFieldsInput) (allRequiredFieldsAddedToOperation bool, requiredFieldRefs []int) {
	walker := astvisitor.NewWalker(48)

	visitor := &requiredFieldsVisitor{
		Walker:            &walker,
		input:             input,
		skipFieldRefs:     make([]int, 0, 2),
		requiredFieldRefs: make([]int, 0, 2),
		testMode:          true,
		allFieldsPresent:  true,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)

	walker.Walk(input.key, input.definition, input.report)

	return visitor.allFieldsPresent, visitor.requiredFieldRefs
}

type requiredFieldsVisitor struct {
	*astvisitor.Walker
	OperationNodes    []ast.Node
	input             *addRequiredFieldsInput
	importer          *astimport.Importer
	skipFieldRefs     []int
	requiredFieldRefs []int
	modifiedFieldRefs []int

	testMode         bool
	allFieldsPresent bool
}

func (v *requiredFieldsVisitor) EnterDocument(_, _ *ast.Document) {
	v.OperationNodes = make([]ast.Node, 0, 3)
	v.OperationNodes = append(v.OperationNodes,
		ast.Node{Kind: ast.NodeKindSelectionSet, Ref: v.input.operationSelectionSet})
}

func (v *requiredFieldsVisitor) EnterSelectionSet(_ int) {
	if v.Walker.Depth == 2 {
		return
	}
	fieldNode := v.OperationNodes[len(v.OperationNodes)-1]

	if fieldSelectionSetRef, ok := v.input.operation.FieldSelectionSet(fieldNode.Ref); ok {
		selectionSetNode := ast.Node{Kind: ast.NodeKindSelectionSet, Ref: fieldSelectionSetRef}
		v.OperationNodes = append(v.OperationNodes, selectionSetNode)
		return
	}

	selectionSetNode := v.input.operation.AddSelectionSet()
	v.input.operation.Fields[fieldNode.Ref].HasSelections = true
	v.input.operation.Fields[fieldNode.Ref].SelectionSet = selectionSetNode.Ref
	v.OperationNodes = append(v.OperationNodes, selectionSetNode)
}

func (v *requiredFieldsVisitor) LeaveSelectionSet(ref int) {
	if v.Walker.Depth == 0 {
		return
	}

	v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
}

func (v *requiredFieldsVisitor) EnterField(ref int) {
	fieldName := v.input.key.FieldNameBytes(ref)

	selectionSetRef := v.OperationNodes[len(v.OperationNodes)-1].Ref

	operationHasField, operationFieldRef := v.input.operation.SelectionSetHasFieldSelectionWithExactName(selectionSetRef, fieldName)
	if operationHasField {
		// we are skipping adding __typename field to the required fields,
		// because we want to depend only on the regular key fields, not the __typename field
		// for entity interface we need real typename, so we use this dependency
		if !bytes.Equal(fieldName, typeNameFieldBytes) || (bytes.Equal(fieldName, typeNameFieldBytes) && v.input.isTypeNameForEntityInterface) {
			v.storeRequiredFieldRef(operationFieldRef)
		}

		// do not add required field if the field is already present in the operation with the same name
		// but add an operation node from operation if the field has selections
		if !v.input.operation.FieldHasSelections(operationFieldRef) {
			return
		}

		v.modifiedFieldRefs = append(v.modifiedFieldRefs, operationFieldRef)
		v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: operationFieldRef})
		return
	}

	if v.testMode {
		v.allFieldsPresent = false
		v.Walker.Stop()
		return
	}

	fieldNode := v.addRequiredField(ref, fieldName, selectionSetRef)
	if v.input.key.FieldHasSelections(ref) {
		v.OperationNodes = append(v.OperationNodes, fieldNode)
	}
}

func (v *requiredFieldsVisitor) LeaveField(ref int) {
	if v.input.key.FieldHasSelections(ref) {
		v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
	}
}

func (v *requiredFieldsVisitor) storeRequiredFieldRef(fieldRef int) {
	if v.input.recordOnlyTopLevelRequiredFields && len(v.Walker.Ancestors) != 2 {
		return
	}

	// we have to store only field refs which are at the root of the fieldset fragment
	v.requiredFieldRefs = append(v.requiredFieldRefs, fieldRef)
}

func (v *requiredFieldsVisitor) addRequiredField(keyRef int, fieldName ast.ByteSlice, selectionSet int) ast.Node {
	field := ast.Field{
		Name:         v.input.operation.Input.AppendInputBytes(fieldName),
		SelectionSet: ast.InvalidRef,
	}
	addedField := v.input.operation.AddField(field)

	if v.input.key.FieldHasArguments(keyRef) {
		importedArgs := v.importer.ImportArguments(v.input.key.Fields[keyRef].Arguments.Refs, v.input.key, v.input.operation)

		for _, arg := range importedArgs {
			v.input.operation.AddArgumentToField(addedField.Ref, arg)
		}
	}

	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  addedField.Ref,
	}
	v.input.operation.AddSelection(selectionSet, selection)

	v.skipFieldRefs = append(v.skipFieldRefs, addedField.Ref)

	// we are skipping adding __typename field to the required fields,
	// because we want to depend only on the regular key fields, not the __typename field
	if !bytes.Equal(fieldName, typeNameFieldBytes) || (bytes.Equal(fieldName, typeNameFieldBytes) && v.input.isTypeNameForEntityInterface) {
		v.storeRequiredFieldRef(addedField.Ref)
	}

	return addedField
}
