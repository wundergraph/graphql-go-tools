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
	"golang.org/x/sync/errgroup"

	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/errorcodes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/xcontext"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	DefaultHeartbeatInterval = 5 * time.Second
)

// ConnectionID identifies a client connection for subscription routing.
type ConnectionID int64

// connectionIDCounter is the monotonic counter backing NewConnectionID.
var connectionIDCounter atomic.Int64

// NewConnectionID returns a unique ConnectionID via an atomic increment.
func NewConnectionID() ConnectionID {
	return ConnectionID(connectionIDCounter.Add(1))
}

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

	// mu protects: shutdown, triggers, subscriptionsByID, subscriptionsByConnection.
	// Lock ordering: subscriptionUpdater.mu > Resolver.mu > trigger.mu (then subscriptionState.writeMu outside those locks).
	mu                        sync.Mutex
	shutdown                  bool
	triggers                  map[uint64]*trigger
	subscriptionsByID         map[SubscriptionIdentifier]*subscriptionState
	subscriptionsByConnection map[ConnectionID]map[SubscriptionIdentifier]*subscriptionState

	allowedErrorExtensionFields map[string]struct{}
	allowedErrorFields          map[string]struct{}

	reporter Reporter

	// errorFormatter is a function provided by the router that formats Go errors into GraphQL responses and writes them to a writer.
	// It's not really async, and it needs to be done under the writer's mutex. This is complex and should be resolved in the future.
	errorFormatter AsyncErrorWriter

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
	r.errorFormatter = w
}

// MaxConcurrentResolves returns the configured maximum number of concurrent
// resolves (the size of the resolver semaphore). Use only for stats.
func (r *Resolver) MaxConcurrentResolves() int {
	return cap(r.maxConcurrency)
}

// InflightResolves returns the number of resolves currently holding a
// semaphore slot. The value is a snapshot and may change immediately after
// being read. Use only for stats.
func (r *Resolver) InflightResolves() int {
	return cap(r.maxConcurrency) - len(r.maxConcurrency)
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

	// AllowCustomExtensionProperties allows custom extension properties to be propagated to the client
	AllowCustomExtensionProperties bool

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
		subscriptionsByConnection:    make(map[ConnectionID]map[SubscriptionIdentifier]*subscriptionState),
		reporter:                     options.Reporter,
		errorFormatter:               options.AsyncErrorWriter,
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

func NewLoader(options ResolverOptions, allowedExtensionFields map[string]struct{}, allowedErrorFields map[string]struct{}, sf *SubgraphRequestSingleFlight, a arena.Arena, db *DataBuffer, resolvable *Resolvable) *Loader {
	return &Loader{
		dataBuffer:                             db,
		resolvable:                             resolvable,
		apolloCompatibilitySuppressFetchErrors: options.ResolvableOptions.ApolloCompatibilitySuppressFetchErrors,
		apolloCompatibilityValueCompletionInExtensions: options.ResolvableOptions.ApolloCompatibilityValueCompletionInExtensions,
		allowCustomExtensionProperties:                 options.AllowCustomExtensionProperties,
		propagateSubgraphErrors:                        options.PropagateSubgraphErrors,
		propagateSubgraphStatusCodes:                   options.PropagateSubgraphStatusCodes,
		subgraphErrorPropagationMode:                   options.SubgraphErrorPropagationMode,
		rewriteSubgraphErrorPaths:                      options.RewriteSubgraphErrorPaths,
		omitSubgraphErrorLocations:                     options.OmitSubgraphErrorLocations,
		omitSubgraphErrorExtensions:                    options.OmitSubgraphErrorExtensions,
		allowedErrorExtensionFields:                    allowedExtensionFields,
		attachServiceNameToErrorExtension:              options.AttachServiceNameToErrorExtensions,
		defaultErrorExtensionCode:                      options.DefaultErrorExtensionCode,
		allowedSubgraphErrorFields:                     allowedErrorFields,
		allowAllErrorExtensionFields:                   options.AllowAllErrorExtensionFields,
		apolloRouterCompatibilitySubrequestHTTPError:   options.ApolloRouterCompatibilitySubrequestHTTPError,
		propagateFetchReasons:                          options.PropagateFetchReasons,
		validateRequiredExternalFields:                 options.ValidateRequiredExternalFields,
		singleFlight:                                   sf,
		jsonArena:                                      a,
	}
}

type GraphQLResolveInfo struct {
	// ResolveAcquireWaitTime is the time spent waiting to acquire the resolver semaphore
	// the semaphore limits the number of concurrent resolve operations
	ResolveAcquireWaitTime time.Duration

	// ResolveDeduplicated indicates whether the resolution of the entire operation was deduplicated via single flight
	ResolveDeduplicated bool

	// ResponseResolveStartTime is the time when GraphQL response completion and rendering started.
	ResponseResolveStartTime time.Time
	// ResponseResolveDuration is the time spent completing and rendering the GraphQL response.
	ResponseResolveDuration time.Duration

	// ResponseWriteStartTime is the time when the resolved response started writing to the client.
	ResponseWriteStartTime time.Time
	// ResponseWriteDuration is the time spent writing the resolved response to the client.
	ResponseWriteDuration time.Duration
}

func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (*GraphQLResolveInfo, error) {
	resp := &GraphQLResolveInfo{}

	start := time.Now()
	<-r.maxConcurrency
	resp.ResolveAcquireWaitTime = time.Since(start)
	defer func() {
		r.maxConcurrency <- struct{}{}
	}()

	resolvable := NewResolvable(nil, r.options.ResolvableOptions)

	err := resolvable.Init(ctx, data, response.Info.OperationType)
	if err != nil {
		return nil, err
	}

	// The DataBuffer wraps the base tree produced by Init (which may already
	// contain initialData). The loader fetches/merges into it.
	db := &DataBuffer{data: resolvable.data}
	loader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, nil, db, resolvable)

	if !ctx.ExecutionOptions.SkipLoader {
		// Pre-fetch field authorization only matters when fetches actually run. When the loader is
		// skipped (e.g. query-plan-only responses) there are no origin fetches, so we must not invoke
		// the authorizer here.
		if err = r.authorizeFieldsPreFetch(ctx, response, resolvable); err != nil {
			return nil, err
		}
		err = loader.LoadGraphQLResponseData(ctx, response)
		if err != nil {
			return nil, err
		}
		// Inject loader output into Resolvable before rendering.
		resolvable.data = loader.dataBuffer.Get()
		resolvable.errors = loader.errors
		resolvable.subgraphExtensions = loader.subgraphExtensions
		resolvable.skipValueCompletion = loader.skipValueCompletion
	}

	responseResolveStart := time.Now()
	err = resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, writer)
	resp.ResponseResolveStartTime = responseResolveStart
	resp.ResponseResolveDuration = time.Since(responseResolveStart)
	if err != nil {
		return nil, err
	}

	ctx.TypeNameStats = resolvable.typeNameStats

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
		responseWriteStart := time.Now()
		_, err = writer.Write(inflight.Data)
		resp.ResponseWriteStartTime = responseWriteStart
		resp.ResponseWriteDuration = time.Since(responseWriteStart)
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
	resolvable := NewResolvable(resolveArena.Arena, r.options.ResolvableOptions)

	err = resolvable.Init(ctx, nil, response.Info.OperationType)
	if err != nil {
		r.inboundRequestSingleFlight.FinishErr(inflight, err)
		r.resolveArenaPool.Release(resolveArena)
		return nil, err
	}

	// The DataBuffer wraps the base tree produced by Init. The loader merges into it.
	db := &DataBuffer{data: resolvable.data}
	loader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena, db, resolvable)

	if !ctx.ExecutionOptions.SkipLoader {
		// Pre-fetch field authorization only matters when fetches actually run. When the loader is
		// skipped (e.g. query-plan-only responses) there are no origin fetches, so we must not invoke
		// the authorizer here.
		if err = r.authorizeFieldsPreFetch(ctx, response, resolvable); err != nil {
			r.inboundRequestSingleFlight.FinishErr(inflight, err)
			r.resolveArenaPool.Release(resolveArena)
			return nil, err
		}
		err = loader.LoadGraphQLResponseData(ctx, response)
		if err != nil {
			r.inboundRequestSingleFlight.FinishErr(inflight, err)
			r.resolveArenaPool.Release(resolveArena)
			return nil, err
		}
		resolvable.data = loader.dataBuffer.Get()
		resolvable.errors = loader.errors
		resolvable.subgraphExtensions = loader.subgraphExtensions
		resolvable.skipValueCompletion = loader.skipValueCompletion
	}

	// only when loading is done, acquire an arena for the response buffer
	responseArena := r.responseBufferPool.Acquire(ctx.Request.ID)
	buf := arena.NewArenaBuffer(responseArena.Arena)

	responseResolveStart := time.Now()
	err = resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, buf)
	resp.ResponseResolveStartTime = responseResolveStart
	resp.ResponseResolveDuration = time.Since(responseResolveStart)
	if err != nil {
		r.inboundRequestSingleFlight.FinishErr(inflight, err)
		r.resolveArenaPool.Release(resolveArena)
		r.responseBufferPool.Release(responseArena)
		return nil, err
	}
	ctx.TypeNameStats = resolvable.typeNameStats

	// first release resolverArena
	// all data is resolved and written into the response arena
	r.resolveArenaPool.Release(resolveArena)
	// next we write back to the client
	// this includes flushing and syscalls
	// as such, it can take some time
	// which is why we split the arenas and released the first one
	responseWriteStart := time.Now()
	_, err = writer.Write(buf.Bytes())
	resp.ResponseWriteStartTime = responseWriteStart
	resp.ResponseWriteDuration = time.Since(responseWriteStart)
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

