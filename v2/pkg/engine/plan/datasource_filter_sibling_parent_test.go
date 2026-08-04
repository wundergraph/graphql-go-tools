package plan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// TestAssignKeysSkipsUnreachableSiblingParent covers the sibling-parent key
// fallback of assignKeys: a not yet selected parent duplicate may provide the
// key for a jump into the current node's datasource, but only when that parent
// itself can be rooted on its datasource. Here the candidate parents
// (PostEdge.node and Repost.content on archive/discovery) sit in a keyless
// non-entity @shareable value type chain, and archive/discovery have no root
// query fields - selecting them produces a planner which degenerates into an
// invalid root query fetch ("Cannot query field \"id\" on type \"Query\"").
func TestAssignKeysSkipsUnreachableSiblingParent(t *testing.T) {
	definition := `
		type Query { account: Account }
		type Account { groups: GroupConnection! }
		type GroupConnection { edges: [GroupEdge] }
		type GroupEdge { node: Group! }
		interface Group { id: ID! posts: PostConnection! }
		type UserGroup implements Group { id: ID! posts: PostConnection! }
		type PostConnection { edges: [PostEdge!]! }
		type PostEdge { node: Post }
		union Post = Message | Comment | Repost
		interface Content { id: ID! }
		type Repost implements Content { id: ID! content: Content! }
		type Message implements Content { id: ID! delivered: Boolean }
		type Comment implements Content { id: ID! author: Author }
		type Attachment implements Content { id: ID! }
		type Author { id: ID! accountId: ID! }
	`

	query := `
		query {
			account { groups { edges { node { posts { edges { node {
				... on Content {
					... on Message { delivered }
					... on Attachment { id }
					... on Repost { content { ... on Comment { author { accountId } } ... on Attachment { id } } }
				}
			} } } } } } }
		}
	`

	accountsSchema := `
		type Query { account: Account }
		type Account @shareable { groups: GroupConnection! }
		type GroupConnection @shareable { edges: [GroupEdge] }
		type GroupEdge @shareable { node: Group! }
		interface Group { id: ID! posts: PostConnection! }
		type UserGroup implements Group @key(fields: "id") { id: ID! posts: PostConnection! }
		type PostConnection @shareable { edges: [PostEdge!]! }
		type PostEdge @shareable { node: Post }
		union Post = Message | Comment | Repost
		interface Content { id: ID! }
		type Repost implements Content @shareable { id: ID! content: Content! }
		type Message implements Content @key(fields: "id") { id: ID! }
		type Comment implements Content @key(fields: "id") { id: ID! }
	`

	messagesSchema := `
		type Message @key(fields: "id") { id: ID! delivered: Boolean }
		type Comment @key(fields: "id") { id: ID! author: Author }
		type Attachment @key(fields: "id") { id: ID! }
		type Author @key(fields: "id") { id: ID! accountId: ID! }
	`

	archiveSchema := `
		type UserGroup @key(fields: "id") { id: ID! posts: PostConnection! @external }
		type PostConnection @shareable { edges: [PostEdge!]! }
		type PostEdge @shareable { node: Post }
		union Post = Message | Comment | Repost
		interface Content { id: ID! }
		type Repost implements Content @shareable { id: ID! content: Content! }
		type Message @key(fields: "id") { id: ID! }
		type Comment @key(fields: "id") { id: ID! }
		type Attachment implements Content @key(fields: "id") { id: ID! }
	`

	newArchiveLikeDS := func(hash DSHash, id string) DataSource {
		return dsb().Hash(hash).Id(id).SchemaMergedWithBase(archiveSchema).
			RootNode("UserGroup", "id").
			RootNode("Message", "id").
			RootNode("Comment", "id").
			RootNode("Attachment", "id").
			ChildNode("PostConnection", "edges").
			ChildNode("PostEdge", "node").
			ChildNode("Repost", "id", "content").
			ChildNode("Content", "id").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "UserGroup", SelectionSet: "id"},
				{TypeName: "Message", SelectionSet: "id"},
				{TypeName: "Comment", SelectionSet: "id"},
				{TypeName: "Attachment", SelectionSet: "id"},
			}).DS()
	}

	dataSources := []DataSource{
		dsb().Hash(11).Id("accounts").SchemaMergedWithBase(accountsSchema).
			RootNode("Query", "account").
			RootNode("UserGroup", "id", "posts").
			RootNode("Message", "id").
			RootNode("Comment", "id").
			ChildNode("Account", "groups").
			ChildNode("GroupConnection", "edges").
			ChildNode("GroupEdge", "node").
			ChildNode("Group", "id", "posts").
			ChildNode("PostConnection", "edges").
			ChildNode("PostEdge", "node").
			ChildNode("Repost", "id", "content").
			ChildNode("Content", "id").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "UserGroup", SelectionSet: "id"},
				{TypeName: "Message", SelectionSet: "id"},
				{TypeName: "Comment", SelectionSet: "id"},
			}).DS(),
		dsb().Hash(22).Id("messages").SchemaMergedWithBase(messagesSchema).
			RootNode("Message", "id", "delivered").
			RootNode("Comment", "id", "author").
			RootNode("Attachment", "id").
			RootNode("Author", "id", "accountId").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "Message", SelectionSet: "id"},
				{TypeName: "Comment", SelectionSet: "id"},
				{TypeName: "Attachment", SelectionSet: "id"},
				{TypeName: "Author", SelectionSet: "id"},
			}).DS(),
		newArchiveLikeDS(33, "archive"),
		newArchiveLikeDS(44, "discovery"),
	}

	parsedDefinition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definition)
	operation := unsafeparser.ParseGraphqlDocumentString(query)
	report := operationreport.Report{}

	astvalidation.DefaultOperationValidator().Validate(&operation, &parsedDefinition, &report)
	require.False(t, report.HasErrors(), report.Error())

	dsFilter := NewDataSourceFilter(&operation, &parsedDefinition, &report, dataSources, nil)
	dsFilter.EnableSelectionReasons()
	planned, _ := dsFilter.findBestDataSourceSet(nil)
	require.False(t, report.HasErrors(), report.Error())

	// The value-type parents on archive/discovery can never be rooted there:
	// Repost/PostEdge have no keys and archive/discovery have no root query fields.
	// Nothing may be selected on those datasources for this operation.
	for _, item := range planned.items {
		if !item.Selected {
			continue
		}
		require.NotContains(t, []DSHash{33, 44}, item.DataSourceHash,
			"unexpected selection on unreachable datasource: %s (reasons: %v)", item.String(), item.SelectionReasons)
	}
}
