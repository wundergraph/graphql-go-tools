//go:generate stringer -type=ValidationState -output astvalidation_string.go
package astvalidation

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astinspect"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

type ValidationState int

const (
	UnknownState ValidationState = iota
	Valid
	Invalid
)

type Rule func(walker *astvisitor.Walker)

type Result struct {
	ValidationState ValidationState
	Reason          string
}

type OperationValidator struct {
	walker astvisitor.Walker
}

func (o *OperationValidator) RegisterRule(rule Rule) {
	rule(&o.walker)
}

func (o *OperationValidator) Validate(operation, definition *ast.Document) Result {

	err := o.walker.Walk(operation, definition)
	if err != nil {
		return Result{
			ValidationState: Invalid,
			Reason:          err.Error(),
		}
	}

	return Result{
		ValidationState: Valid,
	}
}

func OperationNameUniqueness() Rule {
	return func(walker *astvisitor.Walker) {
		walker.RegisterEnterDocumentVisitor(&operationNameUniquenessVisitor{})
	}
}

type operationNameUniquenessVisitor struct{}

func (_ operationNameUniquenessVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	if len(operation.OperationDefinitions) <= 1 {
		return astvisitor.Instruction{}
	}

	for i := range operation.OperationDefinitions {
		for k := range operation.OperationDefinitions {
			if i == k || i > k {
				continue
			}

			left := operation.OperationDefinitions[i].Name
			right := operation.OperationDefinitions[k].Name

			if ast.ByteSliceEquals(left, operation.Input, right, operation.Input) {
				return astvisitor.Instruction{
					Action:  astvisitor.StopWithError,
					Message: fmt.Sprintf("Operation Name %s must be unique", string(operation.Input.ByteSlice(operation.OperationDefinitions[i].Name))),
				}
			}
		}
	}

	return astvisitor.Instruction{}
}

func LoneAnonymousOperation() Rule {
	return func(walker *astvisitor.Walker) {
		walker.RegisterEnterDocumentVisitor(&loneAnonymousOperationVisitor{})
	}
}

type loneAnonymousOperationVisitor struct {
}

func (_ loneAnonymousOperationVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	if len(operation.OperationDefinitions) <= 1 {
		return astvisitor.Instruction{}
	}

	for i := range operation.OperationDefinitions {
		if operation.OperationDefinitions[i].Name.Length() == 0 {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: "Anonymous Operation must be the only operation in a document.",
			}
		}
	}

	return astvisitor.Instruction{}
}

func SubscriptionSingleRootField() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := subscriptionSingleRootFieldVisitor{}
		walker.RegisterEnterDocumentVisitor(visitor)
	}
}

type subscriptionSingleRootFieldVisitor struct {
}

func (_ subscriptionSingleRootFieldVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	for i := range operation.OperationDefinitions {
		if operation.OperationDefinitions[i].OperationType == ast.OperationTypeSubscription {
			selections := len(operation.SelectionSets[operation.OperationDefinitions[i].SelectionSet].SelectionRefs)
			if selections > 1 {
				return astvisitor.Instruction{
					Action:  astvisitor.StopWithError,
					Message: "Subscription must only have one root selection",
				}
			} else if selections == 1 {
				ref := operation.SelectionSets[operation.OperationDefinitions[i].SelectionSet].SelectionRefs[0]
				if operation.Selections[ref].Kind == ast.SelectionKindField {
					return astvisitor.Instruction{}
				}
			}
		}
	}

	return astvisitor.Instruction{}
}

func FieldSelections() Rule {
	return func(walker *astvisitor.Walker) {
		fieldDefined := fieldDefined{}
		walker.RegisterEnterDocumentVisitor(&fieldDefined)
		walker.RegisterEnterFieldVisitor(&fieldDefined)
	}
}

func FieldSelectionMerging() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := fieldSelectionMergingVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterSelectionSetVisitor(&visitor)
	}
}

type fieldSelectionMergingVisitor struct {
	definition, operation *ast.Document
}

