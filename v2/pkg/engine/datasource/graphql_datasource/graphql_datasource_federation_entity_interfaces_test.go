package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederationEntityInterfaces(t *testing.T) {
	federationFactory := &Factory{}

	definition := `
		interface Account {
		  id: ID!
		  title: String!
		  locations: [Location!]
		  age: Int!
		}
		
		type Location {
		  country: String!
		}
		
		type Admin implements Account {
		  id: ID!
		  title: String!
		  locations: [Location!]
		  age: Int!
		}
		
		type Moderator implements Account {
		  id: ID!
		  title: String!
		  locations: [Location!]
		  age: Int!
		}
		
		type User implements Account {
		  id: ID!
		  title: String!
		  locations: [Location!]
		  age: Int!
		}
		
		union Accounts = Admin | Moderator | User
		
		type Query {
		  allAccountsInterface: [Account]
		  allAccountsUnion: [Accounts]
		  user(id: ID!): User
		  admin(id: ID!): Admin
		  accountLocations: [Account!]!
		}`

	firstSubgraphSDL := `	
		interface Account @key(fields: "id") {
			id: ID!
			title: String!
		}
		
		type Admin implements Account @key(fields: "id"){
			id: ID!
			title: String! @external
		}
		
		type Moderator implements Account @key(fields: "id"){
			id: ID!
			title: String!
		}
		
		type User implements Account @key(fields: "id"){
			id: ID!
			title: String!
		}
		
		union Accounts = Admin | Moderator | User
		
		type Query {
			allAccountsInterface: [Account]
			allAccountsUnion: [Accounts]
			user(id: ID!): User
			admin(id: ID!): Admin
		}`

	firstDatasourceConfiguration := plan.DataSourceConfiguration{
		RootNodes: []plan.TypeField{
			{
				TypeName:   "Admin",
				FieldNames: []string{"id"},
			},
			{
				TypeName:   "Moderator",
				FieldNames: []string{"id", "title"},
			},
			{
				TypeName:   "User",
				FieldNames: []string{"id", "title"},
			},
			{
				TypeName:   "Query",
				FieldNames: []string{"allAccountsInterface", "allAccountsUnion", "user", "admin"},
			},
		},
		ChildNodes: []plan.TypeField{
			{
				TypeName:   "Account",
				FieldNames: []string{"id", "title"},
			},
		},
		Custom: ConfigJson(Configuration{
			Fetch: FetchConfiguration{
				URL: "http://first.service",
			},
			Federation: FederationConfiguration{
				Enabled:    true,
				ServiceSDL: firstSubgraphSDL,
			},
			UpstreamSchema: firstSubgraphSDL,
		}),
		Factory: federationFactory,
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Account",
					SelectionSet: "id",
				},
				{
					TypeName:       "Admin",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
				{
					TypeName:       "Moderator",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
				{
					TypeName:       "User",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
			},
		},
	}

	secondSubgraphSDL := `
		type Account @key(fields: "id") @interfaceObject {
			id: ID!
			locations: [Location!]
		}
		
		type Location {
			country: String!
		}
		
		type Query {
			accountLocations: [Account!]!
		}`

	secondDatasourceConfiguration := plan.DataSourceConfiguration{
		RootNodes: []plan.TypeField{
			{
				TypeName:   "Account",
				FieldNames: []string{"id", "locations"},
			},
			{
				TypeName:   "User",
				FieldNames: []string{"id", "locations"},
			},
			{
				TypeName:   "Moderator",
				FieldNames: []string{"id", "locations"},
			},
			{
				TypeName:   "Admin",
				FieldNames: []string{"id", "locations"},
			},
			{
				TypeName:   "Query",
				FieldNames: []string{"accountLocations"},
			},
		},
		ChildNodes: []plan.TypeField{
			{
				TypeName:   "Location",
				FieldNames: []string{"country"},
			},
		},
		Custom: ConfigJson(Configuration{
			Fetch: FetchConfiguration{
				URL: "http://second.service",
			},
			Federation: FederationConfiguration{
				Enabled:    true,
				ServiceSDL: secondSubgraphSDL,
			},
			UpstreamSchema: secondSubgraphSDL,
		}),
		Factory: federationFactory,
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Account",
					SelectionSet: "id",
				},
				{
					TypeName:       "Admin",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
				{
					TypeName:       "Moderator",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
				{
					TypeName:       "User",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
			},
		},
		RenameTypes: plan.TypeConfigurations{
			{
				TypeName: "Admin",
				RenameTo: "Account",
			},
			{
				TypeName: "Moderator",
				RenameTo: "Account",
			},
			{
				TypeName: "User",
				RenameTo: "Account",
			},
		},
	}

	thirdSubgraphSDL := `
		type Admin @key(fields: "id"){
			id: ID!
			title: String!
		}`

	thirdDatasourceConfiguration := plan.DataSourceConfiguration{
		RootNodes: []plan.TypeField{
			{
				TypeName:   "Admin",
				FieldNames: []string{"id", "title"},
			},
		},
		Custom: ConfigJson(Configuration{
			Fetch: FetchConfiguration{
				URL: "http://third.service",
			},
			Federation: FederationConfiguration{
				Enabled:    true,
				ServiceSDL: thirdSubgraphSDL,
			},
			UpstreamSchema: thirdSubgraphSDL,
		}),
		Factory: federationFactory,
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Admin",
					SelectionSet: "id",
				},
			},
		},
	}

	fourthSubgraphSDL := `
		type Account @key(fields: "id") @interfaceObject {
			id: ID!
			age: Int!
		}`

	fourthDatasourceConfiguration := plan.DataSourceConfiguration{
		RootNodes: []plan.TypeField{
			{
				TypeName:   "Account",
				FieldNames: []string{"id", "age"},
			},
			{
				TypeName:   "User",
				FieldNames: []string{"id", "age"},
			},
			{
				TypeName:   "Moderator",
				FieldNames: []string{"id", "age"},
			},
			{
				TypeName:   "Admin",
				FieldNames: []string{"id", "age"},
			},
		},
		Custom: ConfigJson(Configuration{
			Fetch: FetchConfiguration{
				URL: "http://fourth.service",
			},
			Federation: FederationConfiguration{
				Enabled:    true,
				ServiceSDL: fourthSubgraphSDL,
			},
			UpstreamSchema: fourthSubgraphSDL,
		}),
		Factory: federationFactory,
		FederationMetaData: plan.FederationMetaData{
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Account",
					SelectionSet: "id",
				},
				{
					TypeName:       "Admin",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
				{
					TypeName:       "Moderator",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
				{
					TypeName:       "User",
					InterfaceNames: []string{"Account"},
					SelectionSet:   "id",
				},
			},
		},
		RenameTypes: plan.TypeConfigurations{
			{
				TypeName: "Admin",
				RenameTo: "Account",
			},
			{
				TypeName: "Moderator",
				RenameTo: "Account",
			},
			{
				TypeName: "User",
				RenameTo: "Account",
			},
		},
	}

	dataSources := []plan.DataSourceConfiguration{
		firstDatasourceConfiguration,
		secondDatasourceConfiguration,
		thirdDatasourceConfiguration,
		fourthDatasourceConfiguration,
	}

	planConfiguration := plan.Configuration{
		DataSources:                  ShuffleDS(dataSources),
		DisableResolveFieldPositions: true,
		Debug: plan.DebugConfiguration{
			PrintOperationTransformations: true,
			PrintQueryPlans:               true,
			PrintPlanningPaths:            true,
			PrintNodeSuggestions:          true,
		},
	}

	t.Run("query 1 - Interface to interface object", func(t *testing.T) {

		t.Run("run", RunTest(
			definition,
			`
				query _1_InterfaceToInterfaceObject {
					allAccountsInterface {
						id
						locations {
							country
						}
					}
				}`,
			"_1_InterfaceToInterfaceObject",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://first.service","body":{"query":""}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
								DataSource:     &Source{},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{},
					},
				},
			},
			planConfiguration,
		))

	})

}
