package grpcdatasource

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type keyField struct {
	fieldName string
	fieldType string
}

type entityInfo struct {
	name                    string
	keyFields               []keyField
	keyTypeName             string
	entityRootFieldRef      int
	entityInlineFragmentRef int
}

type planningInfo struct {
	entityInfo entityInfo
	// resolvers      []string
	operationType      ast.OperationType
	operationFieldName string
	isEntityLookup     bool
	methodName         string

	requestMessageAncestors  []*RPCMessage
	currentRequestMessage    *RPCMessage
	currentRequestFieldIndex int

	responseMessageAncestors  []*RPCMessage
	currentResponseMessage    *RPCMessage
	currentResponseFieldIndex int

	// TODO variables

	responseFieldIndexAncestors []int
}

var _ astvisitor.EnterDocumentVisitor = &rpcPlanVisitor{}
var _ astvisitor.FieldVisitor = &rpcPlanVisitor{}
var _ astvisitor.EnterOperationDefinitionVisitor = &rpcPlanVisitor{}
var _ astvisitor.EnterSelectionSetVisitor = &rpcPlanVisitor{}
var _ astvisitor.EnterArgumentVisitor = &rpcPlanVisitor{}

type rpcPlanVisitor struct {
	walker     *astvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
	planInfo   planningInfo

	subgraphName           string
	plan                   *RPCExecutionPlan
	operationDefinitionRef int
	operationFieldRef      int
	currentGroupIndex      int
	currentCall            *RPCCall
	currentCallID          int
}

