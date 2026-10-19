package resolve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

type concurrentSubscriptionDataSource struct {
	data []byte

	mu             sync.Mutex
	started        int
	startedEvents  chan int
	expected       int
	allStarted     chan struct{}
	allStartedOnce sync.Once
	release        chan struct{}
	releaseOnce    sync.Once
}

type panickingSubscriptionDataSource struct {
	panicValue any
}

type subscriptionUpdatePanic struct {
	label string
}

type sequencedSubscriptionFilterRenderer struct {
	mu                 sync.Mutex
	expectedContext    context.Context
	calls              int
	sawUnexpectedCtx   bool
	sawNonNilEventData bool
	secondErr          error
}

func (r *sequencedSubscriptionFilterRenderer) GetKind() string {
	return VariableRendererKindJson
}

func (r *sequencedSubscriptionFilterRenderer) RenderVariable(ctx context.Context, data *astjson.Value, out io.Writer) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++
	if ctx != r.expectedContext {
		r.sawUnexpectedCtx = true
	}
	if data != nil {
		r.sawNonNilEventData = true
	}
	if r.calls == 2 {
		return r.secondErr
	}
	_, err := out.Write([]byte(`"match"`))
	return err
}

func (r *sequencedSubscriptionFilterRenderer) results() (calls int, sawUnexpectedContext, sawNonNilEventData bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.sawUnexpectedCtx, r.sawNonNilEventData
}

type chronologicalSubscriptionWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	entries []string
}

func (w *chronologicalSubscriptionWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *chronologicalSubscriptionWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, w.buf.String())
	w.buf.Reset()
	return nil
}

func (w *chronologicalSubscriptionWriter) Complete() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, "<complete>")
}

func (w *chronologicalSubscriptionWriter) Heartbeat() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, "<heartbeat>")
	return nil
}

func (w *chronologicalSubscriptionWriter) Error(data []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, string(data))
}

func (w *chronologicalSubscriptionWriter) Entries() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.entries...)
}

type chronologicalSubscriptionErrorWriter struct{}

type panickingSubscriptionWriter struct {
	chronologicalSubscriptionWriter
	writeErr     error
	flushErr     error
	panicWrite   any
	panicFlush   any
	flushStarted chan struct{}
	flushRelease <-chan struct{}
}

func (w *panickingSubscriptionWriter) Write(p []byte) (int, error) {
	if w.panicWrite != nil {
		panic(w.panicWrite)
	}
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.chronologicalSubscriptionWriter.Write(p)
}

func (w *panickingSubscriptionWriter) Flush() error {
	if w.panicFlush != nil {
		panic(w.panicFlush)
	}
	if w.flushStarted != nil {
		close(w.flushStarted)
	}
	if w.flushRelease != nil {
		<-w.flushRelease
	}
	if w.flushErr != nil {
		return w.flushErr
	}
	return w.chronologicalSubscriptionWriter.Flush()
}

type panickingSubscriptionErrorWriter struct {
	panicValue any
}

func (w *panickingSubscriptionErrorWriter) WriteError(*Context, error, *GraphQLResponse, io.Writer) {
	panic(w.panicValue)
}

type subscriptionSingleFlightWaitContext struct {
	context.Context
	waitObserved chan struct{}
	once         sync.Once
}

func (c *subscriptionSingleFlightWaitContext) Done() <-chan struct{} {
	c.once.Do(func() {
		close(c.waitObserved)
	})
	return c.Context.Done()
}

type subscriptionSingleFlightLoaderHook struct {
	waitObserved chan struct{}
}

func (h *subscriptionSingleFlightLoaderHook) OnLoad(ctx context.Context, _ DataSourceInfo) context.Context {
	return &subscriptionSingleFlightWaitContext{Context: ctx, waitObserved: h.waitObserved}
}

func (h *subscriptionSingleFlightLoaderHook) OnFinished(context.Context, DataSourceInfo, *ResponseInfo) {
}

type subscriptionUpdateCountingReporter struct {
	mu            sync.Mutex
	updates       int
	subscriptions int
	triggers      int
}

type subscriptionUpdateReporterSnapshot struct {
	updates       int
	subscriptions int
	triggers      int
}

func (r *subscriptionUpdateCountingReporter) SubscriptionUpdateSent() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates++
}

func (r *subscriptionUpdateCountingReporter) SubscriptionCountInc(count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions += count
}

func (r *subscriptionUpdateCountingReporter) SubscriptionCountDec(count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions -= count
}

func (r *subscriptionUpdateCountingReporter) TriggerCountInc(count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.triggers += count
}

func (r *subscriptionUpdateCountingReporter) TriggerCountDec(count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.triggers -= count
}

func (r *subscriptionUpdateCountingReporter) snapshot() subscriptionUpdateReporterSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return subscriptionUpdateReporterSnapshot{
		updates:       r.updates,
		subscriptions: r.subscriptions,
		triggers:      r.triggers,
	}
}

func (w *chronologicalSubscriptionErrorWriter) WriteError(_ *Context, err error, _ *GraphQLResponse, out io.Writer) {
	_, _ = fmt.Fprintf(out, `{"errors":[{"message":%q}]}`, err.Error())
	if flusher, ok := out.(interface{ Flush() error }); ok {
		_ = flusher.Flush()
	}
}

func (d *panickingSubscriptionDataSource) Load(context.Context, http.Header, []byte) ([]byte, error) {
	panic(d.panicValue)
}

func (d *panickingSubscriptionDataSource) LoadWithFiles(context.Context, http.Header, []byte, []*httpclient.FileUpload) ([]byte, error) {
	panic(d.panicValue)
}

func newConcurrentSubscriptionDataSource(expected int) *concurrentSubscriptionDataSource {
	return &concurrentSubscriptionDataSource{
		data:          []byte(`{"data":{"resolved":"value"}}`),
		expected:      expected,
		startedEvents: make(chan int, expected+1),
		allStarted:    make(chan struct{}),
		release:       make(chan struct{}),
	}
}

func (d *concurrentSubscriptionDataSource) load() ([]byte, error) {
	d.mu.Lock()
	d.started++
	started := d.started
	if d.started == d.expected {
		d.allStartedOnce.Do(func() {
			close(d.allStarted)
		})
	}
	d.mu.Unlock()
	select {
	case d.startedEvents <- started:
	default:
	}

	<-d.release
	return d.data, nil
}

func (d *concurrentSubscriptionDataSource) Load(context.Context, http.Header, []byte) ([]byte, error) {
	return d.load()
}

func (d *concurrentSubscriptionDataSource) LoadWithFiles(context.Context, http.Header, []byte, []*httpclient.FileUpload) ([]byte, error) {
	return d.load()
}

func (d *concurrentSubscriptionDataSource) AllStarted() <-chan struct{} {
	return d.allStarted
}

func (d *concurrentSubscriptionDataSource) StartedEvents() <-chan int {
	return d.startedEvents
}

func (d *concurrentSubscriptionDataSource) Started() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.started
}

func (d *concurrentSubscriptionDataSource) Release() {
	d.releaseOnce.Do(func() {
		close(d.release)
	})
}

