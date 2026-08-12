package graphql_datasource

import (
	"testing"

	"github.com/wundergraph/astjson"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGraphQLDataSourceFederation_InterfaceTypedProvides(t *testing.T) {
	planConfiguration := plan.Configuration{
		DisableResolveFieldPositions: true,
		DataSources: []plan.DataSource{
			interfaceProvidesDatasourceA(t),
			interfaceProvidesDatasourceB(t),
			interfaceProvidesDatasourceC(t),
		},
	}

	t.Run("provided interface fields", RunTest(
		interfaceProvidesGraphSchema,
		`{ media { id animals { id name } } }`,
		"",
		interfaceProvidesPlan(),
		planConfiguration,
		WithDefaultPostProcessor(),
	))

	t.Run("provided interface fields with concrete extension", RunTest(
		interfaceProvidesGraphSchema,
		`{ media { id animals { id name ... on Cat { age } } } }`,
		"",
		interfaceProvidesWithCatAgePlan(),
		planConfiguration,
		WithDefaultPostProcessor(),
	))
}

func interfaceProvidesPlan() *plan.SynchronousResponsePlan {
	return &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: resolve.Sequence(resolve.Single(&resolve.SingleFetch{
				FetchConfiguration: resolve.FetchConfiguration{
					Input:          `{"method":"POST","url":"http://localhost:4250/provides-on-interface/b","body":{"query":"{media {__typename ... on Book {id animals {__typename id name}}}}"}}`,
					DataSource:     &Source{},
					PostProcessing: DefaultPostProcessingConfiguration,
				},
				FetchDependencies: resolve.FetchDependencies{
					FetchID: 0,
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			})),
			Data: interfaceProvidesResponseData([]*resolve.Field{
				interfaceProvidesAnimalIDField(nil),
				interfaceProvidesAnimalNameField(nil),
			}),
		},
	}
}

func interfaceProvidesWithCatAgePlan() *plan.SynchronousResponsePlan {
	return &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: resolve.Sequence(
				resolve.Single(&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						Input:          `{"method":"POST","url":"http://localhost:4250/provides-on-interface/b","body":{"query":"{media {__typename ... on Book {id animals {__typename ... on Cat {id name __typename} ... on Dog {id name}}}}}"}}`,
						DataSource:     &Source{},
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					FetchDependencies: resolve.FetchDependencies{
						FetchID: 0,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}),
				resolve.SingleWithPath(&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						Input:                                 `{"method":"POST","url":"http://localhost:4250/provides-on-interface/c","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Cat {__typename age}}}","variables":{"representations":[$$0$$]}}}`,
						DataSource:                            &Source{},
						PostProcessing:                        EntitiesPostProcessingConfiguration,
						RequiresEntityBatchFetch:              true,
						SetTemplateOutputToNullOnVariableNull: true,
						Variables: resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("__typename"),
									Value: &resolve.String{
										Path: []string{"__typename"},
									},
									OnTypeNames: [][]byte{[]byte("Cat")},
								},
								{
									Name: []byte("id"),
									Value: &resolve.Scalar{
										Path: []string{"id"},
									},
									OnTypeNames: [][]byte{[]byte("Cat")},
								},
							},
						})),
					},
					FetchDependencies: resolve.FetchDependencies{
						FetchID:           1,
						DependsOnFetchIDs: []int{0},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}, "media.animals", resolve.ObjectPath("media"), resolve.ArrayPath("animals")),
			),
			Data: interfaceProvidesResponseData([]*resolve.Field{
				interfaceProvidesAnimalIDField([][]byte{[]byte("Cat")}),
				interfaceProvidesAnimalNameField([][]byte{[]byte("Cat")}),
				{
					Name: []byte("age"),
					Value: &resolve.Integer{
						Path:     []string{"age"},
						Nullable: true,
					},
					OnTypeNames: [][]byte{[]byte("Cat")},
				},
				interfaceProvidesAnimalIDField([][]byte{[]byte("Dog")}),
				interfaceProvidesAnimalNameField([][]byte{[]byte("Dog")}),
			}),
		},
	}
}

