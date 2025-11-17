package grpctest

import (
	context "context"
	"fmt"
	"math/rand"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
)

// BlogPost query implementations
func (s *MockService) QueryBlogPost(ctx context.Context, in *productv1.QueryBlogPostRequest) (*productv1.QueryBlogPostResponse, error) {
	// Return a default blog post with comprehensive list examples
	result := &productv1.BlogPost{
		Id:      "blog-default",
		Title:   "Default Blog Post",
		Content: "This is a sample blog post content for testing nested lists.",
		Tags:    []string{"tech", "programming", "go"},
		OptionalTags: &productv1.ListOfString{
			List: &productv1.ListOfString_List{
				Items: []string{"optional1", "optional2"},
			},
		},
		Categories: []string{"Technology", "", "Programming"}, // includes null/empty
		Keywords: &productv1.ListOfString{
			List: &productv1.ListOfString_List{
				Items: []string{"keyword1", "keyword2"},
			},
		},
		ViewCounts: []int32{100, 150, 200, 250},
		Ratings: &productv1.ListOfFloat{
			List: &productv1.ListOfFloat_List{
				Items: []float64{4.5, 3.8, 5.0},
			},
		},
		IsPublished: &productv1.ListOfBoolean{
			List: &productv1.ListOfBoolean_List{
				Items: []bool{false, true, true},
			},
		},
		TagGroups: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{
						Items: []string{"tech", "programming"},
					}},
					{List: &productv1.ListOfString_List{
						Items: []string{"golang", "backend"},
					}},
				},
			},
		},
		RelatedTopics: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{Items: []string{"microservices", "api"}}},
					{List: &productv1.ListOfString_List{Items: []string{"databases", "performance"}}},
				},
			},
		},
		CommentThreads: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{Items: []string{"Great post!", "Very helpful"}}},
					{List: &productv1.ListOfString_List{Items: []string{"Could use more examples", "Thanks for sharing"}}},
				},
			},
		},
		Suggestions: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{Items: []string{"Add code examples", "Include diagrams"}}},
				},
			},
		},
		RelatedCategories: []*productv1.Category{
			{Id: "cat-1", Name: "Technology", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
			{Id: "cat-2", Name: "Programming", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
		},
		Contributors: []*productv1.User{
			{Id: "user-1", Name: "John Doe"},
			{Id: "user-2", Name: "Jane Smith"},
		},
		MentionedProducts: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "prod-1", Name: "Sample Product", Price: 99.99},
				},
			},
		},
		MentionedUsers: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "user-3", Name: "Bob Johnson"},
				},
			},
		},
		CategoryGroups: &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{
					{List: &productv1.ListOfCategory_List{
						Items: []*productv1.Category{
							{Id: "cat-3", Name: "Web Development", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
							{Id: "cat-4", Name: "Backend", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
						},
					}},
				},
			},
		},
		ContributorTeams: &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "user-4", Name: "Alice Brown"},
							{Id: "user-5", Name: "Charlie Wilson"},
						},
					}},
				},
			},
		},
	}

	return &productv1.QueryBlogPostResponse{
		BlogPost: result,
	}, nil
}

func (s *MockService) QueryBlogPostById(ctx context.Context, in *productv1.QueryBlogPostByIdRequest) (*productv1.QueryBlogPostByIdResponse, error) {
	id := in.GetId()

	// Return null for specific test IDs
	if id == "not-found" {
		return &productv1.QueryBlogPostByIdResponse{
			BlogPostById: nil,
		}, nil
	}

	// Create different test data based on ID
	var result *productv1.BlogPost

	switch id {
	case "simple":
		result = &productv1.BlogPost{
			Id:         id,
			Title:      "Simple Post",
			Content:    "Simple content",
			Tags:       []string{"simple"},
			Categories: []string{"Basic"},
			ViewCounts: []int32{10},
			// Required nested lists must have data
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"simple"}}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"basic"}}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"Nice post"}}},
					},
				},
			},
			// Required complex lists must have data
			RelatedCategories: []*productv1.Category{
				{Id: "cat-simple", Name: "Basic", Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			Contributors: []*productv1.User{
				{Id: "user-simple", Name: "Simple Author"},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-group-simple", Name: "Simple Category", Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
		}
	case "complex":
		result = &productv1.BlogPost{
			Id:      id,
			Title:   "Complex Blog Post",
			Content: "Complex content with comprehensive lists",
			Tags:    []string{"complex", "advanced", "detailed"},
			OptionalTags: &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{"deep-dive", "tutorial"},
				},
			},
			Categories: []string{"Advanced", "Tutorial", "Guide"},
			Keywords: &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{"advanced", "complex", "comprehensive"},
				},
			},
			ViewCounts: []int32{500, 600, 750, 800, 950},
			Ratings: &productv1.ListOfFloat{
				List: &productv1.ListOfFloat_List{
					Items: []float64{4.8, 4.9, 4.7, 5.0},
				},
			},
			IsPublished: &productv1.ListOfBoolean{
				List: &productv1.ListOfBoolean_List{
					Items: []bool{false, false, true, true},
				},
			},
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"advanced", "expert"}}},
						{List: &productv1.ListOfString_List{Items: []string{"tutorial", "guide", "comprehensive"}}},
						{List: &productv1.ListOfString_List{Items: []string{"deep-dive", "detailed"}}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"architecture", "patterns", "design"}}},
						{List: &productv1.ListOfString_List{Items: []string{"optimization", "performance", "scaling"}}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"Excellent deep dive!", "Very thorough"}}},
						{List: &productv1.ListOfString_List{Items: []string{"Could be longer", "More examples please"}}},
						{List: &productv1.ListOfString_List{Items: []string{"Best tutorial I've read", "Thank you!"}}},
					},
				},
			},
			Suggestions: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{Items: []string{"Add video content", "Include interactive examples"}}},
						{List: &productv1.ListOfString_List{Items: []string{"Create follow-up posts", "Add Q&A section"}}},
					},
				},
			},
			// Complex example includes all new complex list fields
			RelatedCategories: []*productv1.Category{
				{Id: "cat-complex-1", Name: "Advanced Programming", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
				{Id: "cat-complex-2", Name: "Software Architecture", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
			},
			Contributors: []*productv1.User{
				{Id: "user-complex-1", Name: "Expert Author"},
				{Id: "user-complex-2", Name: "Technical Reviewer"},
			},
			MentionedProducts: &productv1.ListOfProduct{
				List: &productv1.ListOfProduct_List{
					Items: []*productv1.Product{
						{Id: "prod-complex-1", Name: "Advanced IDE", Price: 299.99},
						{Id: "prod-complex-2", Name: "Profiling Tool", Price: 149.99},
					},
				},
			},
			MentionedUsers: &productv1.ListOfUser{
				List: &productv1.ListOfUser_List{
					Items: []*productv1.User{
						{Id: "user-complex-3", Name: "Referenced Expert"},
					},
				},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-group-1", Name: "System Design", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
								{Id: "cat-group-2", Name: "Architecture Patterns", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
							},
						}},
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-group-3", Name: "Performance", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
							},
						}},
					},
				},
			},
			ContributorTeams: &productv1.ListOfListOfUser{
				List: &productv1.ListOfListOfUser_List{
					Items: []*productv1.ListOfUser{
						{List: &productv1.ListOfUser_List{
							Items: []*productv1.User{
								{Id: "team-complex-1", Name: "Senior Engineer A"},
								{Id: "team-complex-2", Name: "Senior Engineer B"},
							},
						}},
						{List: &productv1.ListOfUser_List{
							Items: []*productv1.User{
								{Id: "team-complex-3", Name: "QA Lead"},
							},
						}},
					},
				},
			},
		}
	default:
		// Generic response for any other ID
		result = &productv1.BlogPost{
			Id:         id,
			Title:      fmt.Sprintf("Blog Post %s", id),
			Content:    fmt.Sprintf("Content for blog post %s", id),
			Tags:       []string{fmt.Sprintf("tag-%s", id), "general"},
			Categories: []string{"General", fmt.Sprintf("Category-%s", id)},
			ViewCounts: []int32{int32(len(id) * 10), int32(len(id) * 20)},
			// Required nested lists must have data
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("tag-%s", id), "group"},
						}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("topic-%s", id)},
						}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Comment on %s", id)},
						}},
					},
				},
			},
			// Required complex lists must have data
			RelatedCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-%s", id), Name: fmt.Sprintf("Category %s", id), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			Contributors: []*productv1.User{
				{Id: fmt.Sprintf("user-%s", id), Name: fmt.Sprintf("Author %s", id)},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-group-%s", id), Name: fmt.Sprintf("Group Category %s", id), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
		}
	}

	return &productv1.QueryBlogPostByIdResponse{
		BlogPostById: result,
	}, nil
}

