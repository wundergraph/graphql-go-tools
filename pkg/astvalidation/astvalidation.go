//go:generate stringer -type=ValidationState -output astvalidation_string.go
package astvalidation

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astinspect"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

func DefaultOperationValidator() *OperationValidator {

	validator := OperationValidator{
		walker: astvisitor.NewWalker(48),
	}

	validator.RegisterRule(OperationNameUniqueness())
	validator.RegisterRule(LoneAnonymousOperation())
	validator.RegisterRule(SubscriptionSingleRootField())
	validator.RegisterRule(FieldSelections())
	validator.RegisterRule(FieldSelectionMerging())
	validator.RegisterRule(ValidArguments())
	validator.RegisterRule(Values())
	validator.RegisterRule(ArgumentUniqueness())
	validator.RegisterRule(RequiredArguments())
	validator.RegisterRule(Fragments())
	validator.RegisterRule(DirectivesAreDefined())
	validator.RegisterRule(DirectivesAreInValidLocations())
	validator.RegisterRule(VariableUniqueness())
	validator.RegisterRule(DirectivesAreUniquePerLocation())
	validator.RegisterRule(VariablesAreInputTypes())
	validator.RegisterRule(AllVariableUsesDefined())
	validator.RegisterRule(AllVariablesUsed())

	return &validator
}

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
		walker.RegisterEnterDocumentVisitor(&operationNameUniquenessVisitor{walker})
	}
}

type operationNameUniquenessVisitor struct {
	*astvisitor.Walker
}

func (o *operationNameUniquenessVisitor) EnterDocument(operation, definition *ast.Document) {
	if len(operation.OperationDefinitions) <= 1 {
		return
	}

	for i := range operation.OperationDefinitions {
		for k := range operation.OperationDefinitions {
			if i == k || i > k {
				continue
			}

			left := operation.OperationDefinitions[i].Name
			right := operation.OperationDefinitions[k].Name

			if ast.ByteSliceEquals(left, operation.Input, right, operation.Input) {
				o.StopWithErr(fmt.Errorf("Operation Name %s must be unique", string(operation.Input.ByteSlice(operation.OperationDefinitions[i].Name))))
				return
			}
		}
	}
}

func LoneAnonymousOperation() Rule {
	return func(walker *astvisitor.Walker) {
		walker.RegisterEnterDocumentVisitor(&loneAnonymousOperationVisitor{walker})
	}
}

type loneAnonymousOperationVisitor struct {
	*astvisitor.Walker
}

func (l *loneAnonymousOperationVisitor) EnterDocument(operation, definition *ast.Document) {
	if len(operation.OperationDefinitions) <= 1 {
		return
	}

	for i := range operation.OperationDefinitions {
		if operation.OperationDefinitions[i].Name.Length() == 0 {
			l.StopWithErr(fmt.Errorf("Anonymous Operation must be the only operation in a document."))
			return
		}
	}
}

func SubscriptionSingleRootField() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := subscriptionSingleRootFieldVisitor{walker}
		walker.RegisterEnterDocumentVisitor(&visitor)
	}
}

type subscriptionSingleRootFieldVisitor struct {
	*astvisitor.Walker
}

func (s *subscriptionSingleRootFieldVisitor) EnterDocument(operation, definition *ast.Document) {
	for i := range operation.OperationDefinitions {
		if operation.OperationDefinitions[i].OperationType == ast.OperationTypeSubscription {
			selections := len(operation.SelectionSets[operation.OperationDefinitions[i].SelectionSet].SelectionRefs)
			if selections > 1 {
				s.StopWithErr(fmt.Errorf("Subscription must only have one root selection"))
				return
			} else if selections == 1 {
				ref := operation.SelectionSets[operation.OperationDefinitions[i].SelectionSet].SelectionRefs[0]
				if operation.Selections[ref].Kind == ast.SelectionKindField {
					return
				}
			}
		}
	}
}

