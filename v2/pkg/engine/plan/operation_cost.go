package plan

/*

Cost Analysis.

Planning visitor collects information for the costCalculator via EnterField and LeaveField hooks.
Calculator builds a tree of nodes, each node corresponding to the requested field.
After the planning is done, a callee could get a ref to the calculator and request cost calculation.
Cost calculation walks the previously built tree and using variables provided with operation,
estimates the static cost.

https://ibm.github.io/graphql-specs/cost-spec.html

It builds on top of IBM spec for @cost and @listSize directive with a few changes.

* We use Int! for weights instead of floats packed in String!.
* When weight is specified for the type and a field returns the list of that type,
this weight (along with children's costs) is multiplied too.

A few things on the TBD list:

* Support of SizedFields of @listSize
* Weights on fields of InputObjects with recursion
* Weights on arguments of directives

*/

import (
	"fmt"
	"math"
	"strings"

	"github.com/wundergraph/astjson"
)

// We don't allow configuring default weights for enums, scalars and objects.
// But they could be in the future.

const DefaultEnumScalarWeight = 0
const DefaultObjectWeight = 1

// FieldWeight defines cost configuration for a specific field of an object or input object.
type FieldWeight struct {

	// Weight is the cost of this field definition. It could be negative or zero.
	// Should be used only if HasWeight is true.
	Weight int

	// Means that there was weight attached to the field definition.
	HasWeight bool

	// ArgumentWeights maps an argument name to its weight.
	// Location: ARGUMENT_DEFINITION
	ArgumentWeights map[string]int
}

// FieldListSize contains parsed data from the @listSize directive for an object field.
type FieldListSize struct {
	// AssumedSize is the default assumed size when no slicing argument is provided.
	// If 0, the global default list cost is used.
	AssumedSize int

	// SlicingArguments are argument names that control list size (e.g., "first", "last", "limit")
	// The value of these arguments will be used as the multiplier.
	SlicingArguments []string

	// SizedFields are contains field names in the returned object that returns lists.
	// For these lists we estimate the size based on the value of the slicing arguments or AssumedSize.
	SizedFields []string

	// RequireOneSlicingArgument if true, at least one slicing argument must be provided.
	// If false and no slicing argument is provided, AssumedSize is used.
	// It is not used right now since it is required only for validation.
	RequireOneSlicingArgument bool
}

// multiplier returns the multiplier based on arguments and variables.
// It picks the maximum value among slicing arguments, otherwise it tries to use AssumedSize.
// If neither is available, it falls back to defaultListSize.
//
// Does not take into account the SizedFields; TBD later.
func (ls *FieldListSize) multiplier(arguments map[string]ArgumentInfo, vars *astjson.Value, defaultListSize int) int {
	multiplier := -1
	for _, slicingArg := range ls.SlicingArguments {
		arg, ok := arguments[slicingArg]
		if !ok || !arg.isSimple {
			continue
		}

		var value int
		// Argument could be a variable or literal value.
		if arg.hasVariable {
			if vars == nil {
				continue
			}
			if v := vars.Get(arg.varName); v == nil || v.Type() != astjson.TypeNumber {
				continue
			}
			value = vars.GetInt(arg.varName)
		} else if arg.intValue > 0 {
			value = arg.intValue
		}

		if value > 0 && value > multiplier {
			multiplier = value
		}
	}

	if multiplier == -1 && ls.AssumedSize > 0 {
		multiplier = ls.AssumedSize
	}
	if multiplier == -1 {
		multiplier = defaultListSize
	}
	return multiplier
}

// DataSourceCostConfig holds all cost configurations for a data source.
// This data is passed from the composition.
type DataSourceCostConfig struct {
	// Weights maps field coordinate to its weights. Cannot be on fields of interfaces.
	// Location: FIELD_DEFINITION, INPUT_FIELD_DEFINITION
	Weights map[FieldCoordinate]*FieldWeight

	// ListSizes maps field coordinates to their respective list size configurations.
	// Location: FIELD_DEFINITION
	ListSizes map[FieldCoordinate]*FieldListSize

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
		Weights:   make(map[FieldCoordinate]*FieldWeight),
		ListSizes: make(map[FieldCoordinate]*FieldListSize),
		Types:     make(map[string]int),
	}
}

