//go:generate go-enum -f=$GOFILE --noprefix
package lookup

import "github.com/jensneuse/graphql-go-tools/pkg/document"

/*
ENUM(
NONE
OPERATION_DEFINITION
DIRECTIVE
DIRECTIVE_SET
SELECTION_SET
FIELD
INLINE_FRAGMENT
FRAGMENT_SPREAD
FRAGMENT_DEFINITION
ARGUMENT_SET
ARGUMENT
)
*/
type NodeKind int

type Node struct {
	Parent int
	Kind   NodeKind
	Ref    int
}

type Walker struct {
	nodes []Node
	l     *Lookup
	c     walkerCache
}

type walkerCache struct {
	operationDefinition []int
	argumentSets        []int
	arguments           []int
	directives          []int
	directiveSets       []int
	selectionSets       []int
	fields              []int

	path []int
}

func NewWalker(nodeCacheSize int, defaultCacheSize int) *Walker {
	return &Walker{
		nodes: make([]Node, 0, nodeCacheSize),
		c: walkerCache{
			operationDefinition: make([]int, 0, defaultCacheSize),
			argumentSets:        make([]int, 0, defaultCacheSize),
			arguments:           make([]int, 0, defaultCacheSize),
			directives:          make([]int, 0, defaultCacheSize),
			directiveSets:       make([]int, 0, defaultCacheSize),
			selectionSets:       make([]int, 0, defaultCacheSize),
			fields:              make([]int, 0, defaultCacheSize),
			path:                make([]int, 16),
		},
	}
}

func (w *Walker) SetLookup(l *Lookup) {

	w.l = l
	w.nodes = w.nodes[:0]

	w.c.operationDefinition = w.c.operationDefinition[:0]
	w.c.argumentSets = w.c.argumentSets[:0]
	w.c.arguments = w.c.arguments[:0]
	w.c.directives = w.c.directives[:0]
	w.c.directiveSets = w.c.directiveSets[:0]
	w.c.selectionSets = w.c.selectionSets[:0]
	w.c.fields = w.c.fields[:0]
}

func (w *Walker) putNode(node Node) int {
	w.nodes = append(w.nodes, node)
	return len(w.nodes) - 1
}

func (w *Walker) Parent(i int) (Node, bool) {
	if i == -1 {
		return Node{}, false
	}
	return w.nodes[i], true
}

func (w *Walker) Node(ref int) Node {
	return w.nodes[ref]
}

func (w *Walker) WalkExecutable() {
	for i, fragment := range w.l.FragmentDefinitions() {
		w.walkFragmentDefinition(fragment, i, -1)
	}
	for i, operation := range w.l.OperationDefinitions() {
		w.walkOperationDefinition(operation, i)
	}
}

func (w *Walker) walkOperationDefinition(definition document.OperationDefinition, i int) {

	ref := w.putNode(Node{
		Parent: -1,
		Kind:   OPERATION_DEFINITION,
		Ref:    i,
	})

	w.c.operationDefinition = append(w.c.operationDefinition, ref)

	w.walkDirectiveSet(definition.DirectiveSet, ref)
	w.walkSelectionSet(definition.SelectionSet, ref)
}

func (w *Walker) walkDirectiveSet(set int, parent int) {

	ref := w.putNode(Node{
		Parent: parent,
		Kind:   DIRECTIVE_SET,
		Ref:    set,
	})

	w.c.directiveSets = append(w.c.directiveSets, ref)

	directiveSet := w.l.DirectiveSet(set)
	for _, directive := range directiveSet {
		w.walkDirective(directive, ref)
	}
}

func (w *Walker) walkDirective(directive int, parent int) {

	dir := w.l.Directive(directive)

	ref := w.putNode(Node{
		Parent: parent,
		Kind:   DIRECTIVE,
		Ref:    directive,
	})

	w.c.directives = append(w.c.directives, ref)
	w.walkArgumentSet(dir.ArgumentSet, ref)
}

