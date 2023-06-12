package graph

import "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/reviews/graph/model"

var questions = []model.Question{
	{
		Upc:  "top-1",
		Body: "How do I turn it on?",
	},
	{
		Upc:  "top-3",
		Body: "Any recommendations for other teacosies?",
	},
}

var ratings = []model.Rating{{
	Upc:   "top-2",
	Body:  "The best hat I have ever bought in my life.",
	Score: 5,
},
	{
		Upc:   "top-3",
		Body:  "Terrible teacosy!!!",
		Score: 0,
	},
}

var videos = []model.Video{
	{
		Upc:  "top-2",
		Size: 13.37,
	},
	{
		Upc:  "top-3",
		Size: 4.20,
	},
}
