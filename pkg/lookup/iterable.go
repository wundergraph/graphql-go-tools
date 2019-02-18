package lookup

import "github.com/jensneuse/graphql-go-tools/pkg/document"

type Iterable struct {
	current int
	refs    []int
	w       *Walker
}

func (i *Iterable) Next() bool {
	i.current++
	return len(i.refs)-1 >= i.current
}

func (i *Iterable) ref() int {
	return i.refs[i.current]
}

func (i *Iterable) node() Node {
	node := i.w.nodes[i.ref()]
	return node
}

func (w *Walker) newIterable(refs []int) Iterable {
	return Iterable{
		refs:    refs,
		current: -1,
		w:       w,
	}
}

type OperationDefinitionIterable struct {
	Iterable
}

func (o *OperationDefinitionIterable) Value() document.OperationDefinition {
	return o.w.l.OperationDefinition(o.node().Ref)
}

func (w *Walker) OperationDefinitionIterable() OperationDefinitionIterable {
	return OperationDefinitionIterable{
		Iterable: w.newIterable(w.c.operationDefinition),
	}
}

type ArgumentSetIterable struct {
	Iterable
}

func (a *ArgumentSetIterable) Value() (argumentSet document.ArgumentSet, parent int) {
	node := a.node()
	return a.w.l.ArgumentSet(node.Ref), node.Parent
}

func (w *Walker) ArgumentSetIterable() ArgumentSetIterable {
	return ArgumentSetIterable{
		Iterable: w.newIterable(w.c.argumentSets),
	}
}

type DirectiveSetIterable struct {
	Iterable
}

func (d *DirectiveSetIterable) Value() (set document.DirectiveSet, parent int) {
	node := d.node()
	return d.w.l.DirectiveSet(node.Ref), node.Parent
}

func (w *Walker) DirectiveSetIterable() DirectiveSetIterable {
	return DirectiveSetIterable{
		Iterable: w.newIterable(w.c.directiveSets),
	}
}

type SelectionSetIterable struct {
	Iterable
}

func (s *SelectionSetIterable) Value() (set document.SelectionSet, ref, parent int) {
	node := s.node()
	return s.w.l.SelectionSet(node.Ref), s.ref(), node.Parent
}

func (w *Walker) SelectionSetIterable() SelectionSetIterable {
	return SelectionSetIterable{
		Iterable: w.newIterable(w.c.selectionSets),
	}
}

type FieldsIterable struct {
	Iterable
}

func (f *FieldsIterable) Value() (field document.Field, ref, parent int) {
	ref = f.node().Ref
	parent = f.node().Parent
	field = f.w.l.Field(ref)
	return
}

func (w *Walker) FieldsIterable() FieldsIterable {
	return FieldsIterable{
		Iterable: w.newIterable(w.c.fields),
	}
}