func (s *MockService) QueryBlogPostsWithFilter(ctx context.Context, in *productv1.QueryBlogPostsWithFilterRequest) (*productv1.QueryBlogPostsWithFilterResponse, error) {
	filter := in.GetFilter()
	var results []*productv1.BlogPost

	// If no filter provided, return empty results
	if filter == nil {
		return &productv1.QueryBlogPostsWithFilterResponse{
			BlogPostsWithFilter: results,
		}, nil
	}

	titleFilter := ""
	if filter.Title != nil {
		titleFilter = filter.Title.GetValue()
	}

	hasCategories := false
	if filter.HasCategories != nil {
		hasCategories = filter.HasCategories.GetValue()
	}

	minTags := int32(0)
	if filter.MinTags != nil {
		minTags = filter.MinTags.GetValue()
	}

	// Generate filtered results
	for i := 1; i <= 3; i++ {
		title := fmt.Sprintf("Filtered Post %d", i)
		if titleFilter != "" {
			title = fmt.Sprintf("%s - Post %d", titleFilter, i)
		}

		var tags []string
		tagsCount := minTags + int32(i)
		for j := int32(0); j < tagsCount; j++ {
			tags = append(tags, fmt.Sprintf("tag%d", j+1))
		}

		var categories []string
		if hasCategories {
			categories = []string{fmt.Sprintf("Category%d", i), "Filtered"}
		}

		results = append(results, &productv1.BlogPost{
			Id:         fmt.Sprintf("filtered-blog-%d", i),
			Title:      title,
			Content:    fmt.Sprintf("Filtered content %d", i),
			Tags:       tags,
			Categories: categories,
			ViewCounts: []int32{int32(i * 100)},
			// Required nested lists must have data
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("filtered-tag-%d", i)},
						}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("filtered-topic-%d", i)},
						}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Filtered comment %d", i)},
						}},
					},
				},
			},
			// Required complex lists must have data
			RelatedCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-filtered-%d", i), Name: fmt.Sprintf("Filtered Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			Contributors: []*productv1.User{
				{Id: fmt.Sprintf("user-filtered-%d", i), Name: fmt.Sprintf("Filtered Author %d", i)},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-group-filtered-%d", i), Name: fmt.Sprintf("Filtered Group %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
		})
	}

	return &productv1.QueryBlogPostsWithFilterResponse{
		BlogPostsWithFilter: results,
	}, nil
}

func (s *MockService) QueryAllBlogPosts(ctx context.Context, in *productv1.QueryAllBlogPostsRequest) (*productv1.QueryAllBlogPostsResponse, error) {
	var results []*productv1.BlogPost

	// Create a variety of blog posts
	for i := 1; i <= 4; i++ {
		var optionalTags *productv1.ListOfString
		var keywords *productv1.ListOfString
		var ratings *productv1.ListOfFloat

		// Vary the optional fields
		if i%2 == 1 {
			optionalTags = &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{fmt.Sprintf("optional%d", i), "common"},
				},
			}
		}

		if i%3 == 0 {
			keywords = &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{fmt.Sprintf("keyword%d", i)},
				},
			}
		}

		if i%2 == 0 {
			ratings = &productv1.ListOfFloat{
				List: &productv1.ListOfFloat_List{
					Items: []float64{float64(i) + 0.5, float64(i) + 1.0},
				},
			}
		}

		results = append(results, &productv1.BlogPost{
			Id:           fmt.Sprintf("blog-%d", i),
			Title:        fmt.Sprintf("Blog Post %d", i),
			Content:      fmt.Sprintf("Content for blog post %d", i),
			Tags:         []string{fmt.Sprintf("tag%d", i), "common"},
			OptionalTags: optionalTags,
			Categories:   []string{fmt.Sprintf("Category%d", i)},
			Keywords:     keywords,
			ViewCounts:   []int32{int32(i * 100), int32(i * 150)},
			Ratings:      ratings,
			IsPublished: &productv1.ListOfBoolean{
				List: &productv1.ListOfBoolean_List{
					Items: []bool{i%2 == 0, true},
				},
			},
			TagGroups: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("group%d", i), "shared"},
						}},
					},
				},
			},
			RelatedTopics: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("topic%d", i)},
						}},
					},
				},
			},
			CommentThreads: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Comment for post %d", i)},
						}},
					},
				},
			},
			// Required complex lists must have data
			RelatedCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-all-%d", i), Name: fmt.Sprintf("Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			Contributors: []*productv1.User{
				{Id: fmt.Sprintf("user-all-%d", i), Name: fmt.Sprintf("Author %d", i)},
			},
			CategoryGroups: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-group-all-%d", i), Name: fmt.Sprintf("Group Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty
			Suggestions: &productv1.ListOfListOfString{},
		})
	}

	return &productv1.QueryAllBlogPostsResponse{
		AllBlogPosts: results,
	}, nil
}

