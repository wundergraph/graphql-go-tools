package execute

import (
	"io"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

type Executor struct {
}

type SynchronousResponseWriter interface {
	io.Writer
}

type StreamingResponseWriter interface {
	io.WriteCloser
	Flush() error
}

type SubscriptionResponseWriter interface {
	io.Writer
	Flush() error
}

func (e *Executor) PreparePlan(operation string) (plan plan.Reference, err error) {
	return
}

func (e *Executor) ExecuteSynchronousResponsePlan(ctx resolve.Context, planID int, writer SynchronousResponseWriter) (err error) {
	return
}

func (e *Executor) ExecuteStreamingResponsePlan(ctx resolve.Context, planID int, writer StreamingResponseWriter) (err error) {
	return
}

func (e *Executor) ExecuteSubscriptionResponsePlan(ctx resolve.Context, planID int, writer SubscriptionResponseWriter) (err error) {
	return
}
