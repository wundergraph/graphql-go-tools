package validator

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules"
	"github.com/jensneuse/graphql-go-tools/pkg/validation/rules/execution"
)

type Validator struct {
	l *lookup.Lookup
	w *lookup.Walker
}

func New() *Validator {
	return &Validator{
		w: lookup.NewWalker(1024, 8),
	}
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

func (v *Validator) SetInput(p *parser.Parser) {
	if v.l == nil {
		v.l = lookup.New(p, 256)
	} else {
		v.l.SetParser(p)
	}

	v.w.SetLookup(v.l)
}

func (v *Validator) ValidateExecutableDefinition(executionRules []rules.ExecutionRule) (result validation.Result) {
	v.w.WalkExecutable()
	for _, rule := range executionRules {
		result = rule(v.l, v.w)
		if !result.Valid {
			return
		}
	}

	return validation.Valid()
}
