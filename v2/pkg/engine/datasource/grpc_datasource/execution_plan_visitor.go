package grpcdatasource

import (
	"errors"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type planningInfo struct {
	operationType      ast.OperationType
	operationFieldName string

	requestMessageAncestors []*RPCMessage
	currentRequestMessage   *RPCMessage

	responseMessageAncestors  []*RPCMessage
	currentResponseMessage    *RPCMessage
	currentResponseFieldIndex int

	responseFieldIndexAncestors []int
}

type rpcPlanVisitor struct {
	walker     *astvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
	planCtx    *rpcPlanningContext
	planInfo   planningInfo

	subgraphName       string
	mapping            *GRPCMapping
	plan               *RPCExecutionPlan
	operationFieldRef  int
	operationFieldRefs []int
	currentCall        *RPCCall
	currentCallID      int
}

type rpcPlanVisitorConfig struct {
	subgraphName      string
	mapping           *GRPCMapping
	federationConfigs plan.FederationFieldConfigurations
}

// newRPCPlanVisitor creates a new RPCPlanVisitor.
// It registers the visitor with the walker and returns it.
func newRPCPlanVisitor(config rpcPlanVisitorConfig) *rpcPlanVisitor {
	walker := astvisitor.NewWalker(48)
	visitor := &rpcPlanVisitor{
		walker:            &walker,
		plan:              &RPCExecutionPlan{},
		subgraphName:      cases.Title(language.Und, cases.NoLower).String(config.subgraphName),
		mapping:           config.mapping,
		operationFieldRef: -1,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)

	return visitor
}

func (r *rpcPlanVisitor) PlanOperation(operation, definition *ast.Document) (*RPCExecutionPlan, error) {
	report := &operationreport.Report{}
	r.walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil, fmt.Errorf("unable to plan operation: %w", report)
	}

	return r.plan, nil
}

// EnterDocument implements astvisitor.EnterDocumentVisitor.
func (r *rpcPlanVisitor) EnterDocument(operation *ast.Document, definition *ast.Document) {
	r.definition = definition
	r.operation = operation

	r.planCtx = newRPCPlanningContext(operation, definition, r.mapping)
}

// EnterOperationDefinition implements astvisitor.EnterOperationDefinitionVisitor.
// This is called when entering the operation definition node.
// It retrieves information about the operation
// and creates a new group in the plan.
func (r *rpcPlanVisitor) EnterOperationDefinition(ref int) {
	// Retrieves the fields from the root selection set.
	// These fields determine the names for the RPC functions to call.
	// TODO: handle fragments on root level `... on Query {}`
	selectionSetRef := r.operation.OperationDefinitions[ref].SelectionSet
	r.operationFieldRefs = r.operation.SelectionSetFieldSelections(selectionSetRef)

	r.plan.Calls = make([]RPCCall, len(r.operationFieldRefs))
	r.planInfo.operationType = r.operation.OperationDefinitions[ref].OperationType
}

// EnterArgument implements astvisitor.EnterArgumentVisitor.
// This method retrieves the input value definition for the argument
// and builds the request message from the input argument.
//
// TODO handle field arguments to define resolvers
func (r *rpcPlanVisitor) EnterArgument(ref int) {
	a := r.walker.Ancestor()
	if a.Kind != ast.NodeKindField && a.Ref != r.operationFieldRef {
		return
	}
	argumentInputValueDefinitionRef, exists := r.walker.ArgumentInputValueDefinition(ref)
	if !exists {
		return
	}

	// Retrieve the type of the input value definition, and build the request message
	inputValueDefinitionTypeRef := r.definition.InputValueDefinitionType(argumentInputValueDefinitionRef)
	r.enrichRequestMessageFromInputArgument(ref, inputValueDefinitionTypeRef)
}