func (w *Walker) walkArgumentSet(set int, parent int) {

	arguments := w.l.ArgumentSet(set)
	if len(arguments) == 0 {
		return
	}

	ref := w.putNode(Node{
		Parent: parent,
		Ref:    set,
		Kind:   ARGUMENT_SET,
	})

	w.c.argumentSets = append(w.c.argumentSets, ref)

	for _, argument := range arguments {
		w.walkArgument(argument, ref)
	}
}

func (w *Walker) walkArgument(argument, parent int) {

	ref := w.putNode(Node{
		Parent: parent,
		Ref:    argument,
		Kind:   ARGUMENT,
	})

	w.c.arguments = append(w.c.arguments, ref)
}

func (w *Walker) walkSelectionSet(setRef, parent int) {

	set := w.l.SelectionSet(setRef)
	if set.IsEmpty() {
		return
	}

	ref := w.putNode(Node{
		Parent: parent,
		Ref:    setRef,
		Kind:   SELECTION_SET,
	})

	w.c.selectionSets = append(w.c.selectionSets, ref)

	w.walkFields(set.Fields, ref)
	w.walkInlineFragments(set.InlineFragments, ref)
	w.walkFragmentSpreads(set.FragmentSpreads, ref)
}

func (w *Walker) walkFields(i []int, parent int) {
	iter := w.l.FieldsIterator(i)
	for iter.Next() {

		field, index := iter.Value()

		ref := w.putNode(Node{
			Parent: parent,
			Kind:   FIELD,
			Ref:    index,
		})

		w.c.fields = append(w.c.fields, ref)

		w.walkDirectiveSet(field.DirectiveSet, ref)
		w.walkArgumentSet(field.ArgumentSet, ref)
		w.walkSelectionSet(field.SelectionSet, ref)
	}
}

func (w *Walker) walkInlineFragments(refs []int, parent int) {
	fragments := w.l.InlineFragmentIterable(refs)
	for fragments.Next() {
		fragment, fragmentRef := fragments.Value()
		ref := w.putNode(Node{
			Parent: parent,
			Kind:   INLINE_FRAGMENT,
			Ref:    fragmentRef,
		})
		w.walkDirectiveSet(fragment.DirectiveSet, ref)
		w.walkSelectionSet(fragment.SelectionSet, ref)
	}
}

func (w *Walker) walkFragmentSpreads(refs []int, parent int) {
	spreads := w.l.FragmentSpreadIterable(refs)
	for spreads.Next() {
		spread, spreadRef := spreads.Value()
		ref := w.putNode(Node{
			Parent: parent,
			Kind:   FRAGMENT_SPREAD,
			Ref:    spreadRef,
		})

		w.walkDirectiveSet(spread.DirectiveSet, ref)

		if w.referenceFormsCycle(FRAGMENT_SPREAD, spreadRef, parent) {
			continue
		}

		fragmentDefinition, index, ok := w.l.FragmentDefinitionByName(spread.FragmentName)
		if ok {
			w.walkFragmentDefinition(fragmentDefinition, index, ref)
		}
	}
}

func (w *Walker) referenceFormsCycle(kind NodeKind, ref, parent int) bool {
	for {
		node, hasParent := w.Parent(parent)
		if !hasParent {
			return false
		}
		if node.Kind == kind && ref == node.Ref {
			return true
		}
		parent = node.Parent
	}
}

func (w *Walker) walkFragmentDefinition(definition document.FragmentDefinition, index int, parent int) {
	ref := w.putNode(Node{
		Parent: parent,
		Kind:   FRAGMENT_DEFINITION,
		Ref:    index,
	})
	w.walkDirectiveSet(definition.DirectiveSet, ref)
	w.walkSelectionSet(definition.SelectionSet, ref)
}

func (w *Walker) ParentEquals(parent int, kind NodeKind) (Node, bool) {
	p, ok := w.Parent(parent)
	return p, ok && p.Kind == kind
}

