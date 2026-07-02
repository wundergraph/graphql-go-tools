package plan

/*

Cost Control.

Planning visitor collects information for the costCalculator via EnterField and LeaveField hooks.
Calculator builds a tree of nodes, each node corresponding to the requested field.
After the planning is done, a callee could get a ref to the calculator and request cost calculation.
Cost calculation walks the previously built tree and using variables provided with operation,
estimates the static cost.

https://ibm.github.io/graphql-specs/cost-spec.html

It builds on top of IBM spec for @cost and @listSize directive with a few changes.

* We use the Int! type for weights.
* When weight is specified for the type and a field returns the list of that type,
this weight (along with children's costs) is multiplied too.

Weights on arguments of directives are supported. If an argument is of InputObject's type,
then the weight from its fields is not counted.

*/

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// We don't allow configuring default weights for enums, scalars and objects.
// But they could be in the future.

const (
	defaultEnumScalarWeight = 0
	defaultObjectWeight     = 1

	undefinedMultiplier = -1

	actualCostMode = -1 // -1 signals actual mode, otherwise estimation mode.
)

// FieldCost defines cost configuration for a specific field of an object or input object.
type FieldCost struct {

	// Weight is the cost of this field definition. It could be negative or zero.
	// Should be used only if HasWeight is true.
	Weight int

	// Means that there was weight attached to the field definition.
	HasWeight bool

	// ArgumentWeights maps an argument name to its weight.
	// Location: ARGUMENT_DEFINITION
	ArgumentWeights map[string]int

	// DirectiveArgumentWeights maps a directive.argument coords to its weight.
	// Populated by composition from @cost on directive argument definitions.
	DirectiveArgumentWeights map[string]int
}

// FieldListSize contains parsed data from the @listSize directive for an object field.
type FieldListSize struct {
	// AssumedSize is the default assumed size when no slicing argument is provided.
	// If 0, the global default list cost is used.
	AssumedSize int

	// SlicingArguments are argument names that control list size
	// (e.g., "first", "last", "pagination.limit.first").
	// The value of these arguments will be used as the multiplier.
	SlicingArguments []string

	// SizedFields are contains field names in the returned object that returns lists.
	// For these lists we estimate the size based on the value of the slicing arguments or AssumedSize.
	SizedFields []string

	// RequireOneSlicingArgument enforces a check that exactly one slicing argument must be provided.
	// When set to false or no slicing arguments are provided, the check is skipped.
	RequireOneSlicingArgument bool

	// SlicingArgumentDefaults holds the leaf Int default value declared in
	// the schema for each slicing argument path. Per GraphQL, an omitted
	// slicing argument with a default here is treated as effectively provided with that value,
	// both for `RequireOneSlicingArgument` validation and as a cost multiplier for lists.
	SlicingArgumentDefaults map[string]int
}

// multiplier returns the multiplier based on arguments and variables.
// It picks the maximum value among slicing arguments, otherwise it tries to use AssumedSize.
// If neither is available, it falls back to defaultListSize.
func (ls *FieldListSize) multiplier(args map[string]ArgumentInfo, vars resolve.VariablesView, defaultListSize int) int {
	multiplier := undefinedMultiplier
	for _, slicingArg := range ls.SlicingArguments {
		value, found := ls.resolveSlicingArg(slicingArg, args, vars)
		if found && value > 0 && value > multiplier {
			multiplier = value
		}
	}

	if multiplier == undefinedMultiplier {
		if ls.AssumedSize > 0 {
			multiplier = ls.AssumedSize
		} else {
			multiplier = defaultListSize
		}
	}
	return multiplier
}

// resolveSlicingArg resolves the value of a slicing argument from arguments/variables.
// It falls back to SlicingArgumentDefaults when no value is provided.
// The slicingArg may be a simple argument name or a dot-path into an input object argument.
// An explicitly provided [null] value in variables overrides the default value in schema.
func (ls *FieldListSize) resolveSlicingArg(slicingArg string, args map[string]ArgumentInfo, vars resolve.VariablesView) (int, bool) {
	defaultValue, hasDefault := ls.SlicingArgumentDefaults[slicingArg]
	if strings.Contains(slicingArg, ".") {
		value := extractSlicingArgValue(slicingArg, args, vars)
		if value == nil {
			return defaultValue, hasDefault
		}
		if value.Type() == astjson.TypeNumber {
			return value.GetInt(), true
		}
		// TypeNull value should not lead to the defaults being used.
		return 0, false
	}
	arg, found := args[slicingArg]
	if !found {
		return defaultValue, hasDefault
	}
	if !arg.hasVariable {
		return 0, false
	}
	value := vars.Get(arg.varName)
	if value == nil {
		return defaultValue, hasDefault
	}
	if value.Type() == astjson.TypeNumber {
		return value.GetInt(), true
	}
	return 0, false
}

