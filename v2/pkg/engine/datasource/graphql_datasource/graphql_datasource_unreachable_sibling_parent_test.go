package graphql_datasource

import (
	"testing"

	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// TestUnreachableSiblingParentDuplicate distills a production graph where the
// sibling-parent key fallback (assignKeys selecting an unselected parent duplicate
// to provide keys) selected a parent which itself can never be rooted on its
// datasource.
//
// The essential structure is a nested abstract chain rewritten over several
// iterations: an entity group (@key id, posts @external in the sibling
// subgraphs), @shareable connection value types, a union-typed node with a
// fragment on the Content interface, and inside it a non-entity @shareable
// member (Repost) with its own interface field content containing
// ... on Attachment. Attachment implements Content only in archive/discovery;
// accounts can never return it, so the fragment must be dropped for the accounts
// plan. Because deeper levels are rewritten in later iterations, the datasource
// filter sees the not yet rewritten ... on Attachment and its sibling-parent
// fallback selects the archive/discovery duplicate of content - a parent rooted
// in a keyless value-type chain, which degenerates into an invalid root query
// fetch ("Cannot query field \"id\" on type \"Query\"").
func TestUnreachableSiblingParentDuplicate(t *testing.T) {
	definition := `
		type Query {
			account: Account
		}

		type Account {
			groups: GroupConnection!
		}

		type GroupConnection {
			edges: [GroupEdge]
		}

		type GroupEdge {
			node: Group!
		}

		interface Group {
			id: ID!
			posts: PostConnection!
		}

		type UserGroup implements Group {
			id: ID!
			posts: PostConnection!
		}

		type PostConnection {
			edges: [PostEdge!]!
		}

		type PostEdge {
			node: Post
		}

		union Post = Message | Comment | Repost

		interface Content {
			id: ID!
		}

		type Repost implements Content {
			id: ID!
			content: Content!
		}

		type Message implements Content {
			id: ID!
			delivered: Boolean
		}

		type Comment implements Content {
			id: ID!
			author: Author
		}

		type Attachment implements Content {
			id: ID!
		}

		type Author {
			id: ID!
			accountId: ID!
		}
	`

	accountsSDL := `
		type Query {
			account: Account
		}

		type Account @shareable {
			groups: GroupConnection!
		}

		type GroupConnection @shareable {
			edges: [GroupEdge]
		}

		type GroupEdge @shareable {
			node: Group!
		}

		interface Group {
			id: ID!
			posts: PostConnection!
		}

		type UserGroup implements Group @key(fields: "id") {
			id: ID!
			posts: PostConnection!
		}

		type PostConnection @shareable {
			edges: [PostEdge!]!
		}

		type PostEdge @shareable {
			node: Post
		}

		union Post = Message | Comment | Repost

		interface Content {
			id: ID!
		}

		type Repost implements Content @shareable {
			id: ID!
			content: Content!
		}

		type Message implements Content @key(fields: "id") {
			id: ID!
		}

		type Comment implements Content @key(fields: "id") {
			id: ID!
		}
	`

	accountsSubgraph := mustDataSourceConfiguration(
		t,
		"accounts",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"account"}},
				{TypeName: "UserGroup", FieldNames: []string{"id", "posts"}},
				{TypeName: "Message", FieldNames: []string{"id"}},
				{TypeName: "Comment", FieldNames: []string{"id"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "Account", FieldNames: []string{"groups"}},
				{TypeName: "GroupConnection", FieldNames: []string{"edges"}},
				{TypeName: "GroupEdge", FieldNames: []string{"node"}},
				{TypeName: "Group", FieldNames: []string{"id", "posts"}},
				{TypeName: "PostConnection", FieldNames: []string{"edges"}},
				{TypeName: "PostEdge", FieldNames: []string{"node"}},
				{TypeName: "Repost", FieldNames: []string{"id", "content"}},
				{TypeName: "Content", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "UserGroup", SelectionSet: "id"},
					{TypeName: "Message", SelectionSet: "id"},
					{TypeName: "Comment", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://accounts"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: accountsSDL},
					accountsSDL,
				),
			},
		),
	)

	messagesSDL := `
		type Message @key(fields: "id") {
			id: ID!
			delivered: Boolean
		}

		type Comment @key(fields: "id") {
			id: ID!
			author: Author
		}

		type Attachment @key(fields: "id") {
			id: ID!
		}

		type Author @key(fields: "id") {
			id: ID!
			accountId: ID!
		}
	`

	messagesSubgraph := mustDataSourceConfiguration(
		t,
		"messages",
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "Message", FieldNames: []string{"id", "delivered"}},
				{TypeName: "Comment", FieldNames: []string{"id", "author"}},
				{TypeName: "Attachment", FieldNames: []string{"id"}},
				{TypeName: "Author", FieldNames: []string{"id", "accountId"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "Message", SelectionSet: "id"},
					{TypeName: "Comment", SelectionSet: "id"},
					{TypeName: "Attachment", SelectionSet: "id"},
					{TypeName: "Author", SelectionSet: "id"},
				},
			},
		},
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://messages"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: messagesSDL},
					messagesSDL,
				),
			},
		),
	)

	archiveSDL := `
		type UserGroup @key(fields: "id") {
			id: ID!
			posts: PostConnection! @external
		}

		type PostConnection @shareable {
			edges: [PostEdge!]!
		}

		type PostEdge @shareable {
			node: Post
		}

		union Post = Message | Comment | Repost

		interface Content {
			id: ID!
		}

		type Repost implements Content @shareable {
			id: ID!
			content: Content!
		}

		type Message @key(fields: "id") {
			id: ID!
		}

		type Comment @key(fields: "id") {
			id: ID!
		}

		type Attachment implements Content @key(fields: "id") {
			id: ID!
		}
	`

	archiveMetadata := func() *plan.DataSourceMetadata {
		return &plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{
				{TypeName: "UserGroup", FieldNames: []string{"id"}},
				{TypeName: "Message", FieldNames: []string{"id"}},
				{TypeName: "Comment", FieldNames: []string{"id"}},
				{TypeName: "Attachment", FieldNames: []string{"id"}},
			},
			ChildNodes: []plan.TypeField{
				{TypeName: "PostConnection", FieldNames: []string{"edges"}},
				{TypeName: "PostEdge", FieldNames: []string{"node"}},
				{TypeName: "Repost", FieldNames: []string{"id", "content"}},
				{TypeName: "Content", FieldNames: []string{"id"}},
			},
			FederationMetaData: plan.FederationMetaData{
				Keys: plan.FederationFieldConfigurations{
					{TypeName: "UserGroup", SelectionSet: "id"},
					{TypeName: "Message", SelectionSet: "id"},
					{TypeName: "Comment", SelectionSet: "id"},
					{TypeName: "Attachment", SelectionSet: "id"},
				},
			},
		}
	}

	archiveSubgraph := mustDataSourceConfiguration(
		t,
		"archive",
		archiveMetadata(),
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://archive"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: archiveSDL},
					archiveSDL,
				),
			},
		),
	)

	discoverySubgraph := mustDataSourceConfiguration(
		t,
		"discovery",
		archiveMetadata(),
		mustCustomConfiguration(t,
			ConfigurationInput{
				Fetch: &FetchConfiguration{URL: "http://discovery"},
				SchemaConfiguration: mustSchema(t,
					&FederationConfiguration{Enabled: true, ServiceSDL: archiveSDL},
					archiveSDL,
				),
			},
		),
	)

	planConfiguration := plan.Configuration{
		DataSources: []plan.DataSource{
			accountsSubgraph,
			messagesSubgraph,
			archiveSubgraph,
			discoverySubgraph,
		},
		DisableResolveFieldPositions: true,
	}

	t.Run("nested attachment fragment dropped, no fetch targets archive", RunTest(
		definition,
		`query { account { groups { edges { node { posts { edges { node {
			... on Content {
				... on Message { delivered }
				... on Attachment { id }
				... on Repost { content { ... on Comment { author { accountId } } ... on Attachment { id } } }
			}
		} } } } } } } }`,
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
								// The ... on Attachment fragments are dropped on both nesting levels:
								// accounts can never return an Attachment. Message and Comment carry
								// their key fields for the entity jumps into messages.
								Input:          `{"method":"POST","url":"http://accounts","body":{"query":"{account {groups {edges {node {__typename posts {edges {node {__typename ... on Message {__typename id} ... on Repost {content {__typename ... on Comment {__typename id}}}}}}}}}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}),
					resolve.SingleWithPath(
						&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           1,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:                                 `{"method":"POST","url":"http://messages","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Message {__typename delivered}}}","variables":{"representations":[$$0$$]}}}`,
								DataSource:                            &Source{},
								SetTemplateOutputToNullOnVariableNull: true,
								RequiresEntityBatchFetch:              true,
								PostProcessing:                        EntitiesPostProcessingConfiguration,
								Variables: []resolve.Variable{
									&resolve.ResolvableObjectVariable{
										Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("Message")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Message")},
												},
											},
										}),
									},
								},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "account.groups.edges.@.node.posts.edges.@.node",
						resolve.ObjectPath("account"),
						resolve.ObjectPath("groups"),
						resolve.ArrayPath("edges"),
						resolve.ObjectPath("node"),
						resolve.ObjectPath("posts"),
						resolve.ArrayPath("edges"),
						resolve.ObjectPath("node"),
					),
					resolve.SingleWithPath(
						&resolve.SingleFetch{
							FetchDependencies: resolve.FetchDependencies{
								FetchID:           2,
								DependsOnFetchIDs: []int{0},
							},
							FetchConfiguration: resolve.FetchConfiguration{
								Input:                                 `{"method":"POST","url":"http://messages","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Comment {__typename author {accountId}}}}","variables":{"representations":[$$0$$]}}}`,
								DataSource:                            &Source{},
								SetTemplateOutputToNullOnVariableNull: true,
								RequiresEntityBatchFetch:              true,
								PostProcessing:                        EntitiesPostProcessingConfiguration,
								Variables: []resolve.Variable{
									&resolve.ResolvableObjectVariable{
										Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path: []string{"__typename"},
													},
													OnTypeNames: [][]byte{[]byte("Comment")},
												},
												{
													Name: []byte("id"),
													Value: &resolve.Scalar{
														Path: []string{"id"},
													},
													OnTypeNames: [][]byte{[]byte("Comment")},
												},
											},
										}),
									},
								},
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						}, "account.groups.edges.@.node.posts.edges.@.node.content",
						resolve.ObjectPath("account"),
						resolve.ObjectPath("groups"),
						resolve.ArrayPath("edges"),
						resolve.ObjectPath("node"),
						resolve.ObjectPath("posts"),
						resolve.ArrayPath("edges"),
						resolve.ObjectPath("node"),
						resolve.PathElementWithTypeNames(resolve.ObjectPath("content"), []string{"Repost"}),
					),
				),
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("account"),
							Value: &resolve.Object{
								Path:          []string{"account"},
								Nullable:      true,
								PossibleTypes: map[string]struct{}{"Account": {}},
								TypeName:      "Account",
								Fields: []*resolve.Field{
									{
										Name: []byte("groups"),
										Value: &resolve.Object{
											Path:          []string{"groups"},
											PossibleTypes: map[string]struct{}{"GroupConnection": {}},
											TypeName:      "GroupConnection",
											Fields: []*resolve.Field{
												{
													Name: []byte("edges"),
													Value: &resolve.Array{
														Path:     []string{"edges"},
														Nullable: true,
														Item: &resolve.Object{
															Nullable:      true,
															PossibleTypes: map[string]struct{}{"GroupEdge": {}},
															TypeName:      "GroupEdge",
															Fields: []*resolve.Field{
																{
																	Name: []byte("node"),
																	Value: &resolve.Object{
																		Path:          []string{"node"},
																		PossibleTypes: map[string]struct{}{"UserGroup": {}},
																		TypeName:      "Group",
																		Fields: []*resolve.Field{
																			{
																				Name: []byte("posts"),
																				Value: &resolve.Object{
																					Path:          []string{"posts"},
																					PossibleTypes: map[string]struct{}{"PostConnection": {}},
																					TypeName:      "PostConnection",
																					Fields: []*resolve.Field{
																						{
																							Name: []byte("edges"),
																							Value: &resolve.Array{
																								Path: []string{"edges"},
																								Item: &resolve.Object{
																									PossibleTypes: map[string]struct{}{"PostEdge": {}},
																									TypeName:      "PostEdge",
																									Fields: []*resolve.Field{
																										{
																											Name: []byte("node"),
																											Value: &resolve.Object{
																												Path:          []string{"node"},
																												Nullable:      true,
																												PossibleTypes: map[string]struct{}{"Comment": {}, "Message": {}, "Repost": {}},
																												TypeName:      "Post",
																												Fields: []*resolve.Field{
																													{
																														Name: []byte("delivered"),
																														Value: &resolve.Boolean{
																															Path:     []string{"delivered"},
																															Nullable: true,
																														},
																														OnTypeNames: [][]byte{[]byte("Message")},
																													},
																													{
																														Name: []byte("content"),
																														Value: &resolve.Object{
																															Path:          []string{"content"},
																															PossibleTypes: map[string]struct{}{"Attachment": {}, "Comment": {}, "Message": {}, "Repost": {}},
																															TypeName:      "Content",
																															Fields: []*resolve.Field{
																																{
																																	Name: []byte("author"),
																																	Value: &resolve.Object{
																																		Path:          []string{"author"},
																																		Nullable:      true,
																																		PossibleTypes: map[string]struct{}{"Author": {}},
																																		TypeName:      "Author",
																																		Fields: []*resolve.Field{
																																			{
																																				Name: []byte("accountId"),
																																				Value: &resolve.Scalar{
																																					Path: []string{"accountId"},
																																				},
																																			},
																																		},
																																	},
																																	OnTypeNames: [][]byte{[]byte("Comment")},
																																},
																															},
																														},
																														OnTypeNames: [][]byte{[]byte("Repost")},
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
			},
		},
		planConfiguration,
		WithDefaultPostProcessor(),
	))
}