// newRPCPlanVisitor creates a new RPCPlanVisitor.
// It registers the visitor with the walker and returns it.
func newRPCPlanVisitor(walker *astvisitor.Walker, subgraphName string) *rpcPlanVisitor {
	visitor := &rpcPlanVisitor{
		walker:                 walker,
		plan:                   &RPCExecutionPlan{},
		subgraphName:           strings.Title(subgraphName),
		operationDefinitionRef: -1,
		operationFieldRef:      -1,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterInlineFragmentVisitor(visitor)
	walker.RegisterEnterArgumentVisitor(visitor)

	return visitor
}

// EnterDocument implements astvisitor.EnterDocumentVisitor.
func (r *rpcPlanVisitor) EnterDocument(operation *ast.Document, definition *ast.Document) {
	r.definition = definition
	r.operation = operation
}

// EnterOperationDefinition implements astvisitor.EnterOperationDefinitionVisitor.
// This is called when entering the operation definition node.
// It retrieves information about the operation
// and creates a new group in the plan.
//
// The function also checks if this is an entity lookup operation,
// which requires special handling.
func (r *rpcPlanVisitor) EnterOperationDefinition(ref int) {
	if r.currentCallID < 0 {
		r.currentCallID = 0
	}

	r.plan.Groups = append(r.plan.Groups, RPCCallGroup{
		Calls: []RPCCall{},
	})

	r.currentGroupIndex = len(r.plan.Groups) - 1

	r.operationDefinitionRef = ref

	// Retrieves the fields from the root selection set.
	// These fields determine the names for the RPC functions to call.
	selectionSetRef := r.operation.OperationDefinitions[ref].SelectionSet
	fieldRefs := r.operation.SelectionSetFieldSelections(selectionSetRef)
	operationFieldRef := r.operation.Selections[fieldRefs[r.currentCallID]].Ref

	r.operationFieldRef = operationFieldRef
	r.planInfo.operationFieldName = r.operation.FieldNameString(r.operationFieldRef)

	// _entities is a special field that is used to look up entities
	// Entity lookups are handled differently as we use special types for
	// Providing variables (_Any) and the response type is a Union that needs to be
	// determined from the first inline fragment.
	if r.planInfo.operationFieldName == "_entities" {
		r.planInfo.entityInfo = entityInfo{
			entityRootFieldRef:      -1,
			entityInlineFragmentRef: -1,
		}
		r.planInfo.isEntityLookup = true
	}

	r.planInfo.operationType = r.operation.OperationDefinitions[ref].OperationType
}

// LeaveOperationDefinition implements astvisitor.OperationDefinitionVisitor.
func (r *rpcPlanVisitor) LeaveOperationDefinition(ref int) {
	r.currentCallID++
}

// EnterArgument implements astvisitor.EnterArgumentVisitor.
// This method retrieves the input value definition for the argument
// and builds the request message from the input argument.
//
// TODO handle field arguments to define resolvers
func (r *rpcPlanVisitor) EnterArgument(ref int) {
	if r.planInfo.isEntityLookup {
		return
	}

	a := r.walker.Ancestor()
	if a.Kind != ast.NodeKindField && a.Ref != r.operationFieldRef {
		return
	}

	// Retrieve the input value definition for the argument
	argumentInputValueDefinitionRef, exists := r.walker.ArgumentInputValueDefinition(ref)
	if !exists {
		return
	}

	// Retrieve the type of the input value definition, and build the request message
	inputValueDefinitionTypeRef := r.definition.InputValueDefinitionType(argumentInputValueDefinitionRef)
	r.buildRequestMessageFromInputArgument(ref, inputValueDefinitionTypeRef)
}

// buildRequestMessageFromInputArgument constructs a request message from an input argument based on its type.
// It retrieves the underlying type and builds the request message from the underlying type.
// If the underlying type is an input object type, it creates a new message and adds it to the current request message.
// Otherwise, it adds the field to the current request message.
func (r *rpcPlanVisitor) buildRequestMessageFromInputArgument(argRef, typeRef int) {
	underlyingTypeName := r.definition.ResolveTypeNameString(typeRef)
	underlyingTypeNode, found := r.definition.NodeByNameStr(underlyingTypeName)
	if !found {
		return
	}

	jsonPath := r.operation.ArgumentNameString(argRef)

	argument := r.operation.Arguments[argRef]
	switch argument.Value.Kind {
	case ast.ValueKindVariable:
		jsonPath = r.operation.Input.ByteSliceString(r.operation.VariableValues[argument.Value.Ref].Name)
	}

	// If the underlying type is an input object type, create a new message and add it to the current request message.
	switch underlyingTypeNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		msg := &RPCMessage{
			Name:   underlyingTypeName,
			Fields: RPCFields{},
		}

		// Add a field of type message to the current request message.
		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, RPCField{
			Name:     r.operation.ArgumentNameString(argRef),
			TypeName: DataTypeMessage.String(),
			JSONPath: jsonPath,
			Index:    r.planInfo.currentRequestFieldIndex,
			Message:  msg,
		})

		// Add the current request message to the ancestors and set the current request message to the new message.
		r.planInfo.requestMessageAncestors = append(r.planInfo.requestMessageAncestors, r.planInfo.currentRequestMessage)
		r.planInfo.currentRequestMessage = msg

		r.buildMessageFromNode(underlyingTypeNode)

		r.planInfo.currentRequestMessage = r.planInfo.requestMessageAncestors[len(r.planInfo.requestMessageAncestors)-1]
		r.planInfo.requestMessageAncestors = r.planInfo.requestMessageAncestors[:len(r.planInfo.requestMessageAncestors)-1]

	case ast.NodeKindScalarTypeDefinition:
		dt := r.toDataType(&r.definition.Types[underlyingTypeNode.Ref])
		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, RPCField{
			Name:     r.operation.ArgumentNameString(argRef),
			TypeName: dt.String(),
			JSONPath: jsonPath,
			Index:    r.planInfo.currentRequestFieldIndex,
			Repeated: r.definition.TypeIsList(underlyingTypeNode.Ref),
		})

	case ast.NodeKindEnumTypeDefinition: // TODO handle enum types
		fmt.Println("enum")
	}

	r.planInfo.currentRequestFieldIndex++
}

