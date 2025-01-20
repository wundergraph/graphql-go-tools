//go:generate mockgen -self_package=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource

package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/xcontext"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	DefaultHeartbeatInterval = 5 * time.Second
)

var (
	multipartHeartbeat = []byte("{}")
)

type Reporter interface {
	// SubscriptionUpdateSent called when a new subscription update is sent
	SubscriptionUpdateSent()
	// SubscriptionCountInc increased when a new subscription is added to a trigger, this includes inflight subscriptions
	SubscriptionCountInc(count int)
	// SubscriptionCountDec decreased when a subscription is removed from a trigger e.g. on shutdown
	SubscriptionCountDec(count int)
	// TriggerCountInc increased when a new trigger is added e.g. when a trigger is started and initialized
	TriggerCountInc(count int)
	// TriggerCountDec decreased when a trigger is removed e.g. when a trigger is shutdown
	TriggerCountDec(count int)
}

type AsyncErrorWriter interface {
	WriteError(ctx *Context, err error, res *GraphQLResponse, w io.Writer)
}

type Resolver struct {
	ctx            context.Context
	options        ResolverOptions
	maxConcurrency chan struct{}

	triggers               map[uint64]*trigger
	heartbeatSubLock       *sync.Mutex
	heartbeatSubscriptions map[*Context]*sub
	events                 chan subscriptionEvent
	triggerEventsSem       *semaphore.Weighted
	triggerUpdatesSem      *semaphore.Weighted
	triggerUpdateBuf       *bytes.Buffer

	allowedErrorExtensionFields map[string]struct{}
	allowedErrorFields          map[string]struct{}

	connectionIDs atomic.Int64

	reporter         Reporter
	asyncErrorWriter AsyncErrorWriter

	propagateSubgraphErrors       bool
	propagateSubgraphStatusCodes  bool
	multipartSubHeartbeatInterval time.Duration
}

func (r *Resolver) SetAsyncErrorWriter(w AsyncErrorWriter) {
	r.asyncErrorWriter = w
}

type tools struct {
	resolvable *Resolvable
	loader     *Loader
}

type SubgraphErrorPropagationMode int

const (
	// SubgraphErrorPropagationModeWrapped collects all errors and exposes them as a list of errors on the extensions field "errors" of the gateway error.
	SubgraphErrorPropagationModeWrapped SubgraphErrorPropagationMode = iota
	// SubgraphErrorPropagationModePassThrough propagates all errors as root errors as they are.
	SubgraphErrorPropagationModePassThrough
)