// extractSlicingArgValue extracts a value from variables using slicingArg that contains
// a string in the format: "<argumentName>.<inputField1>.<inputField2>..."
func extractSlicingArgValue(slicingArg string, args map[string]ArgumentInfo, vars resolve.VariablesView) *astjson.Value {
	path := strings.Split(slicingArg, ".")
	inputArg := path[0]
	arg, found := args[inputArg]
	if !found || !arg.hasVariable || !arg.isInputObject {
		return nil
	}
	value := vars.Get(arg.varName)
	// Walk nested keys manually rather than passing the full path to vars.Get:
	// we must return an explicit TypeNull encountered mid-path;
	// the caller can distinguish "explicit null in variables" (overrides schema default)
	// from "missing" (uses schema default). Calling Get on a null collapses both cases.
	for _, key := range path[1:] {
		if value == nil || value.Type() == astjson.TypeNull {
			return value
		}
		value = value.Get(key)
	}
	return value
}

// DataSourceCostConfig holds all cost configurations for a data source.
// This data is passed from the composition.
type DataSourceCostConfig struct {
	// Weights maps field coordinate to its weights. Cannot be on fields of interfaces.
	// Location: FIELD_DEFINITION, INPUT_FIELD_DEFINITION
	Weights map[FieldCoordinate]*FieldCost

	// ListSizes maps field coordinates to their respective list size configurations.
	// Location: FIELD_DEFINITION
	ListSizes map[FieldCoordinate]*FieldListSize

	// Types maps TypeName to the weight of the object, scalar or enum definition.
	// If TypeName is not present, the default value for Enums and Scalars is 0, otherwise 1.
	// Weight assigned to the field or argument definitions overrides the weight of type definition.
	// Location: ENUM, OBJECT, SCALAR
	Types map[string]int
}

