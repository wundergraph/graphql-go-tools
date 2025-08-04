package grpcdatasource

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const (
	federationKeyDirectiveName = "key"
	// knownTypeOptionalFieldValueName is the name of the field that is used to wrap optional scalar values
	// in a message as protobuf scalar types are not nullable.
	knownTypeOptionalFieldValueName = "value"
)

// OneOfType represents the type of a oneof field in a protobuf message.
// It can be either an interface or a union type.
type OneOfType uint8

// OneOfType constants define the different types of oneof fields.
const (
	// OneOfTypeNone represents no oneof type (default/zero value)
	OneOfTypeNone OneOfType = iota
	// OneOfTypeInterface represents an interface type oneof field
	OneOfTypeInterface
	// OneOfTypeUnion represents a union type oneof field
	OneOfTypeUnion
)

// FieldName returns the corresponding field name for the OneOfType.
// For interfaces, it returns "instance", for unions it returns "value".
// Returns an empty string for invalid or unknown types.
func (o OneOfType) FieldName() string {
	switch o {
	case OneOfTypeInterface:
		return "instance"
	case OneOfTypeUnion:
		return "value"
	}

	return ""
}

// RPCExecutionPlan represents a plan for executing one or more RPC calls
// to gRPC services. It defines the sequence of calls and their dependencies.
type RPCExecutionPlan struct {
	// Calls is a list of gRPC calls that are executed in the same group
	Calls []RPCCall
	// TODO add mapping to the execution plan
	// instead of the planner and the compiler?
}

// RPCCall represents a single call to a gRPC service method.
// It contains all the information needed to make the call and process the response.
type RPCCall struct {
	// DependentCalls is a list of calls that must be executed before this call
	DependentCalls []int
	// ServiceName is the name of the gRPC service to call
	ServiceName string
	// MethodName is the name of the method on the service to call
	MethodName string
	// Request contains the message structure for the gRPC request
	Request RPCMessage
	// Response contains the message structure for the gRPC response
	Response RPCMessage
}

// RPCMessage represents a gRPC message structure for requests and responses.
// It defines the structure of the message including all its fields.
type RPCMessage struct {
	// Name is the name of the message type in the protobuf definition
	Name string
	// Fields is a list of fields in the message
	Fields RPCFields
	// FieldSelectionSet are field selections based on inline fragments
	FieldSelectionSet RPCFieldSelectionSet
	// OneOfType indicates the type of the oneof field
	OneOfType OneOfType
	// MemberTypes provides the names of the types that are implemented by the Interface or Union
	MemberTypes []string
	// Message represents the nested message type definition for complex fields.
	// This enables recursive construction of nested protobuf message structures.
	Message *RPCMessage
}

// IsOneOf checks if the message is a oneof field.
func (r *RPCMessage) IsOneOf() bool {
	switch r.OneOfType {
	case OneOfTypeInterface, OneOfTypeUnion:
		return true
	}

	return false
}

// SelectValidTypes returns the valid types for a given type name.
func (r *RPCMessage) SelectValidTypes(typeName string) []string {
	if r.Name == typeName {
		return []string{r.Name}
	}

	// If we have an interface or union type, we need to select the provided type as well.
	return []string{r.Name, typeName}
}

// RPCFieldSelectionSet is a map of field selections based on inline fragments
type RPCFieldSelectionSet map[string]RPCFields

// Add adds a field selection set to the map
func (r RPCFieldSelectionSet) Add(fragmentName string, field RPCField) {
	if r[fragmentName] == nil {
		r[fragmentName] = make(RPCFields, 0)
	}

	r[fragmentName] = append(r[fragmentName], field)
}

// SelectFieldsForTypes returns the fields for the given valid types.
// It also makes sure to deduplicate the fields.
func (r RPCFieldSelectionSet) SelectFieldsForTypes(validTypes []string) RPCFields {
	fieldSet := make(map[string]struct{})
	fields := make(RPCFields, 0)
	for _, typeName := range validTypes {
		lookupFields, ok := r[typeName]
		if !ok {
			continue
		}

		for _, field := range lookupFields {
			if _, found := fieldSet[field.AliasOrPath()]; found {
				continue
			}

			fieldSet[field.AliasOrPath()] = struct{}{}
			fields = append(fields, field)
		}
	}

	return fields
}