type ResolverOptions struct {
	// MaxConcurrency limits the number of concurrent tool calls which is used to resolve operations.
	// The limit is only applied to getToolsWithLimit() calls. Intentionally, we don't use this limit for
	// subscription updates to prevent blocking the subscription during a network collapse because a one-to-one
	// relation is not given as in the case of single http request. We already enforce concurrency limits through
	// the MaxSubscriptionWorkers option that is a semaphore to limit the number of concurrent subscription updates.
	//
	// If set to 0, no limit is applied
	// It is advised to set this to a reasonable value to prevent excessive memory usage
	// Each concurrent resolve operation allocates ~50kb of memory
	// In addition, there's a limit of how many concurrent requests can be efficiently resolved
	// This depends on the number of CPU cores available, the complexity of the operations, and the origin services
	MaxConcurrency int

	// MaxSubscriptionWorkers limits the concurrency on how many subscription can be added / removed concurrently.
	// This does not include subscription updates, for that we have a separate semaphore MaxSubscriptionUpdates.
	MaxSubscriptionWorkers int

	// MaxSubscriptionUpdates limits the number of concurrent subscription updates that can be sent to the event loop.
	MaxSubscriptionUpdates int

	Debug bool

	Reporter         Reporter
	AsyncErrorWriter AsyncErrorWriter

	// PropagateSubgraphErrors adds Subgraph Errors to the response
	PropagateSubgraphErrors bool
	// PropagateSubgraphStatusCodes adds the status code of the Subgraph to the extensions field of a Subgraph Error
	PropagateSubgraphStatusCodes bool
	// SubgraphErrorPropagationMode defines how Subgraph Errors are propagated
	// SubgraphErrorPropagationModeWrapped wraps Subgraph Errors in a Subgraph Error to prevent leaking internal information
	// SubgraphErrorPropagationModePassThrough passes Subgraph Errors through without modification
	SubgraphErrorPropagationMode SubgraphErrorPropagationMode
	// RewriteSubgraphErrorPaths rewrites the paths of Subgraph Errors to match the path of the field from the perspective of the client
	// This means that nested entity requests will have their paths rewritten from e.g. "_entities.foo.bar" to "person.foo.bar" if the root field above is "person"
	RewriteSubgraphErrorPaths bool
	// OmitSubgraphErrorLocations omits the locations field of Subgraph Errors
	OmitSubgraphErrorLocations bool
	// OmitSubgraphErrorExtensions omits the extensions field of Subgraph Errors
	OmitSubgraphErrorExtensions bool
	// AllowedErrorExtensionFields defines which fields are allowed in the extensions field of a root subgraph error
	AllowedErrorExtensionFields []string
	// AttachServiceNameToErrorExtensions attaches the service name to the extensions field of a root subgraph error
	AttachServiceNameToErrorExtensions bool
	// DefaultErrorExtensionCode is the default error code to use for subgraph errors if no code is provided
	DefaultErrorExtensionCode string
	// MaxRecyclableParserSize limits the size of the Parser that can be recycled back into the Pool.
	// If set to 0, no limit is applied
	// This helps keep the Heap size more maintainable if you regularly perform large queries.
	MaxRecyclableParserSize int
	// ResolvableOptions are configuration options for the Resolvable struct
	ResolvableOptions ResolvableOptions
	// AllowedCustomSubgraphErrorFields defines which fields are allowed in the subgraph error when in passthrough mode
	AllowedSubgraphErrorFields []string
	// MultipartSubHeartbeatInterval defines the interval in which a heartbeat is sent to all multipart subscriptions
	MultipartSubHeartbeatInterval time.Duration
}

// New returns a new Resolver, ctx.Done() is used to cancel all active subscriptions & streams
func New(ctx context.Context, options ResolverOptions) *Resolver {
	// options.Debug = true
	if options.MaxConcurrency <= 0 {
		options.MaxConcurrency = 32
	}

	if options.MultipartSubHeartbeatInterval <= 0 {
		options.MultipartSubHeartbeatInterval = DefaultHeartbeatInterval
	}

	// We transform the allowed fields into a map for faster lookups
	allowedExtensionFields := make(map[string]struct{}, len(options.AllowedErrorExtensionFields))
	for _, field := range options.AllowedErrorExtensionFields {
		allowedExtensionFields[field] = struct{}{}
	}

	// always allow "message" and "path"
	allowedErrorFields := map[string]struct{}{
		"message": {},
		"path":    {},
	}

	if !options.OmitSubgraphErrorExtensions {
		allowedErrorFields["extensions"] = struct{}{}
	}

	if !options.OmitSubgraphErrorLocations {
		allowedErrorFields["locations"] = struct{}{}
	}

	for _, field := range options.AllowedSubgraphErrorFields {
		allowedErrorFields[field] = struct{}{}
	}

	resolver := &Resolver{
		ctx:                           ctx,
		options:                       options,
		propagateSubgraphErrors:       options.PropagateSubgraphErrors,
		propagateSubgraphStatusCodes:  options.PropagateSubgraphStatusCodes,
		events:                        make(chan subscriptionEvent),
		triggers:                      make(map[uint64]*trigger),
		heartbeatSubLock:              &sync.Mutex{},
		heartbeatSubscriptions:        make(map[*Context]*sub),
		reporter:                      options.Reporter,
		asyncErrorWriter:              options.AsyncErrorWriter,
		triggerUpdateBuf:              bytes.NewBuffer(make([]byte, 0, 1024)),
		allowedErrorExtensionFields:   allowedExtensionFields,
		allowedErrorFields:            allowedErrorFields,
		multipartSubHeartbeatInterval: options.MultipartSubHeartbeatInterval,
	}
	resolver.maxConcurrency = make(chan struct{}, options.MaxConcurrency)
	for i := 0; i < options.MaxConcurrency; i++ {
		resolver.maxConcurrency <- struct{}{}
	}
	if options.MaxSubscriptionWorkers == 0 {
		options.MaxSubscriptionWorkers = 1024
	}
	if options.MaxSubscriptionUpdates == 0 {
		options.MaxSubscriptionUpdates = 1024
	}

	resolver.triggerEventsSem = semaphore.NewWeighted(int64(options.MaxSubscriptionWorkers))
	resolver.triggerUpdatesSem = semaphore.NewWeighted(int64(options.MaxSubscriptionUpdates))

	go resolver.handleEvents()

	return resolver
}

