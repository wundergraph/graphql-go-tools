package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tidwall/gjson"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/errorcodes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const invalidPath = "invalid path"

type Resolvable struct {
	// options holds the resolver-level toggles (Apollo compatibility, allowed
	// subgraph extensions, extension-forwarding algorithm) consulted while rendering.
	options ResolvableOptions

	// data is the response tree being walked and rendered.
	// In the case of defer it is injected before each render; during deferred delivery it points at the shared DataBuffer tree.
	data *astjson.Value

	// errors is the accumulated GraphQL `errors` array (arena-allocated) written
	// into the response envelope.
	errors *astjson.Value

	// valueCompletion collects Apollo "value completion" extension entries for
	// non-nullable fields that resolved to null; emitted under extensions when the
	// ApolloCompatibilityValueCompletionInExtensions option is set.
	valueCompletion *astjson.Value

	// skipAddingNullErrors suppresses further "cannot return null" errors when the
	// response already carries errors but no data (set per render in Resolve).
	skipAddingNullErrors bool

	// astjsonArena is the arena to handle json, supplied by Resolver
	// not thread safe, but Resolvable is single threaded anyways
	astjsonArena arena.Arena

	// parsers are pooled astjson parsers reused across walks to avoid per-call allocation.
	parsers []*astjson.Parser

	// enableRender - controls whether we are doing the validation pre-walk (collect errors, detect null-bubbling)
	// or we are doing the render pass that actually writes output.
	enableRender bool

	// enableDeferRender gates the defer-envelope output (open/close, deferred-field
	// emission, item separation) so it happens only within a single deferred
	// item's render.
	enableDeferRender bool

	// out is the destination writer for the rendered response
	out io.Writer

	// printErr is the first error hit while writing output; once set, rendering short-circuits.
	printErr error

	// path is the current JSON path during the walk, used for error `path` and
	// defer subPath computation.
	path []fastjsonext.PathElement

	// depth is the current object-nesting depth of the walk (e.g. root is depth < 2).
	depth int

	// operationType is the operation kind (query/mutation/subscription). It only
	// supplies the root type name prefix (Query/Mutation/Subscription) when
	// rendering error paths and field coordinates.
	operationType ast.OperationType

	// renameTypeNames holds the __typename rewrite rules, applied while rendering.
	renameTypeNames []RenameTypeName

	// ctx is the request Context (authorizer, rate limiter, field renderer, options).
	ctx *Context

	// authorization holds the per-request field-authorization decisions, shared with the Loader.
	// Set via SetFieldAuthorization by the resolver entry points; lazily created in Init as a
	// fallback for directly constructed Resolvables (tests).
	authorization *FieldAuthorization

	// unreachedAuthWalk arms the synthetic authorization descent (pre-fetch mode, initial
	// pre-render walk only): where the data ends but the plan continues, the walk descends the
	// plan alone to emit errors for denied protected fields the data walk cannot reach.
	unreachedAuthWalk bool

	// inUnreachedSubtree is true while inside such a descent; ordinary null semantics are
	// suppressed there — the walk only reads the decision cache and emits errors.
	inUnreachedSubtree bool

	// authorizationError holds an auth error raised mid-walk;
	// in case of defer it is scoped to the current field/defer and converted into a defer local error.
	authorizationError error

	// wroteErrors records whether the `errors` array has been written to the response
	wroteErrors bool

	// wroteData records whether the `data` section has been written; together with
	// wroteErrors it detects the errors-without-data case.
	wroteData bool

	// skipValueCompletion suppresses value-completion extension output for this
	// render (set from the loader when a fetch had errors but no data).
	skipValueCompletion bool

	// deferMode switches rendering between a single complete response and
	// incremental (@defer) delivery.
	// when enabled, we walk object fields in a different way
	deferMode bool

	// currentDefer is the defer descriptor currently being rendered (nil for the
	// initial response frame).
	currentDefer *DeferDescriptor

	// deferDescriptors holds every defer descriptor for the operation, keyed by defer id.
	deferDescriptors map[int]DeferDescriptor

	// typeNames is a stack of the runtime `__typename` at each object layer; it is
	// indexed by depth to evaluate `... on Type` fragment type conditions.
	typeNames [][]byte

	// marshalBuf is a reusable scratch buffer for marshaling scalar values before
	// writing them out.
	marshalBuf []byte

	// enclosingTypeNames is a stack of the schema type name (Object.TypeName) per
	// object layer; enclosingTypeName() returns the current one for error messages.
	enclosingTypeNames []string

	// currentFieldInfo is the FieldInfo of the field currently being rendered,
	// passed to a custom field-value renderer.
	currentFieldInfo *FieldInfo

	// typeNameStats maps the JSON path to its accumulated array/object stats in the final response.
	// Used to compute the actual cost of the operation.
	// Only populated when ResolvableOptions.EnableCostControl is true.
	typeNameStats map[string]TypeNameStats

	// subgraphExtensions holds the `extensions` objects collected from subgraph
	// fetches, merged into the response extensions during rendering.
	subgraphExtensions []*astjson.Object

	// allowedExtensions filters which subgraph extension keys are forwarded into
	// the response.
	allowedExtensions map[string]*astjson.Value

	// deferIncrementalItemWritten records whether an incremental item has already been
	// written in the current defer batch, so the next item is separated from it by
	// a comma. Reset to false at the start of every batch and on Init.
	deferIncrementalItemWritten bool

	// deferItemDataNull marks that the deferred fragment produced no deliverable
	// data — null-bubbled through a non-nullable chain, or scoped out by an
	// authorizer error — so its error goes on the completed entry instead of an
	// incremental item. Reset at the start of every batch (ResolveDeferBatch) and
	// on Init, so it never leaks across defer batches.
	deferItemDataNull bool
}

type TypeNameStats struct {
	Size      int            // the Size of the resolved array/list. It is 1 for non-list objects.
	TypeNames map[string]int // distribution of TypeNames in the array
}

type ResolvableOptions struct {
	ApolloCompatibilityValueCompletionInExtensions bool
	ApolloCompatibilityTruncateFloatValues         bool
	ApolloCompatibilitySuppressFetchErrors         bool
	ApolloCompatibilityReplaceInvalidVarError      bool
	AllowedSubgraphExtensions                      map[string]struct{}
	ExtensionForwardingAlgorithm                   ExtensionForwardingAlgorithm

	// EnableCostControl gates whether typeNameStats are computed during the walk.
	EnableCostControl bool
}

type ExtensionForwardingAlgorithm string

const (
	ExtensionForwardingAlgorithmFirstWrite ExtensionForwardingAlgorithm = "first_write"
	ExtensionForwardingAlgorithmLastWrite  ExtensionForwardingAlgorithm = "last_write"
)

func (a ExtensionForwardingAlgorithm) isValid() bool {
	switch a {
	case ExtensionForwardingAlgorithmFirstWrite, ExtensionForwardingAlgorithmLastWrite:
		return true
	default:
		return false
	}
}

func MapExtensionForwardingAlgorithm(algorithm string) ExtensionForwardingAlgorithm {
	switch ExtensionForwardingAlgorithm(algorithm) {
	case ExtensionForwardingAlgorithmFirstWrite, ExtensionForwardingAlgorithmLastWrite:
		return ExtensionForwardingAlgorithm(algorithm)
	default:
		return ExtensionForwardingAlgorithmFirstWrite
	}
}

func NewResolvable(a arena.Arena, options ResolvableOptions) *Resolvable {
	return &Resolvable{
		options:       options,
		astjsonArena:  a,
		typeNameStats: make(map[string]TypeNameStats),
	}
}

// SetFieldAuthorization wires the per-request field-authorization decisions produced and read
// during resolution. The resolver entry points call it right after NewResolvable; when unset,
// Init creates one from the request Context.
func (r *Resolvable) SetFieldAuthorization(authorization *FieldAuthorization) {
	r.authorization = authorization
}

func (r *Resolvable) Reset() {
	r.parsers = r.parsers[:0]
	r.typeNames = r.typeNames[:0]
	r.enclosingTypeNames = r.enclosingTypeNames[:0]
	r.wroteErrors = false
	r.wroteData = false
	r.skipValueCompletion = false
	r.data = nil
	r.errors = nil
	r.valueCompletion = nil
	r.depth = 0
	r.enableRender = false
	r.out = nil
	r.printErr = nil
	r.path = r.path[:0]
	r.operationType = ast.OperationTypeUnknown
	r.renameTypeNames = r.renameTypeNames[:0]
	r.authorization = nil
	r.unreachedAuthWalk = false
	r.inUnreachedSubtree = false
	r.authorizationError = nil
	r.astjsonArena = nil
	r.allowedExtensions = nil
	clear(r.subgraphExtensions)
	clear(r.typeNameStats)

	r.deferMode = false
	r.currentDefer = nil
	r.deferDescriptors = nil
	r.enableDeferRender = false
	r.deferIncrementalItemWritten = false
	r.deferItemDataNull = false
}

