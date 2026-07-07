package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type ResponseTreeProcessor interface {
	Process(node resolve.Node)
	ProcessSubscription(node resolve.Node)
}

type FetchTreeProcessor interface {
	ProcessFetchTree(root *resolve.FetchTreeNode)
}

// Processor transforms and optimizes the query plan after
// it's been created by the planner but before execution.
type Processor struct {
	disableExtractFetches  bool
	collectDataSourceInfo  bool
	fetchTreeProcessors    *FetchTreeProcessors
	responseTreeProcessors *ResponseTreeProcessors
	extractDeferFetches    *extractDeferFetches
	buildDeferTree         *buildDeferTree
	// caching orchestrates the caching passes; with no EnableCaching option it
	// is a guaranteed no-op (the planner no-op gate lives inside it).
	caching *cache.Configurator
}

type FetchTreeProcessors struct {
	resolveInputTemplates          *resolveInputTemplates
	appendFetchID                  *fetchIDAppender
	dedupe                         *deduplicateSingleFetches
	addMissingNestedDependencies   *addMissingNestedDependencies
	createConcreteSingleFetchTypes *createConcreteSingleFetchTypes
	orderSequenceByDependencies    *orderSequenceByDependencies
	createParallelNodes            *createParallelNodes
}

// processFlatFetchTree - process a flat fetch tree - single serial fetch with flat list of child fetches
func (p *FetchTreeProcessors) processFlatFetchTree(fetches *resolve.FetchTreeNode) {
	p.dedupe.ProcessFetchTree(fetches)
	// Appending fetchIDs makes query content unique, thus it should happen after "dedupe".
	p.appendFetchID.ProcessFetchTree(fetches)
	p.resolveInputTemplates.ProcessFetchTree(fetches)
	p.addMissingNestedDependencies.ProcessFetchTree(fetches)
	p.createConcreteSingleFetchTypes.ProcessFetchTree(fetches)
}

// organizeFetchTree organizes the fetch tree by ordering sequence nodes by dependencies and creating parallel nodes.
// after this step fetches have tree structure of serial and parallel nodes.
func (p *FetchTreeProcessors) organizeFetchTree(fetches *resolve.FetchTreeNode) {
	p.orderSequenceByDependencies.ProcessFetchTree(fetches)
	p.createParallelNodes.ProcessFetchTree(fetches)
}

type ResponseTreeProcessors struct {
	mergeFields *mergeFields
}

type processorOptions struct {
	disableDeduplicateSingleFetches       bool
	disableCreateConcreteSingleFetchTypes bool
	disableOrderSequenceByDependencies    bool
	disableMergeFields                    bool
	disableRewriteOpNames                 bool
	disableResolveInputTemplates          bool
	disableExtractFetches                 bool
	disableCreateParallelNodes            bool
	disableAddMissingNestedDependencies   bool
	collectDataSourceInfo                 bool
	disableExtractDeferFetches            bool
	disableBuildDeferTree                 bool
	cacheProviders                        map[string]cacheconfig.CacheConfigProvider
	cacheFederation                       map[string]plan.FederationMetaData
	cacheDefinition                       *ast.Document
}

type ProcessorOption func(*processorOptions)

func DisableDeduplicateSingleFetches() ProcessorOption {
	return func(o *processorOptions) {
		o.disableDeduplicateSingleFetches = true
	}
}

func DisableCreateConcreteSingleFetchTypes() ProcessorOption {
	return func(o *processorOptions) {
		o.disableCreateConcreteSingleFetchTypes = true
	}
}

func DisableOrderSequenceByDependencies() ProcessorOption {
	return func(o *processorOptions) {
		o.disableOrderSequenceByDependencies = true
	}
}

func DisableMergeFields() ProcessorOption {
	return func(o *processorOptions) {
		o.disableMergeFields = true
	}
}

func DisableResolveInputTemplates() ProcessorOption {
	return func(o *processorOptions) {
		o.disableResolveInputTemplates = true
		o.disableCreateConcreteSingleFetchTypes = true
	}
}

func CollectDataSourceInfo() ProcessorOption {
	return func(o *processorOptions) {
		o.collectDataSourceInfo = true
	}
}

func DisableCreateParallelNodes() ProcessorOption {
	return func(o *processorOptions) {
		o.disableCreateParallelNodes = true
	}
}

func DisableAddMissingNestedDependencies() ProcessorOption {
	return func(o *processorOptions) {
		o.disableAddMissingNestedDependencies = true
	}
}

func DisableExtractDeferFetches() ProcessorOption {
	return func(o *processorOptions) {
		o.disableExtractDeferFetches = true
	}
}

func DisableBuildDeferTree() ProcessorOption {
	return func(o *processorOptions) {
		o.disableBuildDeferTree = true
	}
}

