package middleware

import (
	"context"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"github.com/jensneuse/graphql-go-tools/pkg/validator"
)

// ValidationMiddleware is a middleware which validates the input Query against the Schema definition
type ValidationMiddleware struct {
}

var validationMiddlewareSchemaExtension = []byte(`
scalar Int
scalar Float
scalar String
scalar Boolean
scalar ID
directive @include(
if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
directive @skip(
	if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
directive @deprecated(
	reason: String = "No longer supported"
) on FIELD_DEFINITION | ENUM_VALUE
`)

// PrepareSchema adds the base scalar and directive types to the schema so that the user doesn't have to add them
// if we omit these definitions from the schema definition the validation will fail
func (v *ValidationMiddleware) PrepareSchema(ctx context.Context, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {

	err := parser.ExtendTypeSystemDefinition(validationMiddlewareSchemaExtension)

	return err
}

func (v *ValidationMiddleware) OnRequest(ctx context.Context, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) error {

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

func (v *ValidationMiddleware) OnResponse(ctx context.Context, response *[]byte, l *lookup.Lookup, w *lookup.Walker, parser *parser.Parser, mod *parser.ManualAstMod) (err error) {
	panic("implement me")
}
