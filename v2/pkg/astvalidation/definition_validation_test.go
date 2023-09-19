package astvalidation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
)

func runDefinitionValidation(t *testing.T, definitionInput string, expectation ValidationState, rules ...Rule) {
	definition, report := astparser.ParseGraphqlDocumentString(definitionInput)
	require.False(t, report.HasErrors())

	err := asttransform.MergeDefinitionWithBaseSchema(&definition)
	require.NoError(t, err)

	validator := &DefinitionValidator{}
	for _, rule := range rules {
		validator.RegisterRule(rule)
	}

	result := validator.Validate(&definition, &report)
	assert.Equal(t, expectation, result)
}
