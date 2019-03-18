//go:generate go-enum -f=$GOFILE --noprefix
package validation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

func Valid() Result {
	return Result{
		Valid: true,
	}
}

func Invalid(ruleName RuleName, description Description, subjectPosition position.Position, subjectNameRef int) Result {
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
	SubjectNameRef  int
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
