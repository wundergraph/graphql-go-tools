package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/pkg/graphql/federationtesting/accounts/graph/generated"
	"github.com/wundergraph/graphql-go-tools/pkg/graphql/federationtesting/accounts/graph/model"
)

func (r *queryResolver) Me(ctx context.Context) (*model.User, error) {
	return &model.User{
		ID:       "1234",
		Username: "Me",
		History:  histories,
	}, nil
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
