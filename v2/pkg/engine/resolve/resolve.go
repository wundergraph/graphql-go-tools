//go:generate mockgen -self_package=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource

package resolve

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/xcontext"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	DefaultHeartbeatInterval = 5 * time.Second
)

// ConnectionIDs is used to create unique connection IDs for each subscription
// Whenever a new connection is created, use this to generate a new ID
// It is public because it can be used in more high level packages to instantiate a new connection
var ConnectionIDs = atomic.NewInt64(0)

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

// Resolver is a single threaded event loop that processes all events on a single goroutine.
// It is absolutely critical to ensure that all events are processed quickly to prevent blocking
// and that resolver modifications are done on the event loop goroutine. Long-running operations
// should be offloaded to the subscription worker goroutine. If a different goroutine needs to emit
// an event, it should be done through the events channel to avoid race conditions.
type Resolver struct {
	ctx            context.Context
	options        ResolverOptions
	maxConcurrency chan struct{}

	triggers         map[uint64]*trigger
	events           chan subscriptionEvent
	triggerUpdateBuf *bytes.Buffer

	allowedErrorExtensionFields map[string]struct{}
	allowedErrorFields          map[string]struct{}

	reporter         Reporter
	asyncErrorWriter AsyncErrorWriter

	propagateSubgraphErrors      bool
	propagateSubgraphStatusCodes bool
	// Subscription heartbeat interval
	heartbeatInterval time.Duration
	// maxSubscriptionFetchTimeout defines the maximum time a subscription fetch can take before it is considered timed out
	maxSubscriptionFetchTimeout time.Duration

	// resolveArenaPool is the arena pool dedicated for Loader & Resolvable
	// ArenaPool automatically adjusts arena buffer sizes per workload
	// resolving & response buffering are very different tasks
	// as such, it was best to have two arena pools in terms of memory usage
	// A single pool for both was much less efficient
	resolveArenaPool *ArenaPool
	// responseBufferPool is the arena pool dedicated for response buffering before sending to the client
	responseBufferPool *ArenaPool

	// subgraphRequestSingleFlight is used to de-duplicate subgraph requests
	subgraphRequestSingleFlight *SubgraphRequestSingleFlight
	// inboundRequestSingleFlight is used to de-duplicate subgraph requests
	inboundRequestSingleFlight *InboundRequestSingleFlight
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

	// AllowAllErrorExtensionFields allows all fields in the extensions field of a root subgraph error
	AllowAllErrorExtensionFields bool

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

	// SubscriptionHeartbeatInterval defines the interval in which a heartbeat is sent to all subscriptions (whether or not this does anything is determined by the subscription response writer)
	SubscriptionHeartbeatInterval time.Duration

	// MaxSubscriptionFetchTimeout defines the maximum time a subscription fetch can take before it is considered timed out
	MaxSubscriptionFetchTimeout time.Duration

	// ApolloRouterCompatibilitySubrequestHTTPError is a compatibility flag for Apollo Router, it is used to handle HTTP errors in subrequests differently
	ApolloRouterCompatibilitySubrequestHTTPError bool

	// PropagateFetchReasons enables adding the "fetch_reasons" extension to
	// upstream subgraph requests. This extension explains why each field was requested.
	// This flag does not expose the data to clients.
	PropagateFetchReasons bool

	ValidateRequiredExternalFields bool
}

