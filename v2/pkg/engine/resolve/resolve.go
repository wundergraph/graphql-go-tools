//go:generate mockgen -self_package=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource

package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

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

	triggers         map[uint64]*trigger
	events           chan subscriptionEvent
	triggerUpdateBuf *bytes.Buffer

	allowedErrorExtensionFields map[string]struct{}
	allowedErrorFields          map[string]struct{}

	connectionIDs atomic.Int64

	reporter         Reporter
	asyncErrorWriter AsyncErrorWriter

	propagateSubgraphErrors      bool
	propagateSubgraphStatusCodes bool
	// Multipart heartbeat interval
	heartbeatInterval time.Duration
	// maxSubscriptionFetchTimeout defines the maximum time a subscription fetch can take before it is considered timed out
	maxSubscriptionFetchTimeout time.Duration
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
	// MaxSubscriptionFetchTimeout defines the maximum time a subscription fetch can take before it is considered timed out
	MaxSubscriptionFetchTimeout time.Duration
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

	if options.MaxSubscriptionFetchTimeout == 0 {
		options.MaxSubscriptionFetchTimeout = 30 * time.Second
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
		allowedErrorExtensionFields:  allowedExtensionFields,
		allowedErrorFields:           allowedErrorFields,
		heartbeatInterval:            options.MultipartSubHeartbeatInterval,
		maxSubscriptionFetchTimeout:  options.MaxSubscriptionFetchTimeout,
	}
	resolver.maxConcurrency = make(chan struct{}, options.MaxConcurrency)
	for i := 0; i < options.MaxConcurrency; i++ {
		resolver.maxConcurrency <- struct{}{}
	}

	go resolver.processEvents()

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

			apolloRouterCompatibilitySubrequestHTTPError: options.ResolvableOptions.ApolloRouterCompatibilitySubrequestHTTPError,
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

	if err := r.doResolve(ctx, t, response, data, writer); err != nil {
		return nil, err
	}

	if iw, ok := writer.(IncrementalResponseWriter); ok {
		if err := iw.Complete(); err != nil {
			return nil, fmt.Errorf("completing response: %w", err)
		}
	}
	return resp, nil
}

var errInvalidWriter = errors.New("invalid writer")

func (r *Resolver) doResolve(ctx *Context, t *tools, response *GraphQLResponse, data []byte, writer io.Writer) error {

	err := t.resolvable.Init(ctx, data, response.Info.OperationType)
	if err != nil {
		return err
	}
	if !ctx.ExecutionOptions.SkipLoader {
		err := t.loader.LoadGraphQLResponseData(ctx, response, t.resolvable)
		if err != nil {
			return err
		}
	}

	buf := &bytes.Buffer{}
	if err := t.resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, buf); err != nil {
		return err
	}

	if _, err := buf.WriteTo(writer); err != nil {
		return fmt.Errorf("writing response: %w", err)
	}

	if iw, ok := writer.(IncrementalResponseWriter); ok {
		if err := iw.Flush(resolvedPath(response.Data.Path)); err != nil {
			return fmt.Errorf("flushing immediate response: %w", err)
		}
	}

	if len(response.DeferredResponses) > 0 {
		iw, ok := writer.(IncrementalResponseWriter)
		if !ok {
			return fmt.Errorf("%w: writer %T does not support incremental writing", errInvalidWriter, writer)
		}

		for i, deferredResponse := range response.DeferredResponses {
			if err := r.doResolve(ctx, t, deferredResponse, nil, iw); err != nil {
				return fmt.Errorf("resolving deferred response %d: %w", i, err)
			}
			if err := iw.Flush(resolvedPath(deferredResponse.Data.Path)); err != nil {
				return fmt.Errorf("flushing incremental response: %w", err)
			}
		}
	}
	return nil
}

func resolvedPath(data []string) []any {
	if len(data) == 0 {
		return nil
	}
	ret := make([]any, len(data))
	for i, v := range data {
		if v == "@" {
			ret[i] = 0 // TODO(cd): need the real values here.
			continue
		}
		ret[i] = v
	}
	return ret
}

type trigger struct {
	id            uint64
	cancel        context.CancelFunc
	subscriptions map[*Context]*sub
	// initialized is set to true when the trigger is started and initialized
	initialized bool
}