func (f *fieldSelectionMergingVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	f.operation = operation
	f.definition = definition
	return astvisitor.Instruction{}
}

func (f *fieldSelectionMergingVisitor) EnterSelectionSet(ref int, info astvisitor.Info) astvisitor.Instruction {
	if !astinspect.SelectionSetCanMerge(ref, info.EnclosingTypeDefinition, f.operation, f.definition) {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: "selectionset cannot merge",
		}
	}
	return astvisitor.Instruction{}
}

func ValidArguments() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := validArgumentsVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterFieldVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type validArgumentsVisitor struct {
	operation, definition *ast.Document
}

func (v *validArgumentsVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	v.operation = operation
	v.definition = definition
	return astvisitor.Instruction{}
}

func (v *validArgumentsVisitor) EnterArgument(ref int, info astvisitor.Info) astvisitor.Instruction {

	if info.InputValueDefinition == -1 {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("argument: %s not defined", v.operation.ArgumentNameString(ref)),
		}
	}

	value := v.operation.ArgumentValue(ref)

	if !v.valueSatisfiesInputFieldDefinition(value, info.InputValueDefinition) {
		definition := v.definition.InputValueDefinitions[info.InputValueDefinition]
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("invalid argument value: %+v for definition: %+v", value, definition),
		}
	}

	return astvisitor.Instruction{}
}

func (_ validArgumentsVisitor) EnterField(ref int, info astvisitor.Info) astvisitor.Instruction {
	return astvisitor.Instruction{}
}

func (v *validArgumentsVisitor) valueSatisfiesInputFieldDefinition(value ast.Value, inputValueDefinition int) bool {

	switch value.Kind {
	case ast.ValueKindVariable:
		return v.variableValueSatisfiesInputValueDefinition(value.Ref, inputValueDefinition)
	case ast.ValueKindEnum:
		return v.enumValueSatisfiesInputValueDefinition(value.Ref, inputValueDefinition)
	case ast.ValueKindNull:
		return v.nullValueSatisfiesInputValueDefinition(inputValueDefinition)
	case ast.ValueKindBoolean:
		return v.booleanValueSatisfiesInputValueDefinition(inputValueDefinition)
	case ast.ValueKindInteger:
		return v.intValueSatisfiesInputValueDefinition(inputValueDefinition)
	default:
		panic(fmt.Sprintf("must implement validArgumentsVisitor.valueSatisfiesInputFieldDefinition() for kind: %s", value.Kind))
	}

	return true
}

func (v *validArgumentsVisitor) intValueSatisfiesInputValueDefinition(inputValueDefinition int) bool {
	inputType := v.definition.Types[v.definition.InputValueDefinitionType(inputValueDefinition)]
	if inputType.TypeKind == ast.TypeKindNonNull {
		inputType = v.definition.Types[inputType.OfType]
	}
	if inputType.TypeKind != ast.TypeKindNamed {
		return false
	}
	if !bytes.Equal(v.definition.Input.ByteSlice(inputType.Name), literal.INT) {
		return false
	}
	return true
}

func (v *validArgumentsVisitor) booleanValueSatisfiesInputValueDefinition(inputValueDefinition int) bool {
	inputType := v.definition.Types[v.definition.InputValueDefinitionType(inputValueDefinition)]
	if inputType.TypeKind == ast.TypeKindNonNull {
		inputType = v.definition.Types[inputType.OfType]
	}
	if inputType.TypeKind != ast.TypeKindNamed {
		return false
	}
	if !bytes.Equal(v.definition.Input.ByteSlice(inputType.Name), literal.BOOLEAN) {
		return false
	}
	return true
}

func (v *validArgumentsVisitor) nullValueSatisfiesInputValueDefinition(inputValueDefinition int) bool {
	inputType := v.definition.Types[v.definition.InputValueDefinitionType(inputValueDefinition)]
	return inputType.TypeKind != ast.TypeKindNonNull
}