// RPCField represents a single field in a gRPC message.
// It contains all information required to extract data from GraphQL variables
// and construct the appropriate protobuf field.
type RPCField struct {
	// Alias can be used to rename the field in the request message
	// This is needed to make sure that during the json composition,
	// the field names match the GraphQL request naming.
	// TODO implement alias handling
	Alias string
	// Repeated indicates if the field is a repeated field (array/list)
	Repeated bool
	// Name is the name of the field as defined in the protobuf message
	Name string
	// TypeName is the name of the type of the field in the protobuf definition
	TypeName string
	// JSONPath defines the path within the variables to provide the value for the field
	// This is used to extract data from the GraphQL variables
	JSONPath string
	// EnumName is the name of the enum if the field is an enum type
	EnumName string
	// StaticValue is the static value of the field
	StaticValue string
	// Optional indicates if the field is optional
	Optional bool
	// IsListType indicates if the field is a list wrapper type
	IsListType bool
	// ListMetadata contains the metadata for the list type
	ListMetadata *ListMetadata
	// Message represents the nested message type definition for complex fields.
	// This enables recursive construction of nested protobuf message structures.
	Message *RPCMessage
}

// ListMetadata contains the metadata for the list type
type ListMetadata struct {
	// NestingLevel is the nesting level of the list type
	NestingLevel int
	// LevelInfo contains the metadata for each nesting level of the list
	LevelInfo []LevelInfo
}

// LevelInfo contains the metadata for the list type
type LevelInfo struct {
	// Optional indicates if the field is optional
	Optional bool
}

// ToOptionalTypeMessage returns a message that wraps the scalar value in a message
// as protobuf scalar types are not nullable.
func (r *RPCField) ToOptionalTypeMessage(protoName string) *RPCMessage {
	if r == nil {
		return nil
	}

	return &RPCMessage{
		Name: protoName,
		Fields: RPCFields{
			RPCField{
				Name:     knownTypeOptionalFieldValueName,
				JSONPath: r.JSONPath,
				TypeName: r.TypeName,
				Repeated: r.Repeated,
				EnumName: r.EnumName,
			},
		},
	}
}

// AliasOrPath returns the alias of the field if it exists, otherwise it returns the JSONPath.
func (r *RPCField) AliasOrPath() string {
	if r.Alias != "" {
		return r.Alias
	}

	return r.JSONPath
}

// IsOptionalScalar checks if the field is an optional scalar value.
func (r *RPCField) IsOptionalScalar() bool {
	return r.Optional && r.TypeName != string(DataTypeMessage)
}

// RPCFields is a list of RPCFields that provides helper methods
// for working with collections of fields.
type RPCFields []RPCField

// ByName returns a field by its name from the collection of fields.
// Returns nil if no field with the given name exists.
func (r RPCFields) ByName(name string) *RPCField {
	for _, field := range r {
		if field.Name == name {
			return &field
		}
	}

	return nil
}

func (r RPCFields) Exists(name, alias string) bool {
	for _, field := range r {
		if field.Name == name && field.Alias == alias {
			return true
		}
	}

	return false
}

func (r *RPCExecutionPlan) String() string {
	var result strings.Builder

	result.WriteString("RPCExecutionPlan:\n")

	for j, call := range r.Calls {
		result.WriteString(fmt.Sprintf("    Call %d:\n", j))

		if len(call.DependentCalls) > 0 {
			result.WriteString("      DependentCalls: [")
			for k, depID := range call.DependentCalls {
				if k > 0 {
					result.WriteString(", ")
				}
				result.WriteString(fmt.Sprintf("%d", depID))
			}
			result.WriteString("]\n")
		} else {
			result.WriteString("      DependentCalls: []\n")
		}

		result.WriteString(fmt.Sprintf("      Service: %s\n", call.ServiceName))
		result.WriteString(fmt.Sprintf("      Method: %s\n", call.MethodName))

		result.WriteString("      Request:\n")
		formatRPCMessage(&result, call.Request, 8)

		result.WriteString("      Response:\n")
		formatRPCMessage(&result, call.Response, 8)
	}

	return result.String()
}

type PlanVisitor interface {
	ExecutionPlan() *RPCExecutionPlan
}

