package engine

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	graphqlDataSource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestNewConfiguration(t *testing.T) {
	var engineConfig Configuration

	t.Run("should create a new engine v2 config", func(t *testing.T) {
		schema, err := graphql.NewSchemaFromString(graphql.CountriesSchema)
		require.NoError(t, err)

		engineConfig = NewConfiguration(schema)
		assert.Len(t, engineConfig.plannerConfig.DataSources, 0)
		assert.Len(t, engineConfig.plannerConfig.Fields, 0)
	})

	t.Run("should successfully add a data source", func(t *testing.T) {
		ds, _ := plan.NewDataSourceConfiguration[any]("1", nil, nil, []byte("1"))
		engineConfig.AddDataSource(ds)

		assert.Len(t, engineConfig.plannerConfig.DataSources, 1)
		assert.Equal(t, ds, engineConfig.plannerConfig.DataSources[0])
	})

	t.Run("should successfully set all data sources", func(t *testing.T) {
		one, _ := plan.NewDataSourceConfiguration[any]("1", nil, nil, []byte("1"))
		two, _ := plan.NewDataSourceConfiguration[any]("2", nil, nil, []byte("2"))
		three, _ := plan.NewDataSourceConfiguration[any]("3", nil, nil, []byte("3"))
		ds := []plan.DataSource{
			one,
			two,
			three,
		}
		engineConfig.SetDataSources(ds)

		assert.Len(t, engineConfig.plannerConfig.DataSources, 3)
		assert.Equal(t, ds, engineConfig.plannerConfig.DataSources)
	})

	t.Run("should successfully add a field config", func(t *testing.T) {
		fieldConfig := plan.FieldConfiguration{FieldName: "a"}
		engineConfig.AddFieldConfiguration(fieldConfig)

		assert.Len(t, engineConfig.plannerConfig.Fields, 1)
		assert.Equal(t, fieldConfig, engineConfig.plannerConfig.Fields[0])
	})

	t.Run("should successfully set all field configs", func(t *testing.T) {
		fieldConfigs := plan.FieldConfigurations{
			{FieldName: "b"},
			{FieldName: "c"},
			{FieldName: "d"},
		}
		engineConfig.SetFieldConfigurations(fieldConfigs)

		assert.Len(t, engineConfig.plannerConfig.Fields, 3)
		assert.Equal(t, fieldConfigs, engineConfig.plannerConfig.Fields)
	})
}

