//go:generate mockgen -self_package=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource

package resolve

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"

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
var ConnectionIDs atomic.Int64

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

// Resolver manages GraphQL subscriptions using a mutex-protected trigger registry.
// All trigger/subscription state is guarded by mu. Long-running I/O (writes to clients)
// is performed outside the lock using a snapshot-and-release pattern.
type Resolver struct {
	ctx            context.Context
	options        ResolverOptions
	maxConcurrency chan struct{}

	mu                        sync.Mutex
	shutdown                  bool
	triggers                  map[uint64]*trigger
	subscriptionsByID         map[SubscriptionIdentifier]*subscriptionState
	subscriptionsByConnection map[int64]map[SubscriptionIdentifier]*subscriptionState

	allowedErrorExtensionFields map[string]struct{}
	allowedErrorFields          map[string]struct{}

	reporter         Reporter
	asyncErrorWriter AsyncErrorWriter

	propagateSubgraphErrors      bool
	propagateSubgraphStatusCodes bool
	// Subscription heartbeat interval for periodic updater heartbeats.
	heartbeatInterval time.Duration
	// maxSubscriptionFetchTimeout defines the maximum time a subscription fetch can take before it is considered timed out
	maxSubscriptionFetchTimeout time.Duration

	// resolveArenaPool is the arena pool dedicated for Loader & Resolvable.
	// ArenaPool automatically adjusts arena buffer sizes per workload.
	// Resolving & response buffering are very different tasks;
	// as such, it was best to have two arena pools in terms of memory usage.
	// A single pool for both was much less efficient.
	resolveArenaPool *arena.Pool
	// responseBufferPool is the arena pool dedicated for response buffering before sending to the client
	responseBufferPool *arena.Pool

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

	// SubgraphRequestDeduplicationShardCount defines the number of shards to use for subgraph request deduplication
	SubgraphRequestDeduplicationShardCount int
	// InboundRequestDeduplicationShardCount defines the number of shards to use for inbound request deduplication
	InboundRequestDeduplicationShardCount int
	// SetDeduplicationShardCountToGOMAXPROCS sets SubgraphRequestDeduplicationShardCount and InboundRequestDeduplicationShardCount to runtime.GOMAXPROCS(0)
	// and will override any values set for those options
	// using runtime.GOMAXPROCS(0) allows the deduplication to scale with the CPU resources available to the process
	SetDeduplicationShardCountToGOMAXPROCS bool
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
		allowedErrorFields[locationsField] = struct{}{}
	}

	for _, field := range options.AllowedSubgraphErrorFields {
		allowedErrorFields[field] = struct{}{}
	}

	if options.SubgraphRequestDeduplicationShardCount <= 0 {
		options.SubgraphRequestDeduplicationShardCount = 8
	}

	if options.InboundRequestDeduplicationShardCount <= 0 {
		options.InboundRequestDeduplicationShardCount = 8
	}

	if options.SetDeduplicationShardCountToGOMAXPROCS {
		/*
			runtime.GOMAXPROCS(0) returns the current value without changing it
			This is the effective CPU limit for Go scheduling
			Since Go 1.20+, this respects:
				- cgroup CPU quotas (Docker, Kubernetes)
				- cpuset constraints

			Setting shard counts to GOMAXPROCS helps allows us to scale deduplication across available CPU resources
		*/
		n := runtime.GOMAXPROCS(0)
		options.SubgraphRequestDeduplicationShardCount = n
		options.InboundRequestDeduplicationShardCount = n
	}

	resolver := &Resolver{
		ctx:                          ctx,
		options:                      options,
		propagateSubgraphErrors:      options.PropagateSubgraphErrors,
		propagateSubgraphStatusCodes: options.PropagateSubgraphStatusCodes,
		triggers:                     make(map[uint64]*trigger),
		subscriptionsByID:            make(map[SubscriptionIdentifier]*subscriptionState),
		subscriptionsByConnection:    make(map[int64]map[SubscriptionIdentifier]*subscriptionState),
		reporter:                     options.Reporter,
		asyncErrorWriter:             options.AsyncErrorWriter,
		allowedErrorExtensionFields:  allowedExtensionFields,
		allowedErrorFields:           allowedErrorFields,
		heartbeatInterval:            options.SubscriptionHeartbeatInterval,
		maxSubscriptionFetchTimeout:  options.MaxSubscriptionFetchTimeout,
		resolveArenaPool:             arena.NewArenaPool(),
		responseBufferPool:           arena.NewArenaPool(),
		subgraphRequestSingleFlight:  NewSingleFlight(options.SubgraphRequestDeduplicationShardCount),
		inboundRequestSingleFlight:   NewRequestSingleFlight(options.InboundRequestDeduplicationShardCount),
	}
	resolver.maxConcurrency = make(chan struct{}, options.MaxConcurrency)
	for i := 0; i < options.MaxConcurrency; i++ {
		resolver.maxConcurrency <- struct{}{}
	}

	go resolver.heartbeatLoop()
	context.AfterFunc(resolver.ctx, func() {
		resolver.shutdownResolver()
	})

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
			singleFlight:                                 sf,
			jsonArena:                                    a,
		},
	}
}

