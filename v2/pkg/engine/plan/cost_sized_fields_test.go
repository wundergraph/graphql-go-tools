package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestActualCost_SizedFieldsParentNotTracked verifies that the actual cost of a multi-entity
// query does not exceed the sum of costs when each entity is queried individually.
//
// When a non-list wrapper field (e.g. items_page: ItemsPage!) is annotated with
// @listSize(sizedFields: ["items"]), its occurrences are not recorded in actualListSizes
// because it does not return a list type. This causes the averaging denominator for the
// child list multiplier to default to 1 (total items) instead of the number of parent
// field occurrences, inflating the combined cost.
//
// Schema:
//
//	type Query {
//	    boards(ids: [ID!], limit: Int): [Board!]! @listSize(slicingArguments: ["limit"]) @cost(weight: 10)
//	}
//	type Board {
//	    id: ID!
//	    items_page(limit: Int!): ItemsPage! @listSize(sizedFields: ["items"]) @cost(weight: 10)
//	}
//	type ItemsPage {
//	    items: [Item!]! @cost(weight: 10)
//	}
//	type Item {
//	    column_values: [ColumnValue!]!
//	}
//	type ColumnValue { id: ID! }
//
// Query:
//
//	boards(limit:4) {
//		items_page(limit:1) {
//			items {
//				column_values {
//					id
//				}
//			}
//		}
//	}
func TestActualCost_SizedFieldsParentNotTracked(t *testing.T) {
	const dsHash DSHash = 1

	costCfg := &DataSourceCostConfig{
		Weights: map[FieldCoordinate]*FieldCost{
			{TypeName: "Query", FieldName: "boards"}:     {HasWeight: true, Weight: 10},
			{TypeName: "Board", FieldName: "items_page"}: {HasWeight: true, Weight: 10},
			{TypeName: "ItemsPage", FieldName: "items"}:  {HasWeight: true, Weight: 10},
		},
		ListSizes: map[FieldCoordinate]*FieldListSize{
			{TypeName: "Query", FieldName: "boards"}:     {SlicingArguments: []string{"limit"}},
			{TypeName: "Board", FieldName: "items_page"}: {SizedFields: []string{"items"}},
		},
		Types: map[string]int{},
	}

	newCalc := func() *CostCalculator {
		root := &CostTreeNode{fieldCoords: FieldCoordinate{"_none", "_root"}}
		boards := &CostTreeNode{parent: root, dataSourceHashes: []DSHash{dsHash}, fieldCoords: FieldCoordinate{"Query", "boards"}, returnsListType: true, jsonPath: "boards"}
		itemsPage := &CostTreeNode{parent: boards, dataSourceHashes: []DSHash{dsHash}, fieldCoords: FieldCoordinate{"Board", "items_page"}, returnsListType: false, jsonPath: "boards.items_page"}
		items := &CostTreeNode{parent: itemsPage, dataSourceHashes: []DSHash{dsHash}, fieldCoords: FieldCoordinate{"ItemsPage", "items"}, returnsListType: true, jsonPath: "boards.items_page.items"}
		columnValues := &CostTreeNode{parent: items, dataSourceHashes: []DSHash{dsHash}, fieldCoords: FieldCoordinate{"Item", "column_values"}, returnsListType: true, jsonPath: "boards.items_page.items.column_values"}
		id := &CostTreeNode{parent: columnValues, dataSourceHashes: []DSHash{dsHash}, fieldCoords: FieldCoordinate{"ColumnValue", "id"}, returnsSimpleType: true, jsonPath: "boards.items_page.items.column_values.id"}

		columnValues.children = []*CostTreeNode{id}
		items.children = []*CostTreeNode{columnValues}
		itemsPage.children = []*CostTreeNode{items}
		boards.children = []*CostTreeNode{itemsPage}
		root.children = []*CostTreeNode{boards}

		return &CostCalculator{
			tree:            root,
			costConfigs:     map[DSHash]*DataSourceCostConfig{dsHash: costCfg},
			defaultListSize: 1,
		}
	}

	t.Run("combined actual cost should not exceed sum of separate costs", func(t *testing.T) {
		// 4 boards in one request; A and C have 0 items, B has 1 item (11 cv), D has 1 item (12 cv).
		combined := newCalc().ActualCost(nil, map[string]int{
			"boards":                                4,
			"boards.items_page.items":               2,
			"boards.items_page.items.column_values": 23,
		})

		boardA := newCalc().ActualCost(nil, map[string]int{"boards": 1})
		boardB := newCalc().ActualCost(nil, map[string]int{
			"boards": 1, "boards.items_page.items": 1, "boards.items_page.items.column_values": 11,
		})
		boardC := newCalc().ActualCost(nil, map[string]int{"boards": 1})
		boardD := newCalc().ActualCost(nil, map[string]int{
			"boards": 1, "boards.items_page.items": 1, "boards.items_page.items.column_values": 12,
		})
		sumSeparate := boardA + boardB + boardC + boardD

		assert.Equal(t, 41, boardB) // board with 1 item and 11 column values
		assert.Equal(t, 42, boardD) // board with 1 item and 12 column values
		assert.Equal(t, 124, combined)
		// Batching multiple entities into one request should never cost more than running them individually
		assert.LessOrEqual(t, combined, sumSeparate)
	})
}
