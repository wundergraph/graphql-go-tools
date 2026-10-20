package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func TestCollectKeysForPathExternalSibling(t *testing.T) {
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(`
		type User { id: ID! info: Info! }
		type Info { name: String! }
	`)
	for _, tc := range []struct {
		selection string
		paths     []KeyInfoFieldPath
	}{
		{
			selection: "id info { name }",
			paths: []KeyInfoFieldPath{
				{Path: "query.me.id"},
				{Path: "query.me.info", IsExternal: true},
				{Path: "query.me.info.name", IsExternal: true},
			},
		},
		{
			selection: "info { name } id",
			paths: []KeyInfoFieldPath{
				{Path: "query.me.info", IsExternal: true},
				{Path: "query.me.info.name", IsExternal: true},
				{Path: "query.me.id"},
			},
		},
	} {
		t.Run(tc.selection, func(t *testing.T) {
			dataSource := dsb().Hash(22).
				RootNode("User", "id").
				AddRootNodeExternalFieldNames("User", "info").
				ChildNode("Info").
				AddChildNodeExternalFieldNames("Info", "name").
				KeysMetadata(FederationFieldConfigurations{{TypeName: "User", SelectionSet: tc.selection}}).DS()
			visitor := &collectNodesDSVisitor{
				definition:          &definition,
				dataSource:          dataSource,
				localSeenKeys:       make(map[SeenKeyPath]struct{}),
				notExternalKeyPaths: make(map[string]struct{}),
			}

			require.NoError(t, visitor.collectKeysForPath("User", "query.me", nil))
			require.Len(t, visitor.keys, 1)
			require.Len(t, visitor.keys[0].Keys, 1)
			key := visitor.keys[0].Keys[0]
			assert.Equal(t, tc.paths, key.FieldPaths)
			assert.False(t, key.Source, "the external info subtree prevents supplying this key in either field order")
		})
	}
}
