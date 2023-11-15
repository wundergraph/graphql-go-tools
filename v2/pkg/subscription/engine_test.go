package subscription

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/graphql"
)

func TestExecutorEngine_StartExecutionBackoff(t *testing.T) {
	t.Run("default retry case", func(t *testing.T) {
		wg := sync.WaitGroup{}
		wg.Add(2)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute)
		defer cancelFunc()

		executorMock := NewMockExecutor(ctrl)
		executorMock.EXPECT().SetContext(gomock.Eq(ctx)).Times(1)

		executeTimes := backoffTestData{
			executionCounter: 0,
			mu:               sync.RWMutex{},
		}
		var lastTime time.Time
		nextBackOff := 100 * time.Millisecond
		sampleErr := errors.New("failed to WebSocket dial")
		executorMock.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).Do(func(arg interface{}) {
			defer func() {
				lastTime = time.Now()
				executeTimes.IncreaseExecutionCounter()
			}()
			if executeTimes.GetExecutionCounterValue() == 0 {
				return
			}
			duration := time.Since(lastTime)
			if duration < nextBackOff {
				t.Fatalf("expected next retry after %s got %s", nextBackOff.String(), duration.String())
			}
			nextBackOff = nextBackOff * 2
		}).Return(sampleErr).AnyTimes()

		executorPoolMock := NewMockExecutorPool(ctrl)
		executorPoolMock.EXPECT().Put(gomock.Eq(executorMock)).Do(func(executor Executor) {
			wg.Done()
		})

		eventHandlerMock := NewMockEventHandler(ctrl)
		eventHandlerMock.EXPECT().Emit(EventTypeOnError, "testID", gomock.AssignableToTypeOf([]byte{}), gomock.AssignableToTypeOf(&ErrorTimeoutExecutingSubscription{})).AnyTimes()

		engine := ExecutorEngine{
			logger:           abstractlogger.Noop{},
			subCancellations: subscriptionCancellations{},
			executorPool:     executorPoolMock,
			bufferPool: &sync.Pool{
				New: func() interface{} {
					writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
					return &writer
				},
			},
			subscriptionUpdateInterval: time.Second,
			maxExecutionTries:          5,
			initialRetryWaitTime:       100 * time.Millisecond,
		}

		go engine.startSubscription(ctx, "testID", executorMock, eventHandlerMock)

		assert.Eventually(t, func() bool {
			result := executeTimes.GetExecutionCounterValue() >= 4
			if result {
				cancelFunc()
				wg.Done()
				wg.Wait()
			}
			return result
		}, time.Second*3, time.Millisecond*100)
	})

	t.Run("test max backoff", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute)
		defer cancelFunc()

		maxRetries := 3
		sampleErr := errors.New("failed to WebSocket dial")

		executorMock := NewMockExecutor(ctrl)
		executorMock.EXPECT().SetContext(gomock.AssignableToTypeOf(ctx)).Times(1)

		executorPoolMock := NewMockExecutorPool(ctrl)
		executorPoolMock.EXPECT().Put(gomock.Eq(executorMock))

		gottenError := backoffTestData{
			gottenError: false,
			mu:          sync.RWMutex{},
		}
		eventHandlerMock := NewMockEventHandler(ctrl)
		eventHandlerMock.EXPECT().Emit(EventTypeOnError, "testID", gomock.AssignableToTypeOf([]byte{}), gomock.AssignableToTypeOf(&ErrorTimeoutExecutingSubscription{})).Times(1).Do(func(arg0, arg1, arg2, arg3 interface{}) {
			gottenError.SetGottenError(true)
		})

		executorMock.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).Return(sampleErr).Times(maxRetries)

		engine := ExecutorEngine{
			logger:           abstractlogger.Noop{},
			subCancellations: subscriptionCancellations{},
			executorPool:     executorPoolMock,
			bufferPool: &sync.Pool{
				New: func() interface{} {
					writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
					return &writer
				},
			},
			subscriptionUpdateInterval: time.Second,
			maxExecutionTries:          maxRetries,
			initialRetryWaitTime:       100 * time.Millisecond,
		}

		go engine.startSubscription(ctx, "testID", executorMock, eventHandlerMock)

		assert.Eventually(t, func() bool {
			result := gottenError.GetGottenError()
			if result {
				cancelFunc()
			}
			return result
		}, time.Second*2, time.Millisecond*100)
	})
}

