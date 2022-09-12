package postprocess

import (
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestProcessInjectHeader_Process(t *testing.T) {

	pre := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					BufferId:   0,
					Input:      `{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"}}`,
					DataSource: nil,
					Variables: []resolve.Variable{
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								Input:    `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"$$0$$","__typename":"User"}]}}}`,
								Variables: resolve.NewVariables(
									&resolve.ObjectVariable{
										Path: []string{"id"},
									},
								),
								DataSource: nil,
								ProcessResponseConfig: resolve.ProcessResponseConfig{
									ExtractGraphqlResponse:    true,
									ExtractFederationEntities: true,
								},
							},
							Path:     []string{"me"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("username"),
									Value: &resolve.String{
										Path: []string{"username"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	expected := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					BufferId: 0,
					Variables: []resolve.Variable{
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					},
					Input:         `{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"},"header":{"X-Tyk-Custom":["hello"]}}`,
					InputTemplate: resolve.InputTemplate{},
					DataSource:    nil,
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								Variables: resolve.NewVariables(
									&resolve.ObjectVariable{
										Path: []string{"id"},
									},
								),
								Input:         `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"$$0$$","__typename":"User"}]}},"header":{"X-Tyk-Custom":["hello"]}}`,
								InputTemplate: resolve.InputTemplate{},
								DataSource:    nil,
								ProcessResponseConfig: resolve.ProcessResponseConfig{
									ExtractGraphqlResponse:    true,
									ExtractFederationEntities: true,
								},
							},
							Path:     []string{"me"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("username"),
									Value: &resolve.String{
										Path: []string{"username"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	processor := &ProcessInjectHeader{
		header: map[string][]string{
			"X-Tyk-Custom": {"hello"},
		},
	}
	actual := processor.Process(pre)

	assert.Equal(t, expected, actual)
}

func TestProcessInjectHeader_injectHeader(t *testing.T) {
	testCases := []struct {
		name     string
		in       []byte
		expected string
	}{
		{
			name:     "no existing header",
			in:       []byte(`{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"}}`),
			expected: `{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"},"header":{"custom":["hello"]}}`,
		},
		{
			name:     "existing header",
			in:       []byte(`{"method":"POST","header":{"test":["holla"]},"url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"}}`),
			expected: `{"method":"POST","header":{"custom":["hello"],"test":["holla"]},"url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"}}`,
		},
		{
			name:     "invalid header",
			in:       []byte(`{"method":"POST","header":1,"url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"}}`),
			expected: `{"method":"POST","header":1,"url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"}}`,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			processor := &ProcessInjectHeader{header: map[string][]string{
				"custom": {"hello"},
			}}
			gotten := processor.injectHeader(test.in)
			assert.Equal(t, test.expected, gotten)
		})
	}
}