func (r *Resolver) authorizeFieldsPreFetch(ctx *Context, response *GraphQLResponse, resolvable *Resolvable) error {
	if ctx.preFetchFieldAuthorizer == nil || response == nil || response.Info == nil || len(response.Info.AuthorizationCoordinates) == 0 {
		return nil
	}

	coordinateIndex := make(map[GraphCoordinate]int, len(response.Info.AuthorizationCoordinates))
	coordinates := make([]GraphCoordinate, 0, len(response.Info.AuthorizationCoordinates))
	for i := range response.Info.AuthorizationCoordinates {
		coordinate := response.Info.AuthorizationCoordinates[i].Coordinate
		if _, exists := coordinateIndex[coordinate]; exists {
			continue
		}
		coordinateIndex[coordinate] = len(coordinates)
		coordinates = append(coordinates, coordinate)
	}
	decisions, err := ctx.preFetchFieldAuthorizer.AuthorizeFields(ctx, coordinates)
	if err != nil {
		return err
	}
	if len(decisions) != len(coordinates) {
		return fmt.Errorf("batch authorizer returned %d decisions for %d coordinates", len(decisions), len(coordinates))
	}
	for i := range response.Info.AuthorizationCoordinates {
		authCoordinate := response.Info.AuthorizationCoordinates[i]
		decision := decisions[coordinateIndex[authCoordinate.Coordinate]]
		if decision.Allowed {
			resolvable.seedAuthorizationAllow(authCoordinate.DataSourceID, authCoordinate.Coordinate)
		} else {
			resolvable.seedAuthorizationDeny(authCoordinate.DataSourceID, authCoordinate.Coordinate, decision.Reason)
		}
	}
	return nil
}