func (v *validArgumentsVisitor) enumValueSatisfiesInputValueDefinition(enumValue, inputValueDefinition int) bool {

	definitionTypeName := v.definition.ResolveTypeName(v.definition.InputValueDefinitions[inputValueDefinition].Type)
	node, exists := v.definition.Index.Nodes[string(definitionTypeName)]
	if !exists {
		return false
	}

	if node.Kind != ast.NodeKindEnumTypeDefinition {
		return false
	}

	enumValueName := v.operation.Input.ByteSlice(v.operation.EnumValueName(enumValue))

	if !v.definition.EnumTypeDefinitionContainsEnumValue(node.Ref, enumValueName) {
		return false
	}

	return true
}

func (v *validArgumentsVisitor) variableValueSatisfiesInputValueDefinition(variableValue, inputValueDefinition int) bool {
	variableName := v.operation.VariableValueName(variableValue)
	variableDefinition, exists := v.operation.VariableDefinitionByName(variableName)
	if !exists {
		return false
	}

	operationType := v.operation.VariableDefinitions[variableDefinition].Type
	definitionType := v.definition.InputValueDefinitions[inputValueDefinition].Type
	hasDefaultValue := v.operation.VariableDefinitions[variableDefinition].DefaultValue.IsDefined ||
		v.definition.InputValueDefinitions[inputValueDefinition].DefaultValue.IsDefined

	if !v.operationTypeSatisfiesDefinitionType(operationType, definitionType, hasDefaultValue) {
		return false
	}

	return true
}

func (v *validArgumentsVisitor) operationTypeSatisfiesDefinitionType(operationType int, definitionType int, hasDefaultValue bool) bool {

	if operationType == -1 || definitionType == -1 {
		return false
	}

	if v.operation.Types[operationType].TypeKind != ast.TypeKindNonNull &&
		v.definition.Types[definitionType].TypeKind == ast.TypeKindNonNull &&
		hasDefaultValue &&
		v.definition.Types[definitionType].OfType != -1 {
		definitionType = v.definition.Types[definitionType].OfType
	}

	if v.operation.Types[operationType].TypeKind == ast.TypeKindNonNull &&
		v.definition.Types[definitionType].TypeKind != ast.TypeKindNonNull &&
		v.operation.Types[operationType].OfType != -1 {
		operationType = v.operation.Types[operationType].OfType
	}

	for {
		if operationType == -1 || definitionType == -1 {
			return false
		}
		if v.operation.Types[operationType].TypeKind != v.definition.Types[definitionType].TypeKind {
			return false
		}
		if v.operation.Types[operationType].TypeKind == ast.TypeKindNamed {
			return bytes.Equal(v.operation.Input.ByteSlice(v.operation.Types[operationType].Name),
				v.definition.Input.ByteSlice(v.definition.Types[definitionType].Name))
		}
		operationType = v.operation.Types[operationType].OfType
		definitionType = v.definition.Types[definitionType].OfType
	}
}

func Values() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := valuesVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type valuesVisitor struct {
	operation, definition *ast.Document
}

func (v *valuesVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	v.operation = operation
	v.definition = definition
	return astvisitor.Instruction{}
}

func (v *valuesVisitor) EnterArgument(ref int, info astvisitor.Info) astvisitor.Instruction {

	value := v.operation.ArgumentValue(ref)
	if value.Kind == ast.ValueKindVariable {
		variableName := v.operation.VariableValueName(value.Ref)
		variableDefinition, exists := v.operation.VariableDefinitionByName(variableName)
		if !exists {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: fmt.Sprintf("variable: %s not defined", string(variableName)),
			}
		}
		if !v.operation.VariableDefinitions[variableDefinition].DefaultValue.IsDefined {
			return astvisitor.Instruction{} // variable has no default value, deep type check not required
		}
		value = v.operation.VariableDefinitions[variableDefinition].DefaultValue.Value
	}

	if !v.valueSatisfiesInputValueDefinitionType(value, v.definition.InputValueDefinitions[info.InputValueDefinition].Type) {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("value for argument: %s doesn't satisfy requirements from input value definition: %s", v.operation.ArgumentNameString(ref), v.definition.InputValueDefinitionName(info.InputValueDefinition)),
		}
	}

	return astvisitor.Instruction{}
}

