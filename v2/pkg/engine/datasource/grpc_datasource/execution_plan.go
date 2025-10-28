package grpcdatasource

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

const (
	// knownTypeOptionalFieldValueName is the name of the field that is used to wrap optional scalar values
	// in a message as protobuf scalar types are not nullable.
	knownTypeOptionalFieldValueName = "value"

	// resolverContextDirectiveName is the name of the directive that is used to configure the resolver context.
	resolverContextDirectiveName = "connect__fieldResolver"

	// typenameFieldName is the name of the field that is used to store the typename of the object.
	typenameFieldName = "__typename"
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
type CallKind int

const (
	// CallKindStandard is a basic fetch operation.
	CallKindStandard CallKind = iota
	// CallKindEntity is a fetch operation for entities.
	CallKindEntity
	// CallKindResolve is a fetch operation for resolving field values.
	CallKindResolve
)

// RPCCall represents a single call to a gRPC service method.
// It contains all the information needed to make the call and process the response.
type RPCCall struct {
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

func (r *RPCMessage) AppendTypeNameField(typeName string) {
	if r.Fields != nil && r.Fields.Exists(typenameFieldName, "") {
		return
	}

	r.Fields = append(r.Fields, RPCField{
		Name:          typenameFieldName,
		ProtoTypeName: DataTypeString,
		StaticValue:   typeName,
		JSONPath:      typenameFieldName,
	})
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
		fmt.Fprintf(sb, "%s    TypeName: %s\n", indentStr, field.ProtoTypeName)
		fmt.Fprintf(sb, "%s    Repeated: %v\n", indentStr, field.Repeated)
		fmt.Fprintf(sb, "%s    JSONPath: %s\n", indentStr, field.JSONPath)

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
	if grpcFieldName, ok := r.mapping.FindFieldMapping(typeName, fieldName); ok {
		return grpcFieldName
	}

	return fieldName
}

func (r *rpcPlanningContext) resolveFieldArgumentMapping(typeName, fieldName, argumentName string) string {
	if grpcFieldName, ok := r.mapping.FindFieldArgumentMapping(typeName, fieldName, argumentName); ok {
		return grpcFieldName
	}

	return argumentName
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
		Name:          r.resolveFieldMapping(parentTypeName, fieldName),
		Alias:         fieldAlias,
		Optional:      !r.definition.TypeIsNonNull(fdt),
		JSONPath:      fieldName,
		ProtoTypeName: typeName,
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
	underlyingTypeNode, found := r.nodeByTypeRef(argDef.Type)
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
		return nil, fmt.Errorf("unsupported type: %s", node.Kind)
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

func (r *rpcPlanningContext) buildMessageFieldFromInputValueDefinition(ivdRef int, node *ast.Node) (RPCField, error) {
	ivd := r.definition.InputValueDefinitions[ivdRef]
	ivdType := r.definition.Types[ivd.Type]

	underlyingTypeNode, found := r.nodeByTypeRef(ivd.Type)
	if !found {
		return RPCField{}, fmt.Errorf("unable to resolve underlying type node for input value definition %s", r.definition.Input.ByteSliceString(ivd.Name))
	}

	var (
		fieldMessage *RPCMessage
		err          error
	)

	dt := DataTypeMessage
	switch underlyingTypeNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		fieldMessage, err = r.buildMessageFromInputObjectType(&underlyingTypeNode)
		if err != nil {
			return RPCField{}, err
		}
	default:
		dt = r.toDataType(&ivdType)
	}

	fieldName := r.definition.Input.ByteSliceString(ivd.Name)
	mappedName := r.resolveFieldMapping(node.NameString(r.definition), fieldName)

	field, err := r.buildInputMessageField(ivd.Type, mappedName, fieldName, dt)
	if err != nil {
		return RPCField{}, err
	}

	field.Message = fieldMessage
	return field, nil
}

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

func (r *rpcPlanningContext) resolveServiceName(subgraphName string) string {
	if r.mapping == nil || r.mapping.Service == "" {
		return subgraphName
	}

	return r.mapping.Service
}

type resolvedField struct {
	callerRef              int
	parentTypeRef          int
	fieldRef               int
	fieldDefinitionTypeRef int
	requiredFieldSelection int
	responsePath           ast.Path

	contextFields  []contextField
	fieldArguments []fieldArgument
}

func (r *rpcPlanningContext) setResolvedField(walker *astvisitor.Walker, fd int, fieldArgs []int, fieldPath ast.Path, resolvedField *resolvedField) error {
	contextFields, err := r.resolveContextFields(walker, fd)
	if err != nil {
		return err
	}

	for _, contextFieldRef := range contextFields {
		contextFieldName := r.definition.FieldDefinitionNameBytes(contextFieldRef)
		resolvedPath := fieldPath.WithFieldNameItem(contextFieldName)

		resolvedField.contextFields = append(resolvedField.contextFields, contextField{
			fieldRef:    contextFieldRef,
			resolvePath: resolvedPath,
		})
	}

	fieldArguments, err := r.parseFieldArguments(walker, fd, fieldArgs)
	if err != nil {
		return err
	}

	resolvedField.fieldArguments = fieldArguments
	return nil
}

func (r *rpcPlanningContext) resolveContextFields(walker *astvisitor.Walker, fd int) ([]int, error) {
	contextDirectiveRef, exists := r.definition.FieldDefinitionDirectiveByName(fd, ast.ByteSlice(resolverContextDirectiveName))
	if exists {
		fields, err := r.getFieldsFromContext(walker.EnclosingTypeDefinition, contextDirectiveRef)
		if err != nil {
			return nil, err
		}

		return fields, nil
	}

	idFieldRef, err := r.findIDField(walker.EnclosingTypeDefinition, fd)
	return []int{idFieldRef}, err
}

func (r *rpcPlanningContext) parseFieldArguments(walker *astvisitor.Walker, fd int, fieldArgs []int) ([]fieldArgument, error) {
	result := make([]fieldArgument, 0, len(fieldArgs))
	for _, fieldArgRef := range fieldArgs {
		arg := r.operation.Arguments[fieldArgRef]
		fieldArg := r.operation.ArgumentNameString(fieldArgRef)
		fieldType := arg.Value.Kind

		argDefRef := r.definition.NodeFieldDefinitionArgumentDefinitionByName(
			walker.EnclosingTypeDefinition,
			r.definition.FieldDefinitionNameBytes(fd),
			r.operation.ArgumentNameBytes(fieldArgRef),
		)

		if argDefRef == ast.InvalidRef {
			return nil, fmt.Errorf("unable to resolve argument input value definition for argument %s", fieldArg)
		}

		jsonValue := fieldArg
		if fieldType == ast.ValueKindVariable {
			jsonValue = r.operation.Input.ByteSliceString(r.operation.VariableValues[arg.Value.Ref].Name)
		}

		result = append(result, fieldArgument{
			fieldDefinitionRef:    fd,
			argumentDefinitionRef: argDefRef,
			parentTypeNode:        walker.EnclosingTypeDefinition,
			jsonPath:              jsonValue,
		})

	}

	return result, nil

}

func (r *rpcPlanningContext) getFieldsFromContext(parentNode ast.Node, contextRef int) ([]int, error) {
	val, exists := r.definition.DirectiveArgumentValueByName(contextRef, []byte("context"))
	if !exists {
		return nil, fmt.Errorf("context directive argument not found")
	}

	fieldsString := r.definition.ValueContentString(val)

	walker := astvisitor.WalkerFromPool()
	defer walker.Release()

	v := newRequiredFieldsVisitor(walker, &RPCMessage{}, r)
	if err := v.visitRequiredFields(r.definition, parentNode.NameString(r.definition), fieldsString); err != nil {
		return nil, err
	}

	return v.fieldDefinitionRefs, nil
}

func (r *rpcPlanningContext) findIDField(parentNode ast.Node, fd int) (int, error) {
	switch parentNode.Kind {
	case ast.NodeKindObjectTypeDefinition:
		o := r.definition.ObjectTypeDefinitions[parentNode.Ref]
		result := slices.Collect(r.filterIDFieldsFunc(o, fd))

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

func (r *rpcPlanningContext) filterIDFieldsFunc(o ast.ObjectTypeDefinition, fd int) func(yield func(int) bool) {
	fieldRefs := o.FieldsDefinition.Refs
	return func(yield func(int) bool) {
		for _, ref := range fieldRefs {
			if ref == fd {
				continue
			}

			typeName := r.definition.FieldDefinitionTypeNameString(ref)
			if typeName != "ID" {
				continue
			}

			if !yield(ref) {
				return
			}
		}
	}
}

// nodeByTypeRef is a helper function to resolve the underlying type node for a given type reference.
func (r *rpcPlanningContext) nodeByTypeRef(typeRef int) (ast.Node, bool) {
	underlyingTypeName := r.definition.ResolveTypeNameString(typeRef)
	return r.definition.NodeByNameStr(underlyingTypeName)
}

type resolveRPCCallConfig struct {
	resolveConfig    *ResolveRPCTypeField
	resolvedField    *resolvedField
	contextMessage   *RPCMessage
	fieldArgsMessage *RPCMessage
}

func (r *rpcPlanningContext) resolveRequiredFields(typeName string, requiredFieldSelection int) (*RPCMessage, error) {
	walker := astvisitor.WalkerFromPool()
	defer walker.Release()
	message := &RPCMessage{
		Name: typeName,
	}

	rfv := newRequiredFieldsVisitor(walker, message, r)
	if err := rfv.visitWithMemberTypes(r.definition, typeName, r.operation.SelectionSetFieldSetString(requiredFieldSelection), nil); err != nil {
		return nil, err
	}
	return message, nil
}

// createResolverRPCCalls creates a new call for each resolved field.
func (r *rpcPlanningContext) createResolverRPCCalls(subgraphName string, resolvedFields []resolvedField) ([]RPCCall, error) {
	// We need to create a new call for each resolved field.
	calls := make([]RPCCall, 0, len(resolvedFields))

	for _, resolvedField := range resolvedFields {
		resolveConfig := r.mapping.FindResolveTypeFieldMapping(
			r.definition.ObjectTypeDefinitionNameString(resolvedField.parentTypeRef),
			r.operation.FieldNameString(resolvedField.fieldRef),
		)

		if resolveConfig == nil {
			return nil, fmt.Errorf("resolve config not found for type: %s, field: %s", r.definition.ResolveTypeNameString(resolvedField.parentTypeRef), r.operation.FieldAliasString(resolvedField.fieldRef))
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
			typeDefNode, found := r.definition.NodeByNameStr(r.definition.ResolveTypeNameString(resolvedField.parentTypeRef))
			if !found {
				return nil, fmt.Errorf("type definition node not found for type: %s", r.definition.ResolveTypeNameString(resolvedField.parentTypeRef))
			}

			field, err := r.buildField(
				typeDefNode,
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

		fieldArgsMessage.Fields = make(RPCFields, len(resolvedField.fieldArguments))
		for i := range resolvedField.fieldArguments {
			field, err := r.createRPCFieldFromFieldArgument(resolvedField.fieldArguments[i])

			if err != nil {
				return nil, err
			}

			fieldArgsMessage.Fields[i] = field
		}

		calls = append(calls, call)
	}

	return calls, nil
}

const (
	resultFieldName    = "result"
	contextFieldName   = "context"
	fieldArgsFieldName = "field_args"
)

func (r *rpcPlanningContext) newResolveRPCCall(config *resolveRPCCallConfig) (RPCCall, error) {
	resolveConfig := config.resolveConfig
	resolvedField := config.resolvedField

	underlyingTypeRef := r.definition.ResolveUnderlyingType(resolvedField.fieldDefinitionTypeRef)
	dataType := r.toDataType(&r.definition.Types[underlyingTypeRef])

	var responseFieldsMessage *RPCMessage
	if dataType == DataTypeMessage {
		var err error
		responseFieldsMessage, err = r.resolveRequiredFields(
			r.definition.ResolveTypeNameString(underlyingTypeRef),
			resolvedField.requiredFieldSelection,
		)

		if err != nil {
			return RPCCall{}, err
		}
	}

	response := RPCMessage{
		Name: resolveConfig.Response,
		Fields: RPCFields{
			{
				Name:          resultFieldName,
				ProtoTypeName: DataTypeMessage,
				JSONPath:      resultFieldName,
				Repeated:      true,
				Message: &RPCMessage{
					Name: resolveConfig.RPC + "Result",
					Fields: RPCFields{
						{
							Name:          resolveConfig.FieldMappingData.TargetName,
							ProtoTypeName: dataType,
							JSONPath:      r.operation.FieldAliasOrNameString(resolvedField.fieldRef),
							Message:       responseFieldsMessage,
							Optional:      !r.definition.TypeIsNonNull(resolvedField.fieldDefinitionTypeRef),
						},
					},
				},
			},
		},
	}

	return RPCCall{
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
