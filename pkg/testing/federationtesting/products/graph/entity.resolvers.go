package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/products/graph/generated"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/products/graph/model"
)

// FindProductByUpc is the resolver for the findProductByUpc field.
func (r *entityResolver) FindProductByUpc(ctx context.Context, upc string) (*model.Product, error) {
	for _, h := range hats {
		if h.Upc == upc {
			return h, nil
		}
	}
	return nil, nil
}

// Entity returns generated.EntityResolver implementation.
func (r *Resolver) Entity() generated.EntityResolver { return &entityResolver{r} }

type entityResolver struct{ *Resolver }
