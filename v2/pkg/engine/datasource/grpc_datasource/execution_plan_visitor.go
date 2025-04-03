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

	// requestMessageAncestors []*RPCMessage
	currentRequestMessage *RPCMessage

	// responseMessageAncestors []*RPCMessage
	currentResponseMessage *RPCMessage

	currentFieldIndex int
	indexAncestors    []int
}

var _ astvisitor.EnterDocumentVisitor = &rpcPlanVisitor{}
var _ astvisitor.FieldVisitor = &rpcPlanVisitor{}
var _ astvisitor.EnterOperationDefinitionVisitor = &rpcPlanVisitor{}
var _ astvisitor.EnterSelectionSetVisitor = &rpcPlanVisitor{}

type rpcPlanVisitor struct {
	planInfo planningInfo

	subgraphName      string
	plan              *RPCExecutionPlan
	currentGroupIndex int
	currentCall       *RPCCall
	currentCallID     int
	walker            *astvisitor.Walker
	operation         *ast.Document
	definition        *ast.Document
	// currentRequestMessage  *RPCMessage
	// currentResponseMessage *RPCMessage
}

// NewRPCPlanVisitor creates a new RPCPlanVisitor
// It registers the visitor with the walker and returns it
func NewRPCPlanVisitor(walker *astvisitor.Walker, subgraphName string) *rpcPlanVisitor {
	visitor := &rpcPlanVisitor{
		walker:       walker,
		plan:         &RPCExecutionPlan{},
		subgraphName: strings.Title(subgraphName),
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterOperationDefinitionVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterInlineFragmentVisitor(visitor)

	return visitor
}

// EnterDocument implements astvisitor.EnterDocumentVisitor.
func (r *rpcPlanVisitor) EnterDocument(operation *ast.Document, definition *ast.Document) {
	r.operation = operation
	r.definition = definition
}

// EnterOperationDefinition implements astvisitor.EnterOperationDefinitionVisitor.
func (r *rpcPlanVisitor) EnterOperationDefinition(ref int) {
	if r.operation == nil {
		return
	}

	selectionSetRef := r.operation.OperationDefinitions[ref].SelectionSet
	fieldNames := r.operation.SelectionSetFieldNames(selectionSetRef)

	r.planInfo.operationFieldName = fieldNames[r.currentCallID]

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
}

// EnterSelectionSet implements astvisitor.EnterSelectionSetVisitor.
func (r *rpcPlanVisitor) EnterSelectionSet(ref int) {
	// r.planInfo.indexAncestors = append(r.planInfo.indexAncestors, r.planInfo.currentFieldIndex)
	r.planInfo.currentFieldIndex = 0 // reset the field index for the current selection set

	if r.walker.Ancestor().Kind == ast.NodeKindOperationDefinition {
		r.plan.Groups = append(r.plan.Groups, RPCCallGroup{
			Calls: []RPCCall{},
		})

		r.currentGroupIndex = len(r.plan.Groups) - 1

		r.currentCall = &RPCCall{
			CallID:      r.currentCallID,
			ServiceName: r.subgraphName,
		}

		r.planInfo.currentRequestMessage = &r.currentCall.Request
		r.planInfo.currentResponseMessage = &r.currentCall.Response

		// we are in the root level below the operation definition.
		// We only scaffold the call here.
		return
	}
}

// LeaveSelectionSet implements astvisitor.SelectionSetVisitor.
func (r *rpcPlanVisitor) LeaveSelectionSet(ref int) {
	if len(r.planInfo.indexAncestors) > 0 {
		r.planInfo.currentFieldIndex = r.planInfo.indexAncestors[len(r.planInfo.indexAncestors)-1]
		r.planInfo.indexAncestors = r.planInfo.indexAncestors[:len(r.planInfo.indexAncestors)-1]
	}

	// if len(r.planInfo.responseMessageAncestors) > 0 {
	// 	r.planInfo.currentResponseMessage = r.planInfo.responseMessageAncestors[len(r.planInfo.responseMessageAncestors)-1]
	// 	r.planInfo.responseMessageAncestors = r.planInfo.responseMessageAncestors[:len(r.planInfo.responseMessageAncestors)-1]
	// }

	anchestor := r.walker.Ancestor()
	if anchestor.Kind == ast.NodeKindOperationDefinition {
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
		Index:    r.planInfo.currentFieldIndex,
	})
}

// LeaveField implements astvisitor.FieldVisitor.
func (r *rpcPlanVisitor) LeaveField(ref int) {
	if ref == r.planInfo.entityInfo.entityRootFieldRef {
		r.planInfo.entityInfo.entityRootFieldRef = -1
	}

	r.planInfo.currentFieldIndex++
}

func (r *rpcPlanVisitor) resolveEntityInformation(inlineFragmentRef int) {
	// TODO we need to support multiple entities in a single query
	if !r.planInfo.isEntityLookup || r.planInfo.entityInfo.name != "" {
		return
	}

	fragmentName := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
	node, found := r.definition.NodeByNameStr(fragmentName)
	if !found {
		return
	}

	// we don't care about other node types
	if node.Kind != ast.NodeKindObjectTypeDefinition {
		return
	}

	// An entity must at least have a key directive
	def := r.definition.ObjectTypeDefinitions[node.Ref]
	if !def.HasDirectives {
		return
	}

	for _, directiveRef := range def.Directives.Refs {
		if r.definition.DirectiveNameString(directiveRef) != FederationKeyDirectiveName {
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

// scaffoldEntityLookup scaffolds the entity lookup call
// it creates the key field message and adds it to the current request message
// it also adds the results message to the current response message
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
			JSONPath: "variables.representations",
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

	// r.planInfo.requestMessageAncestors = append(r.planInfo.requestMessageAncestors, keyFieldMessage)
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

	// r.planInfo.responseMessageAncestors = append(r.planInfo.responseMessageAncestors, resultMessage)
	r.planInfo.currentResponseMessage = resultMessage
}

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

func (r *rpcPlanVisitor) buildQueryMethodName() string {
	if r.planInfo.isEntityLookup && r.planInfo.entityInfo.name != "" {

		return "Lookup" + r.planInfo.entityInfo.keyTypeName
	}

	return "Query" + strings.Title(r.planInfo.operationFieldName)
}

func (r *rpcPlanVisitor) buildMutationMethodName() string {
	// TODO implement mutation method name handling
	return "Mutation" + strings.Title(r.planInfo.operationFieldName)
}

func (r *rpcPlanVisitor) buildSubscriptionMethodName() string {
	// TODO implement subscription method name handling
	return "Subscription" + strings.Title(r.planInfo.operationFieldName)
}

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

func (r *rpcPlanVisitor) parseGraphQLType(t *ast.Type) DataType {
	dt := r.definition.Input.ByteSliceString(t.Name)

	return fromGraphQLType(dt)
}

func titleSlice(s []string) []string {
	for i, v := range s {
		s[i] = strings.Title(v)
	}
	return s
}
