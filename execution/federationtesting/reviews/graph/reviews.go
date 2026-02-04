package graph

import (
	"github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph/model"
)

var reviews = []*model.Review{
	{
		Body:    "A highly effective form of birth control.",
		Product: &model.Product{Upc: "top-1"},
		Author:  &model.User{ID: "1234", Username: "Me"},
	},
	{
		Body:    "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.",
		Product: &model.Product{Upc: "top-2"},
		Author:  &model.User{ID: "1234", Username: "Me"},
	},
	{
		Body:    "This is the last straw. Hat you will wear. 11/10",
		Product: &model.Product{Upc: "top-3"},
		Author:  &model.User{ID: "7777", Username: "User 7777"},
	},
}

// errorReview is a separate review used for cache error testing.
// It has an author ID "error-user" which triggers an error in the accounts subgraph.
// This is accessed via the reviewWithError query, not through normal reviews.
var errorReview = &model.Review{
	Body:    "This review triggers an error when resolving the author",
	Product: &model.Product{Upc: "error-product"},
	Author:  &model.User{ID: "error-user", Username: ""},
}
