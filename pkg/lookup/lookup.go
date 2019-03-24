package lookup

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
)

// Lookup is a helper to easily look things up in a parsed definition
type Lookup struct {
	p        *parser.Parser
	refCache []int
}

func New(p *parser.Parser) *Lookup {
	return &Lookup{
		p:        p,
		refCache: make([]int, 0, 48),
	}
}

func (l *Lookup) SetParser(p *parser.Parser) {
	l.p = p
}

func (l *Lookup) HasOperationDefinitions() bool {
	return len(l.p.ParsedDefinitions.OperationDefinitions) > 0
}

func (l *Lookup) HasFragmentDefinitions() bool {
	return len(l.p.ParsedDefinitions.FragmentDefinitions) > 0
}

func (l *Lookup) OperationDefinition(i int) document.OperationDefinition {
	return l.p.ParsedDefinitions.OperationDefinitions[i]
}

func (l *Lookup) OperationDefinitions() document.OperationDefinitions {
	return l.p.ParsedDefinitions.OperationDefinitions
}

func (l *Lookup) ByteSlice(reference document.ByteSliceReference) document.ByteSlice {
	return l.p.ByteSlice(reference)
}

func (l *Lookup) ByteSliceReference(ref int) document.ByteSliceReference {
	return l.p.ParsedDefinitions.ByteSliceReferences[ref]
}

func (l *Lookup) CachedName(i int) document.ByteSlice {
	return l.p.CachedByteSlice(i)
}

func (l *Lookup) ByteSliceReferenceContentsEquals(left, right document.ByteSliceReference) bool {
	if left.Length() != right.Length() {
		return false
	}
	return bytes.Equal(l.p.ByteSlice(left), l.p.ByteSlice(right))
}

func (l *Lookup) InlineFragment(i int) document.InlineFragment {
	return l.p.ParsedDefinitions.InlineFragments[i]
}

type InlineFragmentIterable struct {
	current int
	refs    []int
	l       *Lookup
}

func (i *InlineFragmentIterable) Next() bool {
	i.current++
	return len(i.refs)-1 >= i.current
}

func (i *InlineFragmentIterable) Value() (inlineFragment document.InlineFragment, ref int) {
	ref = i.refs[i.current]
	inlineFragment = i.l.InlineFragment(ref)
	return
}

func (l *Lookup) InlineFragmentIterable(refs []int) InlineFragmentIterable {
	return InlineFragmentIterable{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) FragmentSpread(i int) document.FragmentSpread {
	return l.p.ParsedDefinitions.FragmentSpreads[i]
}

func (l *Lookup) FragmentSpreads() document.FragmentSpreads {
	return l.p.ParsedDefinitions.FragmentSpreads
}

type FragmentSpreadIterable struct {
	current int
	refs    []int
	l       *Lookup
}

func (f *FragmentSpreadIterable) Next() bool {
	f.current++
	return len(f.refs)-1 >= f.current
}

func (f *FragmentSpreadIterable) Value() (spread document.FragmentSpread, ref int) {
	ref = f.refs[f.current]
	spread = f.l.FragmentSpread(ref)
	return
}

func (l *Lookup) FragmentSpreadIterable(refs []int) FragmentSpreadIterable {
	return FragmentSpreadIterable{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) FragmentDefinitions() document.FragmentDefinitions {
	return l.p.ParsedDefinitions.FragmentDefinitions
}

func (l *Lookup) FragmentDefinition(i int) document.FragmentDefinition {
	return l.p.ParsedDefinitions.FragmentDefinitions[i]
}

func (l *Lookup) FragmentDefinitionByName(name document.ByteSliceReference) (definition document.FragmentDefinition, index int, valid bool) {

	for i, definition := range l.p.ParsedDefinitions.FragmentDefinitions {
		if l.ByteSliceReferenceContentsEquals(definition.FragmentName, name) {
			return definition, i, true
		}
	}

	return document.FragmentDefinition{}, -1, false
}

func (l Lookup) Field(i int) document.Field {
	return l.p.ParsedDefinitions.Fields[i]
}

func (l *Lookup) FragmentSpreadsRootNumFields(spreadRefs []int) (amount int) {
	iter := l.FragmentSpreadIterable(spreadRefs)
	for iter.Next() {
		spread, _ := iter.Value()
		definition, _, _ := l.FragmentDefinitionByName(spread.FragmentName)
		set := l.SelectionSet(definition.SelectionSet)
		amount += len(set.Fields)
		amount += len(set.FragmentSpreads)
		amount += len(set.InlineFragments)
	}
	return
}

func (l *Lookup) InlineFragmentRootNumFields(inlineFragmentRefs []int) (amount int) {
	iter := l.InlineFragmentIterable(inlineFragmentRefs)
	for iter.Next() {
		inlineFragment, _ := iter.Value()
		set := l.SelectionSet(inlineFragment.SelectionSet)
		amount += len(set.Fields)
		amount += len(set.FragmentSpreads)
		amount += len(set.InlineFragments)
	}
	return
}

func (l *Lookup) SelectionSet(ref int) document.SelectionSet {
	if ref == -1 {
		return document.SelectionSet{}
	}
	return l.p.ParsedDefinitions.SelectionSets[ref]
}

func (l *Lookup) SelectionSetNumRootFields(set document.SelectionSet) int {
	return len(set.Fields) + l.FragmentSpreadsRootNumFields(set.FragmentSpreads) + l.InlineFragmentRootNumFields(set.InlineFragments)
}

func (l *Lookup) UnwrappedNamedType(docType document.Type) document.Type {

	for docType.Kind != document.TypeKindNAMED {
		docType = l.Type(docType.OfType)
	}

	return docType
}

func (l *Lookup) Type(i int) document.Type {
	return l.p.ParsedDefinitions.Types[i]
}

type ObjectTypeDefinitionIterable struct {
	current int
	refs    []int
	l       *Lookup
}

func (o *ObjectTypeDefinitionIterable) Next() bool {
	o.current++
	return len(o.refs)-1 >= o.current
}

func (o *ObjectTypeDefinitionIterable) Value() (ref int, definition document.ObjectTypeDefinition) {
	ref = o.refs[o.current]
	definition = o.l.ObjectTypeDefinition(ref)
	return
}

func (l *Lookup) ObjectTypeDefinitionIterable(refs []int) ObjectTypeDefinitionIterable {
	return ObjectTypeDefinitionIterable{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) ObjectTypeDefinition(ref int) document.ObjectTypeDefinition {
	return l.p.ParsedDefinitions.ObjectTypeDefinitions[ref]
}

func (l *Lookup) ObjectTypeDefinitionByName(name document.ByteSliceReference) (definition document.ObjectTypeDefinition, exists bool) {
	for i := range l.p.ParsedDefinitions.ObjectTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(name, l.p.ParsedDefinitions.ObjectTypeDefinitions[i].Name) {
			return l.p.ParsedDefinitions.ObjectTypeDefinitions[i], true
		}
	}

	return document.ObjectTypeDefinition{}, false
}

func (l *Lookup) ScalarTypeDefinitionByName(name document.ByteSliceReference) (document.ScalarTypeDefinition, bool) {
	for _, definition := range l.p.ParsedDefinitions.ScalarTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(name, definition.Name) {
			return definition, true
		}
	}

	return document.ScalarTypeDefinition{}, false
}

type EnumTypeDefinitionIterable struct {
	l       *Lookup
	refs    []int
	current int
}

func (e *EnumTypeDefinitionIterable) Next() bool {
	e.current++
	return len(e.refs)-1 >= e.current
}

func (e *EnumTypeDefinitionIterable) Value() (ref int, definition document.EnumTypeDefinition) {
	ref = e.refs[e.current]
	definition = e.l.p.ParsedDefinitions.EnumTypeDefinitions[ref]
	return
}

func (l *Lookup) EnumTypeDefinitionIterable(refs []int) EnumTypeDefinitionIterable {
	return EnumTypeDefinitionIterable{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) EnumTypeDefinitionByName(name document.ByteSliceReference) (document.EnumTypeDefinition, bool) {
	for _, definition := range l.p.ParsedDefinitions.EnumTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(name, definition.Name) {
			return definition, true
		}
	}

	return document.EnumTypeDefinition{}, false
}

func (l *Lookup) InterfaceTypeDefinitionByName(name document.ByteSliceReference) (document.InterfaceTypeDefinition, bool) {
	for _, definition := range l.p.ParsedDefinitions.InterfaceTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(name, definition.Name) {
			return definition, true
		}
	}
	return document.InterfaceTypeDefinition{}, false
}

