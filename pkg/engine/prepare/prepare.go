// Package prepare takes a schema, data source definitions as well as a GraphQL Operation and prepares a statement from it
package prepare

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/statement"
)

type Planner struct {
}

func (p *Planner) PrepareStatement(operation, definition *ast.Document) (statement.Statement, error) {
	return statement.Statement{}, nil
}