func newTools(options ResolverOptions, allowedExtensionFields map[string]struct{}, allowedErrorFields map[string]struct{}) *tools {
	return &tools{
		resolvable: NewResolvable(options.ResolvableOptions),
		loader: &Loader{
			propagateSubgraphErrors:           options.PropagateSubgraphErrors,
			propagateSubgraphStatusCodes:      options.PropagateSubgraphStatusCodes,
			subgraphErrorPropagationMode:      options.SubgraphErrorPropagationMode,
			rewriteSubgraphErrorPaths:         options.RewriteSubgraphErrorPaths,
			omitSubgraphErrorLocations:        options.OmitSubgraphErrorLocations,
			omitSubgraphErrorExtensions:       options.OmitSubgraphErrorExtensions,
			allowedErrorExtensionFields:       allowedExtensionFields,
			attachServiceNameToErrorExtension: options.AttachServiceNameToErrorExtensions,
			defaultErrorExtensionCode:         options.DefaultErrorExtensionCode,
			allowedSubgraphErrorFields:        allowedErrorFields,
		},
	}
}

type GraphQLResolveInfo struct {
	ResolveAcquireWaitTime time.Duration
}

func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (*GraphQLResolveInfo, error) {

	resp := &GraphQLResolveInfo{}

	start := time.Now()
	<-r.maxConcurrency
	resp.ResolveAcquireWaitTime = time.Since(start)
	defer func() {
		r.maxConcurrency <- struct{}{}
	}()

	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields)

	err := t.resolvable.Init(ctx, data, response.Info.OperationType)
	if err != nil {
		return nil, err
	}

	if !ctx.ExecutionOptions.SkipLoader {
		err = t.loader.LoadGraphQLResponseData(ctx, response, t.resolvable)
		if err != nil {
			return nil, err
		}
	}

	buf := &bytes.Buffer{}
	err = t.resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, buf)
	if err != nil {
		return nil, err
	}

	_, err = buf.WriteTo(writer)
	return resp, err
}

type trigger struct {
	id            uint64
	cancel        context.CancelFunc
	subscriptions map[*Context]*sub
	inFlight      *sync.WaitGroup
	initialized   bool
}

type sub struct {
	mux       sync.Mutex
	resolve   *GraphQLSubscription
	writer    SubscriptionResponseWriter
	id        SubscriptionIdentifier
	completed chan struct{}
	lastWrite time.Time
	// executor is an optional argument that allows us to "schedule" the execution of an update on another thread
	// e.g. if we're using SSE/Multipart Fetch, we can run the execution on the goroutine of the http request
	// this ensures that ctx cancellation works properly when a client disconnects
	executor chan func()
}

func (r *Resolver) executeSubscriptionUpdate(ctx *Context, sub *sub, sharedInput []byte) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:update:%d\n", sub.id.SubscriptionID)
	}
	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields)

	input := make([]byte, len(sharedInput))
	copy(input, sharedInput)

	if err := t.resolvable.InitSubscription(ctx, input, sub.resolve.Trigger.PostProcessing); err != nil {
		sub.mux.Lock()
		r.asyncErrorWriter.WriteError(ctx, err, sub.resolve.Response, sub.writer)
		sub.mux.Unlock()
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:init:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}

	if err := t.loader.LoadGraphQLResponseData(ctx, sub.resolve.Response, t.resolvable); err != nil {
		sub.mux.Lock()
		r.asyncErrorWriter.WriteError(ctx, err, sub.resolve.Response, sub.writer)
		sub.mux.Unlock()
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:load:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}

	sub.mux.Lock()
	defer func() {
		sub.lastWrite = time.Now()
		sub.mux.Unlock()
	}()

	if err := t.resolvable.Resolve(ctx.ctx, sub.resolve.Response.Data, sub.resolve.Response.Fetches, sub.writer); err != nil {
		r.asyncErrorWriter.WriteError(ctx, err, sub.resolve.Response, sub.writer)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:resolve:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}
	err := sub.writer.Flush()
	if err != nil {
		// client disconnected
		_ = r.AsyncUnsubscribeSubscription(sub.id)
		return
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:flushed:%d\n", sub.id.SubscriptionID)
	}
	if r.reporter != nil {
		r.reporter.SubscriptionUpdateSent()
	}
	if t.resolvable.WroteErrorsWithoutData() {
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:completing:errors_without_data:%d\n", sub.id.SubscriptionID)
		}
	}
}