type GraphQLResolveInfo struct {
	// ResolveAcquireWaitTime is the time spent waiting to acquire the resolver semaphore
	// the semaphore limits the number of concurrent resolve operations
	ResolveAcquireWaitTime time.Duration

	// ResolveDeduplicated indicates whether the resolution of the entire operation was deduplicated via single flight
	ResolveDeduplicated bool
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

	ctx.ActualListSizes = t.resolvable.actualListSizes

	return resp, err
}

func (r *Resolver) ArenaResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, writer io.Writer) (*GraphQLResolveInfo, error) {
	resp := &GraphQLResolveInfo{}

	inflight, err := r.inboundRequestSingleFlight.GetOrCreate(ctx, response)
	if err != nil {
		return nil, err
	}

	if inflight != nil && inflight.Data != nil { // follower
		resp.ResolveDeduplicated = true
		// Apply the leader's shared state (e.g. response headers) to this follower's context
		// before writing the response, so the response writer can propagate headers correctly.
		if ctx.SetDeduplicationData != nil && inflight.SharedData != nil {
			ctx.SetDeduplicationData(ctx.ctx, inflight.SharedData)
		}
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
	ctx.ActualListSizes = t.resolvable.actualListSizes

	// first release resolverArena
	// all data is resolved and written into the response arena
	r.resolveArenaPool.Release(resolveArena)
	// next we write back to the client
	// this includes flushing and syscalls
	// as such, it can take some time
	// which is why we split the arenas and released the first one
	_, err = writer.Write(buf.Bytes())
	// Extract data from the leader's context to share with singleflight followers.
	// This runs after the leader has fully resolved and written its response, so all
	// subgraph response headers have been accumulated on the leader's context.
	// SharedData MUST be set BEFORE FinishOk, which closes the Done channel and
	// unblocks followers. Otherwise followers could read SharedData before it is set.
	if inflight != nil && ctx.GetDeduplicationData != nil {
		inflight.SharedData = ctx.GetDeduplicationData(ctx.ctx)
	}
	r.inboundRequestSingleFlight.FinishOk(inflight, buf.Bytes())
	// all data is written to the client
	// we're safe to release our buffer
	r.responseBufferPool.Release(responseArena)
	return resp, err
}

type trigger struct {
	mu            sync.RWMutex
	id            uint64
	cancel        context.CancelFunc
	subscriptions map[SubscriptionIdentifier]*subscriptionState
	updateBuf     *bytes.Buffer
	// initialized is set to true when the trigger is started and initialized
	initialized bool
	updater     *subscriptionUpdater
}

func (t *trigger) subscriptionIds() map[context.Context]SubscriptionIdentifier {
	t.mu.RLock()
	defer t.mu.RUnlock()

	subs := make(map[context.Context]SubscriptionIdentifier, len(t.subscriptions))
	for _, sub := range t.subscriptions {
		subs[sub.ctx.Context()] = sub.id
	}
	return subs
}

type subscriptionState struct {
	triggerID uint64
	resolve   *GraphQLSubscription
	ctx       *Context
	writer    SubscriptionResponseWriter
	id        SubscriptionIdentifier
	heartbeat bool
	completed chan struct{}
	writeMu   sync.Mutex
	// removed guards against writes after the subscription has been removed.
	// Uses CompareAndSwap to prevent double-close of the completed channel.
	removed atomic.Bool
	// lastWriteTime stores unix nanos of the last successful data write.
	lastWriteTime atomic.Int64
}

type subscriptionFinalizer struct {
	sub       *subscriptionState
	complete  bool
	closeKind SubscriptionCloseKind
}

func runSubscriptionFinalizers(finalizers []subscriptionFinalizer) {
	for _, f := range finalizers {
		if f.complete {
			f.sub.complete()
		} else {
			f.sub.close(f.closeKind)
		}
	}
}

// Called when subgraph indicates a "complete" subscription
func (s *subscriptionState) complete() {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// The channel is used to communicate that the subscription is done
	// It is used only in the synchronous subscription case and to avoid sending events
	// to a subscription that is already done.
	defer close(s.completed)

	s.writer.Complete()
}

// Called when subgraph becomes unreachable or closes the connection without a "complete" event
func (s *subscriptionState) close(kind SubscriptionCloseKind) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// The channel is used to communicate that the subscription is done
	// It is used only in the synchronous subscription case and to avoid sending events
	// to a subscription that is already done.
	defer close(s.completed)

	s.writer.Close(kind)
}

