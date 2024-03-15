//go:generate mockgen --build_flags=--mod=mod -self_package=github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource,BeforeFetchHook,AfterFetchHook

package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/alitto/pond"
	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/xcontext"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/pool"
)

var (
	ErrResolverClosed                  = errors.New("resolver closed")
	ErrSubscriptionUpdaterNotCloseable = errors.New("subscription updater is not closeable")
)

type Reporter interface {
	SubscriptionUpdateSent()
}

type Resolver struct {
	ctx                 context.Context
	options             ResolverOptions
	toolPool            sync.Pool
	limitMaxConcurrency bool
	maxConcurrency      chan struct{}

	triggers          map[uint64]*trigger
	events            chan subscriptionEvent
	triggerUpdatePool *pond.WorkerPool

	connectionIDs atomic.Int64

	reporter Reporter
}

type tools struct {
	resolvable *Resolvable
	loader     *Loader
}

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

	Reporter Reporter
}

// New returns a new Resolver, ctx.Done() is used to cancel all active subscriptions & streams
func New(ctx context.Context, options ResolverOptions) *Resolver {
	//options.Debug = true
	resolver := &Resolver{
		ctx:     ctx,
		options: options,
		toolPool: sync.Pool{
			New: func() interface{} {
				return &tools{
					resolvable: NewResolvable(),
					loader:     &Loader{},
				}
			},
		},
		events:   make(chan subscriptionEvent),
		triggers: make(map[uint64]*trigger),
		reporter: options.Reporter,
	}
	/*if options.MaxConcurrency > 0 {
		semaphore := make(chan struct{}, options.MaxConcurrency)
		for i := 0; i < options.MaxConcurrency; i++ {
			semaphore <- struct{}{}
		}
		resolver.limitMaxConcurrency = true
		resolver.maxConcurrency = semaphore
	}*/
	if options.MaxSubscriptionWorkers == 0 {
		options.MaxSubscriptionWorkers = 1024
	}
	resolver.triggerUpdatePool = pond.New(
		options.MaxSubscriptionWorkers,
		0,
		pond.Context(ctx),
		pond.IdleTimeout(time.Second*30),
		pond.Strategy(pond.Lazy()),
		pond.MinWorkers(16),
	)
	go resolver.handleEvents()
	return resolver
}

func (r *Resolver) getTools() *tools {
	t := r.toolPool.Get().(*tools)
	return t
}

func (r *Resolver) putTools(t *tools) {
	t.loader.Free()
	t.resolvable.Reset()
	r.toolPool.Put(t)
}

func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (err error) {
	if response.Info == nil {
		response.Info = &GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		}
	}

	t := r.getTools()
	defer r.putTools(t)

	err = t.resolvable.Init(ctx, data, response.Info.OperationType)
	if err != nil {
		return err
	}

	err = t.loader.LoadGraphQLResponseData(ctx, response, t.resolvable)
	if err != nil {
		return err
	}

	return t.resolvable.Resolve(ctx.ctx, response.Data, writer)
}

type trigger struct {
	id            uint64
	cancel        context.CancelFunc
	subscriptions map[*Context]*sub
	inFlight      *sync.WaitGroup
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
}

func (r *Resolver) executeSubscriptionUpdate(ctx *Context, sub *sub, sharedInput []byte) {
	sub.mux.Lock()
	sub.pendingUpdates++
	sub.mux.Unlock()
	if r.options.Debug {
		fmt.Printf("resolver:trigger:subscription:update:%d\n", sub.id.SubscriptionID)
		defer fmt.Printf("resolver:trigger:subscription:update:done:%d\n", sub.id.SubscriptionID)
	}
	t := r.getTools()
	defer r.putTools(t)
	input := make([]byte, len(sharedInput))
	copy(input, sharedInput)
	if err := t.resolvable.InitSubscription(ctx, input, sub.resolve.Trigger.PostProcessing); err != nil {
		return
	}
	if err := t.loader.LoadGraphQLResponseData(ctx, sub.resolve.Response, t.resolvable); err != nil {
		return
	}
	sub.mux.Lock()
	sub.pendingUpdates--
	defer sub.mux.Unlock()
	if sub.writer == nil {
		return // subscription was already closed by the client
	}
	if err := t.resolvable.Resolve(ctx.ctx, sub.resolve.Response.Data, sub.writer); err != nil {
		return
	}
	sub.writer.Flush()
	if r.reporter != nil {
		r.reporter.SubscriptionUpdateSent()
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
	case subscriptionEventKindUnknown:
		panic("unknown event")
	}
}

