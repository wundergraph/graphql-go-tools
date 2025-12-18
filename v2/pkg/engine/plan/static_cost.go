package plan

// StaticCostDefaults contains default cost values when no specific costs are configured
var StaticCostDefaults = CostDefaults{
	FieldCost:    1,
	ArgumentCost: 0,
	ScalarCost:   0,
	EnumCost:     0,
	ObjectCost:   1,
	ListCost:     10, // The assumed maximum size of a list for fields that return lists.
}

// CostDefaults defines default cost values for different GraphQL elements
type CostDefaults struct {
	FieldCost    int
	ArgumentCost int
	ScalarCost   int
	EnumCost     int
	ObjectCost   int
	ListCost     int
}

// FieldCostConfig defines cost configuration for a specific field
// Includes @listSize directive fields for list cost calculation
type FieldCostConfig struct {
	Weight int

	// ArgumentWeights maps argument name to its weight/cost
	ArgumentWeights map[string]int

	// AssumedSize is the default assumed size when no slicing argument is provided (from @listSize)
	// If 0, the global default list cost is used
	AssumedSize int

	// SlicingArguments are argument names that control list size (e.g., "first", "last", "limit")
	// The value of these arguments will be used as the multiplier (from @listSize)
	SlicingArguments []string

	// SizedFields are field names that return the actual size of the list (from @listSize)
	// These can be used for more accurate, actual cost estimation
	SizedFields []string

	// RequireOneSlicingArgument if true, at least one slicing argument must be provided (from @listSize)
	// If false and no slicing argument is provided, AssumedSize is used
	RequireOneSlicingArgument bool
}

// DataSourceCostConfig holds all cost configurations for a data source
type DataSourceCostConfig struct {
	// FieldConfig maps "TypeName.FieldName" to cost config
	FieldConfig map[string]*FieldCostConfig

	// Object Weights include all object, scalar and enum definitions.

	// ScalarWeights maps scalar type name to weight
	ScalarWeights map[string]int

	// EnumWeights maps enum type name to weight
	EnumWeights map[string]int
}

// NewDataSourceCostConfig creates a new cost config with defaults
func NewDataSourceCostConfig() *DataSourceCostConfig {
	return &DataSourceCostConfig{
		FieldConfig:   make(map[string]*FieldCostConfig),
		ScalarWeights: make(map[string]int),
		EnumWeights:   make(map[string]int),
	}
}

func (c *DataSourceCostConfig) GetFieldCostConfig(typeName, fieldName string) *FieldCostConfig {
	if c == nil {
		return nil
	}

	key := typeName + "." + fieldName
	return c.FieldConfig[key]
}

// GetFieldCost returns the cost for a field, falling back to defaults
func (c *DataSourceCostConfig) GetFieldCost(typeName, fieldName string) int {
	if c == nil {
		return 0
	}

	key := typeName + "." + fieldName
	if fc, ok := c.FieldConfig[key]; ok {
		return fc.Weight
	}

	return StaticCostDefaults.FieldCost
}

// GetSlicingArguments returns the slicing argument names for a field
// These are arguments that control list size (e.g., "first", "last", "limit")
func (c *DataSourceCostConfig) GetSlicingArguments(typeName, fieldName string) []string {
	if c == nil {
		return nil
	}

	key := typeName + "." + fieldName
	if fc, ok := c.FieldConfig[key]; ok {
		return fc.SlicingArguments
	}
	return nil
}

// GetAssumedListSize returns the assumed list size for a field when no slicing argument is provided
func (c *DataSourceCostConfig) GetAssumedListSize(typeName, fieldName string) int {
	if c == nil {
		return 0
	}

	key := typeName + "." + fieldName
	if fc, ok := c.FieldConfig[key]; ok {
		return fc.AssumedSize
	}
	return 0
}

// GetArgumentCost returns the cost for an argument, falling back to defaults
func (c *DataSourceCostConfig) GetArgumentCost(typeName, fieldName, argName string) int {
	if c == nil {
		return 0
	}

	key := typeName + "." + fieldName
	if fc, ok := c.FieldConfig[key]; ok && fc.ArgumentWeights != nil {
		if weight, ok := fc.ArgumentWeights[argName]; ok {
			return weight
		}
	}

	return StaticCostDefaults.ArgumentCost
}

