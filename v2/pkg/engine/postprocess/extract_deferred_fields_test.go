package postprocess

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestExtractDeferredFields_Process(t *testing.T) {
	tests := []struct {
		name     string
		input    *resolve.GraphQLResponse
		defers   []resolve.DeferInfo
		expected *resolve.GraphQLResponse
	}{
		{
			name: "trivial case",
			input: &resolve.GraphQLResponse{
				Info: &resolve.GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Data: &resolve.Object{
					Nullable: false,
					Fields: []*resolve.Field{
						{
							Name: []byte("hero"),
							Value: &resolve.Object{
								Path:          []string{"hero"},
								Nullable:      true,
								TypeName:      "Character",
								PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
					},
				},
			},
			expected: &resolve.GraphQLResponse{
				Info: &resolve.GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Data: &resolve.Object{
					Nullable: false,
					Fields: []*resolve.Field{
						{
							Name: []byte("hero"),
							Value: &resolve.Object{
								Path:          []string{"hero"},
								Nullable:      true,
								TypeName:      "Character",
								PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
					},
				},
			},
		},
		{
			name: "simple case",
			input: &resolve.GraphQLResponse{
				Info: &resolve.GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Data: &resolve.Object{
					Nullable: false,
					Fields: []*resolve.Field{
						{
							Name: []byte("hero"),
							Value: &resolve.Object{
								Path:          []string{"hero"},
								Nullable:      true,
								TypeName:      "Character",
								PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
									{
										Name: []byte("primaryFunction"),
										Value: &resolve.String{
											Path:     []string{"primaryFunction"},
											Nullable: false,
										},
										OnTypeNames: [][]byte{[]byte("Droid")},
										DeferPaths: []ast.Path{
											{
												ast.PathItem{
													Kind:      ast.FieldName,
													FieldName: []byte("query"),
												},
												ast.PathItem{
													Kind:      ast.FieldName,
													FieldName: []byte("hero"),
												},
												ast.PathItem{
													Kind:      ast.InlineFragmentName,
													FieldName: []byte("Droid"),
												},
											},
										},
									},
									{
										Name: []byte("favoriteEpisode"),
										Value: &resolve.Enum{
											Path:     []string{"favoriteEpisode"},
											Nullable: true,
											TypeName: "Episode",
											Values: []string{
												"NEWHOPE",
												"EMPIRE",
												"JEDI",
											},
										},
										OnTypeNames: [][]byte{[]byte("Droid")},
										DeferPaths: []ast.Path{
											{
												ast.PathItem{
													Kind:      ast.FieldName,
													FieldName: []byte("query"),
												},
												ast.PathItem{
													Kind:      ast.FieldName,
													FieldName: []byte("hero"),
												},
												ast.PathItem{
													Kind:      ast.InlineFragmentName,
													FieldName: []byte("Droid"),
												},
											},
										},
									},
								},
							},
							DeferPaths: []ast.Path{
								{
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("query"),
									},
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("hero"),
									},
									ast.PathItem{
										Kind:      ast.InlineFragmentName,
										FieldName: []byte("Droid"),
									},
								},
							},
						},
					},
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
						&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{FetchID: 1},
							DeferInfo: &resolve.DeferInfo{
								Path: ast.Path{
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("query"),
									},
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("hero"),
									},
									ast.PathItem{
										Kind:      ast.InlineFragmentName,
										FieldName: []byte("Droid"),
									},
								},
							},
						},
					},
				},
			},
			defers: []resolve.DeferInfo{
				{
					Path: ast.Path{
						ast.PathItem{
							Kind:      ast.FieldName,
							FieldName: []byte("query"),
						},
						ast.PathItem{
							Kind:      ast.FieldName,
							FieldName: []byte("hero"),
						},
						ast.PathItem{
							Kind:      ast.InlineFragmentName,
							FieldName: []byte("Droid"),
						},
					},
				},
			},
			expected: &resolve.GraphQLResponse{
				Info: &resolve.GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Data: &resolve.Object{
					Nullable: false,
					Fields: []*resolve.Field{
						{
							Name: []byte("hero"),
							Value: &resolve.Object{
								Path:          []string{"hero"},
								Nullable:      true,
								TypeName:      "Character",
								PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
								},
							},
							DeferPaths: []ast.Path{
								{
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("query"),
									},
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("hero"),
									},
									ast.PathItem{
										Kind:      ast.InlineFragmentName,
										FieldName: []byte("Droid"),
									},
								},
							},
						},
					},
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
					},
				},
				DeferredResponses: []*resolve.GraphQLResponse{
					{
						Info: &resolve.GraphQLResponseInfo{
							OperationType: ast.OperationTypeQuery,
						},
						Data: &resolve.Object{
							Nullable: false,
							Fields: []*resolve.Field{
								{
									Name: []byte("hero"),
									Value: &resolve.Object{
										Path:          []string{"hero"},
										Nullable:      true,
										TypeName:      "Character",
										PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}},
										Fields: []*resolve.Field{
											{
												Name: []byte("primaryFunction"),
												Value: &resolve.String{
													Path:     []string{"primaryFunction"},
													Nullable: false,
												},
												OnTypeNames: [][]byte{[]byte("Droid")},
												DeferPaths: []ast.Path{
													{
														ast.PathItem{
															Kind:      ast.FieldName,
															FieldName: []byte("query"),
														},
														ast.PathItem{
															Kind:      ast.FieldName,
															FieldName: []byte("hero"),
														},
														ast.PathItem{
															Kind:      ast.InlineFragmentName,
															FieldName: []byte("Droid"),
														},
													},
												},
											},
											{
												Name: []byte("favoriteEpisode"),
												Value: &resolve.Enum{
													Path:     []string{"favoriteEpisode"},
													Nullable: true,
													TypeName: "Episode",
													Values: []string{
														"NEWHOPE",
														"EMPIRE",
														"JEDI",
													},
												},
												OnTypeNames: [][]byte{[]byte("Droid")},
												DeferPaths: []ast.Path{
													{
														ast.PathItem{
															Kind:      ast.FieldName,
															FieldName: []byte("query"),
														},
														ast.PathItem{
															Kind:      ast.FieldName,
															FieldName: []byte("hero"),
														},
														ast.PathItem{
															Kind:      ast.InlineFragmentName,
															FieldName: []byte("Droid"),
														},
													},
												},
											},
										},
									},
									DeferPaths: []ast.Path{
										{
											ast.PathItem{
												Kind:      ast.FieldName,
												FieldName: []byte("query"),
											},
											ast.PathItem{
												Kind:      ast.FieldName,
												FieldName: []byte("hero"),
											},
											ast.PathItem{
												Kind:      ast.InlineFragmentName,
												FieldName: []byte("Droid"),
											},
										},
									},
								},
							},
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{FetchID: 1},
									DeferInfo: &resolve.DeferInfo{
										Path: ast.Path{
											ast.PathItem{
												Kind:      ast.FieldName,
												FieldName: []byte("query"),
											},
											ast.PathItem{
												Kind:      ast.FieldName,
												FieldName: []byte("hero"),
											},
											ast.PathItem{
												Kind:      ast.InlineFragmentName,
												FieldName: []byte("Droid"),
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
		{
			name: "simple case with arrays",
			input: &resolve.GraphQLResponse{
				Info: &resolve.GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Data: &resolve.Object{
					Nullable: false,
					Fields: []*resolve.Field{
						{
							Name: []byte("searchResults"),
							Value: &resolve.Array{
								Path:     []string{"searchResults"},
								Nullable: true,
								Item: &resolve.Object{
									TypeName:      "SearchResult",
									Nullable:      true,
									PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}, "Starship": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: false,
											},
											OnTypeNames: [][]byte{[]byte("Human")},
										},
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: false,
											},
											OnTypeNames: [][]byte{[]byte("Droid")},
											DeferPaths: []ast.Path{
												{
													ast.PathItem{
														Kind:      ast.FieldName,
														FieldName: []byte("query"),
													},
													ast.PathItem{
														Kind:      ast.FieldName,
														FieldName: []byte("searchResults"),
													},
													ast.PathItem{
														Kind:        ast.InlineFragmentName,
														FieldName:   []byte("Droid"),
														FragmentRef: 1,
													},
												},
											},
										},
										{
											Name: []byte("primaryFunction"),
											Value: &resolve.String{
												Path:     []string{"primaryFunction"},
												Nullable: false,
											},
											OnTypeNames: [][]byte{[]byte("Droid")},
											DeferPaths: []ast.Path{
												{
													ast.PathItem{
														Kind:      ast.FieldName,
														FieldName: []byte("query"),
													},
													ast.PathItem{
														Kind:      ast.FieldName,
														FieldName: []byte("searchResults"),
													},
													ast.PathItem{
														Kind:        ast.InlineFragmentName,
														FieldName:   []byte("Droid"),
														FragmentRef: 1,
													},
												},
											},
										},
										{
											Name: []byte("favoriteEpisode"),
											Value: &resolve.Enum{
												Path:     []string{"favoriteEpisode"},
												Nullable: true,
												TypeName: "Episode",
												Values: []string{
													"NEWHOPE",
													"EMPIRE",
													"JEDI",
												},
												InaccessibleValues: []string{},
											},
											OnTypeNames: [][]byte{[]byte("Droid")},
											DeferPaths: []ast.Path{
												{
													ast.PathItem{
														Kind:      ast.FieldName,
														FieldName: []byte("query"),
													},
													ast.PathItem{
														Kind:      ast.FieldName,
														FieldName: []byte("searchResults"),
													},
													ast.PathItem{
														Kind:        ast.InlineFragmentName,
														FieldName:   []byte("Droid"),
														FragmentRef: 1,
													},
												},
											},
										},
									},
								},
							},
							DeferPaths: []ast.Path{
								{
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("query"),
									},
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("searchResults"),
									},
									ast.PathItem{
										Kind:        ast.InlineFragmentName,
										FieldName:   []byte("Droid"),
										FragmentRef: 1,
									},
								},
							},
						},
					},
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{DataSourceIdentifier: []byte("plan.FakeDataSource")},
						&resolve.SingleFetch{
							DataSourceIdentifier: []byte("plan.FakeDataSource"),
							DeferInfo: &resolve.DeferInfo{
								Path: ast.Path{
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("query"),
									},
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("searchResults"),
									},
									ast.PathItem{
										Kind:        ast.InlineFragmentName,
										FieldName:   []byte("Droid"),
										FragmentRef: 1,
									},
								},
							},
						},
					},
				},
			},
			defers: []resolve.DeferInfo{
				{
					Path: ast.Path{
						ast.PathItem{
							Kind:      ast.FieldName,
							FieldName: []byte("query"),
						},
						ast.PathItem{
							Kind:      ast.FieldName,
							FieldName: []byte("searchResults"),
						},
						ast.PathItem{
							Kind:        ast.InlineFragmentName,
							FieldName:   []byte("Droid"),
							FragmentRef: 1,
						},
					},
				},
			},
			expected: &resolve.GraphQLResponse{
				Info: &resolve.GraphQLResponseInfo{
					OperationType: ast.OperationTypeQuery,
				},
				Data: &resolve.Object{
					Nullable: false,
					Fields: []*resolve.Field{
						{
							Name: []byte("searchResults"),
							Value: &resolve.Array{
								Path:     []string{"searchResults"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable:      true,
									TypeName:      "SearchResult",
									PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}, "Starship": {}},
									Fields: []*resolve.Field{
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: false,
											},
											OnTypeNames: [][]byte{[]byte("Human")},
										},
									},
								},
							},
							DeferPaths: []ast.Path{
								{
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("query"),
									},
									ast.PathItem{
										Kind:      ast.FieldName,
										FieldName: []byte("searchResults"),
									},
									ast.PathItem{
										Kind:        ast.InlineFragmentName,
										FieldName:   []byte("Droid"),
										FragmentRef: 1,
									},
								},
							},
						},
					},
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{DataSourceIdentifier: []byte("plan.FakeDataSource")},
					},
				},
				DeferredResponses: []*resolve.GraphQLResponse{
					{
						Info: &resolve.GraphQLResponseInfo{
							OperationType: ast.OperationTypeQuery,
						},
						Data: &resolve.Object{
							Nullable: false,
							Fields: []*resolve.Field{
								{
									Name: []byte("searchResults"),
									Value: &resolve.Array{
										Nullable: true,
										Path:     []string{"searchResults"},
										Item: &resolve.Object{
											Nullable:      true,
											TypeName:      "SearchResult",
											PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}, "Starship": {}},
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: false,
													},
													OnTypeNames: [][]byte{[]byte("Droid")},
													DeferPaths: []ast.Path{
														{
															ast.PathItem{
																Kind:      ast.FieldName,
																FieldName: []byte("query"),
															},
															ast.PathItem{
																Kind:      ast.FieldName,
																FieldName: []byte("searchResults"),
															},
															ast.PathItem{
																Kind:        ast.InlineFragmentName,
																FieldName:   []byte("Droid"),
																FragmentRef: 1,
															},
														},
													},
												},
												{
													Name: []byte("primaryFunction"),
													Value: &resolve.String{
														Path:     []string{"primaryFunction"},
														Nullable: false,
													},
													OnTypeNames: [][]byte{[]byte("Droid")},
													DeferPaths: []ast.Path{
														{
															ast.PathItem{
																Kind:      ast.FieldName,
																FieldName: []byte("query"),
															},
															ast.PathItem{
																Kind:      ast.FieldName,
																FieldName: []byte("searchResults"),
															},
															ast.PathItem{
																Kind:        ast.InlineFragmentName,
																FieldName:   []byte("Droid"),
																FragmentRef: 1,
															},
														},
													},
												},
												{
													Name: []byte("favoriteEpisode"),
													Value: &resolve.Enum{
														Path:     []string{"favoriteEpisode"},
														Nullable: true,
														TypeName: "Episode",
														Values: []string{
															"NEWHOPE",
															"EMPIRE",
															"JEDI",
														},
														InaccessibleValues: []string{},
													},
													OnTypeNames: [][]byte{[]byte("Droid")},
													DeferPaths: []ast.Path{
														{
															ast.PathItem{
																Kind:      ast.FieldName,
																FieldName: []byte("query"),
															},
															ast.PathItem{
																Kind:      ast.FieldName,
																FieldName: []byte("searchResults"),
															},
															ast.PathItem{
																Kind:        ast.InlineFragmentName,
																FieldName:   []byte("Droid"),
																FragmentRef: 1,
															},
														},
													},
												},
											},
										},
									},
									DeferPaths: []ast.Path{
										{
											ast.PathItem{
												Kind:      ast.FieldName,
												FieldName: []byte("query"),
											},
											ast.PathItem{
												Kind:      ast.FieldName,
												FieldName: []byte("searchResults"),
											},
											ast.PathItem{
												Kind:        ast.InlineFragmentName,
												FieldName:   []byte("Droid"),
												FragmentRef: 1,
											},
										},
									},
								},
							},
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{
									DataSourceIdentifier: []byte("plan.FakeDataSource"),
									DeferInfo: &resolve.DeferInfo{
										Path: ast.Path{
											ast.PathItem{
												Kind:      ast.FieldName,
												FieldName: []byte("query"),
											},
											ast.PathItem{
												Kind:      ast.FieldName,
												FieldName: []byte("searchResults"),
											},
											ast.PathItem{
												Kind:        ast.InlineFragmentName,
												FieldName:   []byte("Droid"),
												FragmentRef: 1,
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &extractDeferredFields{}
			e.Process(tt.input, tt.defers)

			if !assert.Equal(t, tt.expected, tt.input) {
				formatterConfig := map[reflect.Type]interface{}{
					reflect.TypeOf([]byte{}): func(b []byte) string { return fmt.Sprintf(`"%s"`, string(b)) },
				}

				prettyCfg := &pretty.Config{
					Diffable:          true,
					IncludeUnexported: true,
					Formatter:         formatterConfig,
				}

				if diff := prettyCfg.Compare(tt.expected, tt.input); diff != "" {
					t.Errorf("Plan does not match(-want +got)\n%s", diff)
				}
			}
		})
	}
}
