package responsejsonschema

import "fmt"

// responseSchemaTraversalDepthLimit bounds both selection and schema recursion.
// Valid GraphQL operations are normally shallow; this leaves ample headroom
// while ensuring corrupt borrowed ASTs fail deterministically before consuming
// an unbounded goroutine stack.
const responseSchemaTraversalDepthLimit = 256

type buildTraversal struct {
	depth                     int
	activeSelectionWalks      map[int]struct{}
	activeSchemaSelectionSets map[int]struct{}
}

func newBuildTraversal() *buildTraversal {
	return &buildTraversal{
		activeSelectionWalks:      make(map[int]struct{}),
		activeSchemaSelectionSets: make(map[int]struct{}),
	}
}

func (t *buildTraversal) enterDepth(description string) (func(), error) {
	if t == nil {
		return nil, fmt.Errorf("response schema traversal is unavailable")
	}
	if t.depth >= responseSchemaTraversalDepthLimit {
		return nil, fmt.Errorf(
			"response schema recursion depth limit %d exceeded while traversing %s",
			responseSchemaTraversalDepthLimit,
			description,
		)
	}
	t.depth++
	return func() { t.depth-- }, nil
}

func (t *buildTraversal) enterSelectionWalk(selectionSetRef int) (func(), error) {
	if _, active := t.activeSelectionWalks[selectionSetRef]; active {
		return nil, fmt.Errorf("selection set cycle detected at reference %d", selectionSetRef)
	}
	leaveDepth, err := t.enterDepth("selection sets")
	if err != nil {
		return nil, err
	}
	t.activeSelectionWalks[selectionSetRef] = struct{}{}
	return func() {
		delete(t.activeSelectionWalks, selectionSetRef)
		leaveDepth()
	}, nil
}

func (t *buildTraversal) enterSchemaSelectionSets(selectionSetRefs []int) (func(), error) {
	owned := make(map[int]struct{}, len(selectionSetRefs))
	for _, selectionSetRef := range selectionSetRefs {
		if _, duplicate := owned[selectionSetRef]; duplicate {
			continue
		}
		if _, active := t.activeSchemaSelectionSets[selectionSetRef]; active {
			return nil, fmt.Errorf("selection set cycle detected at reference %d during schema traversal", selectionSetRef)
		}
		owned[selectionSetRef] = struct{}{}
	}
	for selectionSetRef := range owned {
		t.activeSchemaSelectionSets[selectionSetRef] = struct{}{}
	}
	return func() {
		for selectionSetRef := range owned {
			delete(t.activeSchemaSelectionSets, selectionSetRef)
		}
	}, nil
}