func newSubscriptionUpdaterHarness(t *testing.T, count int, dataSource DataSource) (*subscriptionUpdater, []SubscriptionIdentifier, []*SubscriptionRecorder, func()) {
	t.Helper()

	resolverCtx, cancelResolver := context.WithCancel(t.Context())
	resolver := New(resolverCtx, ResolverOptions{
		MaxConcurrency:                count,
		AsyncErrorWriter:              &FakeErrorWriter{},
		SubscriptionHeartbeatInterval: time.Hour,
	})

	const triggerID uint64 = 1
	triggerCtx, cancelTrigger := context.WithCancel(resolverCtx)
	updater := &subscriptionUpdater{
		triggerID: triggerID,
		resolver:  resolver,
		ctx:       triggerCtx,
	}
	trig := &trigger{
		id:            triggerID,
		cancel:        cancelTrigger,
		subscriptions: make(map[SubscriptionIdentifier]*subscriptionState, count),
		updater:       updater,
	}
	updater.subsFn = trig.subscriptionIds

	ids := make([]SubscriptionIdentifier, 0, count)
	recorders := make([]*SubscriptionRecorder, 0, count)
	completed := make([]<-chan struct{}, 0, count)

	resolver.mu.Lock()
	resolver.triggers[triggerID] = trig
	for i := 0; i < count; i++ {
		id := SubscriptionIdentifier{
			ConnectionID:   ConnectionID(i + 1),
			SubscriptionID: int64(i + 1),
		}
		recorder := &SubscriptionRecorder{buf: &bytes.Buffer{}}
		subscriptionCtx := NewContext(resolverCtx)
		subscriptionCtx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
		completion := make(chan struct{})

		state := &subscriptionState{
			triggerID: triggerID,
			resolve: &GraphQLSubscription{
				Trigger: GraphQLSubscriptionTrigger{
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				Response: &GraphQLResponse{
					Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
					Fetches: Single(&SingleFetch{
						FetchConfiguration: FetchConfiguration{
							DataSource: dataSource,
							PostProcessing: PostProcessingConfiguration{
								SelectResponseDataPath: []string{"data"},
							},
						},
						InputTemplate: InputTemplate{
							Segments: []TemplateSegment{{
								SegmentType: StaticSegmentType,
								Data:        []byte(`{}`),
							}},
						},
						Info: &FetchInfo{
							DataSourceID:   "source",
							DataSourceName: "source",
							OperationType:  ast.OperationTypeQuery,
							QueryPlan: &QueryPlan{
								Query: "query { resolved }",
							},
						},
					}),
					Data: &Object{
						Fields: []*Field{
							{
								Name: []byte("value"),
								Value: &String{
									Path: []string{"value"},
								},
							},
							{
								Name: []byte("resolved"),
								Value: &String{
									Path: []string{"resolved"},
								},
							},
						},
					},
				},
			},
			ctx:       subscriptionCtx,
			writer:    recorder,
			id:        id,
			completed: completion,
		}

		resolver.registerSubscriptionLocked(trig, state)
		ids = append(ids, id)
		recorders = append(recorders, recorder)
		completed = append(completed, completion)
	}
	resolver.mu.Unlock()

	cleanup := func() {
		if releaser, ok := dataSource.(interface{ Release() }); ok {
			releaser.Release()
		}
		cancelResolver()
		resolver.shutdownResolver()

		deadline := time.NewTimer(time.Second)
		defer deadline.Stop()
		for _, done := range completed {
			select {
			case <-done:
			case <-deadline.C:
				t.Errorf("timed out waiting for subscription updater harness cleanup")
				return
			}
		}
	}

	return updater, ids, recorders, cleanup
}

func subscriptionUpdateTail(updater *subscriptionUpdater, id SubscriptionIdentifier) chan struct{} {
	updater.updateMu.Lock()
	defer updater.updateMu.Unlock()
	return updater.updateTails[id]
}

func waitForSubscriptionUpdateTailChange(t *testing.T, updater *subscriptionUpdater, id SubscriptionIdentifier, previous chan struct{}) chan struct{} {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		current := subscriptionUpdateTail(updater, id)
		if current != nil && current != previous {
			return current
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for subscription update tail to change")
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForSubscriptionUpdateCall(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("timed out waiting for subscription update call")
	}
}

func requireNoSubscriptionUpdateTails(t *testing.T, updater *subscriptionUpdater) {
	t.Helper()
	updater.updateMu.Lock()
	defer updater.updateMu.Unlock()
	require.Empty(t, updater.updateTails)
}

func waitForSubscriptionUpdates(t *testing.T, updater *subscriptionUpdater) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		updater.mu.Lock()
		defer updater.mu.Unlock()
		updater.updateWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("timed out waiting for admitted subscription updates")
	}
}

func startSubscriptionUpdaterCall(call func()) (started, done <-chan struct{}) {
	startedCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		close(startedCh)
		defer close(doneCh)
		call()
	}()
	return startedCh, doneCh
}

func waitForSubscriptionUpdaterCallStart(t *testing.T, started <-chan struct{}, deadline time.Time) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Until(deadline)):
		t.Fatal("timed out waiting for subscription updater call to start")
	}
}

func waitForSubscriptionUpdaterCallToEnter(t *testing.T, updater *subscriptionUpdater, done <-chan struct{}, deadline time.Time) {
	t.Helper()
	for {
		if !updater.mu.TryLock() {
			return
		}
		updater.mu.Unlock()
		select {
		case <-done:
			t.Error("subscription updater call returned before reaching the lifecycle barrier")
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for subscription updater call to reach the lifecycle barrier")
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForSubscriptionUpdaterTerminating(t *testing.T, updater *subscriptionUpdater, done <-chan struct{}, deadline time.Time) {
	t.Helper()
	for !updater.terminating.Load() {
		select {
		case <-done:
			t.Error("subscription updater call returned before publishing termination")
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for subscription updater to publish termination")
		}
		time.Sleep(time.Millisecond)
	}
}

func checkSubscriptionUpdaterCallBlocked(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
		t.Error("subscription updater call returned before admitted update was released")
	case <-time.After(50 * time.Millisecond):
	}
}

func waitForSubscriptionUpdaterCall(t *testing.T, done <-chan struct{}, deadline time.Time) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Until(deadline)):
		t.Error("timed out waiting for subscription updater call")
	}
}

func requireSubscriptionUpdaterCallShortCircuitsWithoutResolver(t *testing.T, updater *subscriptionUpdater, name string, call func()) {
	t.Helper()
	resolver := updater.resolver
	updater.resolver = nil
	defer func() {
		updater.resolver = resolver
	}()
	require.NotPanics(t, call, "%s reached the resolver instead of short-circuiting", name)
}

func subscriptionUpdaterHasID(subscriptions map[context.Context]SubscriptionIdentifier, id SubscriptionIdentifier) bool {
	for _, subscriptionID := range subscriptions {
		if subscriptionID == id {
			return true
		}
	}
	return false
}

func cancelSubscriptionUpdater(t *testing.T, updater *subscriptionUpdater) {
	t.Helper()
	trig, ok := updater.resolver.getTrigger(updater.triggerID)
	require.True(t, ok)
	trig.cancel()
}

func configureSubscriptionUpdaterState(t *testing.T, updater *subscriptionUpdater, id SubscriptionIdentifier, configure func(*subscriptionState)) {
	t.Helper()
	trig, ok := updater.resolver.getTrigger(updater.triggerID)
	require.True(t, ok)
	trig.mu.Lock()
	defer trig.mu.Unlock()
	state, ok := trig.subscriptions[id]
	require.True(t, ok)
	configure(state)
}

func TestSubscriptionState_Terminal_CompleteSuppressesLaterFrames(t *testing.T) {
	writer := &chronologicalSubscriptionWriter{}
	ctx := NewContext(t.Context())
	state := &subscriptionState{
		ctx:       ctx,
		writer:    writer,
		completed: make(chan struct{}),
	}

	state.complete()
	sent, err := state.sendHeartbeat()
	require.NoError(t, err)
	require.False(t, sent)
	state.writeError(&chronologicalSubscriptionErrorWriter{}, ctx, errors.New("late write error"), &GraphQLResponse{})
	state.error([]byte(`{"errors":[{"message":"late terminal error"}]}`))

	require.Equal(t, []string{"<complete>"}, writer.Entries())
}

func TestSubscriptionState_Terminal_ErrorSuppressesLaterFrames(t *testing.T) {
	const terminalError = `{"errors":[{"message":"terminal"}]}`
	writer := &chronologicalSubscriptionWriter{}
	ctx := NewContext(t.Context())
	state := &subscriptionState{
		ctx:       ctx,
		writer:    writer,
		completed: make(chan struct{}),
	}

	state.error([]byte(terminalError))
	sent, err := state.sendHeartbeat()
	require.NoError(t, err)
	require.False(t, sent)
	state.writeError(&chronologicalSubscriptionErrorWriter{}, ctx, errors.New("late write error"), &GraphQLResponse{})
	state.complete()

	require.Equal(t, []string{terminalError}, writer.Entries())
}

