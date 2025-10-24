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

type addRequiredFieldsConfiguration struct {
	operation, definition        *ast.Document
	operationSelectionSetRef     int
	isTypeNameForEntityInterface bool
	isKey                        bool
	allowTypename                bool
	typeName                     string
	fieldSet                     string

	// addTypenameInNestedSelections controls forced addition of __typename to nested selection sets
	// used by "requires" keys, not only when fragments are present.
	addTypenameInNestedSelections bool
}

type AddRequiredFieldsResult struct {
	skipFieldRefs     []int
	requiredFieldRefs []int
	modifiedFieldRefs []int
	remappedPaths     map[string]string // path in a requirements to field name
}

func addRequiredFields(config *addRequiredFieldsConfiguration) (out AddRequiredFieldsResult, report *operationreport.Report) {
	key, report := RequiredFieldsFragment(config.typeName, config.fieldSet, config.allowTypename)
	if report.HasErrors() {
		return out, report
	}

	walker := astvisitor.NewWalkerWithID(4, "RequiredFieldsVisitor")

	visitor := &requiredFieldsVisitor{
		Walker:            &walker,
		config:            config,
		key:               key,
		importer:          &astimport.Importer{},
		skipFieldRefs:     make([]int, 0, 2),
		requiredFieldRefs: make([]int, 0, 2),
		mapping:           make(map[string]string),
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterInlineFragmentVisitor(visitor)

	walker.Walk(key, config.definition, report)

	return AddRequiredFieldsResult{
		skipFieldRefs:     visitor.skipFieldRefs,
		requiredFieldRefs: visitor.requiredFieldRefs,
		modifiedFieldRefs: visitor.modifiedFieldRefs,
		remappedPaths:     visitor.mapping,
	}, report
}

type requiredFieldsVisitor struct {
	*astvisitor.Walker

	OperationNodes []ast.Node
	config         *addRequiredFieldsConfiguration
	importer       *astimport.Importer
	key            *ast.Document

	skipFieldRefs     []int
	requiredFieldRefs []int
	modifiedFieldRefs []int
	mapping           map[string]string // path in a requirements to field name
}

func (v *requiredFieldsVisitor) EnterDocument(_, _ *ast.Document) {
	v.OperationNodes = make([]ast.Node, 0, 3)
	v.OperationNodes = append(v.OperationNodes,
		ast.Node{Kind: ast.NodeKindSelectionSet, Ref: v.config.operationSelectionSetRef})
}

func (v *requiredFieldsVisitor) EnterInlineFragment(ref int) {
	typeName := v.key.InlineFragmentTypeConditionName(ref)

	inlineFragmentRef := v.config.operation.AddInlineFragment(ast.InlineFragment{
		TypeCondition: ast.TypeCondition{
			Type: v.config.operation.AddNamedType(typeName),
		},
	})

	operationNode := v.OperationNodes[len(v.OperationNodes)-1]
	if operationNode.Kind != ast.NodeKindSelectionSet {
		v.Walker.StopWithInternalErr(fmt.Errorf("expected operation node to be of kind selection set, got %s", operationNode.Kind))
		return
	}

	v.config.operation.AddSelection(operationNode.Ref, ast.Selection{
		Kind: ast.SelectionKindInlineFragment,
		Ref:  inlineFragmentRef,
	})

	v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindInlineFragment, Ref: inlineFragmentRef})
}

func (v *requiredFieldsVisitor) LeaveInlineFragment(ref int) {
	v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
}

func (v *requiredFieldsVisitor) EnterSelectionSet(ref int) {
	if v.Walker.Depth == 2 {
		return
	}
	operationNode := v.OperationNodes[len(v.OperationNodes)-1]

	keySelectionSetHasFragments := len(v.key.SelectionSetInlineFragmentSelections(ref)) > 0

	if operationNode.Kind == ast.NodeKindField {
		enforcedTypename := v.config.addTypenameInNestedSelections && !v.config.isKey
		if fieldSelectionSetRef, ok := v.config.operation.FieldSelectionSet(operationNode.Ref); ok {
			selectionSetNode := ast.Node{Kind: ast.NodeKindSelectionSet, Ref: fieldSelectionSetRef}
			if (keySelectionSetHasFragments || enforcedTypename) &&
				!v.selectionSetHasTypeNameSelection(fieldSelectionSetRef) {
				v.addTypenameSelection(fieldSelectionSetRef)
			}
			v.OperationNodes = append(v.OperationNodes, selectionSetNode)
			return
		}

		selectionSetNode := v.config.operation.AddSelectionSet()
		if keySelectionSetHasFragments || enforcedTypename {
			v.addTypenameSelection(selectionSetNode.Ref)
		}

		v.config.operation.Fields[operationNode.Ref].HasSelections = true
		v.config.operation.Fields[operationNode.Ref].SelectionSet = selectionSetNode.Ref
		v.OperationNodes = append(v.OperationNodes, selectionSetNode)
		return
	}

	// operation node kind InlineFragment
	selectionSetNode := v.config.operation.AddSelectionSet()
	v.config.operation.InlineFragments[operationNode.Ref].HasSelections = true
	v.config.operation.InlineFragments[operationNode.Ref].SelectionSet = selectionSetNode.Ref
	v.OperationNodes = append(v.OperationNodes, selectionSetNode)
}

func (v *requiredFieldsVisitor) selectionSetHasTypeNameSelection(operationSelectionSetRef int) bool {
	exists, _ := v.config.operation.SelectionSetHasFieldSelectionWithExactName(operationSelectionSetRef, typeNameFieldBytes)
	return exists
}

