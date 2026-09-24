package astvisitor_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const skipForDefinition = `
	schema { query: Query }
	type Query {
		root: Root
		node: Node
	}
	type Root {
		a: Leaf
		b: Leaf
	}
	type Leaf {
		x: String
		y: String
	}
	interface Node {
		id: ID
	}
	type NodeA implements Node {
		id: ID
		na: Leaf
	}
	type NodeB implements Node {
		id: ID
		nb: Leaf
	}
`

// recordingPlanner mimics a datasource planner: a VisitorIdentifier keeping a node stack
// pushed on enter and popped on leave - the invariant the walker has to keep balanced
type recordingPlanner struct {
	walker *astvisitor.Walker
	op     *ast.Document

	id int

	trace           []string
	fieldStack      []int
	ancestryAtEnter []string
	unbalanced      []string
}

func (r *recordingPlanner) ID() int      { return r.id }
func (r *recordingPlanner) SetID(id int) { r.id = id }
func (r *recordingPlanner) name(ref int) string {
	return r.op.FieldAliasOrNameString(ref)
}

func (r *recordingPlanner) path(ref int) string {
	return r.walker.Path.DotDelimitedString() + "." + r.op.FieldAliasOrNameString(ref)
}

func (r *recordingPlanner) EnterField(ref int) {
	r.trace = append(r.trace, "enter:"+r.path(ref))
	r.fieldStack = append(r.fieldStack, ref)
	ancestors := make([]string, 0, len(r.walker.Ancestors))
	for _, a := range r.walker.Ancestors {
		ancestors = append(ancestors, a.Kind.String())
	}
	r.ancestryAtEnter = append(r.ancestryAtEnter, strings.Join(ancestors, ">"))
}

func (r *recordingPlanner) LeaveField(ref int) {
	r.trace = append(r.trace, "leave:"+r.path(ref))
	if len(r.fieldStack) == 0 {
		r.unbalanced = append(r.unbalanced, fmt.Sprintf("LeaveField(%s) with empty stack", r.path(ref)))
		return
	}
	top := r.fieldStack[len(r.fieldStack)-1]
	if top != ref {
		r.unbalanced = append(r.unbalanced, fmt.Sprintf("LeaveField(%s) but stack top is %s", r.path(ref), r.name(top)))
	}
	r.fieldStack = r.fieldStack[:len(r.fieldStack)-1]
}

func (r *recordingPlanner) EnterSelectionSet(ref int) {
	r.trace = append(r.trace, "enterSS")
}

func (r *recordingPlanner) LeaveSelectionSet(ref int) {
	r.trace = append(r.trace, "leaveSS")
}

func (r *recordingPlanner) EnterInlineFragment(ref int) {
	r.trace = append(r.trace, "enterIF:"+r.op.InlineFragmentTypeConditionNameString(ref))
}

func (r *recordingPlanner) LeaveInlineFragment(ref int) {
	r.trace = append(r.trace, "leaveIF:"+r.op.InlineFragmentTypeConditionNameString(ref))
}

// pathFilter mimics plan.Visitor.AllowVisitor: the field decision is a pure per-path lookup
// which does not consult skipFor, every other node kind inherits the ancestor decision
type pathFilter struct {
	walker       *astvisitor.Walker
	op           *ast.Document
	allowedPaths map[string]bool
}

func (f *pathFilter) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor any, skipFor astvisitor.SkipVisitors) bool {
	if visitor == f {
		return true
	}
	switch kind {
	case astvisitor.EnterField, astvisitor.LeaveField:
		path := f.walker.Path.DotDelimitedString() + "." + f.op.FieldAliasOrNameString(ref)
		return f.allowedPaths[path]
	default:
		return skipFor.Allow(visitor)
	}
}

