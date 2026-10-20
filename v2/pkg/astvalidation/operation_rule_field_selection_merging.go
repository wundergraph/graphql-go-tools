package astvalidation

import (
	"bytes"

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
		visitor := fieldSelectionMergingVisitor{Walker: walker, relaxNullabilityCheck: relax}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterFieldVisitor(&visitor)
		walker.RegisterEnterOperationVisitor(&visitor)
		walker.RegisterEnterFragmentDefinitionVisitor(&visitor)
	}
}

type fieldSelectionMergingVisitor struct {
	*astvisitor.Walker

	definition, operation *ast.Document
	scalarRequirements    scalarRequirements
	nonScalarRequirements nonScalarRequirements
	refs                  []int
	scopes                []fieldScope // lookup table indexed by field refs
	relaxNullabilityCheck bool
}

type nonScalarRequirement struct {
	path                    ast.Path
	objectName              ast.ByteSlice
	fieldRef                int
	fieldTypeRef            int
	fieldTypeDefinitionNode ast.Node
	enclosingTypeDefinition ast.Node
}

type scalarRequirement struct {
	path                    ast.Path
	objectName              ast.ByteSlice
	fieldRef                int
	fieldType               int
	enclosingTypeDefinition ast.Node
	fieldTypeDefinitionNode ast.Node
}

type nonScalarRequirements []nonScalarRequirement
type scalarRequirements []scalarRequirement

// fieldScope is where an operation field is selected.
type fieldScope struct {
	parent    int      // the ref of the nearest ancestor field; ast.InvalidRef for root fields
	enclosing ast.Node // walker's current enclosing type
}

// selectedField is an operation field and the type it is selected on.
type selectedField struct {
	ref       int
	enclosing ast.Node
}

func (f *fieldSelectionMergingVisitor) NonScalarRequirementsByPathField(path ast.Path, objectName ast.ByteSlice) []int {
	f.refs = f.refs[:0]
	for i := range f.nonScalarRequirements {
		if f.nonScalarRequirements[i].path.Equals(path) && f.nonScalarRequirements[i].objectName.Equals(objectName) {
			f.refs = append(f.refs, i)
		}
	}
	return f.refs
}

func (f *fieldSelectionMergingVisitor) ScalarRequirementsByPathField(path ast.Path, objectName ast.ByteSlice) []int {
	f.refs = f.refs[:0]
	for i := range f.scalarRequirements {
		if f.scalarRequirements[i].path.Equals(path) && f.scalarRequirements[i].objectName.Equals(objectName) {
			f.refs = append(f.refs, i)
		}
	}
	return f.refs
}

func (f *fieldSelectionMergingVisitor) resetRequirements() {
	f.scalarRequirements = f.scalarRequirements[:0]
	f.nonScalarRequirements = f.nonScalarRequirements[:0]
}

func (f *fieldSelectionMergingVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
	if cap(f.scopes) < len(operation.Fields) {
		f.scopes = make([]fieldScope, len(operation.Fields))
	}
	f.scopes = f.scopes[:len(operation.Fields)]
}

func (f *fieldSelectionMergingVisitor) EnterFragmentDefinition(_ int) {
	f.resetRequirements()
}

func (f *fieldSelectionMergingVisitor) EnterOperationDefinition(_ int) {
	f.resetRequirements()
}

