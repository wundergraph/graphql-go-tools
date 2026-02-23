package resolve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type FakeErrorWriter struct{}

func (f *FakeErrorWriter) WriteError(ctx *Context, err error, res *GraphQLResponse, w io.Writer) {
}

type FakeSubscriptionWriter struct {
	mu                     sync.Mutex
	buf                    []byte
	writtenMessages        []string
	completed              bool
	closed                 bool
	messageCountOnComplete int
}

var _ SubscriptionResponseWriter = (*FakeSubscriptionWriter)(nil)

func (f *FakeSubscriptionWriter) Write(p []byte) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buf = append(f.buf, p...)
	return len(p), nil
}

func (f *FakeSubscriptionWriter) Flush() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writtenMessages = append(f.writtenMessages, string(f.buf))
	f.buf = nil
	return nil
}

func (f *FakeSubscriptionWriter) Complete() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completed = true
	f.messageCountOnComplete = len(f.writtenMessages)
}

// Heartbeat writes directly to the writtenMessages slice, as the real implementations implicitly flush
func (f *FakeSubscriptionWriter) Heartbeat() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writtenMessages = append(f.writtenMessages, string("heartbeat"))
	return nil
}

func (f *FakeSubscriptionWriter) Close(SubscriptionCloseKind) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.messageCountOnComplete = len(f.writtenMessages)
}

type FakeSource struct {
	updates  []string
	interval time.Duration
}

func (f *FakeSource) Start(ctx *Context, headers http.Header, input []byte, updater SubscriptionUpdater) error {
	go func() {
		for i, u := range f.updates {
			updater.Update([]byte(u))
			if i < len(f.updates)-1 {
				time.Sleep(f.interval)
			}
		}
		updater.Complete()
	}()
	return nil
}

type FailingHeartbeatWriter struct{}

func (f *FailingHeartbeatWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (f *FailingHeartbeatWriter) Flush() error {
	return nil
}

func (f *FailingHeartbeatWriter) Complete() {}

func (f *FailingHeartbeatWriter) Heartbeat() error {
	return errors.New("heartbeat failed")
}

func (f *FailingHeartbeatWriter) Close(SubscriptionCloseKind) {}

type TestReporter struct {
	triggers      atomic.Int64
	subscriptions atomic.Int64
}

func (t *TestReporter) SubscriptionUpdateSent() {
}

func (t *TestReporter) SubscriptionCountInc(count int) {
	t.subscriptions.Add(int64(count))
}

func (t *TestReporter) SubscriptionCountDec(count int) {
	t.subscriptions.Add(-int64(count))
}

func (t *TestReporter) TriggerCountInc(count int) {
	t.triggers.Add(int64(count))
}

func (t *TestReporter) TriggerCountDec(count int) {
	t.triggers.Add(-int64(count))
}

func TestEventLoop(t *testing.T) {
	resolverCtx, stopEventLoop := context.WithCancel(context.Background())
	t.Cleanup(stopEventLoop)

	ew := &FakeErrorWriter{}
	testReporter := &TestReporter{}

	resolver := New(resolverCtx, ResolverOptions{
		MaxConcurrency:                1024,
		Debug:                         false,
		AsyncErrorWriter:              ew,
		PropagateSubgraphErrors:       false,
		PropagateSubgraphStatusCodes:  false,
		SubgraphErrorPropagationMode:  SubgraphErrorPropagationModePassThrough,
		DefaultErrorExtensionCode:     "TEST",
		MaxRecyclableParserSize:       1024 * 1024,
		SubscriptionHeartbeatInterval: DefaultHeartbeatInterval,
		Reporter:                      testReporter,
	})

	subscription := &GraphQLSubscription{
		Trigger: GraphQLSubscriptionTrigger{
			InputTemplate: InputTemplate{},
			Source: &FakeSource{
				interval: time.Millisecond * 100,
				updates: []string{
					`{"data":{"counter":1}}`,
					`{"data":{"counter":2}}`,
					`{"data":{"counter":3}}`,
				},
			},
			PostProcessing: PostProcessingConfiguration{
				SelectResponseDataPath:   []string{"data"},
				SelectResponseErrorsPath: []string{"errors"},
			},
		},
		Response: &GraphQLResponse{
			Data: &Object{
				Fields: []*Field{
					{
						Name: []byte("counter"),
						Value: &Integer{
							Path:     []string{"counter"},
							Nullable: false,
						},
					},
				},
			},
		},
	}

	writer := &FakeSubscriptionWriter{}

	subscriptionCtx := &Context{}
	subscriptionCtx = subscriptionCtx.WithContext(context.Background())

	err := resolver.ResolveGraphQLSubscription(subscriptionCtx, subscription, writer)
	require.NoError(t, err)

	writer.mu.Lock()
	defer writer.mu.Unlock()
	require.Equal(t, true, writer.completed)
	require.Equal(t, 3, len(writer.writtenMessages))
	require.Equal(t, 3, writer.messageCountOnComplete)
	require.Equal(t, `{"data":{"counter":1}}`, writer.writtenMessages[0])
	require.Equal(t, `{"data":{"counter":2}}`, writer.writtenMessages[1])
	require.Equal(t, `{"data":{"counter":3}}`, writer.writtenMessages[2])

	stopEventLoop()

	require.Eventually(t, func() bool {
		triggerCount := testReporter.triggers.Load()
		subscriptionCount := testReporter.subscriptions.Load()
		require.Equal(t, int64(0), triggerCount)
		require.Equal(t, int64(0), subscriptionCount)
		return true
	}, time.Second, time.Millisecond*10)
}

func TestResolver_HeartbeatError_DoesNotDeadlockOnUnsubscribe(t *testing.T) {
	resolverCtx, cancelResolver := context.WithCancel(context.Background())
	defer cancelResolver()

	resolver := New(resolverCtx, ResolverOptions{
		MaxConcurrency:                1,
		AsyncErrorWriter:              &FakeErrorWriter{},
		SubscriptionHeartbeatInterval: time.Millisecond,
	})

	subCtx := (&Context{}).WithContext(context.Background())
	subID := SubscriptionIdentifier{
		ConnectionID:   1,
		SubscriptionID: 1,
	}
	triggerID := uint64(42)
	s := &subscriptionState{
		triggerID: triggerID,
		ctx:       subCtx,
		writer:    &FailingHeartbeatWriter{},
		id:        subID,
		heartbeat: true,
		completed: make(chan struct{}),
	}

	resolver.mu.Lock()
	resolver.triggers[triggerID] = &trigger{
		id:            triggerID,
		cancel:        func() {},
		subscriptions: map[SubscriptionIdentifier]*subscriptionState{subID: s},
		updateBuf:     bytes.NewBuffer(make([]byte, 0, 1024)),
	}
	resolver.subscriptionsByID[subID] = s
	resolver.subscriptionsByConnection[subID.ConnectionID] = map[SubscriptionIdentifier]*subscriptionState{subID: s}
	resolver.mu.Unlock()

	done := make(chan struct{})
	go func() {
		resolver.heartbeatTriggerSubscriptions(triggerID)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("heartbeatTriggerSubscriptions deadlocked after heartbeat error")
	}

	select {
	case <-s.completed:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("subscription was not closed after heartbeat failure")
	}
}
