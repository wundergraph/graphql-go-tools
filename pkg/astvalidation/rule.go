package astvalidation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

var reservedFieldPrefix = []byte{'_', '_'}

// Rule is hook to register callback functions on the Walker
type Rule func(walker *astvisitor.Walker)
