package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestScopedFieldsForType(t *testing.T) {
	// Symmetric model: every field annotated with @requestScoped is both a reader
	// and a writer of its L1 key. Fields with the same L1Key (same @requestScoped(key))
	// share the same L1 entry.
	meta := FederationMetaData{
		RequestScopedFields: []RequestScopedField{
			// Two fields in the viewer subgraph sharing the "viewer" key — both read/write
			// L1 under "viewer.viewer".
			{FieldName: "currentViewer", TypeName: "Query", L1Key: "viewer.viewer"},
			{FieldName: "currentViewer", TypeName: "Personalized", L1Key: "viewer.viewer"},
			// A separate key for locale
			{FieldName: "locale", TypeName: "Query", L1Key: "viewer.locale"},
			{FieldName: "locale", TypeName: "Personalized", L1Key: "viewer.locale"},
			// Unrelated key on a different type
			{FieldName: "theme", TypeName: "Settings", L1Key: "viewer.theme"},
		},
	}

	got := meta.RequestScopedFieldsForType("Personalized")
	assert.Len(t, got, 2)
	assert.Equal(t, "currentViewer", got[0].FieldName)
	assert.Equal(t, "locale", got[1].FieldName)

	got = meta.RequestScopedFieldsForType("Query")
	assert.Len(t, got, 2)

	got = meta.RequestScopedFieldsForType("Settings")
	assert.Len(t, got, 1)
	assert.Equal(t, "theme", got[0].FieldName)

	got = meta.RequestScopedFieldsForType("NonExistent")
	assert.Nil(t, got)
}

func TestRequestScopedExportsForField(t *testing.T) {
	// A field that is @requestScoped exports its own L1 key (symmetric — every
	// participating field writes its value to L1 after fetch, and other fields
	// with the same L1 key inject from it on later fetches).
	meta := FederationMetaData{
		RequestScopedFields: []RequestScopedField{
			{FieldName: "currentViewer", TypeName: "Query", L1Key: "viewer.viewer"},
			{FieldName: "currentViewer", TypeName: "Personalized", L1Key: "viewer.viewer"},
			{FieldName: "locale", TypeName: "Query", L1Key: "viewer.locale"},
			{FieldName: "theme", TypeName: "Settings", L1Key: "viewer.theme"},
		},
	}

	// Query.currentViewer is a @requestScoped field → it exports its L1 key.
	keys := meta.RequestScopedExportsForField("Query", "currentViewer")
	assert.Equal(t, []string{"viewer.viewer"}, keys)

	// Personalized.currentViewer is the same key — also exports.
	keys = meta.RequestScopedExportsForField("Personalized", "currentViewer")
	assert.Equal(t, []string{"viewer.viewer"}, keys)

	// Query.locale exports its own (different) key.
	keys = meta.RequestScopedExportsForField("Query", "locale")
	assert.Equal(t, []string{"viewer.locale"}, keys)

	// A field that is not @requestScoped exports nothing.
	keys = meta.RequestScopedExportsForField("Query", "nonExistent")
	assert.Nil(t, keys)

	// A @requestScoped field on a different type than queried — no match.
	keys = meta.RequestScopedExportsForField("Query", "theme")
	assert.Nil(t, keys)
}