// EnumScalarTypeWeight returns the cost for an enum or scalar types
func (c *DataSourceCostConfig) EnumScalarTypeWeight(enumName string) int {
	if c == nil {
		return 0
	}
	if cost, ok := c.Types[enumName]; ok {
		return cost
	}
	return DefaultEnumScalarWeight
}

// ObjectTypeWeight returns the default object cost
func (c *DataSourceCostConfig) ObjectTypeWeight(name string) int {
	if c == nil {
		return DefaultObjectWeight
	}
	if cost, ok := c.Types[name]; ok {
		return cost
	}
	return DefaultObjectWeight
}

// CostTreeNode represents a node in the cost calculation tree
// Based on IBM GraphQL Cost Specification: https://ibm.github.io/graphql-specs/cost-spec.html
type CostTreeNode struct {
	parent *CostTreeNode

	// dataSourceHashes identifies which data sources resolve this field.
	dataSourceHashes []DSHash

	// children contain child field costs
	children []*CostTreeNode

	// The data below is stored for deferred cost calculation.
	// We populate these fields in EnterField and use them as a source of truth in LeaveField.

	// fieldRef is the AST field reference. Used by the visitor to build the tree.
	fieldRef int

	// Enclosing type name and field name
	fieldCoord FieldCoordinate

	// fieldTypeName contains the name of an unwrapped (named) type that is returned by this field.
	fieldTypeName string

	// implementTypeNames contains the names of all types that implement this interface/union field.
	implementingTypeNames []string

	// arguments contain the values of arguments passed to the field.
	arguments map[string]ArgumentInfo

	jsonPath string // JSON path using aliases too

	returnsListType         bool
	returnsSimpleType       bool
	returnsAbstractType     bool
	isEnclosingTypeAbstract bool
}

func (node *CostTreeNode) maxWeightImplementingField(config *DataSourceCostConfig, fieldName string) *FieldWeight {
	var maxWeight *FieldWeight
	for _, implTypeName := range node.implementingTypeNames {
		// Get the cost config for the field of an implementing type.
		coord := FieldCoordinate{implTypeName, fieldName}
		fieldWeight := config.Weights[coord]

		if fieldWeight != nil {
			if fieldWeight.HasWeight && (maxWeight == nil || fieldWeight.Weight > maxWeight.Weight) {
				maxWeight = fieldWeight
			}
		}
	}
	return maxWeight
}

func (node *CostTreeNode) maxMultiplierImplementingField(config *DataSourceCostConfig, fieldName string, arguments map[string]ArgumentInfo, vars *astjson.Value, defaultListSize int) *FieldListSize {
	var maxMultiplier int
	var maxListSize *FieldListSize
	for _, implTypeName := range node.implementingTypeNames {
		coord := FieldCoordinate{implTypeName, fieldName}
		listSize := config.ListSizes[coord]

		if listSize != nil {
			multiplier := listSize.multiplier(arguments, vars, defaultListSize)
			if maxListSize == nil || multiplier > maxMultiplier {
				maxMultiplier = multiplier
				maxListSize = listSize
			}
		}
	}
	return maxListSize
}

// cost calculates the estimated/actual cost of this node and all descendants.
//
// defaultListSize designates the mode of operation.
// When it is positive, then its value is used as a fallback value of list sizes for the estimated cost.
// When it is negative, then it computes the actual cost. And it uses the actualListSizes map.
// For actual cost, multipliers are computed as averages (totalCount/parentCount).
func (node *CostTreeNode) cost(configs map[DSHash]*DataSourceCostConfig, variables *astjson.Value, defaultListSize int, actualListSizes map[string]int) int {
	if node == nil {
		return 0
	}

	fieldCost, argsCost, directivesCost, multiplier := node.costsAndMultiplier(configs, variables, defaultListSize, actualListSizes)

	// Sum children costs
	var childrenCost int
	for _, child := range node.children {
		childrenCost += child.cost(configs, variables, defaultListSize, actualListSizes)
	}

	// We enforce multiplier=1 for non-list fields.
	if multiplier == 0 && !node.returnsListType {
		multiplier = 1
	}

	cost := argsCost + directivesCost

	// Here we do not follow IBM spec. IBM spec does not use the cost of the object itself
	// in multiplication. It assumes that the weight of the type should be just summed up
	// without regard to the size of the list.
	//
	// We, instead, multiply with field cost.
	// If there is a weight attached to the type that is returned (resolved) by the field,
	// the more objects are requested, the more expensive it should be.
	// This, in turn, has some ambiguity for definitions of the weights for the list types.
	// "A: [Obj] @cost(weight: 5)" means that the cost of the field is 5 for each object in the list.
	// "type Object @cost(weight: 5) { ... }" does exactly the same thing.
	// Weight defined on a field has priority over the weight defined on a type.
	cost += int(math.RoundToEven(float64(childrenCost+fieldCost) * multiplier))
	if cost < 0 {
		cost = 0
	}
	return cost
}