// initCostControl prepares typeNameStats collection for this walk when cost control is active.
func (r *Resolvable) initCostControl() {
	if r.options.EnableCostControl && r.typeNameStats == nil {
		r.typeNameStats = make(map[string]TypeNameStats)
	}
}

func (r *Resolvable) Init(ctx *Context, initialData []byte, operationType ast.OperationType) (err error) {
	r.ctx = ctx
	if r.authorization == nil {
		r.authorization = NewFieldAuthorization(ctx)
	}
	r.operationType = operationType
	r.renameTypeNames = ctx.RenameTypeNames
	r.initCostControl()
	r.data = astjson.ObjectValue(r.astjsonArena)
	// don't init errors! It will heavily increase memory usage
	r.errors = nil
	if initialData != nil {
		initialValue, err := astjson.ParseBytesWithArena(r.astjsonArena, initialData)
		if err != nil {
			return err
		}
		r.data, _, err = astjson.MergeValues(r.astjsonArena, r.data, initialValue)
		if err != nil {
			return err
		}
	}
	return
}

func (r *Resolvable) InitSubscription(ctx *Context, initialData []byte, postProcessing PostProcessingConfiguration) (err error) {
	r.ctx = ctx
	if r.authorization == nil {
		r.authorization = NewFieldAuthorization(ctx)
	}
	r.operationType = ast.OperationTypeSubscription
	r.renameTypeNames = ctx.RenameTypeNames
	r.initCostControl()
	// don't init errors! It will heavily increase memory usage
	r.errors = nil
	if initialData != nil {
		initialValue, err := astjson.ParseBytesWithArena(r.astjsonArena, initialData)
		if err != nil {
			return err
		}
		if postProcessing.SelectResponseDataPath == nil {
			r.data, _, err = astjson.MergeValuesWithPath(r.astjsonArena, r.data, initialValue, postProcessing.MergePath...)
			if err != nil {
				return err
			}
		} else {
			selectedInitialValue := initialValue.Get(postProcessing.SelectResponseDataPath...)
			if selectedInitialValue != nil {
				r.data, _, err = astjson.MergeValuesWithPath(r.astjsonArena, r.data, selectedInitialValue, postProcessing.MergePath...)
				if err != nil {
					return err
				}
			}
		}
		if postProcessing.SelectResponseErrorsPath != nil {
			selectedInitialErrors := initialValue.Get(postProcessing.SelectResponseErrorsPath...)
			if selectedInitialErrors != nil {
				r.errors = selectedInitialErrors
			}
		}
	}
	if r.data == nil {
		r.data = astjson.ObjectValue(r.astjsonArena)
	}
	return
}

func (r *Resolvable) ResolveNode(node Node, data *astjson.Value, out io.Writer) error {
	r.out = out
	r.enableRender = false
	r.printErr = nil
	r.authorizationError = nil
	// don't init errors! It will heavily increase memory usage
	r.errors = nil

	hasErrors := r.walkNode(node, data)
	if hasErrors {
		return fmt.Errorf("error resolving node")
	}

	r.enableRender = true
	hasErrors = r.walkNode(node, data)
	if hasErrors {
		return fmt.Errorf("error resolving node: %w", r.printErr)
	}
	return nil
}

func (r *Resolvable) Resolve(ctx context.Context, rootData *Object, fetchTree *FetchTreeNode, out io.Writer) error {
	r.out = out
	r.enableRender = false
	r.printErr = nil
	r.authorizationError = nil

	if r.ctx.ExecutionOptions.SkipLoader {
		// we didn't resolve any data, so there's no point in generating errors
		// the goal is to only render extensions, e.g. to expose the query plan
		r.printBytes(lBrace)
		r.printBytes(quote)
		r.printBytes(literalData)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(null)
		if r.hasExtensions() {
			r.printBytes(comma)
			r.printErr = r.printExtensions(ctx, fetchTree)
		}
		r.printBytes(rBrace)
		return r.printErr
	}

	r.skipAddingNullErrors = r.hasErrors() && !r.hasData()

	if r.authorization.preFetchEnabled() {
		// Also report denied protected fields the data walk cannot reach (empty list / null
		// parent): past such points the walk descends the plan alone. A denied field stops the
		// descent into its own subtree, so a denied parent is never re-reported via its children.
		r.unreachedAuthWalk = true
	}

	hasErrors := r.walkObject(rootData, r.data)
	r.unreachedAuthWalk = false
	if r.authorizationError != nil {
		return r.authorizationError
	}
	r.printBytes(lBrace)
	if r.hasErrors() {
		r.printErrors()
	}

	if hasErrors {
		r.printBytes(quote)
		r.printBytes(literalData)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(null)
	} else {
		r.printData(rootData)
	}
	if r.hasExtensions() {
		r.printBytes(comma)
		r.printErr = r.printExtensions(ctx, fetchTree)
	}

	if r.deferMode {
		// Announce only the top-level defers whose anchor survived. Nested defers
		// are announced lazily when their parent is released. A recoverable error
		// that null-propagated onto a defer's own anchor cancels just that defer.
		live := r.liveChildDescriptors(0)
		r.printPendingEntries(live)
		r.printHasNext(len(live) > 0)
	}

	r.printBytes(rBrace)

	return r.printErr
}

// ResolveDeferBatch renders the incremental chunk for r.currentDefer, announces
// the pending entries for its direct children whose anchor survived, adjusts the
// outstanding counter (announce children, complete self), writes this frame's
// terminal hasNext, and returns the ids of the live direct children so the caller
// can schedule exactly those. Nested children are announced lazily here, in their
// parent's release frame.
func (r *Resolvable) ResolveDeferBatch(rootData *Object, out io.Writer, outstanding *int64) (liveChildren map[int]DeferDescriptor, err error) {
	r.out = out
	r.printErr = nil
	r.authorizationError = nil

	// First pass (pre-walk): validate, collect errors, decide whether the
	// fragment root survived null-propagation. r.deferItemDataNull is set
	// inside walkObject when null propagated through a non-nullable chain.
	r.enableRender = false
	r.deferMode = true
	r.enableDeferRender = false
	r.deferIncrementalItemWritten = false
	r.deferItemDataNull = false

	_ = r.walkObject(rootData, r.data)
	if r.authorizationError != nil {
		// Scope the authorizer error to this defer: record it as the fragment's
		// error and route it to the completed-with-errors form, completing the
		// announced pending.
		r.addError(r.authorizationError.Error(), nil)
		r.authorizationError = nil
		r.deferItemDataNull = true
	}

	shouldSkipIncremental := r.deferItemDataNull

	// Second pass: render incremental data into a scratch buffer first so a
	// render-phase error (e.g. a custom field-value renderer failing) never leaves
	// a partial frame on the wire — on error the buffer is discarded and the error
	// is scoped to this defer's completed entry.
	var incrementalItems []byte
	if !shouldSkipIncremental {
		savedOut := r.out
		// The scratch buffer is arena-backed (heap fallback when no arena is set),
		// keeping the intermediate bytes on the engine's memory model.
		scratch := arena.NewArenaBuffer(r.astjsonArena)
		r.out = scratch

		r.enableRender = true
		r.deferIncrementalItemWritten = false
		r.enableDeferRender = false

		_ = r.walkObject(rootData, r.data)

		r.out = savedOut
		if r.printErr != nil {
			r.addError(r.printErr.Error(), nil)
			r.printErr = nil
			shouldSkipIncremental = true
		} else {
			incrementalItems = scratch.Bytes()
		}
	}

	// Direct children whose anchor survived the render are announced now (lazily)
	// and scheduled by the caller; the rest are cancelled.
	liveChildren = r.liveChildDescriptors(r.currentDefer.ID)

	// Counter: announce live children, complete self. The frame that drives the
	// outstanding count to zero writes the terminal hasNext:false. Every defer's
	// render runs under dc.db.Lock() (held by the caller), which serialises this
	// mutation with the frame writes.
	*outstanding += int64(len(liveChildren)) - 1
	isLast := *outstanding == 0

	// Open the per-defer envelope.
	r.printBytes(lBrace)

	if !shouldSkipIncremental {
		r.printBytes(quote)
		r.printBytes(literalIncremental)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(lBrack)
		r.printBytes(incrementalItems)
		r.printBytes(rBrack)
		r.printBytes(comma)
	}

	// Always emit completed for this defer id. Errors are attached only when the
	// fragment had no deliverable incremental data (they ride in incremental[]
	// otherwise).
	r.renderCompleted(shouldSkipIncremental && r.hasErrors())

	// Announce the surviving direct children (lazy nested pending). No-op when
	// there are none.
	r.printPendingEntries(liveChildren)

	// hasNext is independent of internal defer errors — they're scoped
	// to this defer's `completed.errors` and do not terminate the response.
	r.printHasNext(!isLast)

	r.printBytes(rBrace)

	return liveChildren, r.printErr
}

