package grpcdatasource

import (
	"errors"
	"fmt"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type planningInfo struct {
	operationType      ast.OperationType
	operationFieldName string

	currentRequestMessage *RPCMessage

	responseMessageAncestors  []*RPCMessage
	currentResponseMessage    *RPCMessage
	currentResponseFieldIndex int

	responseFieldIndexAncestors []int
}

type contextField struct {
	fieldRef    int
	resolvePath ast.Path
}

type fieldArgument struct {
	parentTypeNode        ast.Node
	jsonPath              string
	fieldDefinitionRef    int
	argumentDefinitionRef int
}

type resolvedField struct {
	callerRef     int
	parentTypeRef int
	responsePath  ast.Path

	contextFields  []contextField
	fieldArguments []fieldArgument
}

type rpcPlanVisitor struct {
	walker     *astvisitor.Walker
	operation  *ast.Document
	definition *ast.Document
	planInfo   planningInfo
	planCtx    *rpcPlanningContext

	subgraphName       string
	mapping            *GRPCMapping
	plan               *RPCExecutionPlan
	operationFieldRef  int
	operationFieldRefs []int
	currentCall        *RPCCall
	currentCallID      int

	relatedCallID  int
	resolvedFields []resolvedField
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
		resolvedFields:    make([]resolvedField, 0),
		relatedCallID:     -1,
	}

	walker.RegisterDocumentVisitor(visitor)
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

// LeaveDocument implements astvisitor.DocumentVisitor.
func (r *rpcPlanVisitor) LeaveDocument(_, _ *ast.Document) {
	if len(r.resolvedFields) == 0 {
		return
	}

	// We need to create a new call for each resolved field.
	calls := make([]RPCCall, 0, len(r.resolvedFields))

	for _, resolvedField := range r.resolvedFields {

		contextMessage := &RPCMessage{
			Name: "CategoryProductCountContext",
		}

		fieldArgsMessage := &RPCMessage{
			Name: "CategoryProductCountArgs",
		}

		// Base resolve call can be templated in plan context.
		call := RPCCall{
			DependentCalls: []int{resolvedField.callerRef},
			ResponsePath:   resolvedField.responsePath,
			ServiceName:    r.planCtx.resolveServiceName(r.subgraphName),
			MethodName:     "ResolveCategoryProductCount",
			Kind:           CallKindResolve,
			Request: RPCMessage{
				Name: "ResolveCategoryProductCountRequest",
				Fields: RPCFields{
					{
						Name:     "context",
						TypeName: string(DataTypeMessage),
						JSONPath: "",
						Repeated: true,
						Message:  contextMessage,
					},
					{
						Name:     "field_args",
						TypeName: string(DataTypeMessage),
						JSONPath: "",
						Message:  fieldArgsMessage,
					},
				},
			},
			Response: RPCMessage{
				Name: "ResolveCategoryProductCountResponse",
				Fields: RPCFields{
					{
						Name:     "result",
						TypeName: string(DataTypeMessage),
						JSONPath: "result",
						Repeated: true,
						Message: &RPCMessage{
							Name: "ResolveCategoryProductCountResponseResult",
							Fields: RPCFields{
								{
									Name:     "product_count",
									TypeName: string(DataTypeInt32),
									JSONPath: "productCount",
								},
							},
						},
					},
				},
			},
		}

		contextMessage.Fields = make(RPCFields, 0, len(resolvedField.contextFields))
		for _, contextField := range resolvedField.contextFields {
			typeDefNode, found := r.definition.NodeByNameStr(r.definition.ResolveTypeNameString(resolvedField.parentTypeRef))
			if !found {
				r.walker.StopWithInternalErr(fmt.Errorf("type definition node not found for type: %s", r.definition.ResolveTypeNameString(resolvedField.parentTypeRef)))
				return
			}

			field, err := r.planCtx.buildField(
				typeDefNode,
				contextField.fieldRef,
				r.definition.FieldDefinitionNameString(contextField.fieldRef),
				"",
			)

			field.ResolvePath = contextField.resolvePath

			if err != nil {
				r.walker.StopWithInternalErr(err)
				return
			}

			contextMessage.Fields = append(contextMessage.Fields, field)
		}

		fieldArgsMessage.Fields = make(RPCFields, 0, len(resolvedField.fieldArguments))
		for _, fieldArgument := range resolvedField.fieldArguments {
			field, err := r.planCtx.createRPCFieldFromFieldArgument(fieldArgument)

			if err != nil {
				r.walker.StopWithInternalErr(err)
				return
			}

			fieldArgsMessage.Fields = append(fieldArgsMessage.Fields, field)
		}

		calls = append(calls, call)
	}

	r.plan.Calls = append(r.plan.Calls, calls...)
	r.resolvedFields = nil
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
	ancestor := r.walker.Ancestor()
	if ancestor.Kind != ast.NodeKindField || ancestor.Ref != r.operationFieldRef {
		return
	}
	argumentInputValueDefinitionRef, exists := r.walker.ArgumentInputValueDefinition(ref)
	if !exists {
		return
	}

	// As we check that we are inside of a field we can safely access the second to last type definition.
	parentTypeNode := r.walker.TypeDefinitions[len(r.walker.TypeDefinitions)-2]
	fieldDefinitionRef, exists := r.definition.NodeFieldDefinitionByName(parentTypeNode, r.operation.FieldNameBytes(ancestor.Ref))
	if !exists {
		return
	}

	argument := r.operation.ArgumentValue(ref)
	jsonPath := r.operation.ArgumentNameString(ref)
	if argument.Kind == ast.ValueKindVariable {
		jsonPath = r.operation.Input.ByteSliceString(r.operation.VariableValues[argument.Ref].Name)
	}

	// Retrieve the type of the input value definition, and build the request message
	field, err := r.planCtx.createRPCFieldFromFieldArgument(fieldArgument{
		fieldDefinitionRef:    fieldDefinitionRef,
		parentTypeNode:        parentTypeNode,
		argumentDefinitionRef: argumentInputValueDefinitionRef,
		jsonPath:              jsonPath,
	})

	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	r.planInfo.currentRequestMessage.Fields = append(r.planInfo.currentRequestMessage.Fields, field)

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

func (r *rpcPlanVisitor) handleRootField(isRootField bool, ref int) error {
	if !isRootField {
		return nil
	}

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
	inRootField := r.walker.InRootField()
	if err := r.handleRootField(inRootField, ref); err != nil {
		r.walker.StopWithInternalErr(err)
		return
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

	// Field arguments for non root types will be handled as resolver calls.
	// We need to make sure to handle a hierarchy of arguments in order to perform parallel calls in order to retrieve the data.
	// TODO: this needs to be available for both visitors and added to the plancontext
	if fieldArgs := r.operation.FieldArguments(ref); !inRootField && len(fieldArgs) > 0 {
		r.relatedCallID++
		resolvedField := resolvedField{
			callerRef:     r.relatedCallID,
			parentTypeRef: r.walker.EnclosingTypeDefinition.Ref,
			responsePath: r.walker.Path[1:].WithoutInlineFragmentNames().WithPathElement(ast.PathItem{
				Kind:      ast.FieldName,
				FieldName: r.operation.FieldAliasOrNameBytes(ref),
			}),
		}

		contextFields, err := r.planCtx.resolveContextFields(r.walker, fd)
		if err != nil {
			r.walker.StopWithInternalErr(err)
			return
		}

		for _, contextFieldRef := range contextFields {
			contextFieldName := r.definition.FieldDefinitionNameBytes(contextFieldRef) // TODO handle aliases
			resolvedPath := append(r.walker.Path[1:].WithoutInlineFragmentNames(), ast.PathItem{
				Kind:      ast.FieldName,
				FieldName: contextFieldName,
			})

			resolvedField.contextFields = append(resolvedField.contextFields, contextField{
				fieldRef:    contextFieldRef,
				resolvePath: resolvedPath,
			})
		}

		fieldArguments, err := r.planCtx.parseFieldArguments(r.walker, fd, fieldArgs)
		if err != nil {
			r.walker.StopWithInternalErr(err)
			return
		}

		resolvedField.fieldArguments = fieldArguments
		r.resolvedFields = append(r.resolvedFields, resolvedField)
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
		// If the field has arguments, we need to decrement the related call ID.
		// This is because we can also have nested arguments, which require the underlying field to be resolved
		// by values provided by the parent call.
		if r.operation.FieldHasArguments(ref) {
			r.relatedCallID--
		}

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