// Author query implementations
func (s *MockService) QueryAuthor(ctx context.Context, in *productv1.QueryAuthorRequest) (*productv1.QueryAuthorResponse, error) {
	result := &productv1.Author{
		Id:   "author-default",
		Name: "Default Author",
		Email: &wrapperspb.StringValue{
			Value: "author@example.com",
		},
		Skills:    []string{"Go", "GraphQL", "Protocol Buffers"},
		Languages: []string{"English", "Spanish", ""},
		SocialLinks: &productv1.ListOfString{
			List: &productv1.ListOfString_List{
				Items: []string{"https://twitter.com/author", "https://linkedin.com/in/author"},
			},
		},
		TeamsByProject: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{
						Items: []string{"Alice", "Bob", "Charlie"},
					}},
					{List: &productv1.ListOfString_List{
						Items: []string{"David", "Eve"},
					}},
				},
			},
		},
		Collaborations: &productv1.ListOfListOfString{
			List: &productv1.ListOfListOfString_List{
				Items: []*productv1.ListOfString{
					{List: &productv1.ListOfString_List{
						Items: []string{"Open Source Project A", "Research Paper B"},
					}},
					{List: &productv1.ListOfString_List{
						Items: []string{"Conference Talk C"},
					}},
				},
			},
		},
		WrittenPosts: &productv1.ListOfBlogPost{
			List: &productv1.ListOfBlogPost_List{
				Items: []*productv1.BlogPost{
					{Id: "blog-1", Title: "GraphQL Best Practices", Content: "Content here..."},
					{Id: "blog-2", Title: "gRPC vs REST", Content: "Comparison content..."},
				},
			},
		},
		FavoriteCategories: []*productv1.Category{
			{Id: "cat-fav-1", Name: "Software Engineering", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
			{Id: "cat-fav-2", Name: "Technical Writing", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
		},
		RelatedAuthors: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "author-rel-1", Name: "Related Author One"},
					{Id: "author-rel-2", Name: "Related Author Two"},
				},
			},
		},
		ProductReviews: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "prod-rev-1", Name: "Code Editor Pro", Price: 199.99},
				},
			},
		},
		AuthorGroups: &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "group-auth-1", Name: "Team Lead Alpha"},
							{Id: "group-auth-2", Name: "Senior Dev Beta"},
						},
					}},
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "group-auth-3", Name: "Junior Dev Gamma"},
						},
					}},
					// empty list
					{List: &productv1.ListOfUser_List{}},
					// null item
					nil,
				},
			},
		},
		CategoryPreferences: &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{
					{List: &productv1.ListOfCategory_List{
						Items: []*productv1.Category{
							{Id: "pref-cat-1", Name: "Microservices", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
							{Id: "pref-cat-2", Name: "Cloud Computing", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
						},
					}},
				},
			},
		},
	}

	return &productv1.QueryAuthorResponse{
		Author: result,
	}, nil
}

func (s *MockService) QueryAuthorById(ctx context.Context, in *productv1.QueryAuthorByIdRequest) (*productv1.QueryAuthorByIdResponse, error) {
	id := in.GetId()

	// Return null for specific test IDs
	if id == "not-found" {
		return &productv1.QueryAuthorByIdResponse{
			AuthorById: nil,
		}, nil
	}

	var result *productv1.Author

	switch id {
	case "minimal":
		result = &productv1.Author{
			Id:        id,
			Name:      "Minimal Author",
			Skills:    []string{"Basic"},
			Languages: []string{"English"},
			TeamsByProject: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{"Solo"},
						}},
					},
				},
			},
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: "cat-minimal", Name: "Basic Category", Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-pref-minimal", Name: "Minimal Preference", Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty
			Collaborations: &productv1.ListOfListOfString{},
		}
	case "experienced":
		result = &productv1.Author{
			Id:   id,
			Name: "Experienced Author",
			Email: &wrapperspb.StringValue{
				Value: "experienced@example.com",
			},
			Skills:    []string{"Go", "GraphQL", "gRPC", "Microservices", "Kubernetes"},
			Languages: []string{"English", "French", "German"},
			SocialLinks: &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{
						"https://github.com/experienced",
						"https://twitter.com/experienced",
						"https://medium.com/@experienced",
					},
				},
			},
			TeamsByProject: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{"Senior Dev 1", "Senior Dev 2", "Tech Lead"},
						}},
						{List: &productv1.ListOfString_List{
							Items: []string{"Architect", "Principal Engineer"},
						}},
						{List: &productv1.ListOfString_List{
							Items: []string{"PM", "Designer", "QA Lead"},
						}},
					},
				},
			},
			Collaborations: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{"Major OSS Project", "Industry Standard", "Research Initiative"},
						}},
						{List: &productv1.ListOfString_List{
							Items: []string{"Conference Keynote", "Workshop Series"},
						}},
					},
				},
			},
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: "cat-experienced-1", Name: "Advanced Programming", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
				{Id: "cat-experienced-2", Name: "Technical Leadership", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: "cat-pref-experienced-1", Name: "System Architecture", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
								{Id: "cat-pref-experienced-2", Name: "Team Management", Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
							},
						}},
					},
				},
			},
		}
	default:
		result = &productv1.Author{
			Id:   id,
			Name: fmt.Sprintf("Author %s", id),
			Email: &wrapperspb.StringValue{
				Value: fmt.Sprintf("%s@example.com", id),
			},
			Skills:    []string{fmt.Sprintf("Skill-%s", id), "General"},
			Languages: []string{"English", fmt.Sprintf("Language-%s", id)},
			TeamsByProject: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Team-%s", id)},
						}},
					},
				},
			},
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-%s", id), Name: fmt.Sprintf("Favorite Category %s", id), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-pref-%s", id), Name: fmt.Sprintf("Preference %s", id), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty
			Collaborations: &productv1.ListOfListOfString{},
		}
	}

	return &productv1.QueryAuthorByIdResponse{
		AuthorById: result,
	}, nil
}