func (f *fieldSelectionMergingVisitor) EnterField(ref int) {
	f.scopes[ref] = fieldScope{parent: f.EnclosingFieldRef(), enclosing: f.EnclosingTypeDefinition}

	path := f.Path.WithoutInlineFragmentNames()

	fieldName := f.operation.FieldNameBytes(ref)
	if bytes.Equal(fieldName, literal.TYPENAME) {
		return
	}
	objectName := f.operation.FieldAliasOrNameBytes(ref)
	definition, ok := f.definition.NodeFieldDefinitionByName(f.EnclosingTypeDefinition, fieldName)
	if !ok {
		enclosingTypeName := f.definition.NodeNameBytes(f.EnclosingTypeDefinition)
		f.StopWithExternalErr(operationreport.ErrFieldUndefinedOnType(fieldName, enclosingTypeName))
		return
	}

	fieldType := f.definition.FieldDefinitionType(definition)
	fieldDefinitionTypeNode := f.definition.FieldDefinitionTypeNode(definition)

	// TODO: compare names and arguments of non-scalar fields too, unless they are mutually exclusive.
	// Otherwise different fields with equal subfields pass,
	// e.g. `s: left { __typename } s: right { __typename }`,
	// and so do different enum fields, e.g. `v: a v: b`.
	if fieldDefinitionTypeNode.Kind != ast.NodeKindScalarTypeDefinition {
		current := nonScalarRequirement{
			path:                    path,
			objectName:              objectName,
			fieldRef:                ref,
			fieldTypeRef:            fieldType,
			fieldTypeDefinitionNode: fieldDefinitionTypeNode,
			enclosingTypeDefinition: f.EnclosingTypeDefinition,
		}
		matchedRequirements := f.NonScalarRequirementsByPathField(path, objectName)
		hasDifferentKindInRequirements := false

		for _, i := range matchedRequirements {
			if !f.potentiallySameObject(fieldDefinitionTypeNode, f.nonScalarRequirements[i].fieldTypeDefinitionNode) {
				// This condition below can never be true because if objects aren't potentially the same,
				// and we know objectNames are equal (from the filter), they cannot be not equal at the same time.
				// Perhaps this should be remove altogether?
				if !objectName.Equals(f.nonScalarRequirements[i].objectName) {
					f.StopWithExternalErr(operationreport.ErrResponseOfDifferingTypesMustBeOfSameShape(objectName, f.nonScalarRequirements[i].objectName))
					return
				}
			} else {
				// Check stream directive compatibility for non-scalar fields
				leftDirectives := f.operation.FieldDirectives(f.nonScalarRequirements[i].fieldRef)
				rightDirectives := f.operation.FieldDirectives(ref)
				if !f.operation.DirectiveSetsHasCompatibleStreamDirective(leftDirectives, rightDirectives) {
					f.StopWithExternalErr(operationreport.ErrConflictingStreamDirectivesOnField(objectName))
					return
				}

				if !f.definition.TypesAreCompatibleDeep(f.nonScalarRequirements[i].fieldTypeRef, fieldType) &&
					!f.mergesByResponseShape(f.nonScalarRequirements[i], current) {
					// Deliberate deviation from SameResponseShape (spec sec 5.3.2): when enclosing
					// types cannot overlap at runtime (two distinct concrete object types),
					// we allow nullability differences because only one branch will ever
					// contribute to the response. This is gated behind relaxNullabilityCheck.
					if !f.relaxNullabilityCheck ||
						f.potentiallySameObject(f.nonScalarRequirements[i].enclosingTypeDefinition, f.EnclosingTypeDefinition) ||
						!f.definition.TypesAreCompatibleIgnoringNullability(f.nonScalarRequirements[i].fieldTypeRef, fieldType) {
						left, err := f.definition.PrintTypeBytes(f.nonScalarRequirements[i].fieldTypeRef, nil)
						if err != nil {
							f.StopWithInternalErr(err)
							return
						}
						right, err := f.definition.PrintTypeBytes(fieldType, nil)
						if err != nil {
							f.StopWithInternalErr(err)
							return
						}
						f.StopWithExternalErr(operationreport.ErrTypesForFieldMismatch(objectName, left, right))
						return
					}
				}
			}

			if fieldDefinitionTypeNode.Kind != f.nonScalarRequirements[i].fieldTypeDefinitionNode.Kind {
				hasDifferentKindInRequirements = true
			}
		}

		if hasDifferentKindInRequirements {
			// If we've already checked this field against a requirement with a different Kind,
			// we don't need to add it again to requirements.
			return
		}

		f.nonScalarRequirements = append(f.nonScalarRequirements, current)
		return
	}

	matchedRequirements := f.ScalarRequirementsByPathField(path, objectName)
	hasDifferentKindInRequirements := false

	for _, i := range matchedRequirements {
		if f.potentiallySameObject(f.scalarRequirements[i].enclosingTypeDefinition, f.EnclosingTypeDefinition) &&
			!f.mutuallyExclusive(f.scalarRequirements[i].fieldRef, ref) {
			// here we do not check directives equality, only if the stream directives are the same for the fields
			if !f.operation.FieldsAreEqualFlat(f.scalarRequirements[i].fieldRef, ref, false) {
				f.StopWithExternalErr(operationreport.ErrDifferingFieldsOnPotentiallySameType(objectName))
				return
			}
		}
		if !f.definition.TypesAreCompatibleDeep(f.scalarRequirements[i].fieldType, fieldType) {
			// Per SameResponseShape (spec sec 5.3.2), when enclosing types cannot overlap
			// at runtime (two distinct concrete object types), nullability differences are
			// acceptable because only one branch will ever contribute to the response.
			// This relaxation is gated behind the relaxNullabilityCheck flag.
			if !f.relaxNullabilityCheck ||
				f.potentiallySameObject(f.scalarRequirements[i].enclosingTypeDefinition, f.EnclosingTypeDefinition) ||
				!f.definition.TypesAreCompatibleIgnoringNullability(f.scalarRequirements[i].fieldType, fieldType) {
				left, err := f.definition.PrintTypeBytes(f.scalarRequirements[i].fieldType, nil)
				if err != nil {
					f.StopWithInternalErr(err)
					return
				}
				right, err := f.definition.PrintTypeBytes(fieldType, nil)
				if err != nil {
					f.StopWithInternalErr(err)
					return
				}
				f.StopWithExternalErr(operationreport.ErrFieldsConflict(objectName, left, right))
				return
			}
		}

		if fieldDefinitionTypeNode.Kind != f.scalarRequirements[i].fieldTypeDefinitionNode.Kind {
			hasDifferentKindInRequirements = true
		}
	}

	if hasDifferentKindInRequirements {
		return
	}

	f.scalarRequirements = append(f.scalarRequirements, scalarRequirement{
		path:                    path,
		objectName:              objectName,
		fieldRef:                ref,
		fieldType:               fieldType,
		enclosingTypeDefinition: f.EnclosingTypeDefinition,
		fieldTypeDefinitionNode: fieldDefinitionTypeNode,
	})
}