// buildMessageFromNode builds a message structure from an AST node.
func (r *rpcPlanVisitor) buildMessageFromNode(node ast.Node) {
	switch node.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		inputObjectDefinition := r.definition.InputObjectTypeDefinitions[node.Ref]
		r.planInfo.currentRequestMessage.Fields = make(RPCFields, 0, len(inputObjectDefinition.InputFieldsDefinition.Refs))

		for fieldIndex, inputFieldRef := range inputObjectDefinition.InputFieldsDefinition.Refs {
			fieldDefinition := r.definition.InputValueDefinitions[inputFieldRef]
			fieldName := r.definition.Input.ByteSliceString(fieldDefinition.Name)
			r.buildMessageField(fieldName, fieldIndex, fieldDefinition.Type)
		}
	}
}

// buildMessageField creates a field in the current request message based on the field type.
func (r *rpcPlanVisitor) buildMessageField(fieldName string, index, typeRef int) {
	inputValueDefinitionType := r.definition.Types[typeRef]
	underlyingTypeName := r.definition.ResolveTypeNameString(typeRef)
	underlyingTypeNode, found := r.definition.NodeByNameStr(underlyingTypeName)
	if !found {
		return
	}

	// If the type is not an object, directly add the field to the request message
	// TODO: check interfaces, unions, enums, etc.
	if underlyingTypeNode.Kind != ast.NodeKindInputObjectTypeDefinition {
		dt := r.toDataType(&inputValueDefinitionType)

		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, RPCField{
			Name:     fieldName,
			TypeName: dt.String(),
			JSONPath: fieldName,
			Index:    index,
			Repeated: r.definition.TypeIsList(typeRef),
		})

		return
	}

	msg := &RPCMessage{
		Name: underlyingTypeName,
	}

	r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, RPCField{
		Name:     fieldName,
		TypeName: DataTypeMessage.String(),
		JSONPath: fieldName,
		Index:    index,
		Message:  msg,
	})

	r.planInfo.requestMessageAncestors = append(r.planInfo.requestMessageAncestors, r.planInfo.currentRequestMessage)
	r.planInfo.currentRequestMessage = msg

	r.buildMessageFromNode(underlyingTypeNode)

	r.planInfo.currentRequestMessage = r.planInfo.requestMessageAncestors[len(r.planInfo.requestMessageAncestors)-1]
	r.planInfo.requestMessageAncestors = r.planInfo.requestMessageAncestors[:len(r.planInfo.requestMessageAncestors)-1]
}

// EnterSelectionSet implements astvisitor.EnterSelectionSetVisitor.
// Checks if this is in the root level below the operation definition.
//
// TODO handle multiple entity lookups in a single query.
// We need to create a new call for each entity lookup.
func (r *rpcPlanVisitor) EnterSelectionSet(ref int) {
	if r.walker.Ancestor().Kind == ast.NodeKindOperationDefinition {
		r.currentCall = &RPCCall{
			CallID:      r.currentCallID,
			ServiceName: r.subgraphName,
		}

		r.planInfo.currentRequestMessage = &r.currentCall.Request
		r.planInfo.currentResponseMessage = &r.currentCall.Response

		// The operation is in the root level below the operation definition.
		// Only scaffolding the call here.
		return
	}

	if len(r.planInfo.currentResponseMessage.Fields) == 0 {
		return
	}

	// In nested selection sets, a new message needs to be created, which will be added to the current response message.
	if r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message == nil {
		r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message = r.newMessgeFromSelectionSet(ref)
	}

	// Add the current response message to the ancestors and set the current response message to the current field message
	r.planInfo.responseMessageAncestors = append(r.planInfo.responseMessageAncestors, r.planInfo.currentResponseMessage)
	r.planInfo.currentResponseMessage = r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message

	// Keep track of the field indices for the current response message.
	// This is used to set the correct field index for the current response message
	// when leaving the selection set.
	r.planInfo.responseFieldIndexAncestors = append(r.planInfo.responseFieldIndexAncestors, r.planInfo.currentResponseFieldIndex)
	r.planInfo.currentResponseFieldIndex = 0 // reset the field index for the current selection set
}