func (s *MockService) QueryAuthorsWithFilter(ctx context.Context, in *productv1.QueryAuthorsWithFilterRequest) (*productv1.QueryAuthorsWithFilterResponse, error) {
	filter := in.GetFilter()
	var results []*productv1.Author

	if filter == nil {
		return &productv1.QueryAuthorsWithFilterResponse{
			AuthorsWithFilter: results,
		}, nil
	}

	nameFilter := ""
	if filter.Name != nil {
		nameFilter = filter.Name.GetValue()
	}

	hasTeams := false
	if filter.HasTeams != nil {
		hasTeams = filter.HasTeams.GetValue()
	}

	skillCount := int32(0)
	if filter.SkillCount != nil {
		skillCount = filter.SkillCount.GetValue()
	}

	// Generate filtered results
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("Filtered Author %d", i)
		if nameFilter != "" {
			name = fmt.Sprintf("%s - Author %d", nameFilter, i)
		}

		var skills []string
		skillsNeeded := skillCount + int32(i)
		for j := int32(0); j < skillsNeeded; j++ {
			skills = append(skills, fmt.Sprintf("Skill%d", j+1))
		}

		var teamsByProject *productv1.ListOfListOfString
		if hasTeams {
			teamsByProject = &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Team%d", i), "SharedTeam"},
						}},
					},
				},
			}
		} else {
			teamsByProject = &productv1.ListOfListOfString{List: &productv1.ListOfListOfString_List{}}
		}

		results = append(results, &productv1.Author{
			Id:             fmt.Sprintf("filtered-author-%d", i),
			Name:           name,
			Skills:         skills,
			Languages:      []string{"English", fmt.Sprintf("Lang%d", i)},
			TeamsByProject: teamsByProject,
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-filtered-%d", i), Name: fmt.Sprintf("Filtered Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-pref-filtered-%d", i), Name: fmt.Sprintf("Filtered Preference %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty
			Collaborations: &productv1.ListOfListOfString{},
		})
	}

	return &productv1.QueryAuthorsWithFilterResponse{
		AuthorsWithFilter: results,
	}, nil
}

func (s *MockService) QueryAllAuthors(ctx context.Context, in *productv1.QueryAllAuthorsRequest) (*productv1.QueryAllAuthorsResponse, error) {
	var results []*productv1.Author

	for i := 1; i <= 3; i++ {
		var email *wrapperspb.StringValue
		var socialLinks *productv1.ListOfString
		var collaborations *productv1.ListOfListOfString

		if i%2 == 1 {
			email = &wrapperspb.StringValue{
				Value: fmt.Sprintf("author%d@example.com", i),
			}
		}

		if i%3 == 0 {
			socialLinks = &productv1.ListOfString{
				List: &productv1.ListOfString_List{
					Items: []string{fmt.Sprintf("https://github.com/author%d", i)},
				},
			}
		}

		if i == 2 {
			collaborations = &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{"Collaboration A", "Collaboration B"},
						}},
					},
				},
			}
		} else {
			collaborations = &productv1.ListOfListOfString{}
		}

		results = append(results, &productv1.Author{
			Id:          fmt.Sprintf("author-%d", i),
			Name:        fmt.Sprintf("Author %d", i),
			Email:       email,
			Skills:      []string{fmt.Sprintf("Skill%d", i), "Common"},
			Languages:   []string{"English", fmt.Sprintf("Language%d", i)},
			SocialLinks: socialLinks,
			TeamsByProject: &productv1.ListOfListOfString{
				List: &productv1.ListOfListOfString_List{
					Items: []*productv1.ListOfString{
						{List: &productv1.ListOfString_List{
							Items: []string{fmt.Sprintf("Team%d", i)},
						}},
					},
				},
			},
			// Required complex lists must have data
			FavoriteCategories: []*productv1.Category{
				{Id: fmt.Sprintf("cat-all-%d", i), Name: fmt.Sprintf("All Category %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
			},
			CategoryPreferences: &productv1.ListOfListOfCategory{
				List: &productv1.ListOfListOfCategory_List{
					Items: []*productv1.ListOfCategory{
						{List: &productv1.ListOfCategory_List{
							Items: []*productv1.Category{
								{Id: fmt.Sprintf("cat-pref-all-%d", i), Name: fmt.Sprintf("All Preference %d", i), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
							},
						}},
					},
				},
			},
			// Optional list - can be empty/variable
			Collaborations: collaborations,
		})
	}

	return &productv1.QueryAllAuthorsResponse{
		AllAuthors: results,
	}, nil
}

// BlogPost mutation implementations
func (s *MockService) MutationCreateBlogPost(ctx context.Context, in *productv1.MutationCreateBlogPostRequest) (*productv1.MutationCreateBlogPostResponse, error) {
	input := in.GetInput()

	result := &productv1.BlogPost{
		Id:             fmt.Sprintf("blog-%d", rand.Intn(1000)),
		Title:          input.GetTitle(),
		Content:        input.GetContent(),
		Tags:           input.GetTags(),
		OptionalTags:   input.GetOptionalTags(),
		Categories:     input.GetCategories(),
		Keywords:       input.GetKeywords(),
		ViewCounts:     input.GetViewCounts(),
		Ratings:        input.GetRatings(),
		IsPublished:    input.GetIsPublished(),
		TagGroups:      input.GetTagGroups(),
		RelatedTopics:  input.GetRelatedTopics(),
		CommentThreads: input.GetCommentThreads(),
		Suggestions:    input.GetSuggestions(),
		// Convert input types to output types
		RelatedCategories: convertCategoryInputListToCategories(input.GetRelatedCategories()),
		Contributors:      convertUserInputsToUsers(input.GetContributors()),
		CategoryGroups:    convertNestedCategoryInputsToCategories(input.GetCategoryGroups()),
		MentionedProducts: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "prod-1", Name: "Sample Product", Price: 99.99},
				},
			},
		},
		MentionedUsers: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "user-3", Name: "Bob Johnson"},
				},
			},
		},
		ContributorTeams: &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "user-4", Name: "Alice Brown"},
						},
					}},
				},
			},
		},
	}

	return &productv1.MutationCreateBlogPostResponse{
		CreateBlogPost: result,
	}, nil
}