func interfaceProvidesResponseData(animalFields []*resolve.Field) *resolve.Object {
	return &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name: []byte("media"),
				Value: &resolve.Object{
					Path:     []string{"media"},
					Nullable: true,
					Fields: []*resolve.Field{
						{
							Name: []byte("id"),
							Value: &resolve.Scalar{
								Path: []string{"id"},
							},
							OnTypeNames: [][]byte{[]byte("Book")},
						},
						{
							Name: []byte("animals"),
							Value: &resolve.Array{
								Path:     []string{"animals"},
								Nullable: true,
								Item: &resolve.Object{
									Nullable: true,
									Fields:   animalFields,
									PossibleTypes: map[string]struct{}{
										"Cat": {},
										"Dog": {},
									},
									TypeName: "Animal",
								},
								SkipItem: func(ctx *resolve.Context, value *astjson.Value) bool {
									return false
								},
							},
							OnTypeNames: [][]byte{[]byte("Book")},
						},
					},
					PossibleTypes: map[string]struct{}{
						"Book": {},
					},
					TypeName: "Media",
				},
			},
		},
	}
}

func interfaceProvidesAnimalIDField(onTypeNames [][]byte) *resolve.Field {
	return &resolve.Field{
		Name: []byte("id"),
		Value: &resolve.Scalar{
			Path: []string{"id"},
		},
		OnTypeNames: onTypeNames,
	}
}

func interfaceProvidesAnimalNameField(onTypeNames [][]byte) *resolve.Field {
	return &resolve.Field{
		Name: []byte("name"),
		Value: &resolve.String{
			Path:     []string{"name"},
			Nullable: true,
		},
		OnTypeNames: onTypeNames,
	}
}

func interfaceProvidesDatasourceA(t *testing.T) plan.DataSource {
	t.Helper()

	return mustDataSourceConfiguration(
		t,
		"provides-on-interface-a",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"media", "book"}},
				{TypeName: "Book", FieldNames: []string{"id", "animals"}},
				{TypeName: "Dog", ExternalFieldNames: []string{"id", "name"}},
				{TypeName: "Cat", ExternalFieldNames: []string{"id"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Media", FieldNames: []string{"id"}},
				{TypeName: "Animal", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Dog", SelectionSet: "id"},
					{TypeName: "Cat", SelectionSet: "id"},
				},
				Provides: plan.FederationFieldConfigurations{
					{TypeName: "Query", FieldName: "book", SelectionSet: "animals { ... on Dog { name } }"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{URL: "http://localhost:4250/provides-on-interface/a"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{
				Enabled:    true,
				ServiceSDL: interfaceProvidesSubgraphASDL,
			}, interfaceProvidesSubgraphASDL),
		}),
	)
}

