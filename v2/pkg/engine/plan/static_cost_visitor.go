package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// StaticCostVisitor builds the cost tree during AST traversal.
// It is registered on the same walker as the planning Visitor and uses
// data from the planning visitor (fieldPlanners, planners) to determine
// which data sources are responsible for each field.
type StaticCostVisitor struct {
	Walker *astvisitor.Walker

	// AST documents - set before walking
	Operation  *ast.Document
	Definition *ast.Document

	// References to planning visitor data - set before walking
	planners      []PlannerConfiguration
	fieldPlanners *map[int][]int // Pointer to Visitor.fieldPlanners

	// Pointer to the main visitor's operationDefinition (set during EnterDocument)
	operationDefinition *int

	// stack to keep track of the current node
	stack []*CostTreeNode

	// The final cost tree that is built during plan traversal.
	tree *CostTreeNode
}

// NewStaticCostVisitor creates a new cost tree visitor
func NewStaticCostVisitor(walker *astvisitor.Walker, operation, definition *ast.Document) *StaticCostVisitor {
	stack := make([]*CostTreeNode, 0, 16)
	rootNode := CostTreeNode{
		fieldCoord: FieldCoordinate{"_none", "_root"},
		multiplier: 1,
	}
	stack = append(stack, &rootNode)
	return &StaticCostVisitor{
		Walker:     walker,
		Operation:  operation,
		Definition: definition,
		stack:      stack,
		tree:       &rootNode,
	}
}

// EnterField creates a partial cost node when entering a field.
// The node is filled in full in the LeaveField when fieldPlanners data is available.
func (v *StaticCostVisitor) EnterField(fieldRef int) {
	typeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldName := v.Operation.FieldNameUnsafeString(fieldRef)

	fieldDefinitionRef, ok := v.Walker.FieldDefinition(fieldRef)
	if !ok {
		// Push the sentinel node, so the LeaveField would pop the stack correctly.
		v.stack = append(v.stack, &CostTreeNode{fieldRef: fieldRef})
		return
	}
	fieldDefinitionTypeRef := v.Definition.FieldDefinitionType(fieldDefinitionRef)
	isListType := v.Definition.TypeIsList(fieldDefinitionTypeRef)
	isSimpleType := v.Definition.TypeIsEnum(fieldDefinitionTypeRef, v.Definition) || v.Definition.TypeIsScalar(fieldDefinitionTypeRef, v.Definition)
	unwrappedTypeName := v.Definition.ResolveTypeNameString(fieldDefinitionTypeRef)

	arguments := v.extractFieldArguments(fieldRef)

	// Check and push through if the unwrapped type of this field is interface or union.
	unwrappedTypeNode, exists := v.Definition.NodeByNameStr(unwrappedTypeName)
	var implementingTypeNames []string
	var isAbstractType bool
	if exists {
		if unwrappedTypeNode.Kind == ast.NodeKindInterfaceTypeDefinition {
			impl, ok := v.Definition.InterfaceTypeDefinitionImplementedByObjectWithNames(unwrappedTypeNode.Ref)
			if ok {
				implementingTypeNames = append(implementingTypeNames, impl...)
				isAbstractType = true
			}
		}
		if unwrappedTypeNode.Kind == ast.NodeKindUnionTypeDefinition {
			impl, ok := v.Definition.UnionTypeDefinitionMemberTypeNames(unwrappedTypeNode.Ref)
			if ok {
				implementingTypeNames = append(implementingTypeNames, impl...)
				isAbstractType = true
			}
		}
	}

	isEnclosingTypeAbstract := v.Walker.EnclosingTypeDefinition.Kind.IsAbstractType()
	// Create a skeleton node. dataSourceHashes will be filled in leaveFieldCost
	node := CostTreeNode{
		fieldRef:                fieldRef,
		fieldCoord:              FieldCoordinate{typeName, fieldName},
		multiplier:              1,
		fieldTypeName:           unwrappedTypeName,
		implementingTypeNames:   implementingTypeNames,
		returnsListType:         isListType,
		returnsSimpleType:       isSimpleType,
		returnsAbstractType:     isAbstractType,
		isEnclosingTypeAbstract: isEnclosingTypeAbstract,
		arguments:               arguments,
	}

	// Attach to parent
	if len(v.stack) > 0 {
		parent := v.stack[len(v.stack)-1]
		parent.children = append(parent.children, &node)
	}

	v.stack = append(v.stack, &node)
}

