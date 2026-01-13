package grpcdatasource

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type rpcPlanVisitorFederation struct {
	walker     *astvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
	planCtx    *rpcPlanningContext
	mapping    *GRPCMapping

	planInfo     planningInfo
	entityInfo   entityInfo
	entityConfig entityConfig

	plan         *RPCExecutionPlan
	subgraphName string
	currentCall  *RPCCall

	parentCallID           int
	fieldResolverAncestors stack[int]
	resolvedFields         []resolverField

	fieldPath ast.Path
}

func newRPCPlanVisitorFederation(config rpcPlanVisitorConfig) *rpcPlanVisitorFederation {
	walker := astvisitor.NewWalker(48)
	visitor := &rpcPlanVisitorFederation{
		walker:       &walker,
		plan:         &RPCExecutionPlan{},
		subgraphName: cases.Title(language.Und, cases.NoLower).String(config.subgraphName),
		mapping:      config.mapping,
		entityInfo: entityInfo{
			entityRootFieldRef:      ast.InvalidRef,
			entityInlineFragmentRef: ast.InvalidRef,
		},
		entityConfig:           parseFederationConfigData(config.federationConfigs),
		resolvedFields:         make([]resolverField, 0),
		fieldResolverAncestors: newStack[int](0),
		parentCallID:           ast.InvalidRef,
		fieldPath:              ast.Path{}.WithFieldNameItem([]byte("result")),
	}

	walker.RegisterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	walker.RegisterInlineFragmentVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)

	return visitor
}

func (r *rpcPlanVisitorFederation) PlanOperation(operation, definition *ast.Document) (*RPCExecutionPlan, error) {
	report := &operationreport.Report{}
	r.walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil, fmt.Errorf("unable to plan operation: %w", report)
	}

	return r.plan, nil
}

// EnterDocument implements astvisitor.EnterDocumentVisitor.
func (r *rpcPlanVisitorFederation) EnterDocument(operation *ast.Document, definition *ast.Document) {
	r.operation = operation
	r.definition = definition

	r.planCtx = newRPCPlanningContext(operation, definition, r.mapping)
}

// LeaveDocument implements astvisitor.DocumentVisitor.
func (r *rpcPlanVisitorFederation) LeaveDocument(_, _ *ast.Document) {
	calls, err := r.planCtx.createResolverRPCCalls(r.subgraphName, r.resolvedFields)
	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	if len(calls) > 0 {
		r.plan.Calls = append(r.plan.Calls, calls...)
		r.resolvedFields = nil
	}

	for entityTypeName, entityConfigData := range r.entityConfig {
		if len(entityConfigData.requiredFields) == 0 {
			continue
		}

		calls, err = r.planCtx.createRequiredFieldsRPCCalls(r.subgraphName, entityTypeName, entityConfigData)
		if err != nil {
			r.walker.StopWithInternalErr(err)
			return
		}

		if len(calls) > 0 {
			r.plan.Calls = append(r.plan.Calls, calls...)
		}

	}
}

// EnterOperationDefinition implements astvisitor.EnterOperationDefinitionVisitor.
func (r *rpcPlanVisitorFederation) EnterOperationDefinition(ref int) {
	if r.operation.OperationDefinitions[ref].OperationType != ast.OperationTypeQuery {
		r.walker.StopWithInternalErr(errors.New("only query operations are supported for the federation plan visitor"))
		return
	}

	r.planInfo.operationType = r.operation.OperationDefinitions[ref].OperationType
}

// EnterInlineFragment implements astvisitor.InlineFragmentVisitor.
func (r *rpcPlanVisitorFederation) EnterInlineFragment(ref int) {
	fragmentName := r.operation.InlineFragmentTypeConditionNameString(ref)
	entityConfigData, ok := r.entityConfig.getEntity(fragmentName)
	if !ok {
		return
	}

	r.currentCall = &RPCCall{
		ServiceName: r.planCtx.resolveServiceName(r.subgraphName),
		Kind:        CallKindEntity,
	}

	r.parentCallID = len(r.plan.Calls)

	r.planInfo.currentRequestMessage = &r.currentCall.Request
	r.planInfo.currentResponseMessage = &r.currentCall.Response

	r.entityInfo.entityInlineFragmentRef = ref
	r.entityInfo.typeName = fragmentName
	if err := r.resolveEntityInformation(ref, fragmentName, entityConfigData); err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	r.scaffoldEntityLookup(fragmentName, entityConfigData)
}

