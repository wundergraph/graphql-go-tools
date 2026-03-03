package grpcdatasource

import (
	"bytes"
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

// entityInfo contains the information about the entity that is being looked up.
type entityInfo struct {
	typeName                string
	entityRootFieldRef      int
	entityInlineFragmentRef int
}

type federationConfigData struct {
	entityTypeName string
	keyFields      string
	requiredFields string
}

func newFederationConfigData(entityTypeName string) federationConfigData {
	return federationConfigData{
		entityTypeName: entityTypeName,
		keyFields:      "",
		requiredFields: "",
	}
}

type rpcPlanVisitorFederation struct {
	walker     *astvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
	planCtx    *rpcPlanningContext
	mapping    *GRPCMapping

	planInfo             planningInfo
	entityInfo           entityInfo
	federationConfigData []federationConfigData

	plan         *RPCExecutionPlan
	subgraphName string
	currentCall  *RPCCall

	callIndex int // global counter for all calls.
	// contains the indices of the resolver fields in the resolverFields slice
	fieldResolverAncestors stack[int]
	resolverFields         []resolverField

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
		federationConfigData:   parseFederationConfigData(config.federationConfigs),
		resolverFields:         make([]resolverField, 0),
		fieldResolverAncestors: newStack[int](0),
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
	if len(r.resolverFields) == 0 {
		return
	}

	calls, err := r.planCtx.createResolverRPCCalls(r.subgraphName, r.resolverFields)
	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	r.plan.Calls = append(r.plan.Calls, calls...)
	r.resolverFields = nil

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
	fc, ok := r.FederationConfigDataByEntityTypeName(fragmentName)
	if !ok {
		return
	}

	r.currentCall = &RPCCall{
		ID:          r.callIndex,
		ServiceName: r.planCtx.resolveServiceName(r.subgraphName),
		Kind:        CallKindEntity,
	}

	r.callIndex++

	r.planInfo.currentRequestMessage = &r.currentCall.Request
	r.planInfo.currentResponseMessage = &r.currentCall.Response

	r.entityInfo.entityInlineFragmentRef = ref
	r.entityInfo.typeName = fragmentName
	if err := r.resolveEntityInformation(ref, fc); err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	r.scaffoldEntityLookup(fc)
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
		operationType:            r.planInfo.operationType,
		operationFieldName:       r.planInfo.operationFieldName,
		currentRequestMessage:    &RPCMessage{},
		currentResponseMessage:   &RPCMessage{},
		responseMessageAncestors: []*RPCMessage{},
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
			resolvedField := &r.resolverFields[resolvedFieldAncestor]
			resolvedField.memberTypes = memberTypes
			r.planCtx.enterResolverCompositeSelectionSet(compositType, ref, resolvedField)
			return
		}

		r.resolverFields[resolvedFieldAncestor].fieldsSelectionSetRef = ref
		return
	}

	if r.planInfo.currentRequestMessage == nil || len(r.planInfo.currentResponseMessage.Fields) == 0 || r.walker.Ancestor().Kind != ast.NodeKindField {
		return
	}

	// We ignore selection sets from inline fragments or fragment spreads.
	lastIndex := len(r.planInfo.currentResponseMessage.Fields) - 1

	// In nested selection sets, a new message needs to be created, which will be added to the current response message.
	if r.planInfo.currentResponseMessage.Fields[lastIndex].Message == nil {
		r.planInfo.currentResponseMessage.Fields[lastIndex].Message = r.planCtx.newMessageFromSelectionSet(r.walker.EnclosingTypeDefinition, ref)
	}

	// Add the current response message to the ancestors and set the current response message to the current field message
	r.planInfo.responseMessageAncestors = append(r.planInfo.responseMessageAncestors, r.planInfo.currentResponseMessage)
	r.planInfo.currentResponseMessage = r.planInfo.currentResponseMessage.Fields[lastIndex].Message

	// Check if the ancestor type is a composite type (interface or union)
	// and set the oneof type and member types.
	if err := r.handleCompositeType(r.walker.Ancestor()); err != nil {
		// If the ancestor is a composite type, but we were unable to resolve the member types,
		// we stop the walker and return an internal error.
		r.walker.StopWithInternalErr(err)
		return
	}

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

	field, err := r.planCtx.buildField(r.walker.EnclosingTypeDefinition, fieldDefRef, fieldName, fieldAlias)
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
	}
}

