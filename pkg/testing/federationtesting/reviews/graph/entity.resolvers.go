package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/reviews/graph/generated"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/reviews/graph/model"
)

// FindProductByUpc is the resolver for the findProductByUpc field.
func (r *entityResolver) FindProductByUpc(ctx context.Context, upc string) (*model.Product, error) {
	return &model.Product{
		Upc: upc,
	}, nil
}

// FindUserByID is the resolver for the findUserByID field.
func (r *entityResolver) FindUserByID(ctx context.Context, id string) (*model.User, error) {
	return &model.User{
		ID: id,
	}, nil
}

// Entity returns generated.EntityResolver implementation.
func (r *Resolver) Entity() generated.EntityResolver { return &entityResolver{r} }

type entityResolver struct{ *Resolver }