func (r *Resolver) handleEvents() {
	done := r.ctx.Done()
	heartbeat := time.NewTicker(r.multipartSubHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-done:
			r.handleShutdown()
			return
		case event := <-r.events:
			r.handleEvent(event)
		case <-heartbeat.C:
			r.handleHeartbeat(multipartHeartbeat)
		}
	}
}

// handleEvent is a single threaded function that processes events from the events channel
// All events are processed in the order they are received and need to be processed quickly
// to prevent blocking the event loop and any other events from being processed.
// TODO: consider using a worker pool that distributes events from different triggers to different workers
func (r *Resolver) handleEvent(event subscriptionEvent) {
	switch event.kind {
	case subscriptionEventKindAddSubscription:
		r.handleAddSubscription(event.triggerID, event.addSubscription)
	case subscriptionEventKindRemoveSubscription:
		r.handleRemoveSubscription(event.id)
	case subscriptionEventKindRemoveClient:
		r.handleRemoveClient(event.id.ConnectionID)
	case subscriptionEventKindTriggerUpdate:
		r.handleTriggerUpdate(event.triggerID, event.data)
	case subscriptionEventKindTriggerDone:
		r.handleTriggerDone(event.triggerID)
	case subscriptionEventKindTriggerInitialized:
		r.handleTriggerInitialized(event.triggerID)
	case subscriptionEventKindTriggerShutdown:
		r.handleTriggerShutdown(event)
	case subscriptionEventKindUnknown:
		panic("unknown event")
	}
}

func (r *Resolver) handleHeartbeat(data []byte) {
	r.heartbeatSubLock.Lock()
	defer r.heartbeatSubLock.Unlock()

	if r.options.Debug {
		fmt.Printf("resolver:heartbeat:%d\n", len(r.heartbeatSubscriptions))
	}
	now := time.Now()
	for c, s := range r.heartbeatSubscriptions {
		// check if the last write to the subscription was more than heartbeat interval ago
		c, s := c, s
		s.mux.Lock()
		skipHeartbeat := now.Sub(s.lastWrite) < r.multipartSubHeartbeatInterval
		s.mux.Unlock()
		if skipHeartbeat || (c.Context().Err() != nil && errors.Is(c.Context().Err(), context.Canceled)) {
			continue
		}

		go func() {
			if r.options.Debug {
				fmt.Printf("resolver:heartbeat:subscription:%d\n", s.id.SubscriptionID)
			}

			s.mux.Lock()
			if _, err := s.writer.Write(data); err != nil {
				if errors.Is(err, context.Canceled) {
					// client disconnected
					s.mux.Unlock()
					_ = r.AsyncUnsubscribeSubscription(s.id)
					return
				}
				r.asyncErrorWriter.WriteError(c, err, nil, s.writer)
			}
			err := s.writer.Flush()
			s.mux.Unlock()
			if err != nil {
				// client disconnected
				_ = r.AsyncUnsubscribeSubscription(s.id)
				return
			}
			if r.options.Debug {
				fmt.Printf("resolver:heartbeat:subscription:flushed:%d\n", s.id.SubscriptionID)
			}
			if r.reporter != nil {
				r.reporter.SubscriptionUpdateSent()
			}
		}()
	}
}

func (r *Resolver) handleTriggerShutdown(s subscriptionEvent) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:%d:%d\n", s.triggerID, s.id.SubscriptionID)
	}

	r.shutdownTrigger(s.triggerID)
}

func (r *Resolver) handleTriggerInitialized(triggerID uint64) {
	trig, ok := r.triggers[triggerID]
	if !ok {
		return
	}
	trig.initialized = true

	if r.reporter != nil {
		r.reporter.TriggerCountInc(1)
	}
}

