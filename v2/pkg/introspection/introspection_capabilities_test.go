package introspection

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchema_Capabilities_OmittedWhenEmpty(t *testing.T) {
	s := NewSchema() // no capabilities
	b, err := json.Marshal(&s)
	assert.NoError(t, err)
	// capabilities key must be absent when empty (byte-identical to today)
	assert.NotContains(t, string(b), `"capabilities"`)
}

func TestSchema_Capabilities_Marshaled(t *testing.T) {
	s := NewSchema()
	val := "PROPAGATE"
	s.Capabilities = []Capability{
		{Identifier: "graphql.onError", TypeName: "__Capability"},
		{Identifier: "graphql.defaultErrorBehavior", Value: &val, TypeName: "__Capability"},
	}
	b, err := json.Marshal(&s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), `"capabilities":[{"identifier":"graphql.onError","description":null,"value":null,"__typename":"__Capability"},{"identifier":"graphql.defaultErrorBehavior","description":null,"value":"PROPAGATE","__typename":"__Capability"}]`)
}

func TestBuildServiceCapabilities(t *testing.T) {
	assert.Nil(t, BuildServiceCapabilities(false, "NULL")) // disabled => nothing

	got := BuildServiceCapabilities(true, "") // empty default => PROPAGATE
	pv := "PROPAGATE"
	assert.Equal(t, []Capability{
		{Identifier: "graphql.onError", TypeName: "__Capability"},
		{Identifier: "graphql.defaultErrorBehavior", Value: &pv, TypeName: "__Capability"},
	}, got)

	got = BuildServiceCapabilities(true, "NULL")
	nv := "NULL"
	assert.Equal(t, []Capability{
		{Identifier: "graphql.onError", TypeName: "__Capability"},
		{Identifier: "graphql.defaultErrorBehavior", Value: &nv, TypeName: "__Capability"},
	}, got)
}

func TestSchema_AddCapabilityType(t *testing.T) {
	s := NewSchema()
	s.AddCapabilityType()
	ft := s.TypeByName("__Capability")
	assert.NotNil(t, ft)
	assert.Equal(t, "__Capability", ft.Name)
	assert.Equal(t, OBJECT, ft.Kind)
	names := make([]string, 0, len(ft.Fields))
	for _, f := range ft.Fields {
		names = append(names, f.Name)
	}
	assert.Equal(t, []string{"identifier", "description", "value"}, names)

	// identifier is String!, description and value are nullable String
	assert.Equal(t, NONNULL, ft.Fields[0].Type.Kind)
	assert.Equal(t, SCALAR, ft.Fields[0].Type.OfType.Kind)
	assert.Equal(t, "String", *ft.Fields[0].Type.OfType.Name)
	assert.Equal(t, SCALAR, ft.Fields[1].Type.Kind)
	assert.Equal(t, "String", *ft.Fields[1].Type.Name)
	assert.Equal(t, SCALAR, ft.Fields[2].Type.Kind)

	// idempotent
	s.AddCapabilityType()
	count := 0
	for _, tpe := range s.Types {
		if tpe.Name == "__Capability" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestCapabilities_DefaultSyncsWithConfig(t *testing.T) {
	for _, def := range []string{"PROPAGATE", "NULL", "HALT"} {
		caps := BuildServiceCapabilities(true, def)
		// second entry is graphql.defaultErrorBehavior; its value must equal config
		assert.Equal(t, "graphql.defaultErrorBehavior", caps[1].Identifier)
		assert.NotNil(t, caps[1].Value)
		assert.Equal(t, def, *caps[1].Value)
	}
}