func (r *Resolver) executeSubscriptionUpdate(resolveCtx *Context, sub *subscriptionState, sharedInput []byte) {
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
		sub.writeMu.Lock()
		if !sub.removed.Load() {
			r.asyncErrorWriter.WriteError(resolveCtx, err, sub.resolve.Response, sub.writer)
		}
		sub.writeMu.Unlock()
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
		sub.writeMu.Lock()
		if !sub.removed.Load() {
			r.asyncErrorWriter.WriteError(resolveCtx, err, sub.resolve.Response, sub.writer)
		}
		sub.writeMu.Unlock()
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:load:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}

	sub.writeMu.Lock()
	if sub.removed.Load() {
		sub.writeMu.Unlock()
		r.resolveArenaPool.Release(resolveArena)
		return
	}

	if err := t.resolvable.Resolve(resolveCtx.ctx, sub.resolve.Response.Data, sub.resolve.Response.Fetches, sub.writer); err != nil {
		r.resolveArenaPool.Release(resolveArena)
		r.asyncErrorWriter.WriteError(resolveCtx, err, sub.resolve.Response, sub.writer)
		sub.writeMu.Unlock()
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
		sub.writeMu.Unlock()
		// If flush fails (e.g. client disconnected), remove the subscription.
		_ = r.UnsubscribeSubscription(sub.id)
		return
	}
	sub.lastWriteTime.Store(time.Now().UnixNano())
	sub.writeMu.Unlock()

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

