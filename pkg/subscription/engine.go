package subscription

//go:generate mockgen -destination=engine_mock_test.go -package=subscription . Engine
//go:generate mockgen -destination=websocket/engine_mock_test.go -package=websocket . Engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jensneuse/abstractlogger"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/graphql"
)

type errOnBeforeStartHookFailure struct {
	wrappedErr error
}

func (e *errOnBeforeStartHookFailure) Unwrap() error {
	return e.wrappedErr
}

func (e *errOnBeforeStartHookFailure) Error() string {
	return fmt.Sprintf("on before start hook failed: %s", e.wrappedErr.Error())
}

// Engine defines the function for a subscription engine.
type Engine interface {
	StartOperation(ctx context.Context, id string, payload []byte, eventHandler EventHandler) error
	StopSubscription(id string, eventHandler EventHandler) error
	TerminateAllSubscriptions(eventHandler EventHandler) error
}

type ErrorTimeoutExecutingSubscription struct {
	err error
}

func (e *ErrorTimeoutExecutingSubscription) Error() string {
	return fmt.Sprintf("error executing subsctiption: %v", e.err)
}

func (e *ErrorTimeoutExecutingSubscription) Unwrap() error {
	return e.err
}

// ExecutorEngine is an implementation of Engine and works with subscription.Executor.
type ExecutorEngine struct {
	logger abstractlogger.Logger
	// subCancellations is map containing the cancellation functions to every active subscription.
	subCancellations subscriptionCancellations
	// executorPool is responsible to create and hold executors.
	executorPool ExecutorPool
	// bufferPool will hold buffers.
	bufferPool *sync.Pool
	// subscriptionUpdateInterval is the actual interval on which the server sends subscription updates to the client.
	subscriptionUpdateInterval time.Duration
	// maxExecutionTries is the max amount of times the executeWithBackoff is allowed to run to before closing the connection
	maxExecutionTries int
}

// StartOperation will start any operation.
func (e *ExecutorEngine) StartOperation(ctx context.Context, id string, payload []byte, eventHandler EventHandler) error {
	executor, err := e.executorPool.Get(payload)
	if err != nil {
		return err
	}

	if err = e.handleOnBeforeStart(executor); err != nil {
		eventHandler.Emit(EventTypeOnError, id, nil, err)
		return &errOnBeforeStartHookFailure{wrappedErr: err}
	}

	if ctx, err = e.checkForDuplicateSubscriberID(ctx, id, eventHandler); err != nil {
		return err
	}

	if executor.OperationType() == ast.OperationTypeSubscription {
		go e.startSubscription(ctx, id, executor, eventHandler)
		return nil
	}

	go e.handleNonSubscriptionOperation(ctx, id, executor, eventHandler)
	return nil
}

// StopSubscription will stop an active subscription.
func (e *ExecutorEngine) StopSubscription(id string, eventHandler EventHandler) error {
	e.subCancellations.Cancel(id)
	eventHandler.Emit(EventTypeOnSubscriptionCompleted, id, nil, nil)
	return nil
}

// TerminateAllSubscriptions will cancel all active subscriptions.
func (e *ExecutorEngine) TerminateAllSubscriptions(eventHandler EventHandler) error {
	if e.subCancellations.Len() == 0 {
		return nil
	}

	for id := range e.subCancellations.cancellations {
		e.subCancellations.Cancel(id)
	}

	eventHandler.Emit(EventTypeOnConnectionTerminatedByServer, "", []byte("connection terminated by server"), nil)
	return nil
}

func (e *ExecutorEngine) handleOnBeforeStart(executor Executor) error {
	switch e := executor.(type) {
	case *ExecutorV2:
		if hook := e.engine.GetWebsocketBeforeStartHook(); hook != nil {
			return hook.OnBeforeStart(e.reqCtx, e.operation)
		}
	case *ExecutorV1:
		// do nothing
	}

	return nil
}

func (e *ExecutorEngine) checkForDuplicateSubscriberID(ctx context.Context, id string, eventHandler EventHandler) (context.Context, error) {
	ctx, subsErr := e.subCancellations.AddWithParent(id, ctx)
	if errors.Is(subsErr, ErrSubscriberIDAlreadyExists) {
		eventHandler.Emit(EventTypeOnDuplicatedSubscriberID, id, nil, subsErr)
		return ctx, subsErr
	} else if subsErr != nil {
		eventHandler.Emit(EventTypeOnError, id, nil, subsErr)
		return ctx, subsErr
	}
	return ctx, nil
}