func (l *Lookup) UnionTypeDefinitionByName(name document.ByteSliceReference) (document.UnionTypeDefinition, bool) {
	for _, definition := range l.p.ParsedDefinitions.UnionTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(name, definition.Name) {
			return definition, true
		}
	}

	return document.UnionTypeDefinition{}, false
}

func (l *Lookup) FieldDefinition(ref int) document.FieldDefinition {
	return l.p.ParsedDefinitions.FieldDefinitions[ref]
}

func (l *Lookup) FieldDefinitionByNameFromIndex(index []int, name document.ByteSliceReference) (document.FieldDefinition, bool) {

	for _, i := range index {
		definition := l.p.ParsedDefinitions.FieldDefinitions[i]
		if l.ByteSliceReferenceContentsEquals(name, definition.Name) {
			return definition, true
		}
	}

	return document.FieldDefinition{}, false
}

type FieldsIterator struct {
	l       *Lookup
	current int
	index   []int
}

func (f *FieldsIterator) Next() bool {
	f.current++
	return len(f.index)-1 >= f.current
}

func (f *FieldsIterator) Value() (field document.Field, ref int) {
	ref = f.index[f.current]
	field = f.l.p.ParsedDefinitions.Fields[ref]
	return
}

func (l *Lookup) FieldsIterator(i []int) FieldsIterator {
	return FieldsIterator{
		current: -1,
		index:   i,
		l:       l,
	}
}

type SelectionSetContentsIterator struct {
	set  document.SelectionSet
	kind NodeKind
	ref  int
	l    *Lookup
}

func (s *SelectionSetContentsIterator) Next() bool {

	var field document.Field
	var inline document.InlineFragment
	var spread document.FragmentSpread

	hasField := len(s.set.Fields) > 0
	hasInline := len(s.set.InlineFragments) > 0
	hasSpread := len(s.set.FragmentSpreads) > 0

	if hasField {
		field = s.l.Field(s.set.Fields[0])
	}
	if hasInline {
		inline = s.l.InlineFragment(s.set.InlineFragments[0])
	}
	if hasSpread {
		spread = s.l.FragmentSpread(s.set.FragmentSpreads[0])
	}

	if hasField {
		if !hasSpread || hasSpread && field.Position.IsBefore(spread.Position) {
			if !hasInline || hasInline && field.Position.IsBefore(inline.Position) {
				s.kind = FIELD
				s.ref = s.set.Fields[0]
				s.set.Fields = s.set.Fields[1:]
				return true
			}
		}
	}

	if hasInline {
		if !hasSpread || hasSpread && inline.Position.IsBefore(spread.Position) {
			s.kind = INLINE_FRAGMENT
			s.ref = s.set.InlineFragments[0]
			s.set.InlineFragments = s.set.InlineFragments[1:]
			return true
		}
	}

	if hasSpread {
		s.kind = FRAGMENT_SPREAD
		s.ref = s.set.FragmentSpreads[0]
		s.set.FragmentSpreads = s.set.FragmentSpreads[1:]
		return true
	}

	return false
}

func (s *SelectionSetContentsIterator) Value() (kind NodeKind, ref int) {
	return s.kind, s.ref
}

func (l *Lookup) SelectionSetContentsIterator(ref int) SelectionSetContentsIterator {
	return SelectionSetContentsIterator{
		l:   l,
		set: l.SelectionSet(ref),
	}
}

type SelectionSetFieldsIterator struct {
	l           *Lookup
	setTypeName document.ByteSliceReference
	refs        []int
	current     int
}

func (s *SelectionSetFieldsIterator) traverse(set document.SelectionSet, isRootLevel bool) {
	fields := s.l.FieldsIterator(set.Fields)
	for fields.Next() {
		_, ref := fields.Value()
		s.refs = append(s.refs, ref)
	}
	inlineFragments := s.l.InlineFragmentIterable(set.InlineFragments)
	for inlineFragments.Next() {
		fragment, _ := inlineFragments.Value()
		if fragment.TypeCondition == -1 || s.l.ByteSliceReferenceContentsEquals(s.l.Type(fragment.TypeCondition).Name, s.setTypeName) {
			s.traverse(s.l.SelectionSet(fragment.SelectionSet), false)
		}
	}
	spreads := s.l.FragmentSpreadIterable(set.FragmentSpreads)
	for spreads.Next() {
		spread, _ := spreads.Value()
		fragment, _, ok := s.l.FragmentDefinitionByName(spread.FragmentName)
		if !ok {
			continue
		}
		if s.l.ByteSliceReferenceContentsEquals(s.l.Type(fragment.TypeCondition).Name, s.setTypeName) {
			s.traverse(s.l.SelectionSet(fragment.SelectionSet), false)
		}
	}
}

