package graphql_datasource

import (
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
)

const EntityInterfacesDefinition = `
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

func EntityInterfacesPlanConfiguration(factory plan.PlannerFactory) *plan.Configuration {
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
			{
				TypeName:   "Account",
				FieldNames: []string{"id", "title"},
			},
		},
		Custom: ConfigJson(Configuration{
			Fetch: FetchConfiguration{
				URL: "http://localhost:4001/graphql",
			},
			Federation: FederationConfiguration{
				Enabled:    true,
				ServiceSDL: firstSubgraphSDL,
			},
			UpstreamSchema: firstSubgraphSDL,
		}),
		Factory: factory,
		FederationMetaData: plan.FederationMetaData{
			EntityInterfaces: []plan.EntityInterfaceConfiguration{
				{
					InterfaceTypeName: "Account",
					ConcreteTypeNames: []string{"Admin", "Moderator", "User"},
				},
			},
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Account",
					SelectionSet: "id",
				},
				{
					TypeName:     "Admin",
					SelectionSet: "id",
				},
				{
					TypeName:     "Moderator",
					SelectionSet: "id",
				},
				{
					TypeName:     "User",
					SelectionSet: "id",
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
				URL: "http://localhost:4002/graphql",
			},
			Federation: FederationConfiguration{
				Enabled:    true,
				ServiceSDL: secondSubgraphSDL,
			},
			UpstreamSchema: secondSubgraphSDL,
		}),
		Factory: factory,
		FederationMetaData: plan.FederationMetaData{
			InterfaceObjects: []plan.EntityInterfaceConfiguration{
				{
					InterfaceTypeName: "Account",
					ConcreteTypeNames: []string{"Admin", "Moderator", "User"},
				},
			},
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Account",
					SelectionSet: "id",
				},
				{
					TypeName:     "Admin",
					SelectionSet: "id",
				},
				{
					TypeName:     "Moderator",
					SelectionSet: "id",
				},
				{
					TypeName:     "User",
					SelectionSet: "id",
				},
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
				URL: "http://localhost:4003/graphql",
			},
			Federation: FederationConfiguration{
				Enabled:    true,
				ServiceSDL: thirdSubgraphSDL,
			},
			UpstreamSchema: thirdSubgraphSDL,
		}),
		Factory: factory,
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
				URL: "http://localhost:4004/graphql",
			},
			Federation: FederationConfiguration{
				Enabled:    true,
				ServiceSDL: fourthSubgraphSDL,
			},
			UpstreamSchema: fourthSubgraphSDL,
		}),
		Factory: factory,
		FederationMetaData: plan.FederationMetaData{
			InterfaceObjects: []plan.EntityInterfaceConfiguration{
				{
					InterfaceTypeName: "Account",
					ConcreteTypeNames: []string{"Admin", "Moderator", "User"},
				},
			},
			Keys: plan.FederationFieldConfigurations{
				{
					TypeName:     "Account",
					SelectionSet: "id",
				},
				{
					TypeName:     "Admin",
					SelectionSet: "id",
				},
				{
					TypeName:     "Moderator",
					SelectionSet: "id",
				},
				{
					TypeName:     "User",
					SelectionSet: "id",
				},
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
		DataSources:                  dataSources,
		DisableResolveFieldPositions: true,
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "user",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "admin",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		Debug: plan.DebugConfiguration{
			PrintOperationTransformations: false,
			PrintQueryPlans:               false,
			PrintPlanningPaths:            false,
			PrintNodeSuggestions:          false,

			DatasourceVisitor: false,
		},
	}

	return &planConfiguration
}