type sub struct {
	resolve   *GraphQLSubscription
	resolver  *Resolver
	ctx       *Context
	writer    SubscriptionResponseWriter
	id        SubscriptionIdentifier
	heartbeat bool
	completed chan struct{}
	// workChan is used to send work to the writer goroutine. All work is processed sequentially.
	workChan chan func()
}

// startWorker runs in its own goroutine to process fetches and write data to the client synchronously
// it also takes care of sending heartbeats to the client but only if the subscription supports it
func (s *sub) startWorker() {
	if s.heartbeat {
		s.startWorkerWithHeartbeat()
		return
	}
	s.startWorkerWithoutHeartbeat()
}

func (s *sub) startWorkerWithHeartbeat() {
	heartbeatTicker := time.NewTicker(s.resolver.heartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-s.resolver.ctx.Done(): // Skip sending events if the resolver is shutting down
			return
		case <-heartbeatTicker.C:
			s.resolver.handleHeartbeat(s, multipartHeartbeat)
		case fn := <-s.workChan:
			fn()
			// Reset the heartbeat ticker after each write to avoid sending unnecessary heartbeats
			heartbeatTicker.Reset(s.resolver.heartbeatInterval)
		case <-s.completed: // Shutdown the writer when the subscription is completed
			return
		}
	}
}

func (s *sub) startWorkerWithoutHeartbeat() {

	for {
		select {
		case <-s.resolver.ctx.Done(): // Skip sending events if the resolver is shutting down
			return
		case fn := <-s.workChan:
			fn()
		case <-s.completed: // Shutdown the writer when the subscription is completed
			return
		}
	}
}

func (r *Resolver) executeSubscriptionUpdate(resolveCtx *Context, sub *sub, sharedInput []byte) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:update:%d\n", sub.id.SubscriptionID)
	}

	ctx, cancel := context.WithTimeout(resolveCtx.ctx, r.maxSubscriptionFetchTimeout)
	defer cancel()

	resolveCtx = resolveCtx.WithContext(ctx)

	// Copy the input.
	input := make([]byte, len(sharedInput))
	copy(input, sharedInput)

	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields)

	if err := t.resolvable.InitSubscription(resolveCtx, input, sub.resolve.Trigger.PostProcessing); err != nil {
		r.asyncErrorWriter.WriteError(resolveCtx, err, sub.resolve.Response, sub.writer)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:init:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}

	if err := t.loader.LoadGraphQLResponseData(resolveCtx, sub.resolve.Response, t.resolvable); err != nil {
		r.asyncErrorWriter.WriteError(resolveCtx, err, sub.resolve.Response, sub.writer)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:load:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}

	if err := t.resolvable.Resolve(resolveCtx.ctx, sub.resolve.Response.Data, sub.resolve.Response.Fetches, sub.writer); err != nil {
		r.asyncErrorWriter.WriteError(resolveCtx, err, sub.resolve.Response, sub.writer)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:resolve:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}

	if err := sub.writer.Flush(); err != nil {
		// If flush fails (e.g. client disconnected), remove the subscription.
		_ = r.AsyncUnsubscribeSubscription(sub.id)
		return
	}

	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:flushed:%d\n", sub.id.SubscriptionID)
	}
	if r.reporter != nil {
		r.reporter.SubscriptionUpdateSent()
	}

	if t.resolvable.WroteErrorsWithoutData() && r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:completing:errors_without_data:%d\n", sub.id.SubscriptionID)
	}
}

