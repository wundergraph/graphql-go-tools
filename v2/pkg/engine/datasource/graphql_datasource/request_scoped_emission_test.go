package graphql_datasource

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestRequestScopedRootFetchEmitsRootFields(t *testing.T) {
	responsePlan := planCachingOperation(t,
		requestScopedRootSchema,
		`query RequestScopedRoot { currentViewer { id } viewerSession { token } }`,
		"RequestScopedRoot",
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "accounts", requestScopedRootMetadata(), mustCustomConfiguration(t, ConfigurationInput{
					Fetch:               &FetchConfiguration{URL: "http://accounts.service"},
					SchemaConfiguration: mustSchema(t, nil, requestScopedRootSchema),
				})),
			},
			DisableResolveFieldPositions:    true,
			DisableIncludeInfo:              true,
			DisableIncludeFieldDependencies: true,
		},
	)

	fetch := requireSingleFetch(t, responsePlan.Response.RawFetches[0].Fetch)
	require.NotNil(t, fetch.Cache)
	assert.Equal(t, []resolve.RequestScopedField{
		{
			FieldName: "currentViewer",
			FieldPath: []string{
				"currentViewer",
			},
			L1Key: "accounts.viewer",
		},
		{
			FieldName: "viewerSession",
			FieldPath: []string{
				"viewerSession",
			},
			L1Key: "accounts.session",
		},
	}, requestScopedFieldIdentities(fetch.Cache.RequestScopedFields))
}

func TestRequestScopedEntityFetchEmitsEntityFields(t *testing.T) {
	responsePlan := planRequestScopedEntityOperation(t,
		`query RequestScopedEntity { user(id: "1") { id displayName locale } }`,
		"RequestScopedEntity",
		[]plan.RequestScopedField{
			{
				TypeName:  "User",
				FieldName: "displayName",
				L1Key:     "profiles.viewer",
			},
			{
				TypeName:  "User",
				FieldName: "locale",
				L1Key:     "profiles.locale",
			},
		},
		nil,
	)

	fetch := requireSingleFetch(t, responsePlan.Response.RawFetches[1].Fetch)
	require.NotNil(t, fetch.Cache)
	assert.Equal(t, []resolve.RequestScopedField{}, fetch.Cache.RequestScopedFields)
}

func TestRequestScopedInterfaceObjectDedup(t *testing.T) {
	responsePlan := planRequestScopedArticleOperation(t,
		`query RequestScopedInterfaceObject { article(id: "a1") { id profile } }`,
		"RequestScopedInterfaceObject",
		plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Personalized",
					SelectionSet: "id",
				},
				{
					TypeName:     "Article",
					SelectionSet: "id",
				},
			},
			InterfaceObjects: []plan.EntityInterfaceConfiguration{
				{
					InterfaceTypeName: "Personalized",
					ConcreteTypeNames: []string{
						"Article",
					},
				},
			},
			RequestScopedFields: []plan.RequestScopedField{
				{
					TypeName:  "Article",
					FieldName: "profile",
					L1Key:     "personalization.profile",
				},
				{
					TypeName:  "Personalized",
					FieldName: "profile",
					L1Key:     "personalization.profile",
				},
			},
		},
	)

	fetch := requireSingleFetch(t, responsePlan.Response.RawFetches[1].Fetch)
	require.NotNil(t, fetch.Cache)
	assert.Equal(t, []resolve.RequestScopedField{}, fetch.Cache.RequestScopedFields)
}

func TestRequestScopedResponseKeyMapping(t *testing.T) {
	responsePlan := planRequestScopedEntityOperation(t,
		`query RequestScopedAlias { user(id: "1") { id viewerName: displayName } }`,
		"RequestScopedAlias",
		[]plan.RequestScopedField{
			{
				TypeName:  "User",
				FieldName: "displayName",
				L1Key:     "profiles.viewer",
			},
		},
		nil,
	)

	fetch := requireSingleFetch(t, responsePlan.Response.RawFetches[1].Fetch)
	require.NotNil(t, fetch.Cache)
	assert.Equal(t, []resolve.RequestScopedField{}, fetch.Cache.RequestScopedFields)
}

func TestRequestScopedSymmetricEmission(t *testing.T) {
	responsePlan := planRequestScopedEntityOperation(t,
		`query RequestScopedSymmetric { user(id: "1") { id displayName username } }`,
		"RequestScopedSymmetric",
		[]plan.RequestScopedField{
			{
				TypeName:  "User",
				FieldName: "displayName",
				L1Key:     "profiles.viewer",
			},
			{
				TypeName:  "User",
				FieldName: "username",
				L1Key:     "profiles.viewer",
			},
		},
		nil,
	)

	fetch := requireSingleFetch(t, responsePlan.Response.RawFetches[1].Fetch)
	require.NotNil(t, fetch.Cache)
	assert.Equal(t, []resolve.RequestScopedField{}, fetch.Cache.RequestScopedFields)
}

