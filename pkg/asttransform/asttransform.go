package asttransform

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"sort"
)

type (
	Transformable interface {
		DeleteRootNode(node ast.Node)
		EmptySelectionSet(ref int)
		AppendSelectionSet(ref int, appendRef int)
		ReplaceFragmentSpread(selectionSet int, spreadRef int, replaceWithSelectionSet int)
		ReplaceFragmentSpreadWithInlineFragment(selectionSet int, spreadRef int, replaceWithSelectionSet int, typeCondition ast.TypeCondition)
	}
	transformation interface {
		apply(transformable Transformable)
	}
	Precedence struct {
		Depth int
		Order int
		last  int
	}
	action struct {
		precedence     Precedence
		transformation transformation
	}
	Transformer struct {
		actions []action
	}
)

func (p *Precedence) Next() Precedence {
	p.last++
	return Precedence{
		Depth: p.Depth,
		Order: p.last,
	}
}

func (t *Transformer) Reset() {
	t.actions = t.actions[:0]
}

func (t *Transformer) ApplyTransformations(transformable Transformable) {

	sort.Slice(t.actions, func(i, j int) bool {
		if t.actions[i].precedence.Depth != t.actions[j].precedence.Depth {
			return t.actions[i].precedence.Depth > t.actions[j].precedence.Depth
		}
		return t.actions[i].precedence.Order < t.actions[j].precedence.Order
	})

	for i := range t.actions {
		t.actions[i].transformation.apply(transformable)
	}
}

func (t *Transformer) DeleteRootNode(precedence Precedence, node ast.Node) {
	t.actions = append(t.actions, action{
		precedence:     precedence,
		transformation: deleteRootNode{node: node},
	})
}

func (t *Transformer) EmptySelectionSet(precedence Precedence, ref int) {
	t.actions = append(t.actions, action{
		precedence:     precedence,
		transformation: emptySelectionSet{ref: ref},
	})
}

func (t *Transformer) AppendSelectionSet(precedence Precedence, ref int, appendRef int) {
	t.actions = append(t.actions, action{
		precedence: precedence,
		transformation: appendSelectionSet{
			ref:       ref,
			appendRef: appendRef,
		},
	})
}

func (t *Transformer) ReplaceFragmentSpread(precedence Precedence, selectionSet int, spreadRef int, replaceWithSelectionSet int) {
	t.actions = append(t.actions, action{
		precedence: precedence,
		transformation: replaceFragmentSpread{
			selectionSet:            selectionSet,
			spreadRef:               spreadRef,
			replaceWithSelectionSet: replaceWithSelectionSet,
		},
	})
}

func (t *Transformer) ReplaceFragmentSpreadWithInlineFragment(precedence Precedence, selectionSet int, spreadRef int, replaceWithSelectionSet int, typeCondition ast.TypeCondition) {
	t.actions = append(t.actions, action{
		precedence: precedence,
		transformation: replaceFragmentSpreadWithInlineFragment{
			selectionSet:            selectionSet,
			spreadRef:               spreadRef,
			replaceWithSelectionSet: replaceWithSelectionSet,
			typeCondition:           typeCondition,
		},
	})
}

type replaceFragmentSpread struct {
	selectionSet            int
	spreadRef               int
	replaceWithSelectionSet int
}

func (r replaceFragmentSpread) apply(transformable Transformable) {
	transformable.ReplaceFragmentSpread(r.selectionSet, r.spreadRef, r.replaceWithSelectionSet)
}

type replaceFragmentSpreadWithInlineFragment struct {
	selectionSet            int
	spreadRef               int
	replaceWithSelectionSet int
	typeCondition           ast.TypeCondition
}

func (r replaceFragmentSpreadWithInlineFragment) apply(transformable Transformable) {
	transformable.ReplaceFragmentSpreadWithInlineFragment(r.selectionSet, r.spreadRef, r.replaceWithSelectionSet, r.typeCondition)
}

type deleteRootNode struct {
	node ast.Node
}

func (d deleteRootNode) apply(transformable Transformable) {
	transformable.DeleteRootNode(d.node)
}

type emptySelectionSet struct {
	ref int
}

func (e emptySelectionSet) apply(transformable Transformable) {
	transformable.EmptySelectionSet(e.ref)
}

type appendSelectionSet struct {
	ref       int
	appendRef int
}

func (a appendSelectionSet) apply(transformable Transformable) {
	transformable.AppendSelectionSet(a.ref, a.appendRef)
}
