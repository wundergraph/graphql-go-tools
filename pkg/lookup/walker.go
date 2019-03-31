//go:generate go-enum -f=$GOFILE --noprefix
package lookup

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

/*
ENUM(
UNKNOWN
QUERY
MUTATION
SUBSCRIPTION
FIELD
FRAGMENT_DEFINITION
FRAGMENT_SPREAD
INLINE_FRAGMENT
SCHEMA
SCALAR
SCALAR_TYPE_DEFINITION
OBJECT
OBJECT_TYPE_DEFINITION
FIELD_DEFINITION
ARGUMENT_DEFINITION
INTERFACE
INTERFACE_TYPE_DEFINITION
UNION
UNION_TYPE_DEFINITION
ENUM
ENUM_VALUE
ENUM_TYPE_DEFINITION
INPUT_OBJECT
INPUT_OBJECT_TYPE_DEFINITION
INPUT_FIELD_DEFINITION
INPUT_VALUE_DEFINITION
OPERATION_DEFINITION
DIRECTIVE_SET
DIRECTIVE
DIRECTIVE_DEFINITION
SELECTION_SET
ARGUMENT
ARGUMENT_SET
)
*/
type NodeKind int

type Node struct {
	Parent   int
	Kind     NodeKind
	Ref      int
	Position position.Position
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

	path                []document.ByteSliceReference
	rootNodes           []int
	fragmentDefinitions []int
	fragmentSpreads     []int

	fieldsContainingDirectiveFields     []int
	fieldsContainingDirectiveObjects    []int
	fieldsContainingDirectiveDirectives []int

	directiveDefinitions []int
}