func (v *valuesVisitor) valueSatisfiesInputValueDefinitionType(value ast.Value, definitionTypeRef int) bool {

	switch v.definition.Types[definitionTypeRef].TypeKind {
	case ast.TypeKindNonNull:
		if value.Kind == ast.ValueKindNull {
			return false
		}
		return v.valueSatisfiesInputValueDefinitionType(value, v.definition.Types[definitionTypeRef].OfType)
	case ast.TypeKindNamed:
		node, exists := v.definition.Index.Nodes[string(v.definition.ResolveTypeName(definitionTypeRef))]
		if !exists {
			return false
		}
		return v.valueSatisfiesTypeDefinitionNode(value, node)
	case ast.TypeKindList:
		return v.valueSatisfiesListType(value, v.definition.Types[definitionTypeRef].OfType)
	default:
		return false
	}
}

func (v *valuesVisitor) valueSatisfiesListType(value ast.Value, listType int) bool {
	if value.Kind != ast.ValueKindList {
		return false
	}

	if v.definition.Types[listType].TypeKind == ast.TypeKindNonNull {
		if len(v.operation.ListValues[value.Ref].Refs) == 0 {
			return false
		}
		listType = v.definition.Types[listType].OfType
	}

	for _, i := range v.operation.ListValues[value.Ref].Refs {
		listValue := v.operation.Value(i)
		if !v.valueSatisfiesInputValueDefinitionType(listValue, listType) {
			return false
		}
	}

	return true
}

func (v *valuesVisitor) valueSatisfiesTypeDefinitionNode(value ast.Value, node ast.Node) bool {
	switch node.Kind {
	case ast.NodeKindEnumTypeDefinition:
		return v.valueSatisfiesEnum(value, node)
	case ast.NodeKindScalarTypeDefinition:
		return v.valueSatisfiesScalar(value, node.Ref)
	case ast.NodeKindInputObjectTypeDefinition:
		return v.valueSatisfiesInputObjectTypeDefinition(value, node.Ref)
	default:
		return false
	}
}

func (v *valuesVisitor) valueSatisfiesEnum(value ast.Value, node ast.Node) bool {
	if value.Kind != ast.ValueKindEnum {
		return false
	}
	enumValue := v.operation.EnumValueNameBytes(value.Ref)
	return v.definition.EnumTypeDefinitionContainsEnumValue(node.Ref, enumValue)
}

func (v *valuesVisitor) valueSatisfiesInputObjectTypeDefinition(value ast.Value, inputObjectTypeDefinition int) bool {
	if value.Kind != ast.ValueKindObject {
		return false
	}

	for _, i := range v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].InputFieldsDefinition.Refs {
		if !v.objectValueSatisfiesInputValueDefinition(value.Ref, i) {
			return false
		}
	}

	for _, i := range v.operation.ObjectValues[value.Ref].Refs {
		if !v.objectFieldDefined(i, inputObjectTypeDefinition) {
			objectFieldName := string(v.operation.ObjectFieldName(i))
			def := string(v.definition.Input.ByteSlice(v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].Name))
			_, _ = objectFieldName, def
			return false
		}
	}

	if v.objectValueHasDuplicateFields(value.Ref) {
		return false
	}

	return true
}

func (v *valuesVisitor) objectValueHasDuplicateFields(objectValue int) bool {
	for i, j := range v.operation.ObjectValues[objectValue].Refs {
		for k, l := range v.operation.ObjectValues[objectValue].Refs {
			if i == k || i > k {
				continue
			}
			if bytes.Equal(v.operation.ObjectFieldName(j), v.operation.ObjectFieldName(l)) {
				return true
			}
		}
	}
	return false
}