// LeaveInlineFragment implements astvisitor.InlineFragmentVisitor.
func (r *rpcPlanVisitorFederation) LeaveInlineFragment(ref int) {
	if r.entityInfo.entityInlineFragmentRef != ref {
		// We only handle the entity inline fragment
		return
	}

	r.plan.Calls = append(r.plan.Calls, *r.currentCall)
	r.currentCall = &RPCCall{}

	r.planInfo = planningInfo{
		operationType:               r.planInfo.operationType,
		operationFieldName:          r.planInfo.operationFieldName,
		currentRequestMessage:       &RPCMessage{},
		currentResponseMessage:      &RPCMessage{},
		currentResponseFieldIndex:   0,
		responseMessageAncestors:    []*RPCMessage{},
		responseFieldIndexAncestors: []int{},
	}

	r.entityInfo.entityInlineFragmentRef = ast.InvalidRef
}

// EnterSelectionSet implements astvisitor.SelectionSetVisitor.
func (r *rpcPlanVisitorFederation) EnterSelectionSet(ref int) {
	if r.walker.Ancestor().Kind == ast.NodeKindOperationDefinition {
		return
	}

	// If we are inside of a resolved field that selects multiple fields, we get all the fields from the input and pass them to the required fields visitor.
	if r.fieldResolverAncestors.len() > 0 {
		if r.walker.Ancestor().Kind == ast.NodeKindInlineFragment {
			return
		}

		resolvedFieldAncestor := r.fieldResolverAncestors.peek()
		if compositType := r.planCtx.getCompositeType(r.walker.EnclosingTypeDefinition); compositType != OneOfTypeNone {
			memberTypes, err := r.planCtx.getMemberTypes(r.walker.EnclosingTypeDefinition)
			if err != nil {
				r.walker.StopWithInternalErr(err)
				return
			}
			resolvedField := &r.resolvedFields[resolvedFieldAncestor]
			resolvedField.memberTypes = memberTypes
			r.planCtx.enterResolverCompositeSelectionSet(compositType, ref, resolvedField)
			return
		}

		r.resolvedFields[resolvedFieldAncestor].fieldsSelectionSetRef = ref
		return
	}

	if r.planInfo.currentRequestMessage == nil || len(r.planInfo.currentResponseMessage.Fields) == 0 || len(r.planInfo.currentResponseMessage.Fields) <= r.planInfo.currentResponseFieldIndex {
		return
	}

	// In nested selection sets, a new message needs to be created, which will be added to the current response message.
	if r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message == nil {
		r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message = r.planCtx.newMessageFromSelectionSet(r.walker.EnclosingTypeDefinition, ref)
	}

	// Add the current response message to the ancestors and set the current response message to the current field message
	r.planInfo.responseMessageAncestors = append(r.planInfo.responseMessageAncestors, r.planInfo.currentResponseMessage)
	r.planInfo.currentResponseMessage = r.planInfo.currentResponseMessage.Fields[r.planInfo.currentResponseFieldIndex].Message

	// Ensure that the entity inline fragment message has a typename field,
	// to map the json data after receiving the response.
	if r.IsEntityInlineFragment(r.walker.Ancestor()) {
		r.planInfo.currentResponseMessage.AppendTypeNameField(r.entityInfo.typeName)
	}

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

	// Reset the field index for the current selection set to the length of the current response message fields.
	r.planInfo.currentResponseFieldIndex = len(r.planInfo.currentResponseMessage.Fields)
}

