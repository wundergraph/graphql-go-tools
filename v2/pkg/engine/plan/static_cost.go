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
	// Defaults overrides the global defaults for this data source
	Defaults *CostDefaults

	// FieldConfig maps "TypeName.FieldName" to cost config
	FieldConfig map[string]*FieldCostConfig

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

// GetFieldCost returns the cost for a field, falling back to defaults
func (c *DataSourceCostConfig) GetFieldCost(typeName, fieldName string) int {
	if c == nil {
		return 0
	}

	key := typeName + "." + fieldName
	if fc, ok := c.FieldConfig[key]; ok {
		return fc.Weight
	}

	if c.Defaults != nil {
		return c.Defaults.FieldCost
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

	if c.Defaults != nil {
		return c.Defaults.ArgumentCost
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

	if c.Defaults != nil {
		return c.Defaults.ScalarCost
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

	if c.Defaults != nil {
		return c.Defaults.EnumCost
	}
	return StaticCostDefaults.EnumCost
}

// GetListCost returns the default list cost
func (c *DataSourceCostConfig) GetListCost() int {
	if c == nil {
		return 0
	}

	if c.Defaults != nil {
		return c.Defaults.ListCost
	}
	return StaticCostDefaults.ListCost
}

// GetObjectCost returns the default object cost
func (c *DataSourceCostConfig) GetObjectCost() int {
	if c == nil {
		return 0
	}

	if c.Defaults != nil {
		return c.Defaults.ObjectCost
	}
	return StaticCostDefaults.ObjectCost
}

// CostTreeNode represents a node in the cost calculation tree
type CostTreeNode struct {
	// FieldRef is the AST field reference
	FieldRef int

	// TypeName is the enclosing type name
	TypeName string

	// FieldName is the field name
	FieldName string

	// DataSourceHashes identifies which data sources this field is resolved from
	// A field can be planned on multiple data sources in federation scenarios
	DataSourceHashes []DSHash

	// FieldCost is the base cost of this field (aggregated from all data sources)
	FieldCost int

	// ArgumentsCost is the total cost of all arguments (aggregated from all data sources)
	ArgumentsCost int

	// TypeCost is the cost based on return type (scalar/enum/object)
	TypeCost int

	// Multiplier is applied to child costs (e.g., from "first" or "limit" arguments)
	Multiplier int

	// Children contains child field costs
	Children []*CostTreeNode

	// Parent points to the parent node
	Parent *CostTreeNode

	// isListType and arguments are stored temporarily for deferred cost calculation
	isListType bool
	arguments  []CostFieldArgument
}

// TotalCost calculates the total cost of this node and all descendants
func (n *CostTreeNode) TotalCost() int {
	if n == nil {
		return 0
	}

	// Base cost for this field
	cost := n.FieldCost + n.ArgumentsCost + n.TypeCost

	// Sum children costs
	var childrenCost int
	for _, child := range n.Children {
		childrenCost += child.TotalCost()
	}

	// Apply multiplier to children cost
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
func (c *CostCalculator) EnterField(fieldRef int, typeName, fieldName string, isListType bool, arguments []CostFieldArgument) {
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
		node.Parent = parent
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
func (c *CostCalculator) calculateNodeCosts(node *CostTreeNode) {
	dsHashes := node.DataSourceHashes
	typeName := node.TypeName
	fieldName := node.FieldName
	arguments := node.arguments
	isListType := node.isListType

	// Aggregate costs from all data sources this field is planned on
	// We sum the costs because each data source will be queried
	for _, dsHash := range dsHashes {
		config := c.getCostConfig(dsHash)

		node.FieldCost += config.GetFieldCost(typeName, fieldName)

		// Calculate argument costs for this data source
		for _, arg := range arguments {
			node.ArgumentsCost += config.GetArgumentCost(typeName, fieldName, arg.Name)
		}

		// Calculate multiplier from @listSize directive
		c.calculateListMultiplier(node, config, typeName, fieldName, arguments)

		// Add list cost if this is a list type (only once, take highest)
		if isListType {
			listCost := config.GetListCost()
			if listCost > node.TypeCost {
				node.TypeCost = listCost
			}
		}
	}

	// If no data sources, use default config
	if len(dsHashes) == 0 {
		config := c.getDefaultCostConfig()
		node.FieldCost = config.GetFieldCost(typeName, fieldName)

		for _, arg := range arguments {
			node.ArgumentsCost += config.GetArgumentCost(typeName, fieldName, arg.Name)
		}

		c.calculateListMultiplier(node, config, typeName, fieldName, arguments)

		if isListType {
			node.TypeCost = config.GetListCost()
		}
	}
}

// calculateListMultiplier calculates the list multiplier based on @listSize directive
func (c *CostCalculator) calculateListMultiplier(node *CostTreeNode, config *DataSourceCostConfig, typeName, fieldName string, arguments []CostFieldArgument) {
	slicingArguments := config.GetSlicingArguments(typeName, fieldName)
	assumedSize := config.GetAssumedListSize(typeName, fieldName)

	// If no list size config, nothing to do
	if len(slicingArguments) == 0 && assumedSize == 0 {
		return
	}

	// Check if any slicing argument is provided
	slicingArgFound := false
	for _, arg := range arguments {
		for _, slicingArg := range slicingArguments {
			if arg.Name == slicingArg && arg.IntValue > 0 {
				// Use the highest multiplier
				if arg.IntValue > node.Multiplier {
					node.Multiplier = arg.IntValue
				}
				slicingArgFound = true
			}
		}
	}

	// If no slicing argument found, use assumed size
	if !slicingArgFound && assumedSize > 0 {
		if assumedSize > node.Multiplier {
			node.Multiplier = assumedSize
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

// CostFieldArgument represents a parsed field argument for cost calculation
type CostFieldArgument struct {
	Name     string
	IntValue int
	// Add other value types as needed
}
