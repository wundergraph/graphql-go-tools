package astnormalization

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization/uploads"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

func extractVariables(walker *astvisitor.Walker) *variablesExtractionVisitor {
	visitor := &variablesExtractionVisitor{
		Walker:       walker,
		uploadFinder: uploads.NewUploadFinder(),
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	return visitor
}

type variablesExtractionVisitor struct {
	*astvisitor.Walker

	operation, definition     *ast.Document
	importer                  astimport.Importer
	skip                      bool
	extractedVariables        [][]byte
	extractedVariableTypeRefs []int
	uploadFinder              *uploads.UploadFinder
	uploadsPath               []uploads.UploadPathMapping
	// fieldArgumentMapping maps field arguments to their variable names.
	// Key: "fieldPath.argumentName" (e.g., "user.posts.limit")
	// Value: variable name after extraction (e.g., "a", "b", "userId")
	fieldArgumentMapping FieldArgumentMapping
}

func (v *variablesExtractionVisitor) EnterArgument(ref int) {
	if len(v.Ancestors) == 0 || v.Ancestors[0].Kind != ast.NodeKindOperationDefinition {
		return
	}

	for i := range v.Ancestors {
		if v.Ancestors[i].Kind == ast.NodeKindDirective {
			return // skip all directives in any case
		}
	}

	inputValueDefinition, ok := v.Walker.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}

	uploadsMapping, err := v.uploadFinder.FindUploads(v.operation, v.definition, v.operation.Input.Variables, ref, inputValueDefinition)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
	v.uploadFinder.Reset()

	if v.operation.Arguments[ref].Value.Kind == ast.ValueKindVariable {
		if len(uploadsMapping) > 0 {
			v.uploadsPath = append(v.uploadsPath, uploadsMapping...)
		}
		// Record the field argument mapping for existing variables
		v.recordFieldArgumentMapping(ref)
		return
	}

	valueBytes, err := v.operation.ValueToJSON(v.operation.Arguments[ref].Value)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
	if exists, name, _ := v.variableExists(valueBytes, inputValueDefinition); exists {
		variable := ast.VariableValue{
			Name: v.operation.Input.AppendInputBytes(name),
		}
		value := v.operation.AddVariableValue(variable)
		v.operation.Arguments[ref].Value.Kind = ast.ValueKindVariable
		v.operation.Arguments[ref].Value.Ref = value
		return
	}
	variableNameBytes := v.operation.GenerateUnusedVariableDefinitionName(v.Ancestors[0].Ref)
	v.operation.Input.Variables, err = sjson.SetRawBytes(v.operation.Input.Variables, unsafebytes.BytesToString(variableNameBytes), valueBytes)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}

	if len(uploadsMapping) > 0 {
		// when we are extracting an input object into a variable and there were uploads inside
		// we have to update the upload path mapping to reflect the new extracted variable path
		for i := range uploadsMapping {
			if uploadsMapping[i].NewUploadPath != "" {
				// we alter a path only when upload was in a nested value
				// NewUploadPath, which returned from upload finder, is relative to the extracted argument "nested.f"
				variableNameString := string(variableNameBytes)
				// in order to replace file map values we prepend it with fully quilifying argument path in variables
				// e.g. variables.a.nested.f
				uploadsMapping[i].NewUploadPath = fmt.Sprintf("variables.%s.%s", variableNameString, uploadsMapping[i].NewUploadPath)
				// update variable name which holds an upload
				uploadsMapping[i].VariableName = variableNameString
			}
			v.uploadsPath = append(v.uploadsPath, uploadsMapping[i])
		}
	}

	v.extractedVariables = append(v.extractedVariables, variableNameBytes)
	v.extractedVariableTypeRefs = append(v.extractedVariableTypeRefs, v.definition.InputValueDefinitions[inputValueDefinition].Type)

	variable := ast.VariableValue{
		Name: v.operation.Input.AppendInputBytes(variableNameBytes),
	}

	v.operation.VariableValues = append(v.operation.VariableValues, variable)

	varRef := len(v.operation.VariableValues) - 1

	v.operation.Arguments[ref].Value.Ref = varRef
	v.operation.Arguments[ref].Value.Kind = ast.ValueKindVariable

	defRef, ok := v.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}

	defType := v.definition.InputValueDefinitions[defRef].Type

	importedDefType := v.importer.ImportType(defType, v.definition, v.operation)

	v.operation.VariableDefinitions = append(v.operation.VariableDefinitions, ast.VariableDefinition{
		VariableValue: ast.Value{
			Kind: ast.ValueKindVariable,
			Ref:  varRef,
		},
		Type: importedDefType,
	})

	newVariableRef := len(v.operation.VariableDefinitions) - 1

	v.operation.OperationDefinitions[v.Ancestors[0].Ref].VariableDefinitions.Refs =
		append(v.operation.OperationDefinitions[v.Ancestors[0].Ref].VariableDefinitions.Refs, newVariableRef)
	v.operation.OperationDefinitions[v.Ancestors[0].Ref].HasVariableDefinitions = true

	// Record the field argument mapping for the newly extracted variable
	v.recordFieldArgumentMappingWithVarName(ref, string(variableNameBytes))
}

