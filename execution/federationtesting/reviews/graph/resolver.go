// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.
package graph

import "github.com/wundergraph/graphql-go-tools/execution/federationtesting/reviews/graph/model"

type Resolver struct {
	reviews     []*model.Review
	attachments []model.Attachment
}
