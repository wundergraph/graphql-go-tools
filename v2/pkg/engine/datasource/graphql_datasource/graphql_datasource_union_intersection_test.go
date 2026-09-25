package graphql_datasource

import (
	"fmt"
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Reproduces The Guild federation-gateway-audit "union-intersection" suite.
//
// A union field is shareable on two subgraphs which have divergent union members:
//
//	subgraph A: union Media = Book | Song
//	subgraph B: union Media = Book | Movie
//
// When the query requests a fragment on a divergent member (Movie), the union
// members are shrunk to the intersection {Book}, and the divergent member fields
// are kept in the operation as unfetchable - they have a tree node in the node
// suggestions tree, but no suggestion items. The duplicate nodes selection
// must tolerate such empty tree nodes.
func unionIntersectionPlanConfiguration(t *testing.T) plan.Configuration {
	subgraphASDL := `
		type Query {
			media: Media @shareable
		}

		union Media = Book | Song

		type Book @key(fields: "id") {
			id: ID!
			title: String! @shareable
		}

		type Song @key(fields: "id") {
			id: ID!
			title: String! @shareable
		}
	`

	subgraphA := mustDataSourceConfiguration(
		t,
		"subgraph-a",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"media"}},
				{TypeName: "Book", FieldNames: []string{"id", "title"}},
				{TypeName: "Song", FieldNames: []string{"id", "title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Song", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://subgraph-a"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: subgraphASDL},
					subgraphASDL,
				),
			},
		),
	)

	subgraphBSDL := `
		type Query {
			media: Media @shareable
		}

		union Media = Book | Movie

		type Book @key(fields: "id") {
			id: ID!
			title: String! @shareable
		}

		type Movie @key(fields: "id") {
			id: ID!
			title: String! @shareable
		}
	`

	subgraphB := mustDataSourceConfiguration(
		t,
		"subgraph-b",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"media"}},
				{TypeName: "Book", FieldNames: []string{"id", "title"}},
				{TypeName: "Movie", FieldNames: []string{"id", "title"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Book", SelectionSet: "id"},
					{TypeName: "Movie", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://subgraph-b"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: subgraphBSDL},
					subgraphBSDL,
				),
			},
		),
	)

	return plan.Configuration{
		DataSources: []plan.DataSource{
			subgraphA,
			subgraphB,
		},
		DisableResolveFieldPositions: true,
	}
}

const unionIntersectionDefinition = `
	type Query {
		media: Media
	}

	union Media = Book | Movie | Song

	type Book {
		id: ID!
		title: String!
	}

	type Song {
		id: ID!
		title: String!
	}

	type Movie {
		id: ID!
		title: String!
	}
`

// unionIntersectionSingleFetchPlan builds the expected plan for a query resolving
// `media { ... }` in a single subgraph fetch.
func unionIntersectionSingleFetchPlan(url, query string, mediaFields []*resolve.Field) *plan.SynchronousResponsePlan {
	return &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Fetches: resolve.Sequence(
				resolve.Single(&resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						Input:          fmt.Sprintf(`{"method":"POST","url":"%s","body":{"query":%q}}`, url, query),
						PostProcessing: DefaultPostProcessingConfiguration,
						DataSource:     &Source{},
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				}),
			),
			Data: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("media"),
						Value: &resolve.Object{
							Path:          []string{"media"},
							Nullable:      true,
							PossibleTypes: map[string]struct{}{"Book": {}, "Movie": {}, "Song": {}},
							TypeName:      "Media",
							Fields:        mediaFields,
						},
					},
				},
			},
		},
	}
}

// TestUnionIntersection reproduces the union-intersection audit suite queries
// requesting fragments on divergent union members.
func TestUnionIntersection(t *testing.T) {
	planConfiguration := unionIntersectionPlanConfiguration(t)

	t.Run("fragments on an intersection member and a divergent member", RunTest(
		unionIntersectionDefinition,
		`query { media { ... on Book { title } ... on Movie { title } } }`,
		"",
		unionIntersectionSingleFetchPlan(
			"http://subgraph-a",
			"{media {__typename ... on Book {title}}}",
			[]*resolve.Field{
				{
					Name:        []byte("title"),
					Value:       &resolve.String{Path: []string{"title"}},
					OnTypeNames: [][]byte{[]byte("Book")},
				},
				{
					Name:        []byte("title"),
					Value:       &resolve.String{Path: []string{"title"}},
					OnTypeNames: [][]byte{[]byte("Movie")},
				},
			},
		),
		planConfiguration,
		WithDefaultPostProcessor(),
	))
}
