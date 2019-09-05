//go:generate go-enum -f=$GOFILE --noprefix
package validation

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
)

func Valid() Result {
	return Result{
		Valid: true,
	}
}

func Invalid(ruleName RuleName, description Description, subjectPosition position.Position, subjectNameRef document.ByteSliceReference) Result {
	return Result{
		Valid:       false,
		RuleName:    ruleName,
		Description: description,
		Meta: Meta{
			SubjectPosition: subjectPosition,
			SubjectNameRef:  subjectNameRef,
		},
	}
}

type Result struct {
	Valid       bool
	RuleName    RuleName
	Description Description
	Meta        Meta
}

type Meta struct {
	SubjectPosition position.Position
	SubjectNameRef  document.ByteSliceReference
}

/*
ENUM(
NoRule
ArgumentUniqueness
DirectivesAreDefined
DirectivesAreInValidLocations
DirectivesAreUniquePerLocation
DirectivesHaveRequiredArguments
DirectivesArgumentsAreDefined
DirectiveArgumentsAreConstants
DirectiveDefinitionArgumentsAreConstants
DirectiveDefinitionDefaultValuesAreOfCorrectType
FieldSelectionMerging
FieldSelections
Fragments
LoneAnonymousOperation
OperationNameUniqueness
RequiredArguments
SubscriptionSingleRootField
ValidArguments
Values
VariableUniqueness
VariablesAreInputTypes
AllVariablesUsed
AllVariableUsesDefined
)
*/
type RuleName int

/*
ENUM(
NoDescription
AnonymousOperationMustBeLonePerDocument
ArgumentMustBeUnique
ArgumentRequired
ArgumentValueTypeMismatch
DirectiveNotDefined
DirectiveLocationInvalid
DirectiveMustBeUniquePerLocation
FieldNameOrAliasMismatch
FieldSelectionsInvalid
FragmentNotDefined
FragmentSpreadCyclicReference
FragmentDefinitionOnLeafNode
FragmentRedeclared
FragmentDeclaredButNeverUsed
InputValueNotDefined
OperationNameMustBeUnique
RootTypeNotDefined
SelectionSetInvalid
SelectionSetResponseShapesCannotMerge
SubscriptionsMustHaveMaxOneRootField
TypeNotDefined
ValueInvalid
VariableMustBeUniquePerOperation
VariableMustBeValidInputType
VariableNotDefined
VariableDefinedButNotUsed
)
*/
type Description int
