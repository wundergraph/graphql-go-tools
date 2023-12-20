//go:generate mockgen --build_flags=--mod=mod -self_package=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource,BeforeFetchHook,AfterFetchHook

package resolve

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type Resolver struct {
	ctx                 context.Context
	toolPool            sync.Pool
	limitMaxConcurrency bool
	maxConcurrency      chan struct{}
}

type tools struct {
	resolvable *Resolvable
	loader     *Loader
}

type ResolverOptions struct {
	// MaxConcurrency limits the number of concurrent resolve operations
	// if set to 0, no limit is applied
	// It is advised to set this to a reasonable value to prevent excessive memory usage
	// Each concurrent resolve operation allocates ~50kb of memory
	// In addition, there's a limit of how many concurrent requests can be efficiently resolved
	// This depends on the number of CPU cores available, the complexity of the operations, and the origin services
	MaxConcurrency int
}

// New returns a new Resolver, ctx.Done() is used to cancel all active subscriptions & streams
func New(ctx context.Context, options ResolverOptions) *Resolver {
	resolver := &Resolver{
		ctx: ctx,
		toolPool: sync.Pool{
			New: func() interface{} {
				return &tools{
					resolvable: NewResolvable(),
					loader:     &Loader{},
				}
			},
		},
	}
	if options.MaxConcurrency > 0 {
		semaphore := make(chan struct{}, options.MaxConcurrency)
		for i := 0; i < options.MaxConcurrency; i++ {
			semaphore <- struct{}{}
		}
		resolver.limitMaxConcurrency = true
		resolver.maxConcurrency = semaphore
	}
	return resolver
}

func (r *Resolver) getTools() *tools {
	if r.limitMaxConcurrency {
		<-r.maxConcurrency
	}
	t := r.toolPool.Get().(*tools)
	return t
}

func (r *Resolver) putTools(t *tools) {
	t.loader.Free()
	t.resolvable.Reset()
	r.toolPool.Put(t)
	if r.limitMaxConcurrency {
		r.maxConcurrency <- struct{}{}
	}
}

func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (err error) {
	if response.Info == nil {
		response.Info = &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		}
	}

	t := r.getTools()
	defer r.putTools(t)

	err = t.resolvable.Init(ctx, data, response.Info.OperationType)
	if err != nil {
		return err
	}

	err = t.loader.LoadGraphQLResponseData(ctx, response, t.resolvable)
	if err != nil {
		return err
	}

	return t.resolvable.Resolve(ctx.ctx, response.Data, writer)
}

func (r *Resolver) ResolveGraphQLSubscription(ctx *Context, subscription *GraphQLSubscription, writer FlushWriter) (err error) {

	if subscription.Trigger.Source == nil {
		msg := []byte(`{"errors":[{"message":"no data source found"}]}`)
		return writeAndFlush(writer, msg)
	}

	buf := bytes.NewBuffer(nil)
	err = subscription.Trigger.InputTemplate.Render(ctx, nil, buf)
	if err != nil {
		return err
	}
	subscriptionInput := buf.Bytes()

	if len(ctx.InitialPayload) > 0 {
		subscriptionInput, err = jsonparser.Set(subscriptionInput, ctx.InitialPayload, "initial_payload")
		if err != nil {
			return err
		}
	}

	if ctx.Extensions != nil {
		subscriptionInput, err = jsonparser.Set(subscriptionInput, ctx.Extensions, "body", "extensions")
	}

	c, cancel := context.WithCancel(ctx.Context())
	defer cancel()
	resolverDone := r.ctx.Done()

	next := make(chan []byte)

	cancellableContext := ctx.WithContext(c)

	if err := subscription.Trigger.Source.Start(cancellableContext, subscriptionInput, next); err != nil {
		if errors.Is(err, ErrUnableToResolve) {
			msg := []byte(`{"errors":[{"message":"unable to resolve"}]}`)
			return writeAndFlush(writer, msg)
		}
		return err
	}

	for {
		select {
		case <-resolverDone:
			return nil
		case data, ok := <-next:
			if !ok {
				return nil
			}
			t := r.getTools()
			if err := t.resolvable.InitSubscription(ctx, data, subscription.Trigger.PostProcessing); err != nil {
				r.putTools(t)
				return err
			}
			if err := t.loader.LoadGraphQLResponseData(ctx, subscription.Response, t.resolvable); err != nil {
				r.putTools(t)
				return err
			}
			if err := t.resolvable.Resolve(ctx.ctx, subscription.Response.Data, writer); err != nil {
				r.putTools(t)
				return err
			}
			writer.Flush()
			r.putTools(t)
		}
	}
}