func TestExecutorEngine_StartOperation(t *testing.T) {
	t.Run("execute non-subscription operation", func(t *testing.T) {
		t.Run("on execution failure", func(t *testing.T) {
			wg := &sync.WaitGroup{}
			wg.Add(2)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx, cancelFunc := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancelFunc()

			idQuery := "1"
			payloadQuery := []byte(`{"query":"{ hello }"}`)

			idMutation := "2"
			payloadMutation := []byte(`{"query":"mutation { update }"}`)

			executorMock := NewMockExecutor(ctrl)
			executorMock.EXPECT().OperationType().
				Return(ast.OperationTypeQuery).
				Times(1)
			executorMock.EXPECT().OperationType().
				Return(ast.OperationTypeMutation).
				Times(1)
			executorMock.EXPECT().SetContext(assignableToContextWithCancel(ctx)).
				Times(2)
			executorMock.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).
				Return(errors.New("error")).
				Times(2)

			executorPoolMock := NewMockExecutorPool(ctrl)
			executorPoolMock.EXPECT().Get(gomock.Eq(payloadQuery)).
				Return(executorMock, nil).
				Times(1)
			executorPoolMock.EXPECT().Get(gomock.Eq(payloadMutation)).
				Return(executorMock, nil).
				Times(1)
			executorPoolMock.EXPECT().Put(gomock.Eq(executorMock)).
				Do(func(_ Executor) {
					wg.Done()
				}).
				Times(2)

			eventHandlerMock := NewMockEventHandler(ctrl)
			eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnError), gomock.Eq(idQuery), gomock.Nil(), gomock.Any()).
				Times(1)
			eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnError), gomock.Eq(idMutation), gomock.Nil(), gomock.Any()).
				Times(1)

			engine := ExecutorEngine{
				logger:           abstractlogger.Noop{},
				subCancellations: subscriptionCancellations{},
				executorPool:     executorPoolMock,
				bufferPool: &sync.Pool{
					New: func() interface{} {
						writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
						return &writer
					},
				},
				subscriptionUpdateInterval: 0,
			}

			assert.Eventually(t, func() bool {
				err := engine.StartOperation(ctx, idQuery, payloadQuery, eventHandlerMock)
				assert.NoError(t, err)

				err = engine.StartOperation(ctx, idMutation, payloadMutation, eventHandlerMock)
				assert.NoError(t, err)

				<-ctx.Done()
				wg.Wait()
				return true
			}, 1*time.Second, 10*time.Millisecond)
		})

		t.Run("on execution success", func(t *testing.T) {
			wg := &sync.WaitGroup{}
			wg.Add(2)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx, cancelFunc := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancelFunc()

			idQuery := "1"
			payloadQuery := []byte(`{"query":"{ hello }"}`)

			idMutation := "2"
			payloadMutation := []byte(`{"query":"mutation { update }"}`)

			executorMock := NewMockExecutor(ctrl)
			executorMock.EXPECT().OperationType().
				Return(ast.OperationTypeQuery).
				Times(1)
			executorMock.EXPECT().OperationType().
				Return(ast.OperationTypeMutation).
				Times(1)
			executorMock.EXPECT().SetContext(assignableToContextWithCancel(ctx)).
				Times(2)
			executorMock.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).
				Times(2)

			executorPoolMock := NewMockExecutorPool(ctrl)
			executorPoolMock.EXPECT().Get(gomock.Eq(payloadQuery)).
				Return(executorMock, nil).
				Times(1)
			executorPoolMock.EXPECT().Get(gomock.Eq(payloadMutation)).
				Return(executorMock, nil).
				Times(1)
			executorPoolMock.EXPECT().Put(gomock.Eq(executorMock)).
				Do(func(_ Executor) {
					wg.Done()
				}).
				Times(2)

			eventHandlerMock := NewMockEventHandler(ctrl)
			eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnNonSubscriptionExecutionResult), gomock.Eq(idQuery), gomock.AssignableToTypeOf([]byte{}), gomock.Nil()).
				Times(1)
			eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnNonSubscriptionExecutionResult), gomock.Eq(idMutation), gomock.AssignableToTypeOf([]byte{}), gomock.Nil()).
				Times(1)

			engine := ExecutorEngine{
				logger:           abstractlogger.Noop{},
				subCancellations: subscriptionCancellations{},
				executorPool:     executorPoolMock,
				bufferPool: &sync.Pool{
					New: func() interface{} {
						writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
						return &writer
					},
				},
				subscriptionUpdateInterval: 0,
			}

			assert.Eventually(t, func() bool {
				err := engine.StartOperation(ctx, idQuery, payloadQuery, eventHandlerMock)
				assert.NoError(t, err)

				err = engine.StartOperation(ctx, idMutation, payloadMutation, eventHandlerMock)
				assert.NoError(t, err)

				<-ctx.Done()
				wg.Wait()
				return true
			}, 1*time.Second, 10*time.Millisecond)
		})
	})

	t.Run("execute subscription operation", func(t *testing.T) {
		t.Run("on execution failure", func(t *testing.T) {
			wg := &sync.WaitGroup{}
			wg.Add(1)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx, cancelFunc := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancelFunc()

			id := "1"
			payload := []byte(`{"query":"subscription { receiveData }"}`)

			executorMock := NewMockExecutor(ctrl)
			executorMock.EXPECT().OperationType().
				Return(ast.OperationTypeSubscription).
				Times(1)
			executorMock.EXPECT().SetContext(assignableToContextWithCancel(ctx)).
				Times(1)
			executorMock.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).
				Return(errors.New("error")).
				MinTimes(1)

			executorPoolMock := NewMockExecutorPool(ctrl)
			executorPoolMock.EXPECT().Get(gomock.Eq(payload)).
				Return(executorMock, nil).
				Times(1)
			executorPoolMock.EXPECT().Put(gomock.Eq(executorMock)).
				Do(func(_ Executor) {
					wg.Done()
				}).
				Times(1)

			eventHandlerMock := NewMockEventHandler(ctrl)
			eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnError), gomock.Eq(id), gomock.Nil(), gomock.Any()).
				Times(1)

			engine := ExecutorEngine{
				logger:           abstractlogger.Noop{},
				subCancellations: subscriptionCancellations{},
				executorPool:     executorPoolMock,
				bufferPool: &sync.Pool{
					New: func() interface{} {
						writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
						return &writer
					},
				},
				subscriptionUpdateInterval: 2 * time.Millisecond,
				maxExecutionTries:          1,
			}

			assert.Eventually(t, func() bool {
				err := engine.StartOperation(ctx, id, payload, eventHandlerMock)
				<-ctx.Done()
				wg.Wait()
				return assert.NoError(t, err)
			}, 1*time.Second, 10*time.Millisecond)
		})

		t.Run("on execution success", func(t *testing.T) {
			wg := sync.WaitGroup{}
			wg.Add(1)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx, cancelFunc := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancelFunc()

			id := "1"
			payload := []byte(`{"query":"subscription { receiveData }"}`)

			executorMock := NewMockExecutor(ctrl)
			executorMock.EXPECT().OperationType().
				Return(ast.OperationTypeSubscription).
				Times(1)
			executorMock.EXPECT().SetContext(assignableToContextWithCancel(ctx)).
				Times(1)
			executorMock.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).
				Do(func(resultWriter *graphql.EngineResultWriter) {
					_, _ = resultWriter.Write([]byte(`{ "data": { "update": "newData" } }`))
				}).
				MinTimes(2)

			executorPoolMock := NewMockExecutorPool(ctrl)
			executorPoolMock.EXPECT().Get(gomock.Eq(payload)).
				Return(executorMock, nil).
				Times(1)
			executorPoolMock.EXPECT().Put(gomock.Eq(executorMock)).
				Do(func(_ Executor) {
					wg.Done()
				}).
				Times(1)

			eventHandlerMock := NewMockEventHandler(ctrl)
			eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnSubscriptionData), gomock.Eq(id), gomock.AssignableToTypeOf([]byte{}), gomock.Nil()).
				MinTimes(2)

			engine := ExecutorEngine{
				logger:           abstractlogger.Noop{},
				subCancellations: subscriptionCancellations{},
				executorPool:     executorPoolMock,
				bufferPool: &sync.Pool{
					New: func() interface{} {
						writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
						return &writer
					},
				},
				subscriptionUpdateInterval: 2 * time.Millisecond,
			}

			assert.Eventually(t, func() bool {
				err := engine.StartOperation(ctx, id, payload, eventHandlerMock)
				<-ctx.Done()
				wg.Wait()
				return assert.NoError(t, err)
			}, 1*time.Second, 10*time.Millisecond)
		})
	})

	t.Run("error on duplicate id", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		wg.Add(1)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx, cancelFunc := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancelFunc()

		id := "1"
		payloadSubscription := []byte(`{"query":"subscription { receiveData }"}`)
		payloadQuery := []byte(`{"query":"query { hello }"}`)

		executorMockQuery := NewMockExecutor(ctrl)
		executorMockSubscription := NewMockExecutor(ctrl)
		executorMockSubscription.EXPECT().OperationType().
			Return(ast.OperationTypeSubscription).
			Times(1)
		executorMockSubscription.EXPECT().SetContext(assignableToContextWithCancel(ctx)).
			Times(1)
		executorMockSubscription.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).
			Do(func(resultWriter *graphql.EngineResultWriter) {
				_, _ = resultWriter.Write([]byte(`{ "data": { "receiveData": "newData" } }`))
			}).
			Times(1)

		executorPoolMock := NewMockExecutorPool(ctrl)
		executorPoolMock.EXPECT().Get(gomock.Eq(payloadSubscription)).
			Return(executorMockSubscription, nil).
			Times(1)
		executorPoolMock.EXPECT().Get(gomock.Eq(payloadQuery)).
			Return(executorMockQuery, nil).
			Times(1)
		executorPoolMock.EXPECT().Put(gomock.Eq(executorMockSubscription)).
			Do(func(_ Executor) {
				wg.Done()
			}).
			Times(1)

		eventHandlerMock := NewMockEventHandler(ctrl)
		eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnDuplicatedSubscriberID), gomock.Eq(id), gomock.Nil(), gomock.Any()).
			Times(1)
		eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnSubscriptionData), gomock.Eq(id), gomock.AssignableToTypeOf([]byte{}), gomock.Nil()).
			Times(1)

		engine := ExecutorEngine{
			logger:           abstractlogger.Noop{},
			subCancellations: subscriptionCancellations{},
			executorPool:     executorPoolMock,
			bufferPool: &sync.Pool{
				New: func() interface{} {
					writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
					return &writer
				},
			},
			subscriptionUpdateInterval: 100 * time.Millisecond,
		}

		assert.Eventually(t, func() bool {
			err := engine.StartOperation(ctx, id, payloadSubscription, eventHandlerMock)
			assert.NoError(t, err)

			err = engine.StartOperation(ctx, id, payloadQuery, eventHandlerMock)
			assert.Error(t, err)

			<-ctx.Done()
			wg.Wait()
			return true
		}, 1*time.Second, 10*time.Millisecond)
	})
}