// New returns a new Resolver. ctx.Done() is used to cancel all active subscriptions and streams.
func New(ctx context.Context, options ResolverOptions) *Resolver {
	// options.Debug = true
	if options.MaxConcurrency <= 0 {
		options.MaxConcurrency = 32
	}

	if options.SubscriptionHeartbeatInterval <= 0 {
		options.SubscriptionHeartbeatInterval = DefaultHeartbeatInterval
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
		heartbeatInterval:            options.SubscriptionHeartbeatInterval,
		maxSubscriptionFetchTimeout:  options.MaxSubscriptionFetchTimeout,
		resolveArenaPool:             NewArenaPool(),
		responseBufferPool:           NewArenaPool(),
		subgraphRequestSingleFlight:  NewSingleFlight(8),
		inboundRequestSingleFlight:   NewRequestSingleFlight(8),
	}
	resolver.maxConcurrency = make(chan struct{}, options.MaxConcurrency)
	for i := 0; i < options.MaxConcurrency; i++ {
		resolver.maxConcurrency <- struct{}{}
	}

	go resolver.processEvents()

	return resolver
}

func newTools(options ResolverOptions, allowedExtensionFields map[string]struct{}, allowedErrorFields map[string]struct{}, sf *SubgraphRequestSingleFlight, a arena.Arena) *tools {
	return &tools{
		resolvable: NewResolvable(a, options.ResolvableOptions),
		loader: &Loader{
			propagateSubgraphErrors:                      options.PropagateSubgraphErrors,
			propagateSubgraphStatusCodes:                 options.PropagateSubgraphStatusCodes,
			subgraphErrorPropagationMode:                 options.SubgraphErrorPropagationMode,
			rewriteSubgraphErrorPaths:                    options.RewriteSubgraphErrorPaths,
			omitSubgraphErrorLocations:                   options.OmitSubgraphErrorLocations,
			omitSubgraphErrorExtensions:                  options.OmitSubgraphErrorExtensions,
			allowedErrorExtensionFields:                  allowedExtensionFields,
			attachServiceNameToErrorExtension:            options.AttachServiceNameToErrorExtensions,
			defaultErrorExtensionCode:                    options.DefaultErrorExtensionCode,
			allowedSubgraphErrorFields:                   allowedErrorFields,
			allowAllErrorExtensionFields:                 options.AllowAllErrorExtensionFields,
			apolloRouterCompatibilitySubrequestHTTPError: options.ApolloRouterCompatibilitySubrequestHTTPError,
			propagateFetchReasons:                        options.PropagateFetchReasons,
			validateRequiredExternalFields:               options.ValidateRequiredExternalFields,
			sf:                                           sf,
			jsonArena:                                    a,
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

	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, nil)

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

	err = t.resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, writer)
	if err != nil {
		return nil, err
	}

	return resp, err
}

func (r *Resolver) ArenaResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, writer io.Writer) (*GraphQLResolveInfo, error) {
	resp := &GraphQLResolveInfo{}

	inflight, err := r.inboundRequestSingleFlight.GetOrCreate(ctx, response)
	if err != nil {
		return nil, err
	}

	if inflight != nil && inflight.Data != nil { // follower
		_, err = writer.Write(inflight.Data)
		return resp, err
	}

	start := time.Now()
	<-r.maxConcurrency
	resp.ResolveAcquireWaitTime = time.Since(start)
	defer func() {
		r.maxConcurrency <- struct{}{}
	}()

	resolveArena := r.resolveArenaPool.Acquire(ctx.Request.ID)
	// we're intentionally not using defer Release to have more control over the timing (see below)
	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena)

	err = t.resolvable.Init(ctx, nil, response.Info.OperationType)
	if err != nil {
		r.inboundRequestSingleFlight.FinishErr(inflight, err)
		r.resolveArenaPool.Release(resolveArena)
		return nil, err
	}

	if !ctx.ExecutionOptions.SkipLoader {
		err = t.loader.LoadGraphQLResponseData(ctx, response, t.resolvable)
		if err != nil {
			r.inboundRequestSingleFlight.FinishErr(inflight, err)
			r.resolveArenaPool.Release(resolveArena)
			return nil, err
		}
	}

	// only when loading is done, acquire an arena for the response buffer
	responseArena := r.responseBufferPool.Acquire(ctx.Request.ID)
	buf := arena.NewArenaBuffer(responseArena.Arena)
	err = t.resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, buf)
	if err != nil {
		r.inboundRequestSingleFlight.FinishErr(inflight, err)
		r.resolveArenaPool.Release(resolveArena)
		r.responseBufferPool.Release(responseArena)
		return nil, err
	}

	// first release resolverArena
	// all data is resolved and written into the response arena
	r.resolveArenaPool.Release(resolveArena)
	// next we write back to the client
	// this includes flushing and syscalls
	// as such, it can take some time
	// which is why we split the arenas and released the first one
	_, err = writer.Write(buf.Bytes())
	r.inboundRequestSingleFlight.FinishOk(inflight, buf.Bytes())
	// all data is written to the client
	// we're safe to release our buffer
	r.responseBufferPool.Release(responseArena)
	return resp, err
}