// GetScalarCost returns the cost for a scalar type
func (c *DataSourceCostConfig) GetScalarCost(scalarName string) int {
	if c == nil {
		return 0
	}

	if cost, ok := c.ScalarWeights[scalarName]; ok {
		return cost
	}

	return StaticCostDefaults.ScalarCost
}

// GetEnumCost returns the cost for an enum type
func (c *DataSourceCostConfig) GetEnumCost(enumName string) int {
	if c == nil {
		return 0
	}

	if cost, ok := c.EnumWeights[enumName]; ok {
		return cost
	}

	return StaticCostDefaults.EnumCost
}

// GetObjectCost returns the default object cost
func (c *DataSourceCostConfig) GetObjectCost() int {
	if c == nil {
		return 0
	}

	return StaticCostDefaults.ObjectCost
}

func (c *DataSourceCostConfig) GetDefaultListCost() int {
	return 10
}

// CostTreeNode represents a node in the cost calculation tree
// Based on IBM GraphQL Cost Specification: https://ibm.github.io/graphql-specs/cost-spec.html
type CostTreeNode struct {
	// FieldRef is the AST field reference
	FieldRef int

	// TypeName is the enclosing type name
	TypeName string

	// FieldName is the field name
	FieldName string

	// DataSourceHashes identifies which data sources this field is resolved from
	DataSourceHashes []DSHash

	// FieldCost is the weight of this field from @cost directive
	FieldCost int

	// ArgumentsCost is the sum of argument weights and input fields used on each directive
	ArgumentsCost int

	DirectivesCost int

	// Multiplier is the list size multiplier from @listSize directive
	// Applied to children costs for list fields
	Multiplier int

	// Children contains child field costs
	Children []*CostTreeNode

	// isListType and arguments are stored temporarily for deferred cost calculation
	isListType bool
	arguments  map[string]int
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
			FieldName:  "_root",
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
func (c *CostCalculator) EnterField(fieldRef int, typeName, fieldName string, isListType bool, arguments map[string]int) {
	if !c.enabled {
		return
	}

	// Create skeleton cost node - costs will be calculated in LeaveField
	node := &CostTreeNode{
		FieldRef:   fieldRef,
		TypeName:   typeName,
		FieldName:  fieldName,
		Multiplier: 1,
		isListType: isListType,
		arguments:  arguments,
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
	if current.FieldRef != fieldRef {
		return
	}

	// Now calculate costs with the data source information
	current.DataSourceHashes = dsHashes
	c.calculateNodeCosts(current)

	// Pop from stack
	c.stack = c.stack[:len(c.stack)-1]
}

// calculateNodeCosts fills in the cost values for a node based on its data sources
// calculateNodeCosts implements IBM GraphQL Cost Specification
// See: https://ibm.github.io/graphql-specs/cost-spec.html#sec-Field-Cost
func (c *CostCalculator) calculateNodeCosts(node *CostTreeNode) {
	typeName := node.TypeName
	fieldName := node.FieldName

	// Get the cost config (use first data source config, or default)
	var config *DataSourceCostConfig
	if len(node.DataSourceHashes) > 0 {
		config = c.getCostConfig(node.DataSourceHashes[0])
	} else {
		config = c.getDefaultCostConfig()
	}

	node.FieldCost = config.GetFieldCost(typeName, fieldName)

	for argName := range node.arguments {
		node.ArgumentsCost += config.GetArgumentCost(typeName, fieldName, argName)
		// TODO: arguments should include costs of input object fields
	}

	// TODO: Directives Cost should includes the weights of all its arguments

	// TODO: arguments, directives and fields of input object are mutually recursive,
	// we should recurse on them and sum all of possible values.

	// Compute multiplier
	if !node.isListType {
		node.Multiplier = 1
		return
	}

	fieldCostConfig := config.GetFieldCostConfig(typeName, fieldName)
	node.Multiplier = 0
	for _, slicingArg := range fieldCostConfig.SlicingArguments {
		if argValue, ok := node.arguments[slicingArg]; ok && argValue > 0 {
			if argValue > node.Multiplier {
				node.Multiplier = argValue
			}
		}
	}
	if node.Multiplier == 0 && fieldCostConfig.AssumedSize > 0 {
		node.Multiplier = fieldCostConfig.AssumedSize
		return
	}
	node.Multiplier = config.GetDefaultListCost()

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