func FieldSelections() Rule {
	return func(walker *astvisitor.Walker) {
		fieldDefined := fieldDefined{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&fieldDefined)
		walker.RegisterEnterFieldVisitor(&fieldDefined)
	}
}

func FieldSelectionMerging() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := fieldSelectionMergingVisitor{Walker: walker}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterSelectionSetVisitor(&visitor)
	}
}

type fieldSelectionMergingVisitor struct {
	*astvisitor.Walker
	definition, operation *ast.Document
}

func (f *fieldSelectionMergingVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
}

func (f *fieldSelectionMergingVisitor) EnterSelectionSet(ref int) {
	if !astinspect.SelectionSetCanMerge(ref, f.EnclosingTypeDefinition, f.operation, f.definition) {
		f.StopWithErr(fmt.Errorf("selectionset cannot merge"))
	}
}

func ValidArguments() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := validArgumentsVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type validArgumentsVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (v *validArgumentsVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *validArgumentsVisitor) EnterArgument(ref int) {

	definition, exists := v.ArgumentInputValueDefinition(ref)

	if !exists {
		v.StopWithErr(fmt.Errorf("argument: %s not defined", v.operation.ArgumentNameString(ref)))
		return
	}

	value := v.operation.ArgumentValue(ref)

	if !v.valueSatisfiesInputFieldDefinition(value, definition) {
		definition := v.definition.InputValueDefinitions[definition]
		v.StopWithErr(fmt.Errorf("invalid argument value: %+v for definition: %+v", value, definition))
		return
	}
}

func (v *validArgumentsVisitor) valueSatisfiesInputFieldDefinition(value ast.Value, inputValueDefinition int) bool {

	// object- and list values are covered by Values() / valuesVisitor
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
		visitor := valuesVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type valuesVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (v *valuesVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *valuesVisitor) EnterArgument(ref int) {

	definition, exists := v.ArgumentInputValueDefinition(ref)

	if !exists {
		v.StopWithErr(fmt.Errorf("argument: %s not defined", v.operation.ArgumentNameString(ref)))
		return
	}

	value := v.operation.ArgumentValue(ref)
	if value.Kind == ast.ValueKindVariable {
		variableName := v.operation.VariableValueName(value.Ref)
		variableDefinition, exists := v.operation.VariableDefinitionByName(variableName)
		if !exists {
			v.StopWithErr(fmt.Errorf("variable: %s not defined", string(variableName)))
			return
		}
		if !v.operation.VariableDefinitions[variableDefinition].DefaultValue.IsDefined {
			return // variable has no default value, deep type check not required
		}
		value = v.operation.VariableDefinitions[variableDefinition].DefaultValue.Value
	}

	if !v.valueSatisfiesInputValueDefinitionType(value, v.definition.InputValueDefinitions[definition].Type) {
		v.StopWithErr(fmt.Errorf("value for argument: %s doesn't satisfy requirements from input value definition: %s", v.operation.ArgumentNameString(ref), v.definition.InputValueDefinitionNameBytes(definition)))
		return
	}
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
		if bytes.Equal(name, v.definition.InputValueDefinitionNameBytes(i)) {
			return true
		}
	}
	return false
}

func (v *valuesVisitor) objectValueSatisfiesInputValueDefinition(objectValue, inputValueDefinition int) bool {

	name := v.definition.InputValueDefinitionNameBytes(inputValueDefinition)
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
	scalarName := v.definition.ScalarTypeDefinitionNameBytes(scalar)
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
		visitor := argumentUniquenessVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type argumentUniquenessVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (a *argumentUniquenessVisitor) EnterDocument(operation, definition *ast.Document) {
	a.operation = operation
}

func (a *argumentUniquenessVisitor) EnterArgument(ref int) {

	argumentName := a.operation.ArgumentName(ref)
	argumentsAfter := a.operation.ArgumentsAfter(a.Ancestors[len(a.Ancestors)-1], ref)

	for _, i := range argumentsAfter {
		if bytes.Equal(argumentName, a.operation.ArgumentName(i)) {
			a.StopWithErr(fmt.Errorf("argument: %s must be unique", string(argumentName)))
			return
		}
	}
}

func RequiredArguments() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := requiredArgumentsVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterFieldVisitor(&visitor)
	}
}

type requiredArgumentsVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (r *requiredArgumentsVisitor) EnterDocument(operation, definition *ast.Document) {
	r.operation = operation
	r.definition = definition
}

func (r *requiredArgumentsVisitor) EnterField(ref int) {

	fieldName := r.operation.FieldNameBytes(ref)
	inputValueDefinitions := r.definition.NodeFieldDefinitionArgumentsDefinitions(r.EnclosingTypeDefinition, fieldName)

	for _, i := range inputValueDefinitions {
		if r.definition.InputValueDefinitionArgumentIsOptional(i) {
			continue
		}

		name := r.definition.InputValueDefinitionNameBytes(i)

		argument, exists := r.operation.FieldArgument(ref, name)
		if !exists {
			r.StopWithErr(fmt.Errorf("required argument: %s on field: %s missing", string(name), r.operation.FieldNameString(ref)))
			return
		}

		if r.operation.ArgumentValue(argument).Kind == ast.ValueKindNull {
			r.StopWithErr(fmt.Errorf("required argument: %s on field: %s must not be null", string(name), r.operation.FieldNameString(ref)))
			return
		}
	}
}

func Fragments() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := fragmentsVisitor{
			Walker:                     walker,
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
	*astvisitor.Walker
	operation, definition      *ast.Document
	fragmentDefinitionsVisited []ast.ByteSlice
}

func (f *fragmentsVisitor) EnterFragmentSpread(ref int) {
	if f.Ancestors[0].Kind == ast.NodeKindOperationDefinition {
		f.StopWithErr(fmt.Errorf("fragment spread: %s forms fragment cycle", f.operation.FragmentSpreadName(ref)))
	}
}

func (f *fragmentsVisitor) LeaveDocument(operation, definition *ast.Document) {
	for i := range f.fragmentDefinitionsVisited {
		if !f.operation.FragmentDefinitionIsUsed(f.fragmentDefinitionsVisited[i]) {
			f.StopWithErr(fmt.Errorf("fragment: %s is never used", string(f.fragmentDefinitionsVisited[i])))
			return
		}
	}
}

func (f *fragmentsVisitor) fragmentOnNodeIsAllowed(node ast.Node) bool {
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
		return true
	default:
		return false
	}
}

func (f *fragmentsVisitor) EnterInlineFragment(ref int) {

	if !f.operation.InlineFragmentHasTypeCondition(ref) {
		return
	}

	typeName := f.operation.InlineFragmentTypeConditionName(ref)

	node, exists := f.definition.Index.Nodes[string(typeName)]
	if !exists {
		f.StopWithErr(fmt.Errorf("type: %s on inline framgent is not defined", string(typeName)))
		return
	}

	if !f.fragmentOnNodeIsAllowed(node) {
		f.StopWithErr(fmt.Errorf("inline fragment on type: %s of kind: %s is disallowed", string(typeName), node.Kind))
		return
	}

	if !f.definition.NodeFragmentIsAllowedOnNode(node, f.EnclosingTypeDefinition) {
		f.StopWithErr(fmt.Errorf("inline fragment on type: %s of kind: %s is disallowed", string(typeName), node.Kind))
		return
	}
}

func (f *fragmentsVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
	f.fragmentDefinitionsVisited = f.fragmentDefinitionsVisited[:0]
}