func (s *MockService) MutationUpdateBlogPost(ctx context.Context, in *productv1.MutationUpdateBlogPostRequest) (*productv1.MutationUpdateBlogPostResponse, error) {
	id := in.GetId()
	input := in.GetInput()

	if id == "non-existent" {
		return &productv1.MutationUpdateBlogPostResponse{
			UpdateBlogPost: nil,
		}, nil
	}

	result := &productv1.BlogPost{
		Id:             id,
		Title:          input.GetTitle(),
		Content:        input.GetContent(),
		Tags:           input.GetTags(),
		OptionalTags:   input.GetOptionalTags(),
		Categories:     input.GetCategories(),
		Keywords:       input.GetKeywords(),
		ViewCounts:     input.GetViewCounts(),
		Ratings:        input.GetRatings(),
		IsPublished:    input.GetIsPublished(),
		TagGroups:      input.GetTagGroups(),
		RelatedTopics:  input.GetRelatedTopics(),
		CommentThreads: input.GetCommentThreads(),
		Suggestions:    input.GetSuggestions(),
		// Convert input types to output types
		RelatedCategories: convertCategoryInputListToCategories(input.GetRelatedCategories()),
		Contributors:      convertUserInputsToUsers(input.GetContributors()),
		CategoryGroups:    convertNestedCategoryInputsToCategories(input.GetCategoryGroups()),
		MentionedProducts: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "prod-updated", Name: "Updated Product", Price: 149.99},
				},
			},
		},
		MentionedUsers: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "user-updated", Name: "Updated User"},
				},
			},
		},
		ContributorTeams: &productv1.ListOfListOfUser{
			List: &productv1.ListOfListOfUser_List{
				Items: []*productv1.ListOfUser{
					{List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: "user-team-updated", Name: "Updated Team Member"},
						},
					}},
				},
			},
		},
	}

	return &productv1.MutationUpdateBlogPostResponse{
		UpdateBlogPost: result,
	}, nil
}

// Author mutation implementations
func (s *MockService) MutationCreateAuthor(ctx context.Context, in *productv1.MutationCreateAuthorRequest) (*productv1.MutationCreateAuthorResponse, error) {
	input := in.GetInput()

	result := &productv1.Author{
		Id:             fmt.Sprintf("author-%d", rand.Intn(1000)),
		Name:           input.GetName(),
		Email:          input.GetEmail(),
		Skills:         input.GetSkills(),
		Languages:      input.GetLanguages(),
		SocialLinks:    input.GetSocialLinks(),
		TeamsByProject: input.GetTeamsByProject(),
		Collaborations: input.GetCollaborations(),
		// Convert input types to output types for complex fields
		FavoriteCategories: convertCategoryInputsToCategories(input.GetFavoriteCategories()),
		AuthorGroups:       convertNestedUserInputsToUsers(input.GetAuthorGroups()),
		ProjectTeams:       convertNestedUserInputsToUsers(input.GetProjectTeams()),
		// Keep other complex fields with mock data since they're not in the simplified input
		WrittenPosts: &productv1.ListOfBlogPost{
			List: &productv1.ListOfBlogPost_List{
				Items: []*productv1.BlogPost{
					{Id: "blog-created", Title: "Created Post", Content: "Content..."},
				},
			},
		},
		RelatedAuthors: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "related-author", Name: "Related Author"},
				},
			},
		},
		ProductReviews: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "reviewed-product", Name: "Code Editor", Price: 199.99},
				},
			},
		},
		CategoryPreferences: &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{
					{List: &productv1.ListOfCategory_List{
						Items: []*productv1.Category{
							{Id: "pref-cat", Name: "Backend Development", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
						},
					}},
				},
			},
		},
	}

	return &productv1.MutationCreateAuthorResponse{
		CreateAuthor: result,
	}, nil
}

