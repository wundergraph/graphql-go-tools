package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestRequestScopedWideningBuildsUnionForCoKeyedSameTypeFields(t *testing.T) {
	responsePlan := planRequestScopedWideningOperation(t,
		`query RequestScopedWidening {
			currentViewer {
				id
				name
			}
			article {
				id
				title
				currentViewer {
					id
					email
				}
			}
		}`,
		requestScopedWideningViewerMetadata(),
	)

	require.Len(t, responsePlan.Response.RawFetches, 3)
	rootViewerFetch := requireSingleFetch(t, responsePlan.Response.RawFetches[0].Fetch)
	viewerEntityFetch := requireSingleFetch(t, responsePlan.Response.RawFetches[2].Fetch)

	assert.Equal(t, `{"method":"POST","url":"http://viewer.service","body":{"query":"{currentViewer {id name email}}"}}`, rootViewerFetch.Input)
	assert.Equal(t, `{"method":"POST","url":"http://viewer.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Article {__typename currentViewer {id email name}}}}","variables":{"representations":[$$0$$]}}}`, viewerEntityFetch.Input)
	require.NotNil(t, rootViewerFetch.Cache)
	require.NotNil(t, viewerEntityFetch.Cache)
	require.Len(t, rootViewerFetch.Cache.RequestScopedFields, 1)
	require.Len(t, viewerEntityFetch.Cache.RequestScopedFields, 1)
	assert.Equal(t, "currentViewer", rootViewerFetch.Cache.RequestScopedFields[0].FieldName)
	assert.Equal(t, "currentViewer", viewerEntityFetch.Cache.RequestScopedFields[0].FieldName)
	assert.Equal(t, []string{"id", "name", "email"}, requestScopedProvidesFieldNames(rootViewerFetch.Cache.RequestScopedFields[0].ProvidesData))
	assert.Equal(t, []string{"id", "email", "name"}, requestScopedProvidesFieldNames(viewerEntityFetch.Cache.RequestScopedFields[0].ProvidesData))
}

func TestRequestScopedWideningReturnsEarlyForDifferingReturnTypes(t *testing.T) {
	responsePlan := planRequestScopedDifferingTypesOperation(t,
		`query RequestScopedDifferingTypes {
			currentViewer {
				id
				name
			}
			article {
				id
				currentAccount {
					id
					org
				}
			}
		}`,
	)

	require.Len(t, responsePlan.Response.RawFetches, 3)
	rootViewerFetch := requireSingleFetch(t, responsePlan.Response.RawFetches[0].Fetch)
	accountEntityFetch := requireSingleFetch(t, responsePlan.Response.RawFetches[2].Fetch)

	assert.Equal(t, `{"method":"POST","url":"http://viewer.service","body":{"query":"{currentViewer {id name}}"}}`, rootViewerFetch.Input)
	assert.Equal(t, `{"method":"POST","url":"http://viewer.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Article {__typename currentAccount {id org}}}}","variables":{"representations":[$$0$$]}}}`, accountEntityFetch.Input)
}

func TestRequestScopedWideningUsesInterfaceObjectFallback(t *testing.T) {
	responsePlan := planRequestScopedWideningOperation(t,
		`query RequestScopedInterfaceObjectWidening {
			currentViewer {
				email
			}
			article {
				id
				title
				currentViewer {
					id
				}
			}
		}`,
		requestScopedWideningInterfaceObjectMetadata(),
	)

	require.Len(t, responsePlan.Response.RawFetches, 3)
	rootViewerFetch := requireSingleFetch(t, responsePlan.Response.RawFetches[0].Fetch)
	viewerEntityFetch := requireSingleFetch(t, responsePlan.Response.RawFetches[2].Fetch)

	assert.Equal(t, `{"method":"POST","url":"http://viewer.service","body":{"query":"{currentViewer {email id}}"}}`, rootViewerFetch.Input)
	assert.Equal(t, `{"method":"POST","url":"http://viewer.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Personalized {currentViewer {id email}}}}","variables":{"representations":[$$0$$]}}}`, viewerEntityFetch.Input)
}

func planRequestScopedWideningOperation(t *testing.T, operation string, viewerMetadata *plan.DataSourceMetadata) *plan.SynchronousResponsePlan {
	t.Helper()

	return planCachingOperation(t,
		requestScopedWideningSchema,
		operation,
		"",
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "viewer", viewerMetadata, mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://viewer.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `type Query { currentViewer: Viewer } type Viewer { id: ID! name: String email: String } type Personalized @key(fields: "id") @interfaceObject { id: ID! currentViewer: Viewer } extend type Article @key(fields: "id") { id: ID! currentViewer: Viewer }`,
					}, `type Query { currentViewer: Viewer } type Viewer { id: ID! name: String email: String } type Personalized { id: ID! currentViewer: Viewer } type Article { id: ID! currentViewer: Viewer }`),
				})),
				mustDataSourceConfiguration(t, "articles", requestScopedWideningArticleMetadata(), mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://articles.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `type Query { article: Article! } type Article @key(fields: "id") { id: ID! title: String }`,
					}, `type Query { article: Article! } type Article { id: ID! title: String }`),
				})),
			},
			DisableResolveFieldPositions:    true,
			DisableIncludeInfo:              true,
			DisableIncludeFieldDependencies: true,
		},
	)
}

func planRequestScopedDifferingTypesOperation(t *testing.T, operation string) *plan.SynchronousResponsePlan {
	t.Helper()

	return planCachingOperation(t,
		requestScopedDifferingTypesSchema,
		operation,
		"",
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "viewer", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName: "Query",
							FieldNames: []string{
								"currentViewer",
							},
						},
						{
							TypeName: "Viewer",
							FieldNames: []string{
								"id",
								"name",
							},
						},
						{
							TypeName: "Article",
							FieldNames: []string{
								"id",
								"currentAccount",
							},
						},
						{
							TypeName: "Account",
							FieldNames: []string{
								"id",
								"org",
							},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "Article",
								SelectionSet: "id",
							},
						},
						RequestScopedFields: []plan.RequestScopedField{
							{
								TypeName:  "Query",
								FieldName: "currentViewer",
								L1Key:     "viewer.current",
							},
							{
								TypeName:  "Article",
								FieldName: "currentAccount",
								L1Key:     "viewer.current",
							},
						},
					},
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://viewer.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `type Query { currentViewer: Viewer } type Viewer { id: ID! name: String } type Account { id: ID! org: String } extend type Article @key(fields: "id") { id: ID! currentAccount: Account }`,
					}, `type Query { currentViewer: Viewer } type Viewer { id: ID! name: String } type Account { id: ID! org: String } type Article { id: ID! currentAccount: Account }`),
				})),
				mustDataSourceConfiguration(t, "articles", requestScopedWideningArticleMetadata(), mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://articles.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `type Query { article: Article! } type Article @key(fields: "id") { id: ID! title: String }`,
					}, `type Query { article: Article! } type Article { id: ID! title: String }`),
				})),
			},
			DisableResolveFieldPositions:    true,
			DisableIncludeInfo:              true,
			DisableIncludeFieldDependencies: true,
		},
	)
}

func requestScopedProvidesFieldNames(obj *resolve.Object) []string {
	if obj == nil {
		return nil
	}
	out := make([]string, 0, len(obj.Fields))
	for _, field := range obj.Fields {
		out = append(out, string(field.Name))
	}
	return out
}

func requestScopedWideningViewerMetadata() *plan.DataSourceMetadata {
	return &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{
				TypeName: "Query",
				FieldNames: []string{
					"currentViewer",
				},
			},
			{
				TypeName: "Viewer",
				FieldNames: []string{
					"id",
					"name",
					"email",
				},
			},
			{
				TypeName: "Article",
				FieldNames: []string{
					"id",
					"currentViewer",
				},
			},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Article",
					SelectionSet: "id",
				},
			},
			RequestScopedFields: []plan.RequestScopedField{
				{
					TypeName:  "Query",
					FieldName: "currentViewer",
					L1Key:     "viewer.currentViewer",
				},
				{
					TypeName:  "Article",
					FieldName: "currentViewer",
					L1Key:     "viewer.currentViewer",
				},
			},
		},
	}
}

func requestScopedWideningInterfaceObjectMetadata() *plan.DataSourceMetadata {
	metadata := requestScopedWideningViewerMetadata()
	metadata.FederationMetaData.InterfaceObjects = []plan.EntityInterfaceConfiguration{
		{
			InterfaceTypeName: "Personalized",
			ConcreteTypeNames: []string{
				"Article",
			},
		},
	}
	metadata.FederationMetaData.RequestScopedFields = []plan.RequestScopedField{
		{
			TypeName:  "Query",
			FieldName: "currentViewer",
			L1Key:     "viewer.currentViewer",
		},
		{
			TypeName:  "Personalized",
			FieldName: "currentViewer",
			L1Key:     "viewer.currentViewer",
		},
	}
	return metadata
}

func requestScopedWideningArticleMetadata() *plan.DataSourceMetadata {
	return &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{
				TypeName: "Query",
				FieldNames: []string{
					"article",
				},
			},
			{
				TypeName: "Article",
				FieldNames: []string{
					"id",
					"title",
				},
			},
		},
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Article",
					SelectionSet: "id",
				},
			},
		},
	}
}

const requestScopedWideningSchema = `
	schema { query: Query }
	type Query {
		currentViewer: Viewer
		article: Article!
	}
	type Viewer {
		id: ID!
		name: String
		email: String
	}
	interface Personalized {
		currentViewer: Viewer
	}
	type Article implements Personalized {
		id: ID!
		title: String
		currentViewer: Viewer
	}
`

const requestScopedDifferingTypesSchema = `
	schema { query: Query }
	type Query {
		currentViewer: Viewer
		article: Article!
	}
	type Viewer {
		id: ID!
		name: String
	}
	type Account {
		id: ID!
		org: String
	}
	type Article {
		id: ID!
		title: String
		currentAccount: Account
	}
`