// mergesByResponseShape reports whether two fields with different type names can merge.
// They can, when they return the same JSON,
// and either never apply to the same object, or are the same field with the same arguments.
// See https://spec.graphql.org/September2025/#FieldsInSetCanMerge()
func (f *fieldSelectionMergingVisitor) mergesByResponseShape(a, b nonScalarRequirement) bool {
	exclusive, identical := f.fieldsRelation(a.fieldRef, b.fieldRef)
	return (exclusive || identical) && f.haveSameResponseShape(
		selectedField{ref: a.fieldRef, enclosing: a.enclosingTypeDefinition},
		selectedField{ref: b.fieldRef, enclosing: b.enclosingTypeDefinition},
	)
}

func (f *fieldSelectionMergingVisitor) mutuallyExclusive(a, b int) bool {
	exclusive, _ := f.fieldsRelation(a, b)
	return exclusive
}

// fieldsRelation compares two fields (passed by ref) with equal paths,
// then their parents, pair by pair, up to the lowest common field.
//
// They are identical, when every pair has equal names and arguments.
//
// They are mutually exclusive, when they never end up in the same response object.
// Fields in fragments on two different object types are exclusive,
// but only while their parents are the same field and return the same object:
//
//	p: parent { ... on Left { list } }  p: parent { ... on Right { list } }  # exclusive
//	b: left { list }                    b: right { list }                    # not exclusive
//
// In the second line both b fields are resolved for the same object,
// so both list fields end up in one b.
func (f *fieldSelectionMergingVisitor) fieldsRelation(a, b int) (exclusive, identical bool) {
	identical = true
	for {
		same := bytes.Equal(f.operation.FieldNameBytes(a), f.operation.FieldNameBytes(b)) &&
			f.operation.ArgumentSetsAreEquals(f.operation.FieldArguments(a), f.operation.FieldArguments(b))
		if !same {
			// Different fields return different objects, so exclusivity found below is lost.
			exclusive = false
		}
		if f.onDifferentObjectTypes(a, b) {
			exclusive = true
		}
		identical = identical && same
		if f.scopes[a].parent == f.scopes[b].parent {
			return exclusive, identical
		}
		a, b = f.scopes[a].parent, f.scopes[b].parent
	}
}

// onDifferentObjectTypes reports whether two fields are selected on two different object types.
func (f *fieldSelectionMergingVisitor) onDifferentObjectTypes(a, b int) bool {
	left, right := f.scopes[a].enclosing, f.scopes[b].enclosing
	return left.Kind == ast.NodeKindObjectTypeDefinition && left.Kind == right.Kind && !f.potentiallySameObject(left, right)
}