func runSkipForCase(t *testing.T, operation string, allowed map[string]bool) *recordingPlanner {
	t.Helper()

	def := unsafeparser.ParseGraphqlDocumentString(skipForDefinition)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&def))
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	report := operationreport.Report{}

	walker := astvisitor.NewWalker(48)
	planner := &recordingPlanner{walker: &walker, op: &op}
	filter := &pathFilter{walker: &walker, op: &op, allowedPaths: allowed}

	walker.RegisterFieldVisitor(planner)
	walker.RegisterSelectionSetVisitor(planner)
	walker.RegisterInlineFragmentVisitor(planner)
	walker.SetVisitorFilter(filter)

	walker.Walk(&op, &def, &report)
	require.False(t, report.HasErrors(), "%v", report)

	return planner
}

func TestSkipForExplicitAllowUnderSkippedAncestor(t *testing.T) {
	planner := runSkipForCase(t,
		`query { root { a { x } b { y } } }`,
		map[string]bool{
			"query.root.a":   true,
			"query.root.a.x": true,
		})

	assert.Empty(t, planner.unbalanced, "enter/leave must stay balanced")
	assert.Empty(t, planner.fieldStack, "planner node stack must be empty at the end of the walk")

	assert.Equal(t, []string{
		"enter:query.root.a",
		"enter:query.root.a.x",
		"leave:query.root.a.x",
		"leave:query.root.a",
	}, fieldTrace(planner.trace))

	assert.Equal(t,
		"NodeKindOperationDefinition>NodeKindSelectionSet>NodeKindField>NodeKindSelectionSet",
		planner.ancestryAtEnter[0])
}

func TestSkipForDenyThenAllowSiblings(t *testing.T) {
	planner := runSkipForCase(t,
		`query { root { a { x y } b { x y } } }`,
		map[string]bool{
			"query.root":     false,
			"query.root.a":   true,
			"query.root.a.y": true,
			"query.root.b":   true,
			"query.root.b.x": true,
		})

	assert.Empty(t, planner.unbalanced)
	assert.Empty(t, planner.fieldStack)
	assert.Equal(t, []string{
		"enter:query.root.a",
		"enter:query.root.a.y",
		"leave:query.root.a.y",
		"leave:query.root.a",
		"enter:query.root.b",
		"enter:query.root.b.x",
		"leave:query.root.b.x",
		"leave:query.root.b",
	}, fieldTrace(planner.trace))
}

func TestSkipForNestedFragmentsMixedAllows(t *testing.T) {
	planner := runSkipForCase(t,
		`query {
			node {
				id
				... on NodeA { id na { x } }
				... on NodeB { nb { y } }
			}
		}`,
		map[string]bool{
			"query.node":              false,
			"query.node.id":           true,
			"query.node.$0NodeA.na":   true,
			"query.node.$0NodeA.na.x": true,
			"query.node.$1NodeB.nb":   true,
		})

	assert.Empty(t, planner.unbalanced)
	assert.Empty(t, planner.fieldStack)
	assert.Equal(t, []string{
		"enter:query.node.id",
		"leave:query.node.id",
		"enter:query.node.$0NodeA.na",
		"enter:query.node.$0NodeA.na.x",
		"leave:query.node.$0NodeA.na.x",
		"leave:query.node.$0NodeA.na",
		"enter:query.node.$1NodeB.nb",
		"leave:query.node.$1NodeB.nb",
	}, fieldTrace(planner.trace))
}

func TestSkipForDeepestAllowUnderTwoDeniedAncestors(t *testing.T) {
	planner := runSkipForCase(t,
		`query { root { a { x y } } }`,
		map[string]bool{
			"query.root.a.y": true,
		})

	assert.Empty(t, planner.unbalanced)
	assert.Empty(t, planner.fieldStack)
	assert.Equal(t, []string{
		"enter:query.root.a.y",
		"leave:query.root.a.y",
	}, fieldTrace(planner.trace))
}

func fieldTrace(trace []string) []string {
	out := make([]string, 0, len(trace))
	for _, t := range trace {
		if strings.HasPrefix(t, "enter:") || strings.HasPrefix(t, "leave:") {
			out = append(out, t)
		}
	}
	return out
}