func TestRequestScopedSubgraphWithoutRequestScopedFieldsEmitsEmptySlice(t *testing.T) {
	cacheTTL := time.Minute
	responsePlan := planCachingOperation(t,
		requestScopedRootSchema,
		`query NoRequestScopedFields { currentViewer { id } }`,
		"NoRequestScopedFields",
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "accounts", &plan.DataSourceMetadata{
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
							},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						RootFieldCacheConfig: plan.RootFieldCacheConfigurations{
							{
								TypeName:  "Query",
								FieldName: "currentViewer",
								CacheName: "accounts",
								TTL:       cacheTTL,
							},
						},
					},
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch:               &FetchConfiguration{URL: "http://accounts.service"},
					SchemaConfiguration: mustSchema(t, nil, requestScopedRootSchema),
				})),
			},
			DisableResolveFieldPositions:    true,
			DisableIncludeInfo:              true,
			DisableIncludeFieldDependencies: true,
		},
	)

	fetch := requireSingleFetch(t, responsePlan.Response.RawFetches[0].Fetch)
	require.NotNil(t, fetch.Cache)
	assert.Equal(t, []resolve.RequestScopedField(nil), fetch.Cache.RequestScopedFields)
}

func requestScopedFieldIdentities(fields []resolve.RequestScopedField) []resolve.RequestScopedField {
	out := make([]resolve.RequestScopedField, 0, len(fields))
	for _, field := range fields {
		out = append(out, resolve.RequestScopedField{
			FieldName: field.FieldName,
			FieldPath: append([]string(nil), field.FieldPath...),
			L1Key:     field.L1Key,
		})
	}
	return out
}

func planRequestScopedEntityOperation(t *testing.T, operation, operationName string, fields []plan.RequestScopedField, interfaceObjects []plan.EntityInterfaceConfiguration) *plan.SynchronousResponsePlan {
	t.Helper()

	return planCachingOperation(t,
		requestScopedEntitySchema,
		operation,
		operationName,
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "accounts", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName: "Query",
							FieldNames: []string{
								"user",
							},
						},
						{
							TypeName: "User",
							FieldNames: []string{
								"id",
							},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://accounts.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `type Query { user(id: ID): User } type User @key(fields: "id") { id: ID! }`,
					}, `type Query { user(id: ID): User } type User { id: ID! }`),
				})),
				mustDataSourceConfiguration(t, "profiles", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName: "User",
							FieldNames: []string{
								"id",
								"displayName",
								"locale",
								"username",
							},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
						InterfaceObjects:    interfaceObjects,
						RequestScopedFields: fields,
					},
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://profiles.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `extend type User @key(fields: "id") { id: ID! displayName: String locale: String username: String }`,
					}, `type User { id: ID! displayName: String locale: String username: String }`),
				})),
			},
			DisableResolveFieldPositions:    true,
			DisableIncludeInfo:              true,
			DisableIncludeFieldDependencies: true,
		},
	)
}

func planRequestScopedArticleOperation(t *testing.T, operation, operationName string, personalization plan.FederationMetaData) *plan.SynchronousResponsePlan {
	t.Helper()

	return planCachingOperation(t,
		requestScopedArticleSchema,
		operation,
		operationName,
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(t, "content", &plan.DataSourceMetadata{
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
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://content.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `type Query { article(id: ID): Article } type Article @key(fields: "id") { id: ID! }`,
					}, `type Query { article(id: ID): Article } type Article { id: ID! }`),
				})),
				mustDataSourceConfiguration(t, "personalization", &plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName: "Article",
							FieldNames: []string{
								"id",
								"profile",
							},
						},
						{
							TypeName: "Personalized",
							FieldNames: []string{
								"id",
								"profile",
							},
						},
					},
					FederationMetaData: personalization,
				}, mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{URL: "http://personalization.service"},
					SchemaConfiguration: mustSchema(t, &FederationConfiguration{
						Enabled:    true,
						ServiceSDL: `type Personalized @key(fields: "id") @interfaceObject { id: ID! profile: String } extend type Article @key(fields: "id") { id: ID! profile: String }`,
					}, `type Personalized { id: ID! profile: String } type Article { id: ID! profile: String }`),
				})),
			},
			DisableResolveFieldPositions:    true,
			DisableIncludeInfo:              true,
			DisableIncludeFieldDependencies: true,
		},
	)
}

func requestScopedRootMetadata() *plan.DataSourceMetadata {
	return &plan.DataSourceMetadata{
		RootNodes: []plan.TypeField{
			{
				TypeName: "Query",
				FieldNames: []string{
					"currentViewer",
					"viewerSession",
				},
			},
			{
				TypeName: "Viewer",
				FieldNames: []string{
					"id",
				},
			},
			{
				TypeName: "Session",
				FieldNames: []string{
					"token",
				},
			},
		},
		FederationMetaData: plan.FederationMetaData{
			RequestScopedFields: []plan.RequestScopedField{
				{
					TypeName:  "Query",
					FieldName: "currentViewer",
					L1Key:     "accounts.viewer",
				},
				{
					TypeName:  "Query",
					FieldName: "viewerSession",
					L1Key:     "accounts.session",
				},
			},
		},
	}
}

const requestScopedRootSchema = `
	schema { query: Query }
	type Query {
		currentViewer: Viewer
		viewerSession: Session
	}
	type Viewer {
		id: ID!
	}
	type Session {
		token: String
	}
`

const requestScopedEntitySchema = `
	schema { query: Query }
	type Query {
		user(id: ID): User
	}
	type User {
		id: ID!
		displayName: String
		locale: String
		username: String
	}
`

const requestScopedArticleSchema = `
	schema { query: Query }
	type Query {
		article(id: ID): Article
	}
	interface Personalized {
		profile: String
	}
	type Article implements Personalized {
		id: ID!
		profile: String
	}
`