func interfaceProvidesDatasourceB(t *testing.T) plan.DataSource {
	t.Helper()

	return mustDataSourceConfiguration(
		t,
		"provides-on-interface-b",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"media"}},
				{TypeName: "Book", FieldNames: []string{"id"}, ExternalFieldNames: []string{"animals"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Media", FieldNames: []string{"id", "animals"}},
				{TypeName: "Animal", FieldNames: []string{"id", "name"}},
				{TypeName: "Dog", ExternalFieldNames: []string{"id", "name"}},
				{TypeName: "Cat", ExternalFieldNames: []string{"id", "name"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id", DisableEntityResolver: true},
					{
						TypeName:              "Dog",
						SelectionSet:          "id",
						DisableEntityResolver: true,
						Conditions: []plan.KeyCondition{
							{
								FieldPath: []string{"media", "animals", "id"},
								Coordinates: []plan.FieldCoordinate{
									{TypeName: "Query", FieldName: "media"},
									{TypeName: "Media", FieldName: "animals"},
									{TypeName: "Animal", FieldName: "id"},
								},
							},
						},
					},
					{
						TypeName:              "Cat",
						SelectionSet:          "id",
						DisableEntityResolver: true,
						Conditions: []plan.KeyCondition{
							{
								FieldPath: []string{"media", "animals", "id"},
								Coordinates: []plan.FieldCoordinate{
									{TypeName: "Query", FieldName: "media"},
									{TypeName: "Media", FieldName: "animals"},
									{TypeName: "Animal", FieldName: "id"},
								},
							},
						},
					},
				},
				Provides: plan.FederationFieldConfigurations{
					{TypeName: "Query", FieldName: "media", SelectionSet: "animals { id name }"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{URL: "http://localhost:4250/provides-on-interface/b"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{
				Enabled:    true,
				ServiceSDL: interfaceProvidesSubgraphBSDL,
			}, interfaceProvidesSubgraphBSDL),
		}),
	)
}

func interfaceProvidesDatasourceC(t *testing.T) plan.DataSource {
	t.Helper()

	return mustDataSourceConfiguration(
		t,
		"provides-on-interface-c",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Book", FieldNames: []string{"id", "animals"}},
				{TypeName: "Dog", FieldNames: []string{"id", "name", "age"}},
				{TypeName: "Cat", FieldNames: []string{"id", "name", "age"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Media", FieldNames: []string{"id", "animals"}},
				{TypeName: "Animal", FieldNames: []string{"id", "name"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Dog", SelectionSet: "id"},
					{TypeName: "Cat", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t, ConfigurationInput{
			Fetch: &FetchConfiguration{URL: "http://localhost:4250/provides-on-interface/c"},
			SchemaConfiguration: mustSchema(t, &FederationConfiguration{
				Enabled:    true,
				ServiceSDL: interfaceProvidesSubgraphCSDL,
			}, interfaceProvidesSubgraphCSDL),
		}),
	)
}

const interfaceProvidesGraphSchema = `
schema {
	query: Query
}

type Query {
	media: Media
	book: Book
}

interface Media {
	id: ID!
	animals: [Animal]
}

interface Animal {
	id: ID!
	name: String
}

type Book implements Media {
	id: ID!
	animals: [Animal]
}

type Dog implements Animal {
	id: ID!
	name: String
	age: Int
}

type Cat implements Animal {
	id: ID!
	name: String
	age: Int
}
`

const interfaceProvidesSubgraphASDL = `
extend schema
	@link(
		url: "https://specs.apollo.dev/federation/v2.3"
		import: ["@key", "@shareable", "@external", "@provides"]
	)

type Query {
	media: Media @shareable
	book: Book @provides(fields: "animals { ... on Dog { name } }")
}

interface Media {
	id: ID!
}

interface Animal {
	id: ID!
}

type Book implements Media @key(fields: "id") {
	id: ID!
	animals: [Animal] @shareable
}

type Dog implements Animal @key(fields: "id") {
	id: ID! @external
	name: String @external
}

type Cat implements Animal @key(fields: "id") {
	id: ID! @external
}
`

const interfaceProvidesSubgraphBSDL = `
extend schema
	@link(
		url: "https://specs.apollo.dev/federation/v2.3"
		import: ["@key", "@shareable", "@provides", "@external"]
	)

type Query {
	media: Media @shareable @provides(fields: "animals { id name }")
}

interface Media {
	id: ID!
	animals: [Animal]
}

interface Animal {
	id: ID!
	name: String
}

type Book implements Media {
	id: ID! @shareable
	animals: [Animal] @external
}

type Dog implements Animal {
	id: ID! @external
	name: String @external
}

type Cat implements Animal {
	id: ID! @external
	name: String @external
}
`

const interfaceProvidesSubgraphCSDL = `
extend schema
	@link(
		url: "https://specs.apollo.dev/federation/v2.3"
		import: ["@key", "@shareable"]
	)

interface Media {
	id: ID!
	animals: [Animal]
}

interface Animal {
	id: ID!
	name: String
}

type Book implements Media @key(fields: "id") {
	id: ID!
	animals: [Animal] @shareable
}

type Dog implements Animal @key(fields: "id") {
	id: ID!
	name: String @shareable
	age: Int
}

type Cat implements Animal @key(fields: "id") {
	id: ID!
	name: String @shareable
	age: Int
}
`
