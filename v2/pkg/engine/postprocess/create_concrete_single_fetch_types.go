package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// CreateConcreteSingleFetchTypes is a postprocessor that transforms fetches into more concrete fetch types
type CreateConcreteSingleFetchTypes struct{}

func (d *CreateConcreteSingleFetchTypes) Process(node resolve.Node) {
	d.traverseNode(node)
}

func (d *CreateConcreteSingleFetchTypes) ProcessSubscription(node resolve.Node, trigger *resolve.GraphQLSubscriptionTrigger) {
	d.traverseNode(node)
}

func (d *CreateConcreteSingleFetchTypes) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		n.Fetch = d.traverseFetch(n.Fetch)
		for i := range n.Fields {
			d.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		d.traverseNode(n.Item)
	}
}

func (d *CreateConcreteSingleFetchTypes) traverseFetch(fetch resolve.Fetch) resolve.Fetch {
	if fetch == nil {
		return nil
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return d.traverseSingleFetch(f)
	case *resolve.ParallelFetch:
		fetches := make([]resolve.Fetch, 0, len(f.Fetches))
		for i := range f.Fetches {
			fetches = append(fetches, d.traverseFetch(f.Fetches[i]))
		}
		f.Fetches = fetches
	case *resolve.SerialFetch:
		fetches := make([]resolve.Fetch, 0, len(f.Fetches))
		for i := range f.Fetches {
			fetches = append(fetches, d.traverseFetch(f.Fetches[i]))
		}
		f.Fetches = fetches
	}

	return fetch
}

func (d *CreateConcreteSingleFetchTypes) traverseSingleFetch(fetch *resolve.SingleFetch) resolve.Fetch {
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

func (d *CreateConcreteSingleFetchTypes) createParallelListItemFetch(fetch *resolve.SingleFetch) resolve.Fetch {
	return &resolve.ParallelListItemFetch{
		Fetch: fetch,
	}
}

func (d *CreateConcreteSingleFetchTypes) createEntityBatchFetch(fetch *resolve.SingleFetch) resolve.Fetch {
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

func (d *CreateConcreteSingleFetchTypes) createEntityFetch(fetch *resolve.SingleFetch) resolve.Fetch {
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
