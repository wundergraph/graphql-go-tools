package grpcdatasource

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
	planInfo   planningInfo

	subgraphName           string
	mapping                *GRPCMapping
	plan                   *RPCExecutionPlan
	operationDefinitionRef int
	operationFieldRef      int
	operationFieldRefs     []int
	currentCall            *RPCCall
	currentCallID          int
}

type rpcPlanVisitorConfig struct {
	subgraphName string
	mapping      *GRPCMapping
}

// newRPCPlanVisitor creates a new RPCPlanVisitor.
// It registers the visitor with the walker and returns it.
func newRPCPlanVisitor(walker *astvisitor.Walker, config rpcPlanVisitorConfig) *rpcPlanVisitor {
	visitor := &rpcPlanVisitor{
		walker:                 walker,
		plan:                   &RPCExecutionPlan{},
		subgraphName:           cases.Title(language.Und, cases.NoLower).String(config.subgraphName),
		mapping:                config.mapping,
		operationDefinitionRef: -1,
		operationFieldRef:      -1,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
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
	r.operationDefinitionRef = ref

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
	if r.planInfo.isEntityLookup {
		return
	}

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
//
// TODO handle multiple entity lookups in a single query.
// We need to create a new call for each entity lookup.
func (r *rpcPlanVisitor) EnterSelectionSet(ref int) {
	if r.walker.Ancestor().Kind == ast.NodeKindOperationDefinition {
		return
	}

	if len(r.planInfo.currentResponseMessage.Fields) == 0 || len(r.planInfo.currentResponseMessage.Fields) <= r.planInfo.currentResponseFieldIndex {
		return
	}

	// In nested selection sets, a new message needs to be created, which will be added to the current response message.
	if r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message == nil {
		r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message = r.newMessageFromSelectionSet(ref)
	}

	// Add the current response message to the ancestors and set the current response message to the current field message
	r.planInfo.responseMessageAncestors = append(r.planInfo.responseMessageAncestors, r.planInfo.currentResponseMessage)
	r.planInfo.currentResponseMessage = r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message

	r.planInfo.currentResponseMessage.OneOf = r.isInterface(r.walker.Ancestor())

	// Keep track of the field indices for the current response message.
	// This is used to set the correct field index for the current response message
	// when leaving the selection set.
	r.planInfo.responseFieldIndexAncestors = append(r.planInfo.responseFieldIndexAncestors, r.planInfo.currentResponseFieldIndex)

	r.planInfo.currentResponseFieldIndex = 0 // reset the field index for the current selection set
}

func (r *rpcPlanVisitor) isInterface(node ast.Node) bool {
	if node.Kind == ast.NodeKindInterfaceTypeDefinition {
		return true
	}

	switch node.Kind {
	case ast.NodeKindField:
		if r.walker.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
			return true
		}
	}

	return false
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
}

// EnterInlineFragment implements astvisitor.InlineFragmentVisitor.
func (r *rpcPlanVisitor) EnterInlineFragment(ref int) {
	entityInfo := &r.planInfo.entityInfo
	if entityInfo.entityRootFieldRef != -1 && entityInfo.entityInlineFragmentRef == -1 {
		entityInfo.entityInlineFragmentRef = ref
		r.resolveEntityInformation(ref)
		r.scaffoldEntityLookup()

		return
	}
}

// LeaveInlineFragment implements astvisitor.InlineFragmentVisitor.
func (r *rpcPlanVisitor) LeaveInlineFragment(ref int) {
	if ref == r.planInfo.entityInfo.entityInlineFragmentRef {
		r.planInfo.entityInfo.entityInlineFragmentRef = -1
	}
}

func (r *rpcPlanVisitor) isInRootField() bool {
	return len(r.walker.Ancestors) == 2 && r.walker.Ancestors[0].Kind == ast.NodeKindOperationDefinition
}

func (r *rpcPlanVisitor) handleRootField(ref int) error {
	r.operationFieldRef = ref
	r.planInfo.operationFieldName = r.operation.FieldNameString(ref)

	r.currentCall = &RPCCall{
		CallID:      r.currentCallID,
		ServiceName: r.resolveServiceName(),
	}

	r.planInfo.currentRequestMessage = &r.currentCall.Request
	r.planInfo.currentResponseMessage = &r.currentCall.Response

	// attempt to resolve the name from the mapping
	if err := r.resolveRPCMethodMapping(); err != nil {
		return err
	}

	return nil
}

// EnterField implements astvisitor.EnterFieldVisitor.
func (r *rpcPlanVisitor) EnterField(ref int) {
	fieldName := r.operation.FieldNameString(ref)
	if r.isInRootField() {
		if err := r.handleRootField(ref); err != nil {
			r.walker.StopWithInternalErr(err)
			return
		}
	}

	if fieldName == "_entities" {
		// _entities is a special field that is used to look up entities
		// Entity lookups are handled differently as we use special types for
		// Providing variables (_Any) and the response type is a Union that needs to be
		// determined from the first inline fragment.
		r.planInfo.entityInfo = entityInfo{
			entityRootFieldRef:      ref,
			entityInlineFragmentRef: -1,
		}
		r.planInfo.isEntityLookup = true
		r.planInfo.entityInfo.entityRootFieldRef = ref
		return
	}

	// prevent duplicate fields
	if r.planInfo.currentResponseMessage.Fields.Exists(fieldName) {
		return
	}

	fd, ok := r.walker.FieldDefinition(ref)
	if !ok {
		r.walker.Report.AddExternalError(operationreport.ExternalError{
			Message: fmt.Sprintf("Field %s not found in definition %s", r.operation.FieldNameString(ref), r.walker.EnclosingTypeDefinition.NameString(r.definition)),
		})
		return
	}

	fdt := r.definition.FieldDefinitionType(fd)
	typeName := r.toDataType(&r.definition.Types[fdt])

	parentTypeName := r.walker.EnclosingTypeDefinition.NameString(r.definition)

	field := RPCField{
		Name:     r.resolveFieldMapping(parentTypeName, fieldName),
		TypeName: typeName.String(),
		JSONPath: fieldName,
		Repeated: r.definition.TypeIsList(fdt),
	}

	if typeName == DataTypeEnum {
		field.EnumName = r.definition.FieldDefinitionTypeNameString(fd)
	}

	if fieldName == "__typename" {
		field.StaticValue = parentTypeName
	}

	r.planInfo.currentResponseMessage.Fields = append(r.planInfo.currentResponseMessage.Fields, field)
}

// LeaveField implements astvisitor.FieldVisitor.
func (r *rpcPlanVisitor) LeaveField(ref int) {
	if ref == r.planInfo.entityInfo.entityRootFieldRef {
		r.planInfo.entityInfo.entityRootFieldRef = -1
	}

	// If we are not in the operation field, we can increment the response field index.
	if !r.isInRootField() {
		r.planInfo.currentResponseFieldIndex++
		return
	}

	// If we left the operation field, we need to finalize the current call and prepare the next one.
	if r.currentCall.MethodName == "" {
		methodName := r.rpcMethodName()
		r.currentCall.MethodName = methodName
		r.currentCall.Request.Name = methodName + "Request"
		r.currentCall.Response.Name = methodName + "Response"
	}

	r.plan.Calls[r.currentCallID] = *r.currentCall
	r.currentCall = &RPCCall{}

	r.currentCallID++
	if r.currentCallID < len(r.operationFieldRefs) {
		r.operationFieldRef = r.operationFieldRefs[r.currentCallID]
	}

	r.planInfo.currentResponseFieldIndex = 0
}

// newMessageFromSelectionSet creates a new message from a selection set.
func (r *rpcPlanVisitor) newMessageFromSelectionSet(ref int) *RPCMessage {
	message := &RPCMessage{
		Name:   r.walker.EnclosingTypeDefinition.NameString(r.definition),
		Fields: make(RPCFields, 0, len(r.operation.SelectionSets[ref].SelectionRefs)),
	}

	return message
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

	// If the underlying type is an input object type, create a new message and add it to the current request message.
	switch underlyingTypeNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		msg := &RPCMessage{
			Name:   underlyingTypeName,
			Fields: RPCFields{},
		}

		// Add a field of type message to the current request message.
		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, RPCField{
			Name:     r.resolveFieldMapping(underlyingTypeName, fieldName),
			TypeName: DataTypeMessage.String(),
			JSONPath: jsonPath,
			Message:  msg,
			// Repeated: r.definition.TypeIsList(typeRef), TODO: handle repeated complex types
		})

		// Add the current request message to the ancestors and set the current request message to the new message.
		r.planInfo.requestMessageAncestors = append(r.planInfo.requestMessageAncestors, r.planInfo.currentRequestMessage)
		r.planInfo.currentRequestMessage = msg

		r.buildMessageFromNode(underlyingTypeNode)

		r.planInfo.currentRequestMessage = r.planInfo.requestMessageAncestors[len(r.planInfo.requestMessageAncestors)-1]
		r.planInfo.requestMessageAncestors = r.planInfo.requestMessageAncestors[:len(r.planInfo.requestMessageAncestors)-1]

	case ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
		rootNode := r.walker.TypeDefinitions[len(r.walker.TypeDefinitions)-2]
		baseType := r.definition.NodeNameString(rootNode)
		dt := r.toDataType(&r.definition.Types[typeRef])

		field := RPCField{
			Name:     r.resolveInputArgument(baseType, r.walker.Ancestor().Ref, fieldName),
			TypeName: dt.String(),
			JSONPath: jsonPath,
			Repeated: r.definition.TypeIsList(typeRef),
		}

		if dt == DataTypeEnum {
			field.EnumName = underlyingTypeName
		}

		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, field)
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

	// If the type is not an object, directly add the field to the request message
	// TODO: check interfaces, unions, etc.
	if underlyingTypeNode.Kind != ast.NodeKindInputObjectTypeDefinition {
		dt := r.toDataType(&inputValueDefinitionType)

		field := RPCField{
			Name:     r.resolveFieldMapping(parentTypeName, fieldName),
			TypeName: dt.String(),
			JSONPath: fieldName,
			Repeated: r.definition.TypeIsList(typeRef),
		}

		if dt == DataTypeEnum {
			field.EnumName = underlyingTypeName
		}

		r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, field)

		return
	}

	msg := &RPCMessage{
		Name: underlyingTypeName,
	}

	r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, RPCField{
		Name:     r.resolveFieldMapping(parentTypeName, fieldName),
		TypeName: DataTypeMessage.String(),
		JSONPath: fieldName,
		Message:  msg,
	})

	r.planInfo.requestMessageAncestors = append(r.planInfo.requestMessageAncestors, r.planInfo.currentRequestMessage)
	r.planInfo.currentRequestMessage = msg

	r.buildMessageFromNode(underlyingTypeNode)

	r.planInfo.currentRequestMessage = r.planInfo.requestMessageAncestors[len(r.planInfo.requestMessageAncestors)-1]
	r.planInfo.requestMessageAncestors = r.planInfo.requestMessageAncestors[:len(r.planInfo.requestMessageAncestors)-1]
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

		break
	}

	keyFields := make([]string, 0, len(r.planInfo.entityInfo.keyFields))
	for _, key := range r.planInfo.entityInfo.keyFields {
		keyFields = append(keyFields, key.fieldName)
	}

	if ei, exists := r.mapping.EntityRPCs[r.planInfo.entityInfo.name]; exists {
		r.currentCall.Request.Name = ei.RPCConfig.Request
		r.currentCall.Response.Name = ei.RPCConfig.Response
		r.planInfo.methodName = ei.RPCConfig.RPC
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
		Name: r.rpcMethodName() + "Key",
	}
	for _, key := range entityInfo.keyFields {
		keyFieldMessage.Fields = append(keyFieldMessage.Fields, RPCField{
			Name:     key.fieldName,
			TypeName: key.fieldType,
			JSONPath: key.fieldName,
		})
	}

	r.planInfo.currentRequestMessage.Fields = []RPCField{
		{
			Name:     "keys",
			TypeName: DataTypeMessage.String(),
			Repeated: true, // The inputs are always a list of objects
			JSONPath: "representations",
			Message:  keyFieldMessage,
		},
	}

	r.planInfo.currentResponseMessage.Fields = []RPCField{
		{
			Name:     "result",
			TypeName: DataTypeMessage.String(),
			JSONPath: "_entities",
			Repeated: true,
		},
	}
}

