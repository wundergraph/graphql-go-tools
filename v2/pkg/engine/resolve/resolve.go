//go:generate mockgen -self_package=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource

package resolve

import (
	"bytes"
	"context"
	"fmt"
	"golang.org/x/sync/semaphore"
	"io"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/xcontext"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	HearbeatInterval = 5 * time.Second
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
	bufPool        sync.Pool
	maxConcurrency chan struct{}

	triggers         map[uint64]*trigger
	events           chan subscriptionEvent
	triggerUpdateSem *semaphore.Weighted
	triggerUpdateBuf *bytes.Buffer

	connectionIDs atomic.Int64

	reporter         Reporter
	asyncErrorWriter AsyncErrorWriter

	propagateSubgraphErrors      bool
	propagateSubgraphStatusCodes bool

	tools *sync.Pool
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
	// MaxConcurrency limits the number of concurrent resolve operations
	// if set to 0, no limit is applied
	// It is advised to set this to a reasonable value to prevent excessive memory usage
	// Each concurrent resolve operation allocates ~50kb of memory
	// In addition, there's a limit of how many concurrent requests can be efficiently resolved
	// This depends on the number of CPU cores available, the complexity of the operations, and the origin services
	MaxConcurrency int

	MaxSubscriptionWorkers int

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
	// ResolvableOptions are configuration options for the Resolbable struct
	ResolvableOptions ResolvableOptions
	// AllowedCustomSubgraphErrorFields defines which fields are allowed in the subgraph error when in passthrough mode
	AllowedSubgraphErrorFields []string
}

// New returns a new Resolver, ctx.Done() is used to cancel all active subscriptions & streams
func New(ctx context.Context, options ResolverOptions) *Resolver {
	// options.Debug = true
	if options.MaxConcurrency <= 0 {
		options.MaxConcurrency = 32
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
		ctx:                          ctx,
		options:                      options,
		propagateSubgraphErrors:      options.PropagateSubgraphErrors,
		propagateSubgraphStatusCodes: options.PropagateSubgraphStatusCodes,
		events:                       make(chan subscriptionEvent),
		triggers:                     make(map[uint64]*trigger),
		reporter:                     options.Reporter,
		asyncErrorWriter:             options.AsyncErrorWriter,
		triggerUpdateBuf:             bytes.NewBuffer(make([]byte, 0, 1024)),
		tools: &sync.Pool{
			New: func() any {
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
			},
		},
	}
	resolver.maxConcurrency = make(chan struct{}, options.MaxConcurrency)
	for i := 0; i < options.MaxConcurrency; i++ {
		resolver.maxConcurrency <- struct{}{}
	}
	if options.MaxSubscriptionWorkers == 0 {
		options.MaxSubscriptionWorkers = 1024
	}

	resolver.triggerUpdateSem = semaphore.NewWeighted(int64(options.MaxSubscriptionWorkers))

	go resolver.handleEvents()

	return resolver
}

func (r *Resolver) getTools() (time.Duration, *tools) {
	start := time.Now()
	<-r.maxConcurrency
	tool := r.tools.Get().(*tools)
	return time.Since(start), tool
}

func (r *Resolver) putTools(t *tools) {
	t.loader.Free()
	t.resolvable.Reset(r.options.MaxRecyclableParserSize)
	r.tools.Put(t)
	r.maxConcurrency <- struct{}{}
}

func (r *Resolver) getBuffer() *bytes.Buffer {
	maybeBuffer := r.bufPool.Get()
	if maybeBuffer == nil {
		return &bytes.Buffer{}
	}
	return maybeBuffer.(*bytes.Buffer)
}

func (r *Resolver) releaseBuffer(buf *bytes.Buffer) {
	buf.Reset()
	r.bufPool.Put(buf)
}

type GraphQLResolveInfo struct {
	ResolveAcquireWaitTime time.Duration
}

func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (*GraphQLResolveInfo, error) {

	resp := &GraphQLResolveInfo{}

	toolsCleaned := false
	acquireWaitTime, t := r.getTools()
	resp.ResolveAcquireWaitTime = acquireWaitTime

	// Ensure that the tools are returned even on panic
	// This is important because getTools() acquires a semaphore
	// and if we don't return the tools, we will have a deadlock
	defer func() {
		if !toolsCleaned {
			r.putTools(t)
		}
	}()

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

	buf := r.getBuffer()
	defer r.releaseBuffer(buf)
	err = t.resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, buf)

	// Return the tools as soon as possible. More efficient in case of a slow client / network.
	r.putTools(t)
	toolsCleaned = true

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

