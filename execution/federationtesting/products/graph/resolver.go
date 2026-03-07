// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.
package graph

import (
	"sync"
	"time"

	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/products/graph/model"
)

type Resolver struct {
	products          []*model.Product
	extraProducts     []*model.Product
	digitalProducts   []*model.DigitalProduct
	randomnessEnabled bool
	minPrice          int
	maxPrice          int
	currentPrice      int
	updateInterval    time.Duration
	priceMu           sync.Mutex
}

// findProduct searches both products and extraProducts by UPC.
func (r *Resolver) findProduct(upc string) *model.Product {
	for _, p := range r.products {
		if p.Upc == upc {
			return p
		}
	}
	for _, p := range r.extraProducts {
		if p.Upc == upc {
			return p
		}
	}
	return nil
}

// findDigitalProduct searches digitalProducts by UPC.
func (r *Resolver) findDigitalProduct(upc string) *model.DigitalProduct {
	for _, d := range r.digitalProducts {
		if d.Upc == upc {
			return d
		}
	}
	return nil
}
