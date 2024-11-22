package graphql_datasource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
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

func EntityInterfacesPlanConfiguration(t *testing.T, factory plan.PlannerFactory[Configuration]) *plan.Configuration {
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

	firstDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		firstSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: firstSubgraphSDL,
		},
	)
	require.NoError(t, err)

	firstCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4001/graphql",
		},
		SchemaConfiguration: firstDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	firstDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"first",
		factory,
		&plan.DataSourceMetadata{
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
		},
		firstCustomConfiguration,
	)
	require.NoError(t, err)

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

	secondDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		secondSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: secondSubgraphSDL,
		},
	)
	require.NoError(t, err)

	secondCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4002/graphql",
		},
		SchemaConfiguration: secondDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	secondDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"second",
		factory,
		&plan.DataSourceMetadata{
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
		},
		secondCustomConfiguration,
	)
	require.NoError(t, err)

	thirdSubgraphSDL := `
		type Admin @key(fields: "id"){
			id: ID!
			title: String!
		}`

	thirdDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		thirdSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: thirdSubgraphSDL,
		},
	)
	require.NoError(t, err)

	thirdCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4003/graphql",
		},
		SchemaConfiguration: thirdDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	thirdDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"third",
		factory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Admin",
					FieldNames: []string{"id", "title"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				},
			},
		},
		thirdCustomConfiguration,
	)
	require.NoError(t, err)

	fourthSubgraphSDL := `
		type Account @key(fields: "id") @interfaceObject {
			id: ID!
			age: Int!
		}`

	fourthDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		fourthSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: fourthSubgraphSDL,
		},
	)
	require.NoError(t, err)

	fourthCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4004/graphql",
		},
		SchemaConfiguration: fourthDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	fourthDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"fourth",
		factory,
		&plan.DataSourceMetadata{
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
		},
		fourthCustomConfiguration,
	)
	require.NoError(t, err)

	dataSources := []plan.DataSource{
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

func EntityInterfacesPlanConfigurationBench(t *testing.B, factory plan.PlannerFactory[Configuration]) *plan.Configuration {
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

	firstDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		firstSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: firstSubgraphSDL,
		},
	)
	require.NoError(t, err)

	firstCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4001/graphql",
		},
		SchemaConfiguration: firstDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	firstDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"first",
		factory,
		&plan.DataSourceMetadata{
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
		},
		firstCustomConfiguration,
	)
	require.NoError(t, err)

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

	secondDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		secondSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: secondSubgraphSDL,
		},
	)
	require.NoError(t, err)

	secondCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4002/graphql",
		},
		SchemaConfiguration: secondDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	secondDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"second",
		factory,
		&plan.DataSourceMetadata{
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
		},
		secondCustomConfiguration,
	)
	require.NoError(t, err)

	thirdSubgraphSDL := `
		type Admin @key(fields: "id"){
			id: ID!
			title: String!
		}`

	thirdDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		thirdSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: thirdSubgraphSDL,
		},
	)
	require.NoError(t, err)

	thirdCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4003/graphql",
		},
		SchemaConfiguration: thirdDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	thirdDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"third",
		factory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Admin",
					FieldNames: []string{"id", "title"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:     "Admin",
						SelectionSet: "id",
					},
				},
			},
		},
		thirdCustomConfiguration,
	)
	require.NoError(t, err)

	fourthSubgraphSDL := `
		type Account @key(fields: "id") @interfaceObject {
			id: ID!
			age: Int!
		}`

	fourthDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		fourthSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: fourthSubgraphSDL,
		},
	)
	require.NoError(t, err)

	fourthCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4004/graphql",
		},
		SchemaConfiguration: fourthDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	fourthDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"fourth",
		factory,
		&plan.DataSourceMetadata{
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
		},
		fourthCustomConfiguration,
	)
	require.NoError(t, err)

	dataSources := []plan.DataSource{
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