func (r *rpcPlanVisitorFederation) handleCompositeType(node ast.Node) error {
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
func (r *rpcPlanVisitorFederation) LeaveSelectionSet(ref int) {
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

// EnterField implements astvisitor.FieldVisitor.
func (r *rpcPlanVisitorFederation) EnterField(ref int) {
	fieldName := r.operation.FieldNameString(ref)
	inRootField := r.walker.InRootField()
	if inRootField {
		r.planInfo.operationFieldName = r.operation.FieldNameString(ref)
	}

	if fieldName == "_entities" {
		// _entities is a special field that is used to look up entities
		// Entity lookups are handled differently as we use special types for
		// Providing variables (_Any) and the response type is a Union that needs to be
		// determined from the first inline fragment.
		r.entityInfo = entityInfo{
			entityRootFieldRef:      ref,
			entityInlineFragmentRef: ast.InvalidRef,
		}

		r.entityInfo.entityRootFieldRef = ref
		return
	}

	fieldDefRef, ok := r.walker.FieldDefinition(ref)
	if !ok {
		r.walker.Report.AddExternalError(operationreport.ExternalError{
			Message: fmt.Sprintf("Field %s not found in definition %s", r.operation.FieldNameString(ref), r.walker.EnclosingTypeDefinition.NameString(r.definition)),
		})
		return
	}

	// If the field is a required field, we don't want to add it to the current response message.
	if r.planCtx.isRequiredField(fieldDefRef) {
		return
	}

	// If the field is a field resolver, we need to handle it later in a separate resolver call.
	// We only store the information about the field and create the call later.
	if r.planCtx.isFieldResolver(fieldDefRef, inRootField) {
		r.enterFieldResolver(ref, fieldDefRef)
		return
	}

	// Check if the field is inside of a resolver call.
	if r.fieldResolverAncestors.len() > 0 {
		// We don't want to call LeaveField here because we ignore the field entirely.
		r.walker.SkipNode()
		return
	}

	// prevent duplicate fields
	fieldAlias := r.operation.FieldAliasString(ref)
	if r.planInfo.currentResponseMessage.Fields.Exists(fieldName, fieldAlias) {
		r.walker.SkipNode()
		return
	}

	field, err := r.planCtx.buildField(
		r.walker.EnclosingTypeDefinition.NameString(r.definition),
		fieldDefRef,
		fieldName,
		fieldAlias,
	)

	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	// If we have a nested or nullable list, we add a @ prefix to indicate the nesting level.
	prefix := ""
	if field.ListMetadata != nil {
		prefix = strings.Repeat("@", field.ListMetadata.NestingLevel)
	}

	r.fieldPath = r.fieldPath.WithFieldNameItem([]byte(prefix + field.Name))

	// check if we are inside of an inline fragment and not the entity inline fragment
	if ref, ok := r.walker.ResolveInlineFragment(); ok && r.entityInfo.entityInlineFragmentRef != ref {
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
func (r *rpcPlanVisitorFederation) LeaveField(ref int) {
	r.fieldPath = r.fieldPath.RemoveLastItem()

	inRootField := r.walker.InRootField()
	if inRootField {
		return
	}

	fieldDefRef, ok := r.walker.FieldDefinition(ref)
	if !ok {
		r.walker.Report.AddExternalError(operationreport.ExternalError{
			Message: fmt.Sprintf("Field %s not found in definition %s", r.operation.FieldNameString(ref), r.walker.EnclosingTypeDefinition.NameString(r.definition)),
		})
		return
	}

	if r.planCtx.isFieldResolver(fieldDefRef, inRootField) {
		// Pop the field resolver ancestor only when leaving a field resolver field.
		r.fieldResolverAncestors.pop()

		// If the field has arguments, we need to decrement the related call ID.
		// This is because we can also have nested arguments, which require the underlying field to be resolved
		// by values provided by the parent call.
		r.parentCallID--

		// We handle field resolvers differently, so we don't want to increment the response field index.
		return
	}

	// If we are not in the operation field, we can increment the response field index.
	r.planInfo.currentResponseFieldIndex++
}

// enterFieldResolver enters a field resolver.
// ref is the field reference in the operation document.
// fieldDefRef is the field definition reference in the definition document.
func (r *rpcPlanVisitorFederation) enterFieldResolver(ref int, fieldDefRef int) {
	// Field arguments for non root types will be handled as resolver calls.
	// We need to make sure to handle a hierarchy of arguments in order to perform parallel calls in order to retrieve the data.
	fieldArgs := r.operation.FieldArguments(ref)
	// We don't want to add fields from the selection set to the actual call
	resolvedField := resolverField{
		callerRef:              r.parentCallID,
		parentTypeNode:         r.walker.EnclosingTypeDefinition,
		fieldRef:               ref,
		responsePath:           r.walker.Path[1:].WithoutInlineFragmentNames().WithFieldNameItem(r.operation.FieldAliasOrNameBytes(ref)),
		fieldDefinitionTypeRef: r.definition.FieldDefinitionType(fieldDefRef),
	}

	if err := r.planCtx.setResolvedField(r.walker, fieldDefRef, fieldArgs, r.fieldPath, &resolvedField); err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	r.resolvedFields = append(r.resolvedFields, resolvedField)
	r.fieldResolverAncestors.push(len(r.resolvedFields) - 1)
	r.fieldPath = r.fieldPath.WithFieldNameItem(r.operation.FieldNameBytes(ref))

	// In case of nested fields with arguments, we need to increment the related call ID.
	r.parentCallID++
}

func (r *rpcPlanVisitorFederation) resolveEntityInformation(inlineFragmentRef int, entityTypeName string, entityConfigData entityConfigData) error {
	fragmentName := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
	node, found := r.definition.NodeByNameStr(r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef))
	if !found {
		return errors.New("definition node not found for inline fragment: " + fragmentName)
	}

	// Only process object type definitions
	// TODO: handle interfaces
	if node.Kind != ast.NodeKindObjectTypeDefinition {
		return nil
	}

	rpcConfig, exists := r.mapping.FindEntityRPCConfig(entityTypeName, entityConfigData.keyFields)
	if !exists {
		return fmt.Errorf("entity type %s not found in mapping", entityTypeName)
	}

	r.currentCall.Request.Name = rpcConfig.Request
	r.currentCall.Response.Name = rpcConfig.Response
	r.currentCall.MethodName = rpcConfig.RPC

	return nil
}

// scaffoldEntityLookup creates the entity lookup call structure
// by creating the key field message and adding it to the current request message.
// It also adds the results message to the current response message.
func (r *rpcPlanVisitorFederation) scaffoldEntityLookup(typeName string, ecd entityConfigData) {
	keyFieldMessage := &RPCMessage{
		Name: r.currentCall.MethodName + "Key",
	}

	walker := astvisitor.WalkerFromPool()
	defer walker.Release()

	requiredFieldsVisitor := newRequiredFieldsVisitor(walker, keyFieldMessage, r.planCtx)
	err := requiredFieldsVisitor.visitWithDefaults(r.definition, typeName, ecd.keyFields)
	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	r.planInfo.currentRequestMessage.Fields = []RPCField{
		{
			Name:          "keys",
			ProtoTypeName: DataTypeMessage,
			Repeated:      true, // The inputs are always a list of objects
			JSONPath:      "representations",
			Message:       keyFieldMessage,
		},
	}

	r.entityConfig.setEntityKeyMessage(typeName, keyFieldMessage)

	// The proto response message has a field `result` which is a list of entities.
	// As this is a special case we directly map it to _entities.
	r.planInfo.currentResponseMessage.Fields = []RPCField{
		{
			Name:          "result",
			ProtoTypeName: DataTypeMessage,
			JSONPath:      "_entities",
			Repeated:      true,
		},
	}
}

func (r *rpcPlanVisitorFederation) IsEntityInlineFragment(node ast.Node) bool {
	if node.Kind != ast.NodeKindInlineFragment {
		return false
	}

	if r.entityInfo.entityInlineFragmentRef == ast.InvalidRef {
		return false
	}

	return r.entityInfo.entityInlineFragmentRef == node.Ref
}

// entityInfo contains the information about the entity that is being looked up.
type entityInfo struct {
	typeName                string
	entityRootFieldRef      int
	entityInlineFragmentRef int
}

type requiredFieldData struct {
	typeName     string
	fieldName    string
	selectionSet string
}

type entityConfig map[string]entityConfigData

type entityConfigData struct {
	keyFields       string
	keyFieldMessage *RPCMessage
	requiredFields  map[string]string
}

func (e entityConfig) setEntity(typeName string, data entityConfigData) {
	e[typeName] = data
}

func (e entityConfig) setEntityKeyMessage(typeName string, message *RPCMessage) {
	data, ok := e[typeName]
	if !ok {
		return
	}

	data.keyFieldMessage = message
	e[typeName] = data
}

func (e entityConfig) getEntity(typeName string) (entityConfigData, bool) {
	data, ok := e[typeName]
	return data, ok
}

func (e entityConfig) setRequiredField(typeName, fieldName, selectionSet string) {
	if _, ok := e[typeName]; !ok {
		e[typeName] = entityConfigData{
			requiredFields: make(map[string]string),
		}
	}

	e[typeName].requiredFields[fieldName] = selectionSet
}

func parseFederationConfigData(federationConfigs plan.FederationFieldConfigurations) entityConfig {
	config := make(entityConfig)

	for _, fc := range federationConfigs {
		data, ok := config.getEntity(fc.TypeName)
		if !ok {
			data = entityConfigData{
				requiredFields: make(map[string]string),
			}
		}

		if fc.FieldName != "" {
			data.requiredFields[fc.FieldName] = fc.SelectionSet
		} else {
			data.keyFields = fc.SelectionSet
		}

		config.setEntity(fc.TypeName, data)
	}

	return config
}