// renderCompleted writes `"completed":[{"id":"<n>"[,"errors":[...]]}]` for the
// current defer. When withErrors is true the accumulated r.errors are attached
// to the completed entry (used when the fragment had no deliverable incremental
// data, e.g. it null-bubbled or failed before/around its render).
func (r *Resolvable) renderCompleted(withErrors bool) {
	r.printBytes(quote)
	r.printBytes(literalCompleted)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrack)
	r.printBytes(lBrace)
	// "id":"<n>"
	r.printBytes(quote)
	r.printBytes(literalId)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(quote)
	r.printBytes([]byte(strconv.Itoa(r.currentDefer.ID)))
	r.printBytes(quote)
	if withErrors {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalErrors)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printNode(r.errors)
	}
	r.printBytes(rBrace)
	r.printBytes(rBrack)
}

// ResolveDeferError writes a terminal defer envelope that reports a
// fragment-scoped error on the completed entry (no incremental data) and
// terminates with hasNext. It is used when a deferred group fails in its fetch
// phase (e.g. a hard pre-fetch authorizer/rate-limiter error): the announced
// pending is completed with the error and the multipart stream terminates.
func (r *Resolvable) ResolveDeferError(out io.Writer, message string, outstanding *int64) error {
	r.out = out
	r.printErr = nil
	r.path = r.path[:0]
	r.errors = nil
	r.addError(message, nil)

	// The failing defer completes; drive the outstanding count down by one. The
	// frame that reaches zero writes the terminal hasNext:false. Serialised by
	// dc.db.Lock() (held by the caller), so a plain mutation is safe.
	*outstanding--
	isLast := *outstanding == 0

	// {"completed":[{"id":"<n>","errors":[...]}],"hasNext":<bool>}
	r.printBytes(lBrace)
	r.renderCompleted(true)
	r.printHasNext(!isLast)
	r.printBytes(rBrace)

	return r.printErr
}

func (r *Resolvable) renderPath() {
	r.printBytes(lBrack)
	for i, p := range r.path {
		if i > 0 {
			r.printBytes(comma)
		}
		if p.Name != "" {
			r.printBytes(quote)
			r.printBytes(unsafebytes.StringToBytes(p.Name))
			r.printBytes(quote)
		} else {
			r.printBytes(unsafebytes.StringToBytes(strconv.Itoa(p.Idx)))
		}
	}
	r.printBytes(rBrack)
}

// deferAnchorAlive reports whether the object a @defer fragment is mounted on
// survived the initial render. The initial validation walk sets nullable objects
// to null in r.data when a non-null child null-propagated, so a dead anchor reads
// back as null/absent here. An empty path refers to the root data object.
func (r *Resolvable) deferAnchorAlive(path []string) bool {
	if r.data == nil {
		return false
	}
	v := r.data.Get(path...)
	return v != nil && v.Type() != astjson.TypeNull
}

// liveChildDescriptors returns the descriptors of the defers whose parent is
// parentID and whose anchor survived the render (present and non-null in r.data).
// parentID 0 selects the top-level defers (announced in the initial frame); any
// other id selects that defer's direct children (announced lazily when it is
// released). The result feeds both the pending announcement (printPendingEntries)
// and the execution-tree pruning (pruneDeadDefers), so it carries the full
// descriptors, not just ids.
func (r *Resolvable) liveChildDescriptors(parentID int) map[int]DeferDescriptor {
	var live map[int]DeferDescriptor
	for id, d := range r.deferDescriptors {
		if d.ParentID == parentID && r.deferAnchorAlive(d.Path) {
			if live == nil {
				live = make(map[int]DeferDescriptor)
			}
			live[id] = d
		}
	}
	return live
}

// printPendingEntries writes `,"pending":[...]` listing every descriptor in
// the map, sorted by id ascending. Writes nothing if the map is empty/nil.
func (r *Resolvable) printPendingEntries(descriptors map[int]DeferDescriptor) {
	if len(descriptors) == 0 {
		return
	}
	ids := make([]int, 0, len(descriptors))
	for id := range descriptors {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	r.printBytes(comma)
	r.printBytes(quote)
	r.printBytes(literalPending)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrack)
	for i, id := range ids {
		if i > 0 {
			r.printBytes(comma)
		}
		d := descriptors[id]
		r.printBytes(lBrace)
		// "id":"<n>"
		r.printBytes(quote)
		r.printBytes(literalId)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(quote)
		r.printBytes([]byte(strconv.Itoa(d.ID)))
		r.printBytes(quote)
		// "path":[...]
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalPath)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printPathArray(d.Path)
		// "label":"<l>"  — only if non-empty
		if d.Label != "" {
			r.printBytes(comma)
			r.printBytes(quote)
			r.printBytes(literalLabel)
			r.printBytes(quote)
			r.printBytes(colon)
			r.printBytes(strconv.AppendQuote(nil, d.Label))
		}
		r.printBytes(rBrace)
	}
	r.printBytes(rBrack)
}

// printPathArray writes a precomputed []string path as a JSON string array.
func (r *Resolvable) printPathArray(path []string) {
	r.printBytes(lBrack)
	for i, segment := range path {
		if i > 0 {
			r.printBytes(comma)
		}
		r.printBytes(strconv.AppendQuote(nil, segment))
	}
	r.printBytes(rBrack)
}

func (r *Resolvable) printHasNext(hasNext bool) {
	if r.printErr != nil {
		return
	}
	r.printBytes(comma)
	r.printBytes(quote)
	r.printBytes(literalHasNext)
	r.printBytes(quote)
	r.printBytes(colon)
	if hasNext {
		r.printBytes(literalTrue)
	} else {
		r.printBytes(literalFalse)
	}
}

func (r *Resolvable) printDeferEnvelopeOpen() {
	if !r.render() {
		return
	}

	// Render Incremental Item Envelope: {"data":{...},"path":[...]}
	r.printBytes(lBrace)
	r.printBytes(quote)
	r.printBytes(literalData)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrace)
}

// printDeferIdAndErrors writes "id":"<n>" optionally followed by
// ,"errors":[...] when recoverable errors are pending on this incremental item.
func (r *Resolvable) printDeferIdAndErrors() {
	r.printBytes(quote)
	r.printBytes(literalId)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(quote)
	r.printBytes([]byte(strconv.Itoa(r.currentDefer.ID)))
	r.printBytes(quote)
	r.printDeferSubPathIfAny()
	if r.hasErrors() {
		r.printBytes(comma)
		r.printBytes(quote)
		r.printBytes(literalErrors)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printNode(r.errors)
	}
}

// printDeferSubPathIfAny writes ,"subPath":[...] when the resolver's runtime
// path goes deeper than the current defer's descriptor path.
//
// Rule: subPath = runtime_path − descriptor.path. Walk r.path; track a
// cursor into descriptor.Path. When a runtime segment's name matches the
// cursor's named segment, advance the cursor and skip the segment (it's
// "consumed" by the descriptor prefix). Every other segment — unmatched
// names AND list indices — flows into subPath. Emit nothing when subPath
// is empty.
func (r *Resolvable) printDeferSubPathIfAny() {
	descPath := r.currentDefer.Path
	descIdx := 0

	suffixStart := -1
	for i, p := range r.path {
		if descIdx < len(descPath) && p.Name != "" && p.Name == descPath[descIdx] {
			descIdx++
			continue
		}
		suffixStart = i
		break
	}
	if suffixStart < 0 {
		return
	}

	r.printBytes(comma)
	r.printBytes(quote)
	r.printBytes(literalSubPath)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrack)
	first := true
	for i := suffixStart; i < len(r.path); i++ {
		if !first {
			r.printBytes(comma)
		}
		first = false
		p := r.path[i]
		if p.Name != "" {
			r.printBytes(quote)
			r.printBytes(unsafebytes.StringToBytes(p.Name))
			r.printBytes(quote)
		} else {
			r.printBytes(unsafebytes.StringToBytes(strconv.Itoa(p.Idx)))
		}
	}
	r.printBytes(rBrack)
}

func (r *Resolvable) printDeferEnvelopeClose() {
	if !r.render() {
		return
	}

	r.printBytes(rBrace)
	r.printBytes(comma)
	r.printDeferIdAndErrors()
	r.printBytes(rBrace)
}

// ensureErrorsInitialized is used to lazily init r.errors if needed
func (r *Resolvable) ensureErrorsInitialized() {
	if r.errors == nil {
		r.errors = astjson.ArrayValue(r.astjsonArena)
	}
}