type Planner struct {
	visitor PlanVisitor
	walker  *astvisitor.Walker
}

// NewPlanner returns a new Planner instance.
//
// The planner is responsible for creating an RPCExecutionPlan from a given
// GraphQL operation. It is used by the engine to execute operations against
// gRPC services.
func NewPlanner(subgraphName string, mapping *GRPCMapping, federationConfigs plan.FederationFieldConfigurations) *Planner {
	walker := astvisitor.NewWalker(48)

	if mapping == nil {
		mapping = new(GRPCMapping)
	}

	var visitor PlanVisitor
	if len(federationConfigs) > 0 {
		visitor = newRPCPlanVisitorFederation(&walker, rpcPlanVisitorConfig{
			subgraphName:      subgraphName,
			mapping:           mapping,
			federationConfigs: federationConfigs,
		})
	} else {
		visitor = newRPCPlanVisitor(&walker, rpcPlanVisitorConfig{
			subgraphName:      subgraphName,
			mapping:           mapping,
			federationConfigs: federationConfigs,
		})
	}

	return &Planner{
		visitor: visitor,
		walker:  &walker,
	}
}

func (p *Planner) PlanOperation(operation, definition *ast.Document) (*RPCExecutionPlan, error) {
	report := &operationreport.Report{}
	p.walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return nil, fmt.Errorf("unable to plan operation: %w", report)
	}

	return p.visitor.ExecutionPlan(), nil
}

// formatRPCMessage formats an RPCMessage and adds it to the string builder with the specified indentation
func formatRPCMessage(sb *strings.Builder, message RPCMessage, indent int) {
	indentStr := strings.Repeat(" ", indent)

	sb.WriteString(fmt.Sprintf("%sName: %s\n", indentStr, message.Name))
	sb.WriteString(fmt.Sprintf("%sFields:\n", indentStr))

	for _, field := range message.Fields {
		sb.WriteString(fmt.Sprintf("%s  - Name: %s\n", indentStr, field.Name))
		sb.WriteString(fmt.Sprintf("%s    TypeName: %s\n", indentStr, field.TypeName))
		sb.WriteString(fmt.Sprintf("%s    Repeated: %v\n", indentStr, field.Repeated))
		sb.WriteString(fmt.Sprintf("%s    JSONPath: %s\n", indentStr, field.JSONPath))

		if field.Message != nil {
			sb.WriteString(fmt.Sprintf("%s    Message:\n", indentStr))
			formatRPCMessage(sb, *field.Message, indent+6)
		}
	}
}

type rpcPlanningContext struct {
	operation  *ast.Document
	definition *ast.Document
	mapping    *GRPCMapping
}

func newRPCPlanningContext(operation *ast.Document, definition *ast.Document, mapping *GRPCMapping) *rpcPlanningContext {
	return &rpcPlanningContext{
		operation:  operation,
		definition: definition,
		mapping:    mapping,
	}
}

