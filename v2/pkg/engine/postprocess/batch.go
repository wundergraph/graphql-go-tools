package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type ProcessDataSourceBatch struct{}

func (d *ProcessDataSourceBatch) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		d.traverseNode(t.Response.Data)
	case *plan.SubscriptionResponsePlan:
		d.traverseNode(t.Response.Response.Data)
	}
	return pre
}

func (d *ProcessDataSourceBatch) traverseNode(node resolve.Node) {
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

func (d *ProcessDataSourceBatch) traverseFetch(fetch resolve.Fetch) resolve.Fetch {
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

func (d *ProcessDataSourceBatch) traverseSingleFetch(fetch *resolve.SingleFetch) resolve.Fetch {
	if !fetch.RequiresBatchFetch {
		return fetch
	}

	representationsVariableIndex := -1
	for i, segment := range fetch.InputTemplate.Segments {
		if segment.SegmentType == resolve.VariableSegmentType &&
			segment.VariableKind == resolve.ResolvableObjectVariableKind {
			representationsVariableIndex = i
			break
		}
	}

	return &resolve.BatchFetch{
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
			SkipNullItems: true,
			SkipErrItems:  true,
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