func (r *Resolvable) enclosingTypeName() string {
	if len(r.enclosingTypeNames) > 0 {
		return r.enclosingTypeNames[len(r.enclosingTypeNames)-1]
	}
	return ""
}

func (r *Resolvable) err() bool {
	return true
}

func (r *Resolvable) render() bool {
	if !r.deferMode {
		return r.enableRender
	}

	return r.enableRender && r.enableDeferRender
}

func (r *Resolvable) printErrors() {
	r.printBytes(quote)
	r.printBytes(literalErrors)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printNode(r.errors)
	r.printBytes(comma)
	r.wroteErrors = true
}

func (r *Resolvable) printData(root *Object) {
	r.printBytes(quote)
	r.printBytes(literalData)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrace)
	r.enableRender = true
	_ = r.walkObject(root, r.data)
	r.enableRender = false
	r.printBytes(rBrace)
	r.wroteData = true
}

func (r *Resolvable) printExtensions(ctx context.Context, fetchTree *FetchTreeNode) error {
	r.printBytes(quote)
	r.printBytes(literalExtensions)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrace)

	var writeComma bool

	if r.ctx.authorizer != nil && r.ctx.authorizer.HasResponseExtensionData(r.ctx) {
		writeComma = true
		err := r.printAuthorizerExtension()
		if err != nil {
			return err
		}
	}

	if r.ctx.RateLimitOptions.Enable && r.ctx.RateLimitOptions.IncludeStatsInResponseExtension && r.ctx.rateLimiter != nil {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true
		err := r.printRateLimitingExtension()
		if err != nil {
			return err
		}
	}

	if r.ctx.ExecutionOptions.IncludeQueryPlanInResponse {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true
		err := r.printQueryPlanExtension(fetchTree)
		if err != nil {
			return err
		}
	}

	if r.ctx.TracingOptions.Enable && r.ctx.TracingOptions.IncludeTraceOutputInResponseExtensions {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true
		err := r.printTraceExtension(ctx, fetchTree)
		if err != nil {
			return err
		}
	}

	if !r.skipValueCompletion && r.valueCompletion != nil {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true
		err := r.printValueCompletionExtension()
		if err != nil {
			return err
		}
	}

	if len(r.allowedExtensions) > 0 {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true //nolint:all // should we add another print func, we should not forget to write a comma

		counter := 0
		for key, value := range r.allowedExtensions {
			if counter > 0 {
				r.printBytes(comma)
			}
			counter++
			r.printBytes(quote)
			r.printBytes([]byte(key))
			r.printBytes(quote)
			r.printBytes(colon)
			r.printNode(value)

		}
	}

	r.printBytes(rBrace)
	return nil
}

func (r *Resolvable) printAuthorizerExtension() error {
	r.printBytes(quote)
	r.printBytes(literalAuthorization)
	r.printBytes(quote)
	r.printBytes(colon)
	return r.ctx.authorizer.RenderResponseExtension(r.ctx, r.out)
}

func (r *Resolvable) printRateLimitingExtension() error {
	r.printBytes(quote)
	r.printBytes(literalRateLimit)
	r.printBytes(quote)
	r.printBytes(colon)
	return r.ctx.rateLimiter.RenderResponseExtension(r.ctx, r.out)
}

func (r *Resolvable) printTraceExtension(ctx context.Context, fetchTree *FetchTreeNode) error {
	trace := GetTrace(ctx, fetchTree)
	content, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	r.printBytes(quote)
	r.printBytes(literalTrace)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(content)
	return nil
}

func (r *Resolvable) printQueryPlanExtension(fetchTree *FetchTreeNode) error {
	queryPlan := fetchTree.QueryPlan()
	content, err := json.Marshal(queryPlan)
	if err != nil {
		return err
	}
	r.printBytes(quote)
	r.printBytes(literalQueryPlan)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(content)
	return nil
}

func (r *Resolvable) printValueCompletionExtension() error {
	r.printBytes(quote)
	r.printBytes(literalValueCompletion)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printNode(r.valueCompletion)
	return nil
}

func getDefaultReservedExtensions() map[string]struct{} {
	return map[string]struct{}{
		string(literalAuthorization):   {},
		string(literalRateLimit):       {},
		string(literalQueryPlan):       {},
		string(literalTrace):           {},
		string(literalValueCompletion): {},
	}
}

func (r *Resolvable) hasExtensions() bool {
	// Apply the filter first to avoid missing extensions or applying empty extensions.
	if r.filterAllowedSubgraphExtensions(getDefaultReservedExtensions()) {
		return true
	}
	if r.ctx.authorizer != nil && r.ctx.authorizer.HasResponseExtensionData(r.ctx) {
		return true
	}
	if r.ctx.RateLimitOptions.Enable && r.ctx.RateLimitOptions.IncludeStatsInResponseExtension && r.ctx.rateLimiter != nil {
		return true
	}
	if r.ctx.TracingOptions.Enable && r.ctx.TracingOptions.IncludeTraceOutputInResponseExtensions {
		return true
	}
	if r.ctx.ExecutionOptions.IncludeQueryPlanInResponse {
		return true
	}
	if !r.skipValueCompletion && r.valueCompletion != nil {
		return true
	}
	return false
}

func (r *Resolvable) filterAllowedSubgraphExtensions(writtenExtensions map[string]struct{}) bool {
	if len(r.subgraphExtensions) == 0 {
		return false
	}

	r.allowedExtensions = make(map[string]*astjson.Value)
	algorithm := r.options.ExtensionForwardingAlgorithm

	if !algorithm.isValid() {
		algorithm = ExtensionForwardingAlgorithmFirstWrite
	}

	override := algorithm == ExtensionForwardingAlgorithmLastWrite

	// filter only allowed extensions. If the allowed extensions are empty, all extensions are allowed
	for _, extension := range r.subgraphExtensions {
		extension.Visit(func(key []byte, v *astjson.Value) {
			keyString := string(key)
			if len(r.options.AllowedSubgraphExtensions) > 0 {
				if _, ok := r.options.AllowedSubgraphExtensions[keyString]; !ok {
					return
				}
			}

			// don't print the same extension twice
			if _, exists := writtenExtensions[keyString]; exists {
				return
			}

			// We either add the extension to the valid extension map or we override it when we're in last write mode
			if _, exists := r.allowedExtensions[keyString]; !exists || (exists && override) {
				r.allowedExtensions[string(key)] = v
			}
		})
	}

	return len(r.allowedExtensions) > 0
}

func (r *Resolvable) WroteErrorsWithoutData() bool {
	return r.wroteErrors && !r.wroteData
}

func (r *Resolvable) hasErrors() bool {
	if r.errors == nil {
		return false
	}
	values, err := r.errors.Array()
	if err != nil {
		return false
	}
	return len(values) > 0
}

func (r *Resolvable) hasData() bool {
	if r.data == nil {
		return false
	}
	obj, err := r.data.Object()
	if err != nil {
		return false
	}
	return obj.Len() > 0
}

func (r *Resolvable) printBytes(b []byte) {
	if r.printErr != nil {
		return
	}
	_, r.printErr = r.out.Write(b)
}

func (r *Resolvable) printNode(value *astjson.Value) {
	if r.printErr != nil {
		return
	}
	r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
	_, r.printErr = r.out.Write(r.marshalBuf)
}

func (r *Resolvable) renderEnumValue(value *astjson.Value, nullable bool) {
	if r.printErr != nil {
		return
	}
	r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
	r.renderFieldValue(value, r.marshalBuf, nullable, true)
}

func (r *Resolvable) renderScalarFieldValue(value *astjson.Value, nullable bool) {
	if r.printErr != nil {
		return
	}
	r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
	r.renderFieldValue(value, r.marshalBuf, nullable, false)
}

// renderScalarFieldString - is used when value require some pre-processing, e.g. unescaping or custom rendering
func (r *Resolvable) renderScalarFieldBytes(data []byte, nullable bool) {
	value, err := astjson.ParseBytesWithArena(r.astjsonArena, data)
	if err != nil {
		r.printErr = err
		return
	}

	r.renderFieldValue(value, data, nullable, false)
}

func (r *Resolvable) renderFieldValue(value *astjson.Value, valueBytes []byte, nullable bool, isEnum bool) {
	if r.printErr != nil {
		return
	}
	// if we render a variable that's actually a node, we don't have a context
	// as such, we skip here because this is not rendering the client response
	if r.ctx != nil && r.ctx.fieldRenderer != nil {
		fieldValue := FieldValue{
			Name:       r.currentFieldInfo.Name,
			Type:       r.currentFieldInfo.NamedType,
			ParentType: r.currentFieldInfo.ExactParentTypeName,
			IsNullable: nullable,
			IsEnum:     isEnum,
			Path:       r.renderFieldPath(),
			Data:       valueBytes,
			ParsedData: value,
		}
		if len(r.path) > 0 {
			fieldValue.IsListItem = r.path[len(r.path)-1].Name == ""
		}
		r.printErr = r.ctx.fieldRenderer.RenderFieldValue(r.ctx, fieldValue, r.out)
		if r.printErr != nil {
			return
		}
	} else {
		_, r.printErr = r.out.Write(valueBytes)
	}
}