func (r *Resolver) ResolveGraphQLDeferResponse(ctx *Context, response *GraphQLDeferResponse, writer DeferResponseWriter) (*GraphQLResolveInfo, error) {
	resolveInfo := &GraphQLResolveInfo{}

	start := time.Now()
	<-r.maxConcurrency
	resolveInfo.ResolveAcquireWaitTime = time.Since(start)
	defer func() {
		r.maxConcurrency <- struct{}{}
	}()

	// One arena backs the whole deferred response: the resolvable, the initial
	// loader, and every defer group's loader allocate from it. This is safe
	// despite groups running concurrently because every arena allocation happens
	// under db's lock — the loader allocates only in its prepare/merge phases
	// (both hold db.Lock()), the off-lock network phase touches no arena, and the
	// resolvable's defer-batch renders also run under the lock. The arena retains
	// the entire response tree until every frame is flushed, so it is released
	// only when this function returns (after resolveDeferTree has joined all
	// groups), matching the lifetime the heap gave it before.
	resolveArena := r.resolveArenaPool.Acquire(ctx.Request.ID)
	defer r.resolveArenaPool.Release(resolveArena)

	resolvable := NewResolvable(resolveArena.Arena, r.options.ResolvableOptions)

	err := resolvable.Init(ctx, nil, response.Response.Info.OperationType)
	if err != nil {
		return nil, err
	}

	// The DataBuffer wraps the base tree produced by Init. The loader and every
	// defer group merge into it.
	db := &DataBuffer{data: resolvable.data}
	loader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena, db, resolvable)

	if !ctx.ExecutionOptions.SkipLoader {
		// Pre-fetch field authorization: seed the batch decisions before the initial fetch, so denied
		// fields are skipped/nulled during the initial and deferred renders, matching the
		// non-deferred paths. The seeded decisions live on the shared resolvable and cover every
		// selected coordinate, including those inside @defer fragments.
		if err := r.authorizeFieldsPreFetch(ctx, response.Response, resolvable); err != nil {
			return nil, err
		}

		loader.Init(ctx, response.Response.Info)

		// fetch initial response
		if err := loader.ResolveFetchNode(response.Response.Fetches); err != nil {
			loader.appendSubgraphErrorsToContext()
			return nil, err
		}

		loader.appendSubgraphErrorsToContext()

		// Inject loader output before the initial defer render.
		resolvable.data = loader.dataBuffer.Get()
		resolvable.errors = loader.errors
		resolvable.subgraphExtensions = loader.subgraphExtensions
		resolvable.skipValueCompletion = loader.skipValueCompletion

		resolvable.deferMode = true
		resolvable.currentDefer = nil
		resolvable.deferDescriptors = response.DeferDescriptors

		// render initial response
		err = resolvable.Resolve(ctx.ctx, response.Response.Data, response.Response.Fetches, writer)
		if err != nil {
			// Nothing has been committed to the wire yet (the writer only buffers
			// until Flush), so return the error for the router to format as a
			// top-level error response.
			return nil, err
		}

		err = writer.Flush()
		if err != nil {
			return nil, err
		}

		// The initial frame is now on the wire — the multipart response is
		// committed. From here every exit must terminate the stream, so Complete()
		// is registered only now: a pre-flush error above can still return cleanly
		// to the caller for top-level formatting.
		defer func() {
			writer.Complete()
		}()

		// Fetch deferred responses using the parallel execution tree. Each top-level
		// defer is gated on its anchor surviving the initial render; a defer whose
		// anchor null-propagated is pruned away here. Nested defers are announced
		// lazily as their parent is released (see ResolveDeferBatch).
		if response.DeferTree != nil {
			liveTop := resolvable.liveChildDescriptors(0)
			liveTree := pruneDeadDefers(response.DeferTree, liveTop)
			if liveTree != nil {
				// outstanding tracks announced-but-not-completed defers: it starts at
				// the top-level live count and is adjusted per frame as parents
				// announce children and defers complete. The frame that drives it to
				// zero writes hasNext:false.
				outstanding := int64(len(liveTop))
				dc := &deferContext{
					response:   response,
					info:       response.Response.Info,
					db:         db,
					resolvable: resolvable,
					writer:     writer,
					arena:      resolveArena.Arena,
				}
				if err := r.resolveDeferTree(dc, ctx, liveTree, &outstanding); err != nil {
					return nil, err
				}
			}
		}
	}

	return resolveInfo, err
}

// deferContext bundles the request-scoped state shared by every node in a
// defer tree walk. ctx is NOT included — it varies per goroutine (cloned in the
// parallel branch) and is passed as its own arg.
type deferContext struct {
	response   *GraphQLDeferResponse
	info       *GraphQLResponseInfo
	db         *DataBuffer
	resolvable *Resolvable
	writer     DeferResponseWriter
	// arena backs every defer group's loader. It is shared across groups; every
	// allocation from it is serialised by db's lock (see resolveDeferSingle).
	arena arena.Arena
}

// resolveDeferSingle fetches and renders a single deferred fragment, announcing
// the pending entries for its direct children whose anchor survived, and returns
// those live child ids so the caller can schedule exactly them.
//
// outstanding is the dynamic count of announced-but-not-completed defers; the
// frame that drives it to zero writes hasNext:false (the final frame). The
// counter is adjusted inside ResolveDeferBatch/ResolveDeferError, under the lock.
//
// Each defer group gets its OWN Loader (via NewLoader) sharing only the parent
// DataBuffer; the render phase is serialised by db.Lock(). Network I/O via
// ResolveFetchNode runs before the lock, allowing sibling defer fetches to overlap.
func (r *Resolver) resolveDeferSingle(dc *deferContext, ctx *Context, group *DeferFetchGroup, outstanding *int64) (map[int]DeferDescriptor, error) {
	// FETCH PHASE — runs outside the DataBuffer lock.
	// Each goroutine gets its OWN Loader but they all share one arena (dc.arena).
	// That is safe even though groups run concurrently: a Loader allocates from
	// the arena only in its prepare and merge phases, both of which hold
	// dc.db.Lock(), and the off-lock network phase allocates nothing from it. The
	// lock therefore serialises every arena allocation across all groups.
	groupLoader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, dc.arena, dc.db, nil)
	groupLoader.Init(ctx, dc.info) // fresh taintedObjs; errors=nil

	if fetchErr := groupLoader.ResolveFetchNode(group.Fetches); fetchErr != nil {
		// A hard fetch-phase error (e.g. pre-fetch authorizer/rate-limiter error) is
		// scoped to this defer's completed entry: the announced pending is completed
		// with the error and the stream terminates.
		dc.db.Lock()
		defer dc.db.Unlock()
		groupLoader.appendSubgraphErrorsToContext()
		descriptor := dc.resolvable.deferDescriptors[group.DeferID]
		dc.resolvable.currentDefer = &descriptor
		if err := dc.resolvable.ResolveDeferError(dc.writer, fetchErr.Error(), outstanding); err != nil {
			return nil, err
		}
		return nil, dc.writer.Flush()
	}

	// RENDER PHASE — serialised by the DataBuffer lock.
	dc.db.Lock()
	defer dc.db.Unlock()

	// Inject group-local state into Resolvable for this render.
	dc.resolvable.data = dc.db.Get()
	dc.resolvable.errors = groupLoader.errors
	dc.resolvable.subgraphExtensions = groupLoader.subgraphExtensions
	groupLoader.appendSubgraphErrorsToContext()

	// TODO: skipValueCompletion is set inside mergeResult when a fetch response
	// has errors but no data and apolloCompatibilityValueCompletionInExtensions
	// is enabled. Within a single group's fetch tree, resolveParallel spawns
	// concurrent sub-fetches that share this group's Loader — if one sub-fetch
	// sets skipValueCompletion=true it contaminates the others in that group.
	// This is a pre-existing issue to be addressed separately.
	dc.resolvable.skipValueCompletion = groupLoader.skipValueCompletion

	descriptor := dc.resolvable.deferDescriptors[group.DeferID]
	dc.resolvable.currentDefer = &descriptor
	liveChildren, err := dc.resolvable.ResolveDeferBatch(dc.response.Response.Data, dc.writer, outstanding)
	if err != nil {
		return nil, err
	}
	return liveChildren, dc.writer.Flush()
}

