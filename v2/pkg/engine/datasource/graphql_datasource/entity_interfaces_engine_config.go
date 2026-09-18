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
          fullTitle: String!
          uniqueTitle: String!
		  locations: [Location!]
		  age: Int!
		  fieldWithArg(arg: String!): String!
		}

		type Location {
		  country: String!
		}

		type Admin implements Account {
		  id: ID!
		  title: String!
          fullTitle: String!
          uniqueTitle: String!
		  locations: [Location!]
		  age: Int!
		  fieldWithArg(arg: String!): String!
		}

		type Moderator implements Account {
		  id: ID!
		  title: String!
          fullTitle: String!
          uniqueTitle: String!
		  locations: [Location!]
		  age: Int!
		  fieldWithArg(arg: String!): String!
		}

		type User implements Account {
		  id: ID!
		  title: String!
          fullTitle: String!
          uniqueTitle: String!
		  locations: [Location!]
		  age: Int!
		  fieldWithArg(arg: String!): String!
		}

		union Accounts = Admin | Moderator | User

		interface OrphanEvent {
		  id: ID!
		}

		type OrphanA implements OrphanEvent {
		  id: ID!
		}

		type OrphanB implements OrphanEvent {
		  id: ID!
		}

		type Query {
		  allAccountsInterface: [Account]
		  allAccountsUnion: [Accounts]
		  user(id: ID!): User
		  admin(id: ID!): Admin
		  accountLocations: [Account!]!
		}

		type Subscription {
		  accountEvents: Account!
		  userEvents: User!
		  orphanEvents: OrphanEvent!
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
			title: String! @external
			uniqueTitle: String! @requires(fields: "title")
			fieldWithArg(arg: String!): String!
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
					TypeName:           "Account",
					FieldNames:         []string{"id", "locations", "uniqueTitle", "fieldWithArg"},
					ExternalFieldNames: []string{"title"},
				},
				{
					TypeName:           "User",
					FieldNames:         []string{"id", "locations", "uniqueTitle", "fieldWithArg"},
					ExternalFieldNames: []string{"title"},
				},
				{
					TypeName:           "Moderator",
					FieldNames:         []string{"id", "locations", "uniqueTitle", "fieldWithArg"},
					ExternalFieldNames: []string{"title"},
				},
				{
					TypeName:           "Admin",
					FieldNames:         []string{"id", "locations", "uniqueTitle", "fieldWithArg"},
					ExternalFieldNames: []string{"title"},
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
				Requires: plan.FederationFieldConfigurations{
					{
						TypeName:     "Account",
						SelectionSet: "title",
						FieldName:    "uniqueTitle",
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
			title: String! @external
			fullTitle: String! @requires(fields: "title")
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
					TypeName:           "Account",
					FieldNames:         []string{"id", "age", "fullTitle"},
					ExternalFieldNames: []string{"title"},
				},
				{
					TypeName:   "User",
					FieldNames: []string{"id", "age", "fullTitle"},
				},
				{
					TypeName:   "Moderator",
					FieldNames: []string{"id", "age", "fullTitle"},
				},
				{
					TypeName:   "Admin",
					FieldNames: []string{"id", "age", "fullTitle"},
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
				Requires: plan.FederationFieldConfigurations{
					{
						TypeName:     "Account",
						SelectionSet: "title",
						FieldName:    "fullTitle",
					},
				},
			},
		},
		fourthCustomConfiguration,
	)
	require.NoError(t, err)

	// The fifth subgraph models an event-driven subgraph: it publishes entity interface events but
	// cannot resolve entities itself, so all of its keys have the entity resolver disabled.
	fifthSubgraphSDL := `
		interface Account @key(fields: "id", resolvable: false) {
			id: ID!
		}

		type Admin implements Account @key(fields: "id", resolvable: false) {
			id: ID!
		}

		type Moderator implements Account @key(fields: "id", resolvable: false) {
			id: ID!
		}

		type User implements Account @key(fields: "id", resolvable: false) {
			id: ID!
		}

		type Subscription {
			accountEvents: Account!
			userEvents: User!
		}`

	fifthDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		fifthSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: fifthSubgraphSDL,
		},
	)
	require.NoError(t, err)

	fifthCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4005/graphql",
		},
		Subscription: &SubscriptionConfiguration{
			URL: "ws://localhost:4005/graphql",
		},
		SchemaConfiguration: fifthDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	fifthDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"events",
		factory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Subscription",
					FieldNames: []string{"accountEvents", "userEvents"},
				},
				{
					TypeName:   "Account",
					FieldNames: []string{"id"},
				},
				{
					TypeName:   "Admin",
					FieldNames: []string{"id"},
				},
				{
					TypeName:   "Moderator",
					FieldNames: []string{"id"},
				},
				{
					TypeName:   "User",
					FieldNames: []string{"id"},
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
						TypeName:              "Account",
						SelectionSet:          "id",
						DisableEntityResolver: true,
					},
					{
						TypeName:              "Admin",
						SelectionSet:          "id",
						DisableEntityResolver: true,
					},
					{
						TypeName:              "Moderator",
						SelectionSet:          "id",
						DisableEntityResolver: true,
					},
					{
						TypeName:              "User",
						SelectionSet:          "id",
						DisableEntityResolver: true,
					},
				},
			},
		},
		fifthCustomConfiguration,
	)
	require.NoError(t, err)

	// The sixth subgraph publishes an entity interface which no other subgraph declares, so no
	// datasource is able to resolve it. Its __typename has to stay on this datasource.
	sixthSubgraphSDL := `
		interface OrphanEvent @key(fields: "id", resolvable: false) {
			id: ID!
		}

		type OrphanA implements OrphanEvent @key(fields: "id", resolvable: false) {
			id: ID!
		}

		type OrphanB implements OrphanEvent @key(fields: "id", resolvable: false) {
			id: ID!
		}

		type Subscription {
			orphanEvents: OrphanEvent!
		}`

	sixthDatasourceSchemaConfiguration, err := NewSchemaConfiguration(
		sixthSubgraphSDL,
		&FederationConfiguration{
			Enabled:    true,
			ServiceSDL: sixthSubgraphSDL,
		},
	)
	require.NoError(t, err)

	sixthCustomConfiguration, err := NewConfiguration(ConfigurationInput{
		Fetch: &FetchConfiguration{
			URL: "http://localhost:4006/graphql",
		},
		Subscription: &SubscriptionConfiguration{
			URL: "ws://localhost:4006/graphql",
		},
		SchemaConfiguration: sixthDatasourceSchemaConfiguration,
	})
	require.NoError(t, err)

	sixthDatasourceConfiguration, err := plan.NewDataSourceConfiguration[Configuration](
		"orphan-events",
		factory,
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{
					TypeName:   "Subscription",
					FieldNames: []string{"orphanEvents"},
				},
				{
					TypeName:   "OrphanEvent",
					FieldNames: []string{"id"},
				},
				{
					TypeName:   "OrphanA",
					FieldNames: []string{"id"},
				},
				{
					TypeName:   "OrphanB",
					FieldNames: []string{"id"},
				},
			},
			FederationMetaData: plan.FederationMetaData{
				EntityInterfaces: []plan.EntityInterfaceConfiguration{
					{
						InterfaceTypeName: "OrphanEvent",
						ConcreteTypeNames: []string{"OrphanA", "OrphanB"},
					},
				},
				Keys: plan.FederationFieldConfigurations{
					{
						TypeName:              "OrphanEvent",
						SelectionSet:          "id",
						DisableEntityResolver: true,
					},
					{
						TypeName:              "OrphanA",
						SelectionSet:          "id",
						DisableEntityResolver: true,
					},
					{
						TypeName:              "OrphanB",
						SelectionSet:          "id",
						DisableEntityResolver: true,
					},
				},
			},
		},
		sixthCustomConfiguration,
	)
	require.NoError(t, err)

	dataSources := []plan.DataSource{
		firstDatasourceConfiguration,
		secondDatasourceConfiguration,
		thirdDatasourceConfiguration,
		fourthDatasourceConfiguration,
		fifthDatasourceConfiguration,
		sixthDatasourceConfiguration,
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
			{
				TypeName:  "Account",
				FieldName: "fieldWithArg",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "arg",
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
			{
				TypeName:  "Account",
				FieldName: "fieldWithArg",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "arg",
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