func TestSubscriptionState_RemovedSuppressesTerminalFrames(t *testing.T) {
	const terminalError = `{"errors":[{"message":"terminal"}]}`
	tests := []struct {
		name    string
		deliver func(*subscriptionState)
	}{
		{
			name: "Complete",
			deliver: func(state *subscriptionState) {
				state.complete()
			},
		},
		{
			name: "Error",
			deliver: func(state *subscriptionState) {
				state.error([]byte(terminalError))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := &chronologicalSubscriptionWriter{}
			state := &subscriptionState{
				ctx:       NewContext(t.Context()),
				writer:    writer,
				completed: make(chan struct{}),
			}
			state.removed.Store(true)

			test.deliver(state)

			require.Empty(t, writer.Entries())
			require.False(t, state.terminal)
		})
	}
}

func TestSubscriptionState_Terminal_CompetingTerminalFramesEmitExactlyOnce(t *testing.T) {
	const terminalError = `{"errors":[{"message":"terminal"}]}`
	writer := &chronologicalSubscriptionWriter{}
	state := &subscriptionState{
		ctx:       NewContext(t.Context()),
		writer:    writer,
		completed: make(chan struct{}),
	}

	start := make(chan struct{})
	var calls sync.WaitGroup
	for i := 0; i < 32; i++ {
		calls.Add(1)
		go func(i int) {
			defer calls.Done()
			<-start
			if i%2 == 0 {
				state.complete()
				return
			}
			state.error([]byte(terminalError))
		}(i)
	}
	close(start)

	done := make(chan struct{})
	go func() {
		calls.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for competing terminal frames")
	}

	entries := writer.Entries()
	require.Len(t, entries, 1)
	require.True(t, entries[0] == "<complete>" || entries[0] == terminalError,
		"unexpected terminal frame %q", entries[0])
}

func TestSubscriptionState_Terminal_LateDataDropsResolvedData(t *testing.T) {
	const terminalError = `{"errors":[{"message":"terminal"}]}`
	tests := []struct {
		name     string
		terminal string
		deliver  func(*Resolver, uint64)
	}{
		{
			name:     "Complete",
			terminal: "<complete>",
			deliver: func(resolver *Resolver, triggerID uint64) {
				resolver.handleTriggerComplete(triggerID)
			},
		},
		{
			name:     "Error",
			terminal: terminalError,
			deliver: func(resolver *Resolver, triggerID uint64) {
				resolver.handleTriggerError(triggerID, []byte(terminalError))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataSource := newConcurrentSubscriptionDataSource(1)
			updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
			defer cleanup()

			writer := &chronologicalSubscriptionWriter{}
			var state *subscriptionState
			configureSubscriptionUpdaterState(t, updater, ids[0], func(subscription *subscriptionState) {
				subscription.writer = writer
				state = subscription
			})

			updateDone := make(chan struct{})
			go func() {
				defer close(updateDone)
				updater.resolver.executeSubscriptionUpdate(state.ctx, state, []byte(`{"data":{"value":"late"}}`))
			}()
			defer func() {
				dataSource.Release()
				select {
				case <-updateDone:
				case <-time.After(time.Second):
					t.Error("timed out joining late subscription update")
				}
			}()

			select {
			case <-dataSource.AllStarted():
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for late subscription update to enter the data source")
			}

			test.deliver(updater.resolver, updater.triggerID)
			require.Equal(t, []string{test.terminal}, writer.Entries())

			dataSource.Release()
			select {
			case <-updateDone:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for late subscription update to return")
			}
			require.Equal(t, []string{test.terminal}, writer.Entries())
		})
	}
}

func TestSubscriptionState_WritePanic_ReleasesLifecycleLock(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Resolver, *panickingSubscriptionWriter, any)
	}{
		{
			name: "Write",
			configure: func(_ *Resolver, writer *panickingSubscriptionWriter, panicValue any) {
				writer.panicWrite = panicValue
			},
		},
		{
			name: "Flush",
			configure: func(_ *Resolver, writer *panickingSubscriptionWriter, panicValue any) {
				writer.panicFlush = panicValue
			},
		},
		{
			name: "ErrorFormatter",
			configure: func(resolver *Resolver, writer *panickingSubscriptionWriter, panicValue any) {
				writer.writeErr = errors.New("subscription writer failed")
				resolver.SetAsyncErrorWriter(&panickingSubscriptionErrorWriter{panicValue: panicValue})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataSource := newConcurrentSubscriptionDataSource(1)
			dataSource.Release()
			updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)

			panicValue := &subscriptionUpdatePanic{label: test.name}
			writer := &panickingSubscriptionWriter{}
			test.configure(updater.resolver, writer, panicValue)
			var state *subscriptionState
			var completed <-chan struct{}
			configureSubscriptionUpdaterState(t, updater, ids[0], func(subscription *subscriptionState) {
				subscription.writer = writer
				state = subscription
				completed = subscription.completed
			})
			arenaKey := state.ctx.Request.ID
			expectedArena := updater.resolver.resolveArenaPool.Acquire(arenaKey)
			updater.resolver.resolveArenaPool.Release(expectedArena)

			var recovered any
			func() {
				defer func() {
					recovered = recover()
				}()
				updater.resolver.executeSubscriptionUpdate(state.ctx, state, []byte(`{"data":{"value":"event"}}`))
			}()
			require.Same(t, panicValue, recovered)
			observedArena := updater.resolver.resolveArenaPool.Acquire(arenaKey)
			require.Same(t, expectedArena, observedArena, "resolve arena was not returned after panic")
			updater.resolver.resolveArenaPool.Release(observedArena)

			done := make(chan struct{})
			go func() {
				updater.Done()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("Done blocked on writeMu after recovered writer panic")
			}
			select {
			case <-completed:
			case <-time.After(time.Second):
				t.Fatal("Done did not close completion after recovered writer panic")
			}
			cleanup()
		})
	}
}

func TestSubscriptionState_ArenaReleasedAfterInitializationError(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	dataSource.Release()
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	writer := &chronologicalSubscriptionWriter{}
	var state *subscriptionState
	configureSubscriptionUpdaterState(t, updater, ids[0], func(subscription *subscriptionState) {
		subscription.writer = writer
		state = subscription
	})

	arenaKey := state.ctx.Request.ID
	expectedArena := updater.resolver.resolveArenaPool.Acquire(arenaKey)
	updater.resolver.resolveArenaPool.Release(expectedArena)

	updater.resolver.executeSubscriptionUpdate(state.ctx, state, []byte(`{`))

	observedArena := updater.resolver.resolveArenaPool.Acquire(arenaKey)
	require.Same(t, expectedArena, observedArena, "resolve arena was not returned after initialization error")
	updater.resolver.resolveArenaPool.Release(observedArena)
	require.Zero(t, dataSource.Started())
}