// costsAndMultiplier returns the cost values for a node based on its data sources.
//
// For this node we sum weights of the field or its returned type for all the data sources.
// Each data source can have its own cost configuration. If we plan field on two data sources,
// it means more work for the router: we should sum the costs.
//
// fieldCost is the weight of this field or its returned type
// argsCost is the sum of argument weights and input fields used on this field.
// Weights on directives ignored for now.
//
// defaultListSize designates the mode of operation.
// When it is positive, then its value is used as a fallback value of list sizes for the estimated cost.
// When it is negative, then it computes the actual cost. And it uses the actualListSizes map.
//
// When estimating cost, it picks the highest multiplier among different data sources.
// Also, it picks the maximum field weight of implementing types and then
// the maximum among slicing arguments.
func (node *CostTreeNode) costsAndMultiplier(configs map[DSHash]*DataSourceCostConfig, variables *astjson.Value, defaultListSize int, actualListSizes map[string]int) (fieldCost, argsCost, directiveCost int, multiplier float64) {
	if len(node.dataSourceHashes) <= 0 {
		// no data source is responsible for this field
		return
	}

	parent := node.parent
	fieldCost = 0
	argsCost = 0
	directiveCost = 0
	multiplier = 0

	estimated := defaultListSize > 0

	for _, dsHash := range node.dataSourceHashes {
		dsCostConfig, ok := configs[dsHash]
		if !ok || dsCostConfig == nil {
			dsCostConfig = &DataSourceCostConfig{}
			// Save it for later use by other fields:
			configs[dsHash] = dsCostConfig
		}

		fieldWeight := dsCostConfig.Weights[node.fieldCoord]
		listSize := dsCostConfig.ListSizes[node.fieldCoord]
		// The cost directive is not allowed on fields in an interface.
		// The cost of a field on an interface can be calculated based on the costs of
		// the corresponding field on each concrete type implementing that interface,
		// either directly or indirectly through other interfaces.
		if fieldWeight != nil && node.isEnclosingTypeAbstract && parent.returnsAbstractType {
			// Composition should not let interface fields have weights, so we assume that
			// the enclosing type is concrete.
			fmt.Printf("WARNING: cost directive on field %v of interface %v\n", node.fieldCoord, parent.fieldCoord)
		}
		if node.isEnclosingTypeAbstract && parent.returnsAbstractType {
			// This field is part of the enclosing interface/union.
			// We look into implementing types and find the max-weighted field.
			// Found fieldWeight can be used for all the calculations.
			fieldWeight = parent.maxWeightImplementingField(dsCostConfig, node.fieldCoord.FieldName)
			// If this field has listSize defined, then do not look into implementing types.
			if listSize == nil && node.returnsListType {
				listSize = parent.maxMultiplierImplementingField(dsCostConfig, node.fieldCoord.FieldName, node.arguments, variables, defaultListSize)
			}
		}

		if fieldWeight != nil && fieldWeight.HasWeight {
			fieldCost += fieldWeight.Weight
		} else {
			// Use the weight of the type returned by this field
			switch {
			case node.returnsSimpleType:
				fieldCost += dsCostConfig.EnumScalarTypeWeight(node.fieldTypeName)
			case node.returnsAbstractType:
				// For the abstract field, find the max weight among all implementing types
				maxWeight := 0
				for _, implTypeName := range node.implementingTypeNames {
					weight := dsCostConfig.ObjectTypeWeight(implTypeName)
					if weight > maxWeight {
						maxWeight = weight
					}
				}
				fieldCost += maxWeight
			default:
				fieldCost += dsCostConfig.ObjectTypeWeight(node.fieldTypeName)
			}
		}

		for argName, arg := range node.arguments {
			if fieldWeight != nil {
				if weight, ok := fieldWeight.ArgumentWeights[argName]; ok {
					argsCost += weight
					continue
				}
			}
			// Take into account the type of the argument.
			// If the argument definition itself does not have weight attached,
			// but the type of the argument does have weight attached to it.
			if arg.isSimple {
				argsCost += dsCostConfig.EnumScalarTypeWeight(arg.typeName)
			} else if arg.isInputObject {
				// TODO: arguments should include costs of input object fields
			} else {
				argsCost += dsCostConfig.ObjectTypeWeight(arg.typeName)
			}

		}

		// Return early, since we do not support sizedFields yet. That parameter means
		// that lisSize could be applied to fields that return non-lists.
		if !node.returnsListType {
			continue
		}

		// Compute multiplier as the maximum of data sources.
		if estimated && listSize != nil {
			localMultiplier := float64(listSize.multiplier(node.arguments, variables, defaultListSize))
			// If this node returns a list of abstract types, then it could have listSize defined.
			// Spec allows defining listSize on the fields of interfaces.
			if localMultiplier > multiplier {
				multiplier = localMultiplier
			}
		}

	}

	if !node.returnsListType {
		return
	}
	if !estimated { // actual or dynamic
		totalCount, ok := actualListSizes[node.jsonPath]
		if ok && totalCount != 0 {
			parentCount := 1
			if lastDot := strings.LastIndex(node.jsonPath, "."); lastDot != -1 {
				parentPath := node.jsonPath[:lastDot]
				if pc, found := actualListSizes[parentPath]; found && pc > 0 {
					parentCount = pc
				}
			}
			// We compute average to avoid double counting for nested lists
			multiplier = float64(totalCount) / float64(parentCount)
		} else {
			// No items in the list mean that we completely disregard children of the list.
			// That is not very accurate because we called the resolver of this field anyway.
			// So we need to account for that by setting multiplier to 1.
			multiplier = 1.0
		}
		return
	}
	if multiplier == 0 {
		multiplier = float64(defaultListSize)
	}
	return
}