func (r *Resolver) executeSubscriptionHeartbeat(sub *subscriptionState) {
	if r.options.Debug {
		fmt.Printf("resolver:heartbeat:subscription:%d\n", sub.id.SubscriptionID)
	}

	if r.ctx.Err() != nil || sub.ctx.Context().Err() != nil {
		return
	}

	sub.writeMu.Lock()

	if sub.removed.Load() {
		sub.writeMu.Unlock()
		return
	}

	if err := sub.writer.Heartbeat(); err != nil {
		sub.writeMu.Unlock()
		_ = r.UnsubscribeSubscription(sub.id)
		return
	}
	sub.writeMu.Unlock()

	if r.reporter != nil {
		r.reporter.SubscriptionUpdateSent()
	}
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

type StartupHookContext struct {
	Context context.Context
	Updater func(data []byte)
}

func (r *Resolver) executeStartupHooks(add *addSubscription, updater *subscriptionUpdater) error {
	hook, ok := add.resolve.Trigger.Source.(HookableSubscriptionDataSource)
	if ok {
		hookCtx := StartupHookContext{
			Context: add.ctx.Context(),
			Updater: func(data []byte) {
				updater.UpdateSubscription(add.id, data)
			},
		}

		err := hook.SubscriptionOnStart(hookCtx, add.input)
		if err != nil {
			if r.options.Debug {
				fmt.Printf("resolver:trigger:subscription:startup:failed:%d\n", add.id.SubscriptionID)
			}
			r.asyncErrorWriter.WriteError(add.ctx, err, add.resolve.Response, add.writer)
			_ = r.UnsubscribeSubscription(add.id)
			return err
		}
	}
	return nil
}

func (r *Resolver) watchSubscriptionCancellation(ctx context.Context, id SubscriptionIdentifier, completed <-chan struct{}) {
	go func() {
		select {
		case <-ctx.Done():
			_ = r.UnsubscribeSubscription(id)
		case <-completed:
		case <-r.ctx.Done():
		}
	}()
}

func (r *Resolver) addSubscriptionIndex(s *subscriptionState) {
	id := s.id
	r.subscriptionsByID[id] = s
	byConn, ok := r.subscriptionsByConnection[id.ConnectionID]
	if !ok {
		byConn = make(map[SubscriptionIdentifier]*subscriptionState)
		r.subscriptionsByConnection[id.ConnectionID] = byConn
	}
	byConn[id] = s
}

func (r *Resolver) removeSubscriptionIndex(id SubscriptionIdentifier) {
	delete(r.subscriptionsByID, id)
	byConn, ok := r.subscriptionsByConnection[id.ConnectionID]
	if !ok {
		return
	}
	delete(byConn, id)
	if len(byConn) == 0 {
		delete(r.subscriptionsByConnection, id.ConnectionID)
	}
}

// handleAddSubscription must be called with r.mu held.
func (r *Resolver) handleAddSubscription(triggerID uint64, add *addSubscription) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:add:%d:%d\n", triggerID, add.id.SubscriptionID)
	}
	s := &subscriptionState{
		triggerID: triggerID,
		ctx:       add.ctx,
		resolve:   add.resolve,
		writer:    add.writer,
		id:        add.id,
		completed: add.completed,
	}
	if add.ctx.ExecutionOptions.SendHeartbeat {
		s.heartbeat = true
	}

	trig, ok := r.triggers[triggerID]
	if ok {
		if r.reporter != nil {
			r.reporter.SubscriptionCountInc(1)
		}
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:added:%d:%d\n", triggerID, add.id.SubscriptionID)
		}
		// Add the subscription to the registry so it can receive events
		trig.mu.Lock()
		trig.subscriptions[add.id] = s
		trig.mu.Unlock()
		r.addSubscriptionIndex(s)
		r.watchSubscriptionCancellation(add.ctx.Context(), add.id, add.completed)
		// Execute the startup hooks in a goroutine to avoid holding the lock
		go func() {
			_ = r.executeStartupHooks(add, trig.updater)
		}()
		return
	}

	if r.options.Debug {
		fmt.Printf("resolver:create:trigger:%d\n", triggerID)
	}
	ctx, cancel := context.WithCancel(xcontext.Detach(add.ctx.Context()))
	updater := &subscriptionUpdater{
		debug:     r.options.Debug,
		triggerID: triggerID,
		resolver:  r,
		ctx:       ctx,
	}
	cloneCtx := add.ctx.clone(ctx)
	trig = &trigger{
		id:            triggerID,
		subscriptions: make(map[SubscriptionIdentifier]*subscriptionState),
		updateBuf:     bytes.NewBuffer(make([]byte, 0, 1024)),
		cancel:        cancel,
		updater:       updater,
	}
	r.triggers[triggerID] = trig
	trig.mu.Lock()
	trig.subscriptions[add.id] = s
	trig.mu.Unlock()
	updater.subsFn = trig.subscriptionIds
	r.addSubscriptionIndex(s)
	r.watchSubscriptionCancellation(add.ctx.Context(), add.id, add.completed)

	if r.reporter != nil {
		r.reporter.SubscriptionCountInc(1)
	}

	go func() {
		if r.options.Debug {
			fmt.Printf("resolver:trigger:start:%d\n", triggerID)
		}

		// This is blocking so the startup hook can decide if a subscription should be started or not by returning an error
		err := r.executeStartupHooks(add, trig.updater)
		if err != nil {
			return
		}

		err = add.resolve.Trigger.Source.Start(cloneCtx, add.headers, add.input, trig.updater)
		if err != nil {
			if r.options.Debug {
				fmt.Printf("resolver:trigger:failed:%d\n", triggerID)
			}
			r.asyncErrorWriter.WriteError(add.ctx, err, add.resolve.Response, add.writer)
			r.closeTriggerFromUpdater(triggerID, SubscriptionCloseKindNormal)
			return
		}

		r.markTriggerInitialized(triggerID)

		if r.options.Debug {
			fmt.Printf("resolver:trigger:started:%d\n", triggerID)
		}
	}()
}

// markTriggerInitialized marks a trigger as initialized under the lock.
func (r *Resolver) markTriggerInitialized(triggerID uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handleTriggerInitialized(triggerID)
}

// closeTriggerFromUpdater closes a trigger from a datasource/updater goroutine.
func (r *Resolver) closeTriggerFromUpdater(triggerID uint64, kind SubscriptionCloseKind) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:%d\n", triggerID)
	}
	r.mu.Lock()
	removed, finalizers, cancel, initialized := r.detachTriggerLocked(triggerID, false, kind)
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
		if initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
	r.mu.Unlock()
	runSubscriptionFinalizers(finalizers)
	if cancel != nil {
		cancel()
	}
}

