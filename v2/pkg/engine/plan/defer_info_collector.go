package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// deferInfoCollector records, per defer id, a DeferDescriptor describing
// where the @defer fragment is mounted (path), its label, and parent id.
//
// It registers as an EnterSelectionSet visitor: when a selection set is
// entered, every direct field child is inspected; the first time we see
// a given defer id on any direct child, that selection set's path becomes
// the candidate descriptor path. The candidate is then truncated at the
// outermost list-typed field ancestor. Any field not on a direct child of this
// selection set is ignored — its defer id will be picked up at a more
// specific selection set higher in the document order.
type deferInfoCollector struct {
	*astvisitor.Walker
	operation   *ast.Document
	definition  *ast.Document
	descriptors map[int]resolve.DeferDescriptor
}

func registerDeferInfoCollector(walker *astvisitor.Walker) *deferInfoCollector {
	c := &deferInfoCollector{Walker: walker}
	walker.RegisterEnterDocumentVisitor(c)
	walker.RegisterEnterSelectionSetVisitor(c)
	return c
}

func (c *deferInfoCollector) EnterDocument(operation, definition *ast.Document) {
	c.operation = operation
	c.definition = definition
	c.descriptors = make(map[int]resolve.DeferDescriptor)
}

func (c *deferInfoCollector) EnterSelectionSet(ref int) {
	for _, selectionRef := range c.operation.SelectionSetFieldSelections(ref) {
		fieldRef := c.operation.Selections[selectionRef].Ref
		id, label, parentID, ok := c.operation.FieldDeferInfo(fieldRef)
		if !ok {
			continue
		}
		if _, seen := c.descriptors[id]; seen {
			continue
		}
		c.descriptors[id] = resolve.DeferDescriptor{
			ID:       id,
			ParentID: parentID,
			Label:    label,
			Path:     c.deferPath(),
		}
	}
}

// deferPath returns the response path of the inline fragment for the defer
// being recorded. The candidate path is built from c.Walker.Path (skipping
// the operation-type root segment), then truncated at the outermost
// list-typed field ancestor — a deliberate deviation from spec that lets a
// single defer id span all runtime list iterations, with subPath
// disambiguating each individual item at render time.
func (c *deferInfoCollector) deferPath() []string {
	if len(c.Walker.Path) <= 1 {
		return nil
	}
	candidate := make([]string, 0, len(c.Walker.Path)-1)
	for i := 1; i < len(c.Walker.Path); i++ {
		item := c.Walker.Path[i]
		if item.Kind != ast.FieldName {
			continue
		}
		candidate = append(candidate, string(item.FieldName))
	}
	if len(candidate) == 0 {
		return nil
	}

	listIdx := c.outermostListFieldIndex()
	if listIdx >= 0 && listIdx+1 < len(candidate) {
		candidate = candidate[:listIdx+1]
	}
	return candidate
}

// outermostListFieldIndex returns the field-only index (0-based, parallel
// to the candidate path) of the outermost Field ancestor whose schema type
// is a list (possibly NonNull-wrapped). Returns -1 when no Field ancestor
// is list-typed
func (c *deferInfoCollector) outermostListFieldIndex() int {
	if len(c.Walker.TypeDefinitions) == 0 {
		return -1
	}
	parentType := c.Walker.TypeDefinitions[0]
	fieldIdx := -1
	for _, ancestor := range c.Walker.Ancestors {
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		fieldIdx++
		fdRef, ok := c.definition.NodeFieldDefinitionByName(parentType, c.operation.FieldNameBytes(ancestor.Ref))
		if !ok {
			return -1
		}
		if c.definition.TypeIsList(c.definition.FieldDefinitionType(fdRef)) {
			return fieldIdx
		}
		next, ok := c.definition.NodeByName(c.definition.FieldDefinitionTypeNameBytes(fdRef))
		if !ok {
			return -1
		}
		parentType = next
	}
	return -1
}