// EnableCaching wires the caching postprocess passes. It is an INTERNAL
// detail: the public entry point is the engine Configuration's SetCaching,
// which builds these inputs (providers and federation keyed by datasource ID,
// plus the composed schema) and passes them through.
func EnableCaching(providers map[string]cacheconfig.CacheConfigProvider, federation map[string]plan.FederationMetaData, definition *ast.Document) ProcessorOption {
	return func(o *processorOptions) {
		o.cacheProviders = providers
		o.cacheFederation = federation
		o.cacheDefinition = definition
	}
}

func NewProcessor(options ...ProcessorOption) *Processor {
	opts := &processorOptions{}
	for _, o := range options {
		o(opts)
	}
	return &Processor{
		collectDataSourceInfo: opts.collectDataSourceInfo,
		disableExtractFetches: opts.disableExtractFetches,
		fetchTreeProcessors: &FetchTreeProcessors{
			resolveInputTemplates: &resolveInputTemplates{
				disable: opts.disableResolveInputTemplates,
			},
			appendFetchID: &fetchIDAppender{
				disable: opts.disableRewriteOpNames,
			},
			dedupe: &deduplicateSingleFetches{
				disable: opts.disableDeduplicateSingleFetches,
			},
			// this must go first, as we need to deduplicate fetches so that subsequent processors can work correctly
			addMissingNestedDependencies: &addMissingNestedDependencies{
				disable: opts.disableAddMissingNestedDependencies,
			},
			// this must go after deduplication because it relies on the existence of a "sequence" fetch node in the root
			createConcreteSingleFetchTypes: &createConcreteSingleFetchTypes{
				disable: opts.disableCreateConcreteSingleFetchTypes,
			},
			orderSequenceByDependencies: &orderSequenceByDependencies{
				disable: opts.disableOrderSequenceByDependencies,
			},
			createParallelNodes: &createParallelNodes{
				disable: opts.disableCreateParallelNodes,
			},
		},
		responseTreeProcessors: &ResponseTreeProcessors{
			mergeFields: &mergeFields{
				disable: opts.disableMergeFields,
			},
		},
		extractDeferFetches: &extractDeferFetches{
			disable: opts.disableExtractDeferFetches,
		},
		buildDeferTree: &buildDeferTree{
			disable: opts.disableBuildDeferTree,
		},
		caching: cache.NewConfigurator(opts.cacheProviders, opts.cacheFederation, opts.cacheDefinition),
	}
}

// Process takes a raw query plan and optimizes it by deduplicating fetches,
// ordering them correctly by dependencies, and resolving any templated inputs.
// It groups already-ordered fetches into parallel execution batches
// when they have the same dependency requirements satisfied.
func (p *Processor) Process(pre plan.Plan) {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Data)
		// initialize the fetch tree
		p.createFetchTree(t.Response)
		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Fetches)
		// caching passes run on the flat tree, after the concrete fetch types exist
		p.caching.ConfigureCaching(t.Response, nil, t.Response.Fetches)
		p.fetchTreeProcessors.organizeFetchTree(t.Response.Fetches)

	case *plan.DeferResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Response.Data)
		p.createFetchTree(t.Response.Response)
		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Response.Fetches)

		// extract deferred fetches into their own fetch trees
		p.extractDeferFetches.Process(t)

		// process the initial response fetch tree
		p.fetchTreeProcessors.organizeFetchTree(t.Response.Response.Fetches)

		// process each deferred response fetch tree
		for _, deferResp := range t.Response.Defers {
			p.fetchTreeProcessors.organizeFetchTree(deferResp.Fetches)
		}

		// order defer fetches into parallel/sequence groups
		p.buildDeferTree.Process(t.Response)

		// caching passes run AFTER the defer tree is built: the group trees
		// and their ancestry come from the AUTHORITATIVE DeferTree the
		// resolver executes (a parent group resolves fully before its
		// children), not from a parallel derivation
		cachingTrees, cachingTreeParents := collectDeferCachingTrees(t.Response)
		p.caching.ConfigureCaching(t.Response.Response, cachingTreeParents, cachingTrees...)
		// emptily defers, as they are now ordered in a separate tree
		t.Response.Defers = nil

	case *plan.SubscriptionResponsePlan:
		p.responseTreeProcessors.mergeFields.Process(t.Response.Response.Data)
		p.createFetchTree(t.Response.Response)
		p.appendTriggerToFetchTree(t.Response)

		p.fetchTreeProcessors.processFlatFetchTree(t.Response.Response.Fetches)
		p.caching.ConfigureCaching(t.Response.Response, nil, t.Response.Response.Fetches)

		// resolve input template for the root query in the subscription trigger
		p.fetchTreeProcessors.resolveInputTemplates.ProcessTrigger(&t.Response.Trigger)

		p.fetchTreeProcessors.organizeFetchTree(t.Response.Response.Fetches)
	}
}

