package astvalidation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestDeferStreamHaveUniqueLabels(t *testing.T) {
	t.Run("[L1] label with variable if reserves its value", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q($enabled: Boolean!) {
				dog {
					...fragment1 @defer(label: "a", if: $enabled)
					...fragment2 @defer(label: "a")
				}
			}
			fragment fragment1 on Dog { name }
			fragment fragment2 on Dog { nickname }`, "", "")

		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" label "a" must be unique, but was already used on "@defer" directive`,
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
				Locations: []operationreport.Location{
					{
						Line:   4,
						Column: 19,
					},
					{
						Line:   5,
						Column: 19,
					},
				},
			}},
		}, report)
	})

	t.Run("[L2] label with if false reserves its value", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q {
				dog {
					...fragment1 @defer(label: "a", if: false)
					...fragment2 @defer(label: "a")
				}
			}
			fragment fragment1 on Dog { name }
			fragment fragment2 on Dog { nickname }`, "", "")

		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" label "a" must be unique, but was already used on "@defer" directive`,
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
				Locations: []operationreport.Location{
					{
						Line:   4,
						Column: 19,
					},
					{
						Line:   5,
						Column: 19,
					},
				},
			}},
		}, report)
	})

	t.Run("[L3] defer and conditional stream share one label namespace", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q($enabled: Boolean!) {
				dog {
					...fragment1 @defer(label: "a")
					extras @stream(label: "a", if: $enabled) { string }
				}
			}
			fragment fragment1 on Dog { name }`, "", "")

		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@stream" label "a" must be unique, but was already used on "@defer" directive`,
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
						Kind:      ast.FieldName,
						FieldName: []byte("extras"),
					},
				},
				Locations: []operationreport.Location{
					{
						Line:   4,
						Column: 19,
					},
					{
						Line:   5,
						Column: 13,
					},
				},
			}},
		}, report)
	})

	t.Run("[L4] duplicate in an operation that the request did not select is valid", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query A {
				dog {
					...fragment1 @defer(label: "a")
				}
			}
			query B {
				dog {
					...fragment1 @defer(label: "a")
					...fragment2 @defer(label: "a", if: false)
				}
			}
			fragment fragment1 on Dog { name }
			fragment fragment2 on Dog { nickname }`, "", "A")

		assert.Equal(t, operationreport.Report{}, report)
	})

	t.Run("[L5] disabled duplicate in the selected operation is an error", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query A {
				dog {
					...fragment1 @defer(label: "a")
				}
			}
			query B {
				dog {
					...fragment1 @defer(label: "a")
					...fragment2 @defer(label: "a", if: false)
				}
			}
			fragment fragment1 on Dog { name }
			fragment fragment2 on Dog { nickname }`, "", "B")

		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" label "a" must be unique, but was already used on "@defer" directive`,
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
				Locations: []operationreport.Location{
					{
						Line:   9,
						Column: 19,
					},
					{
						Line:   10,
						Column: 19,
					},
				},
			}},
		}, report)
	})

	t.Run("[L6] label in a fragment defined before the operation", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			fragment fragment1 on Dog {
				... @defer(label: "a") { name }
			}
			query Q {
				dog {
					...fragment1
					... @defer(label: "a") { nickname }
				}
			}`, "", "")

		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" label "a" must be unique, but was already used on "@defer" directive`,
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
				Locations: []operationreport.Location{
					{
						Line:   3,
						Column: 9,
					},
					{
						Line:   8,
						Column: 10,
					},
				},
			}},
		}, report)
	})

	t.Run("[L7] one labeled fragment spread twice is valid", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q {
				dog { ...fragment1 }
				second: dog { ...fragment1 }
			}
			fragment fragment1 on Dog {
				... @defer(label: "a") { name }
			}`, "", "")

		assert.Equal(t, operationreport.Report{}, report)
	})

	t.Run("[L8] the same labeled spread directive written twice is an error", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q {
				dog { ...F @defer(label: "a") }
				dog2: dog { ...F @defer(label: "a") }
			}
			fragment F on Dog { name }`, "", "")

		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" label "a" must be unique, but was already used on "@defer" directive`,
				Path: ast.Path{
					{
						Kind:      ast.FieldName,
						FieldName: []byte("query"),
					},
					{
						Kind:      ast.FieldName,
						FieldName: []byte("dog2"),
					},
				},
				Locations: []operationreport.Location{
					{
						Line:   3,
						Column: 16,
					},
					{
						Line:   4,
						Column: 22,
					},
				},
			}},
		}, report)
	})

	t.Run("[L9] two defers with a null label literal are valid", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q {
				dog {
					... @defer(label: null) { name }
					... @defer(label: null) { nickname }
				}
			}`, "", "")

		assert.Equal(t, operationreport.Report{}, report)
	})

	t.Run("[L10] stream with a null label literal is valid", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q {
				dog {
					extras @stream(label: null) { string }
				}
			}`, "", "")

		assert.Equal(t, operationreport.Report{}, report)
	})

	t.Run("[L11] defer with an int label literal is an error", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q {
				dog {
					... @defer(label: 1) { name }
				}
			}`, "", "")

		assert.Equal(t, operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" label argument must be a static string value`,
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
					Line:   4,
					Column: 10,
				}},
			}},
		}, report)
	})

	t.Run("[L12] a label that the walker visits again after a skip removal is valid", func(t *testing.T) {
		report := normalizeWithDeferPrevalidation(t, testDefinition, `
			query Q {
				dog {
					... @defer(label: "a", if: false) { name }
					nickname @skip(if: true)
				}
			}`, "", "")

		assert.Equal(t, operationreport.Report{}, report)
	})
}
