package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"time"

	"github.com/wundergraph/graphql-go-tools/pkg/graphql/federationtesting/products/graph/generated"
	"github.com/wundergraph/graphql-go-tools/pkg/graphql/federationtesting/products/graph/model"
)

func (r *queryResolver) TopProducts(ctx context.Context, first *int) ([]*model.Product, error) {
	return hats, nil
}

func (r *subscriptionResolver) UpdateProductPrice(ctx context.Context, upc string) (<-chan *model.Product, error) {
	updatedPrice := make(chan *model.Product)
	var product *model.Product

	for _, hat := range hats {
		if hat.Upc == upc {
			product = hat
			break
		}
	}

	if product == nil {
		return nil, fmt.Errorf("unknown product upc: %s", upc)
	}

	go func() {
		var num int

		for {
			num++

			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				product.Price = num
				updatedPrice <- product
			}
		}
	}()

	return updatedPrice, nil
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Subscription returns generated.SubscriptionResolver implementation.
func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }

type queryResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }
