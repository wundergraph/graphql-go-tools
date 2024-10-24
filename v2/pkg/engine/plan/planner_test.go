package plan

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jensneuse/abstractlogger"
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
			plan := testLogic(t, definition, operation, operationName, config, &report)
			if report.HasErrors() {
				t.Fatal(report.Error())
			}
			assert.Equal(t, expectedPlan, plan)

			toJson := func(v interface{}) string {
				b := &strings.Builder{}
				e := json.NewEncoder(b)
				e.SetIndent("", " ")
				_ = e.Encode(v)
				return b.String()
			}

			assert.Equal(t, toJson(expectedPlan), toJson(plan))

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
										OnTypeNames: [][]byte{[]byte("Human"), []byte("Droid")},
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

directive @defer on FIELD

directive @flushInterval(milliSeconds: Int!) on QUERY | SUBSCRIPTION

directive @stream(initialBatchSize: Int) on FIELD

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
	}
}

func (f *FakeFactory[T]) Context() context.Context {
	return context.TODO()
}

type FakePlanner[T any] struct {
	id             int
	source         *StatefulSource
	upstreamSchema *ast.Document
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
	return DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
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

func (f *FakeDataSource) LoadWithFiles(ctx context.Context, input []byte, files []httpclient.File, out *bytes.Buffer) (err error) {
	return
}
