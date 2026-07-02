package asttransform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
)

func mergedDefinition(t *testing.T, options MergeOptions) *ast.Document {
	t.Helper()
	def, report := astparser.ParseGraphqlDocumentString(`type Query { hello: String }`)
	require.False(t, report.HasErrors())
	require.NoError(t, MergeDefinitionWithBaseSchemaOptions(&def, options))
	return &def
}

func schemaFieldNames(t *testing.T, def *ast.Document, typeName string) []string {
	t.Helper()
	node, ok := def.Index.FirstNodeByNameStr(typeName)
	require.True(t, ok, "type %s should exist", typeName)
	require.Equal(t, ast.NodeKindObjectTypeDefinition, node.Kind)
	refs := def.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, def.FieldDefinitionNameString(ref))
	}
	return names
}

// Disabled: __Schema has no capabilities field and __Capability type is absent.
func TestMergeBaseSchema_Capabilities_Disabled(t *testing.T) {
	def := mergedDefinition(t, MergeOptions{})
	names := schemaFieldNames(t, def, "__Schema")
	assert.Equal(t, []string{"description", "types", "queryType", "mutationType", "subscriptionType", "directives", "__typename"}, names)
	_, ok := def.Index.FirstNodeByNameStr("__Capability")
	assert.False(t, ok, "__Capability type must not exist when disabled")
}

// Enabled: __Schema gains a capabilities field and __Capability type is defined.
func TestMergeBaseSchema_Capabilities_Enabled(t *testing.T) {
	def := mergedDefinition(t, MergeOptions{ServiceCapabilities: true})
	names := schemaFieldNames(t, def, "__Schema")
	assert.Equal(t, []string{"description", "types", "queryType", "mutationType", "subscriptionType", "directives", "capabilities", "__typename"}, names)

	capFields := schemaFieldNames(t, def, "__Capability")
	assert.Equal(t, []string{"identifier", "description", "value", "__typename"}, capFields)
}