func (t *trigger) hasPendingUpdates() bool {
	for _, s := range t.subscriptions {
		s.mux.Lock()
		hasUpdates := s.pendingUpdates != 0
		s.mux.Unlock()
		if hasUpdates {
			return true
		}
	}
	return false
}

type sub struct {
	mux            sync.Mutex
	resolve        *GraphQLSubscription
	writer         SubscriptionResponseWriter
	id             SubscriptionIdentifier
	pendingUpdates int
	completed      chan struct{}
	sendHeartbeat  bool
}

func (r *Resolver) executeSubscriptionUpdate(ctx *Context, sub *sub, sharedInput []byte) {
	sub.mux.Lock()
	sub.pendingUpdates++
	sub.mux.Unlock()

	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:update:%d\n", sub.id.SubscriptionID)
	}
	_, t := r.getTools()
	defer r.putTools(t)

	input := make([]byte, len(sharedInput))
	copy(input, sharedInput)

	if err := t.resolvable.InitSubscription(ctx, input, sub.resolve.Trigger.PostProcessing); err != nil {
		sub.mux.Lock()
		r.asyncErrorWriter.WriteError(ctx, err, sub.resolve.Response, sub.writer)
		sub.pendingUpdates--
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
		sub.pendingUpdates--
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
	sub.pendingUpdates--
	sub.mux.Unlock()

	sub.mux.Lock()
	sub.pendingUpdates--
	defer sub.mux.Unlock()

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
	for {
		select {
		case <-done:
			r.handleShutdown()
			return
		case event := <-r.events:
			r.handleEvent(event)
		}
	}
}

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
	case subscriptionEventKindHeartbeat:
		r.handleHeartbeat(event.triggerID, event.data)
	case subscriptionEventKindUnknown:
		panic("unknown event")
	}
}

func (r *Resolver) handleHeartbeat(id uint64, data []byte) {
	trig, ok := r.triggers[id]
	if !ok {
		return
	}
	if r.options.Debug {
		fmt.Printf("resolver:heartbeat:%d\n", id)
	}
	for c, s := range trig.subscriptions {
		c, s := c, s
		// Only send heartbeats to subscriptions who have enabled it
		if !s.sendHeartbeat {
			continue
		}
		if err := r.triggerUpdateSem.Acquire(r.ctx, 1); err != nil {
			return
		}
		go func() {
			defer r.triggerUpdateSem.Release(1)

			if r.options.Debug {
				fmt.Printf("resolver:heartbeat:subscription:%d\n", s.id.SubscriptionID)
			}

			s.mux.Lock()
			if _, err := s.writer.Write(data); err != nil {
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
	trig, ok := r.triggers[triggerID]
	if !ok {
		return
	}
	isInitialized := trig.initialized
	wg := trig.inFlight
	subscriptionCount := len(trig.subscriptions)

	delete(r.triggers, triggerID)

	go func() {
		if wg != nil {
			wg.Wait()
		}
		for _, s := range trig.subscriptions {
			s.writer.Complete()
		}
		if r.reporter != nil {
			r.reporter.SubscriptionCountDec(subscriptionCount)
			if isInitialized {
				r.reporter.TriggerCountDec(1)
			}
		}
	}()
}

func (r *Resolver) handleAddSubscription(triggerID uint64, add *addSubscription) {
	var (
		err error
	)
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:add:%d:%d\n", triggerID, add.id.SubscriptionID)
	}
	s := &sub{
		resolve:       add.resolve,
		writer:        add.writer,
		id:            add.id,
		completed:     add.completed,
		sendHeartbeat: add.ctx.ExecutionOptions.SendHeartbeat,
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

	go func() {
		if r.options.Debug {
			fmt.Printf("resolver:trigger:start:%d\n", triggerID)
		}

		err = add.resolve.Trigger.Source.Start(cloneCtx, add.input, updater)
		if err != nil {
			if r.options.Debug {
				fmt.Printf("resolver:trigger:failed:%d\n", triggerID)
			}
			r.asyncErrorWriter.WriteError(add.ctx, err, add.resolve.Response, add.writer)
			r.emitTriggerShutdown(triggerID)
			return
		}

		r.emitTriggerInitialized(triggerID)

		if r.options.Debug {
			fmt.Printf("resolver:trigger:started:%d\n", triggerID)
		}
	}()

}

func (r *Resolver) emitTriggerShutdown(triggerID uint64) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:%d\n", triggerID)
	}

	select {
	case <-r.ctx.Done():
		return
	case r.events <- subscriptionEvent{
		triggerID: triggerID,
		kind:      subscriptionEventKindTriggerShutdown,
	}:
	}
}

func (r *Resolver) emitTriggerInitialized(triggerID uint64) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:initialized:%d\n", triggerID)
	}

	select {
	case <-r.ctx.Done():
		return
	case r.events <- subscriptionEvent{
		triggerID: triggerID,
		kind:      subscriptionEventKindTriggerInitialized,
	}:
	}
}

