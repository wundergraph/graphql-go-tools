package plan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/kylelemons/godebug/diff"
	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestPlanner_Plan(t *testing.T) {
	testLogic := func(t *testing.T, definition, operation, operationName string, config Configuration, report *operationreport.Report) Plan {
		t.Helper()

		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		if err != nil {
			t.Fatal(err)
		}
		norm := astnormalization.NewNormalizer(true, true)
		norm.NormalizeOperation(&op, &def, report)
		valid := astvalidation.DefaultOperationValidator()
		valid.Validate(&op, &def, report)

		p, err := NewPlanner(config)
		require.NoError(t, err)

		pp := p.Plan(&op, &def, operationName, report)

		return pp
	}

	test := func(definition, operation, operationName string, expectedPlan Plan, config Configuration) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			var report operationreport.Report
			actualPlan := testLogic(t, definition, operation, operationName, config, &report)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}

			formatterConfig := map[reflect.Type]interface{}{
				// normalize byte slices to strings
				reflect.TypeOf([]byte{}): func(b []byte) string { return fmt.Sprintf(`"%s"`, string(b)) },
				// normalize map[string]struct{} to json array of keys
				reflect.TypeOf(map[string]struct{}{}): func(m map[string]struct{}) string {
					var keys []string
					for k := range m {
						keys = append(keys, k)
					}
					slices.Sort(keys)

					keysPrinted, _ := json.Marshal(keys)
					return string(keysPrinted)
				},
			}

			prettyCfg := &pretty.Config{
				Diffable:          true,
				IncludeUnexported: false,
				Formatter:         formatterConfig,
			}

			exp := prettyCfg.Sprint(expectedPlan)
			act := prettyCfg.Sprint(actualPlan)

			if !assert.Equal(t, exp, act) {
				if diffResult := diff.Diff(exp, act); diffResult != "" {
					t.Errorf("Plan does not match(-want +got)\n%s", diffResult)
				}
			}

		}
	}

	testWithError := func(definition, operation, operationName string, config Configuration) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			var report operationreport.Report
			_ = testLogic(t, definition, operation, operationName, config, &report)
			assert.True(t, report.HasErrors())
		}
	}

	t.Run("Union response type with interface fragments", test(testDefinition, `
		query SearchResults {
			searchResults {
				... on Character {
					name
				}
				... on Vehicle {
					length
				}
			}
		}
	`, "SearchResults", &SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
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
								PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}, "Starship": {}},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
										OnTypeNames: [][]byte{[]byte("Droid")},
									},
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
										OnTypeNames: [][]byte{[]byte("Human")},
									},
									{
										Name: []byte("length"),
										Value: &resolve.Float{
											Path:     []string{"length"},
											Nullable: false,
										},
										OnTypeNames: [][]byte{[]byte("Starship")},
									},
								},
							},
						},
					},
				},
				Fetches: []resolve.Fetch{
					&resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							DataSource: &FakeDataSource{&StatefulSource{}},
						},
						DataSourceIdentifier: []byte("plan.FakeDataSource"),
					},
				},
			},
		},
	}, Configuration{
		DisableResolveFieldPositions: true,
		DisableIncludeInfo:           true,
		DataSources:                  []DataSource{testDefinitionDSConfiguration},
	}))

	t.Run("Merging duplicate fields in response should not happen", func(t *testing.T) {
		t.Run("Interface response type with type fragments and shared field", test(testDefinition, `
			query Hero {
				hero {
					name
					... on Droid {
						name
					}
					... on Human {
						name
					}
				}
			}
			`, "Hero", &SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Nullable: false,
					Fields: []*resolve.Field{
						{
							Name: []byte("hero"),
							Value: &resolve.Object{
								Path:          []string{"hero"},
								Nullable:      true,
								TypeName:      "Character",
								PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
										OnTypeNames: [][]byte{[]byte("Droid")},
									},
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
					},
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource: &FakeDataSource{&StatefulSource{}},
							},
							DataSourceIdentifier: []byte("plan.FakeDataSource"),
						},
					},
				},
			},
		}, Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{testDefinitionDSConfiguration},
		}))

		t.Run("Interface response type with type fragments", test(testDefinition, `
			query Hero {
				hero {
					... on Droid {
						name
					}
					... on Human {
						name
					}
				}
			}
			`, "Hero", &SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Nullable: false,
					Fields: []*resolve.Field{
						{
							Name: []byte("hero"),
							Value: &resolve.Object{
								Path:          []string{"hero"},
								Nullable:      true,
								TypeName:      "Character",
								PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path:     []string{"name"},
											Nullable: false,
										},
										OnTypeNames: [][]byte{[]byte("Droid")},
									},
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
					},
					Fetches: []resolve.Fetch{
						&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource: &FakeDataSource{&StatefulSource{}},
							},
							DataSourceIdentifier: []byte("plan.FakeDataSource"),
						},
					},
				},
			},
		}, Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{testDefinitionDSConfiguration},
		}))
	})

	t.Run("defer planning", func(t *testing.T) {
		t.Run("simple inline fragment", test(testDefinition, `
			query WithInlineDefer {
				hero {
					name
					... on Droid @defer {
						primaryFunction
						favoriteEpisode
					}
				}
			}
		`, "WithInlineDefer", &SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
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
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource: &FakeDataSource{&StatefulSource{}},
							},
							DataSourceIdentifier: []byte("plan.FakeDataSource"),
						},
						&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource: &FakeDataSource{&StatefulSource{}},
							},
							DataSourceIdentifier: []byte("plan.FakeDataSource"),
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
			DeferredFragments: []resolve.DeferInfo{
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
			DeferredFields: map[int]resolve.DeferInfo{
				1: {
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
				2: {
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
		}, Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{testDefinitionDSConfiguration},
		}))

		t.Run("simple inline fragment with arrays", test(testDefinition, `
			query WithInlineDeferWithArray {
				searchResults {
					... on Human {
						name
					}
					... on Droid @defer {
						name
						primaryFunction
						favoriteEpisode
					}
				}
			}
		`, "WithInlineDeferWithArray", &SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
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
						&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource: &FakeDataSource{&StatefulSource{}},
							},
							DataSourceIdentifier: []byte("plan.FakeDataSource"),
						},
						&resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource: &FakeDataSource{&StatefulSource{}},
							},
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
			DeferredFragments: []resolve.DeferInfo{
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
			DeferredFields: map[int]resolve.DeferInfo{
				1: {
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
				2: {
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
				3: {
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
		}, Configuration{
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
			DataSources:                  []DataSource{testDefinitionDSConfiguration},
		}))
	})

	t.Run("operation selection", func(t *testing.T) {
		cfg := Configuration{
			DataSources:        []DataSource{testDefinitionDSConfiguration},
			DisableIncludeInfo: true,
		}

		t.Run("should successfully plan a single named query by providing an operation name", test(testDefinition, `
				query MyHero {
					hero{
						name
					}
				}
			`, "MyHero", expectedMyHeroPlan, cfg,
		))

		t.Run("should successfully plan unnamed query with fragment", test(testDefinition, `
				fragment CharacterFields on Character {
					name
				}
				query {
					hero {
						...CharacterFields
					}
				}
			`, "", expectedMyHeroPlanWithFragment, cfg,
		))

		t.Run("should successfully plan multiple named queries by providing an operation name", test(testDefinition, `
				query MyHero {
					hero {
						name
					}
				}

				query MyDroid($id: ID!) {
					droid(id: $id){
						name
					}
				}
			`, "MyHero", expectedMyHeroPlan, cfg,
		))

		t.Run("should successfully plan a single named query without providing an operation name", test(testDefinition, `
				query MyHero {
					hero {
						name
					}
				}
			`, "", expectedMyHeroPlan, cfg,
		))

		t.Run("should successfully plan a single unnamed query without providing an operation name", test(testDefinition, `
				{
					hero {
						name
					}
				}
			`, "", expectedMyHeroPlan, cfg,
		))

		t.Run("should write into error report when no query with name was found", testWithError(testDefinition, `
				query MyHero {
					hero{
						name
					}
				}
			`, "NoHero", cfg,
		))

		t.Run("should write into error report when no operation name was provided on multiple named queries", testWithError(testDefinition, `
				query MyDroid($id: ID!) {
					droid(id: $id){
						name
					}
				}

				query MyHero {
					hero{
						name
					}
				}
			`, "", cfg,
		))
	})

	t.Run("unescape response json", func(t *testing.T) {
		schema := `
			scalar JSON

			schema {
				query: Query
			}

			type Query {
				hero: Character!
			}

			type Character {
				info: JSON!
				infos: [JSON!]!
			}
		`

		dsConfig := dsb().Schema(schema).
			RootNode("Query", "hero").
			ChildNode("Character", "info", "infos").
			DS()

		t.Run("with field configuration", func(t *testing.T) {
			t.Run("field with json type", test(
				schema, `
				{
					hero {
						info
					}
				}
			`, "",
				&SynchronousResponsePlan{
					FlushInterval: 0,
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("hero"),
									Value: &resolve.Object{
										Path:          []string{"hero"},
										TypeName:      "Character",
										PossibleTypes: map[string]struct{}{"Character": {}},
										Fields: []*resolve.Field{
											{
												Name: []byte("info"),
												Value: &resolve.String{
													Path:                 []string{"info"},
													UnescapeResponseJson: true,
												},
											},
										},
									},
								},
							},
							Fetches: []resolve.Fetch{&resolve.SingleFetch{
								FetchConfiguration: resolve.FetchConfiguration{
									DataSource: &FakeDataSource{&StatefulSource{}},
								},
								DataSourceIdentifier: []byte("plan.FakeDataSource"),
							}},
						},
					},
				},
				Configuration{
					DisableResolveFieldPositions: true,
					DisableIncludeInfo:           true,
					Fields: FieldConfigurations{
						FieldConfiguration{
							TypeName:             "Character",
							FieldName:            "info",
							UnescapeResponseJson: true,
						},
					},
					DataSources: []DataSource{dsConfig},
				},
			))
			t.Run("list with json type", test(
				schema, `
				{
					hero {
						infos
					}
				}
			`, "",
				&SynchronousResponsePlan{
					FlushInterval: 0,
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("hero"),
									Value: &resolve.Object{
										Path:          []string{"hero"},
										TypeName:      "Character",
										PossibleTypes: map[string]struct{}{"Character": {}},
										Fields: []*resolve.Field{
											{
												Name: []byte("infos"),
												Value: &resolve.Array{
													Path: []string{"infos"},
													Item: &resolve.String{
														UnescapeResponseJson: true,
													},
												},
											},
										},
									},
								},
							},
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										DataSource: &FakeDataSource{&StatefulSource{}},
									},
									DataSourceIdentifier: []byte("plan.FakeDataSource"),
								},
							},
						},
					},
				},
				Configuration{
					DisableResolveFieldPositions: true,
					DisableIncludeInfo:           true,
					Fields: FieldConfigurations{
						FieldConfiguration{
							TypeName:             "Character",
							FieldName:            "infos",
							UnescapeResponseJson: true,
						},
					},
					DataSources: []DataSource{dsConfig},
				},
			))
		})

		t.Run("without field configuration", func(t *testing.T) {
			t.Run("field with json type", test(
				schema, `
				{
					hero {
						info
					}
				}
			`, "",
				&SynchronousResponsePlan{
					FlushInterval: 0,
					Response: &resolve.GraphQLResponse{
						Data: &resolve.Object{
							Fields: []*resolve.Field{
								{
									Name: []byte("hero"),
									Value: &resolve.Object{
										Path:          []string{"hero"},
										TypeName:      "Character",
										PossibleTypes: map[string]struct{}{"Character": {}},
										Fields: []*resolve.Field{
											{
												Name: []byte("info"),
												Value: &resolve.Scalar{
													Path: []string{"info"},
												},
											},
										},
									},
								},
							},
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{
									FetchConfiguration: resolve.FetchConfiguration{
										DataSource: &FakeDataSource{&StatefulSource{}},
									},
									DataSourceIdentifier: []byte("plan.FakeDataSource"),
								},
							},
						},
					},
				},
				Configuration{
					DisableResolveFieldPositions: true,
					DisableIncludeInfo:           true,
					DataSources:                  []DataSource{dsConfig},
				},
			))
		})
	})

	t.Run("two different queries in different executions should not affect each other", func(t *testing.T) {
		definition := `
			type Account {
				id: ID!
				name: String
			}
			type Query {
				account: Account
			}
		`
		var accountDS = dsb().
			WithBehavior(DataSourcePlanningBehavior{
				MergeAliasedRootNodes: true,
			}).
			Schema(`type Account {
				id: ID!
			}
			type Query {
				account: Account
			}`).
			Id("accountDS").
			Hash(1).
			RootNode("Query", "account").
			RootNode("Account", "id").
			KeysMetadata(FederationFieldConfigurations{
				{
					TypeName:     "Account",
					SelectionSet: "id",
				},
			}).
			DS()
		var addressDS = dsb().
			WithBehavior(DataSourcePlanningBehavior{
				MergeAliasedRootNodes: true,
			}).
			Schema(`type Account {
				id: ID!
				name: String
			}`).
			KeysMetadata(FederationFieldConfigurations{
				{
					TypeName:     "Account",
					SelectionSet: "id",
				},
			}).
			Id("addressDS").
			Hash(2).
			RootNode("Account", "id", "name").
			DS()
		planConfiguration := Configuration{
			DataSources:                  []DataSource{accountDS, addressDS},
			DisableResolveFieldPositions: true,
			DisableIncludeInfo:           true,
		}
		report := &operationreport.Report{}
		def := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definition)

		operation1 := `
			query MyHero {
				account {
					name
				}
			}`
		operation2 := `
			query MyHero {
				account {
					name
					id
				}
			}`
		op2Expected := unsafeparser.ParseGraphqlDocumentString(operation2)
		planner1, err := NewPlanner(planConfiguration)
		require.NoError(t, err)
		plan2Expected := planner1.Plan(&op2Expected, &def, "", report)
		require.False(t, report.HasErrors())

		sharedPlanner, err := NewPlanner(planConfiguration)
		require.NoError(t, err)

		op1 := unsafeparser.ParseGraphqlDocumentString(operation1)
		_ = sharedPlanner.Plan(&op1, &def, "", report)
		require.False(t, report.HasErrors())

		op2 := unsafeparser.ParseGraphqlDocumentString(operation2)
		plan2 := sharedPlanner.Plan(&op2, &def, "", report)
		require.False(t, report.HasErrors())

		assert.Equal(t, plan2Expected, plan2)
	})
}

