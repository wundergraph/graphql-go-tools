package recursion_depth_limiter

import (
	"fmt"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// LimitRecursionDepth traverses the selection set and keeps track of how many times each object has been visited
// once an object reaches the maxRecursionDepth limit the traversing will stop and an error will be returned
func LimitRecursionDepth(definition, operation *ast.Document, maxRecursionDepth int) error {
	walker := astvisitor.NewWalker(48)
	report := &operationreport.Report{}
	visitor := &recursionDepthLimiter{
		walker:            &walker,
		operation:         operation,
		definition:        definition,
		objectVisited:     make(map[int]int),
		maxRecursionDepth: maxRecursionDepth,
	}

	walker.RegisterFieldVisitor(visitor)
	walker.Walk(operation, definition, report)

	if report.HasErrors() {
		return report
	}

	return nil
}

type recursionDepthLimiter struct {
	walker            *astvisitor.Walker
	operation         *ast.Document
	definition        *ast.Document
	objectVisited     map[int]int
	maxRecursionDepth int
}

func (c *recursionDepthLimiter) EnterField(ref int) {
	def, ok := c.walker.FieldDefinition(ref)
	if !ok {
		return
	}

	c.objectVisited[def]++
	if c.objectVisited[def] > c.maxRecursionDepth {
		if def >= len(c.definition.FieldDefinitions) {
			// this should never happen, if it gets to this point it's a problem in the implementation code
			c.walker.Report.AddInternalError(fmt.Errorf("referenced field not in definitions"))
			c.walker.Stop()
		}

		objectTypeName := c.definition.TypeNameString(c.definition.FieldDefinitions[def].Type)
		// push the current field to the path
		path := append(c.walker.Path, ast.PathItem{
			Kind:      ast.FieldName,
			FieldName: c.operation.FieldAliasOrNameBytes(ref),
		})
		c.walker.Report.AddExternalError(operationreport.ExternalError{
			Message: fmt.Sprintf("Recursion detected: type '%s' exceeds allowed depth of %d", objectTypeName, c.maxRecursionDepth),
			Path:    path,
		})
		// stop traversing the node any deeper to not waste unnecessary resources and prevent infinite recursion
		c.walker.Stop()
		return
	}
}

func (c *recursionDepthLimiter) LeaveField(ref int) {
	def, ok := c.walker.FieldDefinition(ref)
	if !ok {
		return
	}

	c.objectVisited[def]--
}
