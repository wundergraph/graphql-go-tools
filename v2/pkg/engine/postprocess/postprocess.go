package postprocess

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
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
	disableExtractFetches bool
	collectDataSourceInfo bool
	processFetchTree      *FetchTreeProcessors
	processResponseTree   *ResponseTreeProcessors
	deferProcessor        *deferProcessor
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
	disableDefer                          bool
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

func DisableDefer() ProcessorOption {
	return func(o *processorOptions) {
		o.disableDefer = true
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
		processFetchTree: &FetchTreeProcessors{
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
		processResponseTree: &ResponseTreeProcessors{
			mergeFields: &mergeFields{
				disable: opts.disableMergeFields,
			},
		},
		deferProcessor: &deferProcessor{
			disable: opts.disableDefer,
		},
	}
}

// Process takes a raw query plan and optimizes it by deduplicating fetches,
// ordering them correctly by dependencies, and resolving any templated inputs.
// It groups already-ordered fetches into parallel execution batches
// when they have the same dependency requirements satisfied.
func (p *Processor) Process(pre plan.Plan) {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.processResponseTree.mergeFields.Process(t.Response.Data)
		// initialize the fetch tree
		p.createFetchTree(t.Response)
		// NOTE: deduplication relies on the fact that the fetch tree
		// have flat structure of child fetches
		p.processFetchTree.dedupe.ProcessFetchTree(t.Response.Fetches)
		// Appending fetchIDs makes query content unique, thus it should happen after "dedupe".
		p.processFetchTree.appendFetchID.ProcessFetchTree(t.Response.Fetches)
		p.processFetchTree.resolveInputTemplates.ProcessFetchTree(t.Response.Fetches)
		p.processFetchTree.addMissingNestedDependencies.ProcessFetchTree(t.Response.Fetches)
		p.processFetchTree.createConcreteSingleFetchTypes.ProcessFetchTree(t.Response.Fetches)
		p.processFetchTree.orderSequenceByDependencies.ProcessFetchTree(t.Response.Fetches)
		p.processFetchTree.createParallelNodes.ProcessFetchTree(t.Response.Fetches)
	case *plan.DeferResponsePlan:
		p.processResponseTree.mergeFields.Process(t.RawResponse.Data)
		p.createFetchTree(t.RawResponse)
		p.processFetchTree.dedupe.ProcessFetchTree(t.RawResponse.Fetches)
		p.processFetchTree.appendFetchID.ProcessFetchTree(t.RawResponse.Fetches)
		p.processFetchTree.resolveInputTemplates.ProcessFetchTree(t.RawResponse.Fetches)
		p.processFetchTree.addMissingNestedDependencies.ProcessFetchTree(t.RawResponse.Fetches)
		p.processFetchTree.createConcreteSingleFetchTypes.ProcessFetchTree(t.RawResponse.Fetches)

		// extract deferred fetches and fields into their own fetch trees
		p.deferProcessor.Process(t)

		// process the initial response fetch tree
		p.processFetchTree.orderSequenceByDependencies.ProcessFetchTree(t.InitialResponse.Fetches)
		p.processFetchTree.createParallelNodes.ProcessFetchTree(t.InitialResponse.Fetches)

		// process each deferred response fetch tree
		for _, deferResp := range t.DeferResponses {
			p.processFetchTree.orderSequenceByDependencies.ProcessFetchTree(deferResp.Fetches)
			p.processFetchTree.createParallelNodes.ProcessFetchTree(deferResp.Fetches)
		}

	case *plan.SubscriptionResponsePlan:
		p.processResponseTree.mergeFields.Process(t.Response.Response.Data)
		p.createFetchTree(t.Response.Response)
		p.appendTriggerToFetchTree(t.Response)
		p.processFetchTree.dedupe.ProcessFetchTree(t.Response.Response.Fetches)
		// Appending fetchIDs makes query content unique, thus it should happen after "dedupe".
		p.processFetchTree.appendFetchID.ProcessFetchTree(t.Response.Response.Fetches)
		// resolve input templates for nested fetches
		p.processFetchTree.resolveInputTemplates.ProcessFetchTree(t.Response.Response.Fetches)
		// resolve input template for the root query in the subscription trigger
		p.processFetchTree.resolveInputTemplates.ProcessTrigger(&t.Response.Trigger)
		p.processFetchTree.addMissingNestedDependencies.ProcessFetchTree(t.Response.Response.Fetches)
		p.processFetchTree.createConcreteSingleFetchTypes.ProcessFetchTree(t.Response.Response.Fetches)
		p.processFetchTree.orderSequenceByDependencies.ProcessFetchTree(t.Response.Response.Fetches)
		p.processFetchTree.createParallelNodes.ProcessFetchTree(t.Response.Response.Fetches)
	}
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