func (r *Resolver) handleTriggerDone(triggerID uint64) {
	r.shutdownTrigger(triggerID)
}

func (r *Resolver) handleAddSubscription(triggerID uint64, add *addSubscription) {
	var (
		err error
	)
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:add:%d:%d\n", triggerID, add.id.SubscriptionID)
	}
	s := &sub{
		resolve:   add.resolve,
		writer:    add.writer,
		id:        add.id,
		completed: add.completed,
		lastWrite: time.Now(),
		executor:  add.executor,
	}
	if add.ctx.ExecutionOptions.SendHeartbeat {
		r.heartbeatSubLock.Lock()
		r.heartbeatSubscriptions[add.ctx] = s
		r.heartbeatSubLock.Unlock()
	}
	trig, ok := r.triggers[triggerID]
	if ok {
		trig.subscriptions[add.ctx] = s
		if r.reporter != nil {
			r.reporter.SubscriptionCountInc(1)
		}
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:added:%d:%d\n", triggerID, add.id.SubscriptionID)
		}
		return
	}

	if r.options.Debug {
		fmt.Printf("resolver:create:trigger:%d\n", triggerID)
	}
	ctx, cancel := context.WithCancel(xcontext.Detach(add.ctx.Context()))
	updater := &subscriptionUpdater{
		debug:     r.options.Debug,
		triggerID: triggerID,
		ch:        r.events,
		ctx:       ctx,
		updateSem: r.triggerUpdatesSem,
	}
	cloneCtx := add.ctx.clone(ctx)
	trig = &trigger{
		id:            triggerID,
		subscriptions: make(map[*Context]*sub),
		cancel:        cancel,
	}
	r.triggers[triggerID] = trig
	trig.subscriptions[add.ctx] = s

	if r.reporter != nil {
		r.reporter.SubscriptionCountInc(1)
	}

	var asyncDataSource AsyncSubscriptionDataSource

	if async, ok := add.resolve.Trigger.Source.(AsyncSubscriptionDataSource); ok {
		trig.cancel = func() {
			cancel()
			async.AsyncStop(triggerID)
		}
		asyncDataSource = async
	}

	go func() {
		if r.options.Debug {
			fmt.Printf("resolver:trigger:start:%d\n", triggerID)
		}
		if asyncDataSource != nil {
			err = asyncDataSource.AsyncStart(cloneCtx, triggerID, add.input, updater)
		} else {
			err = add.resolve.Trigger.Source.Start(cloneCtx, add.input, updater)
		}
		if err != nil {
			if r.options.Debug {
				fmt.Printf("resolver:trigger:failed:%d\n", triggerID)
			}
			r.asyncErrorWriter.WriteError(add.ctx, err, add.resolve.Response, add.writer)
			_ = r.emitTriggerShutdown(triggerID)
			return
		}

		_ = r.emitTriggerInitialized(triggerID)

		if r.options.Debug {
			fmt.Printf("resolver:trigger:started:%d\n", triggerID)
		}
	}()

}

func (r *Resolver) emitTriggerShutdown(triggerID uint64) error {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:%d\n", triggerID)
	}

	if err := r.triggerEventsSem.Acquire(r.ctx, 1); err != nil {
		return err
	}
	defer r.triggerEventsSem.Release(1)

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		triggerID: triggerID,
		kind:      subscriptionEventKindTriggerShutdown,
	}:
	}

	return nil
}

func (r *Resolver) emitTriggerInitialized(triggerID uint64) error {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:initialized:%d\n", triggerID)
	}

	if err := r.triggerEventsSem.Acquire(r.ctx, 1); err != nil {
		return err
	}
	defer r.triggerEventsSem.Release(1)

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		triggerID: triggerID,
		kind:      subscriptionEventKindTriggerInitialized,
	}:
	}

	return nil
}

func (r *Resolver) handleRemoveSubscription(id SubscriptionIdentifier) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:%d:%d\n", id.ConnectionID, id.SubscriptionID)
	}
	removed := 0
	for u := range r.triggers {
		trig := r.triggers[u]
		removed += r.shutdownTriggerSubscriptions(u, func(sID SubscriptionIdentifier) bool {
			return sID == id
		})
		if len(trig.subscriptions) == 0 {
			r.shutdownTrigger(trig.id)
		}
	}
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
	}
}