// resolveDeferTree walks a DeferTreeNode and resolves deferred fragments:
//   - Single nodes are resolved directly.
//   - Sequence nodes resolve the parent first, then schedule only the children it
//     announced as live (anchor survived its render); dead children are cancelled.
//   - Parallel nodes spawn concurrent goroutines; rendering is serialised by the
//
// shared DataBuffer lock. Sibling fetch I/O can overlap.
func (r *Resolver) resolveDeferTree(dc *deferContext, ctx *Context, node *DeferTreeNode, outstanding *int64) error {
	switch node.Kind {
	case DeferTreeNodeKindSingle:
		_, err := r.resolveDeferSingle(dc, ctx, node.Item, outstanding)
		return err

	case DeferTreeNodeKindSequence:
		// buildDeferTree shape: ChildNodes[0] is the parent Single, the rest is the
		// child subtree. Resolve the parent, then schedule only the children it
		// announced as live (anchor survived its render).
		parentNode := node.ChildNodes[0]
		liveChildren, err := r.resolveDeferSingle(dc, ctx, parentNode.Item, outstanding)
		if err != nil {
			return err
		}
		for _, child := range node.ChildNodes[1:] {
			pruned := pruneDeadDefers(child, liveChildren)
			if pruned == nil {
				continue
			}
			if err := r.resolveDeferTree(dc, ctx, pruned, outstanding); err != nil {
				return err
			}
		}
		return nil

	case DeferTreeNodeKindParallel:
		// Plain errgroup.Group (NOT errgroup.WithContext): a failed defer group
		// must not cancel its siblings, so we never let errgroup's error-driven
		// cancellation fire. errgroup is used only to spawn + wait + collect one
		// error. The client context (ctx.ctx) still cancels every in-flight fetch
		// on disconnect.
		var g errgroup.Group
		for _, child := range node.ChildNodes {
			g.Go(func() error {
				// Groups share the request *Context. Its only mutation during defer
				// resolution is ctx.subgraphErrors, written exclusively under
				// dc.db.Lock() (appendSubgraphErrorsToContext and render-time
				// addRejectFieldError), so no per-goroutine clone is needed and
				// group subgraph errors aggregate into Context.SubgraphErrors().
				err := r.resolveDeferTree(dc, ctx, child, outstanding)
				// Surface the error only if the client context was cancelled
				// (disconnect). Ordinary defer-level subgraph errors are rendered
				// into the group's incremental frame, not propagated, so they
				// don't abort sibling groups.
				if err != nil && ctx.ctx.Err() != nil {
					return err
				}
				return nil
			})
		}
		return g.Wait()
	}
	return nil
}

// authorizeSubscriptionPreFetch authorizes a subscription's single protected root field before the
// trigger is started, so an unauthorized subscription never opens (or holds) an upstream subscription.
// It returns the response body to write and true when the subscription is unauthorized. Nested
// protected fields are still authorized per update during resolution.
func (r *Resolver) authorizeSubscriptionPreFetch(ctx *Context, response *GraphQLResponse) (deny []byte, denied bool, err error) {
	if ctx.preFetchFieldAuthorizer == nil {
		return nil, false, nil
	}
	if response == nil || response.Data == nil || len(response.Data.Fields) == 0 {
		return nil, false, nil
	}
	rootField := response.Data.Fields[0]
	if rootField.Info == nil || !rootField.Info.HasAuthorizationRule || len(rootField.Info.Source.IDs) == 0 {
		return nil, false, nil
	}
	coordinate := GraphCoordinate{
		TypeName:  rootField.Info.ExactParentTypeName,
		FieldName: rootField.Info.Name,
	}
	decisions, err := ctx.preFetchFieldAuthorizer.AuthorizeFields(ctx, []GraphCoordinate{coordinate})
	if err != nil {
		return nil, false, err
	}
	// Fail closed: a wrong decision count is an authorizer bug, not an authorization grant.
	if len(decisions) != 1 {
		return nil, false, fmt.Errorf("pre-fetch field authorizer returned %d decisions for 1 coordinate", len(decisions))
	}
	if decisions[0].Allowed {
		return nil, false, nil
	}
	message := fmt.Sprintf("Unauthorized to load field '%s.%s'.", coordinate.TypeName, coordinate.FieldName)
	if decisions[0].Reason != "" {
		message = fmt.Sprintf("Unauthorized to load field '%s.%s', Reason: %s.", coordinate.TypeName, coordinate.FieldName, decisions[0].Reason)
	}
	body := fmt.Sprintf(`{"errors":[{"message":%q,"extensions":{"code":%q}}],"data":null}`, message, errorcodes.UnauthorizedFieldOrType)
	return []byte(body), true, nil
}

// trigger groups subscriptions that share a data source and input.
type trigger struct {
	// mu protects subscriptions.
	// Uses snapshot-and-release: held only during map access, released before I/O.
	mu            sync.RWMutex
	id            uint64
	cancel        context.CancelFunc
	subscriptions map[SubscriptionIdentifier]*subscriptionState
	// initialized is set to true when the trigger is started and initialized.
	initialized atomic.Bool
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

// snapshotSubscriptions returns a point-in-time copy of all subscriptions.
func (t *trigger) snapshotSubscriptions() []*subscriptionState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	subs := make([]*subscriptionState, 0, len(t.subscriptions))
	for _, s := range t.subscriptions {
		subs = append(subs, s)
	}
	return subs
}

// evalFilter runs SkipEvent for a single subscription. Must be called under t.mu.
func (t *trigger) evalFilter(s *subscriptionState, data []byte) (*subscriptionState, *pendingFilterError) {
	if s.ctx.ctx.Err() != nil {
		return nil, nil
	}
	skip, err := s.resolve.Filter.SkipEvent(s.ctx, data)
	if err != nil {
		fe := pendingFilterError{s.ctx, err, s.resolve.Response, s}
		return nil, &fe
	}
	if skip {
		return nil, nil
	}
	return s, nil
}

