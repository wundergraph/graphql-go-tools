package astvalidation

import (
	"bytes"
	"hash/maphash"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// FieldSelectionMerging returns a validation rule that ensures field selections can be merged.
//
// This rule implements the validation described in the GraphQL specification section 5.3.2:
// "Field Selection Merging". It ensures that when multiple fields with the same response key
// (name or alias) are selected in overlapping selection sets, they can be unambiguously merged
// into a single field in the response.
//
// The rule is applied to each operation and fragment definition in the document.
func FieldSelectionMerging(relaxNullabilityCheck ...bool) Rule {
	relax := len(relaxNullabilityCheck) > 0 && relaxNullabilityCheck[0]
	return func(walker *astvisitor.Walker) {
		visitor := fieldSelectionMergingVisitor{
			Walker:                walker,
			relaxNullabilityCheck: relax,
			visitedGroups:         map[visitedKey]int{},
			seed:                  maphash.MakeSeed(),
			inherited:             map[[2]int]fieldPair{},
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterFieldVisitor(&visitor)
		walker.RegisterEnterOperationVisitor(&visitor)
		walker.RegisterEnterFragmentDefinitionVisitor(&visitor)
	}
}

type fieldSelectionMergingVisitor struct {
	*astvisitor.Walker

	definition, operation *ast.Document
	relaxNullabilityCheck bool

	visited       [][]int              // refs of the entered fields in the operation or fragment, grouped by visitedKey
	visitedGroups map[visitedKey]int   // index of a group in visited
	seed          maphash.Seed         // for hashes of response keys in visitedKey
	fields        []mergingField       // lookup table indexed by field refs
	inherited     map[[2]int]fieldPair // fieldPair inherited from two parent fields, keyed by their refs
}

// visitedKey is what fields with the same response path share: the depth and the response key.
// It holds a hash of the response key, because a string key would allocate for every new group.
type visitedKey struct {
	depth int
	hash  uint64
}

// mergingField is an entered field.
type mergingField struct {
	key       ast.ByteSlice // response key
	depth     int           // number of ancestor fields
	parent    int           // nearest ancestor field; ast.InvalidRef for root fields
	enclosing ast.Node      // type the field is selected on
	typeRef   int           // ast.InvalidRef for __typename, which is String!
	typeNode  ast.Node      // ast.InvalidNode for __typename
}

// fieldRelation is how two fields with the same response path relate.
type fieldRelation uint8

const (
	sameField       fieldRelation = iota // equal names and arguments at every level below their common parent
	exclusive                            // never in the same response object
	differentFields                      // different fields in the same response object
)

// fieldPair is what two fields with the same response path inherit from their parent fields.
type fieldPair struct {
	relation fieldRelation
	strict   bool // below a pair merged by response shape, so the pair must have the same response shape
}

func (f *fieldSelectionMergingVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
	if cap(f.fields) < len(operation.Fields) {
		f.fields = make([]mergingField, len(operation.Fields))
	}
	f.fields = f.fields[:len(operation.Fields)]
	clear(f.inherited)
}

func (f *fieldSelectionMergingVisitor) EnterFragmentDefinition(_ int) {
	f.resetVisited()
}

func (f *fieldSelectionMergingVisitor) EnterOperationDefinition(_ int) {
	f.resetVisited()
}

func (f *fieldSelectionMergingVisitor) resetVisited() {
	f.visited = f.visited[:0]
	clear(f.visitedGroups)
}

func (f *fieldSelectionMergingVisitor) EnterField(ref int) {
	field := mergingField{
		key:       f.operation.FieldAliasOrNameBytes(ref),
		parent:    f.EnclosingFieldRef(),
		enclosing: f.EnclosingTypeDefinition,
		typeRef:   ast.InvalidRef,
		typeNode:  ast.InvalidNode,
	}
	if field.parent != ast.InvalidRef {
		field.depth = f.fields[field.parent].depth + 1
	}
	name := f.operation.FieldNameBytes(ref)
	if !bytes.Equal(name, literal.TYPENAME) {
		definition, ok := f.definition.NodeFieldDefinitionByName(f.EnclosingTypeDefinition, name)
		if !ok {
			enclosingTypeName := f.definition.NodeNameBytes(f.EnclosingTypeDefinition)
			f.StopWithExternalErr(operationreport.ErrFieldUndefinedOnType(name, enclosingTypeName))
			return
		}
		field.typeRef = f.definition.FieldDefinitionType(definition)
		field.typeNode = f.definition.FieldDefinitionTypeNode(definition)
	}
	f.fields[ref] = field

	group := f.visitedGroup(field)
	for _, other := range f.visited[group] {
		if f.sameResponsePath(other, ref) && !f.canMergeFields(other, ref) {
			return
		}
	}
	f.visited[group] = append(f.visited[group], ref)
}

// visitedGroup returns the index of the group in visited with the depth and response key of the field.
// It adds an empty group when there is none.
func (f *fieldSelectionMergingVisitor) visitedGroup(field mergingField) int {
	key := visitedKey{depth: field.depth, hash: maphash.Bytes(f.seed, field.key)}
	if group, ok := f.visitedGroups[key]; ok {
		return group
	}
	group := len(f.visited)
	f.visitedGroups[key] = group
	if group < cap(f.visited) {
		// Reuse the slice of a group from an earlier operation or fragment.
		f.visited = f.visited[:group+1]
		f.visited[group] = f.visited[group][:0]
	} else {
		f.visited = append(f.visited, nil)
	}
	return group
}

// sameResponsePath reports whether two fields have equal response keys at every level.
func (f *fieldSelectionMergingVisitor) sameResponsePath(a, b int) bool {
	if f.fields[a].depth != f.fields[b].depth {
		return false
	}
	for a != b {
		if !bytes.Equal(f.fields[a].key, f.fields[b].key) {
			return false
		}
		a, b = f.fields[a].parent, f.fields[b].parent
	}
	return true
}

// pairOf returns how two field refs with the same response path relate.
// The first level, counting from the common parent, that is not the same field, decides the relation.
func (f *fieldSelectionMergingVisitor) pairOf(a, b int) fieldPair {
	aParent, bParent := f.fields[a].parent, f.fields[b].parent
	pair := fieldPair{relation: sameField}
	if aParent != bParent {
		// Every pair of children asks for the parent pair, and mergesByResponseShape can be slow.
		key := [2]int{aParent, bParent}
		inherited, found := f.inherited[key]
		if !found {
			parents := f.pairOf(aParent, bParent)
			inherited = fieldPair{
				relation: parents.relation,
				strict:   parents.strict || f.mergesByResponseShape(parents, aParent, bParent),
			}
			f.inherited[key] = inherited
		}
		pair = inherited
	}
	if pair.relation != sameField {
		return pair
	}
	switch {
	case f.onDifferentObjectTypes(a, b):
		pair.relation = exclusive
	case !bytes.Equal(f.operation.FieldNameBytes(a), f.operation.FieldNameBytes(b)) ||
		!f.operation.ArgumentSetsAreEquals(f.operation.FieldArguments(a), f.operation.FieldArguments(b)):
		pair.relation = differentFields
	}
	return pair
}

// canMergeFields reports whether two field refs with the same response path can merge into one response field.
func (f *fieldSelectionMergingVisitor) canMergeFields(a, b int) bool {
	pair := f.pairOf(a, b)
	if pair.strict && !f.sameResponseShape(a, b) {
		f.stopWithTypesConflict(operationreport.ErrTypesForFieldMismatch, a, b)
		return false
	}

	f1, f2 := f.fields[a], f.fields[b]
	aIsScalar := f1.typeNode.Kind == ast.NodeKindScalarTypeDefinition
	bIsScalar := f2.typeNode.Kind == ast.NodeKindScalarTypeDefinition
	switch {
	case f1.typeRef == ast.InvalidRef || f2.typeRef == ast.InvalidRef || aIsScalar != bIsScalar:
		// A pair with __typename, or with a scalar and a non-scalar,
		// is checked only by sameResponseShape for strict pairs.
		return true
	case aIsScalar:
		return f.canMergeScalars(pair, a, b)
	default:
		return f.canMergeNonScalars(pair, a, b)
	}
}

func (f *fieldSelectionMergingVisitor) canMergeScalars(pair fieldPair, a, b int) bool {
	if pair.relation != exclusive && f.potentiallySameObject(f.fields[a].enclosing, f.fields[b].enclosing) &&
		!f.operation.FieldsAreEqualFlat(a, b, false) {
		f.StopWithExternalErr(operationreport.ErrDifferingFieldsOnPotentiallySameType(f.operation.FieldAliasOrNameBytes(b)))
		return false
	}
	if f.definition.TypesAreCompatibleDeep(f.fields[a].typeRef, f.fields[b].typeRef) || f.relaxedNullability(a, b) {
		return true
	}
	f.stopWithTypesConflict(operationreport.ErrFieldsConflict, a, b)
	return false
}

func (f *fieldSelectionMergingVisitor) canMergeNonScalars(pair fieldPair, a, b int) bool {
	// TODO: compare names and arguments of non-scalar fields too, unless they are mutually exclusive.
	// Otherwise different fields with equal subfields pass,
	// e.g. `s: left { __typename } s: right { __typename }`,
	// and so do different enum fields, e.g. `v: a v: b`.
	if !f.potentiallySameObject(f.fields[a].typeNode, f.fields[b].typeNode) {
		// Enums, unions and two different object types are not compared.
		return true
	}
	if !f.operation.DirectiveSetsHasCompatibleStreamDirective(f.operation.FieldDirectives(a), f.operation.FieldDirectives(b)) {
		f.StopWithExternalErr(operationreport.ErrConflictingStreamDirectivesOnField(f.operation.FieldAliasOrNameBytes(b)))
		return false
	}
	if f.definition.TypesAreCompatibleDeep(f.fields[a].typeRef, f.fields[b].typeRef) ||
		f.mergesByResponseShape(pair, a, b) || f.relaxedNullability(a, b) {
		return true
	}
	f.stopWithTypesConflict(operationreport.ErrTypesForFieldMismatch, a, b)
	return false
}

// mergesByResponseShape reports whether two fields merge only because their responses have the same shape:
// their types are differently named interfaces or objects with the same list and non-null wrappers,
// and either the fields never share a response object, or they are the same field.
// Then all fields below them must have the same response shape too.
// See https://spec.graphql.org/September2025/#FieldsInSetCanMerge()
func (f *fieldSelectionMergingVisitor) mergesByResponseShape(pair fieldPair, a, b int) bool {
	return pair.relation != differentFields &&
		f.potentiallySameObject(f.fields[a].typeNode, f.fields[b].typeNode) &&
		!f.definition.TypesAreCompatibleDeep(f.fields[a].typeRef, f.fields[b].typeRef) &&
		f.sameResponseShape(a, b)
}

// sameResponseShape compares two fields on one level, per SameResponseShape.
// Leaf types must be equal, composite types must have equal list and non-null wrappers.
// Subfields are compared when the walker enters them.
// See https://spec.graphql.org/September2025/#SameResponseShape()
func (f *fieldSelectionMergingVisitor) sameResponseShape(a, b int) bool {
	aField, bField := f.fields[a], f.fields[b]
	aComposite := aField.typeNode.Kind.IsCompositeType()
	bComposite := bField.typeNode.Kind.IsCompositeType()
	if !aComposite || !bComposite {
		return !aComposite && !bComposite && f.leafTypesAreEqual(aField.typeRef, bField.typeRef)
	}
	return f.typeWrappersAreEqual(aField.typeRef, bField.typeRef)
}

// relaxedNullability reports whether relaxNullabilityCheck lets two fields differ in nullability.
// Deliberate deviation from SameResponseShape: when enclosing types cannot overlap at runtime,
// only one of the fields contributes to the response.
func (f *fieldSelectionMergingVisitor) relaxedNullability(a, b int) bool {
	return f.relaxNullabilityCheck &&
		!f.potentiallySameObject(f.fields[a].enclosing, f.fields[b].enclosing) &&
		f.definition.TypesAreCompatibleIgnoringNullability(f.fields[a].typeRef, f.fields[b].typeRef)
}

func (f *fieldSelectionMergingVisitor) stopWithTypesConflict(conflict func(objectName, leftType, rightType ast.ByteSlice) operationreport.ExternalError, a, b int) {
	left, err := f.printType(f.fields[a].typeRef)
	if err != nil {
		f.StopWithInternalErr(err)
		return
	}
	right, err := f.printType(f.fields[b].typeRef)
	if err != nil {
		f.StopWithInternalErr(err)
		return
	}
	f.StopWithExternalErr(conflict(f.operation.FieldAliasOrNameBytes(b), left, right))
}

func (f *fieldSelectionMergingVisitor) printType(ref int) ([]byte, error) {
	if ref == ast.InvalidRef {
		return []byte("String!"), nil
	}
	return f.definition.PrintTypeBytes(ref, nil)
}

// onDifferentObjectTypes reports whether two fields are selected on two different object types.
func (f *fieldSelectionMergingVisitor) onDifferentObjectTypes(a, b int) bool {
	left, right := f.fields[a].enclosing, f.fields[b].enclosing
	return left.Kind == ast.NodeKindObjectTypeDefinition && left.Kind == right.Kind && !f.potentiallySameObject(left, right)
}

// leafTypesAreEqual compares leaf types, treating ast.InvalidRef as the String! type of __typename.
func (f *fieldSelectionMergingVisitor) leafTypesAreEqual(a, b int) bool {
	switch {
	case a == ast.InvalidRef && b == ast.InvalidRef:
		return true
	case a == ast.InvalidRef:
		return f.isNonNullString(b)
	case b == ast.InvalidRef:
		return f.isNonNullString(a)
	default:
		return f.definition.TypesAreEqualDeep(a, b)
	}
}

// isNonNullString reports whether a schema type is exactly String!.
func (f *fieldSelectionMergingVisitor) isNonNullString(ref int) bool {
	if !f.definition.TypeIsNonNull(ref) {
		return false
	}
	ofType := f.definition.Types[ref].OfType
	return f.definition.Types[ofType].TypeKind == ast.TypeKindNamed && bytes.Equal(f.definition.TypeNameBytes(ofType), literal.STRING)
}

// typeWrappersAreEqual compares list and non-null wrappers, ignoring the named type.
func (f *fieldSelectionMergingVisitor) typeWrappersAreEqual(a, b int) bool {
	for {
		aType, bType := f.definition.Types[a], f.definition.Types[b]
		if aType.TypeKind != bType.TypeKind {
			return false
		}
		if aType.TypeKind == ast.TypeKindNamed {
			return true
		}
		a, b = aType.OfType, bType.OfType
	}
}

// potentiallySameObject reports whether one runtime object can have both types.
// Two interfaces always can, because some object might implement both.
// An object and an interface can, when the object implements the interface.
// Two objects can, when they have the same name.
// Unions and leaf types report false.
func (f *fieldSelectionMergingVisitor) potentiallySameObject(a, b ast.Node) bool {
	switch {
	case a.Kind == ast.NodeKindInterfaceTypeDefinition && b.Kind == ast.NodeKindInterfaceTypeDefinition:
		return true
	case a.Kind == ast.NodeKindInterfaceTypeDefinition:
		return f.definition.NodeImplementsInterfaceNode(b, a)
	case b.Kind == ast.NodeKindInterfaceTypeDefinition:
		return f.definition.NodeImplementsInterfaceNode(a, b)
	default:
		return a.Kind == ast.NodeKindObjectTypeDefinition && b.Kind == ast.NodeKindObjectTypeDefinition &&
			bytes.Equal(f.definition.ObjectTypeDefinitionNameBytes(a.Ref), f.definition.ObjectTypeDefinitionNameBytes(b.Ref))
	}
}