// processEvents maintains the single threaded event loop that processes all events
func (r *Resolver) processEvents() {
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

// handleHeartbeat sends a heartbeat to the client. It needs to be executed on the same goroutine as the writer.
func (r *Resolver) handleHeartbeat(sub *sub, data []byte) {
	if r.options.Debug {
		fmt.Printf("resolver:heartbeat\n")
	}

	if r.ctx.Err() != nil {
		return
	}

	if sub.ctx.Context().Err() != nil {
		return
	}

	if r.options.Debug {
		fmt.Printf("resolver:heartbeat:subscription:%d\n", sub.id.SubscriptionID)
	}

	if _, err := sub.writer.Write(data); err != nil {
		if errors.Is(err, context.Canceled) {
			// If Write fails (e.g. client disconnected), remove the subscription.
			_ = r.AsyncUnsubscribeSubscription(sub.id)
			return
		}
		r.asyncErrorWriter.WriteError(sub.ctx, err, nil, sub.writer)
	}
	err := sub.writer.Flush()
	if err != nil {
		// If flush fails (e.g. client disconnected), remove the subscription.
		_ = r.AsyncUnsubscribeSubscription(sub.id)
		return
	}

	if r.options.Debug {
		fmt.Printf("resolver:heartbeat:subscription:flushed:%d\n", sub.id.SubscriptionID)
	}
	if r.reporter != nil {
		r.reporter.SubscriptionUpdateSent()
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
		ctx:       add.ctx,
		resolve:   add.resolve,
		writer:    add.writer,
		id:        add.id,
		completed: add.completed,
		workChan:  make(chan func(), 32),
		resolver:  r,
	}

	if add.ctx.ExecutionOptions.SendHeartbeat {
		s.heartbeat = true
	}

	// Start the dedicated worker goroutine where the subscription updates are processed
	// and writes are written to the client in a single threaded manner
	go s.startWorker()

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
			return sID.ConnectionID == id
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

		fn := func() {
			r.executeSubscriptionUpdate(c, s, data)
		}

		select {
		case <-r.ctx.Done():
			// Skip sending all events if the resolver is shutting down
			return
		case <-c.ctx.Done():
			// Skip sending the event if the client disconnected
		case s.workChan <- fn:
			// Send the work to the subscription worker
		case <-s.completed:
			// Stop sending if the subscription is completed. Otherwise, this could block the event loop forever
			// when the subscription worker was shutdown after channel close but the event was still scheduled.
			if s.resolver.options.Debug {
				fmt.Printf("resolver:trigger:subscription:completed:%d:%d\n", s.id.ConnectionID, s.id.SubscriptionID)
			}
		}
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

	removed := r.shutdownTriggerSubscriptions(id, nil)

	// Cancels the async datasource and cleanup the connection
	trig.cancel()

	delete(r.triggers, id)

	if r.options.Debug {
		fmt.Printf("resolver:trigger:done:%d\n", trig.id)
	}

	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
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
		// We close the completed channel on the work channel of the subscription
		// to ensure that all jobs are processed before the channel is closed.
		select {
		case <-r.ctx.Done():
			// Skip sending the event if the resolver is shutting down
		case <-c.ctx.Done():
			// Skip sending the event if the client disconnected
		case s.workChan <- func() {
			// We put the complete handshake to the work channel of the subscription
			// to ensure that it is the last message that is sent to the client.
			if c.Context().Err() == nil {
				s.writer.Complete()
			}
			// This will shutdown the subscription worker
			close(s.completed)
		}:
		}

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

	// This will immediately block until one of the following conditions is met:
	select {
	case <-ctx.ctx.Done():
		// Client disconnected, request context canceled.
		// We will ignore the error and remove the subscription in the next step.
	case <-r.ctx.Done():
		// Resolver shutdown, no way to gracefully shut down the subscription
		// because the event loop is not running anymore.
		return r.ctx.Err()
	case <-completed:
		// Subscription completed and drained. No need to do anything.
		return nil
	}

	if r.options.Debug {
		fmt.Printf("resolver:trigger:unsubscribe:sync:%d:%d\n", uniqueID, id.SubscriptionID)
	}

	// Remove the subscription when the client disconnects.

	r.events <- subscriptionEvent{
		triggerID: uniqueID,
		kind:      subscriptionEventKindRemoveSubscription,
		id:        id,
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

	event := subscriptionEvent{
		triggerID: xxh.Sum64(),
		kind:      subscriptionEventKindAddSubscription,
		addSubscription: &addSubscription{
			ctx:       ctx,
			input:     input,
			resolve:   subscription,
			writer:    writer,
			id:        id,
			completed: make(chan struct{}),
		},
	}

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- event:
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
	debug     bool
	triggerID uint64
	ch        chan subscriptionEvent
	ctx       context.Context
}

func (s *subscriptionUpdater) Update(data []byte) {
	if s.debug {
		fmt.Printf("resolver:subscription_updater:update:%d\n", s.triggerID)
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

	select {
	case <-s.ctx.Done():
		return
	case s.ch <- subscriptionEvent{
		triggerID: s.triggerID,
		kind:      subscriptionEventKindTriggerDone,
	}:
	}
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
)

type SubscriptionUpdater interface {
	// Update sends an update to the client. It is not guaranteed that the update is sent immediately.
	Update(data []byte)
	// Done also takes care of cleaning up the trigger and all subscriptions. No more updates should be sent after calling Done.
	Done()
}