// haveSameResponseShape implements SameResponseShape for fields that never share a response object.
// Leaf types must be equal, composite types must have equal wrappers and same-shaped subfields.
// It reports false for unknown fields, unknown types and fragment spreads, so they still require equal type names.
// See https://spec.graphql.org/September2025/#SameResponseShape()
func (f *fieldSelectionMergingVisitor) haveSameResponseShape(a, b selectedField) bool {
	aType, aNode, ok := f.selectedFieldType(a)
	if !ok {
		return false
	}
	bType, bNode, ok := f.selectedFieldType(b)
	if !ok {
		return false
	}
	aComposite, bComposite := aNode.Kind.IsCompositeType(), bNode.Kind.IsCompositeType()
	if !aComposite || !bComposite {
		return !aComposite && !bComposite && f.leafTypesAreEqual(aType, bType)
	}
	aSet, aHasSet := f.operation.FieldSelectionSet(a.ref)
	bSet, bHasSet := f.operation.FieldSelectionSet(b.ref)
	if !aHasSet || !bHasSet || !f.typeWrappersAreEqual(aType, bType) {
		return false
	}
	aFields, ok := f.collectSelectedFields(aSet, aNode, nil)
	if !ok {
		return false
	}
	bFields, ok := f.collectSelectedFields(bSet, bNode, nil)
	if !ok {
		return false
	}
	for _, aField := range aFields {
		for _, bField := range bFields {
			if bytes.Equal(f.operation.FieldAliasOrNameBytes(aField.ref), f.operation.FieldAliasOrNameBytes(bField.ref)) &&
				!f.haveSameResponseShape(aField, bField) {
				return false
			}
		}
	}
	return true
}

// selectedFieldType returns the type of the selected field.
// __typename might not have field definition, its type is ast.InvalidRef.
func (f *fieldSelectionMergingVisitor) selectedFieldType(field selectedField) (typeRef int, typeNode ast.Node, ok bool) {
	name := f.operation.FieldNameBytes(field.ref)
	if bytes.Equal(name, literal.TYPENAME) {
		return ast.InvalidRef, ast.InvalidNode, true
	}
	definition, ok := f.definition.NodeFieldDefinitionByName(field.enclosing, name)
	if !ok {
		return ast.InvalidRef, ast.InvalidNode, false
	}
	return f.definition.FieldDefinitionType(definition), f.definition.FieldDefinitionTypeNode(definition), true
}

// collectSelectedFields appends the fields of a selection set, flattening inline fragments.
// It reports false for fragment spreads and unknown type conditions.
func (f *fieldSelectionMergingVisitor) collectSelectedFields(set int, enclosing ast.Node, fields []selectedField) ([]selectedField, bool) {
	for _, selection := range f.operation.SelectionSets[set].SelectionRefs {
		ref := f.operation.Selections[selection].Ref
		switch f.operation.Selections[selection].Kind {
		case ast.SelectionKindField:
			fields = append(fields, selectedField{ref: ref, enclosing: enclosing})
		case ast.SelectionKindInlineFragment:
			fragmentType := enclosing
			if f.operation.InlineFragmentHasTypeCondition(ref) {
				node, ok := f.definition.Index.FirstNodeByNameBytes(f.operation.InlineFragmentTypeConditionName(ref))
				if !ok {
					return nil, false
				}
				fragmentType = node
			}
			fragmentSet, ok := f.operation.InlineFragmentSelectionSet(ref)
			if !ok {
				continue
			}
			if fields, ok = f.collectSelectedFields(fragmentSet, fragmentType, fields); !ok {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return fields, true
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
	if f.definition.Types[ref].TypeKind != ast.TypeKindNonNull {
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

// potentiallySameObject reports whether two enclosing type definitions could apply
// to the same runtime object. This determines whether field merging must enforce
// strict type equality (including nullability) or may relax it.
//
//   - If one type is an interface and the other is an object, returns true only
//     when the object type implements the interface (otherwise they cannot overlap).
//   - If both types are interfaces, returns true (conservative: some concrete
//     type might implement both).
//   - Two object types overlap only when they share the same name.
//   - All other combinations return false.
func (f *fieldSelectionMergingVisitor) potentiallySameObject(a, b ast.Node) bool {
	switch {
	case a.Kind == ast.NodeKindInterfaceTypeDefinition && b.Kind == ast.NodeKindInterfaceTypeDefinition:
		return true
	case a.Kind == ast.NodeKindInterfaceTypeDefinition && b.Kind == ast.NodeKindObjectTypeDefinition:
		return f.definition.NodeImplementsInterfaceNode(b, a)
	case a.Kind == ast.NodeKindObjectTypeDefinition && b.Kind == ast.NodeKindInterfaceTypeDefinition:
		return f.definition.NodeImplementsInterfaceNode(a, b)
	case a.Kind == ast.NodeKindObjectTypeDefinition && b.Kind == ast.NodeKindObjectTypeDefinition:
		return bytes.Equal(f.definition.ObjectTypeDefinitionNameBytes(a.Ref), f.definition.ObjectTypeDefinitionNameBytes(b.Ref))
	default:
		return false
	}
}