func (r *Resolvable) pushArrayPathElement(index int) {
	r.path = append(r.path, fastjsonext.PathElement{
		Idx: index,
	})
}

func (r *Resolvable) popArrayPathElement() {
	r.path = r.path[:len(r.path)-1]
}

func (r *Resolvable) pushNodePathElement(path []string) {
	r.depth++
	for i := range path {
		r.path = append(r.path, fastjsonext.PathElement{
			Name: path[i],
		})
	}
}

func (r *Resolvable) popNodePathElement(path []string) {
	r.path = r.path[:len(r.path)-len(path)]
	r.depth--
}

func (r *Resolvable) walkNode(node Node, value *astjson.Value) bool {
	if r.authorizationError != nil {
		return true
	}
	switch n := node.(type) {
	case *Object:
		return r.walkObject(n, value)
	case *Array:
		return r.walkArray(n, value)
	case *Null:
		return r.walkNull()
	case *String:
		return r.walkString(n, value)
	case *StaticString:
		return r.walkStaticString(n)
	case *Boolean:
		return r.walkBoolean(n, value)
	case *Integer:
		return r.walkInteger(n, value)
	case *Float:
		return r.walkFloat(n, value)
	case *BigInt:
		return r.walkBigInt(n, value)
	case *Scalar:
		return r.walkScalar(n, value)
	case *EmptyObject:
		return r.walkEmptyObject(n)
	case *EmptyArray:
		return r.walkEmptyArray(n)
	case *CustomNode:
		return r.walkCustom(n, value)
	case *Enum:
		return r.walkEnum(n, value)
	default:
		return false
	}
}

func (r *Resolvable) walkObject(obj *Object, parent *astjson.Value) (hasError bool) {
	r.enclosingTypeNames = append(r.enclosingTypeNames, obj.TypeName)
	defer func() {
		r.enclosingTypeNames = r.enclosingTypeNames[:len(r.enclosingTypeNames)-1]
	}()
	value := parent.Get(obj.Path...)
	if value == nil || value.Type() == astjson.TypeNull {
		if r.unreachedAuthWalk {
			r.pushNodePathElement(obj.Path)
			r.walkUnreachedFields(obj)
			r.popNodePathElement(obj.Path)
			if r.inUnreachedSubtree {
				// synthetic level: no data to render or null-propagate
				return false
			}
		}
		if obj.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(obj.Path, parent)
		return r.err()
	}
	r.pushNodePathElement(obj.Path)
	isRoot := r.depth < 2
	defer r.popNodePathElement(obj.Path)
	if value.Type() != astjson.TypeObject {
		r.addError("Object cannot represent non-object value.", obj.Path)
		return r.err()
	}

	typeName := value.GetStringBytes("__typename")
	if typeName != nil && len(obj.PossibleTypes) > 0 {
		// when we have a typename field present in a json object, we need to check if the type is valid

		if _, ok := obj.PossibleTypes[string(typeName)]; !ok {
			if !r.render() {
				// during pre-walk we need to add an error when the typename do not match a possible type
				if r.options.ApolloCompatibilityValueCompletionInExtensions {
					r.addValueCompletion(fmt.Sprintf("Invalid __typename found for object at %s.", r.pathLastElementDescription(obj.TypeName)), errorcodes.InvalidGraphql)
				} else {
					r.addErrorWithCode(fmt.Sprintf("Subgraph '%s' returned invalid value '%s' for __typename field.", obj.SourceName, string(typeName)), errorcodes.InvalidGraphql)
				}

				// if object is not nullable at pre-walk we need to return an error
				// to immediately stop the resolving of the current object and bubble up null
				if !obj.Nullable {
					return r.err()
				}

				// if object is nullable we can just set it to null
				// so return no error here
				return false
			} else {
				// at print walk we will render the object to null if it was nullable
				// in case it is not nullable - we already reported an error and won't walk this object again
				return r.walkNull()
			}
		}
	}

	if !r.render() && r.options.EnableCostControl {
		r.recordObjectTypeStats(obj, typeName) // For Cost Control
	}

	// render opening object brace for defer and non defer situation
	if r.render() && !isRoot {
		r.printBytes(lBrace)
	}

	r.typeNames = append(r.typeNames, typeName)
	defer func() {
		r.typeNames = r.typeNames[:len(r.typeNames)-1]
	}()

	if !r.deferMode {
		if r.walkFields(obj, value, parent, walkFieldsFilter{}) {
			return true
		}

		// close the object brace for non defer mode
		if r.render() && !isRoot {
			r.printBytes(rBrace)
		}
		return false
	}

	renderFields, passThroughFields := r.collectDeferFields(obj)

	if len(renderFields) > 0 {
		startedRender := false

		if !r.enableDeferRender {
			r.enableDeferRender = true
			startedRender = true

			if r.enableRender && r.deferIncrementalItemWritten {
				r.printBytes(comma)
			}

			if r.currentDefer != nil {
				r.printDeferEnvelopeOpen()
			}
		}

		// render the initial batch of fields
		hasErrors := r.walkFields(obj, value, parent, walkFieldsFilter{renderFields: renderFields, passThrough: false, enabled: true})

		if startedRender {
			if r.currentDefer != nil {
				if !r.enableRender && hasErrors {
					// Pre-walk: null propagated through a non-nullable chain; signal render pass.
					r.deferItemDataNull = true
				}
				r.printDeferEnvelopeClose()
				r.deferIncrementalItemWritten = true
			}
			r.enableDeferRender = false
		}

		if hasErrors {
			return true
		}
	}

	// we do not search for the other fields when defer is 0 because it is impossible to have non-deferred fields under the deferred parent
	if r.currentDefer != nil && len(passThroughFields) > 0 {
		// look for additional nested fields which may have matching defer id
		if r.walkFields(obj, value, parent, walkFieldsFilter{passThroughFields: passThroughFields, passThrough: true, enabled: true}) {
			return true
		}
	}

	// close the object brace in the defer mode
	if r.render() && !isRoot {
		r.printBytes(rBrace)
	}
	return false
}

func (r *Resolvable) collectDeferFields(obj *Object) (renderFields map[int]struct{}, passThroughFields map[int]struct{}) {
	renderFields = make(map[int]struct{})
	passThroughFields = make(map[int]struct{})

	for i := range obj.Fields {
		if r.shouldSkipFieldByTypeCondition(obj.Fields[i]) {
			continue
		}

		if r.currentDefer == nil {
			// we are rendering the initial response

			// skip all fields with defer
			if obj.Fields[i].Defer != nil {
				continue
			}

			// collect object fields without defer
			renderFields[i] = struct{}{}
		}

		// we are rendering defer response

		// collect fields without defer into passThrough fields
		if obj.Fields[i].Defer == nil {
			if !r.fieldNodeKindAllowsSeek(obj.Fields[i]) {
				continue
			}

			passThroughFields[i] = struct{}{}
			continue
		}

		// allow looking into the fields with other defer ids
		if obj.Fields[i].Defer.DeferID != r.currentDefer.ID {
			// but only if they are ancestor to the current defer id
			if !r.isDeferAncestor(obj.Fields[i].Defer.DeferID, r.currentDefer.ParentID) {
				continue
			}

			if !r.fieldNodeKindAllowsSeek(obj.Fields[i]) {
				continue
			}

			passThroughFields[i] = struct{}{}
			continue
		}

		// store fields with matching defer id
		renderFields[i] = struct{}{}
	}

	return
}

func (r *Resolvable) isDeferAncestor(fieldDeferID, parentID int) bool {
	for {
		// top level defer can't have a parent
		if parentID == 0 {
			return false
		}

		if fieldDeferID == parentID {
			return true
		}

		descriptor := r.deferDescriptors[parentID]
		parentID = descriptor.ParentID
	}
}

func (r *Resolvable) fieldNodeKindAllowsSeek(field *Field) bool {
	kind := field.Value.NodeKind()
	if kind != NodeKindObject {
		if kind != NodeKindArray {
			// skip scalar fields
			return false
		}

		// Skip array if its item type is not an object kind.
		if field.Value.(*Array).Item.NodeKind() != NodeKindObject {
			// we could have a nested array,
			// but we do not care for now
			return false
		}
	}

	return true
}

