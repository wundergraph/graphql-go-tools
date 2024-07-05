package postprocess

import (
	"context"
	"io"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
)

func TestDataSourceInput_Process(t *testing.T) {
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
								SetTemplateOutputToNullOnVariableNull: true,
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

								{

									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("reviews"),
									Value: &resolve.Array{
										Path:     []string{"reviews"},
										Nullable: true,
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("body"),
													Value: &resolve.String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("product"),
													Value: &resolve.Object{
														Path: []string{"product"},
														Fetch: &resolve.SingleFetch{
															BufferId:   2,
															Input:      `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"$$0$$","__typename":"Product"}]}}}`,
															DataSource: nil,
															Variables: resolve.NewVariables(
																&resolve.ObjectVariable{
																	Path: []string{"upc"},
																},
															),
															ProcessResponseConfig: resolve.ProcessResponseConfig{
																ExtractGraphqlResponse:    true,
																ExtractFederationEntities: true,
															},
															SetTemplateOutputToNullOnVariableNull: true,
														},
														Fields: []*resolve.Field{
															{
																HasBuffer: true,
																BufferID:  2,
																Name:      []byte("name"),
																Value: &resolve.String{
																	Path: []string{"name"},
																},
															},
														},
													},
												},
											},
										},
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
					InputTemplate: resolve.InputTemplate{
						Segments: []resolve.TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:4001/`),
								SegmentType: resolve.StaticSegmentType,
							},
							{
								SegmentType:        resolve.VariableSegmentType,
								VariableKind:       resolve.HeaderVariableKind,
								VariableSourcePath: []string{"Authorization"},
							},
							{
								Data:        []byte(`","body":{"query":"{me {id username}}"}}`),
								SegmentType: resolve.StaticSegmentType,
							},
						},
					},
					DataSource: nil,
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("me"),
						Value: &resolve.Object{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								InputTemplate: resolve.InputTemplate{
									Segments: []resolve.TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"`),
											SegmentType: resolve.StaticSegmentType,
										},
										{
											SegmentType:        resolve.VariableSegmentType,
											VariableKind:       resolve.ObjectVariableKind,
											VariableSourcePath: []string{"id"},
										},
										{
											Data:        []byte(`","__typename":"User"}]}}}`),
											SegmentType: resolve.StaticSegmentType,
										},
									},
									SetTemplateOutputToNullOnVariableNull: true,
								},
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

								{

									HasBuffer: true,
									BufferID:  1,
									Name:      []byte("reviews"),
									Value: &resolve.Array{
										Path:     []string{"reviews"},
										Nullable: true,
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("body"),
													Value: &resolve.String{
														Path: []string{"body"},
													},
												},
												{
													Name: []byte("product"),
													Value: &resolve.Object{
														Path: []string{"product"},
														Fetch: &resolve.SingleFetch{
															BufferId: 2,
															InputTemplate: resolve.InputTemplate{
																Segments: []resolve.TemplateSegment{
																	{
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																	{
																		SegmentType:        resolve.VariableSegmentType,
																		VariableKind:       resolve.ObjectVariableKind,
																		VariableSourcePath: []string{"upc"},
																	},
																	{
																		Data:        []byte(`","__typename":"Product"}]}}}`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																},
																SetTemplateOutputToNullOnVariableNull: true,
															},
															ProcessResponseConfig: resolve.ProcessResponseConfig{
																ExtractGraphqlResponse:    true,
																ExtractFederationEntities: true,
															},
														},
														Fields: []*resolve.Field{
															{
																HasBuffer: true,
																BufferID:  2,
																Name:      []byte("name"),
																Value: &resolve.String{
																	Path: []string{"name"},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	processor := &ProcessDataSource{}
	actual := processor.Process(pre)

	assert.Equal(t, expected, actual)
}

func TestDataSourceInput_Subscription_Process(t *testing.T) {

	pre := &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"method":"POST","url":"http://localhost:4001/$$0$$","body":{"query":"{me {id username}}"}}`),
				Variables: []resolve.Variable{
					&resolve.HeaderVariable{
						Path: []string{"Authorization"},
					},
				},
			},
			Response: &resolve.GraphQLResponse{},
		},
	}

	expected := &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				InputTemplate: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001/`),
							SegmentType: resolve.StaticSegmentType,
						},
						{
							SegmentType:        resolve.VariableSegmentType,
							VariableKind:       resolve.HeaderVariableKind,
							VariableSourcePath: []string{"Authorization"},
						},
						{
							Data:        []byte(`","body":{"query":"{me {id username}}"}}`),
							SegmentType: resolve.StaticSegmentType,
						},
					},
				},
			},
			Response: &resolve.GraphQLResponse{},
		},
	}

	processor := &ProcessDataSource{}
	actual := processor.Process(pre)

	assert.Equal(t, expected, actual)
}