func TestExecutorEngine_StopSubscription(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	id := "1"
	payload := []byte(`{"query":"subscription { receiveData }"}`)

	eventHandlerMock := NewMockEventHandler(ctrl)
	eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnSubscriptionCompleted), gomock.Eq(id), gomock.Nil(), gomock.Nil()).
		Times(1)
	eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnSubscriptionData), gomock.Eq(id), gomock.AssignableToTypeOf([]byte{}), gomock.Nil()).
		MinTimes(1)

	executorMock := NewMockExecutor(ctrl)
	executorMock.EXPECT().OperationType().
		Return(ast.OperationTypeSubscription).
		Times(1)
	executorMock.EXPECT().SetContext(assignableToContextWithCancel(ctx)).
		Times(1)
	executorMock.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).
		Do(func(resultWriter *graphql.EngineResultWriter) {
			_, _ = resultWriter.Write([]byte(`{ "data": { "receiveData": "newData" } }`))
		}).
		MinTimes(1)

	executorPoolMock := NewMockExecutorPool(ctrl)
	executorPoolMock.EXPECT().Get(gomock.Eq(payload)).
		Return(executorMock, nil).
		Times(1)
	executorPoolMock.EXPECT().Put(gomock.Eq(executorMock)).
		Do(func(_ Executor) {
			wg.Done()
		}).
		Times(1)

	engine := ExecutorEngine{
		logger:           abstractlogger.Noop{},
		subCancellations: subscriptionCancellations{},
		executorPool:     executorPoolMock,
		bufferPool: &sync.Pool{
			New: func() interface{} {
				writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
				return &writer
			},
		},
		subscriptionUpdateInterval: 2 * time.Millisecond,
	}

	assert.Eventually(t, func() bool {
		err := engine.StartOperation(ctx, id, payload, eventHandlerMock)
		assert.NoError(t, err)
		assert.Equal(t, 1, engine.subCancellations.Len())
		time.Sleep(5 * time.Millisecond)

		err = engine.StopSubscription(id, eventHandlerMock)
		assert.NoError(t, err)
		assert.Equal(t, 0, engine.subCancellations.Len())
		wg.Wait()

		return true
	}, 1*time.Second, 5*time.Millisecond)
}