// newMessgeFromSelectionSet creates a new message from a selection set.
func (r *rpcPlanVisitor) newMessgeFromSelectionSet(ref int) *RPCMessage {
	message := &RPCMessage{
		Name:   r.walker.EnclosingTypeDefinition.NameString(r.definition),
		Fields: make(RPCFields, 0, len(r.operation.SelectionSets[ref].SelectionRefs)),
	}

	return message
}

// LeaveSelectionSet implements astvisitor.SelectionSetVisitor.
// It updates the current response field index and response message ancestors.
// If the ancestor is an operation definition, it adds the current call to the group.
func (r *rpcPlanVisitor) LeaveSelectionSet(ref int) {
	if len(r.planInfo.responseFieldIndexAncestors) > 0 {
		r.planInfo.currentResponseFieldIndex = r.planInfo.responseFieldIndexAncestors[len(r.planInfo.responseFieldIndexAncestors)-1]
		r.planInfo.responseFieldIndexAncestors = r.planInfo.responseFieldIndexAncestors[:len(r.planInfo.responseFieldIndexAncestors)-1]
	}

	if len(r.planInfo.responseMessageAncestors) > 0 {
		r.planInfo.currentResponseMessage = r.planInfo.responseMessageAncestors[len(r.planInfo.responseMessageAncestors)-1]
		r.planInfo.responseMessageAncestors = r.planInfo.responseMessageAncestors[:len(r.planInfo.responseMessageAncestors)-1]
	}

	if r.walker.Ancestor().Kind == ast.NodeKindOperationDefinition {
		methodName := r.rpcMethodName()

		r.currentCall.MethodName = methodName
		r.currentCall.Request.Name = methodName + "Request"
		r.currentCall.Response.Name = methodName + "Response"

		r.plan.Groups[r.currentGroupIndex].Calls = append(r.plan.Groups[r.currentGroupIndex].Calls, *r.currentCall)
		r.currentCall = nil
	}
}

// EnterInlineFragment implements astvisitor.InlineFragmentVisitor.
func (r *rpcPlanVisitor) EnterInlineFragment(ref int) {
	entityInfo := &r.planInfo.entityInfo
	if entityInfo.entityRootFieldRef != -1 && entityInfo.entityInlineFragmentRef == -1 {
		entityInfo.entityInlineFragmentRef = ref
		r.resolveEntityInformation(ref)
		r.scaffoldEntityLookup()
	}
}

// LeaveInlineFragment implements astvisitor.InlineFragmentVisitor.
func (r *rpcPlanVisitor) LeaveInlineFragment(ref int) {
	if ref == r.planInfo.entityInfo.entityInlineFragmentRef {
		r.planInfo.entityInfo.entityInlineFragmentRef = -1
	}
}

// EnterField implements astvisitor.EnterFieldVisitor.
func (r *rpcPlanVisitor) EnterField(ref int) {
	fieldName := r.operation.FieldNameString(ref)
	if fieldName == "_entities" {
		r.planInfo.entityInfo.entityRootFieldRef = ref
		return
	}

	fd, ok := r.walker.FieldDefinition(ref)
	if !ok {
		r.walker.Report.AddExternalError(operationreport.ExternalError{
			Message: fmt.Sprintf("Field %s not found in definition", r.operation.FieldNameString(ref)),
		})
		return
	}

	fdt := r.definition.FieldDefinitionType(fd)
	typeName := r.toDataType(&r.definition.Types[fdt])

	r.planInfo.currentResponseMessage.Fields = append(r.planInfo.currentResponseMessage.Fields, RPCField{
		Name:     fieldName, // TODO: this needs to be in snake_case
		TypeName: typeName.String(),
		JSONPath: fieldName,
		Index:    r.planInfo.currentResponseFieldIndex,
		Repeated: r.definition.TypeIsList(fdt),
		// TODO check for list of lists
	})
}