// addTypenameSelection adds __typename selection to the operation when the key/requires selection set has inline fragments
func (v *requiredFieldsVisitor) addTypenameSelection(operationSelectionSetRef int) {
	field := v.config.operation.AddField(ast.Field{
		Name: v.config.operation.Input.AppendInputString("__typename"),
	})
	v.skipFieldRefs = append(v.skipFieldRefs, field.Ref)

	v.config.operation.AddSelection(operationSelectionSetRef, ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	})
}

func (v *requiredFieldsVisitor) LeaveSelectionSet(ref int) {
	if v.Walker.Depth == 0 {
		return
	}

	v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
}

func (v *requiredFieldsVisitor) EnterField(ref int) {
	if v.config.isKey {
		v.handleKeyField(ref)
		return
	}

	v.handleRequiredField(ref)
}

func (v *requiredFieldsVisitor) handleRequiredField(ref int) {
	fieldName := v.key.FieldNameBytes(ref)
	isTypeName := bytes.Equal(fieldName, typeNameFieldBytes)

	// we need to add alias if operation has such field and:
	// - the field is not a leaf
	// - the field has arguments
	isLeafField := !v.key.FieldHasSelections(ref)
	needAlias := v.key.FieldHasArguments(ref)

	selectionSetRef := v.OperationNodes[len(v.OperationNodes)-1].Ref
	operationHasField, operationFieldRef := v.config.operation.SelectionSetHasFieldSelectionWithExactName(selectionSetRef, fieldName)

	if operationHasField && !needAlias {
		// we are skipping adding __typename field to the required fields,
		// because we want to depend only on the regular key fields, not the __typename field
		// for entity interface we need real typename, so we use this dependency
		if !isTypeName || v.config.isTypeNameForEntityInterface {
			v.storeRequiredFieldRef(operationFieldRef)
		}

		// do not add required field if the field is already present in the operation with the same name
		// but add an operation node from operation if the field has selections
		if !v.config.operation.FieldHasSelections(operationFieldRef) {
			return
		}

		v.modifiedFieldRefs = append(v.modifiedFieldRefs, operationFieldRef)
		v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: operationFieldRef})
		return
	}

	fieldNode := v.addRequiredField(ref, fieldName, selectionSetRef, operationHasField && needAlias)
	if !isLeafField {
		v.OperationNodes = append(v.OperationNodes, fieldNode)
	}
}

func (v *requiredFieldsVisitor) handleKeyField(ref int) {
	fieldName := v.key.FieldNameBytes(ref)
	isTypeName := bytes.Equal(fieldName, typeNameFieldBytes)
	isLeafField := !v.key.FieldHasSelections(ref)

	selectionSetRef := v.OperationNodes[len(v.OperationNodes)-1].Ref
	operationHasField, operationFieldRef := v.config.operation.SelectionSetHasFieldSelectionWithExactName(selectionSetRef, fieldName)
	if operationHasField {
		// we are skipping adding __typename field to the required fields,
		// because we want to depend only on the regular key fields, not the __typename field
		// for entity interface we need real typename, so we use this dependency
		if !isTypeName {
			v.storeRequiredFieldRef(operationFieldRef)
		}

		// do not add required field if the field is already present in the operation with the same name
		// but add an operation node from operation if the field has selections
		if isLeafField {
			return
		}

		v.modifiedFieldRefs = append(v.modifiedFieldRefs, operationFieldRef)
		v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: operationFieldRef})
		return
	}

	fieldNode := v.addRequiredField(ref, fieldName, selectionSetRef, false)
	if !isLeafField {
		v.OperationNodes = append(v.OperationNodes, fieldNode)
	}
}

func (v *requiredFieldsVisitor) LeaveField(ref int) {
	if v.key.FieldHasSelections(ref) {
		v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
	}
}

func (v *requiredFieldsVisitor) storeRequiredFieldRef(fieldRef int) {
	v.requiredFieldRefs = append(v.requiredFieldRefs, fieldRef)
}

func (v *requiredFieldsVisitor) addRequiredField(keyRef int, fieldName ast.ByteSlice, selectionSet int, addAlias bool) ast.Node {
	field := ast.Field{
		Name:         v.config.operation.Input.AppendInputBytes(fieldName),
		SelectionSet: ast.InvalidRef,
	}

	if addAlias {
		aliasName := bytes.NewBuffer([]byte("__internal_"))
		aliasName.Write(fieldName)
		fullAliasName := aliasName.Bytes()

		field.Alias = ast.Alias{
			IsDefined: true,
			Name:      v.config.operation.Input.AppendInputBytes(fullAliasName),
		}

		currentPath := v.Walker.Path.DotDelimitedString() + "." + string(fieldName)
		v.mapping[currentPath] = string(fullAliasName)
	}

	addedField := v.config.operation.AddField(field)

	if v.key.FieldHasArguments(keyRef) {
		importedArgs := v.importer.ImportArguments(v.key.Fields[keyRef].Arguments.Refs, v.key, v.config.operation)

		for _, arg := range importedArgs {
			v.config.operation.AddArgumentToField(addedField.Ref, arg)
		}
	}

	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  addedField.Ref,
	}
	v.config.operation.AddSelection(selectionSet, selection)

	v.skipFieldRefs = append(v.skipFieldRefs, addedField.Ref)

	// we are skipping adding __typename field to the required fields,
	// because we want to depend only on the regular key fields, not the __typename field
	if !bytes.Equal(fieldName, typeNameFieldBytes) || (bytes.Equal(fieldName, typeNameFieldBytes) && v.config.isTypeNameForEntityInterface) {
		v.storeRequiredFieldRef(addedField.Ref)
	}

	return addedField
}
