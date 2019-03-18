package rules

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/validation"
)

type Rule func(l *lookup.Lookup, w *lookup.Walker) validation.Result

// https://facebook.github.io/graphql/draft/#sec-Executable-Definitions
// the parser impl does not allow parsing such documents

// https://facebook.github.io/graphql/draft/#sec-Lone-Anonymous-Operation
// the parser impl does not allow anonymous operations together with named operations and fragments