// EnterSelectionSet implements astvisitor.EnterSelectionSetVisitor.
// Checks if this is in the root level below the operation definition.
func (r *rpcPlanVisitor) EnterSelectionSet(ref int) {
	if r.walker.Ancestor().Kind == ast.NodeKindOperationDefinition {
		return
	}

	if len(r.planInfo.currentResponseMessage.Fields) == 0 || len(r.planInfo.currentResponseMessage.Fields) <= r.planInfo.currentResponseFieldIndex {
		return
	}

	// In nested selection sets, a new message needs to be created, which will be added to the current response message.
	if r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message == nil {
		r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message = r.planCtx.newMessageFromSelectionSet(r.walker.EnclosingTypeDefinition, ref)
	}

	// Add the current response message to the ancestors and set the current response message to the current field message
	r.planInfo.responseMessageAncestors = append(r.planInfo.responseMessageAncestors, r.planInfo.currentResponseMessage)
	r.planInfo.currentResponseMessage = r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message

	// Check if the ancestor type is a composite type (interface or union)
	// and set the oneof type and member types.
	if err := r.handleCompositeType(r.walker.Ancestor()); err != nil {
		// If the ancestor is a composite type, but we were unable to resolve the member types,
		// we stop the walker and return an internal error.
		r.walker.StopWithInternalErr(err)
		return
	}

	// Keep track of the field indices for the current response message.
	// This is used to set the correct field index for the current response message
	// when leaving the selection set.
	r.planInfo.responseFieldIndexAncestors = append(r.planInfo.responseFieldIndexAncestors, r.planInfo.currentResponseFieldIndex)

	r.planInfo.currentResponseFieldIndex = 0 // reset the field index for the current selection set
}

func (r *rpcPlanVisitor) handleCompositeType(node ast.Node) error {
	if node.Ref == ast.InvalidRef {
		return nil
	}

	var (
		ok          bool
		oneOfType   OneOfType
		memberTypes []string
	)

	switch node.Kind {
	case ast.NodeKindField:
		return r.handleCompositeType(r.walker.EnclosingTypeDefinition)
	case ast.NodeKindInterfaceTypeDefinition:
		oneOfType = OneOfTypeInterface
		memberTypes, ok = r.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
		if !ok {
			return fmt.Errorf("interface type %s is not implemented by any object", r.definition.InterfaceTypeDefinitionNameString(node.Ref))
		}
	case ast.NodeKindUnionTypeDefinition:
		oneOfType = OneOfTypeUnion
		memberTypes, ok = r.definition.UnionTypeDefinitionMemberTypeNames(node.Ref)
		if !ok {
			return fmt.Errorf("union type %s is not defined", r.definition.UnionTypeDefinitionNameString(node.Ref))
		}
	default:
		return nil
	}

	r.planInfo.currentResponseMessage.OneOfType = oneOfType
	r.planInfo.currentResponseMessage.MemberTypes = memberTypes

	return nil
}

// LeaveSelectionSet implements astvisitor.SelectionSetVisitor.
// It updates the current response field index and response message ancestors.
// If the ancestor is an operation definition, it adds the current call to the group.
func (r *rpcPlanVisitor) LeaveSelectionSet(ref int) {
	if r.walker.Ancestor().Kind == ast.NodeKindInlineFragment {
		return
	}

	if len(r.planInfo.responseFieldIndexAncestors) > 0 {
		r.planInfo.currentResponseFieldIndex = r.planInfo.responseFieldIndexAncestors[len(r.planInfo.responseFieldIndexAncestors)-1]
		r.planInfo.responseFieldIndexAncestors = r.planInfo.responseFieldIndexAncestors[:len(r.planInfo.responseFieldIndexAncestors)-1]
	}

	if len(r.planInfo.responseMessageAncestors) > 0 {
		r.planInfo.currentResponseMessage = r.planInfo.responseMessageAncestors[len(r.planInfo.responseMessageAncestors)-1]
		r.planInfo.responseMessageAncestors = r.planInfo.responseMessageAncestors[:len(r.planInfo.responseMessageAncestors)-1]
	}
}

func (r *rpcPlanVisitor) handleRootField(ref int) error {
	r.operationFieldRef = ref
	r.planInfo.operationFieldName = r.operation.FieldNameString(ref)

	r.currentCall = &RPCCall{
		ServiceName: r.planCtx.resolveServiceName(r.subgraphName),
	}

	r.planInfo.currentRequestMessage = &r.currentCall.Request
	r.planInfo.currentResponseMessage = &r.currentCall.Response

	// attempt to resolve the name from the mapping
	rpcConfig, err := r.planCtx.resolveRPCMethodMapping(r.planInfo.operationType, r.planInfo.operationFieldName)
	if err != nil {
		return err
	}

	r.currentCall.MethodName = rpcConfig.RPC
	r.currentCall.Request.Name = rpcConfig.Request
	r.currentCall.Response.Name = rpcConfig.Response

	return nil
}

