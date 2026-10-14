package astvalidation

import (
	"bytes"
	"slices"

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
// For compatibility, an interface and an object that does not implement it may
// select different field names or arguments under the same response name. This
// deliberately relaxes FieldsInSetCanMerge; response shapes are still checked.
// Setting relaxNullabilityCheck permits nullability differences between fields
// with non-overlapping enclosing types, deliberately relaxing the spec.
//
// See https://spec.graphql.org/September2025/#sec-Field-Selection-Merging
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
	requirements          []fieldRequirement
	relaxNullabilityCheck bool
}

type fieldRequirement struct {
	path                    ast.Path
	responseName            ast.ByteSlice
	fieldRef                int
	fieldTypeRef            int // InvalidRef denotes the implicit String! type of __typename.
	fieldTypeDefinitionNode ast.Node
	enclosingTypeDefinition ast.Node
	parent                  int
}

func (f *fieldSelectionMergingVisitor) resetRequirements() {
	f.requirements = f.requirements[:0]
}

func (f *fieldSelectionMergingVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
	f.definition = definition
}

func (f *fieldSelectionMergingVisitor) EnterFragmentDefinition(_ int) {
	f.resetRequirements()
}

func (f *fieldSelectionMergingVisitor) EnterOperationDefinition(_ int) {
	f.resetRequirements()
}

func (f *fieldSelectionMergingVisitor) EnterField(ref int) {
	fieldName := f.operation.FieldNameBytes(ref)
	// __typename is an implicit String! field even without the base schema.
	// See https://spec.graphql.org/September2025/#sec-Type-Name-Introspection
	fieldType := ast.InvalidRef
	fieldTypeNode := ast.Node{Kind: ast.NodeKindScalarTypeDefinition, Ref: ast.InvalidRef}
	if !bytes.Equal(fieldName, literal.TYPENAME) {
		definition, ok := f.definition.NodeFieldDefinitionByName(f.EnclosingTypeDefinition, fieldName)
		if !ok {
			enclosingTypeName := f.definition.NodeNameBytes(f.EnclosingTypeDefinition)
			f.StopWithExternalErr(operationreport.ErrFieldUndefinedOnType(fieldName, enclosingTypeName))
			return
		}
		fieldType = f.definition.FieldDefinitionType(definition)
		fieldTypeNode = f.definition.FieldDefinitionTypeNode(definition)
	}
	current := fieldRequirement{
		path:                    f.Path.WithoutInlineFragmentNames(),
		responseName:            f.operation.FieldAliasOrNameBytes(ref),
		fieldRef:                ref,
		fieldTypeRef:            fieldType,
		fieldTypeDefinitionNode: fieldTypeNode,
		enclosingTypeDefinition: f.EnclosingTypeDefinition,
		parent:                  f.parentRequirement(),
	}
	for i := range f.requirements {
		previous := &f.requirements[i]
		if !previous.path.Equals(current.path) || !previous.responseName.Equals(current.responseName) {
			continue
		}
		bothComposite := f.nodeIsCompositeType(previous.fieldTypeDefinitionNode) && f.nodeIsCompositeType(current.fieldTypeDefinitionNode)
		// FieldsInSetCanMerge step 2.2 requires matching names and arguments outside
		// mutually exclusive branches. The compatibility exception documented on
		// FieldSelectionMerging also applies; SameResponseShape is always checked.
		// See https://spec.graphql.org/September2025/#FieldsInSetCanMerge()
		identityConflict := false
		if !f.fieldsAreMutuallyExclusive(previous, &current) {
			compatibleStream := f.operation.DirectiveSetsHasCompatibleStreamDirective(f.operation.FieldDirectives(previous.fieldRef), f.operation.FieldDirectives(ref))
			if !compatibleStream {
				if bothComposite {
					f.StopWithExternalErr(operationreport.ErrConflictingStreamDirectivesOnField(current.responseName))
				} else {
					f.StopWithExternalErr(operationreport.ErrDifferingFieldsOnPotentiallySameType(current.responseName))
				}
				return
			}
			identityConflict = !bytes.Equal(f.operation.FieldNameBytes(previous.fieldRef), fieldName) ||
				!f.operation.ArgumentSetsAreEqualUnordered(f.operation.FieldArguments(previous.fieldRef), f.operation.FieldArguments(ref))
			// Preserve the existing leaf-field diagnostic when both identity and type differ.
			if identityConflict && !bothComposite {
				f.StopWithExternalErr(operationreport.ErrDifferingFieldsOnPotentiallySameType(current.responseName))
				return
			}
		}
		// Intentional deviation: SameResponseShape step 3 requires matching
		// nullability even across mutually exclusive branches. The existing opt-in
		// relaxation applies only when the immediate enclosing types cannot overlap.
		// See https://spec.graphql.org/September2025/#SameResponseShape()
		ignoreNullability := f.relaxNullabilityCheck && !f.potentiallySameObject(previous.enclosingTypeDefinition, current.enclosingTypeDefinition)
		if !f.responseShapesAreCompatible(previous, &current, ignoreNullability) {
			left, err := f.fieldTypeBytes(previous.fieldTypeRef)
			if err != nil {
				f.StopWithInternalErr(err)
				return
			}
			right, err := f.fieldTypeBytes(current.fieldTypeRef)
			if err != nil {
				f.StopWithInternalErr(err)
				return
			}
			if bothComposite {
				f.StopWithExternalErr(operationreport.ErrTypesForFieldMismatch(current.responseName, left, right))
			} else {
				f.StopWithExternalErr(operationreport.ErrFieldsConflict(current.responseName, left, right))
			}
			return
		}
		if identityConflict {
			f.StopWithExternalErr(operationreport.ErrDifferingFieldsOnPotentiallySameType(current.responseName))
			return
		}
	}
	// FieldsInSetCanMerge step 2 checks every pair. Retain every selection;
	// compatibility with one field does not establish compatibility with others.
	f.requirements = append(f.requirements, current)
}