func TestExecutorEngine_TerminateAllConnections(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(3)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	payload := []byte(`{"query":"subscription { receiveData }"}`)

	eventHandlerMock := NewMockEventHandler(ctrl)
	eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnConnectionTerminatedByServer), gomock.Eq(""), gomock.Eq([]byte("connection terminated by server")), gomock.Nil()).
		Times(1)
	eventHandlerMock.EXPECT().Emit(gomock.Eq(EventTypeOnSubscriptionData), gomock.Any(), gomock.AssignableToTypeOf([]byte{}), gomock.Nil()).
		MinTimes(3)

	executorMock := NewMockExecutor(ctrl)
	executorMock.EXPECT().OperationType().
		Return(ast.OperationTypeSubscription).
		Times(3)
	executorMock.EXPECT().SetContext(assignableToContextWithCancel(ctx)).
		Times(3)
	executorMock.EXPECT().Execute(gomock.AssignableToTypeOf(&graphql.EngineResultWriter{})).
		Do(func(resultWriter *graphql.EngineResultWriter) {
			_, _ = resultWriter.Write([]byte(`{ "data": { "receiveData": "newData" } }`))
		}).
		MinTimes(3)

	executorPoolMock := NewMockExecutorPool(ctrl)
	executorPoolMock.EXPECT().Get(gomock.Eq(payload)).
		Return(executorMock, nil).
		Times(3)
	executorPoolMock.EXPECT().Put(gomock.Eq(executorMock)).
		Do(func(_ Executor) {
			wg.Done()
		}).
		Times(3)

	engine := ExecutorEngine{
		logger:           abstractlogger.Noop{},
		subCancellations: subscriptionCancellations{},
		executorPool:     executorPoolMock,
		bufferPool: &sync.Pool{
			New: func() interface{} {
				writer := graphql.NewEngineResultWriterFromBuffer(bytes.NewBuffer(make([]byte, 0, 1024)))
				return &writer
			},
		},
		subscriptionUpdateInterval: 2 * time.Millisecond,
	}

	assert.Eventually(t, func() bool {
		err := engine.StartOperation(ctx, "1", payload, eventHandlerMock)
		assert.NoError(t, err)
		err = engine.StartOperation(ctx, "2", payload, eventHandlerMock)
		assert.NoError(t, err)
		err = engine.StartOperation(ctx, "3", payload, eventHandlerMock)
		assert.NoError(t, err)
		assert.Equal(t, 3, engine.subCancellations.Len())
		time.Sleep(5 * time.Millisecond)

		err = engine.TerminateAllSubscriptions(eventHandlerMock)
		assert.NoError(t, err)
		assert.Equal(t, 0, engine.subCancellations.Len())
		wg.Wait()

		return true
	}, 1*time.Second, 5*time.Millisecond)
}

func assignableToContextWithCancel(ctx context.Context) gomock.Matcher {
	ctxWithCancel, _ := context.WithCancel(ctx) //nolint:govet
	return gomock.AssignableToTypeOf(ctxWithCancel)
}

type backoffTestData struct {
	executionCounter int
	gottenError      bool
	mu               sync.RWMutex
}

func (b *backoffTestData) IncreaseExecutionCounter() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.executionCounter++
}

func (b *backoffTestData) GetExecutionCounterValue() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.executionCounter
}

func (b *backoffTestData) SetGottenError(value bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gottenError = value
}

func (b *backoffTestData) GetGottenError() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.gottenError
}