func (r *Resolver) handleRemoveSubscription(id SubscriptionIdentifier) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:%d:%d\n", id.ConnectionID, id.SubscriptionID)
	}
	removed := 0
	for u := range r.triggers {
		trig := r.triggers[u]
		for ctx, s := range trig.subscriptions {
			if s.id == id {

				if ctx.Context().Err() == nil {
					s.writer.Complete()
				}

				delete(trig.subscriptions, ctx)
				if r.options.Debug {
					fmt.Printf("resolver:trigger:subscription:removed:%d:%d\n", trig.id, id.SubscriptionID)
				}
				removed++
			}
		}
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
		for c, s := range r.triggers[u].subscriptions {
			if s.id.ConnectionID == id && !s.id.internal {

				if c.Context().Err() == nil {
					s.writer.Complete()
				}

				delete(r.triggers[u].subscriptions, c)
				if r.options.Debug {
					fmt.Printf("resolver:trigger:subscription:done:%d:%d\n", u, s.id.SubscriptionID)
				}
				removed++
			}
		}
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
		skip, err := s.resolve.Filter.SkipEvent(c, data, r.triggerUpdateBuf)
		if err != nil {
			r.asyncErrorWriter.WriteError(c, err, s.resolve.Response, s.writer)
			continue
		}
		if skip {
			continue
		}

		if err := r.triggerUpdateSem.Acquire(r.ctx, 1); err != nil {
			return
		}

		wg.Add(1)
		go func() {
			defer r.triggerUpdateSem.Release(1)
			defer wg.Done()
			r.executeSubscriptionUpdate(c, s, data)
		}()
	}
}

func (r *Resolver) shutdownTrigger(id uint64) {
	trig, ok := r.triggers[id]
	if !ok {
		return
	}
	count := len(trig.subscriptions)
	for c, s := range trig.subscriptions {
		if c.Context().Err() == nil {
			s.writer.Complete()
		}
		if s.completed != nil {
			close(s.completed)
		}
		delete(trig.subscriptions, c)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:done:%d:%d\n", trig.id, s.id.SubscriptionID)
		}
	}
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
	if subscription.Trigger.Source == nil {
		return errors.New("no data source found")
	}
	input, err := r.subscriptionInput(ctx, subscription)
	if err != nil {
		msg := []byte(`{"errors":[{"message":"invalid input"}]}`)
		return writeFlushComplete(writer, msg)
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
		},
	}:
	}
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
	if subscription.Trigger.Source == nil {
		return errors.New("no data source found")
	}
	input, err := r.subscriptionInput(ctx, subscription)
	if err != nil {
		msg := []byte(`{"errors":[{"message":"invalid input"}]}`)
		return writeFlushComplete(writer, msg)
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
}

func (s *subscriptionUpdater) Heartbeat() {
	if s.debug {
		fmt.Printf("resolver:subscription_updater:heartbeat:%d\n", s.triggerID)
	}
	if s.done {
		return
	}

	select {
	case <-s.ctx.Done():
		return
	case s.ch <- subscriptionEvent{
		triggerID: s.triggerID,
		kind:      subscriptionEventKindHeartbeat,
		data:      multipartHeartbeat,
		// Currently, the only heartbeat we support is for multipart subscriptions. If we need to support future types
		// of subscriptions, we can evaluate then how we can save on the subscription level what kind of heartbeat it
		// requires
	}:
	}
}

func (s *subscriptionUpdater) Update(data []byte) {
	if s.debug {
		fmt.Printf("resolver:subscription_updater:update:%d\n", s.triggerID)
	}
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
	subscriptionEventKindHeartbeat
)

type SubscriptionUpdater interface {
	// Update sends an update to the client. It is not guaranteed that the update is sent immediately.
	Update(data []byte)
	// Heartbeat sends a heartbeat to the client. It is not guaranteed that the update is sent immediately. When calling,
	// clients should reset their heartbeat timer after an Update call to make sure that we don't send needless heartbeats
	// downstream
	Heartbeat()
	// Done also takes care of cleaning up the trigger and all subscriptions. No more updates should be sent after calling Done.
	Done()
}
