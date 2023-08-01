package graph

import "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/reviews/graph/model"

var attachments = []model.Attachment{
	model.Question{
		Upc:  "top-1",
		Body: "How do I turn it on?",
	},
	model.Question{
		Upc:  "top-3",
		Body: "Any recommendations for other teacosies?",
	},
	model.Rating{
		Upc:   "top-2",
		Body:  "The best hat I have ever bought in my life.",
		Score: 5,
	},
	model.Rating{
		Upc:   "top-3",
		Body:  "Terrible teacosy!!!",
		Score: 0,
	},
	model.Video{
		Upc:  "top-2",
		Size: 13.37,
	},
	model.Video{
		Upc:  "top-3",
		Size: 4.20,
	},
}