func (v *variablesExtractionVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
	v.extractedVariables = v.extractedVariables[:0]
	v.extractedVariableTypeRefs = v.extractedVariableTypeRefs[:0]
	v.fieldArgumentMapping = make(FieldArgumentMapping)
}

// buildFieldPath builds the field path from the walker's ancestors.
// It returns a dot-separated path of field names/aliases (e.g., "user.posts").
func (v *variablesExtractionVisitor) buildFieldPath() string {
	var parts []string
	for _, anc := range v.Ancestors {
		if anc.Kind == ast.NodeKindField {
			alias := v.operation.FieldAliasString(anc.Ref)
			if alias != "" {
				parts = append(parts, alias)
			} else {
				parts = append(parts, v.operation.FieldNameString(anc.Ref))
			}
		}
	}
	return strings.Join(parts, ".")
}

// recordFieldArgumentMapping records the field argument mapping for an existing variable.
func (v *variablesExtractionVisitor) recordFieldArgumentMapping(ref int) {
	fieldPath := v.buildFieldPath()
	if fieldPath == "" {
		return
	}
	argName := v.operation.ArgumentNameString(ref)
	key := fieldPath + "." + argName
	varName := v.operation.VariableValueNameString(v.operation.Arguments[ref].Value.Ref)
	v.fieldArgumentMapping[key] = varName
}

// recordFieldArgumentMappingWithVarName records the field argument mapping for a newly extracted variable.
func (v *variablesExtractionVisitor) recordFieldArgumentMappingWithVarName(ref int, varName string) {
	fieldPath := v.buildFieldPath()
	if fieldPath == "" {
		return
	}
	argName := v.operation.ArgumentNameString(ref)
	key := fieldPath + "." + argName
	v.fieldArgumentMapping[key] = varName
}

func (v *variablesExtractionVisitor) variableExists(variableValue []byte, inputValueDefinition int) (exists bool, name []byte, definition int) {
	_ = jsonparser.ObjectEach(v.operation.Input.Variables, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		if !v.extractedVariablesContainsKey(key, inputValueDefinition) {
			// skip variables that were not extracted but user defined
			return nil
		}
		if dataType == jsonparser.String {
			value = v.operation.Input.Variables[offset-len(value)-2 : offset]
		}
		if bytes.Equal(value, variableValue) {
			exists = true
			name = key
		}
		return nil
	})
	if exists {
		definition, exists = v.operation.VariableDefinitionByNameAndOperation(v.Ancestors[0].Ref, name)
	}
	return
}

func (v *variablesExtractionVisitor) extractedVariablesContainsKey(key []byte, inputValueDefinition int) bool {
	typeRef := v.definition.InputValueDefinitions[inputValueDefinition].Type
	for i := range v.extractedVariables {
		if bytes.Equal(v.extractedVariables[i], key) && v.definition.TypesAreEqualDeep(typeRef, v.extractedVariableTypeRefs[i]) {
			return true
		}
	}
	return false
}
