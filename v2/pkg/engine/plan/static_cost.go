package plan

import "fmt"

// StaticCostDefaults contains default cost values when no specific costs are configured
var StaticCostDefaults = WeightDefaults{
	Field:      1,
	EnumScalar: 0,
	Object:     1,
	List:       10, // The assumed maximum size of a list for fields that return lists.
}

// WeightDefaults defines default cost values for different GraphQL elements
type WeightDefaults struct {
	Field      int
	EnumScalar int
	Object     int
	List       int
}

// FieldCostConfig defines cost configuration for a specific field of an object or input object.
// Includes @listSize directive fields for objects.
type FieldCostConfig struct {
	Weight int

	// ArgumentWeights maps an argument name to its weight.
	// Location: ARGUMENT_DEFINITION
	ArgumentWeights map[string]int

	// Fields below are defined only on FIELD_DEFINITION from the @listSize directive.

	// AssumedSize is the default assumed size when no slicing argument is provided.
	// If 0, the global default list cost is used.
	AssumedSize int

	// SlicingArguments are argument names that control list size (e.g., "first", "last", "limit")
	// The value of these arguments will be used as the multiplier.
	SlicingArguments []string

	// SizedFields are field names that return the actual size of the list.
	// These can be used for more accurate, actual cost estimation.
	SizedFields []string

	// RequireOneSlicingArgument if true, at least one slicing argument must be provided.
	// If false and no slicing argument is provided, AssumedSize is used.
	RequireOneSlicingArgument bool
}

// DataSourceCostConfig holds all cost configurations for a data source.
// This data is passed from the composition.
type DataSourceCostConfig struct {
	// Fields maps field coordinate to its cost config. Cannot be on fields of interfaces.
	// Location: FIELD_DEFINITION, INPUT_FIELD_DEFINITION
	Fields map[FieldCoordinate]*FieldCostConfig

	// Types maps TypeName to the weight of the object, scalar or enum definition.
	// If TypeName is not present, the default value for Enums and Scalars is 0, otherwise 1.
	// Weight assigned to the field or argument definitions overrides the weight of type definition.
	// Location: ENUM, OBJECT, SCALAR
	Types map[string]int

	// Arguments on directives is a special case. They use a special kind of coordinate:
	// directive name + argument name. That should be the key mapped to the weight.
	//
	// Directives can be used on [input] object fields and arguments of fields. This creates
	// mutual recursion between them; it complicates cost calculation.
	// We avoid them intentionally in the first iteration.
}

// NewDataSourceCostConfig creates a new cost config with defaults
func NewDataSourceCostConfig() *DataSourceCostConfig {
	return &DataSourceCostConfig{
		Fields: make(map[FieldCoordinate]*FieldCostConfig),
		Types:  make(map[string]int),
	}
}

// EnumScalarWeight returns the cost for an enum or scalar types
func (c *DataSourceCostConfig) EnumScalarWeight(enumName string) int {
	if c == nil {
		return 0
	}
	if cost, ok := c.Types[enumName]; ok {
		return cost
	}
	return StaticCostDefaults.EnumScalar
}

// ObjectWeight returns the default object cost
func (c *DataSourceCostConfig) ObjectWeight(name string) int {
	if c == nil {
		return 0
	}
	if cost, ok := c.Types[name]; ok {
		return cost
	}
	return StaticCostDefaults.Object
}

// CostTreeNode represents a node in the cost calculation tree
// Based on IBM GraphQL Cost Specification: https://ibm.github.io/graphql-specs/cost-spec.html
type CostTreeNode struct {
	// fieldRef is the AST field reference
	fieldRef int

	// Enclosing type name and field name
	fieldCoord FieldCoordinate

	// dataSourceHashes identifies which data sources resolve this field.
	dataSourceHashes []DSHash

	// FieldCost is the weight of this field from @cost directive
	FieldCost int

	// ArgumentsCost is the sum of argument weights and input fields used on this field.
	ArgumentsCost int

	// Weights on directives ignored for now.
	DirectivesCost int

	// multiplier is the list size multiplier from @listSize directive
	// Applied to children costs for list fields
	multiplier int

	// Children contain child field costs
	Children []*CostTreeNode

	// The data below is stored for deferred cost calculation.
	// We populate these fields in EnterField and use them as a source of truth in LeaveField.
	//
	// fieldTypeName contains the name of an unwrapped (named) type that is returned by this field.
	fieldTypeName string

	// implementTypeNames contains the names of all types that implement this interface/union field.
	implementingTypeNames []string

	// arguments contain the values of arguments passed to the field.
	arguments map[string]ArgumentInfo

	isListType     bool
	isSimpleType   bool
	isAbstractType bool
}

type ArgumentInfo struct {
	intValue int

	// The name of an unwrapped type.
	typeName string

	// If argument is passed an input object, we want to gather counts
	// for all the field coordinates with non-null values used in the argument.
	//
	// For example, for
	//    "input A { x: Int, rec: A! }"
	// following value is passed:
	//    { x: 1, rec: { x: 2, rec: { x: 3 } } },
	// then coordCounts will be:
	//    { {"A", "rec"}: 2, {"A", "x"}: 3 }
	//
	coordCounts map[FieldCoordinate]int

	// isInputObject is true for an input object passed to the argument,
	// otherwise the argument is Scalar or Enum.
	isInputObject bool

	isScalar bool
}

