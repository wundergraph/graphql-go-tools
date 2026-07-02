package introspection_datasource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
)

const capabilitiesSchema = `type Query { hello: String }`

func newCapabilitiesFactory(t *testing.T, opts IntrospectionOptions) *IntrospectionConfigFactory {
	t.Helper()
	def := unsafeparser.ParseGraphqlDocumentString(capabilitiesSchema)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchemaOptions(&def, asttransform.MergeOptions{
		ServiceCapabilities: opts.ServiceCapabilities,
	}))
	f, err := NewIntrospectionConfigFactoryWithOptions(&def, opts)
	require.NoError(t, err)
	return f
}

// With the feature enabled, a __schema request serializes the advertised
// capabilities using the operator-configured default error behavior.
func TestServiceCapabilities_Load_Enabled(t *testing.T) {
	f := newCapabilitiesFactory(t, IntrospectionOptions{ServiceCapabilities: true, DefaultErrorBehavior: "NULL"})
	source := &Source{introspectionData: f.introspectionData}
	responseData, err := source.Load(context.Background(), nil, []byte(`{"request_type":1}`))
	require.NoError(t, err)

	// source.Load marshals the Schema directly, so capabilities is top-level.
	var resp struct {
		Capabilities []introspection.Capability `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(responseData, &resp))
	nv := "NULL"
	assert.Equal(t, []introspection.Capability{
		{Identifier: "graphql.onError", TypeName: "__Capability"},
		{Identifier: "graphql.defaultErrorBehavior", Value: &nv, TypeName: "__Capability"},
	}, resp.Capabilities)
}

// With the feature disabled, a __schema request emits no capabilities key.
func TestServiceCapabilities_Load_Disabled(t *testing.T) {
	f := newCapabilitiesFactory(t, IntrospectionOptions{})
	source := &Source{introspectionData: f.introspectionData}
	responseData, err := source.Load(context.Background(), nil, []byte(`{"request_type":1}`))
	require.NoError(t, err)
	assert.NotContains(t, string(responseData), `"capabilities"`)
}

// When enabled, the data source can resolve __schema { capabilities } and the
// __Capability fields (so the query is plannable); when disabled it cannot.
func TestServiceCapabilities_ChildNodes(t *testing.T) {
	enabled := newCapabilitiesFactory(t, IntrospectionOptions{ServiceCapabilities: true}).BuildDataSourceConfigurations()[0]
	assert.True(t, enabled.HasChildNode("__Schema", "capabilities"))
	assert.True(t, enabled.HasChildNode("__Capability", "identifier"))
	assert.True(t, enabled.HasChildNode("__Capability", "value"))

	disabled := newCapabilitiesFactory(t, IntrospectionOptions{}).BuildDataSourceConfigurations()[0]
	assert.False(t, disabled.HasChildNode("__Schema", "capabilities"))
	assert.False(t, disabled.HasChildNode("__Capability", "identifier"))
}