// filterSubscriptions evaluates SkipEvent for every active subscription and
// partitions them into pending updates and filter errors.
func (t *trigger) filterSubscriptions(data []byte) ([]*subscriptionState, []pendingFilterError) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var subs []*subscriptionState
	var filterErrors []pendingFilterError

	for _, s := range t.subscriptions {
		pending, filterErr := t.evalFilter(s, data)
		if pending != nil {
			subs = append(subs, pending)
		}
		if filterErr != nil {
			filterErrors = append(filterErrors, *filterErr)
		}
	}

	return subs, filterErrors
}

// filterSubscription evaluates SkipEvent for a single subscription by ID.
func (t *trigger) filterSubscription(id SubscriptionIdentifier, data []byte) (*subscriptionState, *pendingFilterError) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s, ok := t.subscriptions[id]
	if !ok {
		return nil, nil
	}

	sub, filterErr := t.evalFilter(s, data)

	return sub, filterErr
}

// subscriptionState tracks a single active subscription.
type subscriptionState struct {
	triggerID uint64
	resolve   *GraphQLSubscription
	ctx       *Context
	writer    SubscriptionResponseWriter
	id        SubscriptionIdentifier
	heartbeat bool
	completed chan struct{}
	// writeMu protects all writes to writer (Complete, Error, Write, Flush, Heartbeat).
	// Paired with the removed atomic to prevent writes after removal.
	writeMu sync.Mutex
	// removed guards against writes after the subscription has been removed.
	// Uses CompareAndSwap to prevent double-close of the completed channel.
	removed atomic.Bool
	// lastWriteTime stores unix nanos of the last successful data write.
	lastWriteTime atomic.Int64
}

func closeSubs(subs []*subscriptionState) {
	for _, s := range subs {
		s.done()
	}
}

// done closes the completed channel to signal that the subscription is finished.
// It does not send any downstream messages — Complete/Error are sent separately.
func (s *subscriptionState) done() {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	close(s.completed)
}

// complete delivers a "subscription done" signal to the downstream writer.
// Called by handleTriggerComplete, not through toClose.
func (s *subscriptionState) complete() {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.writer.Complete()
}

// error delivers a terminal error payload to the downstream writer.
// Called by handleTriggerError, not through toClose.
func (s *subscriptionState) error(data []byte) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.writer.Error(data)
}

// writeError delivers a formatted error to the downstream writer under writeMu.
func (s *subscriptionState) writeError(w AsyncErrorWriter, ctx *Context, err error, response *GraphQLResponse) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.removed.Load() {
		return
	}
	w.WriteError(ctx, err, response, s.writer)
}

