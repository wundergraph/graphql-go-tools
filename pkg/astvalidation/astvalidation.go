//go:generate stringer -type=ValidationState -output astvalidation_string.go
package astvalidation

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astinspect"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
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
		walker.RegisterEnterFieldVisitor(visitor)
	}
}

type validArgumentsVisitor struct {
}

func (_ validArgumentsVisitor) EnterField(ref int, info astvisitor.Info) astvisitor.Instruction {
	return astvisitor.Instruction{}
}

func Values() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func ArgumentUniqueness() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func RequiredArguments() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func Fragments() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func DirectivesAreDefined() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func DirectivesAreInValidLocations() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func VariableUniqueness() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func DirectivesAreUniquePerLocation() Rule {
	return func(walker *astvisitor.Walker) {

	}
}

func VariablesAreInputTypes() Rule {
	return func(walker *astvisitor.Walker) {

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