var expectedMyHeroPlan = &SynchronousResponsePlan{
	FlushInterval: 0,
	Response: &resolve.GraphQLResponse{
		Data: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("hero"),
					Position: resolve.Position{
						Line:   3,
						Column: 6,
					},
					Value: &resolve.Object{
						Path:          []string{"hero"},
						Nullable:      true,
						TypeName:      "Character",
						PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
						Fields: []*resolve.Field{
							{
								Name: []byte("name"),
								Value: &resolve.String{
									Path: []string{"name"},
								},
								Position: resolve.Position{
									Line:   4,
									Column: 7,
								},
							},
						},
					},
				},
			},
			Fetches: []resolve.Fetch{
				&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &FakeDataSource{&StatefulSource{}},
					},
					DataSourceIdentifier: []byte("plan.FakeDataSource"),
				},
			},
		},
	},
}

var expectedMyHeroPlanWithFragment = &SynchronousResponsePlan{
	FlushInterval: 0,
	Response: &resolve.GraphQLResponse{
		Data: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("hero"),
					Position: resolve.Position{
						Line:   6,
						Column: 6,
					},
					Value: &resolve.Object{
						Path:          []string{"hero"},
						Nullable:      true,
						TypeName:      "Character",
						PossibleTypes: map[string]struct{}{"Human": {}, "Droid": {}},
						Fields: []*resolve.Field{
							{
								Name: []byte("name"),
								Value: &resolve.String{
									Path: []string{"name"},
								},
								// During fragment inlining we are creating a new selections, so they will not have positions
								Position: resolve.Position{
									Line:   0,
									Column: 0,
								},
							},
						},
					},
				},
			},
			Fetches: []resolve.Fetch{
				&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &FakeDataSource{&StatefulSource{}},
					},
					DataSourceIdentifier: []byte("plan.FakeDataSource"),
				},
			},
		},
	},
}

