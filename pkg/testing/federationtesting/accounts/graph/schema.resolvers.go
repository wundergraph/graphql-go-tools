package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.36

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph/generated"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph/model"
)

// Me is the resolver for the me field.
func (r *queryResolver) Me(ctx context.Context) (*model.User, error) {
	return &model.User{
		ID:       "1234",
		Username: "Me",
		History:  histories,
		RealName: "User Usington",
	}, nil
}

// Identifiable is the resolver for the identifiable field.
func (r *queryResolver) Identifiable(ctx context.Context) (model.Identifiable, error) {
	return &model.User{
		ID:       "1234",
		Username: "Me",
		History:  histories,
		RealName: "User Usington",
	}, nil
}

// Histories is the resolver for the histories field.
func (r *queryResolver) Histories(ctx context.Context) ([]model.History, error) {
	return allHistories, nil
}

// Cat is the resolver for the cat field.
func (r *queryResolver) Cat(ctx context.Context) (*model.Cat, error) {
	return &model.Cat{
		Name: "Pepper",
	}, nil
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
