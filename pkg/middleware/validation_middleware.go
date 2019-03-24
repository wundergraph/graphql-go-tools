package middleware

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/validator"
)

// ValidationMiddleware is a middleware which validates the input Query against the Schema definition
type ValidationMiddleware struct {
}

func (v *ValidationMiddleware) OnRequest(userValues map[string][]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {

	w.SetLookup(l)
	w.WalkExecutable()

	valid := validator.New()
	valid.SetInput(l, w)

	result := valid.ValidateExecutableDefinition(validator.DefaultExecutionRules)
	if result.Valid {
		return nil
	}

	return fmt.Errorf("ValidationMiddleware: Invalid Request: RuleName: %s, Description: %s", result.RuleName.String(), result.Description.String())
}

func (v *ValidationMiddleware) OnResponse(userValues map[string][]byte, response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {
	panic("implement me")
}
