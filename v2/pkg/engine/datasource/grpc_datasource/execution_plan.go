package grpcdatasource

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

const (
	// knownTypeOptionalFieldValueName is the name of the field that is used to wrap optional scalar values
	// in a message as protobuf scalar types are not nullable.
	knownTypeOptionalFieldValueName = "value"

	// fieldResolverDirectiveName is the name of the directive that is used to configure the resolver context.
	fieldResolverDirectiveName = "connect__fieldResolver"

	// requiresDirectiveName specifies the name of the @requires federation directive.
	requiresDirectiveName = "requires"

	// typenameFieldName is the name of the field that is used to store the typename of the object.
	typenameFieldName = "__typename"
)

const (
	// resultFieldName is the name of the field that is used to store the result of the RPC call.
	resultFieldName = "result"
	// contextFieldName is the name of the field that is used to store the context of the RPC call.
	contextFieldName = "context"
	// fieldArgsFieldName is the name of the field that is used to store the field arguments of the RPC call.
	fieldArgsFieldName = "field_args"
	// requiresArgumentsFieldName is the name of the field that is used to store the required fields arguments of the RPC call.
	requiresArgumentsFieldName = "fields"
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

// CallKind is the type of call operation to perform.
type CallKind uint8

const (
	// CallKindStandard is a basic fetch operation.
	CallKindStandard CallKind = iota
	// CallKindEntity is a fetch operation for entities.
	CallKindEntity
	// CallKindResolve is a fetch operation for resolving field values.
	CallKindResolve
	// CallKindRequired is a fetch operation which is similar to Resolve, but it can be executed in parallel.
	// Required fields are indicated by the @requires federation directive.
	CallKindRequired
)

// RPCCall represents a single call to a gRPC service method.
// It contains all the information needed to make the call and process the response.
type RPCCall struct {
	// ID indicates the expected index of the call in the execution plan
	ID int
	// Kind of call, used to decide how to execute the call
	// This is used to identify the call type and execution behaviour.
	Kind CallKind
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
	// ResponsePath is the path to the response in the JSON response
	ResponsePath ast.Path
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
func (r RPCFieldSelectionSet) Add(fragmentName string, field ...RPCField) {
	r[fragmentName] = append(r[fragmentName], field...)
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
	Alias string
	// Repeated indicates if the field is a repeated field (array/list)
	Repeated bool
	// Name is the name of the field as defined in the protobuf message
	Name string
	// ProtoTypeName is the name of the type of the field in the protobuf definition
	ProtoTypeName DataType
	// JSONPath either holds the path to the variable definition for the request message,
	// or defines the name of the response field in the message.
	JSONPath string
	// ResolvePath is used to resolve values from another message.
	ResolvePath ast.Path
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
				Name:          knownTypeOptionalFieldValueName,
				JSONPath:      r.JSONPath,
				ProtoTypeName: r.ProtoTypeName,
				Repeated:      r.Repeated,
				EnumName:      r.EnumName,
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
	return r.Optional && r.ProtoTypeName != DataTypeMessage
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

// Exists checks if a field with the given name and alias exists in the collection of fields.
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

	for _, call := range r.Calls {
		result.WriteString(fmt.Sprintf("    Call %d:\n", call.ID))

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
	PlanOperation(operation, definition *ast.Document) (*RPCExecutionPlan, error)
}

// NewPlanner returns a new PlanVisitor instance.
//
// The planner is responsible for creating an RPCExecutionPlan from a given
// GraphQL operation. It is used by the engine to execute operations against
// gRPC services.
func NewPlanner(subgraphName string, mapping *GRPCMapping, federationConfigs plan.FederationFieldConfigurations) (PlanVisitor, error) {
	if mapping == nil {
		return nil, fmt.Errorf("mapping is required")
	}

	if len(federationConfigs) > 0 {
		return newRPCPlanVisitorFederation(rpcPlanVisitorConfig{
			subgraphName:      subgraphName,
			mapping:           mapping,
			federationConfigs: federationConfigs,
		}), nil
	}

	return newRPCPlanVisitor(rpcPlanVisitorConfig{
		subgraphName:      subgraphName,
		mapping:           mapping,
		federationConfigs: federationConfigs,
	}), nil
}

// formatRPCMessage formats an RPCMessage and adds it to the string builder with the specified indentation
func formatRPCMessage(sb *strings.Builder, message RPCMessage, indent int) {
	indentStr := strings.Repeat(" ", indent)

	fmt.Fprintf(sb, "%sName: %s\n", indentStr, message.Name)
	fmt.Fprintf(sb, "%sFields:\n", indentStr)

	for _, field := range message.Fields {
		fmt.Fprintf(sb, "%s  - Name: %s\n", indentStr, field.Name)
		fmt.Fprintf(sb, "%s    TypeName: %s (%d)\n", indentStr, field.ProtoTypeName.String(), field.ProtoTypeName)
		fmt.Fprintf(sb, "%s    Repeated: %v\n", indentStr, field.Repeated)
		fmt.Fprintf(sb, "%s    JSONPath: %s\n", indentStr, field.JSONPath)
		fmt.Fprintf(sb, "%s    ResolvePath: %s\n", indentStr, field.ResolvePath.String())

		if field.Message != nil {
			fmt.Fprintf(sb, "%s    Message:\n", indentStr)
			formatRPCMessage(sb, *field.Message, indent+6)
		}
	}
}

type rpcPlanningContext struct {
	operation  *ast.Document
	definition *ast.Document
	mapping    *GRPCMapping
}

// newRPCPlanningContext creates a new RPCPlanningContext.
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
	dt := r.definition.Input.ByteSlice(t.Name)

	// Retrieve the node to check the kind
	node, found := r.definition.NodeByName(dt)
	if !found {
		return fromGraphQLType(dt)
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

// resolveRPCMethodMapping resolves the RPC method mapping for a given operation type and operation field name.
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

func (r *rpcPlanningContext) findResolverFieldMapping(typeName, fieldName string) string {
	resolveConfig := r.mapping.FindResolveTypeFieldMapping(typeName, fieldName)
	if resolveConfig == nil {
		return fieldName
	}

	return resolveConfig.FieldMappingData.TargetName
}

// resolveFieldMapping resolves the field mapping for a field.
// This applies both for complex types in the input and for all fields in the response.
func (r *rpcPlanningContext) resolveFieldMapping(typeName, fieldName string) string {
	if grpcFieldName, ok := r.mapping.FindFieldMapping(typeName, fieldName); ok {
		return grpcFieldName
	}

	return fieldName
}

// resolveFieldArgumentMapping resolves the field argument mapping for a given type name, field name and argument name.
func (r *rpcPlanningContext) resolveFieldArgumentMapping(typeName, fieldName, argumentName string) string {
	if grpcFieldName, ok := r.mapping.FindFieldArgumentMapping(typeName, fieldName, argumentName); ok {
		return grpcFieldName
	}

	return argumentName
}

// typeIsNullableOrNestedList checks if a type is nullable or a nested list.
func (r *rpcPlanningContext) typeIsNullableOrNestedList(typeRef int) bool {
	if !r.definition.TypeIsNonNull(typeRef) && r.definition.TypeIsList(typeRef) {
		return true
	}

	if r.definition.TypeNumberOfListWraps(typeRef) > 1 {
		return true
	}

	return false
}

// createListMetadata creates a list metadata for a given type reference.
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
func (r *rpcPlanningContext) buildField(parentTypeName string, fieldDef int, fieldName, fieldAlias string) (RPCField, error) {
	fieldDefType := r.definition.FieldDefinitionType(fieldDef)
	typeName := r.toDataType(&r.definition.Types[fieldDefType])

	field := RPCField{
		Name:          r.resolveFieldMapping(parentTypeName, fieldName),
		Alias:         fieldAlias,
		Optional:      !r.definition.TypeIsNonNull(fieldDefType),
		JSONPath:      fieldName,
		ProtoTypeName: typeName,
	}

	if r.definition.TypeIsList(fieldDefType) {
		switch {
		// for nullable or nested lists we need to build a wrapper message
		// Nullability is handled by the datasource during the execution.
		case r.typeIsNullableOrNestedList(fieldDefType):
			md, err := r.createListMetadata(fieldDefType)
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
		field.EnumName = r.definition.FieldDefinitionTypeNameString(fieldDef)
	}

	if fieldName == typenameFieldName {
		field.StaticValue = parentTypeName
	}

	return field, nil
}

// createRPCFieldFromFieldArgument builds an RPCField from an input value definition.
// It handles scalar, enum, and input object types.
// If the type is an input object type, a message is created and added to the field.
func (r *rpcPlanningContext) createRPCFieldFromFieldArgument(fieldArg fieldArgument) (RPCField, error) {
	argDef := r.definition.InputValueDefinitions[fieldArg.argumentDefinitionRef]
	argName := r.definition.Input.ByteSliceString(argDef.Name)
	underlyingTypeNode, found := r.definition.ResolveNodeFromTypeRef(argDef.Type)
	if !found {
		return RPCField{}, fmt.Errorf("unable to resolve underlying type node for argument %s", argName)
	}

	var (
		fieldMessage *RPCMessage
		err          error
		dt           = DataTypeMessage
	)

	// only scalar, enum and input object types are supported
	switch underlyingTypeNode.Kind {
	case ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
		dt = r.toDataType(&r.definition.Types[argDef.Type])
	case ast.NodeKindInputObjectTypeDefinition:
		// If the type is an input object type, a message is created and added to the field.
		if fieldMessage, err = r.buildMessageFromInputObjectType(&underlyingTypeNode); err != nil {
			return RPCField{}, err
		}
	default:
		return RPCField{}, fmt.Errorf("unsupported type: %s", underlyingTypeNode.Kind)
	}

	parentTypeName := fieldArg.parentTypeNode.NameString(r.definition)
	fieldName := r.definition.FieldDefinitionNameString(fieldArg.fieldDefinitionRef)
	mappedName := r.resolveFieldArgumentMapping(parentTypeName, fieldName, argName)
	field, err := r.buildInputMessageField(
		argDef.Type,
		mappedName,
		fieldArg.jsonPath,
		dt,
	)
	if err != nil {
		return RPCField{}, err
	}

	field.Message = fieldMessage
	return field, nil
}

// buildMessageFromInputObjectType builds a message from an input object type definition.
func (r *rpcPlanningContext) buildMessageFromInputObjectType(node *ast.Node) (*RPCMessage, error) {
	if node.Kind != ast.NodeKindInputObjectTypeDefinition {
		return nil, fmt.Errorf("unable to build message from input object type definition - incorrect type: %s", node.Kind)
	}

	inputObjectDefinition := r.definition.InputObjectTypeDefinitions[node.Ref]
	message := &RPCMessage{
		Name:   node.NameString(r.definition),
		Fields: make(RPCFields, 0, len(inputObjectDefinition.InputFieldsDefinition.Refs)),
	}
	for _, inputFieldRef := range inputObjectDefinition.InputFieldsDefinition.Refs {
		field, err := r.buildMessageFieldFromInputValueDefinition(inputFieldRef, node)
		if err != nil {
			return nil, err
		}

		message.Fields = append(message.Fields, field)
	}

	return message, nil
}

// buildMessageFieldFromInputValueDefinition builds an RPCField from an input value definition.
func (r *rpcPlanningContext) buildMessageFieldFromInputValueDefinition(ivdRef int, node *ast.Node) (RPCField, error) {
	inputValueDef := r.definition.InputValueDefinitions[ivdRef]
	inputValueDefType := r.definition.Types[inputValueDef.Type]

	// We need to resolve the underlying type to determine whether we are building a nested message or a scalar type.
	underlyingTypeNode, found := r.definition.ResolveNodeFromTypeRef(inputValueDef.Type)
	if !found {
		return RPCField{}, fmt.Errorf("unable to resolve underlying type node for input value definition %s", r.definition.Input.ByteSliceString(inputValueDef.Name))
	}

	var (
		fieldMessage *RPCMessage
		err          error
	)

	// If the type is an input object type, we need to build a nested message.
	dt := DataTypeMessage
	switch underlyingTypeNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		fieldMessage, err = r.buildMessageFromInputObjectType(&underlyingTypeNode)
		if err != nil {
			return RPCField{}, err
		}
	default:
		dt = r.toDataType(&inputValueDefType)
	}

	fieldName := r.definition.Input.ByteSliceString(inputValueDef.Name)
	mappedName := r.resolveFieldMapping(node.NameString(r.definition), fieldName)

	field, err := r.buildInputMessageField(inputValueDef.Type, mappedName, fieldName, dt)
	if err != nil {
		return RPCField{}, err
	}

	field.Message = fieldMessage
	return field, nil
}

// buildInputMessageField builds an RPCField from an input value definition.
// It handles scalar, enum and list types.
func (r *rpcPlanningContext) buildInputMessageField(typeRef int, fieldName, jsonPath string, dt DataType) (RPCField, error) {
	field := RPCField{
		Name:          fieldName,
		Optional:      !r.definition.TypeIsNonNull(typeRef),
		ProtoTypeName: dt,
		JSONPath:      jsonPath,
	}

	if r.definition.TypeIsList(typeRef) {
		switch {
		// for nullable or nested lists we need to build a wrapper message
		// Nullability is handled by the datasource during the execution.
		case r.typeIsNullableOrNestedList(typeRef):
			md, err := r.createListMetadata(typeRef)
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

	if dt == DataTypeEnum {
		field.EnumName = r.definition.ResolveTypeNameString(typeRef)
	}

	return field, nil
}

// buildFieldMessage builds a message from a field definition.
// It handles complex and composite types.
func (r *rpcPlanningContext) buildFieldMessage(fieldTypeNode ast.Node, fieldRef int) (*RPCMessage, error) {
	field := r.operation.Fields[fieldRef]
	if !field.HasSelections {
		return nil, fmt.Errorf("unable to build field message: field %s has no selections", r.operation.FieldAliasOrNameString(fieldRef))
	}

	fieldRefs := make([]int, 0)
	inlineFragmentRefs := make([]int, 0)
	selections := r.operation.SelectionSets[field.SelectionSet].SelectionRefs
	for i := range selections {
		selection := r.operation.Selections[selections[i]]
		switch selection.Kind {
		case ast.SelectionKindField:
			fieldRefs = append(fieldRefs, selection.Ref)
		case ast.SelectionKindInlineFragment:
			inlineFragmentRefs = append(inlineFragmentRefs, selection.Ref)
		}
	}

	message := &RPCMessage{
		Name: fieldTypeNode.NameString(r.definition),
	}

	if compositeType := r.getCompositeType(fieldTypeNode); compositeType != OneOfTypeNone {
		memberTypes, err := r.getMemberTypes(fieldTypeNode)
		if err != nil {
			return nil, err
		}
		message.MemberTypes = memberTypes
		message.OneOfType = compositeType
	}

	for _, inlineFragmentRef := range inlineFragmentRefs {
		selectionSetRef, ok := r.operation.InlineFragmentSelectionSet(inlineFragmentRef)
		if !ok {
			continue
		}

		typeName := r.operation.InlineFragmentTypeConditionNameString(inlineFragmentRef)
		inlineFragmentTypeNode, found := r.definition.NodeByNameStr(typeName)
		if !found {
			return nil, fmt.Errorf("unable to resolve type node for inline fragment %s", typeName)
		}

		fields, err := r.buildCompositeFields(inlineFragmentTypeNode, fragmentSelection{
			typeName:        typeName,
			selectionSetRef: selectionSetRef,
		})

		if err != nil {
			return nil, err
		}

		if message.FieldSelectionSet == nil {
			message.FieldSelectionSet = make(RPCFieldSelectionSet)
		}

		message.FieldSelectionSet.Add(typeName, fields...)
	}

	for _, fieldRef := range fieldRefs {
		fieldDefRef, found := r.definition.NodeFieldDefinitionByName(fieldTypeNode, r.operation.FieldNameBytes(fieldRef))
		if !found {
			return nil, fmt.Errorf("unable to build required field: field definition not found for field %s", r.operation.FieldNameString(fieldRef))
		}

		if r.isFieldResolver(fieldDefRef, false) {
			continue
		}

		field, err := r.buildResolverField(fieldTypeNode, fieldRef, fieldDefRef)
		if err != nil {
			return nil, err
		}
		message.Fields = append(message.Fields, field)
	}

	return message, nil
}

// resolveServiceName resolves the service name for a given subgraph name.
func (r *rpcPlanningContext) resolveServiceName(subgraphName string) string {
	if r.mapping == nil || r.mapping.Service == "" {
		return subgraphName
	}

	return r.mapping.Service
}

type resolverField struct {
	id                     int
	callerRef              int
	parentTypeNode         ast.Node
	fieldRef               int
	fieldDefinitionTypeRef int
	fieldsSelectionSetRef  int
	responsePath           ast.Path
	contextPath            ast.Path

	contextFields      []contextField
	fieldArguments     []fieldArgument
	fragmentSelections []fragmentSelection
	fragmentType       OneOfType
	listNestingLevel   int
	memberTypes        []string
}

type fragmentSelection struct {
	typeName        string
	selectionSetRef int
}

// enterResolverCompositeSelectionSet handles logic when entering a composite selection set for a given field resolver.
// It appends the inline fragment selections to the resolved field and sets the fragment type.
func (r *rpcPlanningContext) enterResolverCompositeSelectionSet(oneOfType OneOfType, selectionSetRef int, resolvedField *resolverField) {
	resolvedField.fieldsSelectionSetRef = ast.InvalidRef
	resolvedField.fragmentType = oneOfType

	// In case of an interface we can select individual fields from the interface without having to use an inline fragment.
	if len(r.operation.SelectionSetFieldRefs(selectionSetRef)) > 0 {
		resolvedField.fieldsSelectionSetRef = selectionSetRef
	}

	inlineFragSelections := r.operation.SelectionSetInlineFragmentSelections(selectionSetRef)
	if len(inlineFragSelections) == 0 {
		return
	}

	for _, inlineFragSelectionRef := range inlineFragSelections {
		inlineFragRef := r.operation.Selections[inlineFragSelectionRef].Ref
		inlinFragSelectionSetRef, ok := r.operation.InlineFragmentSelectionSet(inlineFragRef)
		if !ok {
			continue
		}

		resolvedField.fragmentSelections = append(resolvedField.fragmentSelections, fragmentSelection{
			typeName:        r.operation.InlineFragmentTypeConditionNameString(inlineFragRef),
			selectionSetRef: inlinFragSelectionSetRef,
		})
	}
}

// isFieldResolver checks if a field is a field resolver.
func (r *rpcPlanningContext) isFieldResolver(fieldDefRef int, isRootField bool) bool {
	if isRootField || fieldDefRef == ast.InvalidRef {
		return false
	}

	return r.definition.FieldDefinitionHasArgumentsDefinitions(fieldDefRef)
}

// isRequiredField checks if a field is a required field.
func (r *rpcPlanningContext) isRequiredField(fieldDefRef int) bool {
	return r.definition.FieldDefinitionHasNamedDirective(fieldDefRef, requiresDirectiveName)
}

// getCompositeType checks whether the node is an interface or union type.
// It returns OneOfTypeNone for non-composite types.
func (r *rpcPlanningContext) getCompositeType(node ast.Node) OneOfType {
	switch node.Kind {
	case ast.NodeKindInterfaceTypeDefinition:
		return OneOfTypeInterface
	case ast.NodeKindUnionTypeDefinition:
		return OneOfTypeUnion
	default:
		return OneOfTypeNone
	}
}

func (r *rpcPlanningContext) getMemberTypes(node ast.Node) ([]string, error) {
	switch node.Kind {
	case ast.NodeKindInterfaceTypeDefinition:
		memberTypes, ok := r.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
		if !ok {
			return nil, fmt.Errorf("interface type %s is not implemented by any object", r.definition.InterfaceTypeDefinitionNameString(node.Ref))
		}
		return memberTypes, nil
	case ast.NodeKindUnionTypeDefinition:
		memberTypes, ok := r.definition.UnionTypeDefinitionMemberTypeNames(node.Ref)
		if !ok {
			return nil, fmt.Errorf("union type %s is not defined", r.definition.UnionTypeDefinitionNameString(node.Ref))
		}
		return memberTypes, nil
	default:
		return nil, fmt.Errorf("invalid node kind: %s", node.Kind)
	}
}

// setResolvedField sets the resolved field for a given field definition reference.
func (r *rpcPlanningContext) setResolvedField(walker *astvisitor.Walker, fieldDefRef int, fieldArgs []int, fieldPath ast.Path, resolvedField *resolverField) error {
	// We need to resolve the context fields for the given field definition reference.
	contextFields, err := r.resolveContextFields(walker, fieldDefRef)
	if err != nil {
		return err
	}

	for _, contextFieldRef := range contextFields {
		mapping := r.resolveFieldMapping(
			walker.EnclosingTypeDefinition.NameString(r.definition),
			r.definition.FieldDefinitionNameString(contextFieldRef),
		)
		resolvedPath := fieldPath.WithFieldNameItem([]byte(mapping))

		resolvedField.contextFields = append(resolvedField.contextFields, contextField{
			fieldRef:    contextFieldRef,
			resolvePath: resolvedPath,
		})
	}

	fieldArguments, err := r.parseFieldArguments(walker, fieldDefRef, fieldArgs)
	if err != nil {
		return err
	}

	resolvedField.fieldArguments = fieldArguments

	fieldDefType := r.definition.FieldDefinitionType(fieldDefRef)
	if r.typeIsNullableOrNestedList(fieldDefType) {
		resolvedField.listNestingLevel = r.definition.TypeNumberOfListWraps(fieldDefType)
	}

	return nil
}

// resolveContextFields resolves the context fields for a given field definition reference.
// The function attempts to resolve the context fields from the @connect__fieldResolver directive.
// If the directive is not present it instead attempts to resolve the ID field.
func (r *rpcPlanningContext) resolveContextFields(walker *astvisitor.Walker, fieldDefRef int) ([]int, error) {
	resolverDirectiveRef, exists := r.definition.FieldDefinitionDirectiveByName(fieldDefRef, ast.ByteSlice(fieldResolverDirectiveName))
	if exists {
		fields, err := r.getFieldsFromFieldResolverDirective(walker.EnclosingTypeDefinition, resolverDirectiveRef)
		if err != nil {
			return nil, err
		}

		return fields, nil
	}

	// If the directive is not present it instead attempts to resolve the ID field.
	idFieldRef, err := r.findIDField(walker.EnclosingTypeDefinition, fieldDefRef)
	return []int{idFieldRef}, err
}

// parseFieldArguments parses the field arguments for a given field definition reference.
func (r *rpcPlanningContext) parseFieldArguments(walker *astvisitor.Walker, fieldDefRef int, fieldArgs []int) ([]fieldArgument, error) {
	result := make([]fieldArgument, 0, len(fieldArgs))
	for _, fieldArgRef := range fieldArgs {
		arg := r.operation.Arguments[fieldArgRef]

		if arg.Value.Kind != ast.ValueKindVariable {
			return nil, fmt.Errorf("unsupported argument value kind: %s", arg.Value.Kind)
		}

		argDefRef := r.definition.NodeFieldDefinitionArgumentDefinitionByName(
			walker.EnclosingTypeDefinition,
			r.definition.FieldDefinitionNameBytes(fieldDefRef),
			r.operation.ArgumentNameBytes(fieldArgRef),
		)

		if argDefRef == ast.InvalidRef {
			return nil, fmt.Errorf("unable to resolve argument input value definition for argument %s", r.operation.ArgumentNameString(fieldArgRef))
		}

		result = append(result, fieldArgument{
			fieldDefinitionRef:    fieldDefRef,
			argumentDefinitionRef: argDefRef,
			parentTypeNode:        walker.EnclosingTypeDefinition,
			jsonPath:              r.operation.VariableValueNameString(arg.Value.Ref),
		})

	}

	return result, nil

}

// getFieldsFromFieldResolverDirective gets the fields from the @connect__fieldResolver directive.
// It returns the field definition references for the fields in the context.
func (r *rpcPlanningContext) getFieldsFromFieldResolverDirective(parentNode ast.Node, contextRef int) ([]int, error) {
	val, exists := r.definition.DirectiveArgumentValueByName(contextRef, []byte("context"))
	if !exists {
		return nil, fmt.Errorf("context directive argument not found")
	}

	fieldsString := r.definition.ValueContentString(val)

	walker := astvisitor.WalkerFromPool()
	defer walker.Release()

	v := newRequiredFieldsVisitor(walker, &RPCMessage{}, r)
	if err := v.visitWithDefaults(r.definition, parentNode.NameString(r.definition), fieldsString); err != nil {
		return nil, err
	}

	return v.fieldDefinitionRefs, nil
}

// findIDField attempts to find the ID field for a given field definition reference.
// It fails if the parent node is not an object type definition.
// The functions checks whether an available ID field is present in the object type definition.
// If exactly one ID field is found, it returns the field definition reference.
// If none or multiple ID fields are found, it returns an error.
func (r *rpcPlanningContext) findIDField(parentNode ast.Node, fieldDefRef int) (int, error) {
	switch parentNode.Kind {
	case ast.NodeKindObjectTypeDefinition:
		o := r.definition.ObjectTypeDefinitions[parentNode.Ref]
		result := slices.Collect(r.filterIDFieldsFunc(o, fieldDefRef))

		if len(result) == 0 {
			return ast.InvalidRef, fmt.Errorf("unable to determine ID field in object type %s", parentNode.NameString(r.definition))
		}

		if len(result) > 1 {
			return ast.InvalidRef, fmt.Errorf("multiple ID fields found in object type %s", parentNode.NameString(r.definition))
		}

		return result[0], nil
	default:
		return ast.InvalidRef, fmt.Errorf("invalid parent node kind: %s, expected ObjectTypeDefinition", parentNode.Kind)
	}
}

// filterIDFieldsFunc is a helper function to filter the ID fields from the object type definition.
// It yields the field definition references for the ID fields.
// It skips the field definition reference for the given field definition reference.
func (r *rpcPlanningContext) filterIDFieldsFunc(o ast.ObjectTypeDefinition, fieldDefRef int) func(yield func(int) bool) {
	fieldRefs := o.FieldsDefinition.Refs
	const idTypeName = "ID"
	return func(yield func(int) bool) {
		for _, ref := range fieldRefs {
			if ref == fieldDefRef {
				continue
			}

			typeName := r.definition.FieldDefinitionTypeNameString(ref)
			if typeName != idTypeName {
				continue
			}

			if !yield(ref) {
				return
			}
		}
	}
}

type resolveRPCCallConfig struct {
	resolveConfig    *ResolveRPCTypeField
	resolvedField    *resolverField
	contextMessage   *RPCMessage
	fieldArgsMessage *RPCMessage
}

type buildFieldMessageConfig struct {
	typeName              string
	fragmentType          OneOfType
	memberTypes           []string
	fragmentSelections    []fragmentSelection
	fieldsSelectionSetRef int
	fieldRefs             []int
}

// buildMessageForField builds the message for a given field resolver type.
// When a field resolver returns a complex or composite type, we need to build a message for the type.
func (r *rpcPlanningContext) buildMessageForField(config buildFieldMessageConfig) (*RPCMessage, error) {
	message := &RPCMessage{
		Name:        config.typeName,
		OneOfType:   config.fragmentType,
		MemberTypes: config.memberTypes,
	}

	// field resolvers which return a non scalar type must have a selection set.
	// If we don't have a selection set we return an error.
	if len(config.fragmentSelections) == 0 && config.fieldsSelectionSetRef == ast.InvalidRef {
		return nil, errors.New("unable to resolve required fields: no fields selection set found")
	}

	// If the resolved field returns a composite type we need to handle the selection set for the inline fragment.
	if len(config.fragmentSelections) > 0 {
		message.FieldSelectionSet = make(RPCFieldSelectionSet, len(config.fragmentSelections))

		for _, fragmentSelection := range config.fragmentSelections {
			inlineFragmentTypeNode, found := r.definition.NodeByNameStr(fragmentSelection.typeName)
			if !found {
				return nil, fmt.Errorf("unable to build composite field: underlying fragment type node not found for type %s", fragmentSelection.typeName)
			}

			fields, err := r.buildCompositeFields(inlineFragmentTypeNode, fragmentSelection)
			if err != nil {
				return nil, err
			}

			message.FieldSelectionSet[fragmentSelection.typeName] = fields
		}
	}

	if config.fieldsSelectionSetRef == ast.InvalidRef {
		return message, nil
	}

	// If the resolved field does not return a composite type we handle the selection set for the required field.
	parentTypeNode, found := r.definition.NodeByNameStr(config.typeName)
	if !found {
		return nil, fmt.Errorf("parent type node not found for type %s", config.typeName)
	}

	fieldRefs := r.operation.SelectionSetFieldRefs(config.fieldsSelectionSetRef)
	message.Fields = make(RPCFields, 0, len(fieldRefs))

	for _, fieldRef := range fieldRefs {
		fieldDefRef, found := r.definition.NodeFieldDefinitionByName(parentTypeNode, r.operation.FieldNameBytes(fieldRef))
		if !found {
			return nil, fmt.Errorf("unable to build required field: field definition not found for field %s", r.operation.FieldNameString(fieldRef))
		}

		if r.isFieldResolver(fieldDefRef, false) {
			continue
		}

		if message.Fields.Exists(r.operation.FieldNameString(fieldRef), "") {
			continue
		}

		field, err := r.buildResolverField(parentTypeNode, fieldRef, fieldDefRef)
		if err != nil {
			return nil, err
		}

		message.Fields = append(message.Fields, field)
	}

	message.Fields = slices.Clip(message.Fields)
	return message, nil
}

func (r *rpcPlanningContext) buildResolverField(typeNode ast.Node, fieldRef, fieldDefinitionRef int) (RPCField, error) {
	field, err := r.buildField(
		typeNode.NameString(r.definition),
		fieldDefinitionRef,
		r.operation.FieldNameString(fieldRef),
		r.operation.FieldAliasString(fieldRef),
	)
	if err != nil {
		return RPCField{}, err
	}

	// If the field is a message type and has selections, we need to build a nested message.
	if field.ProtoTypeName == DataTypeMessage && r.operation.FieldHasSelections(fieldRef) {
		fieldTypeNode, found := r.definition.ResolveNodeFromTypeRef(r.definition.FieldDefinitionType(fieldDefinitionRef))
		if !found {
			return RPCField{}, fmt.Errorf("unable to build required field: unable to resolve field type node for field %s", r.operation.FieldNameString(fieldRef))
		}

		message, err := r.buildFieldMessage(fieldTypeNode, fieldRef)
		if err != nil {
			return RPCField{}, err
		}

		field.Message = message
	}

	return field, nil
}

// buildCompositeFields creates fields for a given inline fragment node and its selection set.
// It returns a list of fields that have been composed from the inputs.
func (r *rpcPlanningContext) buildCompositeFields(inlineFragmentNode ast.Node, fragmentSelection fragmentSelection) ([]RPCField, error) {
	fieldRefs := r.operation.SelectionSetFieldRefs(fragmentSelection.selectionSetRef)
	result := make([]RPCField, 0, len(fieldRefs))

	for _, fieldRef := range fieldRefs {
		fieldDefRef := r.fieldDefinitionRefForType(r.operation.FieldNameString(fieldRef), fragmentSelection.typeName)
		if fieldDefRef == ast.InvalidRef {
			return nil, fmt.Errorf("unable to build composite field: field definition not found for field %s", r.operation.FieldNameString(fieldRef))
		}

		if r.isFieldResolver(fieldDefRef, false) {
			continue
		}

		field, err := r.buildField(
			inlineFragmentNode.NameString(r.definition),
			fieldDefRef,
			r.operation.FieldNameString(fieldRef),
			r.operation.FieldAliasString(fieldRef),
		)

		if err != nil {
			return nil, err
		}

		if field.ProtoTypeName == DataTypeMessage && r.operation.FieldHasSelections(fieldRef) {
			fieldTypeNode, found := r.definition.ResolveNodeFromTypeRef(r.definition.FieldDefinitionType(fieldDefRef))
			if !found {
				return nil, fmt.Errorf("unable to build composite field: unable to resolve field type node for field %s", r.operation.FieldNameString(fieldRef))
			}

			message, err := r.buildFieldMessage(fieldTypeNode, fieldRef)
			if err != nil {
				return nil, err
			}

			field.Message = message
		}

		result = append(result, field)
	}
	return result, nil
}

func (r *rpcPlanningContext) fieldDefinitionRefForType(fieldName, typeName string) int {
	node, found := r.definition.NodeByNameStr(typeName)
	if !found {
		return ast.InvalidRef
	}

	if ref, found := r.definition.NodeFieldDefinitionByName(node, unsafebytes.StringToBytes(fieldName)); found {
		return ref
	}

	return ast.InvalidRef

}

// createRequiredFieldsRPCCalls creates a new call for each required field.
// It returns a list of calls which are needed to provide certain fields for the entity, which require data from the representation variables.
/*
message RequireWarehouseStockHealthScoreByIdRequest {
  // RequireWarehouseStockHealthScoreByIdContext provides the context for the required fields method RequireWarehouseStockHealthScoreById.
  repeated RequireWarehouseStockHealthScoreByIdContext context = 1;
}

message RequireWarehouseStockHealthScoreByIdContext {
  LookupWarehouseByIdRequestKey key = 1;
  RequireWarehouseStockHealthScoreByIdFields fields = 2;
}

message RequireWarehouseStockHealthScoreByIdResult {
  double stock_health_score = 1;
}

message RequireWarehouseStockHealthScoreByIdFields {
  message RestockData {
    string last_restock_date = 1;
  }

  int32 inventory_count = 1;
  RestockData restock_data = 2;
}
*/
func (r *rpcPlanningContext) createRequiredFieldsRPCCalls(callIndex *int, subgraphName string, entityTypeName string, data entityConfigData) ([]RPCCall, error) {
	calls := make([]RPCCall, 0, len(data.requiredFields))
	for _, requiredField := range data.requiredFields {
		call, err := r.createRequiredFieldsRPCCall(*callIndex, subgraphName, entityTypeName, &requiredField, data)
		if err != nil {
			return nil, err
		}

		*callIndex++
		calls = append(calls, call)
	}

	return calls, nil
}

// createRequiredFieldsRPCCall creates a new required fields RPC call for a given configuration.
func (r *rpcPlanningContext) createRequiredFieldsRPCCall(callIndex int, subgraphName, typeName string, requiredField *requiredField, data entityConfigData) (RPCCall, error) {
	rpcConfig, exists := r.mapping.FindRequiredFieldsRPCConfig(typeName, data.keyFields, requiredField.fieldName)
	if !exists {
		return RPCCall{}, fmt.Errorf("required fields RPC config not found for type: %s, field: %s", typeName, requiredField.fieldName)
	}

	fieldMessage := &RPCMessage{
		Name: rpcConfig.RPC + "Fields",
	}

	fieldDefRef := r.fieldDefinitionRefForType(requiredField.fieldName, typeName)
	if fieldDefRef == ast.InvalidRef {
		return RPCCall{}, fmt.Errorf("unable to build required field: field definition not found for field %s", requiredField.fieldName)
	}

	field, err := r.buildField(typeName, fieldDefRef, requiredField.fieldName, "")
	if err != nil {
		return RPCCall{}, err
	}

	field.Name = rpcConfig.TargetName

	call := RPCCall{
		ID:          callIndex,
		ServiceName: r.resolveServiceName(subgraphName),
		Kind:        CallKindRequired,
		MethodName:  rpcConfig.RPC,
		ResponsePath: ast.Path{
			{Kind: ast.FieldName, FieldName: []byte("_entities")},
			{Kind: ast.FieldName, FieldName: []byte(requiredField.fieldName)},
		},
		Request: RPCMessage{
			Name: rpcConfig.Request,
			Fields: RPCFields{
				{
					Name:          contextFieldName, // Static name for the context field.
					ProtoTypeName: DataTypeMessage,
					JSONPath:      "representations",
					Repeated:      true,
					Message: &RPCMessage{
						Name: rpcConfig.RPC + "Context",
						Fields: RPCFields{
							{
								Name:          "key",
								ProtoTypeName: DataTypeMessage,
								Message:       data.keyFieldMessage,
							},
							{
								Name:          requiresArgumentsFieldName,
								ProtoTypeName: DataTypeMessage,
								Message:       fieldMessage,
							},
						},
					},
				},
			},
		},
		Response: RPCMessage{
			Name: rpcConfig.Response,
			Fields: RPCFields{
				{
					Name:          resultFieldName,
					ProtoTypeName: DataTypeMessage,
					Repeated:      true,
					JSONPath:      resultFieldName,
					Message: &RPCMessage{
						Name:   rpcConfig.RPC + "Result",
						Fields: RPCFields{requiredField.resultField},
					},
				},
			},
		},
	}

	walker := astvisitor.WalkerFromPool()
	defer walker.Release()

	vis := newRequiredFieldsVisitor(walker, fieldMessage, r)
	if err := vis.visit(r.definition, typeName, requiredField.selectionSet, requiredFieldVisitorConfig{
		referenceNestedMessages: true,
	}); err != nil {
		return RPCCall{}, err
	}

	return call, nil
}

// createResolverRPCCalls creates a new call for each resolved field.
func (r *rpcPlanningContext) createResolverRPCCalls(subgraphName string, resolvedFields []resolverField) ([]RPCCall, error) {
	if len(resolvedFields) == 0 {
		return nil, nil
	}

	// We need to create a new call for each resolved field.
	calls := make([]RPCCall, 0, len(resolvedFields))

	for _, resolvedField := range resolvedFields {
		resolveConfig := r.mapping.FindResolveTypeFieldMapping(
			resolvedField.parentTypeNode.NameString(r.definition),
			r.operation.FieldNameString(resolvedField.fieldRef),
		)

		if resolveConfig == nil {
			return nil, fmt.Errorf("resolve config not found for type: %s, field: %s", r.definition.NodeNameString(resolvedField.parentTypeNode), r.operation.FieldAliasString(resolvedField.fieldRef))
		}

		contextMessage := &RPCMessage{
			Name: resolveConfig.RPC + "Context",
		}

		fieldArgsMessage := &RPCMessage{
			Name: resolveConfig.RPC + "Args",
		}

		call, err := r.newResolveRPCCall(&resolveRPCCallConfig{
			resolveConfig:    resolveConfig,
			resolvedField:    &resolvedField,
			contextMessage:   contextMessage,
			fieldArgsMessage: fieldArgsMessage,
		})

		if err != nil {
			return nil, err
		}

		call.ServiceName = r.resolveServiceName(subgraphName)

		contextMessage.Fields = make(RPCFields, len(resolvedField.contextFields))
		for i := range resolvedField.contextFields {

			field, err := r.buildField(
				resolvedField.parentTypeNode.NameString(r.definition),
				resolvedField.contextFields[i].fieldRef,
				r.definition.FieldDefinitionNameString(resolvedField.contextFields[i].fieldRef),
				"",
			)

			field.ResolvePath = resolvedField.contextFields[i].resolvePath

			if err != nil {
				return nil, err
			}

			contextMessage.Fields[i] = field
		}

		if argLen := len(resolvedField.fieldArguments); argLen > 0 {
			fieldArgsMessage.Fields = make(RPCFields, argLen)
			for i := range resolvedField.fieldArguments {
				field, err := r.createRPCFieldFromFieldArgument(resolvedField.fieldArguments[i])

				if err != nil {
					return nil, err
				}

				fieldArgsMessage.Fields[i] = field
			}
		}

		calls = append(calls, call)
	}

	return calls, nil
}

// newResolveRPCCall creates a new resolve RPC call for a given resolved field.
func (r *rpcPlanningContext) newResolveRPCCall(config *resolveRPCCallConfig) (RPCCall, error) {
	resolveConfig := config.resolveConfig
	resolvedField := config.resolvedField

	underlyingTypeRef := r.definition.ResolveUnderlyingType(resolvedField.fieldDefinitionTypeRef)
	dataType := r.toDataType(&r.definition.Types[underlyingTypeRef])

	var responseFieldsMessage *RPCMessage
	if dataType == DataTypeMessage {
		var err error
		responseFieldsMessage, err = r.buildMessageForField(buildFieldMessageConfig{
			typeName:              r.definition.ResolveTypeNameString(underlyingTypeRef),
			fragmentType:          resolvedField.fragmentType,
			memberTypes:           resolvedField.memberTypes,
			fragmentSelections:    resolvedField.fragmentSelections,
			fieldsSelectionSetRef: resolvedField.fieldsSelectionSetRef,
		},
		)

		if err != nil {
			return RPCCall{}, err
		}
	}

	fd := r.fieldDefinitionRefForType(r.operation.FieldNameString(resolvedField.fieldRef), resolvedField.parentTypeNode.NameString(r.definition))
	if fd == ast.InvalidRef {
		return RPCCall{}, fmt.Errorf("unable to build response field: field definition not found for field %s", r.operation.FieldNameString(resolvedField.fieldRef))
	}

	field, err := r.buildField(resolvedField.parentTypeNode.NameString(r.definition), fd, r.operation.FieldNameString(resolvedField.fieldRef), r.operation.FieldAliasString(resolvedField.fieldRef))
	if err != nil {
		return RPCCall{}, err
	}

	field.Name = resolveConfig.FieldMappingData.TargetName
	field.Message = responseFieldsMessage

	response := RPCMessage{
		Name: resolveConfig.Response,
		Fields: RPCFields{
			{
				Name:          resultFieldName,
				ProtoTypeName: DataTypeMessage,
				JSONPath:      resultFieldName,
				Repeated:      true,
				Message: &RPCMessage{
					Name:   resolveConfig.RPC + "Result",
					Fields: RPCFields{field},
				},
			},
		},
	}

	return RPCCall{
		ID:             resolvedField.id,
		DependentCalls: []int{resolvedField.callerRef},
		ResponsePath:   resolvedField.responsePath,
		MethodName:     resolveConfig.RPC,
		Kind:           CallKindResolve,
		Request: RPCMessage{
			Name: resolveConfig.Request,
			Fields: RPCFields{
				{
					Name:          contextFieldName,
					ProtoTypeName: DataTypeMessage,
					Repeated:      true,
					Message:       config.contextMessage,
				},
				{
					Name:          fieldArgsFieldName,
					ProtoTypeName: DataTypeMessage,
					Message:       config.fieldArgsMessage,
				},
			},
		},
		Response: response,
	}, nil
}