type walkFieldsFilter struct {
	renderFields      map[int]struct{}
	passThroughFields map[int]struct{}
	passThrough       bool
	enabled           bool
}

func (r *Resolvable) walkFields(obj *Object, value *astjson.Value, parent *astjson.Value, filter walkFieldsFilter) (hasErrors bool) {
	addComma := false

	for i := range obj.Fields {
		if filter.enabled {
			// if mode is passThrough
			if filter.passThrough {
				// skip all fields to which we should not go into
				if _, ok := filter.passThroughFields[i]; !ok {
					continue
				}
			} else {
				// if mode is render
				// skip all fields that we should not render
				if _, ok := filter.renderFields[i]; !ok {
					continue
				}
			}
		} else {
			if r.shouldSkipFieldByTypeCondition(obj.Fields[i]) {
				continue
			}
		}

		if !r.render() {
			skip := r.authorizeField(value, obj.Fields[i])
			if skip {
				if obj.Fields[i].Value.NodeNullable() {
					// if the field value is nullable, we can just set it to null
					// we already set an error in authorizeField
					path := obj.Fields[i].Value.NodePath()
					field := value.Get(path...)
					if field != nil {
						astjson.SetNull(r.astjsonArena, value, path...)
					}

					continue
				} else if obj.Nullable && len(obj.Path) > 0 {
					// if the field value is not nullable, but the object is nullable
					// we can just set the whole object to null
					astjson.SetNull(r.astjsonArena, parent, obj.Path...)
					return false
				}

				// if the field value is not nullable and the object is not nullable
				// we return true to indicate an error
				return true
			}
		}
		if r.render() {
			if addComma {
				r.printBytes(comma)
			}
			r.printBytes(quote)
			r.printBytes(obj.Fields[i].Name)
			r.printBytes(quote)
			r.printBytes(colon)
		}
		r.currentFieldInfo = obj.Fields[i].Info
		err := r.walkNode(obj.Fields[i].Value, value)
		if err {
			if r.render() {
				// Field key already written; complete with null to produce valid JSON.
				r.printBytes(null)
				if obj.Nullable {
					// Nullable parent: absorb the error, render null, continue to next field.
					addComma = true
					continue
				}
				// Non-nullable parent: propagate error; caller closes the envelope.
				return err
			}
			if obj.Nullable {
				if len(obj.Path) > 0 {
					astjson.SetNull(r.astjsonArena, parent, obj.Path...)
					return false
				}
			}
			return err
		}
		addComma = true
	}

	return false
}

func (r *Resolvable) shouldSkipFieldByTypeCondition(field *Field) bool {
	if field.ParentOnTypeNames != nil && r.skipFieldOnParentTypeNames(field) {
		return true
	}

	if field.OnTypeNames != nil && r.skipFieldOnTypeNames(field) {
		return true
	}

	return false
}

func (r *Resolvable) authorizeField(value *astjson.Value, field *Field) (skipField bool) {
	if field.Info == nil {
		return false
	}
	if !field.Info.HasAuthorizationRule {
		return false
	}
	if r.ctx.authorizer == nil && !r.authorization.preFetchEnabled() {
		return false
	}
	if len(field.Info.Source.IDs) == 0 {
		return false
	}
	dataSourceID := field.Info.Source.IDs[0]
	dataSourceName := field.Info.Source.Names[0]
	typeName := r.objectFieldTypeName(value, field)
	if r.authorization.preFetchEnabled() {
		typeName = field.Info.ExactParentTypeName
	}
	gc := GraphCoordinate{
		TypeName:  typeName,
		FieldName: field.Info.Name,
	}
	result, authErr := r.authorization.decide(value, dataSourceID, gc)
	if authErr != nil {
		r.authorizationError = authErr
		return true
	}
	if result != nil {
		r.addRejectFieldError(result.Reason, DataSourceInfo{
			ID:   dataSourceID,
			Name: dataSourceName,
		}, field)
		return true
	}
	return false
}

func (r *Resolvable) addRejectFieldError(reason string, ds DataSourceInfo, field *Field) {
	nodePath := field.Value.NodePath()
	r.pushNodePathElement(nodePath)
	fieldPath := r.renderFieldPath()

	var errorMessage string
	if reason == "" {
		errorMessage = fmt.Sprintf("Unauthorized to load field '%s'.", fieldPath)
	} else {
		errorMessage = fmt.Sprintf("Unauthorized to load field '%s', Reason: %s.", fieldPath, reason)
	}
	r.ctx.appendSubgraphErrors(ds, errors.New(errorMessage),
		NewSubgraphError(ds, fieldPath, reason, 0))
	r.ensureErrorsInitialized()
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.errors, errorMessage, errorcodes.UnauthorizedFieldOrType, r.path)
	r.popNodePathElement(nodePath)
}

func (r *Resolvable) objectFieldTypeName(v *astjson.Value, field *Field) string {
	typeName := v.GetStringBytes("__typename")
	if typeName != nil {
		return unsafebytes.BytesToString(typeName)
	}
	return field.Info.ExactParentTypeName
}

// walkUnreachedFields descends the plan below a point the data walk cannot reach (null/missing
// object, empty array) and emits UNAUTHORIZED_FIELD_OR_TYPE errors for denied protected fields
// there. A denied field stops the descent — its error covers its subtree. Decisions are pure
// cache reads (pre-fetch mode only); no authorizer calls, no data mutation. Recursion goes
// through the regular walk functions with a null value, re-entering their null branches, so
// r.path — and with it error paths and messages — comes from the ordinary walk bookkeeping.
func (r *Resolvable) walkUnreachedFields(obj *Object) {
	if obj == nil {
		return
	}
	wasInside := r.inUnreachedSubtree
	r.inUnreachedSubtree = true
	for i := range obj.Fields {
		field := obj.Fields[i]
		if r.emitUnreachedFieldDeny(field) {
			continue
		}
		switch field.Value.NodeKind() {
		case NodeKindObject, NodeKindArray:
			r.walkNode(field.Value, astjson.NullValue)
		}
	}
	r.inUnreachedSubtree = wasInside
}

// walkUnreachedItem descends into an array item the data walk has no element for (empty or null
// array)
func (r *Resolvable) walkUnreachedItem(item Node) {
	switch item.NodeKind() {
	case NodeKindObject, NodeKindArray:
	default:
		return
	}

	wasInside := r.inUnreachedSubtree
	r.inUnreachedSubtree = true

	// push the "@" wildcard position any element would occupy
	r.pushNodePathElement([]string{"@"})
	r.walkNode(item, astjson.NullValue)
	r.popNodePathElement([]string{"@"})

	r.inUnreachedSubtree = wasInside
}

