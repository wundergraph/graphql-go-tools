// Package recursionguard recursion_guard
// Detect excessive recursion depth in GraphQL queries.
// Uses only helpers available in operation_complexity.go.
package recursionguard

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type RecursionGuard struct {
	maxDepth int
	walker   *astvisitor.Walker
	visitor  *guardVisitor
}

func NewRecursionGuard(maxDepth int) *RecursionGuard {
	w := astvisitor.NewWalker(48)

	v := &guardVisitor{
		Walker:       &w,
		maxDepth:     maxDepth,
		typeCounts:   make(map[string]int, 16),
		visitedStack: make([][]string, 0, 16),
		fieldPath:    make([]string, 0, 32),
	}

	w.RegisterEnterDocumentVisitor(v)
	w.RegisterEnterSelectionSetVisitor(v)
	w.RegisterLeaveSelectionSetVisitor(v)
	w.RegisterEnterFieldVisitor(v)
	w.RegisterLeaveFieldVisitor(v)

	return &RecursionGuard{maxDepth: maxDepth, walker: &w, visitor: v}
}

func (g *RecursionGuard) Do(op, schema *ast.Document, rep *operationreport.Report) {
	for k := range g.visitor.typeCounts {
		delete(g.visitor.typeCounts, k)
	}
	g.visitor.operation, g.visitor.schema, g.visitor.report = op, schema, rep
	g.visitor.fieldPath = g.visitor.fieldPath[:0]
	g.visitor.visitedStack = g.visitor.visitedStack[:0]

	g.walker.Walk(op, schema, rep)
}

type guardVisitor struct {
	*astvisitor.Walker
	operation, schema *ast.Document
	report            *operationreport.Report
	maxDepth          int

	typeCounts   map[string]int
	visitedStack [][]string
	fieldPath    []string
}

func (v *guardVisitor) EnterDocument(
	operation *ast.Document,
	definition *ast.Document,
) {
	v.operation = operation
	v.schema = definition
}

func (v *guardVisitor) EnterSelectionSet(ref int) {
	if len(v.Ancestors) == 0 ||
		v.Ancestors[len(v.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}

	// 1. push a frame for this set
	v.visitedStack = append(v.visitedStack, nil)

	// 2. if this is the first frame (root field), bump its return-type
	if len(v.visitedStack) == 1 {
		parent := v.Ancestors[len(v.Ancestors)-1] // the root field
		def, ok := v.FieldDefinition(parent.Ref)
		if ok {
			// Get the type via field definition
			typeRef := v.schema.FieldDefinitionType(def)

			// Resolve the underlying named type by walking through any non-null or list wrappers
			for v.schema.Types[typeRef].TypeKind != ast.TypeKindNamed {
				typeRef = v.schema.Types[typeRef].OfType
			}

			// Now get the actual name of the type
			typeName := v.schema.TypeNameString(typeRef)

			v.typeCounts[typeName]++
			v.visitedStack[0] = append(v.visitedStack[0], typeName)
		}
	}
}

func (v *guardVisitor) EnterField(ref int) {
	fieldName := v.operation.FieldAliasOrNameString(ref)
	v.fieldPath = append(v.fieldPath, fieldName)

	// Get field type even for fields without selections (like the root field)
	def, ok := v.FieldDefinition(ref)
	if !ok {
		return
	}

	// Get the type via field definition
	typeRef := v.schema.FieldDefinitionType(def)

	// Resolve the underlying named type by walking through any non-null or list wrappers
	for v.schema.Types[typeRef].TypeKind != ast.TypeKindNamed {
		typeRef = v.schema.Types[typeRef].OfType
	}

	// Now get the actual name of the type
	typeName := v.schema.TypeNameString(typeRef)

	// Check if adding this field would exceed max depth
	if v.typeCounts[typeName] >= v.maxDepth {
		v.report.AddExternalError(operationreport.ExternalError{
			Message: fmt.Sprintf(
				"Recursion detected: type %q exceeds allowed depth of %d at path %q",
				typeName, v.maxDepth, strings.Join(v.fieldPath, "."),
			),
		})
	}

	// Always increment the count, even for fields without selections
	v.typeCounts[typeName]++

	// Only continue with visitedStack processing for fields with selections
	if !v.operation.FieldHasSelections(ref) {
		return
	}
	if len(v.visitedStack) == 0 {
		return
	}

	top := len(v.visitedStack) - 1
	v.visitedStack[top] = append(v.visitedStack[top], typeName)
}

func (v *guardVisitor) LeaveField(ref int) {
	// Get the type to decrement its count when leaving
	def, ok := v.FieldDefinition(ref)
	if ok {
		typeRef := v.schema.FieldDefinitionType(def)
		for v.schema.Types[typeRef].TypeKind != ast.TypeKindNamed {
			typeRef = v.schema.Types[typeRef].OfType
		}
		typeName := v.schema.TypeNameString(typeRef)

		v.typeCounts[typeName]--
		if v.typeCounts[typeName] == 0 {
			delete(v.typeCounts, typeName)
		}
	}

	// Only pop the fieldPath for fields without selections
	// (selection-set fields have their path popped in LeaveSelectionSet)
	if !v.operation.FieldHasSelections(ref) {
		v.fieldPath = v.fieldPath[:len(v.fieldPath)-1]
	}
}

func (v *guardVisitor) LeaveSelectionSet(ref int) {
	if len(v.Ancestors) == 0 || v.Ancestors[len(v.Ancestors)-1].Kind != ast.NodeKindField {
		return
	}

	fmt.Println("Leave selection set at path: ", strings.Join(v.fieldPath, "."))
	fmt.Println("  Current type counts: ", v.typeCounts)

	for typ, n := range v.typeCounts {
		if n > v.maxDepth {
			v.report.AddExternalError(operationreport.ExternalError{
				Message: fmt.Sprintf(
					"Recursion detected: type %q exceeds allowed depth of %d at path %q",
					typ, v.maxDepth, strings.Join(v.fieldPath, "."),
				),
			})
			break
		}
	}

	lastIdx := len(v.visitedStack) - 1
	for _, t := range v.visitedStack[lastIdx] {
		v.typeCounts[t]--
		fmt.Printf("  After decrement: type counts for %s = %d\n", t, v.typeCounts[t])
		if v.typeCounts[t] == 0 {
			delete(v.typeCounts, t)
		}
	}
	v.visitedStack = v.visitedStack[:lastIdx]

	v.fieldPath = v.fieldPath[:len(v.fieldPath)-1]
}