func (r *Resolver) handleRemoveClient(id int64) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:client:%d\n", id)
	}
	removed := 0
	for u := range r.triggers {
		removed += r.shutdownTriggerSubscriptions(u, func(sID SubscriptionIdentifier) bool {
			return sID.ConnectionID == id && !sID.internal
		})
		if len(r.triggers[u].subscriptions) == 0 {
			r.shutdownTrigger(r.triggers[u].id)
		}
	}
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
	}
}

func (r *Resolver) handleTriggerUpdate(id uint64, data []byte) {
	trig, ok := r.triggers[id]
	if !ok {
		return
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:update:%d\n", id)
	}
	wg := &sync.WaitGroup{}
	trig.inFlight = wg
	for c, s := range trig.subscriptions {
		c, s := c, s
		if err := c.ctx.Err(); err != nil {
			continue // no need to schedule an event update when the client already disconnected
		}
		skip, err := s.resolve.Filter.SkipEvent(c, data, r.triggerUpdateBuf)
		if err != nil {
			r.asyncErrorWriter.WriteError(c, err, s.resolve.Response, s.writer)
			continue
		}
		if skip {
			continue
		}
		wg.Add(1)
		fn := func() {
			r.executeSubscriptionUpdate(c, s, data)
		}
		go func(fn func()) {
			defer wg.Done()
			if s.executor != nil {
				select {
				case <-r.ctx.Done():
				case <-c.ctx.Done():
				case s.executor <- fn:
				}
			} else {
				fn()
			}
		}(fn)
	}
}

func (r *Resolver) shutdownTrigger(id uint64) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:%d\n", id)
	}
	trig, ok := r.triggers[id]
	if !ok {
		return
	}
	count := len(trig.subscriptions)
	r.shutdownTriggerSubscriptions(id, nil)
	trig.cancel()
	delete(r.triggers, id)
	if r.options.Debug {
		fmt.Printf("resolver:trigger:done:%d\n", trig.id)
	}
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(count)
		if trig.initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
}

func (r *Resolver) shutdownTriggerSubscriptions(id uint64, shutdownMatcher func(a SubscriptionIdentifier) bool) int {
	trig, ok := r.triggers[id]
	if !ok {
		return 0
	}
	removed := 0
	for c, s := range trig.subscriptions {
		if shutdownMatcher != nil && !shutdownMatcher(s.id) {
			continue
		}
		if c.Context().Err() == nil {
			s.writer.Complete()
		}
		if s.completed != nil {
			close(s.completed)
		}
		r.heartbeatSubLock.Lock()
		delete(r.heartbeatSubscriptions, c)
		r.heartbeatSubLock.Unlock()
		delete(trig.subscriptions, c)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:done:%d:%d\n", trig.id, s.id.SubscriptionID)
		}
		removed++
	}
	return removed
}

func (r *Resolver) handleShutdown() {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown\n")
	}
	for id := range r.triggers {
		r.shutdownTrigger(id)
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:done\n")
	}
	r.triggers = make(map[uint64]*trigger)
}

type SubscriptionIdentifier struct {
	ConnectionID   int64
	SubscriptionID int64
	internal       bool
}

func (r *Resolver) AsyncUnsubscribeSubscription(id SubscriptionIdentifier) error {
	if err := r.triggerEventsSem.Acquire(r.ctx, 1); err != nil {
		return err
	}
	defer r.triggerEventsSem.Release(1)

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		id:   id,
		kind: subscriptionEventKindRemoveSubscription,
	}:
	}
	return nil
}

func (r *Resolver) AsyncUnsubscribeClient(connectionID int64) error {
	if err := r.triggerEventsSem.Acquire(r.ctx, 1); err != nil {
		return err
	}
	defer r.triggerEventsSem.Release(1)

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		id: SubscriptionIdentifier{
			ConnectionID: connectionID,
		},
		kind: subscriptionEventKindRemoveClient,
	}:
	}
	return nil
}

