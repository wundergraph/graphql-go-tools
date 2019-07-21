package astvalidation

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"strconv"
)

type ValidationState int

func (v ValidationState) String() string {
	switch v {
	case UnknownState:
		return "UnknownState"
	case Valid:
		return "Valid"
	case Invalid:
		return "Invalid"
	default:
		return "String() not implemented for ValidationState: " + strconv.Itoa(int(v))
	}
}

const (
	UnknownState ValidationState = iota
	Valid
	Invalid
)

type Rule func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result

type Result struct {
	ValidationState ValidationState
	Explanation     string
}

type OperationValidator struct {
	rules   []Rule
	results []Result
}

func NewOperationValidator(rules ...Rule) *OperationValidator {
	return &OperationValidator{
		rules:   rules,
		results: make([]Result, len(rules)),
	}
}

func (o *OperationValidator) Validate(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) []Result {

	o.results = o.results[:0]

	for i := range o.rules {
		o.results = append(o.results, o.rules[i](operationInput, schemaInput, operationDocument, schemaDocument))
	}

	return o.results
}

func OperationNameUniqueness() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {

		if len(operationDocument.OperationDefinitions) <= 1 {
			return Result{
				ValidationState: Valid,
			}
		}

		for i := range operationDocument.OperationDefinitions {
			for k := range operationDocument.OperationDefinitions {
				if i == k {
					continue
				}

				left := operationDocument.OperationDefinitions[i].Name
				right := operationDocument.OperationDefinitions[k].Name

				if input.ByteSliceEquals(left, operationInput, right, operationInput) {
					return Result{
						ValidationState: Invalid,
						Explanation:     fmt.Sprintf("Operation Name %s must be unique", string(operationInput.ByteSlice(operationDocument.OperationDefinitions[i].Name))),
					}
				}
			}
		}

		return Result{
			ValidationState: Valid,
		}
	}
}

func LoneAnonymousOperation() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {

		if len(operationDocument.OperationDefinitions) <= 1 {
			return Result{
				ValidationState: Valid,
			}
		}

		for i := range operationDocument.OperationDefinitions {
			if operationDocument.OperationDefinitions[i].Name.Length() == 0 {
				return Result{
					ValidationState: Invalid,
					Explanation:     "Anonymous Operation must be the only operation in a document.",
				}
			}
		}

		return Result{
			ValidationState: Valid,
		}
	}
}

func SubscriptionSingleRootField() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {

		for i := range operationDocument.OperationDefinitions {
			if operationDocument.OperationDefinitions[i].OperationType == ast.OperationTypeSubscription {
				selections := len(operationDocument.OperationDefinitions[i].SelectionSet.SelectionRefs)
				if selections > 1 {
					return Result{
						ValidationState: Invalid,
						Explanation:     "Subscription must only have one root selection",
					}
				} else if selections == 1 {
					ref := operationDocument.OperationDefinitions[i].SelectionSet.SelectionRefs[0]
					if operationDocument.Selections[ref].Kind == ast.SelectionKindField {
						return Result{
							ValidationState: Valid,
						}
					}
				}
			}
		}

		return Result{
			ValidationState: Valid,
		}
	}
}

func FieldSelections() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func FieldSelectionMerging() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func ValidArguments() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func Values() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func ArgumentUniqueness() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func RequiredArguments() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func Fragments() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func DirectivesAreDefined() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func DirectivesAreInValidLocations() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func VariableUniqueness() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func DirectivesAreUniquePerLocation() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func VariablesAreInputTypes() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func AllVariableUsesDefined() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}

func AllVariablesUsed() Rule {
	return func(operationInput, schemaInput *input.Input, operationDocument, schemaDocument *ast.Document) Result {
		return Result{}
	}
}