func TestSubscriptionState_ArenaReleasedBeforeBlockingFlush(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	dataSource.Release()
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	flushStarted := make(chan struct{})
	flushRelease := make(chan struct{})
	writer := &panickingSubscriptionWriter{
		flushStarted: flushStarted,
		flushRelease: flushRelease,
	}
	var state *subscriptionState
	configureSubscriptionUpdaterState(t, updater, ids[0], func(subscription *subscriptionState) {
		subscription.writer = writer
		state = subscription
	})

	arenaKey := state.ctx.Request.ID
	expectedArena := updater.resolver.resolveArenaPool.Acquire(arenaKey)
	updater.resolver.resolveArenaPool.Release(expectedArena)

	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"event"}}`))
	}()
	select {
	case <-flushStarted:
	case <-time.After(time.Second):
		close(flushRelease)
		t.Fatal("timed out waiting for blocking Flush")
	}

	observedArena := updater.resolver.resolveArenaPool.Acquire(arenaKey)
	reusedBeforeFlushReturned := observedArena == expectedArena
	updater.resolver.resolveArenaPool.Release(observedArena)
	close(flushRelease)
	waitForSubscriptionUpdateCall(t, updateDone)

	require.True(t, reusedBeforeFlushReturned, "resolve arena remained checked out while Flush was blocked")
}

func TestResolver_TerminalHeartbeat_DoesNotWriteAfterTerminal(t *testing.T) {
	const terminalError = `{"errors":[{"message":"terminal"}]}`
	tests := []struct {
		name     string
		terminal string
		deliver  func(*subscriptionState)
	}{
		{
			name:     "Complete",
			terminal: "<complete>",
			deliver: func(state *subscriptionState) {
				state.complete()
			},
		},
		{
			name:     "Error",
			terminal: terminalError,
			deliver: func(state *subscriptionState) {
				state.error([]byte(terminalError))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataSource := newConcurrentSubscriptionDataSource(1)
			dataSource.Release()
			updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
			defer cleanup()
			updater.resolver.heartbeatInterval = 0

			writer := &chronologicalSubscriptionWriter{}
			var state *subscriptionState
			configureSubscriptionUpdaterState(t, updater, ids[0], func(subscription *subscriptionState) {
				subscription.writer = writer
				subscription.heartbeat = true
				state = subscription
			})

			test.deliver(state)
			updater.resolver.heartbeatTriggerSubscriptions(updater.triggerID)

			require.Equal(t, []string{test.terminal}, writer.Entries())
		})
	}
}

func TestResolver_TerminalHeartbeat_ReportsOnlySentFrames(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	dataSource.Release()
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 2, dataSource)
	defer cleanup()

	reporter := &subscriptionUpdateCountingReporter{}
	updater.resolver.reporter = reporter
	writers := make([]*chronologicalSubscriptionWriter, len(ids))
	states := make([]*subscriptionState, len(ids))
	for i, id := range ids {
		writers[i] = &chronologicalSubscriptionWriter{}
		configureSubscriptionUpdaterState(t, updater, id, func(state *subscriptionState) {
			state.writer = writers[i]
			state.heartbeat = true
			states[i] = state
		})
	}

	states[0].complete()
	updater.resolver.executeSubscriptionHeartbeat(states[0])
	require.Zero(t, reporter.snapshot().updates, "suppressed terminal heartbeat was reported as sent")
	require.Equal(t, []string{"<complete>"}, writers[0].Entries())

	updater.resolver.executeSubscriptionHeartbeat(states[1])
	require.Equal(t, 1, reporter.snapshot().updates)
	require.Equal(t, []string{"<heartbeat>"}, writers[1].Entries())
}

func TestSubscriptionUpdater_DirectUnsubscribe_DoesNotWaitForInFlightUpdate(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	defer dataSource.Release()
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	writer := &chronologicalSubscriptionWriter{}
	var completed <-chan struct{}
	configureSubscriptionUpdaterState(t, updater, ids[0], func(state *subscriptionState) {
		state.writer = writer
		completed = state.completed
	})

	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"late"}}`))
	}()
	defer func() {
		dataSource.Release()
		select {
		case <-updateDone:
		case <-time.After(time.Second):
			t.Error("timed out joining unsubscribed subscription update")
		}
	}()

	select {
	case <-dataSource.AllStarted():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription update to enter the data source")
	}

	unsubscribeDone := make(chan error, 1)
	go func() {
		unsubscribeDone <- updater.resolver.UnsubscribeSubscription(ids[0])
	}()
	select {
	case err := <-unsubscribeDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("direct unsubscribe waited for the in-flight updater barrier")
	}
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("direct unsubscribe did not close the subscription completion channel")
	}

	dataSource.Release()
	select {
	case <-updateDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for late resolve after direct unsubscribe")
	}
	require.Empty(t, writer.Entries())
	waitForSubscriptionUpdates(t, updater)
	requireNoSubscriptionUpdateTails(t, updater)
}