// EnterField implements astvisitor.EnterFieldVisitor.
func (r *rpcPlanVisitor) EnterField(ref int) {
	fieldName := r.operation.FieldNameString(ref)
	if r.walker.InRootField() {
		if err := r.handleRootField(ref); err != nil {
			r.walker.StopWithInternalErr(err)
			return
		}
	}

	if fieldName == "_entities" {
		r.walker.StopWithInternalErr(errors.New("entities field is not supported in this visitor"))
		return
	}

	// prevent duplicate fields
	fieldAlias := r.operation.FieldAliasString(ref)
	if r.planInfo.currentResponseMessage.Fields.Exists(fieldName, fieldAlias) {
		return
	}

	fd, ok := r.walker.FieldDefinition(ref)
	if !ok {
		r.walker.Report.AddExternalError(operationreport.ExternalError{
			Message: fmt.Sprintf("Field %s not found in definition %s", r.operation.FieldNameString(ref), r.walker.EnclosingTypeDefinition.NameString(r.definition)),
		})
		return
	}

	field, err := r.planCtx.buildField(r.walker.EnclosingTypeDefinition, fd, fieldName, fieldAlias)
	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	// check if we are inside of an inline fragment
	if ref, ok := r.walker.ResolveInlineFragment(); ok {
		if r.planInfo.currentResponseMessage.FieldSelectionSet == nil {
			r.planInfo.currentResponseMessage.FieldSelectionSet = make(RPCFieldSelectionSet)
		}

		inlineFragmentName := r.operation.InlineFragmentTypeConditionNameString(ref)
		r.planInfo.currentResponseMessage.FieldSelectionSet.Add(inlineFragmentName, field)
		return
	}

	r.planInfo.currentResponseMessage.Fields = append(r.planInfo.currentResponseMessage.Fields, field)
}

// LeaveField implements astvisitor.FieldVisitor.
func (r *rpcPlanVisitor) LeaveField(ref int) {
	// If we are not in the operation field, we can increment the response field index.
	if !r.walker.InRootField() {
		r.planInfo.currentResponseFieldIndex++
		return
	}

	r.plan.Calls[r.currentCallID] = *r.currentCall
	r.currentCall = &RPCCall{}

	r.currentCallID++
	if r.currentCallID < len(r.operationFieldRefs) {
		r.operationFieldRef = r.operationFieldRefs[r.currentCallID]
	}

	r.planInfo.currentResponseFieldIndex = 0
}

// enrichRequestMessageFromInputArgument constructs a request message from an input argument based on its type.
// It retrieves the underlying type and builds the request message from the underlying type.
// If the underlying type is an input object type, it creates a new message and adds it to the current request message.
// Otherwise, it adds the field to the current request message.
func (r *rpcPlanVisitor) enrichRequestMessageFromInputArgument(argRef, typeRef int) {
	underlyingTypeName := r.definition.ResolveTypeNameString(typeRef)
	underlyingTypeNode, found := r.definition.NodeByNameStr(underlyingTypeName)
	if !found {
		return
	}

	fieldName := r.operation.ArgumentNameString(argRef)
	jsonPath := fieldName
	argument := r.operation.Arguments[argRef]

	// TODO: We should only work with variables as after normalization we don't have and direct input values.
	// Therefore we should error out when we don't have a variable.
	if argument.Value.Kind == ast.ValueKindVariable {
		jsonPath = r.operation.Input.ByteSliceString(r.operation.VariableValues[argument.Value.Ref].Name)
	}

	rootNode := r.walker.TypeDefinitions[len(r.walker.TypeDefinitions)-2]
	baseType := r.definition.NodeNameString(rootNode)
	mappedInputName := r.resolveInputArgument(baseType, r.walker.Ancestor().Ref, fieldName)

	// If the underlying type is an input object type, create a new message and add it to the current request message.
	switch underlyingTypeNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		msg := &RPCMessage{
			Name:   underlyingTypeName,
			Fields: RPCFields{},
		}

		field := r.buildInputMessageField(typeRef, mappedInputName, jsonPath, DataTypeMessage)
		field.Message = msg
		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, field)

		// Add the current request message to the ancestors and set the current request message to the new message.
		r.planInfo.requestMessageAncestors = append(r.planInfo.requestMessageAncestors, r.planInfo.currentRequestMessage)
		r.planInfo.currentRequestMessage = msg

		r.buildMessageFromNode(underlyingTypeNode)

		r.planInfo.currentRequestMessage = r.planInfo.requestMessageAncestors[len(r.planInfo.requestMessageAncestors)-1]
		r.planInfo.requestMessageAncestors = r.planInfo.requestMessageAncestors[:len(r.planInfo.requestMessageAncestors)-1]

	case ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
		dt := r.planCtx.toDataType(&r.definition.Types[typeRef])

		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields,
			r.buildInputMessageField(typeRef, mappedInputName, jsonPath, dt))
	default:
		// TODO unions, interfaces, etc.
		r.walker.Report.AddInternalError(fmt.Errorf("unsupported type: %s", underlyingTypeNode.Kind))
		r.walker.Stop()
		return
	}
}

