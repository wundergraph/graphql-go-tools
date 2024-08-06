package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// createConcreteSingleFetchTypes is a postprocessor that transforms fetches into more concrete fetch types
type createConcreteSingleFetchTypes struct {
	disable bool
}

func (d *createConcreteSingleFetchTypes) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if d.disable {
		return
	}
	d.traverseNode(root)
}

func (d *createConcreteSingleFetchTypes) traverseNode(node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		node.Item.Fetch = d.traverseFetch(node.Item.Fetch)
	case resolve.FetchTreeNodeKindParallel:
		for i := range node.ChildNodes {
			d.traverseNode(node.ChildNodes[i])
		}
	case resolve.FetchTreeNodeKindSequence:
		for i := range node.ChildNodes {
			d.traverseNode(node.ChildNodes[i])
		}
	}
}

func (d *createConcreteSingleFetchTypes) traverseFetch(fetch resolve.Fetch) resolve.Fetch {
	if fetch == nil {
		return nil
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return d.traverseSingleFetch(f)
	}
	return fetch
}

func (d *createConcreteSingleFetchTypes) traverseSingleFetch(fetch *resolve.SingleFetch) resolve.Fetch {
	switch {
	case fetch.RequiresEntityBatchFetch:
		return d.createEntityBatchFetch(fetch)
	case fetch.RequiresEntityFetch:
		return d.createEntityFetch(fetch)
	case fetch.RequiresParallelListItemFetch:
		return d.createParallelListItemFetch(fetch)
	default:
		return fetch
	}
}

func (d *createConcreteSingleFetchTypes) createParallelListItemFetch(fetch *resolve.SingleFetch) resolve.Fetch {
	return &resolve.ParallelListItemFetch{
		Fetch: fetch,
	}
}

func (d *createConcreteSingleFetchTypes) createEntityBatchFetch(fetch *resolve.SingleFetch) resolve.Fetch {
	representationsVariableIndex := -1
	for i, segment := range fetch.InputTemplate.Segments {
		if segment.SegmentType == resolve.VariableSegmentType &&
			segment.VariableKind == resolve.ResolvableObjectVariableKind {
			representationsVariableIndex = i
			break
		}
	}

	return &resolve.BatchEntityFetch{
		FetchDependencies: fetch.FetchDependencies,
		Info:              fetch.Info,
		Input: resolve.BatchInput{
			Header: resolve.InputTemplate{
				Segments:                              fetch.InputTemplate.Segments[:representationsVariableIndex],
				SetTemplateOutputToNullOnVariableNull: fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull,
			},
			Items: []resolve.InputTemplate{
				{
					Segments:                              []resolve.TemplateSegment{fetch.InputTemplate.Segments[representationsVariableIndex]},
					SetTemplateOutputToNullOnVariableNull: fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull,
				},
			},
			SkipNullItems:        true,
			SkipEmptyObjectItems: true,
			SkipErrItems:         true,
			Separator: resolve.InputTemplate{
				Segments: []resolve.TemplateSegment{
					{
						Data:        []byte(`,`),
						SegmentType: resolve.StaticSegmentType,
					},
				},
			},
			Footer: resolve.InputTemplate{
				Segments:                              fetch.InputTemplate.Segments[representationsVariableIndex+1:],
				SetTemplateOutputToNullOnVariableNull: fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull,
			},
		},
		DataSource:     fetch.DataSource,
		PostProcessing: fetch.PostProcessing,
	}
}

func (d *createConcreteSingleFetchTypes) createEntityFetch(fetch *resolve.SingleFetch) resolve.Fetch {
	representationsVariableIndex := -1
	for i, segment := range fetch.InputTemplate.Segments {
		if segment.SegmentType == resolve.VariableSegmentType &&
			segment.VariableKind == resolve.ResolvableObjectVariableKind {
			representationsVariableIndex = i
			break
		}
	}

	return &resolve.EntityFetch{
		FetchDependencies: fetch.FetchDependencies,
		Info:              fetch.Info,
		Input: resolve.EntityInput{
			Header: resolve.InputTemplate{
				Segments:                              fetch.InputTemplate.Segments[:representationsVariableIndex],
				SetTemplateOutputToNullOnVariableNull: fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull,
			},
			Item: resolve.InputTemplate{
				Segments:                              []resolve.TemplateSegment{fetch.InputTemplate.Segments[representationsVariableIndex]},
				SetTemplateOutputToNullOnVariableNull: fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull,
			},
			SkipErrItem: true,
			Footer: resolve.InputTemplate{
				Segments:                              fetch.InputTemplate.Segments[representationsVariableIndex+1:],
				SetTemplateOutputToNullOnVariableNull: fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull,
			},
		},
		DataSource:     fetch.DataSource,
		PostProcessing: fetch.PostProcessing,
	}
}