func TestSubscriptionUpdater_Lifecycle_WaitsForAdmittedSubscriptionUpdate(t *testing.T) {
	const (
		admittedData  = `{"data":{"value":"admitted"}}`
		broadcastData = `{"data":{"value":"broadcast"}}`
		terminalError = `{"errors":[{"message":"terminal"}]}`
	)

	tests := []struct {
		name         string
		invoke       func(*subscriptionUpdater, SubscriptionIdentifier)
		assertAbsent func(*testing.T, *subscriptionUpdater, SubscriptionIdentifier, *concurrentSubscriptionDataSource, *chronologicalSubscriptionWriter, <-chan struct{})
		assertEffect func(*testing.T, *subscriptionUpdater, SubscriptionIdentifier, *concurrentSubscriptionDataSource, *chronologicalSubscriptionWriter, <-chan struct{}, time.Time)
	}{
		{
			name: "Update",
			invoke: func(updater *subscriptionUpdater, _ SubscriptionIdentifier) {
				updater.Update([]byte(broadcastData))
			},
			assertAbsent: func(t *testing.T, _ *subscriptionUpdater, _ SubscriptionIdentifier, dataSource *concurrentSubscriptionDataSource, _ *chronologicalSubscriptionWriter, _ <-chan struct{}) {
				require.Equal(t, 1, dataSource.Started(), "broadcast fetch started before admitted update completed")
			},
			assertEffect: func(t *testing.T, _ *subscriptionUpdater, _ SubscriptionIdentifier, dataSource *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, _ <-chan struct{}, _ time.Time) {
				require.Equal(t, 2, dataSource.Started())
				require.Equal(t, []string{
					`{"data":{"value":"admitted","resolved":"value"}}`,
					`{"data":{"value":"broadcast","resolved":"value"}}`,
				}, writer.Entries())
			},
		},
		{
			name: "Heartbeat",
			invoke: func(updater *subscriptionUpdater, _ SubscriptionIdentifier) {
				updater.Heartbeat()
			},
			assertAbsent: func(t *testing.T, _ *subscriptionUpdater, _ SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, _ <-chan struct{}) {
				require.Empty(t, writer.Entries(), "heartbeat was written before admitted update completed")
			},
			assertEffect: func(t *testing.T, _ *subscriptionUpdater, _ SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, _ <-chan struct{}, _ time.Time) {
				require.Equal(t, []string{
					`{"data":{"value":"admitted","resolved":"value"}}`,
					"<heartbeat>",
				}, writer.Entries())
			},
		},
		{
			name: "Complete",
			invoke: func(updater *subscriptionUpdater, _ SubscriptionIdentifier) {
				updater.Complete()
			},
			assertAbsent: func(t *testing.T, _ *subscriptionUpdater, _ SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, _ <-chan struct{}) {
				require.Empty(t, writer.Entries(), "complete frame was written before admitted update completed")
			},
			assertEffect: func(t *testing.T, _ *subscriptionUpdater, _ SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, _ <-chan struct{}, _ time.Time) {
				require.Equal(t, []string{
					`{"data":{"value":"admitted","resolved":"value"}}`,
					"<complete>",
				}, writer.Entries())
			},
		},
		{
			name: "Error",
			invoke: func(updater *subscriptionUpdater, _ SubscriptionIdentifier) {
				updater.Error([]byte(terminalError))
			},
			assertAbsent: func(t *testing.T, _ *subscriptionUpdater, _ SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, _ <-chan struct{}) {
				require.Empty(t, writer.Entries(), "error frame was written before admitted update completed")
			},
			assertEffect: func(t *testing.T, _ *subscriptionUpdater, _ SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, _ <-chan struct{}, _ time.Time) {
				require.Equal(t, []string{
					`{"data":{"value":"admitted","resolved":"value"}}`,
					terminalError,
				}, writer.Entries())
			},
		},
		{
			name: "CloseSubscription",
			invoke: func(updater *subscriptionUpdater, id SubscriptionIdentifier) {
				updater.CloseSubscription(id)
			},
			assertAbsent: func(t *testing.T, updater *subscriptionUpdater, id SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, _ *chronologicalSubscriptionWriter, completed <-chan struct{}) {
				require.True(t, subscriptionUpdaterHasID(updater.Subscriptions(), id), "target subscription was removed before admitted update completed")
				select {
				case <-completed:
					t.Error("target subscription completed before admitted update completed")
				default:
				}
			},
			assertEffect: func(t *testing.T, updater *subscriptionUpdater, id SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, completed <-chan struct{}, deadline time.Time) {
				require.Equal(t, []string{`{"data":{"value":"admitted","resolved":"value"}}`}, writer.Entries())
				require.False(t, subscriptionUpdaterHasID(updater.Subscriptions(), id))
				select {
				case <-completed:
				case <-time.After(time.Until(deadline)):
					t.Fatal("timed out waiting for closed subscription completion")
				}
			},
		},
		{
			name: "Done",
			invoke: func(updater *subscriptionUpdater, _ SubscriptionIdentifier) {
				updater.Done()
			},
			assertAbsent: func(t *testing.T, updater *subscriptionUpdater, _ SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, _ *chronologicalSubscriptionWriter, completed <-chan struct{}) {
				_, registered := updater.resolver.getTrigger(updater.triggerID)
				require.True(t, registered, "trigger detached before admitted update completed")
				select {
				case <-completed:
					t.Error("subscription completed before admitted update completed")
				default:
				}
			},
			assertEffect: func(t *testing.T, updater *subscriptionUpdater, _ SubscriptionIdentifier, _ *concurrentSubscriptionDataSource, writer *chronologicalSubscriptionWriter, completed <-chan struct{}, deadline time.Time) {
				require.Equal(t, []string{`{"data":{"value":"admitted","resolved":"value"}}`}, writer.Entries())
				_, registered := updater.resolver.getTrigger(updater.triggerID)
				require.False(t, registered)
				select {
				case <-completed:
				case <-time.After(time.Until(deadline)):
					t.Fatal("timed out waiting for done subscription completion")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deadline := time.Now().Add(5 * time.Second)
			dataSource := newConcurrentSubscriptionDataSource(1)
			updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
			defer cleanup()
			updater.resolver.heartbeatInterval = 0

			writer := &chronologicalSubscriptionWriter{}
			var completed <-chan struct{}
			configureSubscriptionUpdaterState(t, updater, ids[0], func(state *subscriptionState) {
				state.writer = writer
				state.heartbeat = true
				completed = state.completed
			})
			_, updateDone := startSubscriptionUpdaterCall(func() {
				updater.UpdateSubscription(ids[0], []byte(admittedData))
			})
			var operationDone <-chan struct{}
			defer func() {
				dataSource.Release()
				waitForSubscriptionUpdaterCall(t, updateDone, deadline)
				if operationDone != nil {
					waitForSubscriptionUpdaterCall(t, operationDone, deadline)
				}
			}()

			select {
			case <-dataSource.AllStarted():
			case <-time.After(time.Until(deadline)):
				t.Fatal("timed out waiting for admitted subscription update")
			}

			operationStarted, done := startSubscriptionUpdaterCall(func() {
				test.invoke(updater, ids[0])
			})
			operationDone = done
			waitForSubscriptionUpdaterCallStart(t, operationStarted, deadline)
			waitForSubscriptionUpdaterCallToEnter(t, updater, operationDone, deadline)
			checkSubscriptionUpdaterCallBlocked(t, operationDone)
			test.assertAbsent(t, updater, ids[0], dataSource, writer, completed)

			dataSource.Release()
			waitForSubscriptionUpdaterCall(t, updateDone, deadline)
			waitForSubscriptionUpdaterCall(t, operationDone, deadline)
			test.assertEffect(t, updater, ids[0], dataSource, writer, completed, deadline)
		})
	}
}

func TestSubscriptionUpdater_Terminal_FirstTerminalWinsAndDoneRemainsAvailable(t *testing.T) {
	const terminalError = `{"errors":[{"message":"terminal"}]}`

	tests := []struct {
		name            string
		win             func(*subscriptionUpdater)
		winningTerminal string
	}{
		{
			name: "CompleteFirst",
			win: func(updater *subscriptionUpdater) {
				updater.Complete()
			},
			winningTerminal: "<complete>",
		},
		{
			name: "ErrorFirst",
			win: func(updater *subscriptionUpdater) {
				updater.Error([]byte(terminalError))
			},
			winningTerminal: terminalError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deadline := time.Now().Add(5 * time.Second)
			dataSource := newConcurrentSubscriptionDataSource(1)
			dataSource.Release()
			updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 2, dataSource)
			defer cleanup()
			updater.resolver.heartbeatInterval = 0

			writers := make([]*chronologicalSubscriptionWriter, len(ids))
			completed := make([]<-chan struct{}, len(ids))
			for i, id := range ids {
				writers[i] = &chronologicalSubscriptionWriter{}
				configureSubscriptionUpdaterState(t, updater, id, func(state *subscriptionState) {
					state.writer = writers[i]
					state.heartbeat = true
					completed[i] = state.completed
				})
			}

			test.win(updater)
			for _, writer := range writers {
				require.Equal(t, []string{test.winningTerminal}, writer.Entries())
			}

			fetchesBeforeSuppressedCalls := dataSource.Started()
			terminalCalls := []struct {
				name string
				call func()
			}{
				{name: "Error", call: func() { updater.Error([]byte(`{"errors":[{"message":"later"}]}`)) }},
				{name: "Complete", call: updater.Complete},
				{name: "Update", call: func() { updater.Update([]byte(`{"data":{"value":"broadcast"}}`)) }},
				{name: "UpdateSubscription", call: func() { updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"targeted"}}`)) }},
				{name: "Heartbeat", call: updater.Heartbeat},
			}
			for _, terminalCall := range terminalCalls {
				requireSubscriptionUpdaterCallShortCircuitsWithoutResolver(t, updater, terminalCall.name, terminalCall.call)
			}
			require.Equal(t, fetchesBeforeSuppressedCalls, dataSource.Started(), "terminal Update or UpdateSubscription reached the data source")
			for _, writer := range writers {
				require.Equal(t, []string{test.winningTerminal}, writer.Entries(), "later updater calls changed a terminal writer")
			}

			updater.CloseSubscription(ids[0])
			subscriptions := updater.Subscriptions()
			require.Len(t, subscriptions, 1)
			require.True(t, subscriptionUpdaterHasID(subscriptions, ids[1]))
			_, triggerRegistered := updater.resolver.getTrigger(updater.triggerID)
			require.True(t, triggerRegistered)
			select {
			case <-completed[0]:
			case <-time.After(time.Until(deadline)):
				t.Fatal("timed out waiting for terminal close completion")
			}
			select {
			case <-completed[1]:
				t.Fatal("closing one terminal subscription completed the remaining subscription")
			default:
			}

			updater.Done()
			_, triggerRegistered = updater.resolver.getTrigger(updater.triggerID)
			require.False(t, triggerRegistered)
			select {
			case <-completed[1]:
			case <-time.After(time.Until(deadline)):
				t.Fatal("timed out waiting for remaining terminal subscription completion")
			}

			updater.terminal = false
			fetchesBeforeDoneCalls := dataSource.Started()
			postDoneCalls := []struct {
				name string
				call func()
			}{
				{name: "Update", call: func() { updater.Update([]byte(`{"data":{"value":"after-done"}}`)) }},
				{name: "UpdateSubscription", call: func() { updater.UpdateSubscription(ids[1], []byte(`{"data":{"value":"after-done-targeted"}}`)) }},
				{name: "Heartbeat", call: updater.Heartbeat},
				{name: "Complete", call: updater.Complete},
				{name: "Error", call: func() { updater.Error([]byte(`{"errors":[{"message":"after done"}]}`)) }},
				{name: "CloseSubscription", call: func() { updater.CloseSubscription(ids[1]) }},
				{name: "Done", call: updater.Done},
			}
			for _, postDoneCall := range postDoneCalls {
				requireSubscriptionUpdaterCallShortCircuitsWithoutResolver(t, updater, postDoneCall.name, postDoneCall.call)
			}
			require.Equal(t, fetchesBeforeDoneCalls, dataSource.Started(), "post-Done Update or UpdateSubscription reached the data source")
			for _, writer := range writers {
				require.Equal(t, []string{test.winningTerminal}, writer.Entries(), "post-Done updater call changed a writer")
			}

			readOnlySnapshot := updater.Subscriptions()
			require.Empty(t, readOnlySnapshot)
			readOnlySnapshot[context.Background()] = SubscriptionIdentifier{ConnectionID: 99, SubscriptionID: 99}
			require.Empty(t, updater.Subscriptions(), "mutating the returned subscription snapshot changed updater state")
		})
	}
}

func TestSubscriptionUpdater_Terminal_RejectsLateSubscriber(t *testing.T) {
	const terminalError = `{"errors":[{"message":"terminal"}]}`
	tests := []struct {
		name            string
		expectedEntries []string
		detaches        bool
		deliver         func(*subscriptionUpdater)
	}{
		{
			name:            "Complete",
			expectedEntries: []string{"<complete>"},
			deliver: func(updater *subscriptionUpdater) {
				updater.Complete()
			},
		},
		{
			name:            "Error",
			expectedEntries: []string{terminalError},
			deliver: func(updater *subscriptionUpdater) {
				updater.Error([]byte(terminalError))
			},
		},
		{
			name:     "Done",
			detaches: true,
			deliver: func(updater *subscriptionUpdater) {
				updater.Done()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deadline := time.Now().Add(5 * time.Second)
			dataSource := newConcurrentSubscriptionDataSource(1)
			dataSource.Release()
			updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
			defer cleanup()

			reporter := &subscriptionUpdateCountingReporter{}
			reporter.SubscriptionCountInc(1)
			updater.resolver.reporter = reporter
			reporterBeforeAdd := reporter.snapshot()
			existingWriter := &chronologicalSubscriptionWriter{}
			var existingCompleted <-chan struct{}
			configureSubscriptionUpdaterState(t, updater, ids[0], func(state *subscriptionState) {
				state.writer = existingWriter
				existingCompleted = state.completed
			})

			updater.updateWG.Add(1)
			barrierReleased := false
			defer func() {
				if !barrierReleased {
					updater.updateWG.Done()
				}
			}()
			_, terminalDone := startSubscriptionUpdaterCall(func() {
				test.deliver(updater)
			})
			waitForSubscriptionUpdaterTerminating(t, updater, terminalDone, deadline)

			hookStarted := make(chan struct{}, 1)
			source := createFakeStream(nil, 0, nil, func(StartupHookContext, []byte) error {
				hookStarted <- struct{}{}
				return nil
			})
			lateID := SubscriptionIdentifier{ConnectionID: 99, SubscriptionID: 99}
			lateCompleted := make(chan struct{})
			addErr := updater.resolver.addSubscription(updater.triggerID, &addSubscription{
				ctx: NewContext(t.Context()),
				resolve: &GraphQLSubscription{
					Trigger:  GraphQLSubscriptionTrigger{Source: source},
					Response: &GraphQLResponse{},
				},
				writer:    &chronologicalSubscriptionWriter{},
				id:        lateID,
				completed: lateCompleted,
			})

			require.ErrorIs(t, addErr, ErrSubscriptionTriggerTerminating)
			require.EqualError(t, addErr, "subscription trigger is no longer accepting subscriptions")
			require.Equal(t, reporterBeforeAdd, reporter.snapshot(), "rejected subscriber changed reporter state")
			require.False(t, subscriptionUpdaterHasID(updater.Subscriptions(), lateID))
			updater.resolver.mu.Lock()
			_, indexedByID := updater.resolver.subscriptionsByID[lateID]
			_, indexedByConnection := updater.resolver.subscriptionsByConnection[lateID.ConnectionID]
			updater.resolver.mu.Unlock()
			require.False(t, indexedByID)
			require.False(t, indexedByConnection)
			select {
			case <-hookStarted:
				t.Fatal("startup hook launched for rejected subscriber")
			default:
			}
			select {
			case <-lateCompleted:
				t.Fatal("rejected subscriber was attached before lifecycle release")
			default:
			}

			updater.updateWG.Done()
			barrierReleased = true
			waitForSubscriptionUpdaterCall(t, terminalDone, deadline)
			select {
			case <-hookStarted:
				t.Fatal("startup hook launched for rejected subscriber")
			case <-time.After(50 * time.Millisecond):
			}

			require.Equal(t, test.expectedEntries, existingWriter.Entries())
			if !test.detaches {
				_, triggerRegistered := updater.resolver.getTrigger(updater.triggerID)
				require.True(t, triggerRegistered)
				select {
				case <-existingCompleted:
					t.Fatal("terminal delivery completed the existing subscriber before Done")
				default:
				}
				updater.Done()
			} else {
				_, triggerRegistered := updater.resolver.getTrigger(updater.triggerID)
				require.False(t, triggerRegistered)
			}
			select {
			case <-existingCompleted:
			case <-time.After(time.Until(deadline)):
				t.Fatal("timed out waiting for existing subscriber completion")
			}
			select {
			case <-lateCompleted:
				t.Fatal("rejected subscriber was attached and completed by lifecycle cleanup")
			default:
			}
			require.Zero(t, reporter.snapshot().subscriptions)
		})
	}
}

func TestSubscriptionUpdater_Terminal_IncludesSubscriberRegisteredFirst(t *testing.T) {
	const terminalError = `{"errors":[{"message":"terminal"}]}`
	tests := []struct {
		name            string
		expectedEntries []string
		detaches        bool
		deliver         func(*subscriptionUpdater)
	}{
		{
			name:            "Complete",
			expectedEntries: []string{"<complete>"},
			deliver: func(updater *subscriptionUpdater) {
				updater.Complete()
			},
		},
		{
			name:            "Error",
			expectedEntries: []string{terminalError},
			deliver: func(updater *subscriptionUpdater) {
				updater.Error([]byte(terminalError))
			},
		},
		{
			name:     "Done",
			detaches: true,
			deliver: func(updater *subscriptionUpdater) {
				updater.Done()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataSource := newConcurrentSubscriptionDataSource(1)
			dataSource.Release()
			updater, _, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
			defer cleanup()

			hookStarted := make(chan struct{})
			source := createFakeStream(nil, 0, nil, func(StartupHookContext, []byte) error {
				close(hookStarted)
				return nil
			})
			registeredID := SubscriptionIdentifier{ConnectionID: 99, SubscriptionID: 99}
			registeredWriter := &chronologicalSubscriptionWriter{}
			registeredCompleted := make(chan struct{})
			err := updater.resolver.addSubscription(updater.triggerID, &addSubscription{
				ctx: NewContext(t.Context()),
				resolve: &GraphQLSubscription{
					Trigger:  GraphQLSubscriptionTrigger{Source: source},
					Response: &GraphQLResponse{},
				},
				writer:    registeredWriter,
				id:        registeredID,
				completed: registeredCompleted,
			})
			require.NoError(t, err)
			select {
			case <-hookStarted:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for registered subscriber startup hook")
			}
			require.True(t, subscriptionUpdaterHasID(updater.Subscriptions(), registeredID))

			test.deliver(updater)

			require.Equal(t, test.expectedEntries, registeredWriter.Entries())
			if !test.detaches {
				select {
				case <-registeredCompleted:
					t.Fatal("terminal delivery completed the registered subscriber before Done")
				default:
				}
				updater.Done()
			}
			select {
			case <-registeredCompleted:
			case <-time.After(time.Second):
				t.Fatal("registered subscriber was not included in lifecycle cleanup")
			}
		})
	}
}

func TestSubscriptionUpdater_Lifecycle_CanceledTriggerContextGuardsCallbacks(t *testing.T) {
	deadline := time.Now().Add(5 * time.Second)
	dataSource := newConcurrentSubscriptionDataSource(1)
	dataSource.Release()
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 2, dataSource)
	defer cleanup()
	updater.resolver.heartbeatInterval = 0

	writers := make([]*chronologicalSubscriptionWriter, len(ids))
	completed := make([]<-chan struct{}, len(ids))
	for i, id := range ids {
		writers[i] = &chronologicalSubscriptionWriter{}
		configureSubscriptionUpdaterState(t, updater, id, func(state *subscriptionState) {
			state.writer = writers[i]
			state.heartbeat = true
			completed[i] = state.completed
		})
	}

	cancelSubscriptionUpdater(t, updater)
	canceledContextCalls := []struct {
		name string
		call func()
	}{
		{name: "Update", call: func() { updater.Update([]byte(`{"data":{"value":"canceled"}}`)) }},
		{name: "UpdateSubscription", call: func() { updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"canceled-targeted"}}`)) }},
		{name: "Heartbeat", call: updater.Heartbeat},
		{name: "Complete", call: updater.Complete},
		{name: "Error", call: func() { updater.Error([]byte(`{"errors":[{"message":"canceled"}]}`)) }},
		{name: "CloseSubscription", call: func() { updater.CloseSubscription(ids[0]) }},
	}
	for _, canceledContextCall := range canceledContextCalls {
		requireSubscriptionUpdaterCallShortCircuitsWithoutResolver(t, updater, canceledContextCall.name, canceledContextCall.call)
	}

	require.Zero(t, dataSource.Started())
	trig, triggerRegistered := updater.resolver.getTrigger(updater.triggerID)
	require.True(t, triggerRegistered)
	require.Len(t, trig.snapshotSubscriptions(), len(ids), "canceled-context callback changed registrations")
	for i, writer := range writers {
		require.Empty(t, writer.Entries())
		select {
		case <-completed[i]:
			t.Fatalf("canceled-context callback completed subscription %d", i)
		default:
		}
	}

	updater.Done()
	_, triggerRegistered = updater.resolver.getTrigger(updater.triggerID)
	require.False(t, triggerRegistered)
	for i, completion := range completed {
		select {
		case <-completion:
		case <-time.After(time.Until(deadline)):
			t.Fatalf("timed out waiting for Done to complete canceled-context subscription %d", i)
		}
	}
}

