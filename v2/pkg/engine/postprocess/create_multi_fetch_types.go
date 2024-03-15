package postprocess

import (
	"slices"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
)

// CreateMultiFetchTypes is a postprocessor that transforms multi fetches into more concrete fetch types
type CreateMultiFetchTypes struct{}

func (d *CreateMultiFetchTypes) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		d.traverseNode(t.Response.Data)
	case *plan.SubscriptionResponsePlan:
		d.traverseNode(t.Response.Response.Data)
	}
	return pre
}

func (d *CreateMultiFetchTypes) traverseNode(node resolve.Node) {
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

func (d *CreateMultiFetchTypes) traverseFetch(fetch resolve.Fetch) resolve.Fetch {
	if fetch == nil {
		return nil
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return f
	case *resolve.MultiFetch:
		return d.processMultiFetch(f)
	}

	return fetch
}

func (d *CreateMultiFetchTypes) processMultiFetch(fetch *resolve.MultiFetch) resolve.Fetch {
	currentFetches := fetch.Fetches
	dependsOn := make([]int, 0, len(fetch.Fetches))

	for _, f := range fetch.Fetches {
		dependsOn = append(dependsOn, f.DependsOnFetchIDs...)
	}

	// at the beginning we collect all dependencies of the current fetches, which not in the list of current fetches
	// that will be parent fetches from lower depth
	seenParentFetches := make(map[int]struct{}, len(fetch.Fetches))
	for _, parentID := range dependsOn {
		if slices.ContainsFunc(currentFetches, func(f *resolve.SingleFetch) bool {
			return parentID == f.FetchID
		}) {
			continue
		}

		seenParentFetches[parentID] = struct{}{}
	}

	layers := make([][]resolve.Fetch, 0, len(fetch.Fetches))

	// Here we create execution layers, layers will be fetched serially,
	// but each layer items could be fetched in parallel.
	// We iterate over items and look for fetches which depends on already seen fetches,
	// such fetches added to the current layer, and removed from current fetches,
	// we also mark them as seen after current layer is created

	for len(currentFetches) > 0 {
		currentLayer := make([]resolve.Fetch, 0, 2)
		// as currentLayer is a slice of interfaces resolve.Fetch, we store fetch ids separately
		currentLayerFetchIds := make([]int, 0, 2)

		for _, fetch := range currentFetches {
			shouldAdd := true
			for _, parentID := range fetch.DependsOnFetchIDs {
				if _, ok := seenParentFetches[parentID]; !ok {
					shouldAdd = false
					break
				}
			}

			if shouldAdd {
				currentLayerFetchIds = append(currentLayerFetchIds, fetch.FetchID)
				currentLayer = append(currentLayer, fetch)
			}
		}

		layers = append(layers, currentLayer)

		for _, fetchID := range currentLayerFetchIds {
			seenParentFetches[fetchID] = struct{}{}
		}

		if len(currentLayer) == 0 {
			panic("not able to setup fetch execution order - wrong execution plan")
		}

		currentFetches = slices.DeleteFunc(currentFetches, func(f *resolve.SingleFetch) bool {
			return slices.Contains(currentLayerFetchIds, f.FetchID)
		})
	}

	if len(layers) == 1 {
		return &resolve.ParallelFetch{
			Fetches: layers[0],
		}
	}

	fetches := make([]resolve.Fetch, 0, len(layers))
	for _, layer := range layers {
		if len(layer) == 1 {
			fetches = append(fetches, layer[0])
			continue
		}

		fetches = append(fetches, &resolve.ParallelFetch{
			Fetches: layer,
		})
	}

	return &resolve.SerialFetch{
		Fetches: fetches,
	}
}
