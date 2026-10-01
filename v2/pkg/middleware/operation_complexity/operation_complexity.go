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

Fragment spreads are expanded where they are used: a spread contributes what
an inline fragment with the fragment's type condition and selections would.
Within one field's selections (or an operation's root selections), a fragment
that was already spread on the same enclosing type adds nothing, as field
collection merges it. Directives on spreads are not evaluated for this. The
estimate can therefore be lower than for the operation with its spreads
inlined, which keeps a copy per spread, but not lower than what field
collection selects. Spreads of undefined fragments and spreads that would
form a cycle contribute nothing. Fragment spreads nested more than 1000
fragments deep stop the estimation with an error. Fragment definitions are
only counted through their spreads.

Note: Introspection fields (__schema and __type) are automatically skipped
from complexity calculations by default.
*/
package operation_complexity

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// OperationStats contains estimates for an operation or root field.
type OperationStats struct {
	// FieldCount is the number of field selections in the operation, with
	// fragment spreads expanded, including leaf fields and __typename. Each
	// selection contributes one, regardless of list-size multipliers.
	// Fields excluded by @nodeCountSkip or skipIntrospection are not counted.
	FieldCount int
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

// OperationComplexityEstimator estimates stats for operations. Operations are
// usually normalized first; fragment spreads left in the operation are
// expanded as described in the package documentation.
// It may be reused sequentially, but is not safe for concurrent use.
type OperationComplexityEstimator struct {
	walker  *astvisitor.Walker
	visitor *complexityVisitor

	fragmentVisitorsRegistered bool
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
	visitor.registerVisitors(&walker)

	return &OperationComplexityEstimator{
		walker:  &walker,
		visitor: visitor,
	}
}

// Do returns global and per-root-field estimates for the operation. If the
// walk adds an error to the report, the estimates are incomplete.
func (n *OperationComplexityEstimator) Do(operation, definition *ast.Document, report *operationreport.Report) (OperationStats, []RootFieldStats) {
	// Fragment expansion swaps the visitor's walker; always start from the
	// operation walker.
	n.visitor.Walker = n.walker
	n.visitor.fieldCount = 0
	n.visitor.count = 0
	n.visitor.complexity = 0
	n.visitor.maxOperationDepth = 0
	n.visitor.multipliers = n.visitor.multipliers[:0]

	n.visitor.fieldDepth = 0
	// An aborted walk can end a root field that it did not start.
	n.visitor.currentRootFieldStats = RootFieldStats{}
	n.visitor.maxRootFieldDepth = 0

	n.visitor.fragments = newFragmentExpansion(operation)
	// Normalized operations have no fragment definitions left, so the operation
	// walker only gets the fragment callbacks once an operation needs them.
	if n.visitor.fragments != nil && !n.fragmentVisitorsRegistered {
		n.visitor.registerFragmentVisitors(n.walker)
		n.fragmentVisitorsRegistered = true
	}

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

	if n.visitor.fragments != nil {
		n.visitor.fragments.releaseWalkers()
		n.visitor.fragments = nil
	}

	globalResult := OperationStats{
		FieldCount: n.visitor.fieldCount,
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
	fieldCount            int
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

	// fragments is the state for expanding fragment spreads. It is nil when the
	// operation has no fragment definitions.
	fragments *fragmentExpansion
}

type multiplier struct {
	fieldRef int
	multi    int
}

// maxFragmentDepth limits how deep fragment expansions can be nested. Each
// level walks its fragment with its own walker, so the limit bounds the stack
// and the walkers in use. Real operations stay far below it.
const maxFragmentDepth = 1000

// fragmentExpansion is the state for expanding the fragment spreads of one
// operation.
type fragmentExpansion struct {
	// definitions maps fragment names to the fragment definitions that are
	// root nodes of the operation. Definitions removed by normalization are not
	// included.
	definitions map[string]int

	// document is a copy of the operation whose only root node is the fragment
	// definition being expanded, so a fragment walk visits nothing else.
	document  ast.Document
	rootNodes [1]ast.Node

	// expanding marks the fragment definitions being expanded, by ref. depth is
	// the number of nested expansions; walkers[i] walks the fragment expanded
	// at depth i.
	expanding []bool
	depth     int
	walkers   []*astvisitor.Walker

	// expanded contains the fragments expanded in each selection scope; scopes
	// contains the ids of the open scopes, innermost last.
	expanded   map[expandedFragment]struct{}
	scopes     []int
	scopeCount int
}

type expandedFragment struct {
	scope         int
	enclosingType ast.Node
	fragmentRef   int
}

// newFragmentExpansion returns the state for expanding the operation's
// fragment spreads, or nil if the operation has no fragment definitions left.
func newFragmentExpansion(operation *ast.Document) *fragmentExpansion {
	if operation == nil {
		return nil
	}

	var f *fragmentExpansion
	for _, node := range operation.RootNodes {
		if node.Kind != ast.NodeKindFragmentDefinition {
			continue
		}
		if f == nil {
			f = &fragmentExpansion{
				definitions: make(map[string]int),
				expanding:   make([]bool, len(operation.FragmentDefinitions)),
				expanded:    make(map[expandedFragment]struct{}),
			}
		}
		// The first definition of a name wins, as in ast.Document.FragmentDefinitionRef.
		name := operation.FragmentDefinitionNameString(node.Ref)
		if _, exists := f.definitions[name]; !exists {
			f.definitions[name] = node.Ref
		}
	}
	if f == nil {
		return nil
	}

	f.document = *operation
	f.document.RootNodes = f.rootNodes[:]
	return f
}

// releaseWalkers returns the fragment walkers to the walker pool, so they keep
// no references to the operation.
func (f *fragmentExpansion) releaseWalkers() {
	for _, walker := range f.walkers {
		walker.Release()
	}
}

// registerVisitors registers the callbacks that count selections and skip
// fragment definitions.
func (c *complexityVisitor) registerVisitors(walker *astvisitor.Walker) {
	walker.RegisterEnterArgumentVisitor(c)
	walker.RegisterLeaveFieldVisitor(c)
	walker.RegisterEnterFieldVisitor(c)
	walker.RegisterEnterSelectionSetVisitor(c)
	walker.RegisterEnterFragmentDefinitionVisitor(c)
}

// registerFragmentVisitors registers the callbacks that expand fragment spreads.
func (c *complexityVisitor) registerFragmentVisitors(walker *astvisitor.Walker) {
	walker.RegisterLeaveSelectionSetVisitor(c)
	walker.RegisterEnterFragmentSpreadVisitor(c)
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
		// __typename is an implicit field and need not have a schema definition.
		if bytes.Equal(c.operation.FieldNameBytes(ref), literal.TYPENAME) {
			c.countField(ref, c.operation.FieldNameString(ref))
		}
		return
	}

	if _, skip := c.definition.FieldDefinitionDirectiveByName(definition, nodeCountSkip); skip {
		c.SkipNode()
		return
	}

	if c.skipIntrospection {
		fieldName := c.definition.FieldDefinitionNameBytes(definition)
		if bytes.Equal(fieldName, literal.UNDERSCORESCHEMA) ||
			bytes.Equal(fieldName, literal.UNDERSCORETYPE) {
			c.SkipNode()
			return
		}
	}

	c.countField(ref, c.definition.FieldDefinitionNameString(definition))

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
	parentKind := c.Ancestors[len(c.Ancestors)-1].Kind

	if c.fragments != nil && opensSelectionScope(parentKind) {
		c.fragments.scopeCount++
		c.fragments.scopes = append(c.fragments.scopes, c.fragments.scopeCount)
	}

	// Operation and fragment selection sets do not represent returned nodes.
	if parentKind != ast.NodeKindField {
		return
	}

	c.count = c.count + c.calculateMultiplied(1)
	c.currentRootFieldStats.Stats.NodeCount = c.currentRootFieldStats.Stats.NodeCount + c.calculateMultiplied(1)
}

func (c *complexityVisitor) LeaveSelectionSet(ref int) {
	if c.fragments != nil && opensSelectionScope(c.Ancestors[len(c.Ancestors)-1].Kind) {
		c.fragments.scopes = c.fragments.scopes[:len(c.fragments.scopes)-1]
	}
}

// opensSelectionScope reports whether a selection set with the given parent
// starts a new scope for repeated fragment spreads. Inline fragment and
// fragment definition selections belong to the enclosing scope.
func opensSelectionScope(parentKind ast.NodeKind) bool {
	return parentKind == ast.NodeKindField || parentKind == ast.NodeKindOperationDefinition
}

func (c *complexityVisitor) EnterFragmentSpread(ref int) {
	f := c.fragments
	if f == nil {
		return
	}

	fragmentRef, exists := f.definitions[string(c.operation.FragmentSpreadNameBytes(ref))]
	if !exists {
		return
	}

	// Fragment cycles make the operation invalid; stop expanding at the cycle.
	if f.expanding[fragmentRef] {
		return
	}

	// Field collection merges repeated spreads of a fragment on the same type
	// within one scope. Counting them once also keeps repeated spreads linear.
	expanded := expandedFragment{
		scope:         f.scopes[len(f.scopes)-1],
		enclosingType: c.EnclosingTypeDefinition,
		fragmentRef:   fragmentRef,
	}
	if _, exists := f.expanded[expanded]; exists {
		return
	}
	f.expanded[expanded] = struct{}{}

	if f.depth == maxFragmentDepth {
		c.StopWithExternalErr(operationreport.ExternalError{
			Message: fmt.Sprintf("fragment spread: %s exceeds the maximum fragment nesting depth of %d", c.operation.FragmentSpreadNameBytes(ref), maxFragmentDepth),
		})
		return
	}

	c.expandFragment(fragmentRef)
}

// expandFragment walks a fragment definition in place of its spread. Walkers
// do not follow spreads, so each expansion level has its own fragment walker,
// which the visitor uses while the fragment is walked. The visitor state
// carries through, so the fragment counts like an inline fragment with the
// same type condition and selections.
//
// The work is proportional to the expanded operation: distinct fields that each
// spread the same chain of fragments expand it separately, which is
// exponential in the chain length. Fragment spread inlining during
// normalization has the same cost.
func (c *complexityVisitor) expandFragment(fragmentRef int) {
	f := c.fragments
	if f.depth == len(f.walkers) {
		walker := astvisitor.WalkerFromPool()
		c.registerVisitors(walker)
		c.registerFragmentVisitors(walker)
		f.walkers = append(f.walkers, walker)
	}

	parent := c.Walker
	report := parent.Report
	errorCount := len(report.InternalErrors) + len(report.ExternalErrors)

	// Enclosing fragment walks walk the same document, so their root node is
	// restored afterwards.
	rootNode := f.rootNodes[0]
	f.rootNodes[0] = ast.Node{Kind: ast.NodeKindFragmentDefinition, Ref: fragmentRef}
	f.expanding[fragmentRef] = true
	c.Walker = f.walkers[f.depth]
	f.depth++

	c.Walker.Walk(&f.document, c.definition, report)

	f.depth--
	c.Walker = parent
	f.expanding[fragmentRef] = false
	f.rootNodes[0] = rootNode

	// An aborted fragment walk aborts the enclosing walk, as an aborted inline
	// fragment would.
	if len(report.InternalErrors)+len(report.ExternalErrors) != errorCount {
		parent.Stop()
	}
}

func (c *complexityVisitor) EnterFragmentDefinition(ref int) {
	// Fragments are only counted through their spreads: the operation walker
	// skips every definition, a fragment walker visits only the one it expands.
	if c.fragments == nil || !c.fragments.expanding[ref] {
		c.SkipNode()
	}
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

func (c *complexityVisitor) countField(ref int, fieldName string) {
	if c.isRootTypeField() {
		typeName := c.EnclosingTypeDefinition.NameString(c.definition)
		alias := c.operation.FieldAliasOrNameString(ref)
		if fieldName == alias {
			alias = ""
		}
		c.resetCurrentRootFieldComplexity(typeName, fieldName, alias)
	}

	c.fieldCount++
	c.currentRootFieldStats.Stats.FieldCount++
}

func (c *complexityVisitor) isRootType(name string) bool {
	_, ok := c.rootOperationTypeNames[name]
	return ok
}

func (c *complexityVisitor) isRootTypeField() bool {
	// Root types can also appear beneath other fields in the operation.
	if c.fieldDepth != 0 {
		return false
	}
	enclosingTypeName := c.EnclosingTypeDefinition.NameString(c.definition)
	return c.isRootType(enclosingTypeName)
}