func (e *ExecutorEngine) startSubscription(ctx context.Context, id string, executor Executor, eventHandler EventHandler) {
	defer func() {
		err := e.executorPool.Put(executor)
		if err != nil {
			e.logger.Error("subscription.Handle.startSubscription()",
				abstractlogger.Error(err),
			)
		}
	}()

	executor.SetContext(ctx)
	buf := e.bufferPool.Get().(*graphql.EngineResultWriter)
	buf.Reset()

	defer e.bufferPool.Put(buf)

	if err := e.executeSubscription(buf, id, executor, eventHandler); err != nil {
		e.logger.Error("subscription.Handle.startSubscription(): error executing subscription, terminating",
			abstractlogger.Error(err),
		)
		return
	}

	for {
		buf.Reset()
		select {
		case <-ctx.Done():
			return
		case <-time.After(e.subscriptionUpdateInterval):
			if err := e.executeSubscription(buf, id, executor, eventHandler); err != nil {
				e.logger.Error("subscription.Handle.startSubscription(): error executing subscription, terminating",
					abstractlogger.Error(err),
				)
				break
			}
		}
	}

}

func (e *ExecutorEngine) executeSubscription(buf *graphql.EngineResultWriter, id string, executor Executor, eventHandler EventHandler) error {
	buf.SetFlushCallback(func(data []byte) {
		e.logger.Debug("subscription.Handle.executeSubscription()",
			abstractlogger.ByteString("execution_result", data),
		)
		eventHandler.Emit(EventTypeOnSubscriptionData, id, data, nil)
	})
	defer buf.SetFlushCallback(nil)

	err := e.executeWithBackOff(executor, buf)
	if err != nil {
		e.logger.Error("subscription.Handle.executeSubscription()",
			abstractlogger.Error(err),
		)

		eventHandler.Emit(EventTypeOnError, id, nil, err)
		return err
	}

	if buf.Len() > 0 {
		data := buf.Bytes()
		e.logger.Debug("subscription.Handle.executeSubscription()",
			abstractlogger.ByteString("execution_result", data),
		)
		eventHandler.Emit(EventTypeOnSubscriptionData, id, data, nil)
	}

	return nil
}

// executeWithBackOff runs the executor wrapped in an exponential backOff algorithm of t=b^c
func (e *ExecutorEngine) executeWithBackOff(executor Executor, buf *graphql.EngineResultWriter) error {
	nextRetry := time.Second
	var err error
	trialCount := 0
	for {
		trialCount++
		err = executor.Execute(buf)
		if err == nil {
			break
		}
		nextRetry *= 2
		e.logger.Error("subscription.Handle.executeSubscription()",
			abstractlogger.Error(fmt.Errorf("%w. retrying in %s", err, nextRetry.String())),
		)

		if trialCount == e.maxExecutionTries {
			break
		}
		time.Sleep(nextRetry)
	}
	if err != nil {
		return &ErrorTimeoutExecutingSubscription{err}
	}
	return nil
}

func (e *ExecutorEngine) handleNonSubscriptionOperation(ctx context.Context, id string, executor Executor, eventHandler EventHandler) {
	defer func() {
		e.subCancellations.Cancel(id)
		err := e.executorPool.Put(executor)
		if err != nil {
			e.logger.Error("subscription.Handle.handleNonSubscriptionOperation()",
				abstractlogger.Error(err),
			)
		}
	}()

	executor.SetContext(ctx)
	buf := e.bufferPool.Get().(*graphql.EngineResultWriter)
	buf.Reset()

	defer e.bufferPool.Put(buf)

	err := executor.Execute(buf)
	if err != nil {
		e.logger.Error("subscription.Handle.handleNonSubscriptionOperation()",
			abstractlogger.Error(err),
		)

		eventHandler.Emit(EventTypeOnError, id, nil, err)
		return
	}

	e.logger.Debug("subscription.Handle.handleNonSubscriptionOperation()",
		abstractlogger.ByteString("execution_result", buf.Bytes()),
	)

	eventHandler.Emit(EventTypeOnNonSubscriptionExecutionResult, id, buf.Bytes(), err)
}

// Interface Guards
var _ Engine = (*ExecutorEngine)(nil)