func (s *MockService) MutationUpdateAuthor(ctx context.Context, in *productv1.MutationUpdateAuthorRequest) (*productv1.MutationUpdateAuthorResponse, error) {
	id := in.GetId()
	input := in.GetInput()

	if id == "non-existent" {
		return &productv1.MutationUpdateAuthorResponse{
			UpdateAuthor: nil,
		}, nil
	}

	result := &productv1.Author{
		Id:             id,
		Name:           input.GetName(),
		Email:          input.GetEmail(),
		Skills:         input.GetSkills(),
		Languages:      input.GetLanguages(),
		SocialLinks:    input.GetSocialLinks(),
		TeamsByProject: input.GetTeamsByProject(),
		Collaborations: input.GetCollaborations(),
		// Convert input types to output types for complex fields
		FavoriteCategories: convertCategoryInputsToCategories(input.GetFavoriteCategories()),
		AuthorGroups:       convertNestedUserInputsToUsers(input.GetAuthorGroups()),
		ProjectTeams:       convertNestedUserInputsToUsers(input.GetProjectTeams()),
		// Keep other complex fields with mock data since they're not in the simplified input
		WrittenPosts: &productv1.ListOfBlogPost{
			List: &productv1.ListOfBlogPost_List{
				Items: []*productv1.BlogPost{
					{Id: "blog-updated", Title: "Updated Post", Content: "Updated content..."},
				},
			},
		},
		RelatedAuthors: &productv1.ListOfUser{
			List: &productv1.ListOfUser_List{
				Items: []*productv1.User{
					{Id: "related-author-updated", Name: "Updated Related Author"},
				},
			},
		},
		ProductReviews: &productv1.ListOfProduct{
			List: &productv1.ListOfProduct_List{
				Items: []*productv1.Product{
					{Id: "reviewed-product-updated", Name: "Updated Code Editor", Price: 249.99},
				},
			},
		},
		CategoryPreferences: &productv1.ListOfListOfCategory{
			List: &productv1.ListOfListOfCategory_List{
				Items: []*productv1.ListOfCategory{
					{List: &productv1.ListOfCategory_List{
						Items: []*productv1.Category{
							{Id: "pref-cat-updated", Name: "Updated Backend Development", Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
						},
					}},
				},
			},
		},
	}

	return &productv1.MutationUpdateAuthorResponse{
		UpdateAuthor: result,
	}, nil
}

// Bulk operation implementations
func (s *MockService) QueryBulkSearchAuthors(ctx context.Context, in *productv1.QueryBulkSearchAuthorsRequest) (*productv1.QueryBulkSearchAuthorsResponse, error) {
	var allResults []*productv1.Author

	// Handle nullable list - if filters is nil, return empty results
	if in.Filters == nil {
		return &productv1.QueryBulkSearchAuthorsResponse{
			BulkSearchAuthors: allResults,
		}, nil
	}

	// Process each filter in the list
	if in.Filters.List != nil {
		for i, filter := range in.Filters.List.Items {
			// Create mock results for each filter
			for j := 1; j <= 2; j++ {
				name := fmt.Sprintf("Bulk Author %d-%d", i+1, j)
				if filter.Name != nil {
					name = fmt.Sprintf("%s - Bulk %d-%d", filter.Name.GetValue(), i+1, j)
				}

				var skills []string
				skillCount := int32(3)
				if filter.SkillCount != nil {
					skillCount = filter.SkillCount.GetValue()
				}
				for k := int32(0); k < skillCount; k++ {
					skills = append(skills, fmt.Sprintf("BulkSkill%d", k+1))
				}

				var teamsByProject *productv1.ListOfListOfString
				if filter.HasTeams != nil && filter.HasTeams.GetValue() {
					teamsByProject = &productv1.ListOfListOfString{
						List: &productv1.ListOfListOfString_List{
							Items: []*productv1.ListOfString{
								{List: &productv1.ListOfString_List{
									Items: []string{fmt.Sprintf("BulkTeam%d", j), "SharedBulkTeam"},
								}},
							},
						},
					}
				} else {
					teamsByProject = &productv1.ListOfListOfString{List: &productv1.ListOfListOfString_List{}}
				}

				allResults = append(allResults, &productv1.Author{
					Id:             fmt.Sprintf("bulk-author-%d-%d", i+1, j),
					Name:           name,
					Skills:         skills,
					Languages:      []string{"English", fmt.Sprintf("BulkLang%d", j)},
					TeamsByProject: teamsByProject,
					FavoriteCategories: []*productv1.Category{
						{Id: fmt.Sprintf("bulk-cat-%d-%d", i+1, j), Name: fmt.Sprintf("Bulk Category %d-%d", i+1, j), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
					},
					CategoryPreferences: &productv1.ListOfListOfCategory{
						List: &productv1.ListOfListOfCategory_List{
							Items: []*productv1.ListOfCategory{
								{List: &productv1.ListOfCategory_List{
									Items: []*productv1.Category{
										{Id: fmt.Sprintf("bulk-pref-%d-%d", i+1, j), Name: fmt.Sprintf("Bulk Preference %d-%d", i+1, j), Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
									},
								}},
							},
						},
					},
				})
			}
		}
	}

	return &productv1.QueryBulkSearchAuthorsResponse{
		BulkSearchAuthors: allResults,
	}, nil
}

func (s *MockService) QueryBulkSearchBlogPosts(ctx context.Context, in *productv1.QueryBulkSearchBlogPostsRequest) (*productv1.QueryBulkSearchBlogPostsResponse, error) {
	var allResults []*productv1.BlogPost

	// Handle nullable list - if filters is nil, return empty results
	if in.Filters == nil {
		return &productv1.QueryBulkSearchBlogPostsResponse{
			BulkSearchBlogPosts: allResults,
		}, nil
	}

	// Process each filter in the list
	if in.Filters.List != nil {
		for i, filter := range in.Filters.List.Items {
			// Create mock results for each filter
			for j := 1; j <= 2; j++ {
				title := fmt.Sprintf("Bulk Blog Post %d-%d", i+1, j)
				if filter.Title != nil {
					title = fmt.Sprintf("%s - Bulk %d-%d", filter.Title.GetValue(), i+1, j)
				}

				var categories []string
				if filter.HasCategories != nil && filter.HasCategories.GetValue() {
					categories = []string{fmt.Sprintf("BulkCategory%d", j), "SharedBulkCategory"}
				} else {
					categories = []string{}
				}

				minTags := int32(2)
				if filter.MinTags != nil {
					minTags = filter.MinTags.GetValue()
				}
				var tags []string
				for k := int32(0); k < minTags; k++ {
					tags = append(tags, fmt.Sprintf("BulkTag%d", k+1))
				}

				allResults = append(allResults, &productv1.BlogPost{
					Id:         fmt.Sprintf("bulk-post-%d-%d", i+1, j),
					Title:      title,
					Content:    fmt.Sprintf("Bulk content for post %d-%d", i+1, j),
					Tags:       tags,
					Categories: categories,
					ViewCounts: []int32{100, 150, 200},
					TagGroups: &productv1.ListOfListOfString{
						List: &productv1.ListOfListOfString_List{
							Items: []*productv1.ListOfString{
								{List: &productv1.ListOfString_List{
									Items: []string{fmt.Sprintf("BulkGroup%d", j)},
								}},
							},
						},
					},
					RelatedTopics: &productv1.ListOfListOfString{
						List: &productv1.ListOfListOfString_List{
							Items: []*productv1.ListOfString{
								{List: &productv1.ListOfString_List{
									Items: []string{fmt.Sprintf("BulkTopic%d", j)},
								}},
							},
						},
					},
					CommentThreads: &productv1.ListOfListOfString{
						List: &productv1.ListOfListOfString_List{
							Items: []*productv1.ListOfString{
								{List: &productv1.ListOfString_List{
									Items: []string{fmt.Sprintf("BulkComment%d", j)},
								}},
							},
						},
					},
					RelatedCategories: []*productv1.Category{
						{Id: fmt.Sprintf("bulk-rel-cat-%d-%d", i+1, j), Name: fmt.Sprintf("Bulk Related %d-%d", i+1, j), Kind: productv1.CategoryKind_CATEGORY_KIND_OTHER},
					},
					Contributors: []*productv1.User{
						{Id: fmt.Sprintf("bulk-contrib-%d-%d", i+1, j), Name: fmt.Sprintf("Bulk Contributor %d-%d", i+1, j)},
					},
					CategoryGroups: &productv1.ListOfListOfCategory{
						List: &productv1.ListOfListOfCategory_List{
							Items: []*productv1.ListOfCategory{
								{List: &productv1.ListOfCategory_List{
									Items: []*productv1.Category{
										{Id: fmt.Sprintf("bulk-grp-cat-%d-%d", i+1, j), Name: fmt.Sprintf("Bulk Group Cat %d-%d", i+1, j), Kind: productv1.CategoryKind_CATEGORY_KIND_BOOK},
									},
								}},
							},
						},
					},
				})
			}
		}
	}

	return &productv1.QueryBulkSearchBlogPostsResponse{
		BulkSearchBlogPosts: allResults,
	}, nil
}

func (s *MockService) MutationBulkCreateAuthors(ctx context.Context, in *productv1.MutationBulkCreateAuthorsRequest) (*productv1.MutationBulkCreateAuthorsResponse, error) {
	var results []*productv1.Author

	// Handle nullable list - if authors is nil, return empty results
	if in.Authors == nil {
		return &productv1.MutationBulkCreateAuthorsResponse{
			BulkCreateAuthors: results,
		}, nil
	}

	// Process each author input in the list
	if in.Authors.List != nil {
		for i, authorInput := range in.Authors.List.Items {
			// Convert nested UserInput lists to Users for complex fields
			var authorGroups *productv1.ListOfListOfUser
			if authorInput.AuthorGroups != nil {
				authorGroups = convertNestedUserInputsToUsers(authorInput.AuthorGroups)
			}

			var projectTeams *productv1.ListOfListOfUser
			if authorInput.ProjectTeams != nil {
				projectTeams = convertNestedUserInputsToUsers(authorInput.ProjectTeams)
			}

			// Convert CategoryInput list to Categories
			var favoriteCategories []*productv1.Category
			if authorInput.FavoriteCategories != nil {
				favoriteCategories = convertCategoryInputsToCategories(authorInput.FavoriteCategories)
			}

			author := &productv1.Author{
				Id:                 fmt.Sprintf("bulk-created-author-%d", i+1),
				Name:               authorInput.Name,
				Email:              authorInput.Email,
				Skills:             authorInput.Skills,
				Languages:          authorInput.Languages,
				SocialLinks:        authorInput.SocialLinks,
				TeamsByProject:     authorInput.TeamsByProject,
				Collaborations:     authorInput.Collaborations,
				FavoriteCategories: favoriteCategories,
				AuthorGroups:       authorGroups,
				ProjectTeams:       projectTeams,
				// Add required complex fields with mock data
				WrittenPosts: &productv1.ListOfBlogPost{
					List: &productv1.ListOfBlogPost_List{
						Items: []*productv1.BlogPost{
							{Id: fmt.Sprintf("bulk-blog-%d", i+1), Title: fmt.Sprintf("Bulk Created Post %d", i+1), Content: "Bulk created content..."},
						},
					},
				},
				RelatedAuthors: &productv1.ListOfUser{
					List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: fmt.Sprintf("bulk-rel-author-%d", i+1), Name: fmt.Sprintf("Bulk Related Author %d", i+1)},
						},
					},
				},
				ProductReviews: &productv1.ListOfProduct{
					List: &productv1.ListOfProduct_List{
						Items: []*productv1.Product{
							{Id: fmt.Sprintf("bulk-prod-%d", i+1), Name: fmt.Sprintf("Bulk Product %d", i+1), Price: 99.99},
						},
					},
				},
				CategoryPreferences: &productv1.ListOfListOfCategory{
					List: &productv1.ListOfListOfCategory_List{
						Items: []*productv1.ListOfCategory{
							{List: &productv1.ListOfCategory_List{
								Items: []*productv1.Category{
									{Id: fmt.Sprintf("bulk-cat-pref-%d", i+1), Name: fmt.Sprintf("Bulk Category Preference %d", i+1), Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
								},
							}},
						},
					},
				},
			}

			results = append(results, author)
		}
	}

	return &productv1.MutationBulkCreateAuthorsResponse{
		BulkCreateAuthors: results,
	}, nil
}

func (s *MockService) MutationBulkUpdateAuthors(ctx context.Context, in *productv1.MutationBulkUpdateAuthorsRequest) (*productv1.MutationBulkUpdateAuthorsResponse, error) {
	var results []*productv1.Author

	// Handle nullable list - if authors is nil, return empty results
	if in.Authors == nil {
		return &productv1.MutationBulkUpdateAuthorsResponse{
			BulkUpdateAuthors: results,
		}, nil
	}

	// Process each author input in the list
	if in.Authors.List != nil {
		for i, authorInput := range in.Authors.List.Items {
			// Convert nested UserInput lists to Users for complex fields
			var authorGroups *productv1.ListOfListOfUser
			if authorInput.AuthorGroups != nil {
				authorGroups = convertNestedUserInputsToUsers(authorInput.AuthorGroups)
			}

			var projectTeams *productv1.ListOfListOfUser
			if authorInput.ProjectTeams != nil {
				projectTeams = convertNestedUserInputsToUsers(authorInput.ProjectTeams)
			}

			// Convert CategoryInput list to Categories
			var favoriteCategories []*productv1.Category
			if authorInput.FavoriteCategories != nil {
				favoriteCategories = convertCategoryInputsToCategories(authorInput.FavoriteCategories)
			}

			author := &productv1.Author{
				Id:                 fmt.Sprintf("bulk-updated-author-%d", i+1),
				Name:               authorInput.Name,
				Email:              authorInput.Email,
				Skills:             authorInput.Skills,
				Languages:          authorInput.Languages,
				SocialLinks:        authorInput.SocialLinks,
				TeamsByProject:     authorInput.TeamsByProject,
				Collaborations:     authorInput.Collaborations,
				FavoriteCategories: favoriteCategories,
				AuthorGroups:       authorGroups,
				ProjectTeams:       projectTeams,
				// Add required complex fields with mock data
				WrittenPosts: &productv1.ListOfBlogPost{
					List: &productv1.ListOfBlogPost_List{
						Items: []*productv1.BlogPost{
							{Id: fmt.Sprintf("bulk-updated-blog-%d", i+1), Title: fmt.Sprintf("Bulk Updated Post %d", i+1), Content: "Bulk updated content..."},
						},
					},
				},
				RelatedAuthors: &productv1.ListOfUser{
					List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: fmt.Sprintf("bulk-updated-rel-author-%d", i+1), Name: fmt.Sprintf("Bulk Updated Related Author %d", i+1)},
						},
					},
				},
				ProductReviews: &productv1.ListOfProduct{
					List: &productv1.ListOfProduct_List{
						Items: []*productv1.Product{
							{Id: fmt.Sprintf("bulk-updated-prod-%d", i+1), Name: fmt.Sprintf("Bulk Updated Product %d", i+1), Price: 149.99},
						},
					},
				},
				CategoryPreferences: &productv1.ListOfListOfCategory{
					List: &productv1.ListOfListOfCategory_List{
						Items: []*productv1.ListOfCategory{
							{List: &productv1.ListOfCategory_List{
								Items: []*productv1.Category{
									{Id: fmt.Sprintf("bulk-updated-cat-pref-%d", i+1), Name: fmt.Sprintf("Bulk Updated Category Preference %d", i+1), Kind: productv1.CategoryKind_CATEGORY_KIND_ELECTRONICS},
								},
							}},
						},
					},
				},
			}

			results = append(results, author)
		}
	}

	return &productv1.MutationBulkUpdateAuthorsResponse{
		BulkUpdateAuthors: results,
	}, nil
}