type trigger struct {
	id            uint64
	cancel        context.CancelFunc
	subscriptions map[*Context]*sub
	// initialized is set to true when the trigger is started and initialized
	initialized bool
}

// workItem is used to encapsulate a function that needs to be
// executed in the worker goroutine. fn will be executed, and if
// final is true the worker will be stopped after fn is executed.
type workItem struct {
	fn    func()
	final bool
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
	workChan chan workItem
}

// startWorker runs in its own goroutine to process fetches and write data to the client synchronously
// it also takes care of sending heartbeats to the client but only if the subscription supports it
// TODO implement a goroutine pool that is sharded by the subscription id to avoid creating a new goroutine for each subscription
func (s *sub) startWorker() {
	if s.heartbeat {
		s.startWorkerWithHeartbeat()
		return
	}
	s.startWorkerWithoutHeartbeat()
}

// startWorkerWithHeartbeat is similar to startWorker but sends heartbeats to the client when enabled.
// It sends a heartbeat to the client every heartbeatInterval. Heartbeats are handled by the SubscriptionResponseWriter interface.
// TODO: Implement a shared timer implementation to avoid creating a new ticker for each subscription.
func (s *sub) startWorkerWithHeartbeat() {
	heartbeatTicker := time.NewTicker(s.resolver.heartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-s.ctx.ctx.Done():
			// Complete when the client request context is done for synchronous subscriptions
			s.close(SubscriptionCloseKindGoingAway)

			return
		case <-s.resolver.ctx.Done():
			// Abort immediately if the resolver is shutting down
			s.close(SubscriptionCloseKindGoingAway)

			return
		case <-heartbeatTicker.C:
			s.resolver.handleHeartbeat(s)
		case work := <-s.workChan:
			work.fn()

			if work.final {
				return
			}

			// Reset the heartbeat ticker after each write to avoid sending unnecessary heartbeats
			heartbeatTicker.Reset(s.resolver.heartbeatInterval)
		}
	}
}

func (s *sub) startWorkerWithoutHeartbeat() {
	for {
		select {
		case <-s.ctx.ctx.Done():
			// Complete when the client request context is done for synchronous subscriptions
			s.close(SubscriptionCloseKindGoingAway)

			return
		case <-s.resolver.ctx.Done():
			// Abort immediately if the resolver is shutting down
			s.close(SubscriptionCloseKindGoingAway)

			return
		case work := <-s.workChan:
			work.fn()

			if work.final {
				return
			}
		}
	}
}

// Called when subgraph indicates a "complete" subscription
func (s *sub) complete() {
	// The channel is used to communicate that the subscription is done
	// It is used only in the synchronous subscription case and to avoid sending events
	// to a subscription that is already done.
	defer close(s.completed)

	s.writer.Complete()
}

