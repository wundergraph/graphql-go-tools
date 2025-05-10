// Package recursion_guard detects excessive recursion depth in GraphQL queries.
package recursion_guard

import (
	"fmt"
	"github.com/wundergraph/graphql-go-tools/pkg/graphql"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type RecursionGuard struct{ MaxDepth int }

func NewRecursionGuard(maxDepth int) *RecursionGuard { return &RecursionGuard{maxDepth} }

func ValidateRecursion(maxDepth int, op, schema *ast.Document, rep *operationreport.Report) graphql.Errors {
	recursionGuard := NewRecursionGuard(maxDepth)
	if maxDepth <= 0 {
		return graphql.RequestErrors{
			{
				Message: "Recursion guard max depth must be greater than 0",
			},
		}
	}

	recursionGuard.Do(op, schema, rep)
	return nil
}

func (g *RecursionGuard) Do(op, schema *ast.Document, rep *operationreport.Report) {
	if g.MaxDepth <= 0 {
		return
	}

	v := &visitor{
		maxDepth:   g.MaxDepth,
		op:         op,
		schema:     schema,
		report:     rep,
		typeCount:  map[string]int{},
		path:       []string{},
		frameStack: []frame{},
	}
	w := astvisitor.NewWalker(48)
	w.RegisterEnterSelectionSetVisitor(v)
	w.RegisterLeaveSelectionSetVisitor(v)
	w.RegisterEnterFieldVisitor(v)
	v.Walker = &w
	w.Walk(op, schema, rep)
}

type frame struct {
	startPath int
	bumped    []string
}

type visitor struct {
	*astvisitor.Walker
	op, schema *ast.Document
	report     *operationreport.Report
	maxDepth   int

	typeCount  map[string]int
	path       []string
	frameStack []frame
	errHit     bool
}

func named(doc *ast.Document, ref int) int {
	// We need this bit to get to the true named type, because in certain cases the name type is deeply embedded
	// into a NOT-NULL or LIST type
	// For eg: Employee!, [Employee], [[Employee!]!]!).
	// The outer wrappers have kinds NON_NULL or LIST; only the innermost node has kind NAMED.
	for doc.Types[ref].TypeKind != ast.TypeKindNamed {
		ref = doc.Types[ref].OfType
	}
	return ref
}

func (v *visitor) EnterSelectionSet(ref int) {
	if len(v.Ancestors) == 0 || v.Ancestors[len(v.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}
	v.frameStack = append(v.frameStack, frame{startPath: len(v.path)})
}

func (v *visitor) LeaveSelectionSet(ref int) {
	if len(v.frameStack) == 0 ||
		len(v.Ancestors) == 0 ||
		v.Ancestors[len(v.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}

	// Here we are computing recursions spotted during this current selection
	for typ, n := range v.typeCount {
		if n > v.maxDepth {
			v.report.AddExternalError(operationreport.ExternalError{
				Message: fmt.Sprintf(
					"Recursion detected: type %q exceeds depth %d at path %q",
					typ, v.maxDepth, strings.Join(v.path, "."),
				),
			})
			v.errHit = true
			break
		}
	}

	// Now we start un-doing the new types we added to the typeCount due to this current selection
	fr := v.frameStack[len(v.frameStack)-1]

	for _, t := range fr.bumped {
		if v.typeCount[t]--; v.typeCount[t] == 0 {
			delete(v.typeCount, t)
		}
	}

	v.path = v.path[:fr.startPath]
	v.frameStack = v.frameStack[:len(v.frameStack)-1]
}

func (v *visitor) EnterField(ref int) {
	if v.errHit {
		return
	}

	v.path = append(v.path, v.op.FieldAliasOrNameString(ref))

	def, ok := v.FieldDefinition(ref)
	if !ok {
		return
	}
	nt := named(v.schema, v.schema.FieldDefinitionType(def))
	typeName := v.schema.TypeNameString(nt)

	// We don't track scalars, enums, unions because they cant contribute to recursions
	node, exists := v.schema.Index.FirstNodeByNameStr(typeName)
	if !exists ||
		(node.Kind != ast.NodeKindObjectTypeDefinition && node.Kind != ast.NodeKindInterfaceTypeDefinition) {
		return // scalar / enum / union
	}

	// We are only interested in the named types that are objects or interfaces
	v.typeCount[typeName]++

	// We save the effects of the current selection as it helps us back track the effects when we are done analysing
	// the current selection
	if len(v.frameStack) > 0 {
		top := &v.frameStack[len(v.frameStack)-1]
		top.bumped = append(top.bumped, typeName)
	}
}