// enterFieldResolver enters a field resolver.
// ref is the field reference in the operation document.
// fieldDefRef is the field definition reference in the definition document.
// TODO: extract to planCtx
func (r *rpcPlanVisitorFederation) enterFieldResolver(ref int, fieldDefRef int) {
	defaultContextPath := ast.Path{{Kind: ast.FieldName, FieldName: []byte("result")}}
	// Field arguments for non root types will be handled as resolver calls.
	// We need to make sure to handle a hierarchy of arguments in order to perform parallel calls in order to retrieve the data.
	fieldArgs := r.operation.FieldArguments(ref)
	// We don't want to add fields from the selection set to the actual call

	parentID := r.currentCall.ID
	fieldPath := r.fieldPath
	if r.fieldResolverAncestors.len() > 0 {
		fieldPath = r.resolverFields[r.fieldResolverAncestors.peek()].contextPath
		parentID = r.resolverFields[r.fieldResolverAncestors.peek()].id
	}

	resolvedField := resolverField{
		id:                     r.callIndex,
		callerRef:              parentID,
		parentTypeNode:         r.walker.EnclosingTypeDefinition,
		fieldRef:               ref,
		responsePath:           r.walker.Path[1:].WithoutInlineFragmentNames().WithFieldNameItem(r.operation.FieldAliasOrNameBytes(ref)),
		fieldDefinitionTypeRef: r.definition.FieldDefinitionType(fieldDefRef),
	}

	r.callIndex++

	if err := r.planCtx.setResolvedField(r.walker, fieldDefRef, fieldArgs, fieldPath, &resolvedField); err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	buf := bytes.Buffer{}
	buf.Write(bytes.Repeat([]byte("@"), resolvedField.listNestingLevel))
	buf.WriteString(r.planCtx.findResolverFieldMapping(
		r.walker.EnclosingTypeDefinition.NameString(r.definition),
		r.definition.FieldDefinitionNameString(fieldDefRef),
	))

	resolvedField.contextPath = defaultContextPath.WithFieldNameItem(buf.Bytes())

	r.resolverFields = append(r.resolverFields, resolvedField)
	r.fieldResolverAncestors.push(len(r.resolverFields) - 1)
	r.fieldPath = r.fieldPath.WithFieldNameItem(buf.Bytes())
}

func (r *rpcPlanVisitorFederation) resolveEntityInformation(inlineFragmentRef int, fc federationConfigData) error {
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

	rpcConfig, exists := r.mapping.FindEntityRPCConfig(fc.entityTypeName, fc.keyFields)
	if !exists {
		return fmt.Errorf("entity type %s not found in mapping", fc.entityTypeName)
	}

	r.currentCall.Request.Name = rpcConfig.Request
	r.currentCall.Response.Name = rpcConfig.Response
	r.currentCall.MethodName = rpcConfig.RPC

	return nil
}

// scaffoldEntityLookup creates the entity lookup call structure
// by creating the key field message and adding it to the current request message.
// It also adds the results message to the current response message.
func (r *rpcPlanVisitorFederation) scaffoldEntityLookup(fc federationConfigData) {
	keyFieldMessage := &RPCMessage{
		Name: r.currentCall.MethodName + "Key",
	}

	walker := astvisitor.WalkerFromPool()
	defer walker.Release()

	requiredFieldsVisitor := newRequiredFieldsVisitor(walker, keyFieldMessage, r.planCtx)
	err := requiredFieldsVisitor.visitWithDefaults(r.definition, fc.entityTypeName, fc.keyFields)
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

	entityMessage := &RPCMessage{
		Name: fc.entityTypeName,
		Fields: []RPCField{
			{
				Name:          "__typename",
				ProtoTypeName: DataTypeString,
				JSONPath:      "__typename",
				StaticValue:   fc.entityTypeName,
			},
		},
	}

	// The proto response message has a field `result` which is a list of entities.
	// As this is a special case we directly map it to _entities.
	r.planInfo.currentResponseMessage.Fields = []RPCField{
		{
			Name:          "result",
			ProtoTypeName: DataTypeMessage,
			JSONPath:      "_entities",
			Repeated:      true,
			Message:       entityMessage,
		},
	}

	r.planInfo.currentResponseMessage = entityMessage
}

// FederationConfigDataByEntityTypeName returns the entity config data for the given entity type name.
func (r *rpcPlanVisitorFederation) FederationConfigDataByEntityTypeName(entityTypeName string) (federationConfigData, bool) {
	for _, fc := range r.federationConfigData {
		if fc.entityTypeName == entityTypeName {
			return fc, true
		}
	}
	return federationConfigData{}, false
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

func parseFederationConfigData(federationConfigs plan.FederationFieldConfigurations) []federationConfigData {
	var out []federationConfigData

	typeNameIndexSet := map[string]int{}
	typeNameIndex := 0

	for _, fc := range federationConfigs {
		// Create a new entity type if it doesn't exist
		if _, ok := typeNameIndexSet[fc.TypeName]; !ok {
			out = append(out, newFederationConfigData(fc.TypeName))
			typeNameIndexSet[fc.TypeName] = typeNameIndex
			typeNameIndex++
		}

		data := &out[typeNameIndexSet[fc.TypeName]]

		// Selection set determines whether we have key fields or additional required fields
		if fc.SelectionSet == "" {
			continue
		}

		// This is a required field, so we add it to the required fields
		if fc.FieldName != "" {
			data.requiredFields = fc.SelectionSet
			continue
		}

		// This is a key field, so we add it to the key fields
		data.keyFields = fc.SelectionSet
	}

	return out
}