// Called when subgraph becomes unreachable or closes the connection without a "complete" event
func (s *sub) close(kind SubscriptionCloseKind) {
	// The channel is used to communicate that the subscription is done
	// It is used only in the synchronous subscription case and to avoid sending events
	// to a subscription that is already done.
	defer close(s.completed)

	s.writer.Close(kind)
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

	resolveArena := r.resolveArenaPool.Acquire(resolveCtx.Request.ID)
	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena)

	if err := t.resolvable.InitSubscription(resolveCtx, input, sub.resolve.Trigger.PostProcessing); err != nil {
		r.resolveArenaPool.Release(resolveArena)
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
		r.resolveArenaPool.Release(resolveArena)
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
		r.resolveArenaPool.Release(resolveArena)
		r.asyncErrorWriter.WriteError(resolveCtx, err, sub.resolve.Response, sub.writer)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:resolve:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}

	r.resolveArenaPool.Release(resolveArena)

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

	// events channel can't be closed here because producers are
	// sending events across multiple goroutines

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
// to avoid blocking the event loop and improve performance.
func (r *Resolver) handleEvent(event subscriptionEvent) {
	switch event.kind {
	case subscriptionEventKindAddSubscription:
		r.handleAddSubscription(event.triggerID, event.addSubscription)
	case subscriptionEventKindRemoveSubscription:
		r.handleRemoveSubscription(event.id)
	case subscriptionEventKindCompleteSubscription:
		r.handleCompleteSubscription(event.id)
	case subscriptionEventKindRemoveClient:
		r.handleRemoveClient(event.id.ConnectionID)
	case subscriptionEventKindTriggerUpdate:
		r.handleTriggerUpdate(event.triggerID, event.data)
	case subscriptionEventKindTriggerComplete:
		r.handleTriggerComplete(event.triggerID)
	case subscriptionEventKindTriggerInitialized:
		r.handleTriggerInitialized(event.triggerID)
	case subscriptionEventKindTriggerClose:
		r.handleTriggerClose(event)
	case subscriptionEventKindUnknown:
		panic("unknown event")
	}
}

// handleHeartbeat sends a heartbeat to the client. It needs to be executed on the same goroutine as the writer.
func (r *Resolver) handleHeartbeat(sub *sub) {
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

	if err := sub.writer.Heartbeat(); err != nil {
		// If heartbeat fails (e.g. client disconnected), remove the subscription.
		_ = r.AsyncUnsubscribeSubscription(sub.id)
		return
	}

	if r.options.Debug {
		fmt.Printf("resolver:heartbeat:subscription:done:%d\n", sub.id.SubscriptionID)
	}

	if r.reporter != nil {
		r.reporter.SubscriptionUpdateSent()
	}
}

func (r *Resolver) handleTriggerClose(s subscriptionEvent) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:%d:%d\n", s.triggerID, s.id.SubscriptionID)
	}

	r.closeTrigger(s.triggerID, s.closeKind)
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

func (r *Resolver) handleTriggerComplete(triggerID uint64) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:complete:%d\n", triggerID)
	}

	r.completeTrigger(triggerID)
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
		workChan:  make(chan workItem, 32),
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
			err = asyncDataSource.AsyncStart(cloneCtx, triggerID, add.headers, add.input, updater)
		} else {
			err = add.resolve.Trigger.Source.Start(cloneCtx, add.headers, add.input, updater)
		}
		if err != nil {
			if r.options.Debug {
				fmt.Printf("resolver:trigger:failed:%d\n", triggerID)
			}
			r.asyncErrorWriter.WriteError(add.ctx, err, add.resolve.Response, add.writer)
			_ = r.emitTriggerClose(triggerID)
			return
		}

		_ = r.emitTriggerInitialized(triggerID)

		if r.options.Debug {
			fmt.Printf("resolver:trigger:started:%d\n", triggerID)
		}
	}()

}

func (r *Resolver) emitTriggerClose(triggerID uint64) error {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:%d\n", triggerID)
	}

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		triggerID: triggerID,
		kind:      subscriptionEventKindTriggerClose,
		closeKind: SubscriptionCloseKindNormal,
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