func (r *Resolver) ResolveGraphQLSubscription(ctx *Context, subscription *GraphQLSubscription, writer SubscriptionResponseWriter) error {
	if err := r.triggerEventsSem.Acquire(r.ctx, 1); err != nil {
		return err
	}
	defer r.triggerEventsSem.Release(1)

	if subscription.Trigger.Source == nil {
		return errors.New("no data source found")
	}
	input, err := r.subscriptionInput(ctx, subscription)
	if err != nil {
		msg := []byte(`{"errors":[{"message":"invalid input"}]}`)
		return writeFlushComplete(writer, msg)
	}

	// If SkipLoader is enabled, we skip retrieving actual data. For example, this is useful when requesting a query plan.
	// By returning early, we avoid starting a subscription and resolve with empty data instead.
	if ctx.ExecutionOptions.SkipLoader {
		t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields)

		err = t.resolvable.InitSubscription(ctx, nil, subscription.Trigger.PostProcessing)
		if err != nil {
			return err
		}

		buf := &bytes.Buffer{}
		err = t.resolvable.Resolve(ctx.ctx, subscription.Response.Data, subscription.Response.Fetches, buf)
		if err != nil {
			return err
		}

		if _, err = writer.Write(buf.Bytes()); err != nil {
			return err
		}
		if err = writer.Flush(); err != nil {
			return err
		}
		writer.Complete()

		return nil
	}

	xxh := pool.Hash64.Get()
	defer pool.Hash64.Put(xxh)
	err = subscription.Trigger.Source.UniqueRequestID(ctx, input, xxh)
	if err != nil {
		msg := []byte(`{"errors":[{"message":"unable to resolve"}]}`)
		return writeFlushComplete(writer, msg)
	}
	uniqueID := xxh.Sum64()
	id := SubscriptionIdentifier{
		ConnectionID:   r.connectionIDs.Inc(),
		SubscriptionID: 0,
		internal:       true,
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscribe:sync:%d:%d\n", uniqueID, id.SubscriptionID)
	}
	completed := make(chan struct{})
	executor := make(chan func())
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		triggerID: uniqueID,
		kind:      subscriptionEventKindAddSubscription,
		addSubscription: &addSubscription{
			ctx:       ctx,
			input:     input,
			resolve:   subscription,
			writer:    writer,
			id:        id,
			completed: completed,
			executor:  executor,
		},
	}:
	}
Loop: // execute fn on the main thread of the incoming request until ctx is done
	for {
		select {
		case <-r.ctx.Done():
			// the resolver ctx was canceled
			// this will trigger the shutdown of the trigger (on another goroutine)
			// as such, we need to wait for the trigger to be shutdown
			// otherwise we might experience a data race between trigger shutdown write (Complete) and reading bytes written to the writer
			// as the shutdown happens asynchronously, we want to wait here for at most 5 seconds or until the client ctx is done
			select {
			case <-completed:
				return r.ctx.Err()
			case <-time.After(time.Second * 5):
				return r.ctx.Err()
			case <-ctx.Context().Done():
				return ctx.Context().Err()
			}
		case <-ctx.Context().Done():
			break Loop
		case fn := <-executor:
			fn()
		}
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:unsubscribe:sync:%d:%d\n", uniqueID, id.SubscriptionID)
	}
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		triggerID: uniqueID,
		kind:      subscriptionEventKindRemoveSubscription,
		id:        id,
	}:
	}
	return nil
}

