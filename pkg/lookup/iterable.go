package lookup

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"sort"
)

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

type FragmentDefinitionIterable struct {
	Iterable
}

func (f *FragmentDefinitionIterable) Value() document.FragmentDefinition {
	return f.w.l.FragmentDefinition(f.node().Ref)
}

func (w *Walker) FragmentDefinitionIterable() FragmentDefinitionIterable {
	return FragmentDefinitionIterable{
		Iterable: w.newIterable(w.c.fragmentDefinitions),
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

func (s *SelectionSetIterable) Value() (set document.SelectionSet, nodeRef, setRef, parent int) {
	node := s.node()
	nodeRef = s.ref()
	parent = node.Parent
	setRef = node.Ref
	set = s.w.l.SelectionSet(setRef)
	return
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

type TypeSystemDefinitionOrderedRootNodes struct {
	Iterable
}

func (t *TypeSystemDefinitionOrderedRootNodes) Value() (ref int, kind NodeKind) {
	node := t.node()
	ref = node.Ref
	kind = node.Kind
	return
}

func (w *Walker) TypeSystemDefinitionOrderedRootNodes() TypeSystemDefinitionOrderedRootNodes {

	refs := w.c.rootNodes[:0]

	for i := range w.nodes {
		if w.nodes[i].Parent == -1 {
			refs = append(refs, i)
		}
	}

	sort.Slice(refs, func(i, j int) bool {
		left := refs[i]
		right := refs[j]
		return w.nodes[left].Position.LineStart < w.nodes[right].Position.LineStart
	})

	return TypeSystemDefinitionOrderedRootNodes{
		Iterable: w.newIterable(refs),
	}
}

type FieldsContainingDirectiveIterator struct {
	fieldRefs                []int
	directives               []int
	objectTypeDefinitionRefs []int
	current                  int
}

func (f *FieldsContainingDirectiveIterator) Next() bool {
	f.current++
	return len(f.fieldRefs)-1 >= f.current
}

func (f *FieldsContainingDirectiveIterator) Value() (fieldDefinitionRef, objectTypeDefinitionRef, directiveRef int) {
	fieldDefinitionRef = f.fieldRefs[f.current]
	objectTypeDefinitionRef = f.objectTypeDefinitionRefs[f.current]
	directiveRef = f.directives[f.current]
	return
}

func (w *Walker) FieldsContainingDirectiveIterator(directiveNameRef int) FieldsContainingDirectiveIterator {

	fields := w.c.fieldsContainingDirectiveFields[:0]
	objects := w.c.fieldsContainingDirectiveObjects[:0]
	directiveRefs := w.c.fieldsContainingDirectiveDirectives[:0]

	sets := w.DirectiveSetIterable()
	for sets.Next() {
		set, parent := sets.Value()
		directives := w.l.DirectiveIterable(set)
		for directives.Next() {
			directive, directiveRef := directives.Value()
			if directive.Name != directiveNameRef {
				continue
			}

			field := w.Node(parent)
			if field.Kind != FIELD_DEFINITION {
				continue
			}

			fieldType := w.Node(field.Parent)
			if fieldType.Kind != OBJECT_TYPE_DEFINITION {
				continue
			}

			fields = append(fields, field.Ref)
			objects = append(objects, fieldType.Ref)
			directiveRefs = append(directiveRefs, directiveRef)
		}
	}

	return FieldsContainingDirectiveIterator{
		current:                  -1,
		objectTypeDefinitionRefs: objects,
		fieldRefs:                fields,
		directives:               directiveRefs,
	}
}
