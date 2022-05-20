package astvalidation

import (
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

// Rule is hook to register callback functions on the Walker
type Rule func(walker *astvisitor.Walker)