func (r *Resolver) handleCompleteSubscription(id SubscriptionIdentifier) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:%d:%d\n", id.ConnectionID, id.SubscriptionID)
	}
	removed := 0
	for u := range r.triggers {
		trig := r.triggers[u]
		removed += r.completeTriggerSubscriptions(u, func(sID SubscriptionIdentifier) bool {
			return sID == id
		})
		if len(trig.subscriptions) == 0 {
			r.completeTrigger(trig.id)
		}
	}
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
	}
}

func (r *Resolver) handleRemoveSubscription(id SubscriptionIdentifier) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:%d:%d\n", id.ConnectionID, id.SubscriptionID)
	}
	removed := 0
	for u := range r.triggers {
		trig := r.triggers[u]
		removed += r.closeTriggerSubscriptions(u, SubscriptionCloseKindNormal, func(sID SubscriptionIdentifier) bool {
			return sID == id
		})
		if len(trig.subscriptions) == 0 {
			r.closeTrigger(trig.id, SubscriptionCloseKindNormal)
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
		removed += r.closeTriggerSubscriptions(u, SubscriptionCloseKindNormal, func(sID SubscriptionIdentifier) bool {
			return sID.ConnectionID == id
		})
		if len(r.triggers[u].subscriptions) == 0 {
			r.closeTrigger(r.triggers[u].id, SubscriptionCloseKindNormal)
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
		case s.workChan <- workItem{fn, false}:
			// Send the event to the subscription worker
		}
	}
}

func (r *Resolver) closeTrigger(id uint64, kind SubscriptionCloseKind) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:close:%d\n", id)
	}
	trig, ok := r.triggers[id]
	if !ok {
		return
	}

	removed := r.closeTriggerSubscriptions(id, kind, nil)

	// Cancels the async datasource and cleanup the connection
	trig.cancel()

	delete(r.triggers, id)

	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
		if trig.initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
}

func (r *Resolver) completeTrigger(id uint64) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:complete:%d\n", id)
	}

	trig, ok := r.triggers[id]
	if !ok {
		return
	}

	removed := r.completeTriggerSubscriptions(id, nil)

	// Cancels the async datasource and cleanup the connection
	trig.cancel()

	delete(r.triggers, id)

	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
		if trig.initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
}

func (r *Resolver) completeTriggerSubscriptions(id uint64, completeMatcher func(a SubscriptionIdentifier) bool) int {
	trig, ok := r.triggers[id]
	if !ok {
		return 0
	}
	removed := 0
	for c, s := range trig.subscriptions {
		if completeMatcher != nil && !completeMatcher(s.id) {
			continue
		}

		// Send a work item to complete the subscription
		s.workChan <- workItem{s.complete, true}

		// Because the event loop is single threaded, we can safely close the channel from this sender
		// The subscription worker will finish processing all events before the channel is closed.
		close(s.workChan)

		// Important because we remove the subscription from the trigger on the same goroutine
		// as we send work to the subscription worker. We can ensure that no new work is sent to the worker after this point.
		delete(trig.subscriptions, c)

		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:closed:%d:%d\n", trig.id, s.id.SubscriptionID)
		}

		removed++
	}
	return removed
}

func (r *Resolver) closeTriggerSubscriptions(id uint64, closeKind SubscriptionCloseKind, closeMatcher func(a SubscriptionIdentifier) bool) int {
	trig, ok := r.triggers[id]
	if !ok {
		return 0
	}
	removed := 0
	for c, s := range trig.subscriptions {
		if closeMatcher != nil && !closeMatcher(s.id) {
			continue
		}

		// Send a work item to close the subscription
		s.workChan <- workItem{func() { s.close(closeKind) }, true}

		// Because the event loop is single threaded, we can safely close the channel from this sender
		// The subscription worker will finish processing all events before the channel is closed.
		close(s.workChan)

		// Important because we remove the subscription from the trigger on the same goroutine
		// as we send work to the subscription worker. We can ensure that no new work is sent to the worker after this point.
		delete(trig.subscriptions, c)

		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:closed:%d:%d\n", trig.id, s.id.SubscriptionID)
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
		r.closeTrigger(id, SubscriptionCloseKindGoingAway)
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

func (r *Resolver) AsyncCompleteSubscription(id SubscriptionIdentifier) error {
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		id:   id,
		kind: subscriptionEventKindCompleteSubscription,
	}:
	}
	return nil
}