func (v *valuesVisitor) objectFieldDefined(objectField, inputObjectTypeDefinition int) bool {
	name := v.operation.ObjectFieldName(objectField)
	for _, i := range v.definition.InputObjectTypeDefinitions[inputObjectTypeDefinition].InputFieldsDefinition.Refs {
		if bytes.Equal(name, v.definition.InputValueDefinitionName(i)) {
			return true
		}
	}
	return false
}

func (v *valuesVisitor) objectValueSatisfiesInputValueDefinition(objectValue, inputValueDefinition int) bool {

	name := v.definition.InputValueDefinitionName(inputValueDefinition)
	definitionType := v.definition.InputValueDefinitionType(inputValueDefinition)

	for _, i := range v.operation.ObjectValues[objectValue].Refs {
		if bytes.Equal(name, v.operation.ObjectFieldName(i)) {
			value := v.operation.ObjectFieldValue(i)
			return v.valueSatisfiesInputValueDefinitionType(value, definitionType)
		}
	}

	// argument is not present on object value, if arg is optional it's still ok, otherwise not satisfied
	return v.definition.InputValueDefinitionArgumentIsOptional(inputValueDefinition)
}

func (v *valuesVisitor) valueSatisfiesScalar(value ast.Value, scalar int) bool {
	scalarName := v.definition.ScalarTypeDefinitionName(scalar)
	switch value.Kind {
	case ast.ValueKindString:
		return bytes.Equal(scalarName, literal.STRING)
	case ast.ValueKindBoolean:
		return bytes.Equal(scalarName, literal.BOOLEAN)
	case ast.ValueKindInteger:
		return bytes.Equal(scalarName, literal.INT) || bytes.Equal(scalarName, literal.FLOAT)
	case ast.ValueKindFloat:
		return bytes.Equal(scalarName, literal.FLOAT)
	default:
		return false
	}
}

func ArgumentUniqueness() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := argumentUniquenessVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type argumentUniquenessVisitor struct {
	operation *ast.Document
}

func (a *argumentUniquenessVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	a.operation = operation
	return astvisitor.Instruction{}
}

func (a *argumentUniquenessVisitor) EnterArgument(ref int, info astvisitor.Info) astvisitor.Instruction {

	argumentName := a.operation.ArgumentName(ref)

	for _, i := range info.ArgumentsAfter {
		if bytes.Equal(argumentName, a.operation.ArgumentName(i)) {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: fmt.Sprintf("argument: %s must be unique", string(argumentName)),
			}
		}
	}

	return astvisitor.Instruction{}
}

func RequiredArguments() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := requiredArgumentsVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterFieldVisitor(&visitor)
	}
}

type requiredArgumentsVisitor struct {
	operation, definition *ast.Document
}

func (r *requiredArgumentsVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	r.operation = operation
	r.definition = definition
	return astvisitor.Instruction{}
}

func (r *requiredArgumentsVisitor) EnterField(ref int, info astvisitor.Info) astvisitor.Instruction {

	for _, i := range info.InputValueDefinitions {
		if r.definition.InputValueDefinitionArgumentIsOptional(i) {
			continue
		}

		name := r.definition.InputValueDefinitionName(i)

		argument, exists := r.operation.FieldArgument(ref, name)
		if !exists {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: fmt.Sprintf("required argument: %s on field: %s missing", string(name), r.operation.FieldNameString(ref)),
			}
		}

		if r.operation.ArgumentValue(argument).Kind == ast.ValueKindNull {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: fmt.Sprintf("required argument: %s on field: %s must not be null", string(name), r.operation.FieldNameString(ref)),
			}
		}
	}

	return astvisitor.Instruction{}
}

func Fragments() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := fragmentsVisitor{
			fragmentDefinitionsVisited: make([]ast.ByteSlice, 0, 8),
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterLeaveDocumentVisitor(&visitor)
		walker.RegisterEnterFragmentDefinitionVisitor(&visitor)
		walker.RegisterEnterInlineFragmentVisitor(&visitor)
		walker.RegisterEnterFragmentSpreadVisitor(&visitor)
	}
}