func (r *Resolver) AsyncResolveGraphQLSubscription(ctx *Context, subscription *GraphQLSubscription, writer SubscriptionResponseWriter, id SubscriptionIdentifier) (err error) {
	if err := r.triggerEventsSem.Acquire(r.ctx, 1); err != nil {
		return err
	}
	defer r.triggerEventsSem.Release(1)

	if subscription.Trigger.Source == nil {
		return errors.New("no data source found")
	}
	input, err := r.subscriptionInput(ctx, subscription)
	if err != nil {
		msg := []byte(`{"errors":[{"message":"invalid input"}]}`)
		return writeFlushComplete(writer, msg)
	}

	// If SkipLoader is enabled, we skip retrieving actual data. For example, this is useful when requesting a query plan.
	// By returning early, we avoid starting a subscription and resolve with empty data instead.
	if ctx.ExecutionOptions.SkipLoader {
		t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields)

		err = t.resolvable.InitSubscription(ctx, nil, subscription.Trigger.PostProcessing)
		if err != nil {
			return err
		}

		buf := &bytes.Buffer{}
		err = t.resolvable.Resolve(ctx.ctx, subscription.Response.Data, subscription.Response.Fetches, buf)
		if err != nil {
			return err
		}

		if _, err = writer.Write(buf.Bytes()); err != nil {
			return err
		}
		if err = writer.Flush(); err != nil {
			return err
		}
		writer.Complete()

		return nil
	}

	xxh := pool.Hash64.Get()
	defer pool.Hash64.Put(xxh)
	err = subscription.Trigger.Source.UniqueRequestID(ctx, input, xxh)
	if err != nil {
		msg := []byte(`{"errors":[{"message":"unable to resolve"}]}`)
		return writeFlushComplete(writer, msg)
	}
	uniqueID := xxh.Sum64()

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		triggerID: uniqueID,
		kind:      subscriptionEventKindAddSubscription,
		addSubscription: &addSubscription{
			ctx:     ctx,
			input:   input,
			resolve: subscription,
			writer:  writer,
			id:      id,
		},
	}:
	}
	return nil
}

func (r *Resolver) subscriptionInput(ctx *Context, subscription *GraphQLSubscription) (input []byte, err error) {
	buf := new(bytes.Buffer)
	err = subscription.Trigger.InputTemplate.Render(ctx, nil, buf)
	if err != nil {
		return nil, err
	}
	input = buf.Bytes()
	if len(ctx.InitialPayload) > 0 {
		input, err = jsonparser.Set(input, ctx.InitialPayload, "initial_payload")
		if err != nil {
			return nil, err
		}
	}
	if ctx.Extensions != nil {
		input, err = jsonparser.Set(input, ctx.Extensions, "body", "extensions")
		if err != nil {
			return nil, err
		}
	}
	return input, nil
}

type subscriptionUpdater struct {
	done      bool
	debug     bool
	triggerID uint64
	ch        chan subscriptionEvent
	ctx       context.Context
	updateSem *semaphore.Weighted
}

func (s *subscriptionUpdater) Update(data []byte) {
	if s.debug {
		fmt.Printf("resolver:subscription_updater:update:%d\n", s.triggerID)
	}

	if err := s.updateSem.Acquire(s.ctx, 1); err != nil {
		return
	}
	defer s.updateSem.Release(1)

	if s.done {
		return
	}
	select {
	case <-s.ctx.Done():
		return
	case s.ch <- subscriptionEvent{
		triggerID: s.triggerID,
		kind:      subscriptionEventKindTriggerUpdate,
		data:      data,
	}:
	}
}

func (s *subscriptionUpdater) Done() {
	if s.debug {
		fmt.Printf("resolver:subscription_updater:done:%d\n", s.triggerID)
	}

	if err := s.updateSem.Acquire(s.ctx, 1); err != nil {
		return
	}
	defer s.updateSem.Release(1)

	if s.done {
		return
	}
	select {
	case <-s.ctx.Done():
		return
	case s.ch <- subscriptionEvent{
		triggerID: s.triggerID,
		kind:      subscriptionEventKindTriggerDone,
	}:
	}
	s.done = true
}

type subscriptionEvent struct {
	triggerID       uint64
	id              SubscriptionIdentifier
	kind            subscriptionEventKind
	data            []byte
	addSubscription *addSubscription
}

type addSubscription struct {
	ctx       *Context
	input     []byte
	resolve   *GraphQLSubscription
	writer    SubscriptionResponseWriter
	id        SubscriptionIdentifier
	completed chan struct{}
	executor  chan func()
}

type subscriptionEventKind int

const (
	subscriptionEventKindUnknown subscriptionEventKind = iota
	subscriptionEventKindTriggerUpdate
	subscriptionEventKindTriggerDone
	subscriptionEventKindAddSubscription
	subscriptionEventKindRemoveSubscription
	subscriptionEventKindRemoveClient
	subscriptionEventKindTriggerInitialized
	subscriptionEventKindTriggerShutdown
)

type SubscriptionUpdater interface {
	// Update sends an update to the client. It is not guaranteed that the update is sent immediately.
	Update(data []byte)
	// Done also takes care of cleaning up the trigger and all subscriptions. No more updates should be sent after calling Done.
	Done()
}