// deferTrees collects the initial response fetch tree plus every defer-group
// tree of a defer plan, so the caching passes see all trees of one response
// (cross-tree passes like optimizeL1Cache need the full set).
// collectDeferCachingTrees gathers the fetch trees the caching passes run
// over — the initial tree plus every defer group — with each tree's parent
// index for the L1 narrowing pass's ancestry ordering. Both come from the
// BUILT DeferTree: a Sequence node's first child is the parent group and the
// remaining children are its nested groups; Parallel children share their
// enclosing parent. A group without fetches contributes no tree, and its
// children attach to the nearest fetch-bearing ancestor (a weaker but still
// sound ordering). Falls back to the flat Defers list (all parented to the
// initial tree) when the defer tree was not built (disableBuildDeferTree).
func collectDeferCachingTrees(response *resolve.GraphQLDeferResponse) ([]*resolve.FetchTreeNode, []int) {
	trees := []*resolve.FetchTreeNode{response.Response.Fetches}
	parents := []int{-1}
	if response.DeferTree == nil {
		for _, group := range response.Defers {
			if group.Fetches == nil {
				continue
			}
			trees = append(trees, group.Fetches)
			parents = append(parents, 0)
		}
		return trees, parents
	}
	var walk func(node *resolve.DeferTreeNode, parentIndex int)
	walk = func(node *resolve.DeferTreeNode, parentIndex int) {
		if node == nil {
			return
		}
		switch node.Kind {
		case resolve.DeferTreeNodeKindSingle:
			if node.Item != nil && node.Item.Fetches != nil {
				trees = append(trees, node.Item.Fetches)
				parents = append(parents, parentIndex)
			}
		case resolve.DeferTreeNodeKindSequence:
			if len(node.ChildNodes) == 0 {
				return
			}
			// buildDeferTree shape: ChildNodes[0] is the parent group, the
			// rest is its child subtree.
			before := len(trees)
			walk(node.ChildNodes[0], parentIndex)
			childParent := parentIndex
			if len(trees) == before+1 {
				childParent = before
			}
			for _, child := range node.ChildNodes[1:] {
				walk(child, childParent)
			}
		case resolve.DeferTreeNodeKindParallel:
			for _, child := range node.ChildNodes {
				walk(child, parentIndex)
			}
		}
	}
	walk(response.DeferTree, 0)
	return trees, parents
}

// createFetchTree creates an initial fetch tree from the raw fetches in the GraphQL response.
// The initial fetch tree is a node of sequence fetch kind, with a flat list of fetches as children.
func (p *Processor) createFetchTree(res *resolve.GraphQLResponse) {
	if p.disableExtractFetches {
		return
	}

	fetches := res.RawFetches
	res.RawFetches = nil

	children := make([]*resolve.FetchTreeNode, len(fetches))

	if p.collectDataSourceInfo {
		var list = make([]resolve.DataSourceInfo, 0, len(fetches))
		for _, fetch := range fetches {
			info := fetch.Fetch.FetchInfo()
			if info != nil {
				dsInfo := resolve.DataSourceInfo{
					ID:   info.DataSourceID,
					Name: info.DataSourceName,
				}
				if !slices.Contains(list, dsInfo) {
					list = append(list, dsInfo)
				}
			}
		}
		res.DataSources = list
	}

	for i := range fetches {
		children[i] = &resolve.FetchTreeNode{
			Kind: resolve.FetchTreeNodeKindSingle,
			Item: fetches[i],
		}
	}
	res.Fetches = &resolve.FetchTreeNode{
		Kind:       resolve.FetchTreeNodeKindSequence,
		ChildNodes: children,
	}
}

func (p *Processor) appendTriggerToFetchTree(sub *resolve.GraphQLSubscription) {
	rootData := sub.Response.Data
	if rootData == nil || len(rootData.Fields) == 0 {
		return
	}

	info := rootData.Fields[0].Info
	if info == nil {
		return
	}

	sub.Response.Fetches.Trigger = &resolve.FetchTreeNode{
		Kind: resolve.FetchTreeNodeKindTrigger,
		Item: &resolve.FetchItem{
			Fetch: &resolve.SingleFetch{
				FetchDependencies: resolve.FetchDependencies{
					FetchID: info.FetchID,
				},
				Info: &resolve.FetchInfo{
					DataSourceID:   info.Source.IDs[0],
					DataSourceName: info.Source.Names[0],
					QueryPlan:      sub.Trigger.QueryPlan,
				},
			},
			ResponsePath: info.Name,
		},
	}
}
