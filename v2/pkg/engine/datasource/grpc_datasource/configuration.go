package grpcdatasource

import (
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/runes"
)

type (
	// RPCConfigMap is a map of RPC names to RPC configurations
	// The key is the field name in the GraphQL operation type (query, mutation, subscription).
	// The value is the RPC configuration for that field.
	RPCConfigMap map[string]RPCConfig
	// FieldMap defines the mapping between a GraphQL field and a gRPC field
	// The key is the field name in the GraphQL type.
	// The value is the FieldMapData for that field which contains the target name and the argument mappings.
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
	// The key is the type name and the value is a list of EntityRPCConfig for that type
	EntityRPCs map[string][]EntityRPCConfig
	// Fields defines the field mappings between GraphQL types and gRPC messages
	// The key is the type name and the value is a map of field name to FieldMapData for that type
	Fields map[string]FieldMap
	// EnumValues defines the enum values for each enum type
	// The key is the enum type name and the value is a list of EnumValueMapping for that enum type
	EnumValues map[string][]EnumValueMapping
}

// EnumValueMapping defines the mapping between a GraphQL enum value and a gRPC enum value
type EnumValueMapping struct {
	Value       string // The GraphQL enum value
	TargetValue string // The gRPC enum value
}

// GRPCConfiguration defines the configuration for a gRPC datasource
type GRPCConfiguration struct {
	Disabled bool         // Whether the RPC is disabled
	Mapping  *GRPCMapping // The mapping between GraphQL types and gRPC messages
	Compiler *RPCCompiler // The compiler for the RPC
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

// FieldMapData defines the mapping between a GraphQL field and a gRPC field
type FieldMapData struct {
	TargetName       string           // The name of the gRPC field
	ArgumentMappings FieldArgumentMap // The mapping between GraphQL field arguments and gRPC request arguments
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

func (g *GRPCMapping) ResolveEntityRPCConfig(typeName, key string) (RPCConfig, bool) {
	rpcConfig, ok := g.EntityRPCs[typeName]
	if !ok {
		return RPCConfig{}, false
	}

	for _, ei := range rpcConfig {
		if compareKeyFields(ei.Key, key) {
			return ei.RPCConfig, true
		}

	}

	return RPCConfig{}, false
}

type keySet map[string]struct{}

func (k keySet) add(keys ...string) {
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey != "" {
			k[trimmedKey] = struct{}{}
		}
	}
}

func (k keySet) equals(other keySet) bool {
	if len(k) != len(other) {
		return false
	}

	for key := range k {
		if _, ok := other[key]; !ok {
			return false
		}
	}

	return true
}

// We compare only top level key
func compareKeyFields(left, right string) bool {
	if left == right {
		return true
	}

	left = stripSelectionSets(left)
	right = stripSelectionSets(right)

	leftKeys := strings.Split(left, " ")
	rightKeys := strings.Split(right, " ")

	leftSet := make(keySet)
	leftSet.add(leftKeys...)

	rightSet := make(keySet)
	rightSet.add(rightKeys...)

	return leftSet.equals(rightSet)
}

func stripSelectionSets(keyString string) string {
	depth := 0

	var prev rune

	keyString = strings.ReplaceAll(keyString, ",", " ")

	var sb strings.Builder

	for _, r := range keyString {
		switch r {
		case runes.LBRACE:
			depth++
		case runes.RBRACE:
			if depth == 0 {
				continue
			}

			depth--
		default:
			if depth != 0 || (r == runes.SPACE && prev == runes.SPACE) {
				break
			}

			sb.WriteRune(r)
			prev = r
		}
	}

	return strings.TrimSpace(sb.String())
}