func TestDataSourceInput_Process_correctGraphQLVariableTypes(t *testing.T) {
	input := "{\"body\":{\"variables\":{\"countryCode\":\"$$0$$\"},\"query\":\"query ($countryCode: ID!) {\\n  country(code: $countryCode) {\\n    emoji\\n  }\\n}\"},\"method\":\"POST\",\"url\":\"https://countries.trevorblades.com/\",\"header\":{}}"

	t.Run("when JsonRootType is NotExists", func(t *testing.T) {
		variable := &resolve.ContextVariable{
			Path:     []string{"countryCode"},
			Renderer: &mockVariableRenderer{renderer: resolve.NewPlainVariableRenderer(), rootValueType: resolve.JsonRootType{Value: jsonparser.NotExist}},
		}
		variables := resolve.NewVariables(variable)
		correctedInput := correctGraphQLVariableTypes(variables, input)
		result, _, _, err := jsonparser.Get([]byte(correctedInput), "body", "variables")
		assert.NoError(t, err)
		assert.Equal(t, "{\"countryCode\":\"$$0$$\"}", string(result))
	})

	t.Run("when JsonRootType is String", func(t *testing.T) {
		variable := &resolve.ContextVariable{
			Path:     []string{"countryCode"},
			Renderer: &mockVariableRenderer{renderer: resolve.NewPlainVariableRenderer(), rootValueType: resolve.JsonRootType{Value: jsonparser.String}},
		}
		variables := resolve.NewVariables(variable)
		correctedInput := correctGraphQLVariableTypes(variables, input)
		result, _, _, err := jsonparser.Get([]byte(correctedInput), "body", "variables")
		assert.NoError(t, err)
		assert.Equal(t, "{\"countryCode\":\"$$0$$\"}", string(result))
	})

	t.Run("when JsonRootType is Number", func(t *testing.T) {
		variable := &resolve.ContextVariable{
			Path:     []string{"countryCode"},
			Renderer: &mockVariableRenderer{renderer: resolve.NewPlainVariableRenderer(), rootValueType: resolve.JsonRootType{Value: jsonparser.Number}},
		}
		variables := resolve.NewVariables(variable)
		correctedInput := correctGraphQLVariableTypes(variables, input)
		result, _, _, err := jsonparser.Get([]byte(correctedInput), "body", "variables")
		assert.NoError(t, err)
		assert.Equal(t, "{\"countryCode\":$$0$$}", string(result))
	})

	t.Run("when JsonRootType is Object", func(t *testing.T) {
		variable := &resolve.ContextVariable{
			Path:     []string{"countryCode"},
			Renderer: &mockVariableRenderer{renderer: resolve.NewPlainVariableRenderer(), rootValueType: resolve.JsonRootType{Value: jsonparser.Object}},
		}
		variables := resolve.NewVariables(variable)
		correctedInput := correctGraphQLVariableTypes(variables, input)
		result, _, _, err := jsonparser.Get([]byte(correctedInput), "body", "variables")
		assert.NoError(t, err)
		assert.Equal(t, "{\"countryCode\":$$0$$}", string(result))
	})

	t.Run("when JsonRootType is Array", func(t *testing.T) {
		variable := &resolve.ContextVariable{
			Path:     []string{"countryCode"},
			Renderer: &mockVariableRenderer{renderer: resolve.NewPlainVariableRenderer(), rootValueType: resolve.JsonRootType{Value: jsonparser.Array}},
		}
		variables := resolve.NewVariables(variable)
		correctedInput := correctGraphQLVariableTypes(variables, input)
		result, _, _, err := jsonparser.Get([]byte(correctedInput), "body", "variables")
		assert.NoError(t, err)
		assert.Equal(t, "{\"countryCode\":$$0$$}", string(result))
	})

	t.Run("when JsonRootType is Boolean", func(t *testing.T) {
		variable := &resolve.ContextVariable{
			Path:     []string{"countryCode"},
			Renderer: &mockVariableRenderer{renderer: resolve.NewPlainVariableRenderer(), rootValueType: resolve.JsonRootType{Value: jsonparser.Boolean}},
		}
		variables := resolve.NewVariables(variable)
		correctedInput := correctGraphQLVariableTypes(variables, input)
		result, _, _, err := jsonparser.Get([]byte(correctedInput), "body", "variables")
		assert.NoError(t, err)
		assert.Equal(t, "{\"countryCode\":$$0$$}", string(result))
	})

	t.Run("when JsonRootType is Null", func(t *testing.T) {
		variable := &resolve.ContextVariable{
			Path:     []string{"countryCode"},
			Renderer: &mockVariableRenderer{renderer: resolve.NewPlainVariableRenderer(), rootValueType: resolve.JsonRootType{Value: jsonparser.Null}},
		}
		variables := resolve.NewVariables(variable)
		correctedInput := correctGraphQLVariableTypes(variables, input)
		result, _, _, err := jsonparser.Get([]byte(correctedInput), "body", "variables")
		assert.NoError(t, err)
		assert.Equal(t, "{\"countryCode\":$$0$$}", string(result))
	})

	t.Run("when JsonRootType is Unknown", func(t *testing.T) {
		variable := &resolve.ContextVariable{
			Path:     []string{"countryCode"},
			Renderer: &mockVariableRenderer{renderer: resolve.NewPlainVariableRenderer(), rootValueType: resolve.JsonRootType{Value: jsonparser.Unknown}},
		}
		variables := resolve.NewVariables(variable)
		correctedInput := correctGraphQLVariableTypes(variables, input)
		result, _, _, err := jsonparser.Get([]byte(correctedInput), "body", "variables")
		assert.NoError(t, err)
		assert.Equal(t, "{\"countryCode\":\"$$0$$\"}", string(result))
	})
}

// mockVariableRenderer is useful to inject resolve.JsonRootType to the renderer.
type mockVariableRenderer struct {
	renderer      resolve.VariableRenderer
	rootValueType resolve.JsonRootType
}

func (m *mockVariableRenderer) GetKind() string {
	return m.renderer.GetKind()
}

func (m *mockVariableRenderer) RenderVariable(ctx context.Context, data []byte, out io.Writer) error {
	return m.renderer.RenderVariable(ctx, data, out)
}

func (m *mockVariableRenderer) GetRootValueType() resolve.JsonRootType {
	return m.rootValueType
}

var _ resolve.VariableRenderer = (*mockVariableRenderer)(nil)
