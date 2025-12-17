package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test DSHash values
const (
	testDSHash1 DSHash = 1001
	testDSHash2 DSHash = 1002
)

func TestCostDefaults(t *testing.T) {
	// Test that defaults are set correctly
	assert.Equal(t, 1, StaticCostDefaults.FieldCost)
	assert.Equal(t, 0, StaticCostDefaults.ArgumentCost)
	assert.Equal(t, 0, StaticCostDefaults.ScalarCost)
	assert.Equal(t, 0, StaticCostDefaults.EnumCost)
	assert.Equal(t, 1, StaticCostDefaults.ObjectCost)
	assert.Equal(t, 10, StaticCostDefaults.ListCost)
}

func TestNewDataSourceCostConfig(t *testing.T) {
	config := NewDataSourceCostConfig()

	assert.NotNil(t, config.FieldConfig)
	assert.NotNil(t, config.ScalarWeights)
	assert.NotNil(t, config.EnumWeights)
}

func TestDataSourceCostConfig_GetFieldCost(t *testing.T) {
	config := NewDataSourceCostConfig()

	// Test default cost
	cost := config.GetFieldCost("Query", "users")
	assert.Equal(t, StaticCostDefaults.FieldCost, cost)

	// Test custom cost
	config.FieldConfig["Query.users"] = &FieldCostConfig{
		Weight: 5,
	}
	cost = config.GetFieldCost("Query", "users")
	assert.Equal(t, 5, cost)

	// Test with custom defaults
	config.Defaults = &CostDefaults{
		FieldCost: 2,
	}
	cost = config.GetFieldCost("Query", "posts")
	assert.Equal(t, 2, cost)
}

func TestDataSourceCostConfig_GetSlicingArguments(t *testing.T) {
	config := NewDataSourceCostConfig()

	// Test no list size config
	args := config.GetSlicingArguments("Query", "users")
	assert.Nil(t, args)

	// Test with list size config
	config.FieldConfig["Query.users"] = &FieldCostConfig{
		Weight:                    1,
		AssumedSize:               100,
		SlicingArguments:          []string{"first", "last"},
		RequireOneSlicingArgument: true,
	}

	// Test GetSlicingArguments
	args = config.GetSlicingArguments("Query", "users")
	assert.Equal(t, []string{"first", "last"}, args)

	// Test GetAssumedListSize
	assumed := config.GetAssumedListSize("Query", "users")
	assert.Equal(t, 100, assumed)
}

func TestCostTreeNode_TotalCost(t *testing.T) {
	// Build a simple tree:
	// root (cost: 1)
	//   └── users (cost: 1, multiplier: 10 from "first" arg)
	//         └── name (cost: 1)
	//         └── email (cost: 1)

	root := &CostTreeNode{
		FieldName:  "_root",
		Multiplier: 1,
	}

	users := &CostTreeNode{
		FieldName:  "users",
		FieldCost:  1,
		Multiplier: 10, // "first: 10"
		Parent:     root,
	}
	root.Children = append(root.Children, users)

	name := &CostTreeNode{
		FieldName: "name",
		FieldCost: 1,
		Parent:    users,
	}
	users.Children = append(users.Children, name)

	email := &CostTreeNode{
		FieldName: "email",
		FieldCost: 1,
		Parent:    users,
	}
	users.Children = append(users.Children, email)

	// Calculate: root cost = users cost + (children cost * multiplier)
	// users: 1 + (1 + 1) * 10 = 1 + 20 = 21
	// root: 0 + 21 * 1 = 21
	total := root.TotalCost()
	assert.Equal(t, 21, total)
}

func TestCostCalculator_BasicFlow(t *testing.T) {
	calc := NewCostCalculator()
	calc.Enable()

	config := NewDataSourceCostConfig()
	config.FieldConfig["Query.users"] = &FieldCostConfig{
		Weight:           2,
		SlicingArguments: []string{"first"},
	}
	calc.SetDataSourceCostConfig(testDSHash1, config)

	// Simulate entering and leaving fields (two-phase: Enter creates skeleton, Leave calculates costs)
	calc.EnterField(1, "Query", "users", true, []CostFieldArgument{
		{Name: "first", IntValue: 10},
	})
	calc.EnterField(2, "User", "name", false, nil)
	calc.LeaveField(2, []DSHash{testDSHash1})
	calc.EnterField(3, "User", "email", false, nil)
	calc.LeaveField(3, []DSHash{testDSHash1})
	calc.LeaveField(1, []DSHash{testDSHash1})

	// Get results
	tree := calc.GetTree()
	assert.NotNil(t, tree)
	assert.True(t, tree.Total > 0)

	totalCost := calc.GetTotalCost()
	// users: 2 (field) + 10 (list) + (1 + 1) * 10 = 12 + 20 = 32
	assert.Equal(t, 32, totalCost)
}

func TestCostCalculator_Disabled(t *testing.T) {
	calc := NewCostCalculator()
	// Don't enable

	calc.EnterField(1, "Query", "users", true, nil)
	calc.LeaveField(1, []DSHash{testDSHash1})

	// Should return 0 when disabled
	assert.Equal(t, 0, calc.GetTotalCost())
}

