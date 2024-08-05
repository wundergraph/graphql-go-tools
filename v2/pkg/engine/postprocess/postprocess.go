package postprocess

import (
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

type Processor struct {
	disableExtractFetches        bool
	disableResolveInputTemplates bool
	processResponseTree          []ResponseTreeProcessor
	processFetchTree             []FetchTreeProcessor
}

type processorOptions struct {
	disableDeduplicateSingleFetches       bool
	disableCreateConcreteSingleFetchTypes bool
	disableMergeFields                    bool
	disableResolveInputTemplates          bool
	disableExtractFetches                 bool
	disableCreateParallelNodes            bool
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

func DisableMergeFields() ProcessorOption {
	return func(o *processorOptions) {
		o.disableMergeFields = true
	}
}

func DisableResolveInputTemplates() ProcessorOption {
	return func(o *processorOptions) {
		o.disableResolveInputTemplates = true
	}
}

func DisableExtractFetches() ProcessorOption {
	return func(o *processorOptions) {
		o.disableExtractFetches = true
	}
}

func DisableCreateParallelNodes() ProcessorOption {
	return func(o *processorOptions) {
		o.disableCreateParallelNodes = true
	}
}

func NewProcessor(options ...ProcessorOption) *Processor {
	opts := &processorOptions{}
	for _, o := range options {
		o(opts)
	}
	return &Processor{
		disableExtractFetches:        opts.disableExtractFetches,
		disableResolveInputTemplates: opts.disableResolveInputTemplates,
		processFetchTree: []FetchTreeProcessor{
			// this must go first, as we need to deduplicate fetches so that subsequent processors can work correctly
			&deduplicateSingleFetches{
				disable: opts.disableDeduplicateSingleFetches,
			},
			// this must go after deduplication because it relies on the existence of a "sequence" fetch node in the root
			&createConcreteSingleFetchTypes{
				disable: opts.disableCreateConcreteSingleFetchTypes,
			},
			&createParallelNodes{
				disable: opts.disableCreateParallelNodes,
			},
		},
		processResponseTree: []ResponseTreeProcessor{
			&mergeFields{
				disable: opts.disableMergeFields,
			},
		},
	}
}

func (p *Processor) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		for i := range p.processResponseTree {
			p.processResponseTree[i].Process(t.Response.Data)
		}
		p.createFetchTree(t.Response)
		if !p.disableResolveInputTemplates {
			resolver := &resolveInputTemplates{}
			resolver.ProcessFetchTree(t.Response.Fetches)
		}
		for i := range p.processFetchTree {
			p.processFetchTree[i].ProcessFetchTree(t.Response.Fetches)
		}
	case *plan.SubscriptionResponsePlan:
		for i := range p.processResponseTree {
			p.processResponseTree[i].ProcessSubscription(t.Response.Response.Data)
		}
		p.createFetchTree(t.Response.Response)
		if !p.disableResolveInputTemplates {
			resolver := &resolveInputTemplates{}
			resolver.ProcessFetchTree(t.Response.Response.Fetches)
			resolver.ProcessTrigger(&t.Response.Trigger)
		}
		for i := range p.processFetchTree {
			p.processFetchTree[i].ProcessFetchTree(t.Response.Response.Fetches)
		}
	}
	return pre
}

func (p *Processor) createFetchTree(res *resolve.GraphQLResponse) {
	if p.disableExtractFetches {
		return
	}
	ex := &extractor{
		info: res.Info,
	}
	fetches := ex.extractFetches(res)
	children := make([]*resolve.FetchTreeNode, len(fetches))
	for i := range fetches {
		children[i] = &resolve.FetchTreeNode{
			Kind: resolve.FetchTreeNodeKindSingle,
			Item: fetches[i],
		}
	}
	res.Fetches = &resolve.FetchTreeNode{
		Kind:        resolve.FetchTreeNodeKindSequence,
		SerialNodes: children,
	}
}
