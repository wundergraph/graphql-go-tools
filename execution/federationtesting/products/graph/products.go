package graph

import (
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph/model"
)

var hats []*model.Product

// extraProducts holds products not returned by TopProducts but accessible by UPC
// (e.g. subscription-specific test products).
var extraProducts []*model.Product

var digitalProducts []*model.DigitalProduct

func Reset() {
	hats = []*model.Product{
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
	extraProducts = []*model.Product{
		{
			Upc:     "top-4",
			Name:    "Bowler",
			Price:   64,
			InStock: 12,
		},
	}
	digitalProducts = []*model.DigitalProduct{
		{
			Upc:         "digital-1",
			Name:        "eBook: GraphQL in Action",
			Price:       29,
			DownloadURL: "https://example.com/downloads/graphql-in-action",
		},
	}
}

// findProduct looks up a product by UPC from both hats and extraProducts.
func findProduct(upc string) *model.Product {
	for _, h := range hats {
		if h.Upc == upc {
			return h
		}
	}
	for _, p := range extraProducts {
		if p.Upc == upc {
			return p
		}
	}
	return nil
}

func findDigitalProduct(upc string) *model.DigitalProduct {
	for _, d := range digitalProducts {
		if d.Upc == upc {
			return d
		}
	}
	return nil
}

func init() {
	Reset()
}