func (s *MockService) MutationBulkCreateBlogPosts(ctx context.Context, in *productv1.MutationBulkCreateBlogPostsRequest) (*productv1.MutationBulkCreateBlogPostsResponse, error) {
	var results []*productv1.BlogPost

	// Handle nullable list - if blogPosts is nil, return empty results
	if in.BlogPosts == nil {
		return &productv1.MutationBulkCreateBlogPostsResponse{
			BulkCreateBlogPosts: results,
		}, nil
	}

	// Process each blog post input in the list
	if in.BlogPosts.List != nil {
		for i, blogPostInput := range in.BlogPosts.List.Items {
			// Convert CategoryInput lists to Categories
			var relatedCategories []*productv1.Category
			if blogPostInput.RelatedCategories != nil {
				relatedCategories = convertCategoryInputListToCategories(blogPostInput.RelatedCategories)
			}

			var contributors []*productv1.User
			if blogPostInput.Contributors != nil {
				contributors = convertUserInputsToUsers(blogPostInput.Contributors)
			}

			var categoryGroups *productv1.ListOfListOfCategory
			if blogPostInput.CategoryGroups != nil {
				categoryGroups = convertNestedCategoryInputsToCategories(blogPostInput.CategoryGroups)
			}

			blogPost := &productv1.BlogPost{
				Id:                fmt.Sprintf("bulk-created-post-%d", i+1),
				Title:             blogPostInput.Title,
				Content:           blogPostInput.Content,
				Tags:              blogPostInput.Tags,
				OptionalTags:      blogPostInput.OptionalTags,
				Categories:        blogPostInput.Categories,
				Keywords:          blogPostInput.Keywords,
				ViewCounts:        blogPostInput.ViewCounts,
				Ratings:           blogPostInput.Ratings,
				IsPublished:       blogPostInput.IsPublished,
				TagGroups:         blogPostInput.TagGroups,
				RelatedTopics:     blogPostInput.RelatedTopics,
				CommentThreads:    blogPostInput.CommentThreads,
				Suggestions:       blogPostInput.Suggestions,
				RelatedCategories: relatedCategories,
				Contributors:      contributors,
				CategoryGroups:    categoryGroups,
				// Add required fields with mock data
				MentionedProducts: &productv1.ListOfProduct{
					List: &productv1.ListOfProduct_List{
						Items: []*productv1.Product{
							{Id: fmt.Sprintf("bulk-prod-%d", i+1), Name: fmt.Sprintf("Bulk Created Product %d", i+1), Price: 99.99},
						},
					},
				},
				MentionedUsers: &productv1.ListOfUser{
					List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: fmt.Sprintf("bulk-user-%d", i+1), Name: fmt.Sprintf("Bulk Created User %d", i+1)},
						},
					},
				},
				ContributorTeams: &productv1.ListOfListOfUser{
					List: &productv1.ListOfListOfUser_List{
						Items: []*productv1.ListOfUser{
							{List: &productv1.ListOfUser_List{
								Items: []*productv1.User{
									{Id: fmt.Sprintf("bulk-team-%d", i+1), Name: fmt.Sprintf("Bulk Created Team Member %d", i+1)},
								},
							}},
						},
					},
				},
			}

			results = append(results, blogPost)
		}
	}

	return &productv1.MutationBulkCreateBlogPostsResponse{
		BulkCreateBlogPosts: results,
	}, nil
}

