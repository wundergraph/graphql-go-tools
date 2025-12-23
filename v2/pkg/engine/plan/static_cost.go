package plan

// StaticCostDefaults contains default cost values when no specific costs are configured
var StaticCostDefaults = WeightDefaults{
	Field:  1,
	Scalar: 0,
	Enum:   0,
	Object: 1,
	List:   10, // The assumed maximum size of a list for fields that return lists.
}

// WeightDefaults defines default cost values for different GraphQL elements
type WeightDefaults struct {
	Field  int
	Scalar int
	Enum   int
	Object int
	List   int
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
	// Fields maps field coordinate to its cost config.
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
	// mutual recursion between them that complicates cost calculation.
	// We avoid them intentionally in the first iteration.
}

// NewDataSourceCostConfig creates a new cost config with defaults
func NewDataSourceCostConfig() *DataSourceCostConfig {
	return &DataSourceCostConfig{
		Fields: make(map[FieldCoordinate]*FieldCostConfig),
		Types:  make(map[string]int),
	}
}

func (c *DataSourceCostConfig) GetFieldCostConfig(typeName, fieldName string) *FieldCostConfig {
	if c == nil {
		return nil
	}
	return c.Fields[FieldCoordinate{typeName, fieldName}]
}

// ScalarWeight returns the cost for a scalar type
func (c *DataSourceCostConfig) ScalarWeight(scalarName string) int {
	if c == nil {
		return 0
	}
	if cost, ok := c.Types[scalarName]; ok {
		return cost
	}
	return StaticCostDefaults.Scalar
}

