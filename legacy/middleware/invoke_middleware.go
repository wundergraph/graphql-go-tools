package middleware

import (
	"bytes"
	"context"
)

// InvokeMiddleware is a one off middleware invocation helper
// This should only be used for testing as it's a waste of resources
// It makes use of panics to don't use this in production!
func InvokeMiddleware(middleware GraphqlMiddleware, ctx context.Context, schema, request string) (result string, err error) {

	invoker := NewInvoker(middleware)
	err = invoker.SetSchema([]byte(schema))
	if err != nil {
		return
	}

	err = invoker.InvokeMiddleWares(ctx, []byte(request))
	if err != nil {
		return
	}

	buff := bytes.Buffer{}
	err = invoker.RewriteRequest(&buff)
	if err != nil {
		return
	}

	return buff.String(), err
}
