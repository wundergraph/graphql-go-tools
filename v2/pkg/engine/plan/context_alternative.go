package plan

import (
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// Context bindings add hidden fields to earlier selection sets. Check the
// actual selected prerequisites after field and key selection has converged.
func (p *NodeSelectionBuilder) retryContextOwners(operation *ast.Document, filter *DataSourceFilter) bool {
	v := p.nodeSelectionsVisitor
	if len(v.contextFields) == 0 {
		return false
	}
	check := contextProjectionCheck{
		filter: filter, dependencies: v.fieldDependsOn,
		checked: make(map[int]int), active: make(map[int]bool),
	}
	var retries map[int]int // one owner per field ref
	for key := range v.contextFields {
		node, ok := filter.nodes.responseTree.find(key.fieldRef)
		if !ok {
			continue
		}
		for _, id := range node.suggestionIndexes {
			owner := filter.nodes.items[id]
			if !owner.Selected || owner.IsProvided || owner.DataSourceHash != key.dsHash {
				continue
			}
			retry := check.owner(id, node)
			if retry < 0 {
				continue
			}
			// Let the ordinary planner try compound key jumps before abandoning
			// any otherwise valid candidate.
			if !filter.allowFallbackKeyJumps {
				filter.EnableFallbackKeyJumps()
				return true
			}
			if retries == nil {
				retries = make(map[int]int)
			}
			retries[filter.nodes.items[retry].FieldRef] = retry
		}
	}
	if len(retries) == 0 {
		return false
	}
	// Apply independent decisions together. A failed prerequisite with another
	// owner is retried before its dependent, so viable outer owners survive.
	for _, id := range retries {
		filter.rejectContextCandidate(id)
	}
	p.cleanupAbandonedFieldDependencies(operation)
	return true
}

// A nonnegative result is the index of a failed candidate that has an
// alternative. The two terminal results require no change of ownership.
const (
	contextReachable   = -1
	contextUnreachable = -2
)

type contextProjectionCheck struct {
	filter       *DataSourceFilter
	dependencies map[fieldIndexKey][]int
	checked      map[int]int
	active       map[int]bool
	cycles       int
}

func (c *contextProjectionCheck) field(ref int) int {
	if result, ok := c.checked[ref]; ok {
		return result
	}
	if c.active[ref] {
		// No fetch on this path can run before its own prerequisite. Unwind
		// to the closest field that has another owner.
		c.cycles++
		return contextUnreachable
	}
	cycles := c.cycles
	c.active[ref] = true
	defer delete(c.active, ref)
	result := contextUnreachable
	if node, ok := c.filter.nodes.responseTree.find(ref); ok {
		for _, id := range node.suggestionIndexes {
			if !c.filter.nodes.items[id].Selected {
				continue
			}
			current := c.owner(id, node)
			if current == contextReachable {
				result = current
				break
			}
			if current >= 0 {
				result = current
			}
		}
	}
	// A failed path through an active ancestor can succeed from another
	// entry point when that ancestor also has an independent owner.
	if result == contextReachable || c.cycles == cycles {
		c.checked[ref] = result
	}
	return result
}

func (c *contextProjectionCheck) owner(id int, node *treeField) int {
	owner := c.filter.nodes.items[id]
	if c.filter.canAssignKeys(id, node.parent.suggestionIndexes) {
		result := contextReachable
		for _, ref := range c.dependencies[fieldIndexKey{owner.FieldRef, owner.DataSourceHash}] {
			result = c.field(ref)
			if result != contextReachable {
				break
			}
		}
		if result != contextUnreachable {
			return result
		}
	}
	for _, other := range node.suggestionIndexes {
		candidate := c.filter.nodes.items[other]
		if candidate.DataSourceHash != owner.DataSourceHash && !candidate.IsOrphan && (!candidate.IsExternal || candidate.IsProvided) {
			return id
		}
	}
	return contextUnreachable
}

func (f *DataSourceFilter) rejectContextCandidate(id int) {
	owner := f.nodes.items[id]
	node := f.nodes.treeField(id)
	owner.orphan()
	node.suggestionIndexes = slices.DeleteFunc(node.suggestionIndexes, func(item int) bool { return item == id })
	for _, collector := range f.nodesCollector.dsCollectors {
		if collector.dataSource.Hash() != owner.DataSourceHash {
			continue
		}
		if collector.rejectedContextOwners == nil {
			collector.rejectedContextOwners = make(map[int]struct{})
		}
		collector.rejectedContextOwners[owner.FieldRef] = struct{}{}
		return
	}
}
