package plan

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestScopedFieldsForType(t *testing.T) {
	metadata := FederationMetaData{
		RequestScopedFields: []RequestScopedField{
			{
				TypeName:  "Query",
				FieldName: "currentViewer",
				L1Key:     "accounts.viewer",
			},
			{
				TypeName:  "Article",
				FieldName: "currentViewer",
				L1Key:     "accounts.viewer",
			},
			{
				TypeName:  "Article",
				FieldName: "workspace",
				L1Key:     "accounts.workspace",
			},
		},
	}

	assert.Equal(t, []RequestScopedField{
		{
			TypeName:  "Article",
			FieldName: "currentViewer",
			L1Key:     "accounts.viewer",
		},
		{
			TypeName:  "Article",
			FieldName: "workspace",
			L1Key:     "accounts.workspace",
		},
	}, metadata.RequestScopedFieldsForType("Article"))

	assert.Equal(t, []RequestScopedField(nil), metadata.RequestScopedFieldsForType("Product"))
}

func TestRequestScopedExportsForField(t *testing.T) {
	metadata := FederationMetaData{
		RequestScopedFields: []RequestScopedField{
			{
				TypeName:  "Query",
				FieldName: "currentViewer",
				L1Key:     "accounts.viewer",
			},
			{
				TypeName:  "Article",
				FieldName: "currentViewer",
				L1Key:     "accounts.viewer",
			},
			{
				TypeName:  "Article",
				FieldName: "workspace",
				L1Key:     "accounts.workspace",
			},
		},
	}

	assert.Equal(t, []RequestScopedField{
		{
			TypeName:  "Query",
			FieldName: "currentViewer",
			L1Key:     "accounts.viewer",
		},
	}, metadata.RequestScopedExportsForField("Query", "currentViewer"))

	assert.Equal(t, []RequestScopedField{
		{
			TypeName:  "Article",
			FieldName: "currentViewer",
			L1Key:     "accounts.viewer",
		},
	}, metadata.RequestScopedExportsForField("Article", "currentViewer"))

	assert.Equal(t, []RequestScopedField(nil), metadata.RequestScopedExportsForField("Article", "viewer"))
}

func TestRequestScopedRequiredFieldsByKey(t *testing.T) {
	metadata := FederationMetaData{
		RequestScopedFields: []RequestScopedField{
			{
				TypeName:  "Query",
				FieldName: "currentViewer",
				L1Key:     "accounts.viewer",
			},
			{
				TypeName:  "Article",
				FieldName: "currentViewer",
				L1Key:     "accounts.viewer",
			},
			{
				TypeName:  "Article",
				FieldName: "workspace",
				L1Key:     "accounts.workspace",
			},
		},
	}

	assert.Equal(t, map[string][]RequestScopedField{
		"accounts.viewer": {
			{
				TypeName:  "Query",
				FieldName: "currentViewer",
				L1Key:     "accounts.viewer",
			},
			{
				TypeName:  "Article",
				FieldName: "currentViewer",
				L1Key:     "accounts.viewer",
			},
		},
		"accounts.workspace": {
			{
				TypeName:  "Article",
				FieldName: "workspace",
				L1Key:     "accounts.workspace",
			},
		},
	}, metadata.RequestScopedRequiredFieldsByKey())
}

func TestValidateRequestScopedFieldsReturnsMandatoryKeyError(t *testing.T) {
	warnings, err := ValidateRequestScopedFields([]RequestScopedField{
		{
			TypeName:  "Query",
			FieldName: "currentViewer",
			L1Key:     "",
		},
	})

	assert.Equal(t, []string(nil), warnings)
	assert.Equal(t, errors.New(`@requestScoped field Query.currentViewer has empty L1Key`), err)
}

func TestValidateRequestScopedFieldsReturnsSingleFieldWarning(t *testing.T) {
	warnings, err := ValidateRequestScopedFields([]RequestScopedField{
		{
			TypeName:  "Query",
			FieldName: "currentViewer",
			L1Key:     "accounts.viewer",
		},
	})

	assert.Equal(t, []string{
		`@requestScoped key "accounts.viewer" appears on only one field: Query.currentViewer`,
	}, warnings)
	assert.Equal(t, nil, err)
}

func TestValidateRequestScopedFieldsReturnsNoWarningForSharedKey(t *testing.T) {
	warnings, err := ValidateRequestScopedFields([]RequestScopedField{
		{
			TypeName:  "Query",
			FieldName: "currentViewer",
			L1Key:     "accounts.viewer",
		},
		{
			TypeName:  "Article",
			FieldName: "currentViewer",
			L1Key:     "accounts.viewer",
		},
	})

	assert.Equal(t, []string(nil), warnings)
	assert.Equal(t, nil, err)
}