// NewDataSourceCostConfig creates a new cost config with defaults
func NewDataSourceCostConfig() *DataSourceCostConfig {
	return &DataSourceCostConfig{
		Weights:   make(map[FieldCoordinate]*FieldCost),
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
	return defaultEnumScalarWeight
}

// ObjectTypeWeight returns the default object cost
func (c *DataSourceCostConfig) ObjectTypeWeight(name string) int {
	if c == nil {
		return defaultObjectWeight
	}
	if cost, ok := c.Types[name]; ok {
		return cost
	}
	return defaultObjectWeight
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
	fieldCoords FieldCoordinate

	// fieldTypeName contains the name of an unwrapped (named) type that is returned by this field.
	fieldTypeName string

	// implementingTypeNames contains the names of all types that implement this interface/union field.
	implementingTypeNames []string

	// arguments contain the values of arguments passed to the field.
	arguments map[string]ArgumentInfo

	jsonPath string // JSON path using aliases too

	returnsListType         bool
	returnsSimpleType       bool
	returnsAbstractType     bool
	isEnclosingTypeAbstract bool
}

type ArgumentInfo struct {
	// The name of an unwrapped type.
	typeName string

	// inputObjectFieldTypes maps field coordinate of an input object to inputObjectField.
	// We have to gather it for later, when a variable's JSON is parsed and
	// there are no types in there.
	inputObjectFieldTypes map[FieldCoordinate]inputObjectField

	// isInputObject is true for an input object passed to the argument,
	// otherwise the argument is Scalar or Enum.
	isInputObject bool

	// isSimple is true for scalars and enums
	isSimple bool

	// When the argument points to a variable, it contains the name of the variable.
	hasVariable bool

	// The name of the variable that has value for this argument.
	varName string
}

// inputObjectField describes the type of input object field.
type inputObjectField struct {
	unwrappedTypeName string
	isList            bool // True if it should be processed as a list. Have priority over isInputObject
	isInputObject     bool // True if it should be processed as an input object.
}

// inputFieldsCost computes the cost of input object fields from the variable value.
// It handles both single objects and arrays of objects.
func (arg *ArgumentInfo) inputFieldsCost(vars resolve.VariablesView, weights map[FieldCoordinate]*FieldCost) int {
	if !arg.hasVariable {
		return 0
	}
	varValue := vars.Get(arg.varName)
	if varValue == nil {
		return 0
	}
	switch varValue.Type() {
	case astjson.TypeObject:
		return inputObjectCost(arg.typeName, varValue.GetObject(), weights, arg.inputObjectFieldTypes)
	case astjson.TypeArray:
		cost := 0
		for _, item := range varValue.GetArray() {
			cost += inputObjectCost(arg.typeName, item.GetObject(), weights, arg.inputObjectFieldTypes)
		}
		// When isList=true and the JSON contains nested arrays (e.g., [[{...}]]),
		// the inner arrays are skipped since item.Type() == astjson.TypeObject is false
		// for array items.
		return cost
	}
	return 0
}

func (node *CostTreeNode) maxWeightImplementingField(config *DataSourceCostConfig, fieldName string) *FieldCost {
	var maxWeight *FieldCost
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

func (node *CostTreeNode) maxMultiplierImplementingField(config *DataSourceCostConfig, fieldName string, arguments map[string]ArgumentInfo, vars resolve.VariablesView, defaultListSize int) *FieldListSize {
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

// requiringOneArgImplementingField returns the first FieldListSize from implementing types
// that has RequireOneSlicingArgument set to true. Used for validation when the enclosing type is abstract.
func (node *CostTreeNode) requiringOneArgImplementingField(config *DataSourceCostConfig, fieldName string) *FieldListSize {
	for _, implTypeName := range node.implementingTypeNames {
		coords := FieldCoordinate{implTypeName, fieldName}
		listSize := config.ListSizes[coords]
		if listSize != nil && listSize.RequireOneSlicingArgument {
			return listSize
		}
	}
	return nil
}

// sizedFieldImplementingFields returns all listSizes from implementing types
// whose SizedFields contains childFieldName.
// Used when the parent field belongs to an interface but @listSize is only on concrete types.
func (node *CostTreeNode) sizedFieldImplementingFields(config *DataSourceCostConfig, parentFieldName, childFieldName string) []*FieldListSize {
	var result []*FieldListSize
	for _, implTypeName := range node.implementingTypeNames {
		coord := FieldCoordinate{implTypeName, parentFieldName}
		listSize := config.ListSizes[coord]
		if listSize == nil {
			continue
		}
		if slices.Contains(listSize.SizedFields, childFieldName) {
			result = append(result, listSize)
		}
	}
	return result
}

// maxDirectiveArgumentWeightsImplementingFields returns the union of DirectiveArgumentWeights
// from implementing types' field definitions. For each directive.argument pair, it takes the
// maximum weight across all implementing types.
func (node *CostTreeNode) maxDirectiveArgumentWeightsImplementingFields(config *DataSourceCostConfig, fieldName string) map[string]int {
	var result map[string]int
	for _, implTypeName := range node.implementingTypeNames {
		coords := FieldCoordinate{implTypeName, fieldName}
		fw := config.Weights[coords]
		if fw == nil || len(fw.DirectiveArgumentWeights) == 0 {
			continue
		}
		if result == nil {
			result = make(map[string]int)
		}
		for dirArg, weight := range fw.DirectiveArgumentWeights {
			if existing, ok := result[dirArg]; !ok || weight > existing {
				result[dirArg] = weight
			}
		}
	}
	return result
}

// costInput holds the immutable inputs for a single cost-calculation pass.
// It is created once per Estimate/Actual call and threaded through the
// recursive cost computation.
type costInput struct {
	configs         map[DSHash]*DataSourceCostConfig
	vars            resolve.VariablesView
	typeStats       map[string]resolve.TypeNameStats
	defaultListSize int

	// isEstimation is true for estimated calculation and false for actual.
	isEstimation bool

	ignoreImplementingTypeWeights bool
}

// newCostInput bundles the cost-calculation inputs.
// defaultListSize designates the mode of operation.
// When it is non-negative, then its value is used as a fallback value for list sizes in estimations.
// Otherwise, it computes the actual cost and uses the typeStats map for list sizes.
func newCostInput(isEstimation bool, c *CostCalculator, vars resolve.VariablesView, typeStats map[string]resolve.TypeNameStats) *costInput {
	defaultLS := c.defaultListSize
	if !isEstimation {
		defaultLS = actualCostMode
	}
	return &costInput{
		configs:         c.costConfigs,
		vars:            vars,
		defaultListSize: defaultLS,
		typeStats:       typeStats,
		isEstimation:    isEstimation,

		ignoreImplementingTypeWeights: c.ignoreImplementingTypeWeights,
	}
}

// returnedTypeNames returns the runtime distribution of __typename for the array/object
// resolved at jsonPath, or nil when no stats were collected for that path.
func (ci *costInput) returnedTypeNames(jsonPath string) map[string]int {
	if stats, ok := ci.typeStats[jsonPath]; ok {
		return stats.TypeNames
	}
	return nil
}

// cost calculates the estimated/actual cost of this node and all descendants.
//
// For actual cost, multipliers are computed as averages (totalCount/parentCount).
func (node *CostTreeNode) cost(input *costInput) float64 {
	if node == nil {
		return 0
	}
	nodeCost := node.costsAndMultiplier(input)
	nodeCost.setDefaultMultiplier(node)

	childrenCost := node.childrenCost(input)

	cost := float64(nodeCost.args + nodeCost.directives)

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
	//
	// The field's own weight scales with multiplier, while children scale with
	// childMultiplier. These are equal except for a non-list object that resolved to
	// null some/all of the time: we still charge the field but not its absent children.
	cost += nodeCost.field*nodeCost.multiplier + childrenCost*nodeCost.childMultiplier
	if cost < 0 {
		cost = 0
	}
	return cost
}

// childrenCost returns the cost of all children.
func (node *CostTreeNode) childrenCost(input *costInput) (total float64) {
	if node.returnsAbstractType {
		// We should charge fields of abstract types once, even if the same field was used
		// in the fragment and on the abstract type.
		perTypeFields := make(map[string]struct{})
		for _, child := range node.children {
			if child.fieldCoords.TypeName != node.fieldTypeName {
				perTypeFields[child.fieldCoords.FieldName] = struct{}{}
			}
		}
		perTypeCost := make(map[string]float64, len(node.implementingTypeNames))
		for _, child := range node.children {
			// Fields used directly on the abstract type are counted anyway.
			if child.fieldCoords.TypeName == node.fieldTypeName {
				if _, covered := perTypeFields[child.fieldCoords.FieldName]; covered {
					continue
				}
				total += child.cost(input) // shared cost among all the children
			} else {
				perTypeCost[child.fieldCoords.TypeName] += child.cost(input)
			}
		}
		var typeCost float64
		if input.isEstimation {
			// max of
			for _, c := range perTypeCost {
				typeCost = max(typeCost, c)
			}
		} else {
			// Actual cost: only charge fragments whose concrete type was actually returned at runtime.
			returnedTypeNames := input.returnedTypeNames(node.jsonPath)
			for typeName, c := range perTypeCost {
				if returnedTypeNames != nil {
					if _, returned := returnedTypeNames[typeName]; !returned {
						continue // type was not returned at runtime
					}
				}
				typeCost += c
			}
		}
		total += typeCost
	} else {
		for _, child := range node.children {
			total += child.cost(input)
		}
	}
	return total
}

// costNodeResult contains intermediate results for a node.
type costNodeResult struct {
	field      float64
	args       int
	directives int
	multiplier float64

	// childMultiplier scales the cost of this node's children. It is normally equal to multiplier,
	// but can differ for a non-list object field that resolves to null part of the time.
	childMultiplier float64
}

// setDefaultMultiplier enforces multiplier=1 for non-list fields including the root node.
func (r *costNodeResult) setDefaultMultiplier(node *CostTreeNode) {
	if (r.multiplier == undefinedMultiplier && !node.returnsListType) || node.fieldCoords == costTreeRootNodeCoords {
		r.multiplier = 1
	}
	if r.multiplier == undefinedMultiplier {
		r.multiplier = 0
	}
	// By default, children scale exactly like the node itself.
	if r.childMultiplier == undefinedMultiplier {
		r.childMultiplier = r.multiplier
	}
}

// costsAndMultiplier returns the cost values for a node based on its data sources.
//
// For this node we sum weights of the field or its returned type for all the data sources.
// Each data source can have its own cost configuration. If we plan field on two data sources,
// it means more work for the router: we should sum the costs.
//
// nodeCost.field is the weight of this field or its returned type
// nodeCost.args is the sum of argument weights and input fields used on this field.
// nodeCost.directives is the sum of directive argument weights.
//
// When estimating cost, it picks the highest multiplier among different data sources.
// Also, it picks the maximum field weight of implementing types and then
// the maximum among slicing arguments.
func (node *CostTreeNode) costsAndMultiplier(input *costInput) (nodeCost costNodeResult) {
	nodeCost.multiplier = undefinedMultiplier
	nodeCost.childMultiplier = undefinedMultiplier
	if len(node.dataSourceHashes) == 0 {
		// no data source is responsible for this field
		return
	}

	parent := node.parent

	for _, dsHash := range node.dataSourceHashes {
		dsCostConfig, ok := input.configs[dsHash]
		if !ok || dsCostConfig == nil {
			continue
		}

		fieldWeight := dsCostConfig.Weights[node.fieldCoords]
		listSize := dsCostConfig.ListSizes[node.fieldCoords]
		// The cost directive is not allowed on fields in an interface.
		// The cost of a field on an interface can be calculated based on the costs of
		// the corresponding field on each concrete type implementing that interface,
		// either directly or indirectly through other interfaces.
		//
		// Composition should not let interface fields have weights, so we assume that
		// the enclosing type is concrete.
		// Commented condition is a good check for that. Might be needed later:
		// fieldWeight != nil && node.isEnclosingTypeAbstract && parent.returnsAbstractType
		if node.isEnclosingTypeAbstract && parent.returnsAbstractType {
			// This field is part of the enclosing interface/union.
			// We look into implementing types and find the max-weighted field.
			// Found fieldWeight can be used for all the calculations.
			if !input.ignoreImplementingTypeWeights {
				fieldWeight = parent.maxWeightImplementingField(dsCostConfig, node.fieldCoords.FieldName)
			}
			// If this field has listSize defined, then do not look into implementing types.
			if input.isEstimation && listSize == nil && node.returnsListType {
				listSize = parent.maxMultiplierImplementingField(dsCostConfig, node.fieldCoords.FieldName, node.arguments, input.vars, input.defaultListSize)
			}
		}

		if fieldWeight != nil && fieldWeight.HasWeight {
			nodeCost.field += float64(fieldWeight.Weight)
		} else {
			// Use the weight of the type returned by this field
			switch {
			case node.returnsSimpleType:
				nodeCost.field += float64(dsCostConfig.EnumScalarTypeWeight(node.fieldTypeName))
			case node.returnsAbstractType:
				returnedTypeNames := input.returnedTypeNames(node.jsonPath)
				treatAsMaximum := false
				if len(returnedTypeNames) == 1 {
					if _, returned := returnedTypeNames[node.fieldTypeName]; returned {
						// Subgraph did not return __typename for elements of this list,
						// the response has seen only the abstract typeName for elements of this list.
						treatAsMaximum = true
					}
				}
				if input.isEstimation || treatAsMaximum {
					// Find the max weight among all implementing types:
					maxWeight := 0
					for _, implTypeName := range node.implementingTypeNames {
						maxWeight = max(maxWeight, dsCostConfig.ObjectTypeWeight(implTypeName))
					}
					nodeCost.field += float64(maxWeight)
				} else {
					// Adjust the cost of field as weighted sum based on the distribution of
					// typeNames in the response.
					var sum, count float64
					for _, implTypeName := range node.implementingTypeNames {
						if returnedTypeNames != nil {
							if actual, returned := returnedTypeNames[implTypeName]; returned {
								sum += float64(actual * dsCostConfig.ObjectTypeWeight(implTypeName))
								count += float64(actual)
							}
						}
					}
					if count > 0 {
						nodeCost.field += sum / count
					}
				}
			default:
				nodeCost.field += float64(dsCostConfig.ObjectTypeWeight(node.fieldTypeName))
			}
		}

		for argName, arg := range node.arguments {
			// Add explicit argument weight if present.
			argumentWeightFound := false
			if fieldWeight != nil {
				if weight, ok := fieldWeight.ArgumentWeights[argName]; ok {
					nodeCost.args += weight
					argumentWeightFound = true
				}
			}

			// Input objects always add field-level costs, as the spec says.
			// For other types, the explicit argument weight replaces the default type weight.
			if arg.isInputObject {
				nodeCost.args += arg.inputFieldsCost(input.vars, dsCostConfig.Weights)
			} else if !argumentWeightFound {
				if arg.isSimple {
					nodeCost.args += dsCostConfig.EnumScalarTypeWeight(arg.typeName)
				} else {
					nodeCost.args += dsCostConfig.ObjectTypeWeight(arg.typeName)
				}
			}
		}

		// Directive weights: sum from the field's own DirectiveArgumentWeights,
		// or from implementing types when the enclosing type is abstract.
		if node.isEnclosingTypeAbstract && parent.returnsAbstractType && !input.ignoreImplementingTypeWeights {
			for _, weight := range parent.maxDirectiveArgumentWeightsImplementingFields(dsCostConfig, node.fieldCoords.FieldName) {
				nodeCost.directives += weight
			}
		} else if fieldWeight != nil {
			for _, weight := range fieldWeight.DirectiveArgumentWeights {
				nodeCost.directives += weight
			}
		}

		if !node.returnsListType || !input.isEstimation {
			continue
		}

		// This field returns a list, and we are in estimation mode.
		// Pick the maximum multiplier of all data sources.

		if listSize != nil {
			m := float64(listSize.multiplier(node.arguments, input.vars, input.defaultListSize))
			// If this node returns a list of abstract types, then it could have listSize defined.
			// Spec allows defining listSize on the fields of interfaces.
			nodeCost.multiplier = max(nodeCost.multiplier, m)
			continue
		}

		// This node does not have listSize. If its parent has the sizedField pointing to the child,
		// calculate multiplier from the parent POV.
		if parent == nil {
			continue
		}
		parentLS := dsCostConfig.ListSizes[parent.fieldCoords]
		if parentLS != nil {
			for _, sf := range parentLS.SizedFields {
				if sf != node.fieldCoords.FieldName {
					continue
				}
				m := float64(parentLS.multiplier(parent.arguments, input.vars, input.defaultListSize))
				nodeCost.multiplier = max(nodeCost.multiplier, m)
			}
			continue
		}

		// This field is on interface, pick the max multiplier among implementing types.
		if parent.isEnclosingTypeAbstract {
			// SizedFields only on concrete types, accessed through interface.
			grandParent := parent.parent
			if grandParent != nil {
				implementing := grandParent.sizedFieldImplementingFields(
					dsCostConfig, parent.fieldCoords.FieldName, node.fieldCoords.FieldName,
				)
				for _, implLS := range implementing {
					m := float64(implLS.multiplier(parent.arguments, input.vars, input.defaultListSize))
					if m > nodeCost.multiplier {
						nodeCost.multiplier = m
					}
				}
			}
		}
	}

	if input.isEstimation {
		if !node.returnsListType {
			return
		}
		if nodeCost.multiplier == undefinedMultiplier {
			nodeCost.multiplier = float64(input.defaultListSize)
		}
		return
	}

	// The block below adjusts the multiplier of the node in ACTUAL mode.

	if node.parent == nil {
		return
	}
	parentStats := input.typeStats[node.parent.jsonPath]

	if node.returnsListType {
		// This node's multiplier is its own array size, averaged over its immediate parent's
		// occurrence count to avoid double-counting.
		if nodeStats, ok := input.typeStats[node.jsonPath]; ok && nodeStats.Size != 0 {
			parentSize := 1.0
			if parentStats.Size > 0 {
				parentSize = float64(parentStats.Size)
			}
			nodeCost.multiplier = float64(nodeStats.Size) / parentSize
		}
		return
	}

	// Non-list field.

	// For a concrete object field, scale its children by how often the object actually
	// resolved non-null relative to its parent's occurrences.
	if !node.returnsSimpleType && parentStats.Size > 0 {
		if nodeStats, ok := input.typeStats[node.jsonPath]; ok {
			// The field's own weight is kept via multiplier while its children
			// are charged only for the fraction of occurrences where the object was present.
			nodeCost.childMultiplier = float64(nodeStats.Size) / float64(parentStats.Size)
		}
	}

	// If the field sits directly under a field resolving an abstract type (a list or a single object),
	// narrow its multiplier by the share of parent occurrences that
	// actually match this field's concrete type.
	if !node.parent.returnsAbstractType || parentStats.Size == 0 {
		return
	}
	// Fields selected on the abstract type itself carry the max implementing-type weight;
	// re-weight by the actual type distribution. Gated to abstract lists only because
	// for a single abstract object this would change how interface fields are billed
	// (max weight today), and it mishandles fields with no explicit per-implementing-type weights.
	if node.parent.returnsListType && node.isEnclosingTypeAbstract && nodeCost.field > 0 && !input.ignoreImplementingTypeWeights {
		var weightedSum float64
		found := false
		for _, implTypeName := range parent.implementingTypeNames {
			count, typeNameFound := parentStats.TypeNames[implTypeName]
			if !typeNameFound {
				continue
			}
			for _, dsHash := range node.dataSourceHashes {
				dsCostConfig, ok := input.configs[dsHash]
				if !ok || dsCostConfig == nil {
					continue
				}

				coords := FieldCoordinate{implTypeName, node.fieldCoords.FieldName}
				fieldWeight := dsCostConfig.Weights[coords]
				if fieldWeight != nil {
					found = true
					weightedSum += float64(fieldWeight.Weight * count)
				}
			}
		}
		if found {
			nodeCost.multiplier = weightedSum / (nodeCost.field * float64(parentStats.Size))
		}
	}
	if !node.isEnclosingTypeAbstract {
		count, typeNameFound := parentStats.TypeNames[node.fieldCoords.TypeName]
		if !typeNameFound {
			nodeCost.multiplier = 0
		} else {
			nodeCost.multiplier = float64(count) / float64(parentStats.Size)
		}
	}
	return
}

// inputObjectCost recursively computes the cost of an input object argument (typeName)
// by walking its JSON value. It adds a cost of each field found in weights. It uses types
// to figure out types and the kind of value it encounters.
// Given a schema "input Filter { name: String @cost(weight: 5), nested: Filter }"
// and a variable "{ name: "foo", nested: { name: "bar" } }", the cost would be 5 + 5 = 10
func inputObjectCost(
	typeName string,
	value *astjson.Object,
	weights map[FieldCoordinate]*FieldCost,
	types map[FieldCoordinate]inputObjectField) int {
	if value == nil {
		return 0
	}
	cost := 0

	processKeyValue := func(fieldName []byte, value *astjson.Value) {
		coords := FieldCoordinate{typeName, string(fieldName)}
		typeInfo, found := types[coords]
		if !found {
			return
		}
		if value == nil || value.Type() == astjson.TypeNull {
			return
		}
		if typeInfo.isList {
			valueArray := value.GetArray()
			for _, item := range valueArray {
				if item.Type() == astjson.TypeObject {
					cost += inputObjectCost(typeInfo.unwrappedTypeName, item.GetObject(), weights, types)
				}
			}
		} else if typeInfo.isInputObject {
			valueObj := value.GetObject()
			if valueObj != nil {
				cost += inputObjectCost(typeInfo.unwrappedTypeName, valueObj, weights, types)
			}
		}
		if fw, ok := weights[coords]; ok && fw.HasWeight {
			cost += fw.Weight
		}
	}
	value.Visit(processKeyValue)
	return cost
}

// CostCalculator manages cost calculation during AST traversal
type CostCalculator struct {
	// tree points to the root of the complete cost tree. Calculator tree is built once per query,
	// then it is cached as part of the plan cache and
	// not supposed to change ever again during the lifetime of a process.
	// Once this tree is built, it is immutable and can be shared between multiple requests.
	tree *CostTreeNode

	// costConfigs is a map of data source hashes to their cost configuration.
	costConfigs map[DSHash]*DataSourceCostConfig

	// defaultListSize is used as a fallback for list sizes when no specific size is provided.
	defaultListSize int

	// ignoreImplementingTypeWeights, when true, ignores @cost weights contributed by
	// implementing types on abstract (interface/union) fields that have no weight of their own.
	// Emulates Apollo's cost behavior.
	ignoreImplementingTypeWeights bool
}

// NewCostCalculator creates a new cost calculator. The defaultListSize is floored to 1.
func NewCostCalculator(config Configuration) *CostCalculator {
	c := CostCalculator{}
	// Extract cost configurations from all data sources, keyed by data source hash.
	// We do it once, it should be immutable in the cache.
	c.costConfigs = make(map[DSHash]*DataSourceCostConfig)
	for _, ds := range config.DataSources {
		dsCostConfig := ds.GetCostConfig()
		if dsCostConfig == nil {
			dsCostConfig = &DataSourceCostConfig{}
		}
		c.costConfigs[ds.Hash()] = dsCostConfig
	}
	// Zero would estimate all lists as zero.
	c.defaultListSize = max(config.StaticCostDefaultListSize, 1)
	c.ignoreImplementingTypeWeights = config.IgnoreImplementingTypeWeights
	return &c
}

// EstimateCost returns the calculated total static cost.
// config should be static per process or instance. vars could change between requests.
func (c *CostCalculator) EstimateCost(vars resolve.VariablesView) int {
	input := newCostInput(true, c, vars, nil)
	// fmt.Println(c.DebugPrint(vars, nil))
	return int(math.RoundToEven(c.tree.cost(input)))
}

// ActualCost returns the actual cost of the operation that is based on the actual sizes of lists.
func (c *CostCalculator) ActualCost(vars resolve.VariablesView, typeStats map[string]resolve.TypeNameStats) int {
	input := newCostInput(false, c, vars, typeStats)
	// fmt.Println(c.DebugPrint(vars, typeStats))
	return int(math.RoundToEven(c.tree.cost(input)))
}

// ValidateSliceArguments checks that all fields with slicingArguments and
// requireOneSlicingArgument are valid against the arguments passed to those fields.
// Violations are collected as external errors into the report.
func (c *CostCalculator) ValidateSliceArguments(vars resolve.VariablesView, report *operationreport.Report) {
	c.tree.validateSliceArguments(c.costConfigs, vars, report)
}

func (node *CostTreeNode) validateSliceArguments(configs map[DSHash]*DataSourceCostConfig, vars resolve.VariablesView, report *operationreport.Report) {
	if node == nil {
		return
	}

	for _, dsHash := range node.dataSourceHashes {
		dsCostConfig := configs[dsHash]
		if dsCostConfig == nil {
			continue
		}

		listSize := dsCostConfig.ListSizes[node.fieldCoords]
		if listSize == nil && node.isEnclosingTypeAbstract && node.parent != nil && node.parent.returnsAbstractType {
			// We pick the first from the list of implementing types. Composition should verify that
			// all implementations are aligned on the slicingArguments within the single subgraph.
			// Otherwise, we would have inconsistent expectations between implementing types.
			listSize = node.parent.requiringOneArgImplementingField(dsCostConfig, node.fieldCoords.FieldName)
		}
		if listSize == nil || !listSize.RequireOneSlicingArgument || len(listSize.SlicingArguments) == 0 {
			continue
		}

		count := 0
		// The engine has all inlined literals converted to variables at this stage.
		// No need to check for literals.
		for _, slicingArg := range listSize.SlicingArguments {
			if _, found := listSize.resolveSlicingArg(slicingArg, node.arguments, vars); found {
				count++
			}
		}
		if count != 1 {
			path := node.buildASTPath()
			if count == 0 {
				report.AddExternalError(operationreport.ExternalError{
					Message:    fmt.Sprintf("field '%s' requires exactly one slicing argument, but none was provided", node.fieldCoords),
					Path:       path,
					StatusCode: http.StatusBadRequest,
				})
			} else {
				report.AddExternalError(operationreport.ExternalError{
					Message:    fmt.Sprintf("field '%s' requires exactly one slicing argument, but %d were provided", node.fieldCoords, count),
					Path:       path,
					StatusCode: http.StatusBadRequest,
				})
			}
		}
		// Only report once per field node, even if multiple data sources agree.
		break
	}

	for _, child := range node.children {
		child.validateSliceArguments(configs, vars, report)
	}
}

// buildASTPath constructs an ast.Path from the node's jsonPath (e.g. "search.items" → [search,items]).
func (node *CostTreeNode) buildASTPath() ast.Path {
	if node.jsonPath == "" {
		return nil
	}
	segments := strings.Split(node.jsonPath, ".")
	path := make(ast.Path, len(segments))
	for i, seg := range segments {
		path[i] = ast.PathItem{
			Kind:      ast.FieldName,
			FieldName: []byte(seg),
		}
	}
	return path
}

// DebugPrint prints the cost tree structure for debugging purposes.
// It shows each node's field coordinate, costs, multipliers, and computed totals.
func (c *CostCalculator) DebugPrint(vars resolve.VariablesView, typeStats map[string]resolve.TypeNameStats) string {
	if c.tree == nil || len(c.tree.children) == 0 {
		return "<empty cost tree>"
	}
	var sb strings.Builder
	var input *costInput
	if typeStats != nil {
		input = newCostInput(false, c, vars, typeStats)
		sb.WriteString("Actual Cost Tree Debug\n")
		sb.WriteString("======================\n")
	} else {
		input = newCostInput(true, c, vars, typeStats)
		sb.WriteString("Estimated Cost Tree Debug\n")
		sb.WriteString("=========================\n")
	}
	c.tree.children[0].debugPrint(&sb, input, 0)
	return sb.String()
}

// debugPrint recursively prints a node and its children with indentation.
func (node *CostTreeNode) debugPrint(sb *strings.Builder, input *costInput, depth int) {
	// implementation is a bit crude and redundant, we could skip calculating nodes all over again.
	// but it should suffice for debugging tests.
	if node == nil || node.fieldCoords.FieldName == "__typename" {
		return
	}

	indent := strings.Repeat("    ", depth)
	fmt.Fprintf(sb, "%s· %s", indent, node.fieldCoords)

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
		fmt.Fprintf(sb, ", path=%s", node.jsonPath)
	}
	if len(node.dataSourceHashes) > 0 {
		fmt.Fprintf(sb, ", dataSources=%v", len(node.dataSourceHashes))
	}
	sb.WriteString("\n")

	// This is somewhat redundant, but it should not be used in production.
	// If there is a need to present a cost tree to the user,
	// printing should be embedded into the tree calculation process.
	subtreeCost := node.cost(input)
	fmt.Fprintf(sb, "%s  cost = %.2f\n", indent, subtreeCost)

	// Compute intermediate cost values for this node to display.
	nodeCost := node.costsAndMultiplier(input)
	nodeCost.setDefaultMultiplier(node)

	fmt.Fprintf(sb, "%s  mult = %.2f", indent, nodeCost.multiplier)
	fmt.Fprintf(sb, ", fieldCost = %.2f", nodeCost.field)
	if nodeCost.childMultiplier != nodeCost.multiplier {
		fmt.Fprintf(sb, ", childMult = %.2f", nodeCost.childMultiplier)
	}
	if nodeCost.args > 0 {
		fmt.Fprintf(sb, ", argsCost = %d", nodeCost.args)
	}
	if nodeCost.directives > 0 {
		fmt.Fprintf(sb, ", directivesCost = %d", nodeCost.directives)
	}

	sb.WriteString("\n")

	if len(node.arguments) > 0 {
		var argStrs []string
		for name, arg := range node.arguments {
			if arg.hasVariable {
				if input.vars.IsEmpty() {
					argStrs = append(argStrs, fmt.Sprintf("%s=$%s", name, arg.varName))
				} else {
					v := input.vars.Get(arg.varName)
					argStrs = append(argStrs, fmt.Sprintf("%s=%s($%s)", name, v, arg.varName))
				}
			} else {
				argStrs = append(argStrs, fmt.Sprintf("%s=<obj>", name))
			}
		}
		fmt.Fprintf(sb, "%s  args: {%s}\n", indent, strings.Join(argStrs, ", "))
	}

	if len(node.implementingTypeNames) > 0 {
		fmt.Fprintf(sb, "%s  implements: [%s]\n", indent, strings.Join(node.implementingTypeNames, ", "))
	}

	for _, child := range node.children {
		child.debugPrint(sb, input, depth+1)
	}
}
