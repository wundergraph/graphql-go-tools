package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// FieldSelectionMergingConfig controls which spec-deviation relaxations are active.
//
// Note: RelaxTypeMismatchCheck is strictly broader than RelaxNullabilityCheck —
// when RelaxTypeMismatchCheck is true on non-overlapping types, all type differences
// (including nullability) are allowed, making RelaxNullabilityCheck redundant for
// those cases. Both flags exist because callers may want the narrower nullability-only
// relaxation without the broader type mismatch relaxation.
type FieldSelectionMergingConfig struct {
	// RelaxNullabilityCheck allows differing nullability (e.g. String! vs String)
	// on fields in non-overlapping concrete object types.
	RelaxNullabilityCheck bool
	// RelaxTypeMismatchCheck allows completely different types (e.g. IssueState vs
	// PullRequestReviewState) on fields in non-overlapping concrete object types.
	RelaxTypeMismatchCheck bool
}

// FieldSelectionMerging validates if field selections can be merged
func FieldSelectionMerging(config ...FieldSelectionMergingConfig) Rule {
	var cfg FieldSelectionMergingConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	return func(walker *astvisitor.Walker) {
		visitor := fieldSelectionMergingVisitor{
			Walker:                 walker,
			relaxNullabilityCheck:  cfg.RelaxNullabilityCheck,
			relaxTypeMismatchCheck: cfg.RelaxTypeMismatchCheck,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterFieldVisitor(&visitor)
		walker.RegisterEnterOperationVisitor(&visitor)
		walker.RegisterEnterFragmentDefinitionVisitor(&visitor)
	}
}

type fieldSelectionMergingVisitor struct {
	*astvisitor.Walker

	definition, operation  *ast.Document
	scalarRequirements     scalarRequirements
	nonScalarRequirements  nonScalarRequirements
	refs                   []int
	relaxNullabilityCheck  bool
	relaxTypeMismatchCheck bool
}
type nonScalarRequirement struct {
	path                    ast.Path
	objectName              ast.ByteSlice
	fieldTypeRef            int
	fieldTypeDefinitionNode ast.Node
	enclosingTypeDefinition ast.Node
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
		fieldDefinitionTypeKindPresentInRequirements := false
		for _, i := range matchedRequirements {

			if !f.potentiallySameObject(fieldDefinitionTypeNode, f.nonScalarRequirements[i].fieldTypeDefinitionNode) {
				if !objectName.Equals(f.nonScalarRequirements[i].objectName) {
					f.StopWithExternalErr(operationreport.ErrResponseOfDifferingTypesMustBeOfSameShape(objectName, f.nonScalarRequirements[i].objectName))
					return
				}
			} else if !f.definition.TypesAreCompatibleDeep(f.nonScalarRequirements[i].fieldTypeRef, fieldType) {
				switch f.checkTypeMismatch(f.nonScalarRequirements[i].enclosingTypeDefinition, f.nonScalarRequirements[i].fieldTypeRef, fieldType) {
				case typeMismatchSkip:
					continue
				case typeMismatchReject:
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
				fieldDefinitionTypeKindPresentInRequirements = true
			}
		}

		if len(matchedRequirements) != 0 && fieldDefinitionTypeKindPresentInRequirements {
			return
		}

		f.nonScalarRequirements = append(f.nonScalarRequirements, nonScalarRequirement{
			path:                    path,
			objectName:              objectName,
			fieldTypeRef:            fieldType,
			fieldTypeDefinitionNode: fieldDefinitionTypeNode,
			enclosingTypeDefinition: f.EnclosingTypeDefinition,
		})
		return
	}

	matchedRequirements := f.ScalarRequirementsByPathField(path, objectName)
	fieldDefinitionTypeKindPresentInRequirements := false

	for _, i := range matchedRequirements {
		if f.potentiallySameObject(f.scalarRequirements[i].enclosingTypeDefinition, f.EnclosingTypeDefinition) {
			// here we do not check directives equality, only if the stream directives are the same for the fields
			if !f.operation.FieldsAreEqualFlat(f.scalarRequirements[i].fieldRef, ref, false) {
				f.StopWithExternalErr(operationreport.ErrDifferingFieldsOnPotentiallySameType(objectName))
				return
			}
		}
		if !f.definition.TypesAreCompatibleDeep(f.scalarRequirements[i].fieldType, fieldType) {
			switch f.checkTypeMismatch(f.scalarRequirements[i].enclosingTypeDefinition, f.scalarRequirements[i].fieldType, fieldType) {
			case typeMismatchSkip:
				continue
			case typeMismatchReject:
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
			fieldDefinitionTypeKindPresentInRequirements = true
		}
	}

	if len(matchedRequirements) != 0 && fieldDefinitionTypeKindPresentInRequirements {
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

type typeMismatchResult int

const (
	// typeMismatchAccept means the types differ only in nullability and the
	// relaxation flag allowed it — no error, but keep processing this requirement.
	typeMismatchAccept typeMismatchResult = iota
	// typeMismatchSkip means the enclosing types are provably disjoint and
	// the relaxation flag allowed the full type mismatch — skip this requirement.
	typeMismatchSkip
	// typeMismatchReject means the type difference must be reported as an error.
	typeMismatchReject
)

// checkTypeMismatch decides how to handle a type incompatibility between an
// existing requirement's field type and the current field's type. It applies
// type mismatch relaxation first (broader), then nullability relaxation (narrower).
//
// Examples:
//   - typeMismatchAccept: User.email is String!, Organization.email is String
//     → types differ only in nullability, RelaxNullabilityCheck allows it
//   - typeMismatchSkip: Issue.state is IssueState, PullRequestReview.state is PullRequestReviewState
//     → enclosing types are disjoint concrete objects, RelaxTypeMismatchCheck skips the check entirely
//   - typeMismatchReject: NonNullStringBox1.scalar is String!, IntBox.scalar is Int
//     → one enclosing type is an interface (could overlap), difference must be reported as error
func (f *fieldSelectionMergingVisitor) checkTypeMismatch(existingEnclosing ast.Node, existingFieldType, currentFieldType int) typeMismatchResult {
	sameObject := f.potentiallySameObject(existingEnclosing, f.EnclosingTypeDefinition)
	// Type mismatch relaxation (spec sec 5.3.2, SameResponseShape): when enclosing
	// types are provably non-overlapping, any type difference is safe.
	if f.relaxTypeMismatchCheck && !sameObject {
		return typeMismatchSkip
	}
	// Nullability relaxation (spec sec 5.3.2, SameResponseShape): when enclosing
	// types cannot overlap at runtime, we allow nullability differences.
	if f.relaxNullabilityCheck && !sameObject &&
		f.definition.TypesAreCompatibleIgnoringNullability(existingFieldType, currentFieldType) {
		return typeMismatchAccept
	}
	return typeMismatchReject
}

// potentiallySameObject reports whether two enclosing type definitions could apply
// to the same runtime object. This determines whether field merging must enforce
// strict type equality (including nullability) or may relax it.
//
//   - If either type is an interface, returns true (conservative: any concrete
//     type might implement that interface).
//   - Two object types overlap only when they share the same name.
//   - All other combinations return false.
//
// Union types are not handled explicitly because the AST walker resolves inline
// fragment type conditions to their concrete member types before visiting fields;
// the enclosing type definition seen here is always the concrete type from the
// inline fragment, never the union itself.
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

func (f *fieldSelectionMergingVisitor) EnterSelectionSet(_ int) {

}