type ArgumentInfo struct {
	intValue int

	// The name of an unwrapped type.
	typeName string

	// If argument is passed an input object, we want to gather counts
	// for all the field coordinates with non-null values used in the argument.
	// TBD later when input objects are supported.
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

	isSimple bool

	// When the argument points to a variable, it contains the name of the variable.
	hasVariable bool

	// The name of the variable that has value for this argument.
	varName string
}

// CostCalculator manages cost calculation during AST traversal
type CostCalculator struct {
	// tree points to the root of the complete cost tree. Calculator tree is built once per query,
	// then it is cached as part of the plan cache and
	// not supposed to change again even during lifetime of a process.
	tree *CostTreeNode
}

// NewCostCalculator creates a new cost calculator. The defaultListSize is floored to 1.
func NewCostCalculator() *CostCalculator {
	c := CostCalculator{}
	return &c
}

// EstimateCost returns the calculated total static cost.
// config should be static per process or instance. variables could change between requests.
func (c *CostCalculator) EstimateCost(config Configuration, variables *astjson.Value) int {
	// costConfigs maps data source hash to its cost configuration. At the runtime we do not change
	// this at all. It could be set once per router process.
	costConfigs := make(map[DSHash]*DataSourceCostConfig)
	for _, ds := range config.DataSources {
		if costConfig := ds.GetCostConfig(); costConfig != nil {
			costConfigs[ds.Hash()] = costConfig
		}
	}
	defaultListSize := config.StaticCostDefaultListSize
	if defaultListSize < 1 {
		defaultListSize = 1
	}
	return c.tree.cost(costConfigs, variables, defaultListSize, nil)
}

const (
	actualCostMode = -1 // -1 signals actual mode
)

func (c *CostCalculator) ActualCost(config Configuration, vars *astjson.Value, actualListSizes map[string]int) int {
	costConfigs := make(map[DSHash]*DataSourceCostConfig)
	for _, ds := range config.DataSources {
		if costConfig := ds.GetCostConfig(); costConfig != nil {
			costConfigs[ds.Hash()] = costConfig
		}
	}
	// most probably variables are not used for actual cost calculation. check that
	return c.tree.cost(costConfigs, vars, actualCostMode, actualListSizes)
}