// sendHeartbeat sends a keep-alive frame to the downstream writer under writeMu.
// @TODO: this is bad, see ENG-9356
func (s *subscriptionState) sendHeartbeat() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.removed.Load() {
		return nil
	}
	return s.writer.Heartbeat()
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
	resolvable := NewResolvable(resolveArena.Arena, r.options.ResolvableOptions)

	if err := resolvable.InitSubscription(resolveCtx, input, sub.resolve.Trigger.PostProcessing); err != nil {
		r.resolveArenaPool.Release(resolveArena)
		sub.writeError(r.errorFormatter, resolveCtx, err, sub.resolve.Response)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:init:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}
	if err := r.authorizeFieldsPreFetch(resolveCtx, sub.resolve.Response, resolvable); err != nil {
		r.resolveArenaPool.Release(resolveArena)
		sub.writeError(r.errorFormatter, resolveCtx, err, sub.resolve.Response)
		if r.options.Debug {
			fmt.Printf("resolver:trigger:subscription:authorization:failed:%d\n", sub.id.SubscriptionID)
		}
		if r.reporter != nil {
			r.reporter.SubscriptionUpdateSent()
		}
		return
	}

	// The DataBuffer wraps the base tree produced by InitSubscription (the
	// subscription event payload). The loader merges fetched data into it.
	db := &DataBuffer{data: resolvable.data}
	loader := NewLoader(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena, db, resolvable)

	if err := loader.LoadGraphQLResponseData(resolveCtx, sub.resolve.Response); err != nil {
		r.resolveArenaPool.Release(resolveArena)
		sub.writeError(r.errorFormatter, resolveCtx, err, sub.resolve.Response)
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

	// Inject loader output into Resolvable before rendering. InitSubscription may
	// have already set resolvable.errors from the event payload, so append the
	// loader's fetch errors rather than overwrite them.
	resolvable.data = loader.dataBuffer.Get()
	resolvable.subgraphExtensions = loader.subgraphExtensions
	if loader.errors != nil {
		if resolvable.errors == nil {
			resolvable.errors = loader.errors
		} else {
			resolvable.errors.AppendArrayItems(resolveArena.Arena, loader.errors)
		}
	}
	resolvable.skipValueCompletion = loader.skipValueCompletion

	if err := resolvable.Resolve(resolveCtx.ctx, sub.resolve.Response.Data, sub.resolve.Response.Fetches, sub.writer); err != nil {
		r.resolveArenaPool.Release(resolveArena)
		r.errorFormatter.WriteError(resolveCtx, err, sub.resolve.Response, sub.writer)
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

	if resolvable.WroteErrorsWithoutData() && r.options.Debug {
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

	if err := sub.sendHeartbeat(); err != nil {
		_ = r.UnsubscribeSubscription(sub.id)
		return
	}

	if r.reporter != nil {
		r.reporter.SubscriptionUpdateSent()
	}
}

type StartupHookContext struct {
	Context context.Context
	Updater func(data []byte)
}

func (r *Resolver) executeStartupHooks(add *addSubscription, updater *subscriptionUpdater) error {
	hook, ok := add.resolve.Trigger.Source.(HookableSubscriptionDataSource)
	if !ok {
		return nil
	}
	hookCtx := StartupHookContext{
		Context: add.ctx.Context(),
		Updater: func(data []byte) {
			updater.UpdateSubscription(add.id, data)
		},
	}
	err := hook.SubscriptionOnStart(hookCtx, add.input)
	if err != nil && r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:startup:failed:%d\n", add.id.SubscriptionID)
	}
	return err
}

// registerSubscriptionLocked updates the by-ID and by-connection indexes.
func (r *Resolver) registerSubscriptionLocked(trig *trigger, s *subscriptionState) {
	trig.mu.Lock()
	trig.subscriptions[s.id] = s
	trig.mu.Unlock()
	id := s.id
	r.subscriptionsByID[id] = s
	byConn, ok := r.subscriptionsByConnection[id.ConnectionID]
	if !ok {
		byConn = make(map[SubscriptionIdentifier]*subscriptionState)
		r.subscriptionsByConnection[id.ConnectionID] = byConn
	}
	byConn[id] = s
}

// unregisterSubscriptionLocked removes from the by-ID and by-connection indexes.
func (r *Resolver) unregisterSubscriptionLocked(id SubscriptionIdentifier) {
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

// addSubscription registers a new subscription under the given trigger.
func (r *Resolver) addSubscription(triggerID uint64, add *addSubscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown {
		return r.ctx.Err()
	}
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
		// Register first so startup hooks can deliver initial data via UpdateSubscription.
		r.registerSubscriptionLocked(trig, s)
		// Execute the startup hooks in a goroutine to avoid holding the lock.
		go func() {
			if err := r.executeStartupHooks(add, trig.updater); err != nil {
				s.writeError(r.errorFormatter, add.ctx, err, add.resolve.Response)
				_ = r.UnsubscribeSubscription(add.id)
			}
		}()
		return nil
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
		cancel:        cancel,
		updater:       updater,
	}
	r.triggers[triggerID] = trig
	updater.subsFn = trig.subscriptionIds
	r.registerSubscriptionLocked(trig, s)

	if r.reporter != nil {
		r.reporter.SubscriptionCountInc(1)
	}

	go func() {
		if r.options.Debug {
			fmt.Printf("resolver:trigger:start:%d\n", triggerID)
		}

		// The startup hook is blocking so it can reject the subscription before Source.Start.
		// If either step fails, broadcast the error to all subs and tear down the trigger.
		err := r.executeStartupHooks(add, trig.updater)
		if err == nil {
			err = add.resolve.Trigger.Source.Start(cloneCtx, add.headers, add.input, trig.updater)
		}
		if err != nil {
			if r.options.Debug {
				fmt.Printf("resolver:trigger:failed:%d\n", triggerID)
			}
			for _, sub := range trig.snapshotSubscriptions() {
				sub.writeError(r.errorFormatter, sub.ctx, err, sub.resolve.Response)
			}
			r.doneTriggerFromUpdater(triggerID)
			return
		}

		r.markTriggerInitialized(triggerID)

		if r.options.Debug {
			fmt.Printf("resolver:trigger:started:%d\n", triggerID)
		}
	}()
	return nil
}

func (r *Resolver) getTrigger(id uint64) (*trigger, bool) {
	r.mu.Lock()
	trig, ok := r.triggers[id]
	r.mu.Unlock()
	return trig, ok
}

// markTriggerInitialized marks a trigger as initialized and reports it.
func (r *Resolver) markTriggerInitialized(triggerID uint64) {
	trig, ok := r.getTrigger(triggerID)
	if !ok {
		return
	}
	trig.initialized.Store(true)
	if r.reporter != nil {
		r.reporter.TriggerCountInc(1)
	}
}

// doneTriggerFromUpdater performs cleanup for a trigger from a datasource/updater goroutine.
// It detaches the trigger, runs done toClose (close completed channels), and cancels the trigger context.
func (r *Resolver) doneTriggerFromUpdater(triggerID uint64) {
	if r.options.Debug {
		fmt.Printf("resolver:trigger:shutdown:%d\n", triggerID)
	}
	r.mu.Lock()
	res := r.detachTriggerLocked(triggerID)
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(res.removed)
		if res.initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
	r.mu.Unlock()
	closeSubs(res.toClose)
	if res.triggerCancel != nil {
		res.triggerCancel()
	}
}

// handleTriggerComplete delivers a complete signal to all subscriptions on the trigger.
// Does NOT detach the trigger — Done() does that.
func (r *Resolver) handleTriggerComplete(triggerID uint64) {
	trig, ok := r.getTrigger(triggerID)
	if !ok {
		return
	}
	subs := trig.snapshotSubscriptions()

	for _, s := range subs {
		if !s.removed.Load() {
			s.complete()
		}
	}
}

// handleTriggerError delivers a terminal error to all subscriptions on the trigger,
// bypassing the resolve pipeline. Does NOT detach the trigger — Done() does that.
func (r *Resolver) handleTriggerError(triggerID uint64, data []byte) {
	trig, ok := r.getTrigger(triggerID)
	if !ok {
		return
	}
	subs := trig.snapshotSubscriptions()

	for _, s := range subs {
		if !s.removed.Load() {
			s.error(data)
		}
	}
}

func (r *Resolver) removeClient(id ConnectionID) removeClientResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shutdown {
		return removeClientResult{}
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:remove:client:%d\n", id)
	}
	removed := 0
	toClose := make([]*subscriptionState, 0)
	cancels := make([]context.CancelFunc, 0)
	triggerDec := 0
	idsForConn := r.subscriptionsByConnection[id]
	ids := make([]SubscriptionIdentifier, 0, len(idsForConn))
	for sid := range idsForConn {
		ids = append(ids, sid)
	}
	for _, sid := range ids {
		res := r.removeSubscriptionLocked(sid)
		removed += res.removed
		toClose = append(toClose, res.toClose...)
		if res.triggerCancel != nil {
			cancels = append(cancels, res.triggerCancel)
			if res.initialized {
				triggerDec++
			}
		}
	}
	res := removeClientResult{
		removed:    removed,
		toClose:    toClose,
		cancels:    cancels,
		triggerDec: triggerDec,
	}
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(res.removed)
		if res.triggerDec > 0 {
			r.reporter.TriggerCountDec(res.triggerDec)
		}
	}
	return res
}

// removeSubscriptionLocked removes a single subscription by id.
// r.mu must be held by the caller.
func (r *Resolver) removeSubscriptionLocked(id SubscriptionIdentifier) removeResult {
	s, ok := r.subscriptionsByID[id]
	if !ok {
		return removeResult{}
	}

	trig, ok := r.triggers[s.triggerID]
	if !ok {
		r.unregisterSubscriptionLocked(id)
		return removeResult{}
	}

	trig.mu.Lock()
	_, ok = trig.subscriptions[id]
	if !ok {
		trig.mu.Unlock()
		r.unregisterSubscriptionLocked(id)
		return removeResult{}
	}

	var toClose []*subscriptionState
	if s.removed.CompareAndSwap(false, true) {
		toClose = append(toClose, s)
	}
	delete(trig.subscriptions, id)
	empty := len(trig.subscriptions) == 0
	trig.mu.Unlock()

	r.unregisterSubscriptionLocked(id)

	var triggerCancel context.CancelFunc
	initialized := false
	if empty {
		delete(r.triggers, trig.id)
		triggerCancel = trig.cancel
		initialized = trig.initialized.Load()
	}

	return removeResult{
		removed:       1,
		toClose:       toClose,
		triggerCancel: triggerCancel,
		initialized:   initialized,
	}
}

