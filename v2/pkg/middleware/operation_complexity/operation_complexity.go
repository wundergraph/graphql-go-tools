/*
Package operation_complexity implements two common algorithms used by GitHub to calculate
GraphQL query complexity:

 1. Node count, the maximum number of Nodes a query may return
 2. Complexity, the maximum number of Node requests that might be needed to execute the query

OperationComplexityEstimator takes a schema definition and a query and then
walks recursively through the query to calculate both variables.

The calculation can be influenced by integer arguments on fields that indicate
the number of Nodes returned by a field.

To help the algorithm understand the schema make use of these two directives:

  - directive @nodeCountMultiply on ARGUMENT_DEFINITION
  - directive @nodeCountSkip on FIELD

"nodeCountMultiply" indicates that the Int value the directive is applied on
should be used as a Node multiplier.

"nodeCountSkip" indicates that the algorithm should skip this Node.
It can be used to allowlist certain query paths.

Note: Introspection fields (__schema and __type) are automatically skipped
from complexity calculations by default.
*/
package operation_complexity

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// OperationStats contains estimates for an operation or root field.
type OperationStats struct {
	// NodeCount is the maximum number of returned nodes.
	NodeCount int
	// Complexity is the maximum number of field requests.
	Complexity int
	// Depth is the maximum number of response field levels on a path.
	// Root-field depth is relative to the root.
	Depth int
}

// RootFieldStats contains the stats for one top-level field. Alias is empty
// when the response name is the same as FieldName.
type RootFieldStats struct {
	TypeName  string
	FieldName string
	Alias     string
	Stats     OperationStats
}

var (
	nodeCountMultiply = []byte("nodeCountMultiply")
	nodeCountSkip     = []byte("nodeCountSkip")
)

const (
	__schemaLiteral = "__schema"
	__typeLiteral   = "__type"
)

// OperationComplexityEstimator estimates stats for normalized operations.
// It may be reused sequentially, but is not safe for concurrent use.
type OperationComplexityEstimator struct {
	walker  *astvisitor.Walker
	visitor *complexityVisitor
}

// NewOperationComplexityEstimator creates an estimator. If skipIntrospection
// is true, __schema and __type root fields are excluded.
func NewOperationComplexityEstimator(skipIntrospection bool) *OperationComplexityEstimator {
	walker := astvisitor.NewWalker(48)
	visitor := &complexityVisitor{
		Walker:            &walker,
		multipliers:       make([]multiplier, 0, 16),
		skipIntrospection: skipIntrospection,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)
	walker.RegisterLeaveFieldVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterEnterSelectionSetVisitor(visitor)
	walker.RegisterEnterFragmentDefinitionVisitor(visitor)

	return &OperationComplexityEstimator{
		walker:  &walker,
		visitor: visitor,
	}
}

// Do returns global and per-root-field estimates for the operation.
func (n *OperationComplexityEstimator) Do(operation, definition *ast.Document, report *operationreport.Report) (OperationStats, []RootFieldStats) {
	n.visitor.count = 0
	n.visitor.complexity = 0
	n.visitor.maxOperationDepth = 0
	n.visitor.multipliers = n.visitor.multipliers[:0]

	n.visitor.fieldDepth = 0

	if n.visitor.calculatedRootFieldStats == nil {
		n.visitor.calculatedRootFieldStats = make([]RootFieldStats, 0, len(definition.RootOperationTypeDefinitions))
	}
	n.visitor.calculatedRootFieldStats = n.visitor.calculatedRootFieldStats[:0]

	if n.visitor.rootOperationTypeNames == nil {
		n.visitor.rootOperationTypeNames = make(map[string]struct{}, len(definition.RootOperationTypeDefinitions))
	}
	for key := range n.visitor.rootOperationTypeNames {
		delete(n.visitor.rootOperationTypeNames, key)
	}

	n.walker.Walk(operation, definition, report)

	globalResult := OperationStats{
		NodeCount:  n.visitor.count,
		Complexity: n.visitor.complexity,
		Depth:      n.visitor.maxOperationDepth,
	}

	return globalResult, n.visitor.calculatedRootFieldStats
}

// Deprecated: use NewOperationComplexityEstimator.
func CalculateOperationComplexity(operation, definition *ast.Document, report *operationreport.Report) (OperationStats, []RootFieldStats) {
	estimator := NewOperationComplexityEstimator(false)
	return estimator.Do(operation, definition, report)
}

type complexityVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document
	count                 int
	complexity            int

	// maxOperationDepth includes the root field.
	maxOperationDepth int

	// multipliers contains @nodeCountMultiply argument values for the active
	// field path.
	multipliers []multiplier

	// fieldDepth counts active fields with selections. Fragments add no depth.
	fieldDepth int

	rootOperationTypeNames map[string]struct{}

	// currentRootFieldStats is reused because root fields are visited depth-first.
	currentRootFieldStats RootFieldStats

	// maxRootFieldDepth is relative to the current root field.
	maxRootFieldDepth int

	calculatedRootFieldStats []RootFieldStats

	// Enforces to ignore introspection queries in calculations.
	skipIntrospection bool
}