// LeaveField fills DataSource hashes for the current node and pop it from the cost stack.
func (v *StaticCostVisitor) LeaveField(fieldRef int) {
	dsHashes := v.getFieldDataSourceHashes(fieldRef)

	if len(v.stack) <= 1 { // Keep root on stack
		return
	}

	// Find the current node (should match fieldRef)
	lastIndex := len(v.stack) - 1
	current := v.stack[lastIndex]
	if current.fieldRef != fieldRef {
		return
	}

	current.dataSourceHashes = dsHashes
	current.parent = v.stack[lastIndex-1]

	v.stack = v.stack[:lastIndex]
}

// getFieldDataSourceHashes returns all data source hashes for the field.
// A field can be planned on multiple data sources in federation scenarios.
func (v *StaticCostVisitor) getFieldDataSourceHashes(fieldRef int) []DSHash {
	plannerIDs, ok := (*v.fieldPlanners)[fieldRef]
	if !ok || len(plannerIDs) == 0 {
		return nil
	}

	dsHashes := make([]DSHash, 0, len(plannerIDs))
	for _, plannerID := range plannerIDs {
		if plannerID >= 0 && plannerID < len(v.planners) {
			dsHash := v.planners[plannerID].DataSourceConfiguration().Hash()
			dsHashes = append(dsHashes, dsHash)
		}
	}
	return dsHashes
}

// extractFieldArguments extracts arguments from a field for cost calculation
// This implementation does not go deep for input objects yet.
// It should return unwrapped type names for arguments and that is it for now.
func (v *StaticCostVisitor) extractFieldArguments(fieldRef int) map[string]ArgumentInfo {
	argRefs := v.Operation.FieldArguments(fieldRef)
	if len(argRefs) == 0 {
		return nil
	}

	arguments := make(map[string]ArgumentInfo, len(argRefs))
	for _, argRef := range argRefs {
		argName := v.Operation.ArgumentNameString(argRef)
		argValue := v.Operation.ArgumentValue(argRef)
		argInfo := ArgumentInfo{}

		switch argValue.Kind {
		case ast.ValueKindVariable:
			variableValue := v.Operation.VariableValueNameString(argValue.Ref)
			if !v.Operation.OperationDefinitionHasVariableDefinition(*v.operationDefinition, variableValue) {
				continue // omit optional argument when the variable is not defined
			}

			// We cannot read values of variables from the context here. Save it for later.
			argInfo.hasVariable = true
			argInfo.varName = variableValue

			variableDefinition, exists := v.Operation.VariableDefinitionByNameAndOperation(*v.operationDefinition, v.Operation.VariableValueNameBytes(argValue.Ref))
			if !exists {
				continue
			}
			variableTypeRef := v.Operation.VariableDefinitions[variableDefinition].Type
			unwrappedVarTypeRef := v.Operation.ResolveUnderlyingType(variableTypeRef)
			argInfo.typeName = v.Operation.TypeNameString(unwrappedVarTypeRef)
			node, exists := v.Definition.NodeByNameStr(argInfo.typeName)
			if !exists {
				continue
			}

			// fmt.Printf("variableTypeRef = %v unwrappedVarTypeRef = %v typeName = %v nodeKind = %v varVal = %v\n", variableTypeRef, unwrappedVarTypeRef, argInfo.typeName, node.Kind, variableValue)

			// Analyze the node to see what kind of variable was passed.
			switch node.Kind {
			case ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
				argInfo.isSimple = true
			case ast.NodeKindInputObjectTypeDefinition:
				argInfo.isInputObject = true

			}

			// TODO: we need to analyze variables that contains input object fields.
			// If these fields has weight attached, use them for calculation.
			// Inline values extracted into variables here, so we need to inspect them via AST.
		}

		arguments[argName] = argInfo
	}

	return arguments
}

func (v *StaticCostVisitor) finalCostTree() *CostTreeNode {
	return v.tree
}
