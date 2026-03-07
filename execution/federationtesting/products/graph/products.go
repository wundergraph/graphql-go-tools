package graph

import (
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph/model"
)

func newProducts() []*model.Product {
	return []*model.Product{
		{
			Upc:     "top-1",
			Name:    "Trilby",
			Price:   11,
			InStock: 500,
		},
		{
			Upc:     "top-2",
			Name:    "Fedora",
			Price:   22,
			InStock: 1200,
		},
		{
			Upc:     "top-3",
			Name:    "Boater",
			Price:   33,
			InStock: 850,
		},
	}
}

// newExtraProducts returns products not listed in TopProducts but findable by UPC.
func newExtraProducts() []*model.Product {
	return []*model.Product{
		{
			Upc:     "top-4",
			Name:    "Bowler",
			Price:   64,
			InStock: 12,
		},
	}
}

func newDigitalProducts() []*model.DigitalProduct {
	return []*model.DigitalProduct{
		{
			Upc:         "digital-1",
			Name:        "eBook: GraphQL in Action",
			Price:       29,
			DownloadURL: "https://example.com/downloads/graphql-in-action",
		},
	}
}
