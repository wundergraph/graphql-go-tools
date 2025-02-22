package postprocess

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestExtractDeferredFields_Process(t *testing.T) {
	tests := []struct {
		name     string
		input    *resolve.GraphQLResponse
		expected *resolve.GraphQLResponse
	}{
		{
			name: "trivial case",
			input: &resolve.GraphQLResponse{
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
										Defer: &resolve.DeferField{
											Path: []string{"query", "hero", "$0Droid"},
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
										Defer: &resolve.DeferField{
											Path: []string{"query", "hero", "$0Droid"},
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
				DeferredResponses: []*resolve.GraphQLResponse{
					{
						Data: &resolve.Object{
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
									Defer: &resolve.DeferField{
										Path: []string{"query", "hero", "$0Droid"},
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
									Defer: &resolve.DeferField{
										Path: []string{"query", "hero", "$0Droid"},
									},
								},
							},
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
							},
						},
					},
				},
			},
		},
		{
			name: "multi-level case",
			input: &resolve.GraphQLResponse{
				// { hero name ...on Droid @defer { ... } ...on Human @defer { ... } }
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
										Defer: &resolve.DeferField{
											Path: []string{"query", "hero", "$1Droid"},
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
										Defer: &resolve.DeferField{
											Path: []string{"query", "hero", "$1Droid"},
										},
									},
									{
										Name: []byte("friends"),
										Value: &resolve.Array{
											Path:     []string{"friends"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path:     []string{"name"},
															Nullable: false,
														},
														Defer: &resolve.DeferField{
															Path: []string{"query", "hero", "$1Droid", "friends"},
														},
													},
													{
														Name: []byte("friends"),
														Value: &resolve.Array{
															Path:     []string{"friends"},
															Nullable: true,
															Item: &resolve.Object{
																Nullable: true,
																Fields: []*resolve.Field{
																	{
																		Name: []byte("name"),
																		Value: &resolve.String{
																			Path:     []string{"name"},
																			Nullable: false},
																		Defer: &resolve.DeferField{
																			Path: []string{"query", "hero", "$1Droid", "friends", "$0Character", "friends"},
																		},
																	},
																},
																PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
																TypeName:      "Character",
															},
														},
														Defer: &resolve.DeferField{
															Path: []string{"query", "hero", "$1Droid", "friends", "$0Character"},
														},
														OnTypeNames: [][]byte{[]byte("Human"), []byte("Droid")},
													},
												},
												PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
												TypeName:      "Character",
											},
										},
										OnTypeNames: [][]byte{[]byte("Droid")},
										Defer: &resolve.DeferField{
											Path: []string{"query", "hero", "$1Droid"},
										},
									},
									{
										Name: []byte("height"),
										Value: &resolve.String{
											Path:     []string{"height"},
											Nullable: false,
										},
										OnTypeNames: [][]byte{[]byte("Human")},
										Defer: &resolve.DeferField{
											Path: []string{"query", "hero", "$2Human"},
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
				// { hero name }
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
				// { hero ...on Droid { primaryFunction favoriteEpisode friends { name ... @defer { friends { name } } } } }
				DeferredResponses: []*resolve.GraphQLResponse{
					{
						// { hero ...on Droid { primaryFunction favoriteEpisode friends { name } } }
						Data: &resolve.Object{
							Path:          []string{"hero"},
							Nullable:      true,
							TypeName:      "Character",
							PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}},
							Fields: []*resolve.Field{
								{
									Name:        []byte("primaryFunction"),
									OnTypeNames: [][]byte{[]byte("Droid")},
									Defer: &resolve.DeferField{
										Path: []string{"query", "hero", "$1Droid"},
									},
									Value: &resolve.String{
										Path:     []string{"primaryFunction"},
										Nullable: false,
									},
								},
								{
									Name:        []byte("favoriteEpisode"),
									OnTypeNames: [][]byte{[]byte("Droid")},
									Defer: &resolve.DeferField{
										Path: []string{"query", "hero", "$1Droid"},
									},
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
								},
								{
									Name:        []byte("friends"),
									OnTypeNames: [][]byte{[]byte("Droid")},
									Defer: &resolve.DeferField{
										Path: []string{"query", "hero", "$1Droid"},
									},
									Value: &resolve.Array{
										Path:     []string{"friends"},
										Nullable: true,
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: false,
													},
													Defer: &resolve.DeferField{
														Path: []string{"query", "hero", "$1Droid"},
													},
												},
											},
											PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
											TypeName:      "Character",
										},
									},
								},
							},
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
							},
						},
						// { hero ...on Droid { friends { friends { name } } } }
						DeferredResponses: []*resolve.GraphQLResponse{
							{
								Data: &resolve.Object{
									Fields: []*resolve.Field{
										{
											Name: []byte("friends"),
											Value: &resolve.Array{
												Path:     []string{"friends"},
												Nullable: true,
												Item: &resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("name"),
															Value: &resolve.String{
																Path:     []string{"name"},
																Nullable: false},
															Defer: &resolve.DeferField{
																Path: []string{"query", "hero", "$1Droid", "friends", "$0Character"},
															},
														},
													},
													PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
													TypeName:      "Character",
												},
											},
											Defer: &resolve.DeferField{
												Path: []string{"query", "hero", "$1Droid", "friends", "$0Character"},
											},
											OnTypeNames: [][]byte{[]byte("Human"), []byte("Droid")},
										},
									},
									// Fetches: []resolve.Fetch{
									// 	&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
									// },
								},
							},
						},
					},
					// { hero ...on Human { height } }
					{
						Data: &resolve.Object{
							Path:          []string{"hero"},
							Nullable:      true,
							TypeName:      "Character",
							PossibleTypes: map[string]struct{}{"Droid": {}, "Human": {}},
							Fields: []*resolve.Field{
								{
									Name: []byte("height"),
									Value: &resolve.String{
										Path:     []string{"height"},
										Nullable: false,
									},
									OnTypeNames: [][]byte{[]byte("Human")},
									Defer: &resolve.DeferField{
										Path: []string{"query", "hero", "$2Human"},
									},
								},
							},
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{FetchDependencies: resolve.FetchDependencies{FetchID: 1}},
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
			e.Process(tt.input)

			if !assert.Equal(t, tt.expected, tt.input) {
				formatterConfig := map[reflect.Type]interface{}{
					reflect.TypeOf([]byte{}): func(b []byte) string { return fmt.Sprintf(`"%s"`, string(b)) },
				}

				prettyCfg := &pretty.Config{
					Diffable:          true,
					IncludeUnexported: false,
					Formatter:         formatterConfig,
				}

				if diff := prettyCfg.Compare(tt.expected, tt.input); diff != "" {
					t.Errorf("Plan does not match(-want +got)\n%s", diff)
				}
			}
		})
	}
}
