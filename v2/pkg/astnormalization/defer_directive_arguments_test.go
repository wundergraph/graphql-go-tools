package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestDeferDirectiveArguments(t *testing.T) {
	normalize := func(t *testing.T, operation, variables string) (string, operationreport.Report) {
		t.Helper()

		definition := unsafeparser.ParseGraphqlDocumentString(testDefinition)
		require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))

		document := unsafeparser.ParseGraphqlDocumentString(operation)
		if variables != "" {
			document.Input.Variables = []byte(variables)
		}

		report := operationreport.Report{}
		NewWithOpts(WithInlineFragmentSpreads(), WithEnableDefer()).NormalizeOperation(&document, &definition, &report)

		printed, err := astprinter.PrintString(&document)
		require.NoError(t, err)
		return printed, report
	}

	t.Run("[A1] absent variable without default uses the argument default true", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: Boolean) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{}`)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: Boolean){dog {name nickname @__defer_internal(id: 1)}}`, printed)
	})

	t.Run("[A2] absent variable without variables input uses the argument default true", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: Boolean) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, ``)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: Boolean){dog {name nickname @__defer_internal(id: 1)}}`, printed)
	})

	t.Run("[A3] absent variable uses the variable default false", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: Boolean = false) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{}`)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: Boolean = false){dog {name nickname}}`, printed)
	})

	t.Run("[A4] explicit true overrides the variable default false", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: Boolean = false) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{"d":true}`)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: Boolean = false){dog {name nickname @__defer_internal(id: 1)}}`, printed)
	})

	t.Run("[A5] explicit false disables the defer", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: Boolean) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{"d":false}`)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: Boolean){dog {name nickname}}`, printed)
	})

	t.Run("[N1] explicit null is an error", func(t *testing.T) {
		_, report := normalize(t, `
			query Q($d: Boolean) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{"d":null}`)
		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `Argument "if" of non-null type "Boolean!" must not be null.`,
				Path: ast.Path{
					{
						Kind:      ast.FieldName,
						FieldName: []byte("query"),
					},
					{
						Kind:      ast.FieldName,
						FieldName: []byte("dog"),
					},
				},
				Locations: []operationreport.Location{{
					Line:   5,
					Column: 21,
				}},
			}},
		}, report)
	})

	t.Run("[N2] explicit null is an error although the variable has a default", func(t *testing.T) {
		_, report := normalize(t, `
			query Q($d: Boolean = false) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{"d":null}`)
		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `Argument "if" of non-null type "Boolean!" must not be null.`,
				Path: ast.Path{
					{
						Kind:      ast.FieldName,
						FieldName: []byte("query"),
					},
					{
						Kind:      ast.FieldName,
						FieldName: []byte("dog"),
					},
				},
				Locations: []operationreport.Location{{
					Line:   5,
					Column: 21,
				}},
			}},
		}, report)
	})

	t.Run("[N3] absent variable with a null default is an error", func(t *testing.T) {
		_, report := normalize(t, `
			query Q($d: Boolean = null) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{}`)
		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `Argument "if" of non-null type "Boolean!" must not be null.`,
				Path: ast.Path{
					{
						Kind:      ast.FieldName,
						FieldName: []byte("query"),
					},
					{
						Kind:      ast.FieldName,
						FieldName: []byte("dog"),
					},
				},
				Locations: []operationreport.Location{{
					Line:   5,
					Column: 21,
				}},
			}},
		}, report)
	})

	t.Run("[N4] null variable in a fragment defer omits an unknown location", func(t *testing.T) {
		_, report := normalize(t, `
			fragment F on Dog {
				... @defer(if: $d) { nickname }
			}
			query Q($d: Boolean) {
				dog { ...F }
			}`, `{"d":null}`)
		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `Argument "if" of non-null type "Boolean!" must not be null.`,
				Path: ast.Path{
					{
						Kind:      ast.FieldName,
						FieldName: []byte("query"),
					},
					{
						Kind:      ast.FieldName,
						FieldName: []byte("dog"),
					},
					{
						Kind:        ast.InlineFragmentName,
						FieldName:   []byte("Dog"),
						FragmentRef: 2,
					},
				},
			}},
		}, report)
	})

	t.Run("[K1] absent non-null variable keeps the directive for variables validation", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: Boolean!) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{}`)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: Boolean!){dog {name ... @defer(if: $d){nickname}}}`, printed)
	})

	t.Run("[K2] null non-null variable keeps the directive for variables validation", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: Boolean!) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{"d":null}`)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: Boolean!){dog {name ... @defer(if: $d){nickname}}}`, printed)
	})

	t.Run("[K3] literal null keeps the directive for operation validation", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q {
				dog {
					name
					... @defer(if: null) { nickname }
				}
			}`, ``)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q {dog {name ... @defer(if: null){nickname}}}`, printed)
	})

	t.Run("[K4] non-boolean variable value keeps the directive for variables validation", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: Boolean) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{"d":"yes"}`)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: Boolean){dog {name ... @defer(if: $d){nickname}}}`, printed)
	})

	t.Run("[K5] non-boolean variable type keeps the directive for operation validation", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q($d: String) {
				dog {
					name
					... @defer(if: $d) { nickname }
				}
			}`, `{"d":"yes"}`)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q($d: String){dog {name ... @defer(if: $d){nickname}}}`, printed)
	})

	t.Run("[L1] null label does not panic and gives an unlabeled defer", func(t *testing.T) {
		printed, report := normalize(t, `
			query Q {
				dog {
					name
					... @defer(label: null) { nickname }
				}
			}`, ``)
		assert.Equal(t, operationreport.Report{}, report)
		assert.Equal(t, `query Q {dog {name nickname @__defer_internal(id: 1)}}`, printed)
	})
}