// completeTriggerFromUpdater completes a trigger from a datasource/updater goroutine.
func (r *Resolver) completeTriggerFromUpdater(triggerID uint64) {
	r.mu.Lock()
	removed, finalizers, cancel, initialized := r.detachTriggerLocked(triggerID, true, SubscriptionCloseKindNormal)
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
		if initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
	r.mu.Unlock()
	runSubscriptionFinalizers(finalizers)
	if cancel != nil {
		cancel()
	}
}

func (r *Resolver) handleCompleteSubscription(id SubscriptionIdentifier) (int, []subscriptionFinalizer, context.CancelFunc, bool) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:%d:%d\n", id.ConnectionID, id.SubscriptionID)
	}
	return r.removeSubscriptionByID(id, true, SubscriptionCloseKindNormal)
}

func (r *Resolver) handleRemoveSubscription(id SubscriptionIdentifier) (int, []subscriptionFinalizer, context.CancelFunc, bool) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:%d:%d\n", id.ConnectionID, id.SubscriptionID)
	}
	return r.removeSubscriptionByID(id, false, SubscriptionCloseKindNormal)
}

func (r *Resolver) handleRemoveClient(id int64, closeKind SubscriptionCloseKind) (int, []subscriptionFinalizer, []context.CancelFunc, int) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:client:%d\n", id)
	}
	removed := 0
	finalizers := make([]subscriptionFinalizer, 0)
	cancels := make([]context.CancelFunc, 0)
	triggerDec := 0
	idsForConn := r.subscriptionsByConnection[id]
	ids := make([]SubscriptionIdentifier, 0, len(idsForConn))
	for sid := range idsForConn {
		ids = append(ids, sid)
	}
	for _, sid := range ids {
		rem, fz, cancel, initialized := r.removeSubscriptionByID(sid, false, closeKind)
		removed += rem
		finalizers = append(finalizers, fz...)
		if cancel != nil {
			cancels = append(cancels, cancel)
			if initialized {
				triggerDec++
			}
		}
	}
	return removed, finalizers, cancels, triggerDec
}

// removeSubscriptionByID removes a single subscription by id and optionally completes it.
// r.mu must be held by the caller.
func (r *Resolver) removeSubscriptionByID(id SubscriptionIdentifier, complete bool, closeKind SubscriptionCloseKind) (int, []subscriptionFinalizer, context.CancelFunc, bool) {
	s, ok := r.subscriptionsByID[id]
	if !ok {
		return 0, nil, nil, false
	}

	trig, ok := r.triggers[s.triggerID]
	if !ok {
		r.removeSubscriptionIndex(id)
		return 0, nil, nil, false
	}

	trig.mu.Lock()
	_, ok = trig.subscriptions[id]
	if !ok {
		trig.mu.Unlock()
		r.removeSubscriptionIndex(id)
		return 0, nil, nil, false
	}

	var finalizers []subscriptionFinalizer
	if s.removed.CompareAndSwap(false, true) {
		finalizers = append(finalizers, subscriptionFinalizer{
			sub:       s,
			complete:  complete,
			closeKind: closeKind,
		})
	}
	delete(trig.subscriptions, id)
	empty := len(trig.subscriptions) == 0
	trig.mu.Unlock()

	r.removeSubscriptionIndex(id)

	var cancel context.CancelFunc
	initialized := false
	if empty {
		delete(r.triggers, trig.id)
		cancel = trig.cancel
		initialized = trig.initialized
	}

	return 1, finalizers, cancel, initialized
}

// detachTriggerLocked removes all subscriptions for the trigger and removes the trigger from resolver maps.
// r.mu must be held by the caller.
func (r *Resolver) detachTriggerLocked(id uint64, complete bool, closeKind SubscriptionCloseKind) (int, []subscriptionFinalizer, context.CancelFunc, bool) {
	trig, ok := r.triggers[id]
	if !ok {
		return 0, nil, nil, false
	}

	finalizers := make([]subscriptionFinalizer, 0, len(trig.subscriptions))
	removed := 0

	trig.mu.Lock()
	for sid, s := range trig.subscriptions {
		if s.removed.CompareAndSwap(false, true) {
			finalizers = append(finalizers, subscriptionFinalizer{
				sub:       s,
				complete:  complete,
				closeKind: closeKind,
			})
		}
		delete(trig.subscriptions, sid)
		r.removeSubscriptionIndex(sid)
		removed++
	}
	trig.mu.Unlock()

	delete(r.triggers, id)

	return removed, finalizers, trig.cancel, trig.initialized
}