// EnumWeight returns the cost for an enum type
func (c *DataSourceCostConfig) EnumWeight(enumName string) int {
	if c == nil {
		return 0
	}
	if cost, ok := c.Types[enumName]; ok {
		return cost
	}
	return StaticCostDefaults.Enum
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

func (c *DataSourceCostConfig) GetDefaultListCost() int {
	return 10
}

// CostTreeNode represents a node in the cost calculation tree
// Based on IBM GraphQL Cost Specification: https://ibm.github.io/graphql-specs/cost-spec.html
type CostTreeNode struct {
	// fieldRef is the AST field reference
	fieldRef int

	// Enclosing type name and field name
	fieldCoord FieldCoordinate

	// dataSourceHashes identifies which data sources this field is resolved from
	dataSourceHashes []DSHash

	// FieldCost is the weight of this field from @cost directive
	FieldCost int

	// ArgumentsCost is the sum of argument weights and input fields used on each directive
	ArgumentsCost int

	DirectivesCost int

	// Multiplier is the list size multiplier from @listSize directive
	// Applied to children costs for list fields
	Multiplier int

	// Children contain child field costs
	Children []*CostTreeNode

	// The data below is stored for deferred cost calculation.

	// What is the name of an unwrapped (named) type that is returned by this field?
	fieldTypeName string

	isListType bool

	// arguments contain the values of arguments passed to the field
	arguments map[string]ArgumentInfo
}

type ArgumentInfo struct {
	intValue int

	// The name of an unwrapped type.
	typeName string

	// isInputObject is true for an input object passed to the argument,
	// otherwise the argument is Scalar or Enum.
	isInputObject bool

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
	isScalar    bool
}

// TotalCost calculates the total cost of this node and all descendants
// Per IBM spec: total = field_weight + argument_weights + (children_total * multiplier)
func (n *CostTreeNode) TotalCost() int {
	if n == nil {
		return 0
	}

	// TODO: negative sum should be rounded up to zero
	cost := n.FieldCost + n.ArgumentsCost + n.DirectivesCost

	// Sum children (fields) costs
	var childrenCost int
	for _, child := range n.Children {
		childrenCost += child.TotalCost()
	}

	// Apply multiplier to children cost (for list fields)
	multiplier := n.Multiplier
	if multiplier == 0 {
		multiplier = 1
	}
	cost += childrenCost * multiplier

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

	// enabled controls whether cost calculation is active
	enabled bool
}

// NewCostCalculator creates a new cost calculator
func NewCostCalculator() *CostCalculator {
	tree := &CostTree{
		Root: &CostTreeNode{
			fieldCoord: FieldCoordinate{"_none", "_root"},
			Multiplier: 1,
		},
	}
	c := CostCalculator{
		stack:       make([]*CostTreeNode, 0, 16),
		costConfigs: make(map[DSHash]*DataSourceCostConfig),
		tree:        tree,
		enabled:     false,
	}
	c.stack = append(c.stack, c.tree.Root)

	return &c
}

// Enable activates cost calculation
func (c *CostCalculator) Enable() {
	c.enabled = true
}

// SetDataSourceCostConfig sets the cost config for a specific data source
func (c *CostCalculator) SetDataSourceCostConfig(dsHash DSHash, config *DataSourceCostConfig) {
	c.costConfigs[dsHash] = config
}

// SetDefaultCostConfig sets the default cost config
func (c *CostCalculator) SetDefaultCostConfig(config *DataSourceCostConfig) {
	c.defaultConfig = config
}

// getCostConfig returns the cost config for a specific data source hash
func (c *CostCalculator) getCostConfig(dsHash DSHash) *DataSourceCostConfig {
	if config, ok := c.costConfigs[dsHash]; ok {
		return config
	}
	return c.getDefaultCostConfig()
}

// getDefaultCostConfig returns the default cost config when no specific data source is available
func (c *CostCalculator) getDefaultCostConfig() *DataSourceCostConfig {
	if c.defaultConfig != nil {
		return c.defaultConfig
	}
	// Return a dummy config with defaults
	return &DataSourceCostConfig{}
}

// IsEnabled returns whether cost calculation is enabled
func (c *CostCalculator) IsEnabled() bool {
	return c.enabled
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
func (c *CostCalculator) EnterField(fieldRef int, coord FieldCoordinate, namedTypeName string,
	isListType bool, arguments map[string]ArgumentInfo) {
	if !c.enabled {
		return
	}

	// Create skeleton cost node. Costs will be calculated in LeaveField
	node := &CostTreeNode{
		fieldRef:      fieldRef,
		fieldCoord:    coord,
		Multiplier:    1,
		fieldTypeName: namedTypeName,
		isListType:    isListType,
		arguments:     arguments,
	}

	// Attach to parent
	parent := c.CurrentNode()
	if parent != nil {
		parent.Children = append(parent.Children, node)
	}

	// Push onto stack
	c.stack = append(c.stack, node)
}

// LeaveField is called when leaving a field during AST traversal.
// This is where we calculate costs because fieldPlanners data is now available.
func (c *CostCalculator) LeaveField(fieldRef int, dsHashes []DSHash) {
	if !c.enabled {
		return
	}

	// Find the current node (should match fieldRef)
	if len(c.stack) <= 1 { // Keep root on stack
		return
	}

	current := c.stack[len(c.stack)-1]
	if current.fieldRef != fieldRef {
		return
	}

	// Now calculate costs with the data source information
	current.dataSourceHashes = dsHashes
	c.calculateNodeCosts(current)

	// Pop from stack
	c.stack = c.stack[:len(c.stack)-1]
}

// calculateNodeCosts fills in the cost values for a node based on its data sources
// calculateNodeCosts implements IBM GraphQL Cost Specification
// See: https://ibm.github.io/graphql-specs/cost-spec.html#sec-Field-Cost
func (c *CostCalculator) calculateNodeCosts(node *CostTreeNode) {
	// Get the cost config (use first data source config, or default)
	var config *DataSourceCostConfig
	if len(node.dataSourceHashes) > 0 {
		config = c.getCostConfig(node.dataSourceHashes[0])
	} else {
		config = c.getDefaultCostConfig()
	}

	fieldConfig := config.Fields[node.fieldCoord]
	if fieldConfig != nil {
		node.FieldCost = fieldConfig.Weight
	} else {
		// use the weight of the type returned by this field
		if typeWeight, ok := config.Types[node.fieldTypeName]; ok {
			node.FieldCost = typeWeight
		}
	}

	// TODO: check how we fill node.arguments
	for argName := range node.arguments {
		weight, ok := fieldConfig.ArgumentWeights[argName]
		if ok {
			node.ArgumentsCost += weight
		}
		// TODO: arguments should include costs of input object fields
	}

	// Compute multiplier
	if !node.isListType {
		node.Multiplier = 1
		return
	}

	if fieldConfig == nil {
		node.Multiplier = 1
		return
	}
	node.Multiplier = 0
	for _, slicingArg := range fieldConfig.SlicingArguments {
		argInfo, ok := node.arguments[slicingArg]
		if ok && argInfo.isScalar && argInfo.intValue > node.Multiplier{
				node.Multiplier = argInfo.intValue
		}
	}
	if node.Multiplier == 0 && fieldConfig.AssumedSize > 0 {
		node.Multiplier = fieldConfig.AssumedSize
		return
	}
	node.Multiplier = StaticCostDefaults.List

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

// CostFieldArgument represents a parsed field argument for cost calculation
type CostFieldArgument struct {
	Name     string
	IntValue int
	// Add other value types as needed
}