// LeaveField implements astvisitor.FieldVisitor.
func (r *rpcPlanVisitor) LeaveField(ref int) {
	if ref == r.planInfo.entityInfo.entityRootFieldRef {
		r.planInfo.entityInfo.entityRootFieldRef = -1
	}

	r.planInfo.currentResponseFieldIndex++
}

func (r *rpcPlanVisitor) resolveEntityInformation(inlineFragmentRef int) {
	// TODO support multiple entities in a single query
	if !r.planInfo.isEntityLookup || r.planInfo.entityInfo.name != "" {
		return
	}

	fragmentName := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
	node, found := r.definition.NodeByNameStr(fragmentName)
	if !found {
		return
	}

	// Only process object type definitions
	if node.Kind != ast.NodeKindObjectTypeDefinition {
		return
	}

	// An entity must at least have a key directive
	def := r.definition.ObjectTypeDefinitions[node.Ref]
	if !def.HasDirectives {
		return
	}

	for _, directiveRef := range def.Directives.Refs {
		if r.definition.DirectiveNameString(directiveRef) != federationKeyDirectiveName {
			continue
		}

		r.planInfo.entityInfo.name = fragmentName

		directive := r.definition.Directives[directiveRef]
		for _, argRef := range directive.Arguments.Refs {
			args := r.definition.Arguments[argRef]

			keyFieldName := r.definition.ValueContentString(args.Value)

			fieldDef, ok := r.definition.NodeFieldDefinitionByName(node, ast.ByteSlice(keyFieldName))
			if !ok {
				r.walker.Report.AddExternalError(operationreport.ExternalError{
					Message: fmt.Sprintf("Field %s not found in definition", keyFieldName),
				})
				return
			}

			fdt := r.definition.FieldDefinitionType(fieldDef)
			ft := r.definition.Types[fdt]

			r.planInfo.entityInfo.keyFields =
				append(r.planInfo.entityInfo.keyFields, keyField{
					fieldName: keyFieldName,
					fieldType: r.toDataType(&ft).String(),
				})
		}
	}

	keyFields := make([]string, 0, len(r.planInfo.entityInfo.keyFields))
	for _, key := range r.planInfo.entityInfo.keyFields {
		keyFields = append(keyFields, key.fieldName)
	}

	r.planInfo.entityInfo.keyTypeName = r.planInfo.entityInfo.name + "By" + strings.Join(titleSlice(keyFields), "And")
}

// scaffoldEntityLookup creates the entity lookup call structure
// by creating the key field message and adding it to the current request message.
// It also adds the results message to the current response message.
func (r *rpcPlanVisitor) scaffoldEntityLookup() {
	if !r.planInfo.isEntityLookup {
		return
	}

	entityInfo := &r.planInfo.entityInfo
	keyFieldMessage := &RPCMessage{
		Name: entityInfo.keyTypeName + "Key",
	}
	for i, key := range entityInfo.keyFields {
		keyFieldMessage.Fields = append(keyFieldMessage.Fields, RPCField{
			Index:    i,
			Name:     key.fieldName,
			TypeName: key.fieldType,
			JSONPath: key.fieldName,
		})
	}

	r.planInfo.currentRequestMessage.Fields = []RPCField{
		{
			Name:     "inputs",
			TypeName: DataTypeMessage.String(),
			Repeated: true, // The inputs are always a list of objects
			JSONPath: "representations",
			Index:    0,
			Message: &RPCMessage{
				Name: r.rpcMethodName() + "Input",
				Fields: RPCFields{
					{
						Index:    0,
						Name:     "key",
						TypeName: DataTypeMessage.String(),
						Message:  keyFieldMessage,
					},
				},
			},
		},
	}

	r.planInfo.currentRequestMessage = keyFieldMessage

	resultMessage := &RPCMessage{
		Name: r.planInfo.entityInfo.name,
	}

	r.planInfo.currentResponseMessage.Fields = []RPCField{
		{
			Index:    0,
			Name:     "results",
			TypeName: DataTypeMessage.String(),
			JSONPath: "results",
			Repeated: true,
			Message: &RPCMessage{
				Name: r.rpcMethodName() + "Result",
				Fields: RPCFields{
					{
						Index:    0,
						Name:     strings.ToLower(r.planInfo.entityInfo.name),
						TypeName: DataTypeMessage.String(),
						Message:  resultMessage,
					},
				},
			},
		},
	}

	r.planInfo.responseMessageAncestors = append(r.planInfo.responseMessageAncestors, r.planInfo.currentResponseMessage)
	r.planInfo.currentResponseMessage = r.planInfo.currentResponseMessage.Fields[0].Message
}

