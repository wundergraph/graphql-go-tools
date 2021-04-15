package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/99designs/gqlgen/example/federation/accounts/graph/model"
	generated1 "github.com/jensneuse/federation-example/accounts/graph/generated"
)

func (r *queryResolver) Me(ctx context.Context) (*model.User, error) {
	return &model.User{
		ID:       "1234",
		Username: "Me",
	}, nil
}

// Query returns generated1.QueryResolver implementation.
func (r *Resolver) Query() generated1.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