func (r *Resolver) AsyncUnsubscribeSubscription(id SubscriptionIdentifier) error {
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		id:   id,
		kind: subscriptionEventKindRemoveSubscription,
	}:
	default:
		// In the event we cannot insert immediately, defer insertion a goroutine, this should prevent deadlocks, at the cost of goroutine creation.
		go func() {
			select {
			case <-r.ctx.Done():
				return
			case r.events <- subscriptionEvent{
				id:   id,
				kind: subscriptionEventKindRemoveSubscription,
			}:
			}
		}()
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
	default:
		// In the event we cannot insert immediately, defer insertion a goroutine, this should prevent deadlocks, at the cost of goroutine creation.
		go func() {
			select {
			case <-r.ctx.Done():
				return
			case r.events <- subscriptionEvent{
				id: SubscriptionIdentifier{
					ConnectionID: connectionID,
				},
				kind: subscriptionEventKindRemoveClient,
			}:
			}
		}()
	}
	return nil
}

// prepareTrigger safely gets the headers for the trigger Subgraph and computes the hash across headers and input
// the generated has is the unique triggerID
// the headers must be forwarded to the DataSource to create the trigger
func (r *Resolver) prepareTrigger(ctx *Context, sourceName string, input []byte) (headers http.Header, triggerID uint64) {
	if ctx.SubgraphHeadersBuilder != nil {
		header, headerHash := ctx.SubgraphHeadersBuilder.HeadersForSubgraph(sourceName)
		keyGen := pool.Hash64.Get()
		_, _ = keyGen.Write(input)
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], headerHash)
		_, _ = keyGen.Write(b[:])
		triggerID = keyGen.Sum64()
		pool.Hash64.Put(keyGen)
		return header, triggerID
	}
	return nil, 0
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
		t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, nil)

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

	headers, triggerID := r.prepareTrigger(ctx, subscription.Trigger.SourceName, input)
	id := SubscriptionIdentifier{
		ConnectionID:   ConnectionIDs.Inc(),
		SubscriptionID: 0,
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscribe:sync:%d:%d\n", triggerID, id.SubscriptionID)
	}

	completed := make(chan struct{})

	select {
	case <-r.ctx.Done():
		// Stop processing if the resolver is shutting down
		return r.ctx.Err()
	case r.events <- subscriptionEvent{
		triggerID: triggerID,
		kind:      subscriptionEventKindAddSubscription,
		addSubscription: &addSubscription{
			ctx:        ctx,
			input:      input,
			resolve:    subscription,
			writer:     writer,
			id:         id,
			completed:  completed,
			sourceName: subscription.Trigger.SourceName,
			headers:    headers,
		},
	}:
	}

	// This will immediately block until one of the following conditions is met:
	select {
	case <-ctx.ctx.Done():
		// Client disconnected, request context canceled.
		// We will ignore the error and remove the subscription in the next step.

		select {
		case <-completed:
			// Wait for the subscription to be completed to avoid race conditions
			// with go sdk request shutdown.
		case <-r.ctx.Done():
			// Resolver shutdown, no way to gracefully shut down the subscription
			return r.ctx.Err()
		}
	case <-r.ctx.Done():
		// Resolver shutdown, no way to gracefully shut down the subscription
		// because the event loop is not running anymore and shutdown all triggers + subscriptions
		return r.ctx.Err()
	case <-completed:
	}

	if r.options.Debug {
		fmt.Printf("resolver:trigger:unsubscribe:sync:%d:%d\n", triggerID, id.SubscriptionID)
	}

	// Remove the subscription when the client disconnects.

	r.events <- subscriptionEvent{
		triggerID: triggerID,
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
		t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, nil)

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

	headers, triggerID := r.prepareTrigger(ctx, subscription.Trigger.SourceName, input)

	select {
	case <-r.ctx.Done():
		// Stop resolving if the resolver is shutting down
		return r.ctx.Err()
	case <-ctx.ctx.Done():
		// Stop resolving if the client is gone
		return ctx.ctx.Err()
	case r.events <- subscriptionEvent{
		triggerID: triggerID,
		kind:      subscriptionEventKindAddSubscription,
		addSubscription: &addSubscription{
			ctx:        ctx,
			input:      input,
			resolve:    subscription,
			writer:     writer,
			id:         id,
			completed:  make(chan struct{}),
			sourceName: subscription.Trigger.SourceName,
			headers:    headers,
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
		// Skip sending events if trigger is already done
		return
	case s.ch <- subscriptionEvent{
		triggerID: s.triggerID,
		kind:      subscriptionEventKindTriggerUpdate,
		data:      data,
	}:
	}
}

func (s *subscriptionUpdater) Complete() {
	if s.debug {
		fmt.Printf("resolver:subscription_updater:complete:%d\n", s.triggerID)
	}

	select {
	case <-s.ctx.Done():
		// Skip sending events if trigger is already done
		if s.debug {
			fmt.Printf("resolver:subscription_updater:complete:skip:%d\n", s.triggerID)
		}
		return
	case s.ch <- subscriptionEvent{
		triggerID: s.triggerID,
		kind:      subscriptionEventKindTriggerComplete,
	}:
		if s.debug {
			fmt.Printf("resolver:subscription_updater:complete:sent_event:%d\n", s.triggerID)
		}
	}
}

func (s *subscriptionUpdater) Close(kind SubscriptionCloseKind) {
	if s.debug {
		fmt.Printf("resolver:subscription_updater:close:%d\n", s.triggerID)
	}

	select {
	case <-s.ctx.Done():
		// Skip sending events if trigger is already done
		if s.debug {
			fmt.Printf("resolver:subscription_updater:close:skip:%d\n", s.triggerID)
		}
		return
	case s.ch <- subscriptionEvent{
		triggerID: s.triggerID,
		kind:      subscriptionEventKindTriggerClose,
		closeKind: kind,
	}:
		if s.debug {
			fmt.Printf("resolver:subscription_updater:close:sent_event:%d\n", s.triggerID)
		}
	}
}

type subscriptionEvent struct {
	triggerID       uint64
	id              SubscriptionIdentifier
	kind            subscriptionEventKind
	data            []byte
	addSubscription *addSubscription
	closeKind       SubscriptionCloseKind
}

type addSubscription struct {
	ctx        *Context
	input      []byte
	resolve    *GraphQLSubscription
	writer     SubscriptionResponseWriter
	id         SubscriptionIdentifier
	completed  chan struct{}
	sourceName string
	headers    http.Header
}

type subscriptionEventKind int

const (
	subscriptionEventKindUnknown subscriptionEventKind = iota
	subscriptionEventKindTriggerUpdate
	subscriptionEventKindTriggerComplete
	subscriptionEventKindAddSubscription
	subscriptionEventKindRemoveSubscription
	subscriptionEventKindCompleteSubscription
	subscriptionEventKindRemoveClient
	subscriptionEventKindTriggerInitialized
	subscriptionEventKindTriggerClose
)

type SubscriptionUpdater interface {
	// Update sends an update to the client. It is not guaranteed that the update is sent immediately.
	Update(data []byte)
	// Complete also takes care of cleaning up the trigger and all subscriptions. No more updates should be sent after calling Complete.
	Complete()
	// Close closes the subscription and cleans up the trigger and all subscriptions. No more updates should be sent after calling Close.
	Close(kind SubscriptionCloseKind)
}
