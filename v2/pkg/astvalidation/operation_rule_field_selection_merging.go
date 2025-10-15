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
func FieldSelectionMerging() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := fieldSelectionMergingVisitor{Walker: walker}
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
}
type nonScalarRequirement struct {
	path                    ast.Path
	objectName              ast.ByteSlice
	fieldRef                int
	fieldTypeRef            int
	fieldTypeDefinitionNode ast.Node
}

type nonScalarRequirements []nonScalarRequirement

func (f *fieldSelectionMergingVisitor) NonScalarRequirementsByPathField(path ast.Path, objectName ast.ByteSlice) []int {
	f.refs = f.refs[:0]
	for i := range f.nonScalarRequirements {
		if f.nonScalarRequirements[i].path.Equals(path) && f.nonScalarRequirements[i].objectName.Equals(objectName) {
			f.refs = append(f.refs, i)
		}
	}
	return f.refs
}

type scalarRequirement struct {
	path                    ast.Path
	objectName              ast.ByteSlice
	fieldRef                int
	fieldType               int
	enclosingTypeDefinition ast.Node
	fieldTypeDefinitionNode ast.Node
}

type scalarRequirements []scalarRequirement

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
}

func (f *fieldSelectionMergingVisitor) EnterFragmentDefinition(_ int) {
	f.resetRequirements()
}

func (f *fieldSelectionMergingVisitor) EnterOperationDefinition(_ int) {
	f.resetRequirements()
}

func (f *fieldSelectionMergingVisitor) EnterField(ref int) {

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
	if fieldDefinitionTypeNode.Kind != ast.NodeKindScalarTypeDefinition {

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

				if !f.definition.TypesAreCompatibleDeep(f.nonScalarRequirements[i].fieldTypeRef, fieldType) {
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

			if fieldDefinitionTypeNode.Kind != f.nonScalarRequirements[i].fieldTypeDefinitionNode.Kind {
				hasDifferentKindInRequirements = true
			}
		}

		if hasDifferentKindInRequirements {
			// If we've already checked this field against a requirement with a different Kind,
			// we don't need to add it again to requirements.
			return
		}

		f.nonScalarRequirements = append(f.nonScalarRequirements, nonScalarRequirement{
			path:                    path,
			objectName:              objectName,
			fieldRef:                ref,
			fieldTypeRef:            fieldType,
			fieldTypeDefinitionNode: fieldDefinitionTypeNode,
		})
		return
	}

	matchedRequirements := f.ScalarRequirementsByPathField(path, objectName)
	hasDifferentKindInRequirements := false

	for _, i := range matchedRequirements {
		if f.potentiallySameObject(f.scalarRequirements[i].enclosingTypeDefinition, f.EnclosingTypeDefinition) {
			// here we do not check directives equality, only if the stream directives are the same for the fields
			if !f.operation.FieldsAreEqualFlat(f.scalarRequirements[i].fieldRef, ref, false) {
				f.StopWithExternalErr(operationreport.ErrDifferingFieldsOnPotentiallySameType(objectName))
				return
			}
		}
		if !f.definition.TypesAreCompatibleDeep(f.scalarRequirements[i].fieldType, fieldType) {
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

func (f *fieldSelectionMergingVisitor) potentiallySameObject(left, right ast.Node) bool {
	switch {
	case left.Kind == ast.NodeKindInterfaceTypeDefinition || right.Kind == ast.NodeKindInterfaceTypeDefinition:
		return true
	case left.Kind == ast.NodeKindObjectTypeDefinition && right.Kind == ast.NodeKindObjectTypeDefinition:
		return bytes.Equal(f.definition.ObjectTypeDefinitionNameBytes(left.Ref), f.definition.ObjectTypeDefinitionNameBytes(right.Ref))
	default:
		return false
	}
}
