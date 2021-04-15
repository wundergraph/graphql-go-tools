package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"math/rand"
	"time"

	"github.com/99designs/gqlgen/example/federation/products/graph/model"

	"github.com/jensneuse/federation-example/products/graph/generated"
)

func (r *queryResolver) TopProducts(ctx context.Context, first *int) ([]*model.Product, error) {
	return hats, nil
}

func (r *subscriptionResolver) UpdatedPrice(ctx context.Context) (<-chan *model.Product, error) {
	updatedPrice := make(chan *model.Product)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				rand.Seed(time.Now().UnixNano())
				product := hats[rand.Intn(len(hats)-1)]
				min := 10
				max := 1499
				product.Price = rand.Intn(max-min+1) + min
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