func (f *fragmentsVisitor) EnterFragmentDefinition(ref int) {

	fragmentDefinitionName := f.operation.FragmentDefinitionNameBytes(ref)
	typeName := f.operation.FragmentDefinitionTypeName(ref)

	node, exists := f.definition.Index.Nodes[string(typeName)]
	if !exists {
		f.StopWithErr(fmt.Errorf("type: %s on fragment: %s is not defined", string(typeName), string(fragmentDefinitionName)))
		return
	}

	if !f.fragmentOnNodeIsAllowed(node) {
		f.StopWithErr(fmt.Errorf("fragment definition: %s on type: %s of kind: %s is disallowed", string(fragmentDefinitionName), string(typeName), node.Kind))
		return
	}

	for i := range f.fragmentDefinitionsVisited {
		if bytes.Equal(fragmentDefinitionName, f.fragmentDefinitionsVisited[i]) {
			f.StopWithErr(fmt.Errorf("fragment: %s must be unique", string(f.fragmentDefinitionsVisited[i])))
			return
		}
	}

	f.fragmentDefinitionsVisited = append(f.fragmentDefinitionsVisited, fragmentDefinitionName)
}

func DirectivesAreDefined() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := directivesAreDefinedVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type directivesAreDefinedVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (d *directivesAreDefinedVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
}

func (d *directivesAreDefinedVisitor) EnterDirective(ref int) {

	directiveName := d.operation.DirectiveNameBytes(ref)
	definition, exists := d.definition.Index.Nodes[string(directiveName)]

	if !exists || definition.Kind != ast.NodeKindDirectiveDefinition {
		d.StopWithErr(fmt.Errorf("directive: %s not defined", string(directiveName)))
	}
}

func DirectivesAreInValidLocations() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := directivesAreInValidLocationsVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type directivesAreInValidLocationsVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (d *directivesAreInValidLocationsVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
}

func (d *directivesAreInValidLocationsVisitor) EnterDirective(ref int) {

	directiveName := d.operation.DirectiveNameBytes(ref)
	definition, exists := d.definition.Index.Nodes[string(directiveName)]

	if !exists || definition.Kind != ast.NodeKindDirectiveDefinition {
		return // not defined, skip
	}

	ancestor := d.Ancestors[len(d.Ancestors)-1]

	if !d.directiveDefinitionContainsNodeLocation(definition.Ref, ancestor) {
		d.StopWithErr(fmt.Errorf("directive: %s not allowed on node kind: %s", d.operation.DirectiveNameString(ref), ancestor.Kind))
		return
	}
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
		visitor := variableUniquenessVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterVariableDefinitionVisitor(&visitor)
	}
}

type variableUniquenessVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (v *variableUniquenessVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *variableUniquenessVisitor) EnterVariableDefinition(ref int) {

	name := v.operation.VariableDefinitionName(ref)

	if v.Ancestors[0].Kind != ast.NodeKindOperationDefinition {
		return
	}

	variableDefinitions := v.operation.OperationDefinitions[v.Ancestors[0].Ref].VariableDefinitions.Refs

	for _, i := range variableDefinitions {
		if i == ref {
			continue
		}
		if bytes.Equal(name, v.operation.VariableDefinitionName(i)) {
			v.StopWithErr(fmt.Errorf("variable: %s must be unique", string(name)))
			return
		}
	}
}

func DirectivesAreUniquePerLocation() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := directivesAreUniquePerLocationVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type directivesAreUniquePerLocationVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (d *directivesAreUniquePerLocationVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
}

func (d *directivesAreUniquePerLocationVisitor) EnterDirective(ref int) {

	directiveName := d.operation.DirectiveNameBytes(ref)
	directives := d.operation.NodeDirectives(d.Ancestors[len(d.Ancestors)-1])

	for _, j := range directives {
		if j == ref {
			continue
		}
		if bytes.Equal(directiveName, d.operation.DirectiveNameBytes(j)) {
			d.StopWithErr(fmt.Errorf("directive: %s must be unique per location", string(directiveName)))
			return
		}
	}
}

func VariablesAreInputTypes() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := variablesAreInputTypesVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterVariableDefinitionVisitor(&visitor)
	}
}

type variablesAreInputTypesVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (v *variablesAreInputTypesVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.definition = definition
}

func (v *variablesAreInputTypesVisitor) EnterVariableDefinition(ref int) {

	typeName := v.operation.ResolveTypeName(v.operation.VariableDefinitions[ref].Type)
	typeDefinitionNode := v.definition.Index.Nodes[string(typeName)]
	switch typeDefinitionNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition, ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
		return
	default:
		v.StopWithErr(fmt.Errorf("variable: %s of type: %s is no valid input type", v.operation.VariableDefinitionName(ref), string(typeName)))
		return
	}
}

func AllVariableUsesDefined() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := allVariableUsesDefinedVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type allVariableUsesDefinedVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
}

func (a *allVariableUsesDefinedVisitor) EnterDocument(operation, definition *ast.Document) {
	a.operation = operation
	a.definition = definition
}

func (a *allVariableUsesDefinedVisitor) EnterArgument(ref int) {

	if a.operation.Arguments[ref].Value.Kind != ast.ValueKindVariable {
		return // skip because no variable
	}

	if a.Ancestors[0].Kind != ast.NodeKindOperationDefinition {
		// skip because variable is not used in operation which happens in case normalization did not merge the fragment definition
		// this happens when a fragment is defined but not used which will itself lead to another validation error
		// in which case we can safely skip here
		return
	}

	variableName := a.operation.VariableValueName(a.operation.Arguments[ref].Value.Ref)

	for _, i := range a.operation.OperationDefinitions[a.Ancestors[0].Ref].VariableDefinitions.Refs {
		if bytes.Equal(variableName, a.operation.VariableDefinitionName(i)) {
			return // return OK because variable is defined
		}
	}

	// at this point we're safe to say this variable was not defined on the root operation of this argument
	a.StopWithErr(fmt.Errorf("variable: %s on argument: %s is not defined", string(variableName), a.operation.ArgumentNameString(ref)))
}

func AllVariablesUsed() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := allVariablesUsedVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterOperationVisitor(&visitor)
		walker.RegisterLeaveOperationVisitor(&visitor)
		walker.RegisterEnterArgumentVisitor(&visitor)
	}
}

type allVariablesUsedVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	variableDefinitions   []int
}

func (a *allVariablesUsedVisitor) EnterDocument(operation, definition *ast.Document) {
	a.operation = operation
	a.definition = definition
}

func (a *allVariablesUsedVisitor) EnterOperationDefinition(ref int) {
	a.variableDefinitions = a.operation.OperationDefinitions[ref].VariableDefinitions.Refs
}

func (a *allVariablesUsedVisitor) LeaveOperationDefinition(ref int) {
	if len(a.variableDefinitions) != 0 {
		variableName := string(a.operation.VariableDefinitionName(a.variableDefinitions[0]))
		operationType := a.operation.OperationDefinitions[ref].OperationType
		operationName := a.operation.Input.ByteSlice(a.operation.OperationDefinitions[ref].Name)
		a.StopWithErr(fmt.Errorf("variable: %s is defined on operation: %s with operation type: %s but never used", variableName, operationName, operationType))
		return
	}
}

func (a *allVariablesUsedVisitor) EnterArgument(ref int) {

	if len(a.variableDefinitions) == 0 {
		return // nothing to check, skip
	}

	if a.operation.Arguments[ref].Value.Kind != ast.ValueKindVariable {
		return // skip non variable value
	}

	variableName := a.operation.VariableValueName(a.operation.Arguments[ref].Value.Ref)
	for i, j := range a.variableDefinitions {
		if bytes.Equal(variableName, a.operation.VariableDefinitionName(j)) {
			a.variableDefinitions = append(a.variableDefinitions[:i], a.variableDefinitions[i+1:]...)
			return
		}
	}
}
