package ast_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestPath_DotDelimitedString(t *testing.T) {
	tests := []struct {
		name      string
		path      ast.Path
		want      string
		wantNoRef string
	}{
		{
			name: "returns operation type for root query path",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
			},
			want:      "query",
			wantNoRef: "query",
		},
		{
			name: "returns operation type for root mutation path",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("mutation")},
			},
			want:      "mutation",
			wantNoRef: "mutation",
		},
		{
			name: "returns operation type for root subscription path",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("subscription")},
			},
			want:      "subscription",
			wantNoRef: "subscription",
		},
		{
			name: "converts empty field name to query as fallback",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("")},
			},
			want:      "query",
			wantNoRef: "query",
		},
		{
			name: "joins operation and field with dot separator",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("user")},
			},
			want:      "query.user",
			wantNoRef: "query.user",
		},
		{
			name: "nested query fields contain all elements in path",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("user")},
				{Kind: ast.FieldName, FieldName: []byte("name")},
			},
			want:      "query.user.name",
			wantNoRef: "query.user.name",
		},
		{
			name: "nested mutation fields contain all elements in path",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("mutation")},
				{Kind: ast.FieldName, FieldName: []byte("createUser")},
				{Kind: ast.FieldName, FieldName: []byte("id")},
			},
			want:      "mutation.createUser.id",
			wantNoRef: "mutation.createUser.id",
		},
		{
			name: "nested subscription fields contain all elements in path",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("subscription")},
				{Kind: ast.FieldName, FieldName: []byte("userUpdated")},
				{Kind: ast.FieldName, FieldName: []byte("name")},
			},
			want:      "subscription.userUpdated.name",
			wantNoRef: "subscription.userUpdated.name",
		},
		{
			name: "includes field aliases in path",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("myUser")}, // alias is stored in path
				{Kind: ast.FieldName, FieldName: []byte("email")},
			},
			want:      "query.myUser.email",
			wantNoRef: "query.myUser.email",
		},
		{
			name: "array indexes are represented as numbers",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("users")},
				{Kind: ast.ArrayIndex, ArrayIndex: 0},
				{Kind: ast.FieldName, FieldName: []byte("name")},
			},
			want:      "query.users.0.name",
			wantNoRef: "query.users.0.name",
		},
		{
			name: "multiple array indexes are all included in sequence",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("matrix")},
				{Kind: ast.ArrayIndex, ArrayIndex: 0},
				{Kind: ast.ArrayIndex, ArrayIndex: 1},
			},
			want:      "query.matrix.0.1",
			wantNoRef: "query.matrix.0.1",
		},
		{
			name: "inline fragments are prefixed with dollar sign and include fragment ref",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("node")},
				{Kind: ast.InlineFragmentName, FieldName: []byte("User"), FragmentRef: 1},
				{Kind: ast.FieldName, FieldName: []byte("name")},
			},
			want:      "query.node.$1User.name",
			wantNoRef: "query.node.$User.name",
		},
		{
			name: "multiple inline fragments each include their own ref number",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("search")},
				{Kind: ast.InlineFragmentName, FieldName: []byte("User"), FragmentRef: 1},
				{Kind: ast.FieldName, FieldName: []byte("profile")},
				{Kind: ast.InlineFragmentName, FieldName: []byte("PublicProfile"), FragmentRef: 2},
				{Kind: ast.FieldName, FieldName: []byte("bio")},
			},
			want:      "query.search.$1User.profile.$2PublicProfile.bio",
			wantNoRef: "query.search.$User.profile.$PublicProfile.bio",
		},
		{
			name: "inline fragments work in subscription operations",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("subscription")},
				{Kind: ast.FieldName, FieldName: []byte("messageAdded")},
				{Kind: ast.InlineFragmentName, FieldName: []byte("TextMessage"), FragmentRef: 1},
				{Kind: ast.FieldName, FieldName: []byte("text")},
			},
			want:      "subscription.messageAdded.$1TextMessage.text",
			wantNoRef: "subscription.messageAdded.$TextMessage.text",
		},
		{
			name: "combines fields, array indexes, and fragments in correct order",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("items")},
				{Kind: ast.ArrayIndex, ArrayIndex: 0},
				{Kind: ast.InlineFragmentName, FieldName: []byte("Product"), FragmentRef: 1},
				{Kind: ast.FieldName, FieldName: []byte("variants")},
				{Kind: ast.ArrayIndex, ArrayIndex: 2},
				{Kind: ast.FieldName, FieldName: []byte("price")},
			},
			want:      "query.items.0.$1Product.variants.2.price",
			wantNoRef: "query.items.0.$Product.variants.2.price",
		},
		{
			name: "handles deeply nested paths with multiple fragments and arrays",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("organization")},
				{Kind: ast.FieldName, FieldName: []byte("teams")},
				{Kind: ast.ArrayIndex, ArrayIndex: 1},
				{Kind: ast.InlineFragmentName, FieldName: []byte("EngineeringTeam"), FragmentRef: 1},
				{Kind: ast.FieldName, FieldName: []byte("members")},
				{Kind: ast.ArrayIndex, ArrayIndex: 0},
				{Kind: ast.InlineFragmentName, FieldName: []byte("Developer"), FragmentRef: 2},
				{Kind: ast.FieldName, FieldName: []byte("languages")},
			},
			want:      "query.organization.teams.1.$1EngineeringTeam.members.0.$2Developer.languages",
			wantNoRef: "query.organization.teams.1.$EngineeringTeam.members.0.$Developer.languages",
		},
		{
			name: "combines aliases and inline fragments",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("mutation")},
				{Kind: ast.FieldName, FieldName: []byte("myUser")}, // aliased field
				{Kind: ast.InlineFragmentName, FieldName: []byte("AdminUser"), FragmentRef: 1},
				{Kind: ast.FieldName, FieldName: []byte("permissions")},
			},
			want:      "mutation.myUser.$1AdminUser.permissions",
			wantNoRef: "mutation.myUser.$AdminUser.permissions",
		},
		{
			name: "zero fragment refs are included in output",
			path: ast.Path{
				{Kind: ast.FieldName, FieldName: []byte("query")},
				{Kind: ast.FieldName, FieldName: []byte("entity")},
				{Kind: ast.InlineFragmentName, FieldName: []byte("Node"), FragmentRef: 0},
				{Kind: ast.FieldName, FieldName: []byte("id")},
			},
			want:      "query.entity.$0Node.id",
			wantNoRef: "query.entity.$Node.id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.path.DotDelimitedString()
			assert.Equal(t, tt.want, got)

			gotNoRef := tt.path.DotDelimitedStringWithoutFragmentRefs()
			assert.Equal(t, tt.wantNoRef, gotNoRef)
		})
	}
}