// TotalCost calculates the total cost of this node and all descendants
// Per IBM spec: total = field_weight + argument_weights + (children_total * multiplier)
func (n *CostTreeNode) TotalCost() int {
	if n == nil {
		return 0
	}

	// Sum children (fields) costs
	var childrenCost int
	for _, child := range n.Children {
		childrenCost += child.TotalCost()
	}

	// Apply multiplier to children cost (for list fields)
	multiplier := n.multiplier
	if multiplier == 0 {
		multiplier = 1
	}
	// TODO: negative sum should be rounded up to zero
	cost := n.ArgumentsCost + n.DirectivesCost + (n.FieldCost+childrenCost)*multiplier

	return cost
}

// CostTree represents the complete cost tree for a query
type CostTree struct {
	Root  *CostTreeNode
	Total int
}

// Calculate computes the total cost and checks against max
func (t *CostTree) Calculate() {
	if t.Root != nil {
		t.Total = t.Root.TotalCost()
	}
}

// CostCalculator manages cost calculation during AST traversal
type CostCalculator struct {
	// stack maintains the current path in the cost tree
	stack []*CostTreeNode

	// tree is the complete cost tree being built
	tree *CostTree

	// costConfigs maps data source hash to its cost configuration
	costConfigs map[DSHash]*DataSourceCostConfig

	// defaultConfig is used when no data source specific config exists
	defaultConfig *DataSourceCostConfig
}

// NewCostCalculator creates a new cost calculator
func NewCostCalculator() *CostCalculator {
	tree := &CostTree{
		Root: &CostTreeNode{
			fieldCoord: FieldCoordinate{"_none", "_root"},
			multiplier: 1,
		},
	}
	c := CostCalculator{
		stack:       make([]*CostTreeNode, 0, 16),
		costConfigs: make(map[DSHash]*DataSourceCostConfig),
		tree:        tree,
	}
	c.stack = append(c.stack, c.tree.Root)

	return &c
}

// SetDataSourceCostConfig sets the cost config for a specific data source
func (c *CostCalculator) SetDataSourceCostConfig(dsHash DSHash, config *DataSourceCostConfig) {
	c.costConfigs[dsHash] = config
}

// CurrentNode returns the current node on the stack
func (c *CostCalculator) CurrentNode() *CostTreeNode {
	if len(c.stack) == 0 {
		return nil
	}
	return c.stack[len(c.stack)-1]
}

// EnterField is called when entering a field during AST traversal.
// It creates a skeleton node and pushes it onto the stack.
// The actual cost calculation happens in LeaveField when fieldPlanners data is available.
func (c *CostCalculator) EnterField(node *CostTreeNode) {
	// Attach to parent
	parent := c.CurrentNode()
	if parent != nil {
		parent.Children = append(parent.Children, node)
	}

	c.stack = append(c.stack, node)
}

// LeaveField is called when leaving a field during AST traversal.
// This is where we calculate costs because fieldPlanners data is now available.
func (c *CostCalculator) LeaveField(fieldRef int, dsHashes []DSHash) {
	// Find the current node (should match fieldRef)
	if len(c.stack) <= 1 { // Keep root on stack
		return
	}

	current := c.stack[len(c.stack)-1]
	if current.fieldRef != fieldRef {
		return
	}

	current.dataSourceHashes = dsHashes
	c.calculateNodeCosts(current)

	c.stack = c.stack[:len(c.stack)-1]
}

// calculateNodeCosts fills in the cost values for a node based on its data sources.
// It implements IBM GraphQL Cost Specification.
// See: https://ibm.github.io/graphql-specs/cost-spec.html#sec-Field-Cost
func (c *CostCalculator) calculateNodeCosts(node *CostTreeNode) {
	// For every data source we get different weights.
	// For this node we sum weights of the field and its arguments.
	// For the multiplier we pick the maximum.
	if len(node.dataSourceHashes) <= 0 {
		// no data source is responsible for this field
		return
	}

	node.multiplier = 0

	for _, dsHash := range node.dataSourceHashes {
		config, ok := c.costConfigs[dsHash]
		if !ok {
			fmt.Printf("WARNING: no cost config for data source %v\n", dsHash)
			continue
		}

		// TODO: handle abstract types

		fieldConfig := config.Fields[node.fieldCoord]
		if fieldConfig != nil {
			node.FieldCost += fieldConfig.Weight
			for argName := range node.arguments {
				weight, ok := fieldConfig.ArgumentWeights[argName]
				if ok {
					node.ArgumentsCost += weight
				}
				// What to do if the argument definition itself does not have weight attached,
				// but the type of the argument does have weight attached to it?
				// TODO: arguments should include costs of input object fields
			}
		} else {
			// use the weight of the type returned by this field
			if node.isSimpleType {
				node.FieldCost += config.EnumScalarWeight(node.fieldTypeName)
			} else {
				node.FieldCost += config.ObjectWeight(node.fieldTypeName)
			}
		}

		// Compute multiplier as the maximum of data sources.
		if !node.isListType || fieldConfig == nil {
			node.multiplier = 1
			continue
		}

		multiplier := -1
		for _, slicingArg := range fieldConfig.SlicingArguments {
			argInfo, ok := node.arguments[slicingArg]
			if ok && argInfo.isScalar && argInfo.intValue > 0 && argInfo.intValue > multiplier {
				multiplier = argInfo.intValue
			}
		}
		if multiplier == -1 && fieldConfig.AssumedSize > 0 {
			multiplier = fieldConfig.AssumedSize
		}
		if multiplier > node.multiplier {
			node.multiplier = multiplier
		}
	}

}

// GetTree returns the cost tree
func (c *CostCalculator) GetTree() *CostTree {
	c.tree.Calculate()
	return c.tree
}

// GetTotalCost returns the calculated total cost
func (c *CostCalculator) GetTotalCost() int {
	c.tree.Calculate()
	return c.tree.Total
}