// toDataType converts an ast.Type to a DataType.
// It handles the different type kinds and non-null types.
func (r *rpcPlanningContext) toDataType(t *ast.Type) DataType {
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
func (r *rpcPlanningContext) parseGraphQLType(t *ast.Type) DataType {
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

func (r *rpcPlanningContext) resolveRPCMethodMapping(operationType ast.OperationType, operationFieldName string) (RPCConfig, error) {
	if r.mapping == nil {
		return RPCConfig{}, nil
	}

	var rpcConfig RPCConfig
	var ok bool

	switch operationType {
	case ast.OperationTypeQuery:
		rpcConfig, ok = r.mapping.QueryRPCs[operationFieldName]
	case ast.OperationTypeMutation:
		rpcConfig, ok = r.mapping.MutationRPCs[operationFieldName]
	case ast.OperationTypeSubscription:
		rpcConfig, ok = r.mapping.SubscriptionRPCs[operationFieldName]
	}

	if !ok {
		return RPCConfig{}, nil
	}

	// We require all fields to be present when defining a mapping for the operation
	if rpcConfig.RPC == "" {
		return RPCConfig{}, fmt.Errorf("no rpc method name mapping found for operation %s", operationFieldName)
	}

	if rpcConfig.Request == "" {
		return RPCConfig{}, fmt.Errorf("no request message name mapping found for operation %s", operationFieldName)
	}

	if rpcConfig.Response == "" {
		return RPCConfig{}, fmt.Errorf("no response message name mapping found for operation %s", operationFieldName)
	}

	return rpcConfig, nil
}

// newMessageFromSelectionSet creates a new message from the enclosing type node and the selection set reference.
func (r *rpcPlanningContext) newMessageFromSelectionSet(enclosingTypeNode ast.Node, selectSetRef int) *RPCMessage {
	message := &RPCMessage{
		Name:   enclosingTypeNode.NameString(r.definition),
		Fields: make(RPCFields, 0, len(r.operation.SelectionSets[selectSetRef].SelectionRefs)),
	}

	return message
}

// resolveFieldMapping resolves the field mapping for a field.
// This applies both for complex types in the input and for all fields in the response.
func (r *rpcPlanningContext) resolveFieldMapping(typeName, fieldName string) string {
	grpcFieldName, ok := r.mapping.ResolveFieldMapping(typeName, fieldName)
	if !ok {
		return fieldName
	}

	return grpcFieldName
}

func (r *rpcPlanningContext) typeIsNullableOrNestedList(typeRef int) bool {
	if !r.definition.TypeIsNonNull(typeRef) && r.definition.TypeIsList(typeRef) {
		return true
	}

	if r.definition.TypeNumberOfListWraps(typeRef) > 1 {
		return true
	}

	return false
}

func (r *rpcPlanningContext) createListMetadata(typeRef int) (*ListMetadata, error) {
	nestingLevel := r.definition.TypeNumberOfListWraps(typeRef)

	md := &ListMetadata{
		NestingLevel: nestingLevel,
		LevelInfo:    make([]LevelInfo, nestingLevel),
	}

	for i := 0; i < nestingLevel; i++ {
		md.LevelInfo[i] = LevelInfo{
			Optional: !r.definition.TypeIsNonNull(typeRef),
		}

		typeRef = r.definition.ResolveNestedListOrListType(typeRef)
		if typeRef == ast.InvalidRef {
			return nil, fmt.Errorf("unable to resolve underlying list type for ref: %d", typeRef)
		}
	}

	return md, nil
}

// buildField builds a field from a field definition.
// It handles lists, enums, and other types.
func (r *rpcPlanningContext) buildField(enclosingTypeNode ast.Node, fd int, fieldName, fieldAlias string) (RPCField, error) {
	fdt := r.definition.FieldDefinitionType(fd)
	typeName := r.toDataType(&r.definition.Types[fdt])
	parentTypeName := enclosingTypeNode.NameString(r.definition)

	field := RPCField{
		Name:     r.resolveFieldMapping(parentTypeName, fieldName),
		Alias:    fieldAlias,
		Optional: !r.definition.TypeIsNonNull(fdt),
		JSONPath: fieldName,
		TypeName: typeName.String(),
	}

	if r.definition.TypeIsList(fdt) {
		switch {
		// for nullable or nested lists we need to build a wrapper message
		// Nullability is handled by the datasource during the execution.
		case r.typeIsNullableOrNestedList(fdt):
			md, err := r.createListMetadata(fdt)
			if err != nil {
				return field, err
			}
			field.ListMetadata = md
			field.IsListType = true
		default:
			// For non-nullable single lists we can directly use the repeated syntax in protobuf.
			field.Repeated = true
		}
	}

	if typeName == DataTypeEnum {
		field.EnumName = r.definition.FieldDefinitionTypeNameString(fd)
	}

	if fieldName == "__typename" {
		field.StaticValue = parentTypeName
	}

	return field, nil
}

func (r *rpcPlanningContext) ensureRequiredFields(message *RPCMessage, fc *federationConfigData) error {
	// If the message is nil, we can't add any fields to it.
	if message == nil {
		return nil
	}

	walker := astvisitor.WalkerFromPool()
	defer walker.Release()

	requiredFieldsVisitor := newRequiredFieldsVisitor(walker, message, r)
	return requiredFieldsVisitor.visitRequiredFields(r.definition, fc.entityTypeName, fc.requiredFields)
}

func (r *rpcPlanningContext) resolveServiceName(subgraphName string) string {
	if r.mapping == nil || r.mapping.Service == "" {
		return subgraphName
	}

	return r.mapping.Service
}