func (r *rpcPlanVisitor) resolveServiceName() string {
	if r.mapping == nil || r.mapping.Service == "" {
		return r.subgraphName
	}

	return r.mapping.Service
}

// resolveFieldMapping resolves the field mapping for a field.
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

func (r *rpcPlanVisitor) resolveRPCMethodMapping() error {
	if r.mapping == nil {
		return nil
	}

	if r.planInfo.isEntityLookup && r.planInfo.entityInfo.name != "" {
		// Resolving the entity lookup method name is done differently
		return nil
	}

	var rpcConfig RPCConfig
	var ok bool

	switch r.planInfo.operationType {
	case ast.OperationTypeQuery:
		rpcConfig, ok = r.mapping.QueryRPCs[r.planInfo.operationFieldName]
	case ast.OperationTypeMutation:
		rpcConfig, ok = r.mapping.MutationRPCs[r.planInfo.operationFieldName]
	case ast.OperationTypeSubscription:
		rpcConfig, ok = r.mapping.SubscriptionRPCs[r.planInfo.operationFieldName]
	}

	// if we don't have a mapping, we can skip the operation
	if !ok {
		return nil
	}

	// We require all fields to be present when defining a mapping for the operation
	if rpcConfig.RPC == "" {
		return fmt.Errorf("no rpc method name mapping found for operation %s", r.planInfo.operationFieldName)
	}

	if rpcConfig.Request == "" {
		return fmt.Errorf("no request message name mapping found for operation %s", r.planInfo.operationFieldName)
	}

	if rpcConfig.Response == "" {
		return fmt.Errorf("no response message name mapping found for operation %s", r.planInfo.operationFieldName)
	}

	r.currentCall.MethodName = rpcConfig.RPC
	r.currentCall.Request.Name = rpcConfig.Request
	r.currentCall.Response.Name = rpcConfig.Response

	return nil
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

	return "Query" + cases.Title(language.Und, cases.NoLower).String(r.planInfo.operationFieldName)
}

// buildMutationMethodName constructs a method name for mutation operations.
func (r *rpcPlanVisitor) buildMutationMethodName() string {
	// TODO implement mutation method name handling
	return "Mutation" + cases.Title(language.Und, cases.NoLower).String(r.planInfo.operationFieldName)
}

// buildSubscriptionMethodName constructs a method name for subscription operations.
func (r *rpcPlanVisitor) buildSubscriptionMethodName() string {
	// TODO implement subscription method name handling
	return "Subscription" + cases.Title(language.Und, cases.NoLower).String(r.planInfo.operationFieldName)
}

// toDataType converts an ast.Type to a DataType.
// It handles the different type kinds and non-null types.
func (r *rpcPlanVisitor) toDataType(t *ast.Type) DataType {
	switch t.TypeKind {
	case ast.TypeKindNamed:
		return r.parseGraphQLType(t)
	case ast.TypeKindList:
		return r.toDataType(&r.definition.Types[t.OfType])
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
		s[i] = cases.Title(language.Und, cases.NoLower).String(v)
	}
	return s
}
