package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/reviews/graph/generated"
	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/reviews/graph/model"
)

// AddReview is the resolver for the addReview field.
func (r *mutationResolver) AddReview(ctx context.Context, authorID string, upc string, review string) (*model.Review, error) {
	record := &model.Review{
		Body:    review,
		Author:  &model.User{ID: authorID},
		Product: &model.Product{Upc: upc},
	}

	reviews = append(reviews, record)

	return record, nil
}

// Reviews is the resolver for the reviews field.
func (r *productResolver) Reviews(ctx context.Context, obj *model.Product) ([]*model.Review, error) {
	var res []*model.Review

	for _, review := range reviews {
		if review.Product.Upc == obj.Upc {
			res = append(res, review)
		}
	}

	return res, nil
}

// Username is the resolver for the username field.
func (r *userResolver) Username(ctx context.Context, obj *model.User) (string, error) {
	username := fmt.Sprintf("User %s", obj.ID)
	if obj.ID == "1234" {
		username = "Me"
	}
	return username, nil
}

// Reviews is the resolver for the reviews field.
func (r *userResolver) Reviews(ctx context.Context, obj *model.User) ([]*model.Review, error) {
	var res []*model.Review

	for _, review := range reviews {
		if review.Author.ID == obj.ID {
			res = append(res, review)
		}
	}

	return res, nil
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Product returns generated.ProductResolver implementation.
func (r *Resolver) Product() generated.ProductResolver { return &productResolver{r} }

// User returns generated.UserResolver implementation.
func (r *Resolver) User() generated.UserResolver { return &userResolver{r} }

type mutationResolver struct{ *Resolver }
type productResolver struct{ *Resolver }
type userResolver struct{ *Resolver }