// emitUnreachedFieldDeny reports whether the field carries a seeded deny decision, emitting the
// corresponding UNAUTHORIZED_FIELD_OR_TYPE error if so.
func (r *Resolvable) emitUnreachedFieldDeny(field *Field) bool {
	if field.Info == nil || !field.Info.HasAuthorizationRule || len(field.Info.Source.IDs) == 0 {
		return false
	}
	dataSourceID := field.Info.Source.IDs[0]
	reason, denied := r.authorization.denyReason(dataSourceID, GraphCoordinate{
		TypeName:  field.Info.ExactParentTypeName,
		FieldName: field.Info.Name,
	})
	if !denied {
		return false
	}
	r.addRejectFieldError(reason, DataSourceInfo{
		ID:   dataSourceID,
		Name: firstString(field.Info.Source.Names),
	}, field)
	return true
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (r *Resolvable) skipFieldOnParentTypeNames(field *Field) bool {
WithNext:
	for i := range field.ParentOnTypeNames {
		typeName := r.typeNames[len(r.typeNames)-1-field.ParentOnTypeNames[i].Depth]
		if typeName == nil {
			// The field has a condition but the JSON response object does not have a __typename field
			// We skip this field
			return true
		}
		for j := range field.ParentOnTypeNames[i].Names {
			if bytes.Equal(typeName, field.ParentOnTypeNames[i].Names[j]) {
				// on each layer of depth, we only need to match one of the names
				// merge_fields.go ensures that we only have on ParentOnTypeNames per depth layer
				// If we have a match, we continue WithNext condition until all layers have been checked
				continue WithNext
			}
		}
		// No match at this depth layer, we skip this field
		return true
	}
	// all layers have at least one matching typeName
	// we don't skip this field (we return false)
	return false
}

func (r *Resolvable) skipFieldOnTypeNames(field *Field) bool {
	typeName := r.typeNames[len(r.typeNames)-1]
	if typeName == nil {
		return true
	}
	for i := range field.OnTypeNames {
		if bytes.Equal(typeName, field.OnTypeNames[i]) {
			return false
		}
	}
	return true
}

func (r *Resolvable) walkArray(arr *Array, value *astjson.Value) bool {
	parent := value
	value = value.Get(arr.Path...)
	if astjson.ValueIsNull(value) {
		if r.unreachedAuthWalk {
			r.pushNodePathElement(arr.Path)
			r.walkUnreachedItem(arr.Item)
			r.popNodePathElement(arr.Path)
			if r.inUnreachedSubtree {
				// synthetic level: no data to render or null-propagate
				return false
			}
		}
		if arr.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(arr.Path, parent)
		return r.err()
	}
	r.pushNodePathElement(arr.Path)
	defer r.popNodePathElement(arr.Path)
	if value.Type() != astjson.TypeArray {
		r.addError("Array cannot represent non-array value.", arr.Path)
		return r.err()
	}
	if r.render() {
		r.printBytes(lBrack)
	}
	values := value.GetArray()

	if len(values) == 0 && r.unreachedAuthWalk && !r.inUnreachedSubtree {
		// no elements to walk: check the item's plan subtree for denied protected fields
		r.walkUnreachedItem(arr.Item)
	}

	if !r.render() && r.options.EnableCostControl {
		// Record arrays stats for Cost Control.
		pathKey := r.currentFieldPath()
		stats := r.typeNameStats[pathKey]
		stats.Size += len(values)
		if stats.TypeNames == nil && len(values) > 0 {
			stats.TypeNames = make(map[string]int)
		}
		for _, arrayValue := range values {
			var typeName string
			if b := arrayValue.GetStringBytes("__typename"); b != nil {
				typeName = string(b)
			} else if obj, ok := arr.Item.(*Object); ok {
				typeName = obj.TypeName
			}
			if typeName != "" {
				stats.TypeNames[typeName]++
			}
		}
		r.typeNameStats[pathKey] = stats
	}

	hasPrintedValue := false
	for i, arrayValue := range values {
		skip := false
		if r.render() && arr.SkipItem != nil {
			skip = arr.SkipItem(r.ctx, arrayValue)
		}

		if skip {
			continue
		}

		if r.render() && i != 0 && hasPrintedValue {
			r.printBytes(comma)
		}

		hasPrintedValue = true

		r.pushArrayPathElement(i)
		err := r.walkNode(arr.Item, arrayValue)
		r.popArrayPathElement()
		if err {
			if arr.Item.NodeKind() == NodeKindObject && arr.Item.NodeNullable() {
				value.SetArrayItem(r.astjsonArena, i, astjson.NullValue)
				continue
			}
			if arr.Nullable {
				astjson.SetNull(r.astjsonArena, parent, arr.Path...)
				return false
			}
			return err
		}
	}
	if r.render() {
		r.printBytes(rBrack)
	}
	return false
}

// recordObjectTypeStats records the runtime __typename of a single (non-array) object.
func (r *Resolvable) recordObjectTypeStats(obj *Object, typeName []byte) {
	// An array item Object has an empty Path
	if len(obj.Path) == 0 {
		return
	}
	pathKey := r.currentFieldPath()
	stats := r.typeNameStats[pathKey]
	stats.Size++
	if stats.TypeNames == nil {
		stats.TypeNames = make(map[string]int, 1)
	}
	// Fall back to the declared abstract type name when the subgraph did not return __typename.
	name := obj.TypeName
	if typeName != nil {
		name = string(typeName)
	}
	stats.TypeNames[name]++
	r.typeNameStats[pathKey] = stats
}

// Helper to build JSON path (field names only, no array indices)
func (r *Resolvable) currentFieldPath() string {
	var parts []string
	for _, elem := range r.path {
		if elem.Name != "" {
			parts = append(parts, elem.Name)
		}
	}
	return strings.Join(parts, ".")
}

func (r *Resolvable) walkNull() bool {
	if r.render() {
		r.printBytes(null)
	}
	return false
}

func (r *Resolvable) walkStaticString(str *StaticString) bool {
	if r.render() {
		r.printBytes(quote)
		r.printBytes([]byte(str.Value))
		r.printBytes(quote)
	}
	return false
}

func (r *Resolvable) walkString(s *String, value *astjson.Value) bool {
	parent := value
	value = value.Get(s.Path...)
	if astjson.ValueIsNull(value) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path, parent)
		return r.err()
	}
	if value.Type() != astjson.TypeString {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("String cannot represent non-string value: \"%s\"", string(r.marshalBuf)), s.Path)
		return r.err()
	}
	if r.render() {
		if s.IsTypeName {
			content := value.GetStringBytes()
			for i := range r.renameTypeNames {
				if bytes.Equal(content, r.renameTypeNames[i].From) {
					r.printBytes(quote)
					r.printBytes(r.renameTypeNames[i].To)
					r.printBytes(quote)
					return false
				}
			}
			r.printNode(value)
			return false
		}
		if s.UnescapeResponseJson {
			content := value.GetStringBytes()
			content = bytes.ReplaceAll(content, []byte(`\"`), []byte(`"`))
			if !gjson.ValidBytes(content) {
				r.printBytes(quote)
				r.printBytes(content)
				r.printBytes(quote)
			} else {
				r.renderScalarFieldBytes(content, s.Nullable)
			}
		} else {
			r.renderScalarFieldValue(value, s.Nullable)
		}
	}
	return false
}

func (r *Resolvable) walkBoolean(b *Boolean, value *astjson.Value) bool {
	parent := value
	value = value.Get(b.Path...)
	if astjson.ValueIsNull(value) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path, parent)
		return r.err()
	}
	if value.Type() != astjson.TypeTrue && value.Type() != astjson.TypeFalse {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("Bool cannot represent non-boolean value: \"%s\"", string(r.marshalBuf)), b.Path)
		return r.err()
	}
	if r.render() {
		r.renderScalarFieldValue(value, b.Nullable)
	}
	return false
}

func (r *Resolvable) walkInteger(i *Integer, value *astjson.Value) bool {
	parent := value
	value = value.Get(i.Path...)
	if astjson.ValueIsNull(value) {
		if i.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(i.Path, parent)
		return r.err()
	}
	if value.Type() != astjson.TypeNumber {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("Int cannot represent non-integer value: \"%s\"", string(r.marshalBuf)), i.Path)
		return r.err()
	}
	if r.render() {
		r.renderScalarFieldValue(value, i.Nullable)
	}
	return false
}

func (r *Resolvable) walkFloat(f *Float, value *astjson.Value) bool {
	parent := value
	value = value.Get(f.Path...)
	if astjson.ValueIsNull(value) {
		if f.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(f.Path, parent)
		return r.err()
	}
	if !r.render() {
		if value.Type() != astjson.TypeNumber {
			r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
			r.addError(fmt.Sprintf("Float cannot represent non-float value: \"%s\"", string(r.marshalBuf)), f.Path)
			return r.err()
		}
	}
	if r.render() {
		if r.options.ApolloCompatibilityTruncateFloatValues {
			floatValue := value.GetFloat64()
			if floatValue == float64(int64(floatValue)) {
				_, _ = fmt.Fprintf(r.out, "%d", int64(floatValue))
				return false
			}
		}
		r.renderScalarFieldValue(value, f.Nullable)
	}
	return false
}

func (r *Resolvable) walkBigInt(b *BigInt, value *astjson.Value) bool {
	parent := value
	value = value.Get(b.Path...)
	if astjson.ValueIsNull(value) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path, parent)
		return r.err()
	}
	if r.render() {
		r.renderScalarFieldValue(value, b.Nullable)
	}
	return false
}

func (r *Resolvable) walkScalar(s *Scalar, value *astjson.Value) bool {
	parent := value
	value = value.Get(s.Path...)
	if astjson.ValueIsNull(value) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path, parent)
		return r.err()
	}
	if r.render() {
		r.renderScalarFieldValue(value, s.Nullable)
	}
	return false
}

func (r *Resolvable) walkEmptyObject(_ *EmptyObject) bool {
	if r.render() {
		r.printBytes(lBrace)
		r.printBytes(rBrace)
	}
	return false
}

func (r *Resolvable) walkEmptyArray(_ *EmptyArray) bool {
	if r.render() {
		r.printBytes(lBrack)
		r.printBytes(rBrack)
	}
	return false
}

func (r *Resolvable) walkCustom(c *CustomNode, value *astjson.Value) bool {
	parent := value
	value = value.Get(c.Path...)
	if astjson.ValueIsNull(value) {
		if c.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(c.Path, parent)
		return r.err()
	}
	r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
	resolved, err := c.Resolve(r.ctx, r.marshalBuf)
	if err != nil {
		r.addError(err.Error(), c.Path)
		return r.err()
	}
	if r.render() {
		r.renderScalarFieldBytes(resolved, c.Nullable)
	}
	return false
}

