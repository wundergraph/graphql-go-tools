package graph

import "github.com/wundergraph/graphql-go-tools/v2/examples/federation/products/graph/model"

var hats = []*model.Product{
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