func TestCostCalculator_MultipleDataSources(t *testing.T) {
	calc := NewCostCalculator()
	calc.Enable()

	// Configure two different data sources with different costs
	config1 := NewDataSourceCostConfig()
	config1.FieldConfig["User.name"] = &FieldCostConfig{
		Weight: 2,
	}
	calc.SetDataSourceCostConfig(testDSHash1, config1)

	config2 := NewDataSourceCostConfig()
	config2.FieldConfig["User.name"] = &FieldCostConfig{
		Weight: 3,
	}
	calc.SetDataSourceCostConfig(testDSHash2, config2)

	// Field planned on both data sources - costs should be aggregated
	calc.EnterField(1, "User", "name", false, nil)
	calc.LeaveField(1, []DSHash{testDSHash1, testDSHash2})

	totalCost := calc.GetTotalCost()
	// Weight from subgraph1 (2) + cost from subgraph2 (3) = 5
	assert.Equal(t, 5, totalCost)
}

func TestCostCalculator_NoDataSource(t *testing.T) {
	calc := NewCostCalculator()
	calc.Enable()

	// Set default config
	defaultConfig := NewDataSourceCostConfig()
	defaultConfig.Defaults = &CostDefaults{
		FieldCost: 2,
	}
	calc.SetDefaultCostConfig(defaultConfig)

	// Field with no data source - should use default config
	calc.EnterField(1, "Query", "unknown", false, nil)
	calc.LeaveField(1, nil)

	totalCost := calc.GetTotalCost()
	assert.Equal(t, 2, totalCost)
}

func TestCostTree_Calculate(t *testing.T) {
	tree := &CostTree{
		Root: &CostTreeNode{
			FieldName:  "_root",
			Multiplier: 1,
			Children: []*CostTreeNode{
				{
					FieldName: "field1",
					FieldCost: 5,
				},
			},
		},
	}

	tree.Calculate()

	assert.Equal(t, 5, tree.Total)
}

func TestNilCostConfig(t *testing.T) {
	var config *DataSourceCostConfig

	// All methods should handle nil gracefully
	assert.Equal(t, 0, config.GetFieldCost("Type", "field"))
	assert.Equal(t, 0, config.GetArgumentCost("Type", "field", "arg"))
	assert.Equal(t, 0, config.GetScalarCost("String"))
	assert.Equal(t, 0, config.GetEnumCost("Status"))
	assert.Equal(t, 0, config.GetListCost())
	assert.Equal(t, 0, config.GetObjectCost())

	assert.Nil(t, config.GetSlicingArguments("Type", "field"))
	assert.Equal(t, 0, config.GetAssumedListSize("Type", "field"))
}

func TestCostCalculator_TwoPhaseFlow(t *testing.T) {
	// Test that the two-phase flow works correctly:
	// EnterField creates skeleton, LeaveField fills in costs
	calc := NewCostCalculator()
	calc.Enable()

	config := NewDataSourceCostConfig()
	config.FieldConfig["Query.users"] = &FieldCostConfig{
		Weight: 5,
	}
	calc.SetDataSourceCostConfig(testDSHash1, config)

	// Enter creates skeleton node
	calc.EnterField(1, "Query", "users", false, nil)

	// At this point, the node exists but has no cost calculated yet
	currentNode := calc.CurrentNode()
	assert.NotNil(t, currentNode)
	assert.Equal(t, "users", currentNode.FieldName)
	assert.Equal(t, 0, currentNode.FieldCost) // Weight not yet calculated

	// Leave fills in DS info and calculates cost
	calc.LeaveField(1, []DSHash{testDSHash1})

	// Now the cost should be calculated
	totalCost := calc.GetTotalCost()
	assert.Equal(t, 5, totalCost)
}

func TestCostCalculator_ListSizeAssumedSize(t *testing.T) {
	// Test that assumed size is used when no slicing argument is provided
	calc := NewCostCalculator()
	calc.Enable()

	config := NewDataSourceCostConfig()
	config.FieldConfig["Query.users"] = &FieldCostConfig{
		Weight:           1,
		AssumedSize:      50, // Assume 50 items if no slicing arg
		SlicingArguments: []string{"first", "last"},
	}
	calc.SetDataSourceCostConfig(testDSHash1, config)

	// Enter field with no slicing arguments
	calc.EnterField(1, "Query", "users", true, nil)

	// Enter child field
	calc.EnterField(2, "User", "name", false, nil)
	calc.LeaveField(2, []DSHash{testDSHash1})

	calc.LeaveField(1, []DSHash{testDSHash1})

	// multiplier should be 50 (assumed size)
	tree := calc.GetTree()
	assert.Equal(t, 50, tree.Root.Children[0].Multiplier)
}

func TestCostCalculator_ListSizeSlicingArg(t *testing.T) {
	// Test that slicing argument overrides assumed size
	calc := NewCostCalculator()
	calc.Enable()

	config := NewDataSourceCostConfig()
	config.FieldConfig["Query.users"] = &FieldCostConfig{
		Weight:           1,
		AssumedSize:      50, // This should NOT be used
		SlicingArguments: []string{"first", "last"},
	}
	calc.SetDataSourceCostConfig(testDSHash1, config)

	// Enter field with "first: 10" argument
	calc.EnterField(1, "Query", "users", true, []CostFieldArgument{
		{Name: "first", IntValue: 10},
	})
	calc.LeaveField(1, []DSHash{testDSHash1})

	// multiplier should be 10 (from slicing arg), not 50
	tree := calc.GetTree()
	assert.Equal(t, 10, tree.Root.Children[0].Multiplier)
}