func (f *fieldSelectionMergingVisitor) responseShapesAreCompatible(left, right *fieldRequirement, ignoreNullability bool) bool {
	if left.fieldTypeRef != ast.InvalidRef && right.fieldTypeRef != ast.InvalidRef {
		return f.definition.TypesHaveCompatibleResponseShape(left.fieldTypeRef, right.fieldTypeRef, left.fieldTypeDefinitionNode, right.fieldTypeDefinitionNode, ignoreNullability)
	}
	if left.fieldTypeRef == right.fieldTypeRef {
		return true // Both fields are __typename.
	}
	other := left.fieldTypeRef
	if other == ast.InvalidRef {
		other = right.fieldTypeRef
	}
	if f.definition.Types[other].TypeKind == ast.TypeKindNonNull {
		other = f.definition.Types[other].OfType
	} else if !ignoreNullability {
		return false
	}
	return f.definition.Types[other].TypeKind == ast.TypeKindNamed && bytes.Equal(f.definition.TypeNameBytes(other), literal.STRING)
}

func (f *fieldSelectionMergingVisitor) fieldTypeBytes(ref int) ([]byte, error) {
	if ref == ast.InvalidRef {
		return []byte("String!"), nil
	}
	return f.definition.PrintTypeBytes(ref, nil)
}

// parentRequirement uses the walker's ancestry because SkipNode can suppress
// LeaveField callbacks in other visitors.
func (f *fieldSelectionMergingVisitor) parentRequirement() int {
	for _, ancestor := range slices.Backward(f.Ancestors) {
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		for j := range slices.Backward(f.requirements) {
			if f.requirements[j].fieldRef == ancestor.Ref {
				return j
			}
		}
		break
	}
	return -1
}

// fieldsAreMutuallyExclusive includes ancestors because FieldsInSetCanMerge
// step 2.2 skips recursive identity checks across distinct object branches.
// It also preserves the interface/object exception documented on FieldSelectionMerging.
// SameResponseShape still checks their descendants' response shapes.
func (f *fieldSelectionMergingVisitor) fieldsAreMutuallyExclusive(left, right *fieldRequirement) bool {
	for {
		leftType, rightType := left.enclosingTypeDefinition, right.enclosingTypeDefinition
		switch {
		case leftType.Kind == ast.NodeKindObjectTypeDefinition && rightType.Kind == ast.NodeKindObjectTypeDefinition:
			if leftType != rightType {
				return true
			}
		case leftType.Kind == ast.NodeKindInterfaceTypeDefinition && rightType.Kind == ast.NodeKindObjectTypeDefinition,
			leftType.Kind == ast.NodeKindObjectTypeDefinition && rightType.Kind == ast.NodeKindInterfaceTypeDefinition:
			if !f.potentiallySameObject(leftType, rightType) {
				return true
			}
		}
		if left.parent == -1 || right.parent == -1 {
			return false
		}
		left, right = &f.requirements[left.parent], &f.requirements[right.parent]
	}
}

func (f *fieldSelectionMergingVisitor) nodeIsCompositeType(node ast.Node) bool {
	switch node.Kind {
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
		return true
	default:
		return false
	}
}

// potentiallySameObject reports whether two enclosing type definitions could apply
// to the same runtime object. This determines whether field merging must enforce
// matching nullability or may apply the optional nullability relaxation.
//
//   - If one type is an interface and the other is an object, returns true only
//     when the object type implements the interface (otherwise they cannot overlap).
//   - If both types are interfaces, returns true (conservative: some concrete
//     type might implement both).
//   - Two object types overlap only when they share the same name.
//   - All other combinations return false.
func (f *fieldSelectionMergingVisitor) potentiallySameObject(left, right ast.Node) bool {
	switch {
	case left.Kind == ast.NodeKindInterfaceTypeDefinition && right.Kind == ast.NodeKindInterfaceTypeDefinition:
		return true
	case left.Kind == ast.NodeKindInterfaceTypeDefinition && right.Kind == ast.NodeKindObjectTypeDefinition:
		return f.definition.NodeImplementsInterfaceNode(right, left)
	case left.Kind == ast.NodeKindObjectTypeDefinition && right.Kind == ast.NodeKindInterfaceTypeDefinition:
		return f.definition.NodeImplementsInterfaceNode(left, right)
	case left.Kind == ast.NodeKindObjectTypeDefinition && right.Kind == ast.NodeKindObjectTypeDefinition:
		return bytes.Equal(f.definition.ObjectTypeDefinitionNameBytes(left.Ref), f.definition.ObjectTypeDefinitionNameBytes(right.Ref))
	default:
		return false
	}
}
