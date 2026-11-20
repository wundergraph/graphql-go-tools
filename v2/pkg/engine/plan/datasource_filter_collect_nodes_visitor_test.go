package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestNodesCollectorAssignFields(t *testing.T) {
	dataSources := []DataSource{
		// 0 - root nodes of Query and User
		dsb().Hash(1).Schema(`type Query { me: User } type User { id: ID! }`).
			RootNode("Query", "me").RootNode("User", "id").DS(),
		// 1 - a child node of Node and a root node of User
		dsb().Hash(2).Schema(`interface Node { id: ID! } type User { name: String }`).
			ChildNode("Node", "id").RootNode("User", "name").DS(),
		// 2 - keys for Product only
		dsb().Hash(3).Schema(`type Product { upc: ID! }`).
			KeysMetadata(FederationFieldConfigurations{{TypeName: "Product", SelectionSet: "upc"}}).DS(),
		// 3 - a root node of Query and a @provides on Query.other
		dsb().Hash(4).Schema(`type Query { other: String }`).RootNode("Query", "other").
			WithMetadata(func(data *FederationMetaData) {
				data.Provides = FederationFieldConfigurations{{TypeName: "Query", FieldName: "other", SelectionSet: "upc"}}
			}).DS(),
		// 4 - a union of the upstream schema
		dsb().Hash(5).Schema(`union SearchResult = Product type Product { upc: ID! }`).DS(),
	}

	fieldInfos := map[int]fieldInfo{
		0: {typeName: "Query", fieldName: "me"},
		1: {typeName: "User", fieldName: "id"},
		2: {typeName: "User", fieldName: "__typename", isTypeName: true},
		3: {typeName: "Node", fieldName: "id", possibleTypeNames: []string{"User", "Product"}},
		// 4 has no field info
		5: {typeName: "Review", fieldName: "body"},
		6: {typeName: "SearchResult", fieldName: "__typename", isTypeName: true},
		// 9 has a @provides, 8 is its child, 7 is its grandchild.
		// The parser adds a child field before its parent, so the parent has the higher ref.
		9: {typeName: "Query", fieldName: "other", parentFieldRef: ast.InvalidRef},
		8: {typeName: "Product", fieldName: "upc", parentFieldRef: 9},
		7: {typeName: "Review", fieldName: "body", parentFieldRef: 8},
	}

	newCollector := func() *nodesCollector {
		c := &nodesCollector{dataSources: dataSources, fieldInfo: fieldInfos}
		c.init()
		return c
	}

	assignedFieldRefs := func(c *nodesCollector) [][]int {
		out := make([][]int, len(c.dsCollectors))
		for i, collector := range c.dsCollectors {
			out[i] = append([]int{}, collector.assignedFieldRefs...)
		}
		return out
	}

	t.Run("assigns the fields of known types in the visit order", func(t *testing.T) {
		c := newCollector()
		c.assignFields([]int{0, 1, 2, 3, 4, 5, 6})

		assert.Equal(t, [][]int{
			{0, 1, 2, 3},
			// field 3 once, as a field of Node and of its possible type User
			{1, 2, 3},
			// field 3 for its possible type Product
			{3},
			// no field is at or under the @provides field
			{0},
			{6},
		}, assignedFieldRefs(c))
	})

	t.Run("assigns the fields at or under a @provides field", func(t *testing.T) {
		c := newCollector()
		c.assignFields([]int{9, 8, 7})

		assert.Equal(t, [][]int{
			{9},
			{},
			{8},
			// field 9 once, for the @provides and for the type Query
			{9, 8, 7},
			{},
		}, assignedFieldRefs(c))
	})

	t.Run("keeps a data source once in the @provides scope", func(t *testing.T) {
		// @provides on field 9 and on its child 8, on the same data source
		ds := dsb().Hash(1).Schema(`type Query { other: String }`).RootNode("Query", "other").
			WithMetadata(func(data *FederationMetaData) {
				data.Provides = FederationFieldConfigurations{
					{TypeName: "Query", FieldName: "other", SelectionSet: "upc"},
					{TypeName: "Product", FieldName: "upc", SelectionSet: "body"},
				}
			}).DS()
		c := &nodesCollector{dataSources: []DataSource{ds}, fieldInfo: fieldInfos}
		c.init()
		c.assignFields([]int{9, 8, 7})

		assert.Equal(t, []int{0}, c.providesScopeDS[8])
		assert.Equal(t, []int{0}, c.providesScopeDS[7])
		assert.Equal(t, []int{9, 8, 7}, c.dsCollectors[0].assignedFieldRefs)
	})

	t.Run("resets the fields between runs", func(t *testing.T) {
		c := newCollector()
		c.assignFields([]int{0, 1, 2, 3, 4, 5, 6})
		c.assignFields([]int{1, 5})

		assert.Equal(t, [][]int{
			{1},
			{1},
			{},
			{},
			{},
		}, assignedFieldRefs(c))
	})

	t.Run("skips unfetchable fields", func(t *testing.T) {
		c := newCollector()
		c.unfetchableFieldRefs = map[int]struct{}{1: {}, 3: {}}
		c.assignFields([]int{0, 1, 2, 3, 4, 5, 6})

		assert.Equal(t, [][]int{
			{0, 2},
			{2},
			{},
			{0},
			{6},
		}, assignedFieldRefs(c))
	})
}

func TestNodesCollectorResetsDSCollectorsWithoutFields(t *testing.T) {
	c := &nodesCollector{
		nodes:     NewNodeSuggestions(),
		report:    &operationreport.Report{},
		seenKeys:  make(map[SeenKeyPath]struct{}),
		fieldInfo: make(map[int]fieldInfo),
	}
	for i := range 2 {
		c.dataSources = append(c.dataSources, dsb().Hash(DSHash(i+1)).DS())
	}
	c.init()

	// a suggestion left from an earlier run, in which this data source had fields
	c.dsCollectors[0].localSuggestions = append(c.dsCollectors[0].localSuggestions, &NodeSuggestion{FieldRef: treeRootID})

	// no data source has fields in this run
	c.collectNodes(map[int]struct{}{})

	assert.Empty(t, c.nodes.items)
	assert.Empty(t, c.dsCollectors[0].localSuggestions)
}