// detachTriggerLocked removes all subscriptions for the trigger and removes the trigger from resolver maps.
// r.mu must be held by the caller.
func (r *Resolver) detachTriggerLocked(id uint64) removeResult {
	trig, ok := r.triggers[id]
	if !ok {
		return removeResult{}
	}

	toClose := make([]*subscriptionState, 0, len(trig.subscriptions))
	removed := 0

	trig.mu.Lock()
	for sid, s := range trig.subscriptions {
		if s.removed.CompareAndSwap(false, true) {
			toClose = append(toClose, s)
		}
		delete(trig.subscriptions, sid)
		r.unregisterSubscriptionLocked(sid)
		removed++
	}
	trig.mu.Unlock()

	delete(r.triggers, id)

	return removeResult{
		removed:       removed,
		toClose:       toClose,
		triggerCancel: trig.cancel,
		initialized:   trig.initialized.Load(),
	}
}

type removeResult struct {
	removed       int
	toClose       []*subscriptionState
	triggerCancel context.CancelFunc // non-nil if trigger became empty
	initialized   bool               // whether the removed trigger was initialized
}

type removeClientResult struct {
	removed    int
	toClose    []*subscriptionState
	cancels    []context.CancelFunc
	triggerDec int
}

type pendingFilterError struct {
	ctx      *Context
	err      error
	response *GraphQLResponse
	sub      *subscriptionState
}

// handleTriggerUpdate sends data to all subscriptions of a trigger.
func (r *Resolver) handleTriggerUpdate(id uint64, data []byte) {
	trig, ok := r.getTrigger(id)
	if !ok {
		return
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:update:%d\n", id)
	}

	subs, filterErrors := trig.filterSubscriptions(data)

	for _, fe := range filterErrors {
		fe.sub.writeError(r.errorFormatter, fe.ctx, fe.err, fe.response)
	}

	var wg sync.WaitGroup
	for _, sub := range subs {
		if sub.removed.Load() {
			continue
		}
		wg.Go(func() {
			r.executeSubscriptionUpdate(sub.ctx, sub, data)
		})
	}
	wg.Wait()
}

// handleUpdateSubscription sends data to a single subscription.
func (r *Resolver) handleUpdateSubscription(id uint64, data []byte, subIdentifier SubscriptionIdentifier) {
	trig, ok := r.getTrigger(id)
	if !ok {
		return
	}

	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:update:%d:%d,%d\n", id, subIdentifier.ConnectionID, subIdentifier.SubscriptionID)
	}

	sub, filterErr := trig.filterSubscription(subIdentifier, data)

	if filterErr != nil {
		filterErr.sub.writeError(r.errorFormatter, filterErr.ctx, filterErr.err, filterErr.response)
	}

	if sub != nil && !sub.removed.Load() {
		r.executeSubscriptionUpdate(sub.ctx, sub, data)
	}
}