func NewWalker(nodeCacheSize int, defaultCacheSize int) *Walker {
	return &Walker{
		nodes: make([]Node, 0, nodeCacheSize),
		c: walkerCache{
			operationDefinition:                 make([]int, 0, defaultCacheSize),
			argumentSets:                        make([]int, 0, defaultCacheSize),
			arguments:                           make([]int, 0, defaultCacheSize),
			directives:                          make([]int, 0, defaultCacheSize),
			directiveSets:                       make([]int, 0, defaultCacheSize),
			selectionSets:                       make([]int, 0, defaultCacheSize),
			fields:                              make([]int, 0, defaultCacheSize),
			fragmentDefinitions:                 make([]int, 0, defaultCacheSize),
			fragmentSpreads:                     make([]int, 0, defaultCacheSize),
			fieldsContainingDirectiveFields:     make([]int, 0, defaultCacheSize),
			fieldsContainingDirectiveObjects:    make([]int, 0, defaultCacheSize),
			fieldsContainingDirectiveDirectives: make([]int, 0, defaultCacheSize),
			directiveDefinitions:                make([]int, 0, defaultCacheSize),
			path:                                make([]document.ByteSliceReference, 16),
			rootNodes:                           make([]int, 32),
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
	w.c.fragmentDefinitions = w.c.fragmentDefinitions[:0]
	w.c.fragmentSpreads = w.c.fragmentSpreads[:0]
	w.c.fieldsContainingDirectiveFields = w.c.fieldsContainingDirectiveFields[:0]
	w.c.fieldsContainingDirectiveObjects = w.c.fieldsContainingDirectiveObjects[:0]
	w.c.fieldsContainingDirectiveDirectives = w.c.fieldsContainingDirectiveDirectives[:0]
	w.c.directiveDefinitions = w.c.directiveDefinitions[:0]
	w.c.path = w.c.path[:0]
	w.c.rootNodes = w.c.rootNodes[:0]
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

func (w *Walker) WalkTypeSystemDefinition() {
	w.WalkSchemaDefinition(w.l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition)
	w.WalkObjectTypeDefinitions()
	w.WalkEnumTypeDefinitions()
	w.WalkDirectiveDefinitions()
	w.WalkInterfaceTypeDefinitions()
	w.WalkScalarTypeDefinitions()
	w.WalkUnionTypeDefinitions()
	w.WalkInputObjectTypeDefinitions()
}

func (w *Walker) WalkSchemaDefinition(definition document.SchemaDefinition) {
	w.putNode(Node{
		Kind:     SCHEMA,
		Parent:   -1,
		Position: definition.Position,
	})
}

func (w *Walker) WalkObjectTypeDefinitions() {
	for ref := range w.l.p.ParsedDefinitions.ObjectTypeDefinitions {
		nodeRef := w.putNode(Node{
			Ref:      ref,
			Kind:     OBJECT_TYPE_DEFINITION,
			Parent:   -1,
			Position: w.l.p.ParsedDefinitions.ObjectTypeDefinitions[ref].Position,
		})

		if w.l.p.ParsedDefinitions.ObjectTypeDefinitions[ref].FieldsDefinition.HasNext() {
			w.walkFieldDefinitions(w.l.p.ParsedDefinitions.ObjectTypeDefinitions[ref].FieldsDefinition, nodeRef)
		}

		if w.l.p.ParsedDefinitions.ObjectTypeDefinitions[ref].DirectiveSet != -1 {
			w.walkDirectiveSet(w.l.p.ParsedDefinitions.ObjectTypeDefinitions[ref].DirectiveSet, nodeRef)
		}
	}
}

func (w *Walker) walkFieldDefinitions(definitions document.FieldDefinitions, parent int) {

	for definitions.Next(w.l) {
		definition, ref := definitions.Value()
		nodeRef := w.putNode(Node{
			Kind:     FIELD_DEFINITION,
			Parent:   parent,
			Ref:      ref,
			Position: definition.Position,
		})

		if definition.ArgumentsDefinition != -1 {
			args := w.l.ArgumentsDefinition(definition.ArgumentsDefinition)
			w.WalkInputValueDefinitions(args.InputValueDefinitions, nodeRef)
		}

		if definition.DirectiveSet != -1 {
			w.walkDirectiveSet(definition.DirectiveSet, nodeRef)
		}
	}
}

func (w *Walker) WalkInputValueDefinitions(definitions document.InputValueDefinitions, parent int) {
	for definitions.Next(w.l.p) {
		definition, ref := definitions.Value()
		nodeRef := w.putNode(Node{
			Kind:     INPUT_VALUE_DEFINITION,
			Position: definition.Position,
			Ref:      ref,
			Parent:   parent,
		})

		if definition.DirectiveSet != -1 {
			w.walkDirectiveSet(definition.DirectiveSet, nodeRef)
		}
	}
}

func (w *Walker) WalkEnumTypeDefinitions() {
	for ref := range w.l.p.ParsedDefinitions.EnumTypeDefinitions {
		w.putNode(Node{
			Kind:     ENUM_TYPE_DEFINITION,
			Ref:      ref,
			Position: w.l.p.ParsedDefinitions.EnumTypeDefinitions[ref].Position,
			Parent:   -1,
		})
	}
}

func (w *Walker) WalkDirectiveDefinitions() {
	for ref := range w.l.p.ParsedDefinitions.DirectiveDefinitions {
		ref := w.putNode(Node{
			Kind:     DIRECTIVE_DEFINITION,
			Parent:   -1,
			Ref:      ref,
			Position: w.l.p.ParsedDefinitions.DirectiveDefinitions[ref].Position,
		})
		w.c.directiveDefinitions = append(w.c.directiveDefinitions, ref)
	}
}

func (w *Walker) WalkInterfaceTypeDefinitions() {
	for ref := range w.l.p.ParsedDefinitions.InterfaceTypeDefinitions {
		w.putNode(Node{
			Kind:     INTERFACE_TYPE_DEFINITION,
			Ref:      ref,
			Parent:   -1,
			Position: w.l.p.ParsedDefinitions.InterfaceTypeDefinitions[ref].Position,
		})
	}
}

func (w *Walker) WalkScalarTypeDefinitions() {
	for ref := range w.l.p.ParsedDefinitions.ScalarTypeDefinitions {
		w.putNode(Node{
			Kind:     SCALAR_TYPE_DEFINITION,
			Ref:      ref,
			Parent:   -1,
			Position: w.l.p.ParsedDefinitions.ScalarTypeDefinitions[ref].Position,
		})
	}
}

func (w *Walker) WalkUnionTypeDefinitions() {
	for ref := range w.l.p.ParsedDefinitions.UnionTypeDefinitions {
		w.putNode(Node{
			Kind:     UNION_TYPE_DEFINITION,
			Parent:   -1,
			Ref:      ref,
			Position: w.l.p.ParsedDefinitions.UnionTypeDefinitions[ref].Position,
		})
	}
}

func (w *Walker) WalkInputObjectTypeDefinitions() {
	for ref := range w.l.p.ParsedDefinitions.InputObjectTypeDefinitions {
		w.putNode(Node{
			Kind:     INPUT_OBJECT_TYPE_DEFINITION,
			Ref:      ref,
			Parent:   -1,
			Position: w.l.p.ParsedDefinitions.InputObjectTypeDefinitions[ref].Position,
		})
	}
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

	if set == -1 {
		return
	}

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

	if set == -1 {
		return
	}

	arguments := w.l.ArgumentSet(set)

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

	if setRef == -1 {
		return
	}

	set := w.l.SelectionSet(setRef)

	ref := w.putNode(Node{
		Parent: parent,
		Ref:    setRef,
		Kind:   SELECTION_SET,
	})

	w.c.selectionSets = append(w.c.selectionSets, ref)

	if set.IsEmpty() {
		return
	}

	w.walkFields(set.Fields, ref)
	w.walkInlineFragments(set.InlineFragments, ref)
	w.walkFragmentSpreads(set.FragmentSpreads, ref)
}

func (w *Walker) walkFields(i []int, parent int) {

	if len(i) == 0 {
		return
	}

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

		w.c.fragmentSpreads = append(w.c.fragmentSpreads, ref)
		w.walkDirectiveSet(spread.DirectiveSet, ref)
	}
}

func (w *Walker) walkFragmentDefinition(definition document.FragmentDefinition, index int, parent int) {

	ref := w.putNode(Node{
		Parent: parent,
		Kind:   FRAGMENT_DEFINITION,
		Ref:    index,
	})

	w.c.fragmentDefinitions = append(w.c.fragmentDefinitions, ref)

	w.walkDirectiveSet(definition.DirectiveSet, ref)
	w.walkSelectionSet(definition.SelectionSet, ref)
}

func (w *Walker) ParentEquals(parent int, kind NodeKind) (Node, bool) {
	p, ok := w.Parent(parent)
	return p, ok && p.Kind == kind
}

func (w *Walker) ArgumentsDefinition(parent int) document.ArgumentsDefinition {

	var typeName document.ByteSliceReference
	path := w.c.path[:0]
	node, ok := w.ParentEquals(parent, FIELD)
	if ok {
		field := w.l.Field(node.Ref)
		typeName, node = w.WalkUpUntilTypeName(node, &path)

		parentTypeName := w.resolveTypeName(typeName, path[1:])
		parentFields := w.l.FieldsDefinitionFromNamedType(parentTypeName)
		fieldDefinition, ok := w.l.FieldDefinitionByNameFromDefinitions(parentFields, field.Name)
		if !ok {
			return document.ArgumentsDefinition{
				InputValueDefinitions: document.NewInputValueDefinitions(-1),
			}
		}

		if fieldDefinition.ArgumentsDefinition == -1 {
			return document.ArgumentsDefinition{
				InputValueDefinitions: document.NewInputValueDefinitions(-1),
			}
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

	return document.ArgumentsDefinition{
		InputValueDefinitions: document.NewInputValueDefinitions(-1),
	}
}

func (w *Walker) WalkUpUntilTypeName(from Node, fieldPath *[]document.ByteSliceReference) (typeName document.ByteSliceReference, node Node) {

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

type NodeUsageInOperationsIterator struct {
	current int
	refs    []int
	w       *Walker
}

func (n *NodeUsageInOperationsIterator) Next() bool {
	n.current++
	return len(n.refs)-1 >= n.current
}

func (n *NodeUsageInOperationsIterator) Value() int {
	return n.refs[n.current]
}

func (w *Walker) NodeUsageInOperationsIterator(ref int) (iter NodeUsageInOperationsIterator) {

	iter.current = -1
	iter.w = w

	rootNode := w.RootNode(ref)

	iter.refs = w.l.p.IndexPoolGet()

	switch rootNode.Kind {
	case OPERATION_DEFINITION:
		iter.refs = append(iter.refs, rootNode.Ref)
	case FRAGMENT_DEFINITION:
		fragmentDefinition := w.l.FragmentDefinition(rootNode.Ref)
		w.FragmentUsageInOperations(fragmentDefinition.FragmentName, &iter.refs)
	}

	return
}

func (w *Walker) FragmentUsageInOperations(fragmentName document.ByteSliceReference, refs *[]int) {
	for i := range w.c.fragmentSpreads {
		ref := w.c.fragmentSpreads[i]
		node := w.Node(ref)
		spread := w.l.FragmentSpread(node.Ref)
		if !w.l.ByteSliceReferenceContentsEquals(spread.FragmentName, fragmentName) {
			continue
		}

		iter := w.NodeUsageInOperationsIterator(ref)
	Loop:
		for iter.Next() {
			operationDefinitionRef := iter.Value()
			for _, current := range *refs {
				if current == operationDefinitionRef {
					continue Loop
				}
			}
			*refs = append(*refs, operationDefinitionRef)
		}
	}
}

func (w *Walker) RootNode(ref int) (node Node) {
	node = w.Node(ref)
	for node.Parent != -1 {
		node = w.Node(node.Parent)
	}
	return
}

func (w *Walker) SelectionSetTypeName(set document.SelectionSet, parent int) (typeName document.ByteSliceReference) {

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

func (w *Walker) resolveTypeName(typeName document.ByteSliceReference, path []document.ByteSliceReference) document.ByteSliceReference {

	if len(path) == 0 {
		return typeName
	}

	for i := len(path) - 1; i >= 0; i-- {

		fieldName := path[i]
		fieldsDefinition := w.l.FieldsDefinitionFromNamedType(typeName)
		definition, ok := w.l.FieldDefinitionByNameFromDefinitions(fieldsDefinition, fieldName)
		if !ok {
			return document.ByteSliceReference{}
		}
		typeName = w.l.UnwrappedNamedType(w.l.Type(definition.Type)).Name
	}

	return typeName
}

func (w *Walker) FieldPath(parent int) (path []document.ByteSliceReference) {

	if parent == -1 {
		return nil
	}

	path = w.c.path[:0]
	node := Node{
		Parent: parent,
	}

	for {
		node = w.Node(node.Parent)
		switch node.Kind {
		case FIELD:
			field := w.l.Field(node.Ref)
			if field.Alias.Length() != 0 {
				path = append(path, field.Alias)
			} else {
				path = append(path, field.Name)
			}
		}

		if node.Parent == -1 {
			return
		}
	}
}
