package resolve_test

import (
	"context"
	"errors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func ExampleResponseErrorHandler() {
	ctx := resolve.NewContext(context.Background())
	ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
		gqlErr, ok := errors.AsType[*resolve.GraphQLError](err)
		if !ok {
			return err
		}
		source, ok := errors.AsType[resolve.SubgraphResponseError](err)
		if !ok {
			gqlErr.Message = "Request failed"
			return err
		}

		name := source.SubgraphName()
		code := name + ":SUBGRAPH_ERROR"
		if downstream := source.DownstreamErrors(); len(downstream) > 0 && downstream[0] != nil {
			for _, key := range []string{"code", "errorType", "classification"} {
				if candidate, ok := downstream[0].Extensions[key].(string); ok && candidate != "" {
					code = candidate
					break
				}
			}
		}
		gqlErr.Message = "Error from " + name + " - details redacted"
		gqlErr.Extensions = map[string]any{"code": code}
		return err
	}
}
