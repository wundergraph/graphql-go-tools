package validator

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules/execution"
)

type Validator struct {
	l *lookup.Lookup
	w *lookup.Walker
}

func New() *Validator {
	return &Validator{}
}

var (
	DefaultExecutionRules = []rules.ExecutionRule{
		execution.ArgumentUniqueness(),
		execution.RequiredArguments(),
		execution.ValidArguments(),
		execution.DirectivesAreUniquePerLocation(),
		execution.DirectivesAreInValidLocations(),
		execution.DirectivesAreDefined(),
		execution.FieldSelections(),
		execution.FieldSelectionMerging(),
		execution.Fragments(),
		execution.LoneAnonymousOperation(),
		execution.OperationNameUniqueness(),
		execution.SubscriptionSingleRootField(),
		execution.Values(),
		execution.VariablesAreInputTypes(),
		execution.VariableUniqueness(),
		execution.AllVariableUsesDefined(),
		execution.AllVariablesUsed(),
	}
)

func (v *Validator) SetInput(l *lookup.Lookup, w *lookup.Walker) {
	v.l = l
	v.w = w
}

func (v *Validator) ValidateExecutableDefinition(executionRules []rules.ExecutionRule) (result validation.Result) {

	for _, rule := range executionRules {
		result = rule(v.l, v.w)
		if !result.Valid {
			return
		}
	}

	return validation.Valid()
}