func (s *MockService) MutationBulkUpdateBlogPosts(ctx context.Context, in *productv1.MutationBulkUpdateBlogPostsRequest) (*productv1.MutationBulkUpdateBlogPostsResponse, error) {
	var results []*productv1.BlogPost

	// Handle nullable list - if blogPosts is nil, return empty results
	if in.BlogPosts == nil {
		return &productv1.MutationBulkUpdateBlogPostsResponse{
			BulkUpdateBlogPosts: results,
		}, nil
	}

	// Process each blog post input in the list
	if in.BlogPosts.List != nil {
		for i, blogPostInput := range in.BlogPosts.List.Items {
			// Convert CategoryInput lists to Categories
			var relatedCategories []*productv1.Category
			if blogPostInput.RelatedCategories != nil {
				relatedCategories = convertCategoryInputListToCategories(blogPostInput.RelatedCategories)
			}

			var contributors []*productv1.User
			if blogPostInput.Contributors != nil {
				contributors = convertUserInputsToUsers(blogPostInput.Contributors)
			}

			var categoryGroups *productv1.ListOfListOfCategory
			if blogPostInput.CategoryGroups != nil {
				categoryGroups = convertNestedCategoryInputsToCategories(blogPostInput.CategoryGroups)
			}

			blogPost := &productv1.BlogPost{
				Id:                fmt.Sprintf("bulk-updated-post-%d", i+1),
				Title:             blogPostInput.Title,
				Content:           blogPostInput.Content,
				Tags:              blogPostInput.Tags,
				OptionalTags:      blogPostInput.OptionalTags,
				Categories:        blogPostInput.Categories,
				Keywords:          blogPostInput.Keywords,
				ViewCounts:        blogPostInput.ViewCounts,
				Ratings:           blogPostInput.Ratings,
				IsPublished:       blogPostInput.IsPublished,
				TagGroups:         blogPostInput.TagGroups,
				RelatedTopics:     blogPostInput.RelatedTopics,
				CommentThreads:    blogPostInput.CommentThreads,
				Suggestions:       blogPostInput.Suggestions,
				RelatedCategories: relatedCategories,
				Contributors:      contributors,
				CategoryGroups:    categoryGroups,
				// Add required fields with mock data
				MentionedProducts: &productv1.ListOfProduct{
					List: &productv1.ListOfProduct_List{
						Items: []*productv1.Product{
							{Id: fmt.Sprintf("bulk-updated-prod-%d", i+1), Name: fmt.Sprintf("Bulk Updated Product %d", i+1), Price: 149.99},
						},
					},
				},
				MentionedUsers: &productv1.ListOfUser{
					List: &productv1.ListOfUser_List{
						Items: []*productv1.User{
							{Id: fmt.Sprintf("bulk-updated-user-%d", i+1), Name: fmt.Sprintf("Bulk Updated User %d", i+1)},
						},
					},
				},
				ContributorTeams: &productv1.ListOfListOfUser{
					List: &productv1.ListOfListOfUser_List{
						Items: []*productv1.ListOfUser{
							{List: &productv1.ListOfUser_List{
								Items: []*productv1.User{
									{Id: fmt.Sprintf("bulk-updated-team-%d", i+1), Name: fmt.Sprintf("Bulk Updated Team Member %d", i+1)},
								},
							}},
						},
					},
				},
			}

			results = append(results, blogPost)
		}
	}

	return &productv1.MutationBulkUpdateBlogPostsResponse{
		BulkUpdateBlogPosts: results,
	}, nil
}
