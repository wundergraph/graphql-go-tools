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

	plan             *RPCExecutionPlan
	subgraphName     string
	currentCall      *RPCCall
	currentCallIndex int
}

func newRPCPlanVisitorFederation(config rpcPlanVisitorConfig) *rpcPlanVisitorFederation {
	walker := astvisitor.NewWalker(48)
	visitor := &rpcPlanVisitorFederation{
		walker:       &walker,
		plan:         &RPCExecutionPlan{},
		subgraphName: cases.Title(language.Und, cases.NoLower).String(config.subgraphName),
		mapping:      config.mapping,
		entityInfo: entityInfo{
			entityRootFieldRef:      -1,
			entityInlineFragmentRef: -1,
		},
		federationConfigData: parseFederationConfigData(config.federationConfigs),
	}

	walker.RegisterEnterDocumentVisitor(visitor)
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
		ServiceName: r.planCtx.resolveServiceName(r.subgraphName),
	}

	r.planInfo.currentRequestMessage = &r.currentCall.Request
	r.planInfo.currentResponseMessage = &r.currentCall.Response

	r.entityInfo.entityInlineFragmentRef = ref
	r.entityInfo.typeName = fragmentName
	r.resolveEntityInformation(ref, fc)
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
	r.currentCallIndex++

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
	if r.walker.InRootField() {
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
	// If we are not in the operation field, we can increment the response field index.
	if !r.walker.InRootField() {
		r.planInfo.currentResponseFieldIndex++
		return
	}

	r.planInfo.currentResponseFieldIndex = 0
}

func (r *rpcPlanVisitorFederation) resolveEntityInformation(inlineFragmentRef int, fc federationConfigData) {
	fragmentName := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
	node, found := r.definition.NodeByNameStr(r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef))
	if !found {
		r.walker.StopWithInternalErr(errors.New("definition node not found for inline fragment: " + fragmentName))
		return
	}

	// Only process object type definitions
	// TODO: handle interfaces
	if node.Kind != ast.NodeKindObjectTypeDefinition {
		return
	}

	rpcConfig, exists := r.mapping.ResolveEntityRPCConfig(fc.entityTypeName, fc.keyFields)
	if !exists {
		return
	}

	r.currentCall.Request.Name = rpcConfig.Request
	r.currentCall.Response.Name = rpcConfig.Response
	r.currentCall.MethodName = rpcConfig.RPC
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
	err := requiredFieldsVisitor.visitRequiredFields(r.definition, fc.entityTypeName, fc.keyFields)
	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
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

	// The proto response message has a field `result` which is a list of entities.
	// As this is a special case we directly map it to _entities.
	r.planInfo.currentResponseMessage.Fields = []RPCField{
		{
			Name:     "result",
			TypeName: DataTypeMessage.String(),
			JSONPath: "_entities",
			Repeated: true,
		},
	}
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

func (r *rpcPlanVisitorFederation) IsEntityInlineFragment(ref int) bool {
	if r.entityInfo.entityInlineFragmentRef == ast.InvalidRef {
		return false
	}

	return r.entityInfo.entityInlineFragmentRef == ref
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