var testDefinitionDSConfiguration = dsb().
	Schema(testDefinition).
	RootNode("Query", "hero", "droid", "search", "searchResults").
	RootNode("Mutation", "createReview").
	ChildNode("Review", "id", "stars", "commentary").
	ChildNode("Character", "name", "friends").
	ChildNode("Human", "name", "friends", "height").
	ChildNode("Droid", "name", "friends", "primaryFunction", "favoriteEpisode").
	ChildNode("Vehicle", "length", "width").
	ChildNode("Starship", "name", "length", "width").
	DS()

const testDefinition = `

directive @flushInterval(milliSeconds: Int!) on QUERY | SUBSCRIPTION

union SearchResult = Human | Droid | Starship

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
    search(name: String!): SearchResult
	searchResults: [SearchResult]
}

type Mutation {
    createReview(episode: Episode!, review: ReviewInput!): Review
}

type Subscription {
    remainingJedis: Int!
	newReviews: Review
}

input ReviewInput {
    stars: Int!
    commentary: String
}

type Review {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character {
    name: String!
    friends: [Character]
}

type Human implements Character {
    name: String!
    height: String!
    friends: [Character]
}

type Droid implements Character {
    name: String!
    primaryFunction: String!
    friends: [Character]
	favoriteEpisode: Episode
}

interface Vehicle {
	width: Float!
	length: Float!
}

type Starship implements Vehicle {
    name: String!
	width: Float!
    length: Float!
}
`