// DebugPrint prints the cost tree structure for debugging purposes.
// It shows each node's field coordinate, costs, multipliers, and computed totals.
func (c *CostCalculator) DebugPrint(config Configuration, variables *astjson.Value, actualListSizes map[string]int) string {
	if c.tree == nil || len(c.tree.children) == 0 {
		return "<empty cost tree>"
	}
	var sb strings.Builder
	costConfigs := make(map[DSHash]*DataSourceCostConfig)
	for _, ds := range config.DataSources {
		if costConfig := ds.GetCostConfig(); costConfig != nil {
			costConfigs[ds.Hash()] = costConfig
		}
	}
	defaultListSize := config.StaticCostDefaultListSize
	if defaultListSize < 1 {
		defaultListSize = 1
	}
	if actualListSizes != nil {
		defaultListSize = -1
		sb.WriteString("Actual Cost Tree Debug\n")
		sb.WriteString("======================\n")
	} else {
		sb.WriteString("Estimated Cost Tree Debug\n")
		sb.WriteString("=========================\n")
	}
	c.tree.children[0].debugPrint(&sb, costConfigs, variables, defaultListSize, actualListSizes, 0)
	return sb.String()
}

// debugPrint recursively prints a node and its children with indentation.
func (node *CostTreeNode) debugPrint(sb *strings.Builder, configs map[DSHash]*DataSourceCostConfig, variables *astjson.Value, defaultListSize int, actualListSizes map[string]int, depth int) {
	// implementation is a bit crude and redundant, we could skip calculating nodes all over again.
	// but it should suffice for debugging tests.
	if node == nil {
		return
	}

	indent := strings.Repeat("    ", depth)

	fieldInfo := fmt.Sprintf("%s.%s", node.fieldCoord.TypeName, node.fieldCoord.FieldName)

	fmt.Fprintf(sb, "%s* %s", indent, fieldInfo)

	if node.fieldTypeName != "" {
		fmt.Fprintf(sb, " : %s", node.fieldTypeName)
	}

	var flags []string
	if node.returnsListType {
		flags = append(flags, "list")
	}
	if node.returnsAbstractType {
		flags = append(flags, "abstract")
	}
	if node.returnsSimpleType {
		flags = append(flags, "simple")
	}
	if len(flags) > 0 {
		fmt.Fprintf(sb, " [%s]", strings.Join(flags, ","))
	}
	if len(node.jsonPath) > 0 {
		fmt.Fprintf(sb, " : path=%s", node.jsonPath)
	}
	sb.WriteString("\n")

	// Compute costs for this node to display in debug output
	fieldCost, argsCost, dirsCost, multiplier := node.costsAndMultiplier(configs, variables, defaultListSize, actualListSizes)
	if fieldCost != 0 || argsCost != 0 || dirsCost != 0 || multiplier != 0 {
		fmt.Fprintf(sb, "%s  fieldCost=%d", indent, fieldCost)

		if argsCost > 0 {
			fmt.Fprintf(sb, ", argsCost=%d", argsCost)
		}
		if dirsCost > 0 {
			fmt.Fprintf(sb, ", directivesCost=%d", dirsCost)
		}
		fmt.Fprintf(sb, ", multiplier=%.2f", multiplier)

		// Show data sources
		if len(node.dataSourceHashes) > 0 {
			fmt.Fprintf(sb, ", dataSources=%v", node.dataSourceHashes)
		}
		sb.WriteString("\n")
	}

	if len(node.arguments) > 0 {
		var argStrs []string
		for name, arg := range node.arguments {
			if arg.hasVariable {
				argStrs = append(argStrs, fmt.Sprintf("%s=$%s", name, arg.varName))
			} else if arg.isSimple {
				argStrs = append(argStrs, fmt.Sprintf("%s=%d", name, arg.intValue))
			} else {
				argStrs = append(argStrs, fmt.Sprintf("%s=<obj>", name))
			}
		}
		fmt.Fprintf(sb, "%s  args: {%s}\n", indent, strings.Join(argStrs, ", "))
	}

	if len(node.implementingTypeNames) > 0 {
		fmt.Fprintf(sb, "%s  implements: [%s]\n", indent, strings.Join(node.implementingTypeNames, ", "))
	}

	// This is somewhat redundant, but it should not be used in production.
	// If there is a need to present cost tree to the user,
	// printing should be embedded into the tree calculation process.
	subtreeCost := node.cost(configs, variables, defaultListSize, actualListSizes)
	fmt.Fprintf(sb, "%s  subCost=%d\n", indent, subtreeCost)

	for _, child := range node.children {
		child.debugPrint(sb, configs, variables, defaultListSize, actualListSizes, depth+1)
	}
}