// pendingWrite holds the context and subscription for a deferred write outside the lock.
type pendingSubscriptionWrite struct {
	sub *subscriptionState
}

// handleTriggerUpdate sends data to all subscriptions of a trigger using snapshot-and-release.
// The lock is released before performing I/O to avoid deadlocks when executeSubscriptionUpdate
// calls AsyncUnsubscribeSubscription on flush failure.
func (r *Resolver) handleTriggerUpdate(id uint64, data []byte) {
	r.mu.Lock()
	trig, ok := r.triggers[id]
	r.mu.Unlock()
	if !ok {
		return
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:update:%d\n", id)
	}

	var pending []pendingSubscriptionWrite
	trig.mu.Lock()
	for _, s := range trig.subscriptions {
		if s.ctx.ctx.Err() != nil {
			continue
		}
		skip, err := s.resolve.Filter.SkipEvent(s.ctx, data, trig.updateBuf)
		if err != nil {
			r.asyncErrorWriter.WriteError(s.ctx, err, s.resolve.Response, s.writer)
			continue
		}
		if skip {
			continue
		}
		pending = append(pending, pendingSubscriptionWrite{s})
	}
	trig.mu.Unlock()

	var wg sync.WaitGroup
	for _, pw := range pending {
		if pw.sub.removed.Load() {
			continue
		}
		wg.Go(func() {
			r.executeSubscriptionUpdate(pw.sub.ctx, pw.sub, data)
		})
	}
	wg.Wait()
}

// handleUpdateSubscription sends data to a single subscription using snapshot-and-release.
func (r *Resolver) handleUpdateSubscription(id uint64, data []byte, subIdentifier SubscriptionIdentifier) {
	r.mu.Lock()
	trig, ok := r.triggers[id]
	r.mu.Unlock()
	if !ok {
		return
	}

	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:update:%d:%d,%d\n", id, subIdentifier.ConnectionID, subIdentifier.SubscriptionID)
	}

	var target *subscriptionState
	trig.mu.Lock()
	s, ok := trig.subscriptions[subIdentifier]
	if ok {
		if s.ctx.ctx.Err() == nil {
			skip, err := s.resolve.Filter.SkipEvent(s.ctx, data, trig.updateBuf)
			if err != nil {
				r.asyncErrorWriter.WriteError(s.ctx, err, s.resolve.Response, s.writer)
			} else if !skip {
				target = s
			}
		}
	}
	trig.mu.Unlock()

	if target != nil && !target.removed.Load() {
		r.executeSubscriptionUpdate(target.ctx, target, data)
	}
}

func (r *Resolver) heartbeatTriggerSubscriptions(id uint64) {
	r.mu.Lock()
	trig, ok := r.triggers[id]
	r.mu.Unlock()
	if !ok {
		return
	}

	targets := make([]*subscriptionState, 0, len(trig.subscriptions))
	trig.mu.RLock()
	for _, s := range trig.subscriptions {
		if !s.heartbeat || s.removed.Load() {
			continue
		}
		if time.Since(time.Unix(0, s.lastWriteTime.Load())) < r.heartbeatInterval {
			continue
		}
		targets = append(targets, s)
	}
	trig.mu.RUnlock()

	for _, s := range targets {
		r.executeSubscriptionHeartbeat(s)
	}
}

func (r *Resolver) shutdownResolver() {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown\n")
	}
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return
	}

	r.shutdown = true
	triggerIDs := make([]uint64, 0, len(r.triggers))
	for id := range r.triggers {
		triggerIDs = append(triggerIDs, id)
	}

	allFinalizers := make([]subscriptionFinalizer, 0)
	cancels := make([]context.CancelFunc, 0, len(triggerIDs))
	removedTotal := 0
	triggerDec := 0

	for _, id := range triggerIDs {
		removed, finalizers, cancel, initialized := r.detachTriggerLocked(id, false, SubscriptionCloseKindGoingAway)
		removedTotal += removed
		allFinalizers = append(allFinalizers, finalizers...)
		if cancel != nil {
			cancels = append(cancels, cancel)
		}
		if initialized {
			triggerDec++
		}
	}

	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removedTotal)
		if triggerDec > 0 {
			r.reporter.TriggerCountDec(triggerDec)
		}
	}

	r.triggers = make(map[uint64]*trigger)
	r.subscriptionsByID = make(map[SubscriptionIdentifier]*subscriptionState)
	r.subscriptionsByConnection = make(map[int64]map[SubscriptionIdentifier]*subscriptionState)
	r.mu.Unlock()

	runSubscriptionFinalizers(allFinalizers)
	for _, cancel := range cancels {
		cancel()
	}

	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:done\n")
	}
}