func (s *SelectionSetFieldsIterator) Next() bool {
	s.current++
	return len(s.refs)-1 >= s.current
}

func (s *SelectionSetFieldsIterator) Value() (ref int, field document.Field) {
	ref = s.refs[s.current]
	field = s.l.Field(ref)
	return
}

func (l *Lookup) SelectionSetCollectedFields(set document.SelectionSet, setTypeName document.ByteSliceReference) SelectionSetFieldsIterator {
	iter := SelectionSetFieldsIterator{
		current:     -1,
		setTypeName: setTypeName,
		l:           l,
		refs:        l.p.IndexPoolGet(),
	}

	iter.traverse(set, true)
	return iter
}

type TypedSet struct {
	SetRef int
	Type   document.Type
}

type SelectionSetDifferingSelectionSetIterator struct {
	l              *Lookup
	ignoreTypeName document.ByteSliceReference
	setRefs        []int
	typeRefs       []int
	current        int
}

func (s *SelectionSetDifferingSelectionSetIterator) traverse(set document.SelectionSet, isRootLevel bool) {
	inlineFragments := s.l.InlineFragmentIterable(set.InlineFragments)
	for inlineFragments.Next() {
		fragment, _ := inlineFragments.Value()
		if fragment.TypeCondition == -1 {
			s.traverse(s.l.SelectionSet(fragment.SelectionSet), false)
		} else if !s.l.ByteSliceReferenceContentsEquals(s.l.Type(fragment.TypeCondition).Name, s.ignoreTypeName) {
			s.setRefs = append(s.setRefs, fragment.SelectionSet)
			s.typeRefs = append(s.typeRefs, fragment.TypeCondition)
		}
	}
	spreads := s.l.FragmentSpreadIterable(set.FragmentSpreads)
	for spreads.Next() {
		spread, _ := spreads.Value()
		fragment, _, _ := s.l.FragmentDefinitionByName(spread.FragmentName)
		if !s.l.ByteSliceReferenceContentsEquals(s.l.Type(fragment.TypeCondition).Name, s.ignoreTypeName) {
			s.setRefs = append(s.setRefs, fragment.SelectionSet)
			s.typeRefs = append(s.typeRefs, fragment.TypeCondition)
		}

		s.traverse(s.l.SelectionSet(fragment.SelectionSet), false)
	}
}

func (s *SelectionSetDifferingSelectionSetIterator) Next() bool {
	s.current++
	return len(s.setRefs)-1 >= s.current
}

func (s *SelectionSetDifferingSelectionSetIterator) Value() TypedSet {
	return TypedSet{
		SetRef: s.setRefs[s.current],
		Type:   s.l.Type(s.typeRefs[s.current]),
	}
}

func (l *Lookup) SelectionSetDifferingSelectionSetIterator(set document.SelectionSet, ignoreTypeName document.ByteSliceReference) SelectionSetDifferingSelectionSetIterator {
	iter := SelectionSetDifferingSelectionSetIterator{
		current:        -1,
		ignoreTypeName: ignoreTypeName,
		l:              l,
		setRefs:        l.p.IndexPoolGet(),
		typeRefs:       l.p.IndexPoolGet(),
	}
	iter.traverse(set, true)
	return iter
}

func (l *Lookup) QueryObjectTypeDefinition() (document.ObjectTypeDefinition, bool) {
	want := l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Query
	for _, definition := range l.p.ParsedDefinitions.ObjectTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(definition.Name, want) {
			return definition, true
		}
	}

	return document.ObjectTypeDefinition{}, false
}

func (l *Lookup) MutationObjectTypeDefinition() (document.ObjectTypeDefinition, bool) {
	want := l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Mutation
	for _, definition := range l.p.ParsedDefinitions.ObjectTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(definition.Name, want) {
			return definition, true
		}
	}

	return document.ObjectTypeDefinition{}, false
}

func (l *Lookup) SubscriptionObjectTypeDefinition() (document.ObjectTypeDefinition, bool) {
	want := l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Subscription
	for _, definition := range l.p.ParsedDefinitions.ObjectTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(definition.Name, want) {
			return definition, true
		}
	}

	return document.ObjectTypeDefinition{}, false
}

func (l *Lookup) UnionTypeDefinitionContainsType(definition document.UnionTypeDefinition, typeName document.ByteSliceReference) bool {
	for _, member := range definition.UnionMemberTypes {
		if l.ByteSliceReferenceContentsEquals(l.ByteSliceReference(member), typeName) {
			return true
		}
	}

	return false
}

func (l *Lookup) ObjectTypeDefinitionImplementsInterface(definition document.ObjectTypeDefinition, interfaceName document.ByteSliceReference) bool {
	for _, implements := range definition.ImplementsInterfaces {
		if l.ByteSliceReferenceContentsEquals(l.ByteSliceReference(implements), interfaceName) {
			return true
		}
	}

	return false
}

func (l *Lookup) ArgumentSet(ref int) document.ArgumentSet {
	if ref == -1 {
		return nil
	}
	return l.p.ParsedDefinitions.ArgumentSets[ref]
}

func (l *Lookup) ArgumentsAreEqual(first []int, second []int) bool {

	if len(first) != len(second) {
		return false
	}

	for _, i := range first {
		firstArg := l.Argument(i)
		secondArg, ok := l.ArgumentByIndexAndName(second, firstArg.Name)
		if !ok {
			return false
		}
		if !l.ValuesAreEqual(firstArg.Value, secondArg.Value) {
			return false
		}
	}

	return true
}

func (l *Lookup) Argument(i int) document.Argument {
	return l.p.ParsedDefinitions.Arguments[i]
}

type ArgumentsIterable struct {
	current int
	refs    []int
	l       *Lookup
}

func (a *ArgumentsIterable) Next() bool {
	a.current++
	return len(a.refs)-1 >= a.current
}

func (a *ArgumentsIterable) Value() (argument document.Argument, ref int) {
	ref = a.refs[a.current]
	argument = a.l.Argument(ref)
	return
}

