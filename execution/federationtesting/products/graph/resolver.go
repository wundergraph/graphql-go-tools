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
	randomnessEnabled bool
	minPrice          int
	maxPrice          int
	currentPrice      int
	updateInterval    time.Duration
	priceMu           sync.Mutex
}