func (r *Resolver) heartbeatLoop() {
	ticker := time.NewTicker(r.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.sendTriggerHeartbeats()
		}
	}
}

func (r *Resolver) sendTriggerHeartbeats() {
	r.mu.Lock()
	triggerIDs := make([]uint64, 0, len(r.triggers))
	for id := range r.triggers {
		triggerIDs = append(triggerIDs, id)
	}
	r.mu.Unlock()

	for _, id := range triggerIDs {
		r.heartbeatTriggerSubscriptions(id)
	}
}

type SubscriptionIdentifier struct {
	ConnectionID   int64
	SubscriptionID int64
}

func (r *Resolver) CompleteSubscription(id SubscriptionIdentifier) error {
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return r.ctx.Err()
	}
	removed, finalizers, cancel, initialized := r.handleCompleteSubscription(id)
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
		if cancel != nil && initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
	r.mu.Unlock()
	runSubscriptionFinalizers(finalizers)
	if cancel != nil {
		cancel()
	}
	return nil
}

func (r *Resolver) UnsubscribeSubscription(id SubscriptionIdentifier) error {
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return r.ctx.Err()
	}
	removed, finalizers, cancel, initialized := r.handleRemoveSubscription(id)
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
		if cancel != nil && initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
	r.mu.Unlock()
	runSubscriptionFinalizers(finalizers)
	if cancel != nil {
		cancel()
	}
	return nil
}

func (r *Resolver) UnsubscribeClient(connectionID int64) error {
	return r.UnsubscribeClientWithReason(connectionID, SubscriptionCloseKindNormal)
}

func (r *Resolver) UnsubscribeClientWithReason(connectionID int64, closeKind SubscriptionCloseKind) error {
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return r.ctx.Err()
	}
	removed, finalizers, cancels, triggerDec := r.handleRemoveClient(connectionID, closeKind)
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(removed)
		if triggerDec > 0 {
			r.reporter.TriggerCountDec(triggerDec)
		}
	}
	r.mu.Unlock()
	runSubscriptionFinalizers(finalizers)
	for _, cancel := range cancels {
		cancel()
	}
	return nil
}

// prepareTrigger safely gets the headers for the trigger Subgraph and computes the hash across headers and input
// the generated hash is the unique triggerID
// the headers must be forwarded to the DataSource to create the trigger
func (r *Resolver) prepareTrigger(ctx *Context, sourceName string, input []byte) (headers http.Header, triggerID uint64) {
	keyGen := pool.Hash64.Get()
	_, _ = keyGen.Write(input)
	if ctx.SubgraphHeadersBuilder != nil {
		var headersHash uint64
		headers, headersHash = ctx.SubgraphHeadersBuilder.HeadersForSubgraph(sourceName)
		if headersHash != 0 {
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:], headersHash)
			_, _ = keyGen.Write(b[:])
		}
	}
	triggerID = keyGen.Sum64()
	pool.Hash64.Put(keyGen)
	return headers, triggerID
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
		ConnectionID:   ConnectionIDs.Add(1),
		SubscriptionID: 0,
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscribe:sync:%d:%d\n", triggerID, id.SubscriptionID)
	}

	completed := make(chan struct{})

	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return r.ctx.Err()
	}
	r.handleAddSubscription(triggerID, &addSubscription{
		ctx:        ctx,
		input:      input,
		resolve:    subscription,
		writer:     writer,
		id:         id,
		completed:  completed,
		sourceName: subscription.Trigger.SourceName,
		headers:    headers,
	})
	r.mu.Unlock()

	// This will immediately block until one of the following conditions is met:
	select {
	case <-ctx.ctx.Done():
		// Client disconnected, request context canceled.
		select {
		case <-completed:
			// Wait for the subscription to be completed to avoid race conditions
			// with go sdk request shutdown.
		case <-r.ctx.Done():
			// Resolver shutdown
			return r.ctx.Err()
		}
	case <-r.ctx.Done():
		// Resolver shutdown
		return r.ctx.Err()
	case <-completed:
	}

	if r.options.Debug {
		fmt.Printf("resolver:trigger:unsubscribe:sync:%d:%d\n", triggerID, id.SubscriptionID)
	}

	// Remove the subscription when the client disconnects.
	_ = r.UnsubscribeSubscription(id)

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

	if err := ctx.ctx.Err(); err != nil {
		return err
	}

	headers, triggerID := r.prepareTrigger(ctx, subscription.Trigger.SourceName, input)

	r.mu.Lock()
	if err := ctx.ctx.Err(); err != nil {
		r.mu.Unlock()
		return err
	}
	if r.shutdown {
		r.mu.Unlock()
		return r.ctx.Err()
	}
	r.handleAddSubscription(triggerID, &addSubscription{
		ctx:        ctx,
		input:      input,
		resolve:    subscription,
		writer:     writer,
		id:         id,
		completed:  make(chan struct{}),
		sourceName: subscription.Trigger.SourceName,
		headers:    headers,
	})
	r.mu.Unlock()
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
	resolver  *Resolver
	ctx       context.Context
	subsFn    func() map[context.Context]SubscriptionIdentifier
}