func TestSubscriptionUpdater_UpdateSubscription_ChronologicalWriterRecordsControlFrames(t *testing.T) {
	writer := &chronologicalSubscriptionWriter{}

	require.NoError(t, writer.Heartbeat())
	writer.Complete()
	writer.Error([]byte(`{"errors":[{"message":"terminal"}]}`))

	require.Equal(t, []string{
		"<heartbeat>",
		"<complete>",
		`{"errors":[{"message":"terminal"}]}`,
	}, writer.Entries())
}

func TestSubscriptionUpdater_UpdateSubscription_CancellationReleasesQueuedSameSubscriberUpdate(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	var callsDone []<-chan struct{}
	defer func() {
		dataSource.Release()
		for _, done := range callsDone {
			waitForSubscriptionUpdateCall(t, done)
		}
	}()

	firstDone := make(chan struct{})
	callsDone = append(callsDone, firstDone)
	go func() {
		defer close(firstDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"first"}}`))
	}()

	select {
	case <-dataSource.AllStarted():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first subscription fetch")
	}
	firstTail := subscriptionUpdateTail(updater, ids[0])
	require.NotNil(t, firstTail)

	secondDone := make(chan struct{})
	callsDone = append(callsDone, secondDone)
	go func() {
		defer close(secondDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"second"}}`))
	}()
	waitForSubscriptionUpdateTailChange(t, updater, ids[0], firstTail)

	cancelSubscriptionUpdater(t, updater)
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("queued subscription update did not return after cancellation")
	}
	require.Equal(t, 1, dataSource.Started(), "queued update entered its fetch after cancellation")

	dataSource.Release()
	waitForSubscriptionUpdateCall(t, firstDone)
	waitForSubscriptionUpdates(t, updater)
	requireNoSubscriptionUpdateTails(t, updater)
}