func (r *Resolvable) writeArrayElementToBuffer(buf *bytes.Buffer, typeName string) {
	_, _ = buf.WriteString("array element of type ")
	_, _ = buf.WriteString(typeName)
	_, _ = buf.WriteString(" at index ")
	_, _ = buf.WriteString(strconv.Itoa(r.path[len(r.path)-1].Idx))
	_, _ = buf.WriteString(".")
}

func (r *Resolvable) renderInaccessibleEnumValueError(e *Enum) {
	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)
	_, _ = buf.WriteString("Invalid value found for ")
	pathLength := len(r.path)
	// The enum is an array element
	if pathLength > 1 && r.path[pathLength-1].Name == "" {
		r.writeArrayElementToBuffer(buf, e.TypeName)
		if r.options.ApolloCompatibilityValueCompletionInExtensions {
			r.addValueCompletion(buf.String(), errorcodes.InvalidGraphql)
		} else {
			r.addErrorWithCode(buf.String(), errorcodes.InvalidGraphql)
		}
		return
	}
	// The enum is a leaf field
	_, _ = buf.WriteString("field ")
	if e.Path == nil {
		if pathLength < 1 {
			_, _ = buf.WriteString(invalidPath)
		} else {
			_, _ = buf.WriteString(r.renderRootFieldCoordinates(r.path[pathLength-1].Name))
		}
		_, _ = buf.WriteString(".")
		return
	}
	leafPathLength := len(e.Path)
	if leafPathLength < 1 {
		_, _ = buf.WriteString(invalidPath)
		_, _ = buf.WriteString(".")
		return
	}
	switch pathLength {
	case 0:
		_, _ = buf.WriteString(r.renderRootFieldCoordinates(e.Path[leafPathLength-1]))
		_, _ = buf.WriteString(".")
	default:
		_, _ = buf.WriteString(r.enclosingTypeName())
		_, _ = buf.WriteString(".")
		_, _ = buf.WriteString(e.Path[leafPathLength-1])
		_, _ = buf.WriteString(".")
	}
	if r.options.ApolloCompatibilityValueCompletionInExtensions {
		r.addValueCompletionWithPath(buf.String(), errorcodes.InvalidGraphql, e.Path)
	} else {
		r.addErrorWithCodeAndPath(buf.String(), errorcodes.InvalidGraphql, e.Path)
	}
}

func (r *Resolvable) walkEnum(e *Enum, value *astjson.Value) bool {
	parent := value
	value = value.Get(e.Path...)
	if astjson.ValueIsNull(value) {
		if e.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(e.Path, parent)
		return r.err()
	}
	if value.Type() != astjson.TypeString {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addErrorWithCodeAndPath(fmt.Sprintf(`Enum "%s" cannot represent value: %s`, e.TypeName, string(r.marshalBuf)), errorcodes.InternalServerError, e.Path)
		return r.err()
	}
	valueString := string(value.GetStringBytes())
	if !e.isValidValue(valueString) {
		/* When an invalid value is returned, the data is set to null.
		 * If the value is nullable, the null data should not propagate up, so r.walkNull() is returned.
		 * To avoid appending an error twice, the appending only happens on the first walk
		 * and not the second walk (which prints the data).
		 */
		if !r.render() {
			if r.options.ApolloCompatibilityValueCompletionInExtensions {
				r.renderInaccessibleEnumValueError(e)
			} else {
				r.addErrorWithCodeAndPath(fmt.Sprintf(`Enum "%s" cannot represent value: "%s"`, e.TypeName, valueString), errorcodes.InternalServerError, e.Path)
			}
		}
		if e.Nullable {
			return r.walkNull()
		}
		return r.err()
	}
	if !e.isAccessibleValue(valueString) {
		/* When an inaccessible value is returned, the data is set to null.
		 * If the value is nullable, the null data should not propagate up, so r.walkNull() is returned.
		 * To avoid appending an error/value completion twice, the appending only happens on the first walk
		 * and not the second walk (which prints the data).
		 */
		if !r.render() {
			r.renderInaccessibleEnumValueError(e)
		}
		// Inaccessible enum values are always converted to null
		if e.Nullable {
			return r.walkNull()
		}
		return r.err()
	}
	if r.render() {
		r.renderEnumValue(value, e.Nullable)
	}
	return false
}

func (r *Resolvable) addNonNullableFieldError(fieldPath []string, parent *astjson.Value) {
	if r.skipAddingNullErrors {
		return
	}
	if fieldPath != nil {
		if ancestor := parent.Get(fieldPath[:len(fieldPath)-1]...); ancestor != nil {
			if ancestor.Exists("__skipErrors") {
				return
			}
		}
	}
	r.pushNodePathElement(fieldPath)
	if r.options.ApolloCompatibilityValueCompletionInExtensions {
		r.addValueCompletion(r.renderApolloCompatibleNonNullableErrorMessage(), errorcodes.InvalidGraphql)
	} else {
		errorMessage := fmt.Sprintf("Cannot return null for non-nullable field '%s'.", r.renderFieldPath())
		r.ensureErrorsInitialized()
		fastjsonext.AppendErrorToArray(r.astjsonArena, r.errors, errorMessage, r.path)
	}
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) renderFieldPath() string {
	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)
	switch r.operationType {
	case ast.OperationTypeQuery:
		_, _ = buf.WriteString("Query")
	case ast.OperationTypeMutation:
		_, _ = buf.WriteString("Mutation")
	case ast.OperationTypeSubscription:
		_, _ = buf.WriteString("Subscription")
	default:
		return invalidPath
	}
	for i := range r.path {
		if r.path[i].Name != "" {
			_, _ = buf.WriteString(".")
			_, _ = buf.WriteString(r.path[i].Name)
		}
	}
	return buf.String()
}

func (r *Resolvable) renderApolloCompatibleNonNullableErrorMessage() string {
	pathLength := len(r.path)
	if pathLength < 1 {
		return invalidPath
	}
	lastPathItem := r.path[pathLength-1]
	if lastPathItem.Name != "" {
		return fmt.Sprintf("Cannot return null for non-nullable field %s.", r.renderFieldCoordinates())
	}
	// If the item has no name, it's a GraphQL list element. A list must be returned by a field.
	if pathLength < 2 {
		return invalidPath
	}
	return fmt.Sprintf("Cannot return null for non-nullable array element of type %s at index %d.", r.enclosingTypeName(), lastPathItem.Idx)
}

func (r *Resolvable) renderRootFieldCoordinates(fieldName string) string {
	switch r.operationType {
	case ast.OperationTypeQuery:
		return fmt.Sprintf("Query.%s", fieldName)
	case ast.OperationTypeMutation:
		return fmt.Sprintf("Mutation.%s", fieldName)
	case ast.OperationTypeSubscription:
		return fmt.Sprintf("Subscription.%s", fieldName)
	default:
		return invalidPath
	}
}

func (r *Resolvable) renderFieldCoordinates() string {
	pathLength := len(r.path)
	switch pathLength {
	case 0:
		return invalidPath
	case 1:
		return r.renderRootFieldCoordinates(r.path[0].Name)
	default:
		return fmt.Sprintf("%s.%s", r.enclosingTypeName(), r.path[pathLength-1].Name)
	}
}

func (r *Resolvable) addError(message string, fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	r.ensureErrorsInitialized()
	fastjsonext.AppendErrorToArray(r.astjsonArena, r.errors, message, r.path)
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) addErrorWithCode(message, code string) {
	r.ensureErrorsInitialized()
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.errors, message, code, r.path)
}

func (r *Resolvable) addErrorWithCodeAndPath(message, code string, fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	r.ensureErrorsInitialized()
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.errors, message, code, r.path)
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) addValueCompletion(message, code string) {
	if r.valueCompletion == nil {
		r.valueCompletion = astjson.ArrayValue(r.astjsonArena)
	}
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.valueCompletion, message, code, r.path)
}

func (r *Resolvable) addValueCompletionWithPath(message, code string, fieldPath []string) {
	if r.valueCompletion == nil {
		r.valueCompletion = astjson.ArrayValue(r.astjsonArena)
	}
	r.pushNodePathElement(fieldPath)
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.valueCompletion, message, code, r.path)
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) pathLastElementDescription(typeName string) string {
	if len(r.path) <= 1 {
		switch r.operationType {
		case ast.OperationTypeQuery:
			typeName = "Query"
		case ast.OperationTypeMutation:
			typeName = "Mutation"
		case ast.OperationTypeSubscription:
			typeName = "Subscription"
		default:
			typeName = invalidPath
		}

		if len(r.path) == 0 {
			return typeName
		}
	}
	elem := r.path[len(r.path)-1]
	if elem.Name != "" {
		return fmt.Sprintf("field %s.%s", typeName, elem.Name)
	}
	return fmt.Sprintf("array element of type %s at index %d", typeName, elem.Idx)
}