func (s *subscriptionUpdater) Update(data []byte) {
	if s.ctx.Err() != nil {
		return
	}
	if s.debug {
		fmt.Printf("resolver:subscription_updater:update:%d\n", s.triggerID)
	}
	s.resolver.handleTriggerUpdate(s.triggerID, data)
}

func (s *subscriptionUpdater) Heartbeat() {
	if s.ctx.Err() != nil {
		return
	}
	s.resolver.heartbeatTriggerSubscriptions(s.triggerID)
}

func (s *subscriptionUpdater) UpdateSubscription(id SubscriptionIdentifier, data []byte) {
	if s.ctx.Err() != nil {
		return
	}
	if s.debug {
		fmt.Printf("resolver:subscription_updater:update:%d\n", s.triggerID)
	}
	s.resolver.handleUpdateSubscription(s.triggerID, data, id)
}

func (s *subscriptionUpdater) Subscriptions() map[context.Context]SubscriptionIdentifier {
	return s.subsFn()
}

func (s *subscriptionUpdater) Complete() {
	if s.ctx.Err() != nil {
		if s.debug {
			fmt.Printf("resolver:subscription_updater:complete:skip:%d\n", s.triggerID)
		}
		return
	}
	if s.debug {
		fmt.Printf("resolver:subscription_updater:complete:%d\n", s.triggerID)
	}
	s.resolver.completeTriggerFromUpdater(s.triggerID)
}

func (s *subscriptionUpdater) Close(kind SubscriptionCloseKind) {
	if s.ctx.Err() != nil {
		if s.debug {
			fmt.Printf("resolver:subscription_updater:close:skip:%d\n", s.triggerID)
		}
		return
	}
	if s.debug {
		fmt.Printf("resolver:subscription_updater:close:%d\n", s.triggerID)
	}
	s.resolver.closeTriggerFromUpdater(s.triggerID, kind)
}

func (s *subscriptionUpdater) CloseSubscription(kind SubscriptionCloseKind, id SubscriptionIdentifier) {
	if s.ctx.Err() != nil {
		if s.debug {
			fmt.Printf("resolver:subscription_updater:close:skip:%d\n", s.triggerID)
		}
		return
	}
	if s.debug {
		fmt.Printf("resolver:subscription_updater:close:%d\n", s.triggerID)
	}
	_ = s.resolver.UnsubscribeSubscription(id)
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

type SubscriptionUpdater interface {
	// Update sends an update to the client. It is not guaranteed that the update is sent immediately.
	Update(data []byte)
	// UpdateSubscription sends an update to a single subscription. It is not guaranteed that the update is sent immediately.
	UpdateSubscription(id SubscriptionIdentifier, data []byte)
	// Complete also takes care of cleaning up the trigger and all subscriptions. No more updates should be sent after calling Complete.
	Complete()
	// Close closes the subscription and cleans up the trigger and all subscriptions. No more updates should be sent after calling Close.
	Close(kind SubscriptionCloseKind)
	// CloseSubscription closes a single subscription. No more updates should be sent to that subscription after calling CloseSubscription.
	CloseSubscription(kind SubscriptionCloseKind, id SubscriptionIdentifier)
	// Subscriptions return all the subscriptions associated to this Updater
	Subscriptions() map[context.Context]SubscriptionIdentifier
}
