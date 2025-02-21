package postprocess

import (
	"encoding/json"
	"fmt"
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

type Processor struct {
	disableExtractFetches bool
	collectDataSourceInfo bool
	resolveInputTemplates *resolveInputTemplates
	dedupe                *deduplicateSingleFetches
	processResponseTree   []ResponseTreeProcessor
	processFetchTree      []FetchTreeProcessor
	extractDeferredFields *extractDeferredFields
}

type processorOptions struct {
	disableDeduplicateSingleFetches       bool
	disableCreateConcreteSingleFetchTypes bool
	disableOrderSequenceByDependencies    bool
	disableMergeFields                    bool
	disableResolveInputTemplates          bool
	disableExtractFetches                 bool
	disableCreateParallelNodes            bool
	disableAddMissingNestedDependencies   bool
	collectDataSourceInfo                 bool
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

func DisableAddMissingNestedDependencies() ProcessorOption {
	return func(o *processorOptions) {
		o.disableAddMissingNestedDependencies = true
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
		resolveInputTemplates: &resolveInputTemplates{
			disable: opts.disableResolveInputTemplates,
		},
		dedupe: &deduplicateSingleFetches{
			disable: opts.disableDeduplicateSingleFetches,
		},
		processFetchTree: []FetchTreeProcessor{
			// this must go first, as we need to deduplicate fetches so that subsequent processors can work correctly
			&addMissingNestedDependencies{
				disable: opts.disableAddMissingNestedDependencies,
			},
			// this must go after deduplication because it relies on the existence of a "sequence" fetch node in the root
			&createConcreteSingleFetchTypes{
				disable: opts.disableCreateConcreteSingleFetchTypes,
			},
			&orderSequenceByDependencies{
				disable: opts.disableOrderSequenceByDependencies,
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
		extractDeferredFields: &extractDeferredFields{},
	}
}

func (p *Processor) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		for i := range p.processResponseTree {
			p.processResponseTree[i].Process(t.Response.Data)
		}
		p.extractDeferredFields.Process(t.Response) // TODO: should this be before or after the fetch tree processors?
		p.createFetchTree(t.Response)
		p.dedupe.ProcessFetchTree(t.Response.Fetches)
		p.resolveInputTemplates.ProcessFetchTree(t.Response.Fetches)
		for i := range p.processFetchTree {
			p.processFetchTree[i].ProcessFetchTree(t.Response.Fetches)
		}
	case *plan.SubscriptionResponsePlan:
		for i := range p.processResponseTree {
			p.processResponseTree[i].ProcessSubscription(t.Response.Response.Data)
		}
		p.createFetchTree(t.Response.Response)
		p.appendTriggerToFetchTree(t.Response)
		p.dedupe.ProcessFetchTree(t.Response.Response.Fetches)
		p.resolveInputTemplates.ProcessFetchTree(t.Response.Response.Fetches)
		p.resolveInputTemplates.ProcessTrigger(&t.Response.Trigger)
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

	if p.collectDataSourceInfo {
		var list = make([]resolve.DataSourceInfo, 0, len(fetches))
		for _, fetch := range fetches {
			dsInfo := fetch.Fetch.DataSourceInfo()
			if !slices.Contains(list, dsInfo) {
				list = append(list, dsInfo)
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

func (p *Processor) appendTriggerToFetchTree(res *resolve.GraphQLSubscription) {
	var input struct {
		Body struct {
			Query string `json:"query"`
		} `json:"body"`
	}

	err := json.Unmarshal(res.Trigger.Input, &input)
	if err != nil {
		fmt.Println("error decoding subscription input", err)
		return
	}

	rootData := res.Response.Data
	if rootData == nil || len(rootData.Fields) == 0 {
		return
	}

	info := rootData.Fields[0].Info
	if info == nil {
		return
	}

	res.Response.Fetches.Trigger = &resolve.FetchTreeNode{
		Kind: resolve.FetchTreeNodeKindTrigger,
		Item: &resolve.FetchItem{
			Fetch: &resolve.SingleFetch{
				FetchDependencies: resolve.FetchDependencies{
					FetchID: info.FetchID,
				},
				Info: &resolve.FetchInfo{
					DataSourceID:   info.Source.IDs[0],
					DataSourceName: info.Source.Names[0],
					QueryPlan: &resolve.QueryPlan{
						Query: input.Body.Query,
					},
				},
			},
			ResponsePath: info.Name,
		},
	}
}