func TestSubscriptionUpdater_UpdateSubscription_PropagatesPanicAndCleansTail(t *testing.T) {
	panicValue := &subscriptionUpdatePanic{label: "subscription fetch panic"}
	dataSource := &panickingSubscriptionDataSource{panicValue: panicValue}
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	recovered := make(chan any, 1)
	go func() {
		defer func() {
			recovered <- recover()
		}()
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"event"}}`))
	}()

	select {
	case actual := <-recovered:
		require.Same(t, panicValue, actual)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription update panic")
	}
	waitForSubscriptionUpdates(t, updater)
	requireNoSubscriptionUpdateTails(t, updater)
}

func TestSubscriptionUpdater_UpdateSubscription_FlushErrorUnsubscribesWithoutDeadlock(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	dataSource.Release()
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)

	flushErr := errors.New("subscription flush failed")
	writer := &panickingSubscriptionWriter{flushErr: flushErr}
	var completed <-chan struct{}
	configureSubscriptionUpdaterState(t, updater, ids[0], func(state *subscriptionState) {
		state.writer = writer
		completed = state.completed
	})

	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"event"}}`))
	}()
	waitForSubscriptionUpdateCall(t, updateDone)
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("flush failure did not complete the removed subscription")
	}

	_, triggerRegistered := updater.resolver.getTrigger(updater.triggerID)
	require.False(t, triggerRegistered)
	updater.resolver.mu.Lock()
	_, indexedByID := updater.resolver.subscriptionsByID[ids[0]]
	_, indexedByConnection := updater.resolver.subscriptionsByConnection[ids[0].ConnectionID]
	updater.resolver.mu.Unlock()
	require.False(t, indexedByID)
	require.False(t, indexedByConnection)
	waitForSubscriptionUpdates(t, updater)
	requireNoSubscriptionUpdateTails(t, updater)
	require.Empty(t, writer.Entries())

	cleanup()
}

func TestSubscriptionUpdater_UpdateSubscription_OrdersFilterErrorAfterPrecedingUpdate(t *testing.T) {
	filterErr := errors.New("subscription filter render failed")
	dataSource := newConcurrentSubscriptionDataSource(1)
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	writer := &chronologicalSubscriptionWriter{}
	renderer := &sequencedSubscriptionFilterRenderer{secondErr: filterErr}
	configureSubscriptionUpdaterState(t, updater, ids[0], func(state *subscriptionState) {
		renderer.expectedContext = state.ctx.Context()
		state.ctx.Variables = astjson.MustParseBytes([]byte(`{"filterValue":"match"}`))
		state.writer = writer
		state.resolve.Filter = &SubscriptionFilter{
			In: &SubscriptionFieldFilter{
				FieldPath: []string{"filter"},
				Values: []InputTemplate{{
					Segments: []TemplateSegment{{
						SegmentType:        VariableSegmentType,
						VariableKind:       ResolvableObjectVariableKind,
						VariableSourcePath: []string{"filterValue"},
						Renderer:           renderer,
					}},
				}},
			},
		}
	})
	updater.resolver.SetAsyncErrorWriter(&chronologicalSubscriptionErrorWriter{})

	var callsDone []<-chan struct{}
	defer func() {
		dataSource.Release()
		for _, done := range callsDone {
			waitForSubscriptionUpdateCall(t, done)
		}
	}()

	firstDone := make(chan struct{})
	callsDone = append(callsDone, firstDone)
	go func() {
		defer close(firstDone)
		updater.UpdateSubscription(ids[0], []byte(`{"filter":"match","data":{"value":"first"}}`))
	}()

	select {
	case <-dataSource.AllStarted():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first subscription fetch")
	}
	firstTail := subscriptionUpdateTail(updater, ids[0])
	require.NotNil(t, firstTail)

	secondDone := make(chan struct{})
	callsDone = append(callsDone, secondDone)
	go func() {
		defer close(secondDone)
		updater.UpdateSubscription(ids[0], []byte(`{"filter":"match","data":{"value":"second"}}`))
	}()
	waitForSubscriptionUpdateTailChange(t, updater, ids[0], firstTail)
	require.Empty(t, writer.Entries(), "queued filter error was written before the preceding update completed")

	dataSource.Release()
	waitForSubscriptionUpdateCall(t, firstDone)
	waitForSubscriptionUpdateCall(t, secondDone)
	require.Equal(t, []string{
		`{"data":{"value":"first","resolved":"value"}}`,
		`{"errors":[{"message":"subscription filter render failed"}]}`,
	}, writer.Entries())
	calls, sawUnexpectedContext, sawNonNilEventData := renderer.results()
	require.Equal(t, 2, calls)
	require.False(t, sawUnexpectedContext)
	require.False(t, sawNonNilEventData)
}