type fragmentsVisitor struct {
	operation, definition      *ast.Document
	fragmentDefinitionsVisited []ast.ByteSlice
}

func (f *fragmentsVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) astvisitor.Instruction {
	if info.Ancestors[0].Kind == ast.NodeKindOperationDefinition {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("fragment spread: %s forms fragment cycle", f.operation.FragmentSpreadName(ref)),
		}
	}
	return astvisitor.Instruction{}
}

func (f *fragmentsVisitor) LeaveDocument(operation, definition *ast.Document) astvisitor.Instruction {
	for i := range f.fragmentDefinitionsVisited {
		if !f.operation.FragmentDefinitionIsUsed(f.fragmentDefinitionsVisited[i]) {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: fmt.Sprintf("fragment: %s is never used", string(f.fragmentDefinitionsVisited[i])),
			}
		}
	}
	return astvisitor.Instruction{}
}

func (f *fragmentsVisitor) fragmentOnNodeIsAllowed(node ast.Node) bool {
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
		return true
	default:
		return false
	}
}

func (f *fragmentsVisitor) EnterInlineFragment(ref int, info astvisitor.Info) astvisitor.Instruction {

	if !f.operation.InlineFragmentHasTypeCondition(ref) {
		return astvisitor.Instruction{}
	}

	typeName := f.operation.InlineFragmentTypeConditionName(ref)

	node, exists := f.definition.Index.Nodes[string(typeName)]
	if !exists {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("type: %s on inline framgent is not defined", string(typeName)),
		}
	}

	if !f.fragmentOnNodeIsAllowed(node) {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("inline fragment on type: %s of kind: %s is disallowed", string(typeName), node.Kind),
		}
	}

	if !f.definition.NodeFragmentIsAllowedOnNode(node, info.EnclosingTypeDefinition) {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("inline fragment on type: %s of kind: %s is disallowed", string(typeName), node.Kind),
		}
	}

	return astvisitor.Instruction{}
}

func (f *fragmentsVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	f.operation = operation
	f.definition = definition
	f.fragmentDefinitionsVisited = f.fragmentDefinitionsVisited[:0]
	return astvisitor.Instruction{}
}

func (f *fragmentsVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {

	fragmentDefinitionName := f.operation.FragmentDefinitionName(ref)
	typeName := f.operation.FragmentDefinitionTypeName(ref)

	node, exists := f.definition.Index.Nodes[string(typeName)]
	if !exists {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("type: %s on fragment: %s is not defined", string(typeName), string(fragmentDefinitionName)),
		}
	}

	if !f.fragmentOnNodeIsAllowed(node) {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("fragment definition: %s on type: %s of kind: %s is disallowed", string(fragmentDefinitionName), string(typeName), node.Kind),
		}
	}

	for i := range f.fragmentDefinitionsVisited {
		if bytes.Equal(fragmentDefinitionName, f.fragmentDefinitionsVisited[i]) {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: fmt.Sprintf("fragment: %s must be unique", string(f.fragmentDefinitionsVisited[i])),
			}
		}
	}

	f.fragmentDefinitionsVisited = append(f.fragmentDefinitionsVisited, fragmentDefinitionName)
	return astvisitor.Instruction{}
}

func DirectivesAreDefined() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := directivesAreDefinedVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type directivesAreDefinedVisitor struct {
	operation, definition *ast.Document
}

func (d *directivesAreDefinedVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	d.operation = operation
	d.definition = definition
	return astvisitor.Instruction{}
}

func (d *directivesAreDefinedVisitor) EnterDirective(ref int, info astvisitor.Info) astvisitor.Instruction {

	if info.DirectiveDefinition == -1 {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("directive: %s not defined", d.operation.DirectiveNameString(ref)),
		}
	}

	return astvisitor.Instruction{}
}