func (l *Lookup) ArgumentsIterable(refs []int) ArgumentsIterable {
	return ArgumentsIterable{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) ArgumentByIndexAndName(index []int, name document.ByteSliceReference) (document.Argument, bool) {
	for _, i := range index {
		arg := l.Argument(i)
		if l.ByteSliceReferenceContentsEquals(arg.Name, name) {
			return arg, true
		}
	}
	return document.Argument{}, false
}

func (l *Lookup) ArgumentsDefinition(i int) document.ArgumentsDefinition {
	if i == -1 {
		return document.ArgumentsDefinition{}
	}
	return l.p.ParsedDefinitions.ArgumentsDefinitions[i]
}

func (l *Lookup) InputValueDefinitionByNameAndIndex(name document.ByteSliceReference, i []int) (document.InputValueDefinition, bool) {
	for _, j := range i {
		definition := l.p.ParsedDefinitions.InputValueDefinitions[j]
		if l.ByteSliceReferenceContentsEquals(name, definition.Name) {
			return definition, true
		}
	}
	return document.InputValueDefinition{}, false
}

type InputValueDefinitionIterator struct {
	current int
	refs    []int
	l       *Lookup
}

func (i *InputValueDefinitionIterator) Next() bool {
	i.current++
	return len(i.refs)-1 >= i.current
}

func (i *InputValueDefinitionIterator) Value() (definition document.InputValueDefinition, ref int) {
	ref = i.refs[i.current]
	definition = i.l.p.ParsedDefinitions.InputValueDefinitions[ref]
	return
}

func (l *Lookup) InputValueDefinitionIterator(refs []int) InputValueDefinitionIterator {
	return InputValueDefinitionIterator{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) ValuesAreEqual(first, second int) bool {
	a := l.Value(first)
	b := l.Value(second)

	if a.ValueType != b.ValueType {
		return false
	}

	switch a.ValueType {
	case document.ValueTypeVariable:
		return bytes.Equal(l.ByteSlice(l.p.ParsedDefinitions.ByteSliceReferences[a.Reference]),
			l.ByteSlice(l.p.ParsedDefinitions.ByteSliceReferences[b.Reference]))
	case document.ValueTypeInt:
		return l.p.ParsedDefinitions.Integers[a.Reference] == l.p.ParsedDefinitions.Integers[b.Reference]
	case document.ValueTypeFloat:
		return l.p.ParsedDefinitions.Floats[a.Reference] == l.p.ParsedDefinitions.Floats[b.Reference]
	case document.ValueTypeString:
		return bytes.Equal(l.ByteSlice(l.p.ParsedDefinitions.ByteSliceReferences[a.Reference]),
			l.ByteSlice(l.p.ParsedDefinitions.ByteSliceReferences[b.Reference]))
	case document.ValueTypeBoolean:
		return l.p.ParsedDefinitions.Booleans[a.Reference] == l.p.ParsedDefinitions.Booleans[b.Reference]
	case document.ValueTypeNull:
		return true
	case document.ValueTypeEnum:
		return bytes.Equal(l.ByteSlice(l.p.ParsedDefinitions.ByteSliceReferences[a.Reference]),
			l.ByteSlice(l.p.ParsedDefinitions.ByteSliceReferences[b.Reference]))
	case document.ValueTypeList:
		return l.ValueListsAreEqual(a.Reference, b.Reference)
	case document.ValueTypeObject:
		return l.ValueObjectsAreEqual(a.Reference, b.Reference)
	default:
		return false
	}
}

func (l *Lookup) Value(i int) document.Value {
	return l.p.ParsedDefinitions.Values[i]
}

func (l *Lookup) ValueListsAreEqual(first, second int) bool {
	a := l.p.ParsedDefinitions.ListValues[first]
	b := l.p.ParsedDefinitions.ListValues[second]

	if len(a) != len(b) {
		return false
	}

Outer:
	for _, left := range a {
		for _, right := range b {
			if l.ValuesAreEqual(left, right) {
				continue Outer
			}
		}
		return false
	}
	return true
}

func (l *Lookup) ValueObjectsAreEqual(first, second int) bool {
	a := l.p.ParsedDefinitions.ObjectValues[first]
	b := l.p.ParsedDefinitions.ObjectValues[second]

	if len(a) != len(b) {
		return false
	}

	for _, i := range a {
		left := l.ObjectField(i)
		right, ok := l.ObjectFieldByIndexAndName(b, left.Name)
		if !ok {
			return false
		}

		if !l.ValuesAreEqual(left.Value, right.Value) {
			return false
		}
	}
	return true
}

func (l *Lookup) ObjectValue(ref int) document.ObjectValue {
	return l.p.ParsedDefinitions.ObjectValues[ref]
}

func (l *Lookup) ObjectField(i int) document.ObjectField {
	return l.p.ParsedDefinitions.ObjectFields[i]
}

type ObjectFieldsIterator struct {
	current int
	refs    []int
	l       *Lookup
}

func (o *ObjectFieldsIterator) Next() bool {
	o.current++
	return len(o.refs)-1 >= o.current
}

func (o *ObjectFieldsIterator) Value() (field document.ObjectField, ref int) {
	ref = o.refs[o.current]
	field = o.l.ObjectField(ref)
	return
}

func (l *Lookup) ObjectFieldsIterator(refs []int) ObjectFieldsIterator {
	return ObjectFieldsIterator{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) ObjectFieldByIndexAndName(index []int, name document.ByteSliceReference) (document.ObjectField, bool) {
	for _, i := range index {
		field := l.ObjectField(i)
		if l.ByteSliceReferenceContentsEquals(name, field.Name) {
			return field, true
		}
	}
	return document.ObjectField{}, false
}

func (l *Lookup) FieldsDefinitionFromNamedType(name document.ByteSliceReference) (fields []int) {

	_, ok := l.UnionTypeDefinitionByName(name)
	if ok {
		return
	}

	objectType, ok := l.ObjectTypeDefinitionByName(name)
	if ok {
		fields = objectType.FieldsDefinition
		return
	}

	interfaceType, ok := l.InterfaceTypeDefinitionByName(name)
	if ok {
		fields = interfaceType.FieldsDefinition
		return
	}

	return
}

func (l *Lookup) TypesAreEqual(left, right document.Type) bool {

	if left.Kind != right.Kind {
		return false
	}

	if left.Kind == document.TypeKindNAMED && !l.ByteSliceReferenceContentsEquals(left.Name, right.Name) {
		return false
	}

	if left.OfType == -1 && right.OfType == -1 {
		return true
	}

	return l.TypesAreEqual(l.Type(left.OfType), l.Type(right.OfType))
}

func (l *Lookup) IsLeafNode(typeName document.ByteSliceReference) bool {
	_, ok := l.ScalarTypeDefinitionByName(typeName)
	if ok {
		return true
	}
	_, ok = l.EnumTypeDefinitionByName(typeName)
	return ok
}

func (l *Lookup) Directive(i int) document.Directive {
	return l.p.ParsedDefinitions.Directives[i]
}

type DirectiveIterable struct {
	current int
	refs    []int
	l       *Lookup
}

func (d *DirectiveIterable) Next() bool {
	d.current++
	return len(d.refs)-1 >= d.current
}

func (d *DirectiveIterable) Value() (directive document.Directive, ref int) {
	ref = d.refs[d.current]
	directive = d.l.Directive(ref)
	return
}

func (l *Lookup) DirectiveIterable(refs []int) DirectiveIterable {
	return DirectiveIterable{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) DirectiveSet(ref int) document.DirectiveSet {
	if ref == -1 {
		return nil
	}
	return l.p.ParsedDefinitions.DirectiveSets[ref]
}

func (l *Lookup) DirectiveDefinition(ref int) document.DirectiveDefinition {
	return l.p.ParsedDefinitions.DirectiveDefinitions[ref]
}

func (l *Lookup) DirectiveDefinitionByName(name document.ByteSliceReference) (document.DirectiveDefinition, bool) {
	for _, definition := range l.p.ParsedDefinitions.DirectiveDefinitions {
		if l.ByteSliceReferenceContentsEquals(name, definition.Name) {
			return definition, true
		}
	}
	return document.DirectiveDefinition{}, false
}

func (l *Lookup) IsUniqueFragmentName(fragmentIndex int, name document.ByteSliceReference) bool {
	for j, k := range l.p.ParsedDefinitions.FragmentDefinitions {
		if fragmentIndex == j {
			continue
		}
		if l.ByteSliceReferenceContentsEquals(name, k.FragmentName) {
			return false
		}
	}
	return true
}

func (l *Lookup) TypeIsValidFragmentTypeCondition(name document.ByteSliceReference) bool {
	_, ok := l.UnionTypeDefinitionByName(name)
	if ok {
		return true
	}
	_, ok = l.InterfaceTypeDefinitionByName(name)
	if ok {
		return true
	}
	_, ok = l.ObjectTypeDefinitionByName(name)
	return ok
}

func (l *Lookup) TypeIsScalarOrEnum(name document.ByteSliceReference) bool {
	_, ok := l.ScalarTypeDefinitionByName(name)
	if ok {
		return true
	}
	_, ok = l.EnumTypeDefinitionByName(name)
	return ok
}

func (l *Lookup) IsFragmentDefinitionUsedInOperation(name document.ByteSliceReference) bool {

	for _, definitions := range l.p.ParsedDefinitions.OperationDefinitions {
		set := l.SelectionSet(definitions.SelectionSet)
		for _, i := range set.FragmentSpreads {
			spread := l.p.ParsedDefinitions.FragmentSpreads[i]
			if l.ByteSliceReferenceContentsEquals(spread.FragmentName, name) {
				return true
			}
		}
	}

	for _, definitions := range l.p.ParsedDefinitions.Fields {
		set := l.SelectionSet(definitions.SelectionSet)
		for _, i := range set.FragmentSpreads {
			spread := l.p.ParsedDefinitions.FragmentSpreads[i]
			if l.ByteSliceReferenceContentsEquals(spread.FragmentName, name) {
				return true
			}
		}
	}

	for _, definitions := range l.p.ParsedDefinitions.FragmentDefinitions {
		set := l.SelectionSet(definitions.SelectionSet)
		for _, i := range set.FragmentSpreads {
			spread := l.p.ParsedDefinitions.FragmentSpreads[i]
			if l.ByteSliceReferenceContentsEquals(spread.FragmentName, name) {
				return true
			}
		}
	}

	for _, definitions := range l.p.ParsedDefinitions.InlineFragments {
		set := l.SelectionSet(definitions.SelectionSet)
		for _, i := range set.FragmentSpreads {
			spread := l.p.ParsedDefinitions.FragmentSpreads[i]
			if l.ByteSliceReferenceContentsEquals(spread.FragmentName, name) {
				return true
			}
		}
	}

	return false
}

func (l *Lookup) SelectionSetContainsFragmentSpread(set document.SelectionSet, spreadName document.ByteSliceReference) bool {

	for _, i := range set.FragmentSpreads {
		if l.ByteSliceReferenceContentsEquals(l.FragmentSpread(i).FragmentName, spreadName) {
			return true
		}
	}

	fragmentSpreadIter := l.FragmentSpreadIterable(set.FragmentSpreads)
	for fragmentSpreadIter.Next() {
		spread, _ := fragmentSpreadIter.Value()
		definition, _, ok := l.FragmentDefinitionByName(spread.FragmentName)
		if ok && l.SelectionSetContainsFragmentSpread(l.SelectionSet(definition.SelectionSet), spreadName) {
			return true
		}
	}
	fieldsIter := l.FieldsIterator(set.Fields)
	for fieldsIter.Next() {
		field, _ := fieldsIter.Value()
		if l.SelectionSetContainsFragmentSpread(l.SelectionSet(field.SelectionSet), spreadName) {
			return true
		}
	}
	inlineFragmentIter := l.InlineFragmentIterable(set.InlineFragments)
	for inlineFragmentIter.Next() {
		inlineFragment, _ := inlineFragmentIter.Value()
		if l.SelectionSetContainsFragmentSpread(l.SelectionSet(inlineFragment.SelectionSet), spreadName) {
			return true
		}
	}

	return false
}

func (l *Lookup) PossibleSelectionTypes(typeName document.ByteSliceReference, possibleTypeNames *[]document.ByteSliceReference) {

	*possibleTypeNames = append(*possibleTypeNames, typeName)

	objectType, ok := l.ObjectTypeDefinitionByName(typeName)
	if ok {
		for _, implementsInterface := range objectType.ImplementsInterfaces {
			*possibleTypeNames = append(*possibleTypeNames, l.ByteSliceReference(implementsInterface))
		}
		l.UnionTypeDefinitionNamesContainingMember(typeName, possibleTypeNames)
		return
	}
	_, ok = l.InterfaceTypeDefinitionByName(typeName)
	if ok {
		for _, objectTypeDefinition := range l.p.ParsedDefinitions.ObjectTypeDefinitions {
			if l.ObjectTypeDefinitionImplementsInterface(objectTypeDefinition, typeName) {
				l.PossibleSelectionTypes(objectTypeDefinition.Name, possibleTypeNames)
			}
		}
		return
	}

	unionTypeDefinition, ok := l.UnionTypeDefinitionByName(typeName)
	if ok {
		for _, member := range unionTypeDefinition.UnionMemberTypes {
			memberNameRef := l.ByteSliceReference(member)
			*possibleTypeNames = append(*possibleTypeNames, memberNameRef)
			l.PossibleSelectionTypes(memberNameRef, possibleTypeNames)
		}
	}
}

func (l *Lookup) FieldType(typeName document.ByteSliceReference, fieldName document.ByteSliceReference) (document.Type, bool) {
	objectTypeDefinition, ok := l.ObjectTypeDefinitionByName(typeName)
	if ok {
		definition, ok := l.FieldDefinitionByNameFromIndex(objectTypeDefinition.FieldsDefinition, fieldName)
		if !ok {
			return document.Type{}, false
		}
		return l.Type(definition.Type), true
	}
	interfaceTypeDefinition, ok := l.InterfaceTypeDefinitionByName(typeName)
	if ok {
		definition, ok := l.FieldDefinitionByNameFromIndex(interfaceTypeDefinition.FieldsDefinition, fieldName)
		if !ok {
			return document.Type{}, false
		}
		return l.Type(definition.Type), true
	}
	return document.Type{}, false
}

func (l *Lookup) FieldSelectionsArePossible(onTypeName document.ByteSliceReference, selections document.SelectionSet) bool {

	fieldDefinitions := l.FieldsDefinitionFromNamedType(onTypeName)

	fields := l.FieldsIterator(selections.Fields)
	for fields.Next() {
		field, _ := fields.Value()

		if bytes.Equal(l.ByteSlice(field.Name), []byte("__typename")) {
			continue
		}

		fieldDefinition, ok := l.FieldDefinitionByNameFromIndex(fieldDefinitions, field.Name)
		if !ok {
			return false
		}

		fieldDefinitionType := l.UnwrappedNamedType(l.Type(fieldDefinition.Type))
		if !l.FieldSelectionsArePossible(fieldDefinitionType.Name, l.SelectionSet(field.SelectionSet)) {
			//return err
			return false
		}

		hasSelections := l.SelectionSetHasSelections(l.SelectionSet(field.SelectionSet))
		isLeafNode := l.IsLeafNode(fieldDefinitionType.Name)
		if !isLeafNode && !hasSelections {
			return false
		}
	}

	inlineFragments := l.InlineFragmentIterable(selections.InlineFragments)
	for inlineFragments.Next() {
		inlineFragment, _ := inlineFragments.Value()
		if inlineFragment.TypeCondition == -1 {
			if !l.FieldSelectionsArePossible(onTypeName, l.SelectionSet(inlineFragment.SelectionSet)) {
				return false
			}
		} else {
			typeCondition := l.Type(inlineFragment.TypeCondition)
			if !l.FieldSelectionsArePossible(typeCondition.Name, l.SelectionSet(inlineFragment.SelectionSet)) {
				return false
			}
		}
	}

	return true
}

func (l *Lookup) ValueIsValid(value document.Value, typeSystemType document.Type, variableDefinitionRefs []int, inputValueDefinitionHasDefaultValue bool) bool {

	if value.ValueType == document.ValueTypeVariable {
		variableValue, ok := l.VariableDefinition(l.p.ParsedDefinitions.ByteSliceReferences[value.Reference], variableDefinitionRefs)
		if !ok {
			return false
		}

		variableType := l.Type(variableValue.Type)

		if typeSystemType.Kind == document.TypeKindNON_NULL && (variableType.Kind == document.TypeKindNON_NULL || variableValue.DefaultValue != -1 || inputValueDefinitionHasDefaultValue) {
			typeSystemType = l.Type(typeSystemType.OfType)
		}

		if !l.TypeSatisfiesTypeSystemType(typeSystemType, variableType) {
			return false
		}

		if variableValue.DefaultValue != -1 {
			defaultValue := l.Value(variableValue.DefaultValue)
			return l.ValueIsValid(defaultValue, typeSystemType, variableDefinitionRefs, inputValueDefinitionHasDefaultValue)
		}

		return true
	}

	nonNull := typeSystemType.Kind == document.TypeKindNON_NULL
	if nonNull {
		typeSystemType = l.Type(typeSystemType.OfType)
	}

	typeNameBytes := l.ByteSlice(typeSystemType.Name)

	switch value.ValueType {
	case document.ValueTypeInt:
		return bytes.Equal(typeNameBytes, []byte("Float")) || bytes.Equal(typeNameBytes, []byte("Int"))
	case document.ValueTypeFloat:
		return bytes.Equal([]byte("Float"), typeNameBytes)
	case document.ValueTypeString:
		return bytes.Equal([]byte("String"), typeNameBytes)
	case document.ValueTypeBoolean:
		return bytes.Equal([]byte("Boolean"), typeNameBytes)
	case document.ValueTypeNull:
		return !nonNull
	case document.ValueTypeEnum:
		enumTypeDefinition, ok := l.EnumTypeDefinitionByName(typeSystemType.Name)
		if !ok {
			return false
		}
		if !l.EnumTypeDefinitionContainsValue(enumTypeDefinition, l.p.ParsedDefinitions.ByteSliceReferences[value.Reference]) {
			return false
		}
	case document.ValueTypeList:

		if typeSystemType.Kind != document.TypeKindLIST {
			return false
		}

		listType := l.Type(typeSystemType.OfType)

		listValue := l.ListValue(value.Reference)
		if !l.listValueIsValid(listValue, listType, variableDefinitionRefs) {
			return false
		}
	case document.ValueTypeObject:
		inputObjectTypeDefinition, ok := l.InputObjectTypeDefinitionByName(typeSystemType.Name)
		if !ok {
			return false
		}
		objectValue := l.p.ParsedDefinitions.ObjectValues[value.Reference]
		if !l.objectValueIsValid(objectValue, inputObjectTypeDefinition, variableDefinitionRefs) {
			return false
		}
	}

	return true
}

func (l *Lookup) ListValue(ref int) document.ListValue {
	return l.p.ParsedDefinitions.ListValues[ref]
}

func (l *Lookup) listValueIsValid(value document.ListValue, typeSystemType document.Type, variableDefinitionRefs []int) bool {
	for _, i := range value {
		listMember := l.Value(i)
		if !l.ValueIsValid(listMember, typeSystemType, variableDefinitionRefs, false) {
			return false
		}
	}
	return true
}

func (l *Lookup) objectValueIsValid(value document.ObjectValue, definition document.InputObjectTypeDefinition, variableDefinitionRefs []int) bool {

	inputFieldsDefinition := l.p.ParsedDefinitions.InputFieldsDefinitions[definition.InputFieldsDefinition]

	inputValueDefinitions := l.InputValueDefinitionIterator(inputFieldsDefinition.InputValueDefinitions)
	for inputValueDefinitions.Next() {
		inputValueDefinition, _ := inputValueDefinitions.Value()
		inputType := l.Type(inputValueDefinition.Type)

		objectField, ok := l.ObjectFieldByIndexAndName(value, inputValueDefinition.Name)
		if inputType.Kind == document.TypeKindNON_NULL && !ok {
			//return fmt.Errorf("validateObjectValue: required objectField '%s' missing", string(l.CachedName(inputValueDefinition.Name)))
			return false
		} else if !ok {
			continue
		}

		objectFieldValue := l.Value(objectField.Value)

		if !l.ValueIsValid(objectFieldValue, inputType, variableDefinitionRefs, l.InputValueDefinitionHasDefaultValue(inputValueDefinition)) {
			//return errors.Wrap(err, "validateObjectValue->")
			return false
		}
	}

	leftFields := l.ObjectFieldsIterator(value)
	for leftFields.Next() {
		left, i := leftFields.Value()
		_, ok := l.InputValueDefinitionByNameAndIndex(left.Name, inputFieldsDefinition.InputValueDefinitions)
		if !ok {
			//return fmt.Errorf("validateObjectValue: input field '%s' not defined", string(l.CachedName(left.Name)))
			return false
		}
		rightFields := l.ObjectFieldsIterator(value)
		for rightFields.Next() {
			right, j := rightFields.Value()
			if i == j {
				continue
			}
			if l.ByteSliceReferenceContentsEquals(left.Name, right.Name) {
				//return fmt.Errorf("validateObjectValue: object field '%s' must be unique", string(l.CachedName(left.Name)))
				return false
			}
		}
	}

	return true
}

func (l *Lookup) InputObjectTypeDefinitionByName(name document.ByteSliceReference) (document.InputObjectTypeDefinition, bool) {
	for _, definition := range l.p.ParsedDefinitions.InputObjectTypeDefinitions {
		if l.ByteSliceReferenceContentsEquals(definition.Name, name) {
			return definition, true
		}
	}
	return document.InputObjectTypeDefinition{}, false
}

func (l *Lookup) VariableDefinition(variableName document.ByteSliceReference, variableDefinitionRefs []int) (document.VariableDefinition, bool) {

	iter := l.VariableDefinitionIterator(variableDefinitionRefs)
	for iter.Next() {
		variableDefinition, _ := iter.Value()
		if l.ByteSliceReferenceContentsEquals(variableDefinition.Variable, variableName) {
			return variableDefinition, true
		}
	}

	return document.VariableDefinition{}, false
}

func (l *Lookup) EnumTypeDefinitionContainsValue(definition document.EnumTypeDefinition, enumValue document.ByteSliceReference) bool {

	if enumValue.Length() == 0 {
		return false
	}

	iter := l.EnumValueDefinitionIterator(definition.EnumValuesDefinition)

	for iter.Next() {
		enumValueDefinition, _ := iter.Value()
		if l.ByteSliceReferenceContentsEquals(enumValueDefinition.EnumValue, enumValue) {
			return true
		}
	}

	return false
}

type EnumValueDefinitionIterator struct {
	current int
	refs    []int
	l       *Lookup
}

func (e *EnumValueDefinitionIterator) Next() bool {
	e.current++
	return len(e.refs)-1 >= e.current
}

func (e *EnumValueDefinitionIterator) Value() (definition document.EnumValueDefinition, ref int) {
	ref = e.refs[e.current]
	definition = e.l.p.ParsedDefinitions.EnumValuesDefinitions[ref]
	return
}

func (l *Lookup) EnumValueDefinitionIterator(refs []int) EnumValueDefinitionIterator {
	return EnumValueDefinitionIterator{
		current: -1,
		refs:    refs,
		l:       l,
	}
}

func (l *Lookup) InputValueDefinitionIsRequired(inputValue document.InputValueDefinition) bool {
	return l.Type(inputValue.Type).Kind == document.TypeKindNON_NULL
}

func (l *Lookup) SelectionSetHasSelections(set document.SelectionSet) bool {
	return len(set.Fields)+len(set.FragmentSpreads)+len(set.InlineFragments) > 0
}

func (l *Lookup) FragmentSelectionsArePossible(onTypeName document.ByteSliceReference, selections document.SelectionSet) bool {

	var possibleTypes []document.ByteSliceReference
	l.PossibleSelectionTypes(onTypeName, &possibleTypes)

	inlineFragments := l.InlineFragmentIterable(selections.InlineFragments)
	for inlineFragments.Next() {
		fragment, _ := inlineFragments.Value()
		if fragment.TypeCondition == -1 {
			continue
		}
		typeCondition := l.Type(fragment.TypeCondition)
		if !l.ByteSliceReferencesContainName(possibleTypes, typeCondition.Name) {
			return false
		}
	}

	spreads := l.FragmentSpreadIterable(selections.FragmentSpreads)
	for spreads.Next() {
		spread, _ := spreads.Value()
		fragment, _, ok := l.FragmentDefinitionByName(spread.FragmentName)
		if !ok {
			return false
		}
		typeCondition := l.Type(fragment.TypeCondition)
		if !l.ByteSliceReferencesContainName(possibleTypes, typeCondition.Name) {
			return false
		}
	}
	return true
}

func (l *Lookup) ByteSliceReferencesContainName(refs []document.ByteSliceReference, name document.ByteSliceReference) bool {

	for i := range refs {
		if l.ByteSliceReferenceContentsEquals(refs[i], name) {
			return true
		}
	}

	return false
}

func (l *Lookup) UnionTypeDefinitionNamesContainingMember(memberName document.ByteSliceReference, nameRefs *[]document.ByteSliceReference) {
	for _, union := range l.p.ParsedDefinitions.UnionTypeDefinitions {
		if l.UnionTypeDefinitionContainsType(union, memberName) {
			*nameRefs = append(*nameRefs, union.Name)
		}
	}
}

type VariableDefinitionsIterator struct {
	current int
	refs    []int
	l       *Lookup
}

func (v *VariableDefinitionsIterator) Next() bool {
	v.current++
	return len(v.refs)-1 >= v.current
}

func (v *VariableDefinitionsIterator) Value() (definition document.VariableDefinition, index int) {
	index = v.refs[v.current]
	return v.l.p.ParsedDefinitions.VariableDefinitions[index], index
}

func (l *Lookup) VariableDefinitionIterator(refs []int) VariableDefinitionsIterator {
	return VariableDefinitionsIterator{
		l:       l,
		current: -1,
		refs:    refs,
	}
}

func (l *Lookup) TypeSatisfiesTypeSystemType(typeSystemTypeDefinition document.Type, executableTypeDefinition document.Type) bool {
	if executableTypeDefinition.Kind == document.TypeKindNON_NULL && typeSystemTypeDefinition.Kind != document.TypeKindNON_NULL {
		executableTypeDefinition = l.Type(executableTypeDefinition.OfType)
	}
	if typeSystemTypeDefinition.Kind != executableTypeDefinition.Kind {
		return false
	}
	if typeSystemTypeDefinition.Kind == document.TypeKindNAMED {
		if !l.ByteSliceReferenceContentsEquals(typeSystemTypeDefinition.Name, executableTypeDefinition.Name) {
			return false
		}
	}
	if typeSystemTypeDefinition.OfType == -1 && executableTypeDefinition.OfType == -1 {
		return true
	}

	return l.TypeSatisfiesTypeSystemType(l.Type(typeSystemTypeDefinition.OfType), l.Type(executableTypeDefinition.OfType))
}

func (l *Lookup) DirectiveLocationFromNode(node Node) document.DirectiveLocation {
	switch node.Kind {
	case FIELD:
		return document.DirectiveLocationFIELD
	case FIELD_DEFINITION:
		return document.DirectiveLocationFIELD_DEFINITION
	case OPERATION_DEFINITION:
		definition := l.OperationDefinition(node.Ref)
		switch definition.OperationType {
		case document.OperationTypeQuery:
			return document.DirectiveLocationQUERY
		case document.OperationTypeMutation:
			return document.DirectiveLocationMUTATION
		case document.OperationTypeSubscription:
			return document.DirectiveLocationSUBSCRIPTION
		}
	case INLINE_FRAGMENT:
		return document.DirectiveLocationINLINE_FRAGMENT
	case FRAGMENT_SPREAD:
		return document.DirectiveLocationFRAGMENT_SPREAD
	case FRAGMENT_DEFINITION:
		return document.DirectiveLocationFRAGMENT_DEFINITION
	}
	return document.DirectiveLocationUNKNOWN
}

func (l *Lookup) InputValueDefinitionHasDefaultValue(definition document.InputValueDefinition) bool {
	return definition.DefaultValue != -1
}

func (l *Lookup) OperationTypeName(definition document.OperationDefinition) document.ByteSliceReference {
	switch definition.OperationType {
	case document.OperationTypeQuery:
		return l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Query
	case document.OperationTypeMutation:
		return l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Mutation
	case document.OperationTypeSubscription:
		return l.p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.Subscription
	default:
		return document.ByteSliceReference{}
	}
}

func (l *Lookup) SelectionSetsAreOfSameResponseShape(leftSet, rightSet TypedSet) bool {

	left := l.SelectionSetCollectedFields(l.SelectionSet(leftSet.SetRef), leftSet.Type.Name)
	right := l.SelectionSetCollectedFields(l.SelectionSet(rightSet.SetRef), rightSet.Type.Name)

	for {
		leftNext := left.Next()
		rightNext := right.Next()
		if !leftNext && !rightNext {
			return true
		} else if leftNext != rightNext {
			return false
		}

		_, leftField := left.Value()
		_, rightField := right.Value()
		if !l.FieldResponseNamesAreEqual(leftField, rightField) {
			return false
		}

		leftType, ok := l.FieldType(leftSet.Type.Name, leftField.Name)
		if !ok {
			return false
		}
		rightType, ok := l.FieldType(rightSet.Type.Name, rightField.Name)
		if !ok {
			return false
		}

		if l.typesAreLeafNodes(leftType, rightType) && !l.TypesAreEqual(leftType, rightType) {
			return false
		}

		leftFieldTypedSet := TypedSet{
			Type:   leftType,
			SetRef: leftField.SelectionSet,
		}

		rightFieldTypedSet := TypedSet{
			Type:   rightType,
			SetRef: rightField.SelectionSet,
		}

		if !l.SelectionSetsAreOfSameResponseShape(leftFieldTypedSet, rightFieldTypedSet) {
			return false
		}
	}
}

func (l *Lookup) typesAreLeafNodes(left, right document.Type) bool {
	return l.IsLeafNode(l.UnwrappedNamedType(left).Name) &&
		l.IsLeafNode(l.UnwrappedNamedType(right).Name)
}

func (l *Lookup) FieldResponseNamesAreEqual(left, right document.Field) bool {
	return l.ByteSliceReferenceContentsEquals(l.responseFieldName(left), l.responseFieldName(right))
}

func (l *Lookup) responseFieldName(field document.Field) document.ByteSliceReference {
	if field.Alias.Length() != 0 {
		return field.Alias
	}
	return field.Name
}

func (l *Lookup) FieldNamesAndAliasesAreEqual(left, right document.Field) bool {
	return l.ByteSliceReferenceContentsEquals(left.Name, right.Name) && l.ByteSliceReferenceContentsEquals(left.Alias, right.Alias)
}

func (l *Lookup) FieldsDeepEqual(left, right document.Field) bool {
	if !l.FieldNamesAndAliasesAreEqual(left, right) {
		return false
	}
	if !l.ArgumentsAreEqual(l.ArgumentSet(left.ArgumentSet), l.ArgumentSet(right.ArgumentSet)) {
		return false
	}
	leftSet := l.SelectionSet(left.SelectionSet)
	rightSet := l.SelectionSet(right.SelectionSet)
	return l.SelectionSetDeepEqual(leftSet, rightSet)
}

func (l *Lookup) SelectionSetDeepEqual(left, right document.SelectionSet) bool {
	if len(left.Fields) != len(right.Fields) {
		return false
	}
	for i := range left.Fields {
		if !l.FieldsDeepEqual(l.Field(left.Fields[i]), l.Field(right.Fields[i])) {
			return false
		}
	}
	if len(left.InlineFragments) != len(right.InlineFragments) {
		return false
	}
	for i := range left.InlineFragments {
		if !l.InlineFragmentsDeepEqual(l.InlineFragment(left.InlineFragments[i]), l.InlineFragment(right.InlineFragments[i])) {
			return false
		}
	}
	if len(left.FragmentSpreads) != len(right.FragmentSpreads) {
		return false
	}
	for i := range left.FragmentSpreads {
		if !l.FragmentSpreadsDeepEqual(l.FragmentSpread(left.FragmentSpreads[i]), l.FragmentSpread(right.FragmentSpreads[i])) {
			return false
		}
	}
	return true
}

// TODO: add directives
func (l *Lookup) InlineFragmentsDeepEqual(left, right document.InlineFragment) bool {
	return l.SelectionSetDeepEqual(l.SelectionSet(left.SelectionSet), l.SelectionSet(right.SelectionSet))
}

func (l *Lookup) FragmentSpreadsDeepEqual(left, right document.FragmentSpread) bool {
	return left.FragmentName == right.FragmentName
}
