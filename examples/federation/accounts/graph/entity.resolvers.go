package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/99designs/gqlgen/example/federation/accounts/graph/model"
	generated1 "github.com/jensneuse/federation-example/accounts/graph/generated"
)

func (r *entityResolver) FindUserByID(ctx context.Context, id string) (*model.User, error) {
	name := "User " + id
	if id == "1234" {
		name = "Me"
	}

	return &model.User{
		ID:       id,
		Username: name,
	}, nil
}

// Entity returns generated1.EntityResolver implementation.
func (r *Resolver) Entity() generated1.EntityResolver { return &entityResolver{r} }

type entityResolver struct{ *Resolver }