type multiplier struct {
	fieldRef int
	multi    int
}

func (c *complexityVisitor) calculateMultiplied(i int) int {
	for _, j := range c.multipliers {
		i = i * j.multi
	}
	return i
}

func (c *complexityVisitor) EnterDocument(operation, definition *ast.Document) {
	c.operation = operation
	c.definition = definition

	for i := 0; i < len(c.definition.RootOperationTypeDefinitions); i++ {
		name := c.definition.Input.ByteSliceString(c.definition.RootOperationTypeDefinitions[i].NamedType.Name)
		c.rootOperationTypeNames[name] = struct{}{}
	}
}

func (c *complexityVisitor) EnterArgument(ref int) {

	if c.Ancestors[len(c.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}

	definition, ok := c.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}

	if !c.definition.InputValueDefinitionHasDirective(definition, nodeCountMultiply) {
		return
	}

	value := c.operation.ArgumentValue(ref)
	if value.Kind == ast.ValueKindInteger {
		multi := c.operation.IntValueAsInt32(value.Ref)
		c.multipliers = append(c.multipliers, multiplier{
			fieldRef: c.Ancestors[len(c.Ancestors)-1].Ref,
			multi:    int(multi),
		})
	}
}

func (c *complexityVisitor) EnterField(ref int) {
	definition, exists := c.FieldDefinition(ref)
	if !exists {
		return
	}

	if _, skip := c.definition.FieldDefinitionDirectiveByName(definition, nodeCountSkip); skip {
		c.SkipNode()
		return
	}

	typeName, fieldName, alias := c.extractFieldRelatedNames(ref, definition)
	if c.skipIntrospection && (fieldName == __schemaLiteral || fieldName == __typeLiteral) {
		c.SkipNode()
		return
	}
	if c.isRootType(typeName) {
		c.resetCurrentRootFieldComplexity(typeName, fieldName, alias)
	}

	if !c.operation.FieldHasSelections(ref) {
		return
	}

	// A field's multiplier applies to its result, not its own request.
	c.complexity = c.complexity + c.calculateMultiplied(1)
	c.fieldDepth++

	// Operation depth includes the selected child. Root depth is root-relative.
	c.maxOperationDepth = max(c.maxOperationDepth, c.fieldDepth+1)

	c.currentRootFieldStats.Stats.Complexity = c.currentRootFieldStats.Stats.Complexity + c.calculateMultiplied(1)
	c.maxRootFieldDepth = max(c.maxRootFieldDepth, c.fieldDepth)
}

func (c *complexityVisitor) LeaveField(ref int) {
	if c.operation.FieldHasSelections(ref) {
		c.fieldDepth--
	}

	if c.isRootTypeField() {
		c.endRootFieldComplexityCalculation()
	}

	if len(c.multipliers) == 0 {
		return
	}

	if c.multipliers[len(c.multipliers)-1].fieldRef == ref {
		c.multipliers = c.multipliers[:len(c.multipliers)-1]
	}
}

func (c *complexityVisitor) EnterSelectionSet(ref int) {

	// Operation and fragment selection sets do not represent returned nodes.
	if c.Ancestors[len(c.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}

	c.count = c.count + c.calculateMultiplied(1)
	c.currentRootFieldStats.Stats.NodeCount = c.currentRootFieldStats.Stats.NodeCount + c.calculateMultiplied(1)
}

func (c *complexityVisitor) EnterFragmentDefinition(ref int) {
	c.SkipNode()
}

func (c *complexityVisitor) resetCurrentRootFieldComplexity(typeName, fieldName, alias string) {
	c.currentRootFieldStats = RootFieldStats{
		TypeName:  typeName,
		FieldName: fieldName,
		Alias:     alias,
		Stats: OperationStats{
			NodeCount:  0,
			Complexity: 0,
			Depth:      0,
		},
	}
}

func (c *complexityVisitor) endRootFieldComplexityCalculation() {
	c.currentRootFieldStats.Stats.Depth = c.maxRootFieldDepth
	c.calculatedRootFieldStats = append(c.calculatedRootFieldStats, c.currentRootFieldStats)

	c.maxRootFieldDepth = 0
}

func (c *complexityVisitor) extractFieldRelatedNames(ref, definitionRef int) (typeName, fieldName, alias string) {
	fieldName = c.definition.FieldDefinitionNameString(definitionRef)
	alias = c.operation.FieldAliasOrNameString(ref)
	if fieldName == alias {
		alias = ""
	}

	return c.EnclosingTypeDefinition.NameString(c.definition), fieldName, alias
}

func (c *complexityVisitor) isRootType(name string) bool {
	_, ok := c.rootOperationTypeNames[name]
	return ok
}

func (c *complexityVisitor) isRootTypeField() bool {
	enclosingTypeName := c.EnclosingTypeDefinition.NameString(c.definition)
	return c.isRootType(enclosingTypeName)
}