func TestGraphQLDataSourceGenerator_Generate(t *testing.T) {
	client := &http.Client{}
	streamingClient := &http.Client{}
	engineCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doc, report := astparser.ParseGraphqlDocumentString(graphqlGeneratorSchema)
	require.Falsef(t, report.HasErrors(), "document parser report has errors")

	expectedRootNodes := plan.TypeFields{
		{
			TypeName:   "Query",
			FieldNames: []string{"me", "_entities"},
		},
		{
			TypeName:   "Mutation",
			FieldNames: []string{"addUser"},
		},
		{
			TypeName:   "Subscription",
			FieldNames: []string{"userCount"},
		},
	}
	expectedChildNodes := plan.TypeFields{
		{
			TypeName:   "User",
			FieldNames: []string{"id", "name", "age", "language"},
		},
		{
			TypeName:   "Language",
			FieldNames: []string{"code", "name"},
		},
	}

	t.Run("without subscription configuration", func(t *testing.T) {
		dataSourceConfig := mustConfiguration(t, graphqlDataSource.ConfigurationInput{
			Fetch: &graphqlDataSource.FetchConfiguration{
				URL:    "http://localhost:8080",
				Method: http.MethodGet,
				Header: map[string][]string{
					"Authorization": {"123abc"},
				},
			},
			SchemaConfiguration: mustSchemaConfig(t,
				nil,
				graphqlGeneratorFullSchema,
			),
		})

		dataSource, err := newGraphQLDataSourceGenerator(engineCtx, &doc).Generate(
			"test",
			dataSourceConfig,
			client,
			nil,
			WithDataSourceGeneratorSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		require.NoError(t, err)

		ds, ok := dataSource.(plan.NodesAccess)
		require.True(t, ok)

		assert.Equal(t, expectedRootNodes, ds.ListRootNodes())
		assert.Equal(t, expectedChildNodes, ds.ListChildNodes())
	})

	t.Run("with subscription configuration (SSE)", func(t *testing.T) {
		dataSourceConfig := mustConfiguration(t, graphqlDataSource.ConfigurationInput{
			Fetch: &graphqlDataSource.FetchConfiguration{
				URL:    "http://localhost:8080",
				Method: http.MethodGet,
				Header: map[string][]string{
					"Authorization": {"123abc"},
				},
			},
			Subscription: &graphqlDataSource.SubscriptionConfiguration{
				UseSSE: true,
			},
			SchemaConfiguration: mustSchemaConfig(t,
				nil,
				graphqlGeneratorFullSchema,
			),
		})

		dataSource, err := newGraphQLDataSourceGenerator(engineCtx, &doc).Generate(
			"test",
			dataSourceConfig,
			client,
			nil,
			WithDataSourceGeneratorSubscriptionConfiguration(streamingClient, SubscriptionTypeSSE),
			WithDataSourceGeneratorSubscriptionClientFactory(&MockSubscriptionClientFactory{}),
		)
		require.NoError(t, err)

		ds, ok := dataSource.(plan.NodesAccess)
		require.True(t, ok)

		assert.Equal(t, expectedRootNodes, ds.ListRootNodes())
		assert.Equal(t, expectedChildNodes, ds.ListChildNodes())
	})

}

func TestGraphqlFieldConfigurationsGenerator_Generate(t *testing.T) {
	schema, err := graphql.NewSchemaFromString(graphqlGeneratorSchema)
	require.NoError(t, err)

	t.Run("should generate field configs without predefined field configs", func(t *testing.T) {
		fieldConfigurations := newGraphQLFieldConfigsGenerator(schema).Generate()
		sort.Slice(fieldConfigurations, func(i, j int) bool { // make the resulting slice deterministic again
			return fieldConfigurations[i].TypeName < fieldConfigurations[j].TypeName
		})

		expectedFieldConfigurations := plan.FieldConfigurations{
			{
				TypeName:  "Mutation",
				FieldName: "addUser",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
					{
						Name:       "age",
						SourceType: plan.FieldArgumentSource,
					},
					{
						Name:       "language",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "_entities",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "representations",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		}

		assert.Equal(t, expectedFieldConfigurations, fieldConfigurations)
	})

	t.Run("should generate field configs with predefined field configs", func(t *testing.T) {
		predefinedFieldConfigs := plan.FieldConfigurations{
			{
				TypeName:  "User",
				FieldName: "name",
			},
			{
				TypeName:  "Query",
				FieldName: "_entities",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "representations",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		}

		fieldConfigurations := newGraphQLFieldConfigsGenerator(schema).Generate(predefinedFieldConfigs...)
		sort.Slice(fieldConfigurations, func(i, j int) bool { // make the resulting slice deterministic again
			return fieldConfigurations[i].TypeName < fieldConfigurations[j].TypeName
		})

		expectedFieldConfigurations := plan.FieldConfigurations{
			{
				TypeName:  "Mutation",
				FieldName: "addUser",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
					{
						Name:       "age",
						SourceType: plan.FieldArgumentSource,
					},
					{
						Name:       "language",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "_entities",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "representations",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "User",
				FieldName: "name",
			},
		}

		assert.Equal(t, expectedFieldConfigurations, fieldConfigurations)
	})

}

var mockSubscriptionClient = graphqlDataSource.NewGraphQLSubscriptionClient(context.Background())

type MockSubscriptionClientFactory struct{}

func (m *MockSubscriptionClientFactory) NewSubscriptionClient(engineCtx context.Context, options ...graphqlDataSource.SubscriptionClientOption) graphqlDataSource.GraphQLSubscriptionClient {
	return mockSubscriptionClient
}

var graphqlGeneratorSchema = `scalar _Any
	union _Entity = User

	type Query {
		me: User!
		_entities(representations: [_Any!]!): [_Entity]!
	}

	type Mutation {
		addUser(name: String!, age: Int!, language: Language!): User!
	}

	type Subscription {
		userCount: Int!
	}

	type User {
		id: ID!
		name: String!
		age: Int!
		language: Language!
	}

	type Language {
		code: String!
		name: String!
	}
`

var graphqlGeneratorFullSchema = `schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

scalar _Any

union _Entity = User

type Query {
    me: User!
    _entities(representations: [_Any!]!): [_Entity]!
    __schema: __Schema!
    __type(name: String!): __Type
    __typename: String!
}

type Mutation {
    addUser(name: String!, age: Int!, language: Language!): User!
    __typename: String!
}

type Subscription {
    userCount: Int!
}

type User {
    id: ID!
    name: String!
    age: Int!
    language: Language!
    __typename: String!
}

type Language {
    code: String!
    name: String!
    __typename: String!
}

"The 'Int' scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1."
scalar Int

"The 'Float' scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point)."
scalar Float

"The 'String' scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text."
scalar String

"The 'Boolean' scalar type represents 'true' or 'false' ."
scalar Boolean

"The 'ID' scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as '4') or integer (such as 4) input value will be accepted as an ID."
scalar ID

"Directs the executor to include this field or fragment only when the argument is true."
directive @include(
    "Included when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT

"Directs the executor to skip this field or fragment when the argument is true."
directive @skip(
    "Skipped when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT

"Marks an element of a GraphQL schema as no longer supported."
directive @deprecated(
    """
    Explains why this element was deprecated, usually also including a suggestion
    for how to access supported similar data. Formatted in
    [Markdown](https://daringfireball.net/projects/markdown/).
    """
    reason: String = "No longer supported"
) on FIELD_DEFINITION | ARGUMENT_DEFINITION | ENUM_VALUE | INPUT_FIELD_DEFINITION

directive @specifiedBy(
    url: String!
) on SCALAR

"""
The @oneOf built-in directive marks an input object as a OneOf Input Object.
Exactly one field must be provided and its value must be non-null at runtime.
All fields defined within a @oneOf input must be nullable in the schema.
"""
directive @oneOf on INPUT_OBJECT

"""
A Directive provides a way to describe alternate runtime execution and type validation behavior in a GraphQL document.
In some cases, you need to provide options to alter GraphQL's execution behavior
in ways field arguments will not suffice, such as conditionally including or
skipping a field. Directives provide this by describing additional information
to the executor.
"""
type __Directive {
    name: String!
    description: String
    locations: [__DirectiveLocation!]!
    args(includeDeprecated: Boolean = false): [__InputValue!]!
    isRepeatable: Boolean!
    __typename: String!
}

"""
A Directive can be adjacent to many parts of the GraphQL language, a
__DirectiveLocation describes one such possible adjacencies.
"""
enum __DirectiveLocation {
    "Location adjacent to a query operation."
    QUERY
    "Location adjacent to a mutation operation."
    MUTATION
    "Location adjacent to a subscription operation."
    SUBSCRIPTION
    "Location adjacent to a field."
    FIELD
    "Location adjacent to a fragment definition."
    FRAGMENT_DEFINITION
    "Location adjacent to a fragment spread."
    FRAGMENT_SPREAD
    "Location adjacent to an inline fragment."
    INLINE_FRAGMENT
    "Location adjacent to a variable definition"
    VARIABLE_DEFINITION
    "Location adjacent to a schema definition."
    SCHEMA
    "Location adjacent to a scalar definition."
    SCALAR
    "Location adjacent to an object type definition."
    OBJECT
    "Location adjacent to a field definition."
    FIELD_DEFINITION
    "Location adjacent to an argument definition."
    ARGUMENT_DEFINITION
    "Location adjacent to an interface definition."
    INTERFACE
    "Location adjacent to a union definition."
    UNION
    "Location adjacent to an enum definition."
    ENUM
    "Location adjacent to an enum value definition."
    ENUM_VALUE
    "Location adjacent to an input object type definition."
    INPUT_OBJECT
    "Location adjacent to an input object field definition."
    INPUT_FIELD_DEFINITION
}

"""
One possible value for a given Enum. Enum values are unique values, not a
placeholder for a string or numeric value. However an Enum value is returned in
a JSON response as a string.
"""
type __EnumValue {
    name: String!
    description: String
    isDeprecated: Boolean!
    deprecationReason: String
    __typename: String!
}

"""
Object and Interface types are described by a list of Fields, each of which has
a name, potentially a list of arguments, and a return type.
"""
type __Field {
    name: String!
    description: String
    args(includeDeprecated: Boolean = false): [__InputValue!]!
    type: __Type!
    isDeprecated: Boolean!
    deprecationReason: String
    __typename: String!
}

"""
Arguments provided to Fields or Directives and the input fields of an
InputObject are represented as Input Values which describe their type and
optionally a default value.
"""
type __InputValue {
    name: String!
    description: String
    type: __Type!
    defaultValue: String
    isDeprecated: Boolean!
    deprecationReason: String
    __typename: String!
}

"""
A GraphQL Schema defines the capabilities of a GraphQL server. It exposes all
available types and directives on the server, as well as the entry points for
query, mutation, and subscription operations.
"""
type __Schema {
    description: String
    "A list of all types supported by this server."
    types: [__Type!]!
    "The type that query operations will be rooted at."
    queryType: __Type!
    "If this server supports mutation, the type that mutation operations will be rooted at."
    mutationType: __Type
    "If this server support subscription, the type that subscription operations will be rooted at."
    subscriptionType: __Type
    "A list of all directives supported by this server."
    directives: [__Directive!]!
    __typename: String!
}

"""
The fundamental unit of any GraphQL Schema is the type. There are many kinds of
types in GraphQL as represented by the '__TypeKind' enum.

Depending on the kind of a type, certain fields describe information about that
type. Scalar types provide no information beyond a name and description, while
Enum types provide their values. Object and Interface types provide the fields
they describe. Abstract types, Union and Interface, provide the Object types
possible at runtime. List and NonNull types compose other types.
"""
type __Type {
    kind: __TypeKind!
    name: String
    description: String
    fields(includeDeprecated: Boolean = false): [__Field!]
    interfaces: [__Type!]
    possibleTypes: [__Type!]
    enumValues(includeDeprecated: Boolean = false): [__EnumValue!]
    inputFields(includeDeprecated: Boolean = false): [__InputValue!]
    ofType: __Type
    specifiedByURL: String
    __typename: String!
}

"An enum describing what kind of type a given '__Type' is."
enum __TypeKind {
    "Indicates this type is a scalar."
    SCALAR
    "Indicates this type is an object. 'fields' and 'interfaces' are valid fields."
    OBJECT
    "Indicates this type is an interface. 'fields' ' and ' 'possibleTypes' are valid fields."
    INTERFACE
    "Indicates this type is a union. 'possibleTypes' is a valid field."
    UNION
    "Indicates this type is an enum. 'enumValues' is a valid field."
    ENUM
    "Indicates this type is an input object. 'inputFields' is a valid field."
    INPUT_OBJECT
    "Indicates this type is a list. 'ofType' is a valid field."
    LIST
    "Indicates this type is a non-null. 'ofType' is a valid field."
    NON_NULL
}`