type StatefulSource struct {
}

func (s *StatefulSource) Start() {

}

type FakeFactory[T any] struct {
	upstreamSchema *ast.Document
	behavior       *DataSourcePlanningBehavior
}

func (f *FakeFactory[T]) UpstreamSchema(dataSourceConfig DataSourceConfiguration[T]) (*ast.Document, bool) {
	return f.upstreamSchema, true
}

func (f *FakeFactory[T]) Planner(logger abstractlogger.Logger) DataSourcePlanner[T] {
	source := &StatefulSource{}
	go source.Start()
	return &FakePlanner[T]{
		source:         source,
		upstreamSchema: f.upstreamSchema,
		behavior:       f.behavior,
	}
}

func (f *FakeFactory[T]) Context() context.Context {
	return context.TODO()
}

type FakePlanner[T any] struct {
	id             int
	source         *StatefulSource
	upstreamSchema *ast.Document
	behavior       *DataSourcePlanningBehavior
}

func (f *FakePlanner[T]) ID() int {
	return f.id
}

func (f *FakePlanner[T]) SetID(id int) {
	f.id = id
}

func (f *FakePlanner[T]) EnterDocument(operation, definition *ast.Document) {

}

func (f *FakePlanner[T]) Register(visitor *Visitor, _ DataSourceConfiguration[T], _ DataSourcePlannerConfiguration) error {
	visitor.Walker.RegisterEnterDocumentVisitor(f)
	return nil
}

func (f *FakePlanner[T]) ConfigureFetch() resolve.FetchConfiguration {
	return resolve.FetchConfiguration{
		DataSource: &FakeDataSource{
			source: f.source,
		},
	}
}

func (f *FakePlanner[T]) ConfigureSubscription() SubscriptionConfiguration {
	return SubscriptionConfiguration{}
}

func (f *FakePlanner[T]) DataSourcePlanningBehavior() DataSourcePlanningBehavior {
	if f.behavior == nil {
		return DataSourcePlanningBehavior{
			MergeAliasedRootNodes:      false,
			OverrideFieldPathFromAlias: false,
		}
	}

	return *f.behavior
}

func (f *FakePlanner[T]) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	return
}

type FakeDataSource struct {
	source *StatefulSource
}

func (f *FakeDataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error) {
	return
}

func (f *FakeDataSource) LoadWithFiles(ctx context.Context, input []byte, files []*httpclient.FileUpload, out *bytes.Buffer) (err error) {
	return
}
