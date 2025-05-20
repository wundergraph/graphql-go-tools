package grpcdatasource

type (
	// RPCConfigMap is a map of RPC names to RPC configurations
	RPCConfigMap map[string]RPCConfig
	// FieldMap defines the mapping between a GraphQL field and a gRPC field
	FieldMap map[string]FieldMapData
)

type GRPCMapping struct {
	// Service is the name of the gRPC service to use
	Service string
	// QueryRPCs maps GraphQL query fields to the corresponding gRPC RPC configurations
	QueryRPCs RPCConfigMap
	// MutationRPCs maps GraphQL mutation fields to the corresponding gRPC RPC configurations
	MutationRPCs RPCConfigMap
	// SubscriptionRPCs maps GraphQL subscription fields to the corresponding gRPC RPC configurations
	SubscriptionRPCs RPCConfigMap
	// EntityRPCs defines how GraphQL types are resolved as entities using specific RPCs
	EntityRPCs map[string]EntityRPCConfig
	// Fields defines the field mappings between GraphQL types and gRPC messages
	Fields map[string]FieldMap
	// EnumValues defines the enum values for each enum type
	EnumValues map[string][]EnumValueMapping
}

type EnumValueMapping struct {
	Value       string
	TargetValue string
}

type GRPCConfiguration struct {
	Mapping  *GRPCMapping
	Compiler *RPCCompiler
}

// RPCConfig defines the configuration for a specific RPC operation
type RPCConfig struct {
	// RPC is the name of the RPC method to call
	RPC string
	// Request is the name of the request message type
	Request string
	// Response is the name of the response message type
	Response string
}

// EntityRPCConfig defines the configuration for entity lookups
type EntityRPCConfig struct {
	// Key is a list of field names that uniquely identify the entity
	Key string
	// RPCConfig is the embedded configuration for the RPC operation
	RPCConfig
}

type FieldMapData struct {
	TargetName       string
	ArgumentMappings FieldArgumentMap
}

// FieldArgumentMap defines the mapping between a GraphQL field argument and a gRPC field
type FieldArgumentMap map[string]string

// ResolveFieldMapping resolves the gRPC field name for a given GraphQL field name and type
func (g *GRPCMapping) ResolveFieldMapping(typeName string, fieldName string) (string, bool) {
	if g == nil || g.Fields == nil {
		return "", false
	}

	fieldMap, ok := g.Fields[typeName]
	if !ok || fieldMap == nil {
		return "", false
	}

	field, ok := fieldMap[fieldName]
	if !ok || field.TargetName == "" {
		return "", false
	}

	return field.TargetName, true
}

func (g *GRPCMapping) ResolveFieldArgumentMapping(typeName, fieldName, argumentName string) (string, bool) {
	if g == nil || g.Fields == nil {
		return "", false
	}

	fieldMap, ok := g.Fields[typeName]
	if !ok || fieldMap == nil {
		return "", false
	}

	args, ok := fieldMap[fieldName]
	if !ok || args.ArgumentMappings == nil {
		return "", false
	}

	grpcFieldName, ok := args.ArgumentMappings[argumentName]
	return grpcFieldName, ok
}

func (g *GRPCMapping) ResolveEnumValue(enumName, enumValue string) (string, bool) {
	if g == nil || g.EnumValues == nil {
		return "", false
	}

	enumValues, ok := g.EnumValues[enumName]
	if !ok {
		return "", false
	}

	for _, ev := range enumValues {
		if ev.Value == enumValue {
			return ev.TargetValue, true
		}

		if ev.TargetValue == enumValue {
			return ev.Value, true
		}
	}

	return "", false
}