func (r *Resolver) heartbeatTriggerSubscriptions(id uint64) {
	trig, ok := r.getTrigger(id)
	if !ok {
		return
	}

	subs := trig.snapshotSubscriptions()
	targets := make([]*subscriptionState, 0, len(subs))
	for _, s := range subs {
		if !s.heartbeat || s.removed.Load() {
			continue
		}
		if time.Since(time.Unix(0, s.lastWriteTime.Load())) < r.heartbeatInterval {
			continue
		}
		targets = append(targets, s)
	}

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

	allToClose := make([]*subscriptionState, 0)
	cancels := make([]context.CancelFunc, 0, len(triggerIDs))
	removedTotal := 0
	triggerDec := 0

	for _, id := range triggerIDs {
		res := r.detachTriggerLocked(id)
		removedTotal += res.removed
		allToClose = append(allToClose, res.toClose...)
		if res.triggerCancel != nil {
			cancels = append(cancels, res.triggerCancel)
		}
		if res.initialized {
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
	r.subscriptionsByConnection = make(map[ConnectionID]map[SubscriptionIdentifier]*subscriptionState)
	r.mu.Unlock()

	closeSubs(allToClose)
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
	ConnectionID   ConnectionID
	SubscriptionID int64
}

func (r *Resolver) UnsubscribeSubscription(id SubscriptionIdentifier) error {
	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return r.ctx.Err()
	}
	res := r.removeSubscriptionLocked(id)
	if r.reporter != nil {
		r.reporter.SubscriptionCountDec(res.removed)
		if res.triggerCancel != nil && res.initialized {
			r.reporter.TriggerCountDec(1)
		}
	}
	r.mu.Unlock()
	closeSubs(res.toClose)
	if res.triggerCancel != nil {
		res.triggerCancel()
	}
	return nil
}

func (r *Resolver) UnsubscribeClient(connectionID ConnectionID) error {
	res := r.removeClient(connectionID)
	closeSubs(res.toClose)
	for _, cancel := range res.cancels {
		cancel()
	}
	return nil
}

// prepareTrigger safely gets the headers for the trigger Subgraph and computes the hash across headers and input
// the generated hash is the unique triggerID
// the headers must be forwarded to the DataSource to create the trigger
func (r *Resolver) prepareTrigger(ctx *Context, sourceName string, input []byte, source SubscriptionDataSource) (
	headers http.Header, triggerID uint64, err error) {
	keyGen := pool.Hash64.Get()
	defer pool.Hash64.Put(keyGen)

	if err = source.HashTriggerInput(input, keyGen); err != nil {
		return nil, 0, err
	}

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

	return headers, triggerID, nil
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
		resolvable := NewResolvable(nil, r.options.ResolvableOptions)

		err = resolvable.InitSubscription(ctx, nil, subscription.Trigger.PostProcessing)
		if err != nil {
			return err
		}

		buf := &bytes.Buffer{}
		err = resolvable.Resolve(ctx.ctx, subscription.Response.Data, subscription.Response.Fetches, buf)
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

	// Authorize the subscription's protected root field before starting the trigger, so an
	// unauthorized subscription never opens an upstream subscription.
	if body, denied, authErr := r.authorizeSubscriptionPreFetch(ctx, subscription.Response); authErr != nil {
		return authErr
	} else if denied {
		return writeFlushComplete(writer, body)
	}

	if hook, ok := subscription.Trigger.Source.(HookablePubsubDatasource); ok {
		input, err = hook.SubscriptionOnCreate(ctx.Context(), input)
		if err != nil {
			msg := []byte(`{"errors":[{"message":"failed to prepare subscription trigger"}]}`)
			return writeFlushComplete(writer, msg)
		}
	}

	headers, triggerID, err := r.prepareTrigger(ctx, subscription.Trigger.SourceName, input, subscription.Trigger.Source)
	if err != nil {
		msg := []byte(`{"errors":[{"message":"failed to prepare subscription trigger"}]}`)
		return writeFlushComplete(writer, msg)
	}
	id := SubscriptionIdentifier{
		ConnectionID:   NewConnectionID(),
		SubscriptionID: 0,
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscribe:sync:%d:%d\n", triggerID, id.SubscriptionID)
	}

	completed := make(chan struct{})

	if err := r.addSubscription(triggerID, &addSubscription{
		ctx:        ctx,
		input:      input,
		resolve:    subscription,
		writer:     writer,
		id:         id,
		completed:  completed,
		sourceName: subscription.Trigger.SourceName,
		headers:    headers,
	}); err != nil {
		return err
	}

	// This will immediately block until one of the following conditions is met:
	select {
	case <-ctx.ctx.Done():
		// Client disconnected, request context canceled.
		_ = r.UnsubscribeSubscription(id)
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
		resolvable := NewResolvable(nil, r.options.ResolvableOptions)

		err = resolvable.InitSubscription(ctx, nil, subscription.Trigger.PostProcessing)
		if err != nil {
			return err
		}

		buf := &bytes.Buffer{}
		err = resolvable.Resolve(ctx.ctx, subscription.Response.Data, subscription.Response.Fetches, buf)
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

	// Authorize the subscription's protected root field before starting the trigger, so an
	// unauthorized subscription never opens an upstream subscription.
	if body, denied, authErr := r.authorizeSubscriptionPreFetch(ctx, subscription.Response); authErr != nil {
		return authErr
	} else if denied {
		return writeFlushComplete(writer, body)
	}

	if hook, ok := subscription.Trigger.Source.(HookablePubsubDatasource); ok {
		input, err = hook.SubscriptionOnCreate(ctx.Context(), input)
		if err != nil {
			msg := []byte(`{"errors":[{"message":"failed to prepare subscription trigger"}]}`)
			return writeFlushComplete(writer, msg)
		}
	}

	headers, triggerID, err := r.prepareTrigger(ctx, subscription.Trigger.SourceName, input, subscription.Trigger.Source)
	if err != nil {
		msg := []byte(`{"errors":[{"message":"failed to prepare subscription trigger"}]}`)
		return writeFlushComplete(writer, msg)
	}

	return r.addSubscription(triggerID, &addSubscription{
		ctx:        ctx,
		input:      input,
		resolve:    subscription,
		writer:     writer,
		id:         id,
		completed:  make(chan struct{}),
		sourceName: subscription.Trigger.SourceName,
		headers:    headers,
	})
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

// subscriptionUpdater implements SubscriptionUpdater, the callback API for data sources.
type subscriptionUpdater struct {
	// mu serves two roles:
	//
	// 1. Event serialization gate -- held across the entire Update() call including
	//    wg.Wait(), ensuring event A fully completes before event B begins.
	//
	// 2. Lifecycle guard -- the done flag prevents callbacks after Done() has torn down
	//    the trigger. Every method checks done || ctx.Err() under the lock before proceeding.
	mu        sync.Mutex
	done      bool
	debug     bool
	triggerID uint64
	resolver  *Resolver
	ctx       context.Context
	subsFn    func() map[context.Context]SubscriptionIdentifier
}

func (s *subscriptionUpdater) Update(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.ctx.Err() != nil {
		return
	}
	if s.debug {
		fmt.Printf("resolver:subscription_updater:update:%d\n", s.triggerID)
	}
	s.resolver.handleTriggerUpdate(s.triggerID, data)
}

func (s *subscriptionUpdater) Heartbeat() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.ctx.Err() != nil {
		return
	}
	s.resolver.heartbeatTriggerSubscriptions(s.triggerID)
}

func (s *subscriptionUpdater) UpdateSubscription(id SubscriptionIdentifier, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.ctx.Err() != nil {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.ctx.Err() != nil {
		if s.debug {
			fmt.Printf("resolver:subscription_updater:complete:skip:%d\n", s.triggerID)
		}
		return
	}
	if s.debug {
		fmt.Printf("resolver:subscription_updater:complete:%d\n", s.triggerID)
	}
	s.resolver.handleTriggerComplete(s.triggerID)
}

func (s *subscriptionUpdater) Error(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.ctx.Err() != nil {
		if s.debug {
			fmt.Printf("resolver:subscription_updater:error:skip:%d\n", s.triggerID)
		}
		return
	}
	if s.debug {
		fmt.Printf("resolver:subscription_updater:error:%d\n", s.triggerID)
	}
	s.resolver.handleTriggerError(s.triggerID, data)
}

func (s *subscriptionUpdater) Done() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	s.done = true
	if s.debug {
		fmt.Printf("resolver:subscription_updater:done:%d\n", s.triggerID)
	}
	s.resolver.doneTriggerFromUpdater(s.triggerID)
}

func (s *subscriptionUpdater) CloseSubscription(id SubscriptionIdentifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.ctx.Err() != nil {
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
	// Complete delivers a "subscription done" signal to all subscriptions on the trigger.
	// Does not perform cleanup — call Done() after Complete().
	Complete()
	// Error delivers a terminal error to all subscriptions on the trigger, bypassing the resolve pipeline.
	// Does not perform cleanup — call Done() after Error().
	Error(data []byte)
	// Done performs internal cleanup: detaches the trigger, closes completed channels.
	// Must always be the final call. Does not send any downstream messages.
	Done()
	// CloseSubscription closes a single subscription. No more updates should be sent to that subscription after calling CloseSubscription.
	CloseSubscription(id SubscriptionIdentifier)
	// Subscriptions return all the subscriptions associated to this Updater
	Subscriptions() map[context.Context]SubscriptionIdentifier
}