func TestSubscriptionUpdater_UpdateSubscription_CleansTailAfterSingleUpdate(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	done := make(chan struct{})
	defer func() {
		dataSource.Release()
		waitForSubscriptionUpdateCall(t, done)
	}()
	go func() {
		defer close(done)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"single"}}`))
	}()

	select {
	case <-dataSource.AllStarted():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription fetch")
	}
	require.NotNil(t, subscriptionUpdateTail(updater, ids[0]))

	dataSource.Release()
	waitForSubscriptionUpdateCall(t, done)
	requireNoSubscriptionUpdateTails(t, updater)
}

func TestSubscriptionUpdater_UpdateSubscription_CleansTailAfterOverlappingSameSubscriberUpdates(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	updater, ids, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	var callsDone []<-chan struct{}
	defer func() {
		dataSource.Release()
		for _, done := range callsDone {
			waitForSubscriptionUpdateCall(t, done)
		}
	}()

	firstDone := make(chan struct{})
	callsDone = append(callsDone, firstDone)
	go func() {
		defer close(firstDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"first"}}`))
	}()

	select {
	case <-dataSource.AllStarted():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first subscription fetch")
	}
	firstTail := subscriptionUpdateTail(updater, ids[0])
	require.NotNil(t, firstTail)

	secondDone := make(chan struct{})
	callsDone = append(callsDone, secondDone)
	go func() {
		defer close(secondDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"second"}}`))
	}()
	waitForSubscriptionUpdateTailChange(t, updater, ids[0], firstTail)

	dataSource.Release()
	waitForSubscriptionUpdateCall(t, firstDone)
	waitForSubscriptionUpdateCall(t, secondDone)
	requireNoSubscriptionUpdateTails(t, updater)
}

func TestSubscriptionUpdater_UpdateSubscription_PreservesSameSubscriberFIFO(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	updater, ids, recorders, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	var callsDone []<-chan struct{}
	defer func() {
		dataSource.Release()
		for _, done := range callsDone {
			waitForSubscriptionUpdateCall(t, done)
		}
	}()

	firstDone := make(chan struct{})
	callsDone = append(callsDone, firstDone)
	go func() {
		defer close(firstDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"first"}}`))
	}()

	select {
	case started := <-dataSource.StartedEvents():
		require.Equal(t, 1, started)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first subscription fetch")
	}
	firstTail := subscriptionUpdateTail(updater, ids[0])
	require.NotNil(t, firstTail)

	secondDone := make(chan struct{})
	callsDone = append(callsDone, secondDone)
	go func() {
		defer close(secondDone)
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"second"}}`))
	}()

	waitForSubscriptionUpdateTailChange(t, updater, ids[0], firstTail)
	select {
	case started := <-dataSource.StartedEvents():
		t.Fatalf("fetch %d began before the preceding update was released", started)
	case <-time.After(250 * time.Millisecond):
	}

	dataSource.Release()
	select {
	case started := <-dataSource.StartedEvents():
		require.Equal(t, 2, started)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second subscription fetch after releasing the first")
	}
	waitForSubscriptionUpdateCall(t, firstDone)
	waitForSubscriptionUpdateCall(t, secondDone)

	require.Equal(t, []string{
		`{"data":{"value":"first","resolved":"value"}}`,
		`{"data":{"value":"second","resolved":"value"}}`,
	}, recorders[0].Messages())
}

func TestSubscriptionUpdater_UpdateSubscription_UnknownSubscriberIsNoOp(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	updater, _, _, cleanup := newSubscriptionUpdaterHarness(t, 1, dataSource)
	defer cleanup()

	unknownID := SubscriptionIdentifier{ConnectionID: 99, SubscriptionID: 99}
	updater.UpdateSubscription(unknownID, []byte(`{"data":{"value":"ignored"}}`))

	require.Zero(t, dataSource.Started())
	requireNoSubscriptionUpdateTails(t, updater)
}

func TestSubscriptionUpdater_UpdateSubscription_ResolvesDistinctSubscribersConcurrently(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(2)
	updater, ids, recorders, cleanup := newSubscriptionUpdaterHarness(t, 2, dataSource)
	defer cleanup()

	var wg sync.WaitGroup
	wg.Add(len(ids))
	for _, id := range ids {
		go func(id SubscriptionIdentifier) {
			defer wg.Done()
			updater.UpdateSubscription(id, []byte(`{"data":{"value":"event"}}`))
		}(id)
	}
	callsDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(callsDone)
	}()
	var callsCleanupOnce sync.Once
	callsJoined := false
	cleanupCalls := func() {
		callsCleanupOnce.Do(func() {
			dataSource.Release()
			select {
			case <-callsDone:
				callsJoined = true
			case <-time.After(time.Second):
				t.Error("timed out waiting for subscription updates to finish")
			}
		})
	}
	defer cleanupCalls()

	startedConcurrently := true
	select {
	case <-dataSource.AllStarted():
	case <-time.After(time.Second):
		startedConcurrently = false
	}

	cleanupCalls()
	if !callsJoined {
		return
	}

	for _, recorder := range recorders {
		require.Equal(t, []string{`{"data":{"value":"event","resolved":"value"}}`}, recorder.Messages())
	}

	if !startedConcurrently {
		t.Fatal("distinct subscribers did not enter their fetches concurrently")
	}
}

func TestSubscriptionUpdater_UpdateSubscription_DeduplicatesConcurrentSubscriberFetches(t *testing.T) {
	dataSource := newConcurrentSubscriptionDataSource(1)
	updater, ids, recorders, cleanup := newSubscriptionUpdaterHarness(t, 2, dataSource)
	defer cleanup()

	waitObserved := make([]chan struct{}, len(ids))
	for i, id := range ids {
		waitObserved[i] = make(chan struct{})
		configureSubscriptionUpdaterState(t, updater, id, func(state *subscriptionState) {
			state.ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = false
			state.ctx.LoaderHooks = &subscriptionSingleFlightLoaderHook{waitObserved: waitObserved[i]}
		})
	}

	updateDone := make([]chan struct{}, len(ids))
	for i := range updateDone {
		updateDone[i] = make(chan struct{})
	}
	defer func() {
		dataSource.Release()
		for _, done := range updateDone {
			waitForSubscriptionUpdateCall(t, done)
		}
	}()

	go func() {
		defer close(updateDone[0])
		updater.UpdateSubscription(ids[0], []byte(`{"data":{"value":"first"}}`))
	}()
	waitForSubscriptionUpdateTailChange(t, updater, ids[0], nil)
	select {
	case <-dataSource.AllStarted():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for single-flight leader fetch")
	}

	go func() {
		defer close(updateDone[1])
		updater.UpdateSubscription(ids[1], []byte(`{"data":{"value":"second"}}`))
	}()
	waitForSubscriptionUpdateTailChange(t, updater, ids[1], nil)
	select {
	case <-waitObserved[1]:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second subscriber to join the single-flight fetch")
	}

	require.Equal(t, 1, dataSource.Started())
	dataSource.Release()
	for _, done := range updateDone {
		waitForSubscriptionUpdateCall(t, done)
	}
	require.Equal(t, 1, dataSource.Started())
	require.Equal(t, []string{`{"data":{"value":"first","resolved":"value"}}`}, recorders[0].Messages())
	require.Equal(t, []string{`{"data":{"value":"second","resolved":"value"}}`}, recorders[1].Messages())
}