func (r *Resolver) handleTriggerDone(triggerID uint64) {
	trig, ok := r.triggers[triggerID]
	if !ok {
		return
	}
	delete(r.triggers, triggerID)
	wg := trig.inFlight
	go func() {
		if wg != nil {
			wg.Wait()
		}
		for _, s := range trig.subscriptions {
			s.writer.Complete()
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
		resolve: add.resolve,
		writer:  add.writer,
		id:      add.id,
	}
	trig, ok := r.triggers[triggerID]
	if ok {
		trig.subscriptions[add.ctx] = s
		return
	}
	if r.options.Debug {
		fmt.Printf("resolver:create:trigger:%d\n", triggerID)
	}
	ctx, cancel := context.WithCancel(xcontext.Detach(add.ctx.Context()))
	updater := &subscriptionUpdater{
		triggerID: triggerID,
		ch:        r.events,
		ctx:       ctx,
	}
	clone := add.ctx.clone(ctx)
	trig = &trigger{
		id:            triggerID,
		subscriptions: make(map[*Context]*sub),
		cancel:        cancel,
	}
	r.triggers[triggerID] = trig
	trig.subscriptions[add.ctx] = s
	err = add.resolve.Trigger.Source.Start(clone, add.input, updater)
	if err != nil {
		cancel()
	}
}

func (r *Resolver) handleRemoveSubscription(id SubscriptionIdentifier) {
	for u := range r.triggers {
		trig := r.triggers[u]
		for ctx, s := range trig.subscriptions {
			if s.id == id {
				s.mux.Lock()
				s.writer.Complete()
				s.writer = nil
				s.mux.Unlock()
				delete(trig.subscriptions, ctx)
				if r.options.Debug {
					fmt.Printf("resolver:trigger:subscription:done:%d:%d\n", trig.id, id.SubscriptionID)
				}
			}
		}
		if len(trig.subscriptions) == 0 {
			r.shutdownTrigger(trig.id)
		}
	}
}

func (r *Resolver) handleRemoveClient(id int64) {
	for u := range r.triggers {
		for c, s := range r.triggers[u].subscriptions {
			if s.id.ConnectionID == id && !s.id.internal {
				s.mux.Lock()
				s.writer.Complete()
				s.writer = nil
				s.mux.Unlock()
				delete(r.triggers[u].subscriptions, c)
				if r.options.Debug {
					fmt.Printf("resolver:trigger:subscription:done:%d:%d\n", u, s.id.SubscriptionID)
				}
			}
		}
		if len(r.triggers[u].subscriptions) == 0 {
			r.shutdownTrigger(r.triggers[u].id)
		}
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
	wg.Add(len(trig.subscriptions))
	trig.inFlight = wg
	for c, s := range trig.subscriptions {
		c, s := c, s
		r.triggerUpdatePool.Submit(func() {
			r.executeSubscriptionUpdate(c, s, data)
			wg.Done()
		})
	}
}

func (r *Resolver) shutdownTrigger(id uint64) {
	trig, ok := r.triggers[id]
	if !ok {
		return
	}
	for c, s := range trig.subscriptions {
		s.mux.Lock()
		s.writer.Complete()
		s.writer = nil
		s.mux.Unlock()
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
		return ErrResolverClosed
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
	select {
	case <-r.ctx.Done():
		return ErrResolverClosed
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
	select {
	case <-r.ctx.Done():
		return ErrResolverClosed
	case <-ctx.Context().Done():
	}
	if r.options.Debug {
		fmt.Printf("resolver:trigger:unsubscribe:sync:%d:%d\n", uniqueID, id.SubscriptionID)
	}
	select {
	case <-r.ctx.Done():
		return ErrResolverClosed
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
		return ErrResolverClosed
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
	triggerID uint64
	ch        chan subscriptionEvent
	done      bool
	ctx       context.Context
}

func (s *subscriptionUpdater) Update(data []byte) {
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

func (s *subscriptionUpdater) Close() {
	close(s.ch)
}

type subscriptionEvent struct {
	triggerID       uint64
	id              SubscriptionIdentifier
	kind            subscriptionEventKind
	data            []byte
	addSubscription *addSubscription
}

type addSubscription struct {
	ctx     *Context
	input   []byte
	resolve *GraphQLSubscription
	writer  SubscriptionResponseWriter
	id      SubscriptionIdentifier
	done    func()
}

type subscriptionEventKind int

const (
	subscriptionEventKindUnknown subscriptionEventKind = iota
	subscriptionEventKindTriggerUpdate
	subscriptionEventKindTriggerDone
	subscriptionEventKindAddSubscription
	subscriptionEventKindRemoveSubscription
	subscriptionEventKindRemoveClient
)

type SubscriptionUpdater interface {
	Update(data []byte)
	Done()
}

type CloseableSubscriptionUpdater interface {
	SubscriptionUpdater
	Close()
}

func AsCloseableSubscriptionUpdater(updater SubscriptionUpdater) (CloseableSubscriptionUpdater, bool) {
	if updater == nil {
		return nil, false
	}
	if closeable, ok := updater.(CloseableSubscriptionUpdater); ok {
		return closeable, true
	}
	return nil, false
}