func DirectivesAreInValidLocations() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := directivesAreInValidLocationsVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type directivesAreInValidLocationsVisitor struct {
	operation, definition *ast.Document
}

func (d *directivesAreInValidLocationsVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	d.operation = operation
	d.definition = definition
	return astvisitor.Instruction{}
}

func (d *directivesAreInValidLocationsVisitor) EnterDirective(ref int, info astvisitor.Info) astvisitor.Instruction {
	if info.DirectiveDefinition == -1 {
		return astvisitor.Instruction{} // not defined, skip
	}

	ancestor := info.Ancestors[len(info.Ancestors)-1]

	if !d.directiveDefinitionContainsNodeLocation(info.DirectiveDefinition, ancestor) {
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("directive: %s not allowed on node kind: %s", d.operation.DirectiveNameString(ref), ancestor.Kind),
		}
	}

	return astvisitor.Instruction{}
}

func (d *directivesAreInValidLocationsVisitor) directiveDefinitionContainsNodeLocation(definition int, node ast.Node) bool {

	nodeDirectiveLocation, err := d.operation.NodeDirectiveLocation(node)
	if err != nil {
		return false
	}

	return d.definition.DirectiveDefinitions[definition].DirectiveLocations.Get(nodeDirectiveLocation)
}

func VariableUniqueness() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := variableUniquenessVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterVariableDefinitionVisitor(&visitor)
	}
}

type variableUniquenessVisitor struct {
	operation, definition *ast.Document
}

func (v *variableUniquenessVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	v.operation = operation
	v.definition = definition
	return astvisitor.Instruction{}
}

func (v *variableUniquenessVisitor) EnterVariableDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {

	name := v.operation.VariableDefinitionName(ref)

	for _, i := range info.VariableDefinitionsAfter {
		if bytes.Equal(name, v.operation.VariableDefinitionName(i)) {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: fmt.Sprintf("variable: %s must be unique", string(name)),
			}
		}
	}

	return astvisitor.Instruction{}
}

func DirectivesAreUniquePerLocation() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := directivesAreUniquePerLocationVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type directivesAreUniquePerLocationVisitor struct {
	operation, definition *ast.Document
}

func (d *directivesAreUniquePerLocationVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	d.operation = operation
	d.definition = definition
	return astvisitor.Instruction{}
}

func (d *directivesAreUniquePerLocationVisitor) EnterDirective(ref int, info astvisitor.Info) astvisitor.Instruction {

	directiveName := d.operation.DirectiveNameBytes(ref)

	for _, i := range info.DirectivesAfter {
		if bytes.Equal(directiveName, d.operation.DirectiveNameBytes(i)) {
			return astvisitor.Instruction{
				Action:  astvisitor.StopWithError,
				Message: fmt.Sprintf("directive: %s must be unique per location", string(directiveName)),
			}
		}
	}

	return astvisitor.Instruction{}
}

func VariablesAreInputTypes() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := variablesAreInputTypesVisitor{}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterVariableDefinitionVisitor(&visitor)
	}
}

type variablesAreInputTypesVisitor struct {
	operation, definition *ast.Document
}

func (v *variablesAreInputTypesVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	v.operation = operation
	v.definition = definition
	return astvisitor.Instruction{}
}

func (v *variablesAreInputTypesVisitor) EnterVariableDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {

	typeName := v.operation.ResolveTypeName(v.operation.VariableDefinitions[ref].Type)
	typeDefinitionNode := v.definition.Index.Nodes[string(typeName)]
	switch typeDefinitionNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition, ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
		return astvisitor.Instruction{}
	default:
		return astvisitor.Instruction{
			Action:  astvisitor.StopWithError,
			Message: fmt.Sprintf("variable: %s of type: %s is no valid input type", v.operation.VariableDefinitionName(ref), string(typeName)),
		}
	}
}

func AllVariableUsesDefined() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func AllVariablesUsed() Rule {
	return func(walker *astvisitor.Walker) {

	}
}
