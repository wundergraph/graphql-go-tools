package responsejsonschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestWithCustomScalarMappings_CopiesInput(t *testing.T) {
	mappings := map[string]json.RawMessage{
		"DateTime": json.RawMessage(`{"type":"string","format":"date-time"}`),
	}
	expected := append(json.RawMessage(nil), mappings["DateTime"]...)
	option := WithCustomScalarMappings(mappings)

	mappings["DateTime"][0] = '['
	mappings["AddedLater"] = json.RawMessage(`{"type":"number"}`)

	firstApplication := &options{}
	option(firstApplication)
	require.Equal(t, expected, firstApplication.customScalarMappings["DateTime"])
	require.NotContains(t, firstApplication.customScalarMappings, "AddedLater")

	firstApplication.customScalarMappings["DateTime"][0] = '['
	firstApplication.customScalarMappings["AddedByFirstApplication"] = json.RawMessage(`{"type":"boolean"}`)

	secondApplication := &options{}
	option(secondApplication)
	require.Equal(t, expected, secondApplication.customScalarMappings["DateTime"])
	require.NotContains(t, secondApplication.customScalarMappings, "AddedByFirstApplication")

	nilApplication := &options{customScalarMappings: map[string]json.RawMessage{"existing": nil}}
	WithCustomScalarMappings(nil)(nilApplication)
	require.Nil(t, nilApplication.customScalarMappings)
}

func TestBuildResponseSchema_RejectsInvalidInput(t *testing.T) {
	definition := parseDocument(t, `
		type Query {
			investigation: Investigation
		}

		type Investigation {
			id: ID!
		}
	`)
	operation := parseDocument(t, `
		query {
			firstInvestigation: investigation {
				id
			}
		}
	`)

	tests := []struct {
		name              string
		operation         bool
		definition        bool
		fieldPath         []string
		operationDocument string
		wantError         string
	}{
		{
			name:       "nil operation",
			definition: true,
			fieldPath:  []string{"firstInvestigation"},
			wantError:  "operation document is nil",
		},
		{
			name:      "nil definition",
			operation: true,
			fieldPath: []string{"firstInvestigation"},
			wantError: "definition document is nil",
		},
		{
			name:       "empty path",
			operation:  true,
			definition: true,
			wantError:  "field path is empty",
		},
		{
			name:       "unresolved alias path",
			operation:  true,
			definition: true,
			fieldPath:  []string{"investigation"},
			wantError:  `response field "investigation" not found`,
		},
		{
			name:              "absent operation definition",
			definition:        true,
			fieldPath:         []string{"firstInvestigation"},
			operationDocument: `fragment InvestigationFields on Investigation { id }`,
			wantError:         "operation document has no operation definition",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var operationInput, definitionInput = (*ast.Document)(nil), (*ast.Document)(nil)
			if test.operation {
				operationInput = &operation
			}
			if test.operationDocument != "" {
				operationWithoutDefinition := parseDocument(t, test.operationDocument)
				operationInput = &operationWithoutDefinition
			}
			if test.definition {
				definitionInput = &definition
			}

			_, err := Build(operationInput, definitionInput, test.fieldPath)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
