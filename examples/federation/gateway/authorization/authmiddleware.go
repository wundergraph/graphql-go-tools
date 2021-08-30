package authorization

import (
	"context"
	"fmt"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/graphql"
)

func NewMiddleware(
	checkRoles func(ctx context.Context, requiredRoles []string) error,
	definition *ast.Document,
) graphql.OperationMiddleware {
	return func(next graphql.OperationHandler) graphql.OperationHandler {
		return func(ctx context.Context, operation *graphql.Request, writer resolve.FlushWriter) error {
			operationDocument, err := operation.OperationDocument()
			if err != nil {
				return err
			}

			requiredRoles, err := GetRoles(operationDocument, definition)
			if err != nil {
				return err
			}

			if err = checkRoles(ctx, requiredRoles); err != nil {
				_, _ = fmt.Fprintf(writer, `{"errors":[{"message":"access denied, reason: %s"}]}`, err)
				return err
			}

			return next(ctx, operation, writer)
		}
	}
}