// buildMessageFromNode builds a message structure from an AST node.
func (r *rpcPlanVisitor) buildMessageFromNode(node ast.Node) {
	switch node.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		inputObjectDefinition := r.definition.InputObjectTypeDefinitions[node.Ref]
		r.planInfo.currentRequestMessage.Fields = make(RPCFields, 0, len(inputObjectDefinition.InputFieldsDefinition.Refs))

		for _, inputFieldRef := range inputObjectDefinition.InputFieldsDefinition.Refs {
			fieldDefinition := r.definition.InputValueDefinitions[inputFieldRef]
			fieldName := r.definition.Input.ByteSliceString(fieldDefinition.Name)
			r.buildMessageField(fieldName, fieldDefinition.Type, node.Ref)
		}
	}
}

// buildMessageField creates a field in the current request message based on the field type.
func (r *rpcPlanVisitor) buildMessageField(fieldName string, typeRef, parentTypeRef int) {
	inputValueDefinitionType := r.definition.Types[typeRef]
	underlyingTypeName := r.definition.ResolveTypeNameString(typeRef)
	underlyingTypeNode, found := r.definition.NodeByNameStr(underlyingTypeName)
	if !found {
		return
	}

	parentTypeName := r.definition.InputObjectTypeDefinitionNameString(parentTypeRef)
	mappedName := r.resolveFieldMapping(parentTypeName, fieldName)

	// If the type is not an object, directly add the field to the request message
	if underlyingTypeNode.Kind != ast.NodeKindInputObjectTypeDefinition {
		dt := r.planCtx.toDataType(&inputValueDefinitionType)

		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields,
			r.buildInputMessageField(typeRef, mappedName, fieldName, dt))

		return
	}

	msg := &RPCMessage{
		Name: underlyingTypeName,
	}

	field := r.buildInputMessageField(typeRef, mappedName, fieldName, DataTypeMessage)
	field.Message = msg

	r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, field)

	r.planInfo.requestMessageAncestors = append(r.planInfo.requestMessageAncestors, r.planInfo.currentRequestMessage)
	r.planInfo.currentRequestMessage = msg

	r.buildMessageFromNode(underlyingTypeNode)

	r.planInfo.currentRequestMessage = r.planInfo.requestMessageAncestors[len(r.planInfo.requestMessageAncestors)-1]
	r.planInfo.requestMessageAncestors = r.planInfo.requestMessageAncestors[:len(r.planInfo.requestMessageAncestors)-1]
}

func (r *rpcPlanVisitor) buildInputMessageField(typeRef int, fieldName, jsonPath string, dt DataType) RPCField {
	field := RPCField{
		Name:     fieldName,
		Optional: !r.definition.TypeIsNonNull(typeRef),
		TypeName: dt.String(),
		JSONPath: jsonPath,
	}

	if r.definition.TypeIsList(typeRef) {
		switch {
		// for nullable or nested lists we need to build a wrapper message
		// Nullability is handled by the datasource during the execution.
		case r.planCtx.typeIsNullableOrNestedList(typeRef):
			md, err := r.planCtx.createListMetadata(typeRef)
			if err != nil {
				r.walker.StopWithInternalErr(err)
				return field
			}
			field.ListMetadata = md
			field.IsListType = true
		default:
			// For non-nullable single lists we can directly use the repeated syntax in protobuf.
			field.Repeated = true
		}
	}

	if dt == DataTypeEnum {
		field.EnumName = r.definition.ResolveTypeNameString(typeRef)
	}

	return field
}

// This applies both for complex types in the input and for all fields in the response.
func (r *rpcPlanVisitor) resolveFieldMapping(typeName, fieldName string) string {
	grpcFieldName, ok := r.mapping.ResolveFieldMapping(typeName, fieldName)
	if !ok {
		return fieldName
	}

	return grpcFieldName
}

// resolveInputArgument resolves the input argument mapping for a field.
// This only applies if the input arguments are scalar values.
// If the input argument is a message, the mapping is resolved by the
// resolveFieldMapping function.
func (r *rpcPlanVisitor) resolveInputArgument(baseType string, fieldRef int, argumentName string) string {
	grpcFieldName, ok := r.mapping.ResolveFieldArgumentMapping(baseType, r.operation.FieldNameString(fieldRef), argumentName)
	if !ok {
		return argumentName
	}

	return grpcFieldName
}
