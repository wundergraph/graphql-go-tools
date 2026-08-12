package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestObjectVsUnionMemberFieldAlias is a characterization test of the CURRENT behavior
// of the field merging alias mechanism (see aliasConflictingMemberFields).
//
// Two members of an abstract selection declare the same field with the SAME nullability
// but different named types, where one type is an object and the other a union containing
// that object:
//
//	type Post    { author: Author } # union Author = User | Bot
//	type Message { author: User }
//
// fieldTypesDifferOnlyInNullability treats the two types as a merge conflict, because
// TypesAreCompatibleIgnoringNullability reports an object as compatible with a union
// it is a member of (see ast.Document.typesAreCompatible, union/object branch), while
// TypesAreEqualDeep reports them unequal. The planner therefore emits per-member
// __internal_merge_* aliases even though the nullability of both fields is identical.
//
// This is intentional documentation of the current behavior, not necessarily the
// desired end state: if fieldTypesDifferOnlyInNullability is ever narrowed to strict
// nullability-only differences (same named type), this test is expected to change.
// Note the aliases are not merely cosmetic here: distinct response paths keep the
// members' author objects from being merged into a single response path.
func TestObjectVsUnionMemberFieldAlias(t *testing.T) {
	definition := `
		type Query {
			feed: [FeedItem!]!
		}

		union FeedItem = Post | Message

		type Post {
			id: ID!
			author: Author
		}

		type Message {
			id: ID!
			author: User
		}

		union Author = User | Bot

		type User {
			id: ID!
			name: String
		}

		type Bot {
			id: ID!
			name: String
		}
	`

	feedSDL := `
		type Query {
			feed: [FeedItem!]!
		}

		union FeedItem = Post | Message

		type Post {
			id: ID!
			author: Author
		}

		type Message {
			id: ID!
			author: User
		}

		union Author = User | Bot

		type User {
			id: ID!
			name: String
		}

		type Bot {
			id: ID!
			name: String
		}
	`

	feedSubgraph := mustDataSourceConfiguration(
		t,
		"feed",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"feed"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Post", FieldNames: []string{"id", "author"}},
				{TypeName: "Message", FieldNames: []string{"id", "author"}},
				{TypeName: "User", FieldNames: []string{"id", "name"}},
				{TypeName: "Bot", FieldNames: []string{"id", "name"}},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://feed"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: feedSDL},
					feedSDL,
				),
			},
		),
	)

	planConfiguration := plan.Configuration{
		DataSources: []plan.DataSource{
			feedSubgraph,
		},
		DisableResolveFieldPositions: true,
	}

	t.Run("same nullability, object vs union member type - members get merge aliases", RunTest(
		definition,
		`query { feed { ... on Post { author { ... on User { name } ... on Bot { name } } } ... on Message { author { name } } } }`,
		"",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Fetches: resolve.Sequence(
					resolve.Single(
						&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID: 0,
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://feed","body":{"query":"{feed {__typename ... on Post {__internal_merge_Post_author: author {__typename ... on User {name} ... on Bot {name}}} ... on Message {__internal_merge_Message_author: author {name}}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("feed"),
							Value: &resolve.Array{
								Path: []string{"feed"},
								Item: &resolve.Object{
									PossibleTypes: map[string]struct{}{"Post": {}, "Message": {}},
									TypeName:      "FeedItem",
									Fields: []*resolve.Field{
										{
											// the client-visible response name is restored,
											// only the upstream fetch uses the internal alias path
											Name:        []byte("author"),
											OnTypeNames: [][]byte{[]byte("Post")},
											Value: &resolve.Object{
												Path:          []string{"__internal_merge_Post_author"},
												Nullable:      true,
												PossibleTypes: map[string]struct{}{"User": {}, "Bot": {}},
												TypeName:      "Author",
												Fields: []*resolve.Field{
													{
														Name:        []byte("name"),
														OnTypeNames: [][]byte{[]byte("User")},
														Value: &resolve.String{
															Path:     []string{"name"},
															Nullable: true,
														},
													},
													{
														Name:        []byte("name"),
														OnTypeNames: [][]byte{[]byte("Bot")},
														Value: &resolve.String{
															Path:     []string{"name"},
															Nullable: true,
														},
													},
												},
											},
										},
										{
											Name:        []byte("author"),
											OnTypeNames: [][]byte{[]byte("Message")},
											Value: &resolve.Object{
												Path:          []string{"__internal_merge_Message_author"},
												Nullable:      true,
												PossibleTypes: map[string]struct{}{"User": {}},
												TypeName:      "User",
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path:     []string{"name"},
															Nullable: true,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		planConfiguration,
		WithDefaultPostProcessor(),
	))
}