func (w *Walker) ArgumentsDefinition(parent int) document.ArgumentsDefinition {

	var typeName int
	path := w.c.path[:0]
	node, ok := w.ParentEquals(parent, FIELD)
	if ok {
		field := w.l.Field(node.Ref)
		typeName, node = w.WalkUpUntilTypeName(node, &path)

		parentTypeName := w.resolveTypeName(typeName, path[1:])
		parentFields := w.l.FieldsDefinitionFromNamedType(parentTypeName)
		fieldDefinition, ok := w.l.FieldDefinitionByNameFromIndex(parentFields, field.Name)
		if !ok {
			return document.ArgumentsDefinition{}
		}

		if fieldDefinition.ArgumentsDefinition == -1 {
			return document.ArgumentsDefinition{}
		}
		return w.l.ArgumentsDefinition(fieldDefinition.ArgumentsDefinition)
	}

	directiveNode, ok := w.ParentEquals(parent, DIRECTIVE)
	if ok {
		directive := w.l.Directive(directiveNode.Ref)
		definition, ok := w.l.DirectiveDefinitionByName(directive.Name)
		if ok {
			return w.l.ArgumentsDefinition(definition.ArgumentsDefinition)
		}
	}

	return document.ArgumentsDefinition{}
}

func (w *Walker) WalkUpUntilTypeName(from Node, fieldPath *[]int) (typeName int, node Node) {

	node = from

	for {
		switch node.Kind {
		case INLINE_FRAGMENT:
			inline := w.l.InlineFragment(node.Ref)
			if inline.TypeCondition == -1 {
				break
			}
			typeName = w.l.Type(inline.TypeCondition).Name
			return typeName, node
		case FRAGMENT_DEFINITION:
			fragmentDefinition := w.l.FragmentDefinition(node.Ref)
			typeName = w.l.Type(fragmentDefinition.TypeCondition).Name
			return typeName, node
		case OPERATION_DEFINITION:
			operationDefinition := w.l.OperationDefinition(node.Ref)
			switch operationDefinition.OperationType {
			case document.OperationTypeQuery:
				typeName = w.l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Query
			case document.OperationTypeMutation:
				typeName = w.l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Mutation
			case document.OperationTypeSubscription:
				typeName = w.l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Subscription
			}
			return typeName, node
		case FIELD:
			field := w.l.Field(node.Ref)
			*fieldPath = append(*fieldPath, field.Name)
		}

		node, _ = w.Parent(node.Parent)
		// we're always returning on root nodes (operation definition, fragment definitions)
		// this way we won't ever reach beyond node 0
	}
}

func (w *Walker) OperationDefinition(parent int) (document.OperationDefinition, bool) {
	node := Node{
		Parent: parent,
	}
	var ok bool
	for node.Kind != OPERATION_DEFINITION {
		node, ok = w.Parent(node.Parent)
		if !ok {
			return document.OperationDefinition{}, false
		}
	}
	return w.l.p.ParsedDefinitions.OperationDefinitions[node.Ref], true
}

func (w *Walker) SelectionSetTypeName(set document.SelectionSet, parent int) int {

	path := w.c.path[:0]

	for {
		node := w.Node(parent)

		switch node.Kind {
		case INLINE_FRAGMENT:
			def := w.l.InlineFragment(node.Ref)
			if def.TypeCondition != -1 {
				typeName := w.l.Type(def.TypeCondition).Name
				return w.resolveTypeName(typeName, path)
			}
		case FIELD:
			def := w.l.Field(node.Ref)
			path = append(path, def.Name)
		case FRAGMENT_DEFINITION:
			def := w.l.FragmentDefinition(node.Ref)
			typeName := w.l.Type(def.TypeCondition).Name
			return w.resolveTypeName(typeName, path)
		case OPERATION_DEFINITION:
			def := w.l.OperationDefinition(node.Ref)
			operationTypeName := w.l.OperationTypeName(def)
			return w.resolveTypeName(operationTypeName, path)
		}

		parent = node.Parent
	}
}

func (w *Walker) resolveTypeName(typeName int, path []int) int {
	if len(path) == 0 {
		return typeName
	}

	for i := len(path) - 1; i >= 0; i-- {

		fieldName := path[i]
		fieldsDefinition := w.l.FieldsDefinitionFromNamedType(typeName)
		definition, ok := w.l.FieldDefinitionByNameFromIndex(fieldsDefinition, fieldName)
		if !ok {
			return -1
		}
		typeName = w.l.UnwrappedNamedType(w.l.Type(definition.Type)).Name
	}

	return typeName
}
