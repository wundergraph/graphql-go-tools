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
		walker.RegisterLeaveFieldVisitor(&visitor)
		walker.RegisterEnterOperationVisitor(&visitor)
		walker.RegisterEnterFragmentDefinitionVisitor(&visitor)
	}
}

type fieldSelectionMergingVisitor struct {
	*astvisitor.Walker

	definition, operation *ast.Document
	requirements          []fieldRequirement
	fieldStack            []int
	relaxNullabilityCheck bool
}

type fieldRequirement struct {
	path                    ast.Path
	responseName            ast.ByteSlice
	fieldRef                int
	fieldTypeRef            int
	fieldTypeDefinitionNode ast.Node
	enclosingTypeDefinition ast.Node
	parent                  int
}

func (f *fieldSelectionMergingVisitor) resetRequirements() {
	f.requirements = f.requirements[:0]
	f.fieldStack = f.fieldStack[:0]
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
	parent := -1
	if len(f.fieldStack) > 0 {
		parent = f.fieldStack[len(f.fieldStack)-1]
	}
	f.fieldStack = append(f.fieldStack, -1)
	fieldName := f.operation.FieldNameBytes(ref)
	if bytes.Equal(fieldName, literal.TYPENAME) {
		return
	}
	definition, ok := f.definition.NodeFieldDefinitionByName(f.EnclosingTypeDefinition, fieldName)
	if !ok {
		enclosingTypeName := f.definition.NodeNameBytes(f.EnclosingTypeDefinition)
		f.StopWithExternalErr(operationreport.ErrFieldUndefinedOnType(fieldName, enclosingTypeName))
		return
	}
	current := fieldRequirement{
		path:                    f.Path.WithoutInlineFragmentNames(),
		responseName:            f.operation.FieldAliasOrNameBytes(ref),
		fieldRef:                ref,
		fieldTypeRef:            f.definition.FieldDefinitionType(definition),
		fieldTypeDefinitionNode: f.definition.FieldDefinitionTypeNode(definition),
		enclosingTypeDefinition: f.EnclosingTypeDefinition,
		parent:                  parent,
	}
	for _, previous := range f.requirements {
		if !previous.path.Equals(current.path) || !previous.responseName.Equals(current.responseName) {
			continue
		}
		bothComposite := f.nodeIsCompositeType(previous.fieldTypeDefinitionNode) && f.nodeIsCompositeType(current.fieldTypeDefinitionNode)
		// Field identity depends on the enclosing objects, not on the return types.
		// Exclusivity is inherited by subfields of mutually exclusive selections.
		identityConflict := false
		if !f.fieldsAreMutuallyExclusive(previous, current) {
			compatibleStream := f.operation.DirectiveSetsHasCompatibleStreamDirective(f.operation.FieldDirectives(previous.fieldRef), f.operation.FieldDirectives(ref))
			if bothComposite && !compatibleStream {
				f.StopWithExternalErr(operationreport.ErrConflictingStreamDirectivesOnField(current.responseName))
				return
			}
			identityConflict = !bytes.Equal(f.operation.FieldNameBytes(previous.fieldRef), fieldName) ||
				!f.operation.ArgumentSetsAreEqualUnordered(f.operation.FieldArguments(previous.fieldRef), f.operation.FieldArguments(ref)) || !compatibleStream
			// Preserve the existing leaf-field diagnostic when both identity and type differ.
			if identityConflict && !bothComposite {
				f.StopWithExternalErr(operationreport.ErrDifferingFieldsOnPotentiallySameType(current.responseName))
				return
			}
		}
		// Nullability relaxation is an explicit extension of SameResponseShape.
		// Preserve its existing scope: the immediate enclosing types cannot overlap.
		ignoreNullability := f.relaxNullabilityCheck && !f.potentiallySameObject(previous.enclosingTypeDefinition, current.enclosingTypeDefinition)
		if !f.typesHaveCompatibleResponseShape(previous.fieldTypeRef, current.fieldTypeRef, ignoreNullability) {
			left, err := f.definition.PrintTypeBytes(previous.fieldTypeRef, nil)
			if err != nil {
				f.StopWithInternalErr(err)
				return
			}
			right, err := f.definition.PrintTypeBytes(current.fieldTypeRef, nil)
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
	// Retain every selection: compatibility with one field does not imply
	// compatibility with the other fields at this response path.
	f.fieldStack[len(f.fieldStack)-1] = len(f.requirements)
	f.requirements = append(f.requirements, current)
}

func (f *fieldSelectionMergingVisitor) LeaveField(_ int) {
	f.fieldStack = f.fieldStack[:len(f.fieldStack)-1]
}

func (f *fieldSelectionMergingVisitor) fieldsAreMutuallyExclusive(left, right fieldRequirement) bool {
	for {
		if left.enclosingTypeDefinition.Kind == ast.NodeKindObjectTypeDefinition &&
			right.enclosingTypeDefinition.Kind == ast.NodeKindObjectTypeDefinition &&
			left.enclosingTypeDefinition != right.enclosingTypeDefinition {
			return true
		}
		if left.parent == -1 || right.parent == -1 {
			return false
		}
		left, right = f.requirements[left.parent], f.requirements[right.parent]
	}
}

func (f *fieldSelectionMergingVisitor) typesHaveCompatibleResponseShape(left, right int, ignoreNullability bool) bool {
	for {
		if left == -1 || right == -1 {
			return false
		}
		if ignoreNullability {
			if f.definition.Types[left].TypeKind == ast.TypeKindNonNull {
				left = f.definition.Types[left].OfType
			}
			if f.definition.Types[right].TypeKind == ast.TypeKindNonNull {
				right = f.definition.Types[right].OfType
			}
		}
		if f.definition.Types[left].TypeKind != f.definition.Types[right].TypeKind {
			return false
		}
		if f.definition.Types[left].TypeKind == ast.TypeKindNamed {
			leftName := f.definition.TypeNameBytes(left)
			rightName := f.definition.TypeNameBytes(right)
			if bytes.Equal(leftName, rightName) {
				return true
			}
			leftNode, leftExists := f.definition.Index.FirstNodeByNameBytes(leftName)
			rightNode, rightExists := f.definition.Index.FirstNodeByNameBytes(rightName)
			if !leftExists || !rightExists {
				return false
			}
			return f.nodeIsCompositeType(leftNode) && f.nodeIsCompositeType(rightNode)
		}
		left = f.definition.Types[left].OfType
		right = f.definition.Types[right].OfType
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