// rpcMethodName determines the appropriate method name based on operation type.
func (r *rpcPlanVisitor) rpcMethodName() string {
	if r.planInfo.methodName != "" {
		return r.planInfo.methodName
	}

	switch r.planInfo.operationType {
	case ast.OperationTypeQuery:
		r.planInfo.methodName = r.buildQueryMethodName()
	case ast.OperationTypeMutation:
		r.planInfo.methodName = r.buildMutationMethodName()
	case ast.OperationTypeSubscription:
		r.planInfo.methodName = r.buildSubscriptionMethodName()
	}

	return r.planInfo.methodName
}

// buildQueryMethodName constructs a method name for query operations.
func (r *rpcPlanVisitor) buildQueryMethodName() string {
	if r.planInfo.isEntityLookup && r.planInfo.entityInfo.name != "" {
		return "Lookup" + r.planInfo.entityInfo.keyTypeName
	}

	return "Query" + strings.Title(r.planInfo.operationFieldName)
}

// buildMutationMethodName constructs a method name for mutation operations.
func (r *rpcPlanVisitor) buildMutationMethodName() string {
	// TODO implement mutation method name handling
	return "Mutation" + strings.Title(r.planInfo.operationFieldName)
}

// buildSubscriptionMethodName constructs a method name for subscription operations.
func (r *rpcPlanVisitor) buildSubscriptionMethodName() string {
	// TODO implement subscription method name handling
	return "Subscription" + strings.Title(r.planInfo.operationFieldName)
}

// toDataType converts an ast.Type to a DataType.
// It handles the different type kinds and non-null types.
func (r *rpcPlanVisitor) toDataType(t *ast.Type) DataType {
	switch t.TypeKind {
	case ast.TypeKindNamed:
		return r.parseGraphQLType(t)
	case ast.TypeKindList:
		return DataTypeMessage
	case ast.TypeKindNonNull:
		return r.toDataType(&r.definition.Types[t.OfType])
	}

	return DataTypeUnknown
}

// parseGraphQLType parses an ast.Type and returns the corresponding DataType.
// It handles the different type kinds and non-null types.
func (r *rpcPlanVisitor) parseGraphQLType(t *ast.Type) DataType {
	dt := r.definition.Input.ByteSliceString(t.Name)

	// Retrieve the node to check the kind
	node, found := r.definition.NodeByNameStr(dt)
	if !found {
		return DataTypeUnknown
	}

	// For non-scalar types, return the corresponding DataType
	switch node.Kind {
	case ast.NodeKindInterfaceTypeDefinition:
		fallthrough
	case ast.NodeKindUnionTypeDefinition:
		fallthrough
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInputObjectTypeDefinition:
		return DataTypeMessage
	case ast.NodeKindEnumTypeDefinition:
		return DataTypeEnum
	default:
		return fromGraphQLType(dt)
	}
}

// titleSlice capitalizes the first letter of each string in a slice.
func titleSlice(s []string) []string {
	for i, v := range s {
		s[i] = strings.Title(v)
	}
	return s
}
