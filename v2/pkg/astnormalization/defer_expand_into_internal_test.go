package astnormalization

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func TestDeferExpandIntoInternal(t *testing.T) {
	deferExpandIntoInternalWithIgnore := func(walker *astvisitor.Walker) {
		visitor := deferExpandIntoInternalVisitor{
			Walker: walker,
			ignore: true,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterInlineFragmentVisitor(&visitor)
		walker.RegisterEnterSelectionSetVisitor(&visitor)
	}

	deferExpandIntoInternalWithDisable := func(walker *astvisitor.Walker) {
		deferExpandIntoInternalWithDisabled(walker, true)
	}

	t.Run("simple - enabled by default", func(t *testing.T) {
		run(t, deferExpandIntoInternal, testDefinition, `
					query dog {
						dog {
							... @defer {
								name
							}
						}
					}`,
			`
					query dog {
						dog {
							... {
								name  @__defer_internal(id: 1)
							}
						}
					}`)
	})
	t.Run("simple - enabled by default but ignored", func(t *testing.T) {
		run(t, deferExpandIntoInternalWithIgnore, testDefinition, `
					query dog {
						dog {
							... @defer {
								name
							}
						}
					}`,
			`
					query dog {
						dog {
							... {
								name
							}
						}
					}`)
	})
	t.Run("simple - explicitly enabled", func(t *testing.T) {
		run(t, deferExpandIntoInternal, testDefinition, `
					query dog {
						dog {
							... @defer(if: true) {
								name
							}
						}
					}`,
			`
					query dog {
						dog {
							... {
								name  @__defer_internal(id: 1)
							}
						}
					}`)
	})
	t.Run("simple - explicitly enabled but ignored", func(t *testing.T) {
		run(t, deferExpandIntoInternalWithIgnore, testDefinition, `
					query dog {
						dog {
							... @defer(if: true) {
								name
							}
						}
					}`,
			`
					query dog {
						dog {
							... {
								name
							}
						}
					}`)
	})
	t.Run("simple - explicitly disabled", func(t *testing.T) {
		run(t, deferExpandIntoInternal, testDefinition, `
					query dog {
						dog {
							... @defer(if: false) {
								name
							}
						}
					}`,
			`
					query dog {
						dog {
							... {
								name
							}
						}
					}`)
	})
	t.Run("simple - disabled no variable value", func(t *testing.T) {
		run(t, deferExpandIntoInternal, testDefinition, `
					query dog($defer: Boolean!) {
						dog {
							... @defer(if: $defer) {
								name
							}
						}
					}`,
			`
					query dog($defer: Boolean!) {
						dog {
							... {
								name
							}
						}
					}`)
	})
	t.Run("simple - enabled via variable", func(t *testing.T) {
		runWithVariables(t, deferExpandIntoInternal, testDefinition, `
					query dog($defer: Boolean!) {
						dog {
							... @defer(if: $defer) {
								name
							}
						}
					}`,
			`
					query dog($defer: Boolean!) {
						dog {
							... {
								name  @__defer_internal(id: 1)
							}
						}
					}`, `{"defer": true}`)
	})
	t.Run("simple - enabled via variable but ignored", func(t *testing.T) {
		runWithVariables(t, deferExpandIntoInternalWithIgnore, testDefinition, `
					query dog($defer: Boolean!) {
						dog {
							... @defer(if: $defer) {
								name
							}
						}
					}`,
			`
					query dog($defer: Boolean!) {
						dog {
							... {
								name
							}
						}
					}`, `{"defer": true}`)
	})
	t.Run("simple - enabled via variable but disabled", func(t *testing.T) {
		runWithVariables(t, deferExpandIntoInternalWithDisable, testDefinition, `
					query dog($defer: Boolean!) {
						dog {
							... @defer(if: $defer) {
								name
							}
						}
					}`,
			`
					query dog($defer: Boolean!) {
						dog {
							... {
								name
							}
						}
					}`, `{"defer": true}`)
	})
	t.Run("simple - disabled via variable", func(t *testing.T) {
		runWithVariables(t, deferExpandIntoInternal, testDefinition, `
					query dog($defer: Boolean!) {
						dog {
							... @defer(if: $defer) {
								name
							}
						}
					}`,
			`
					query dog($defer: Boolean!) {
						dog {
							... {
								name
							}
						}
					}`, `{"defer": false}`)
	})
	t.Run("simple - enabled via variable default value", func(t *testing.T) {
		run(t, deferExpandIntoInternal, testDefinition, `
					query dog($defer: Boolean = true) {
						dog {
							... @defer(if: $defer) {
								name
							}
						}
					}`,
			`
					query dog($defer: Boolean = true) {
						dog {
							... {
								name  @__defer_internal(id: 1)
							}
						}
					}`)
	})
	t.Run("simple - disabled via variable default value", func(t *testing.T) {
		run(t, deferExpandIntoInternal, testDefinition, `
					query dog($defer: Boolean = false) {
						dog {
							... @defer(if: $defer) {
								name
							}
						}
					}`,
			`
					query dog($defer: Boolean = false) {
						dog {
							... {
								name
							}
						}
					}`)
	})
	t.Run("with interface type", func(t *testing.T) {
		run(t, deferExpandIntoInternal, testDefinition, `
					query pet {
						pet {
							... on Dog @defer {
								name
								nickname
								... @defer {
									barkVolume
								}
							}
							... on Dog {
								... @defer {
									extra {
										noString
									}
								}
								... @defer {
									extra {
										string
										noString
									}
								}
							}
							... on Cat @defer {
								name
								meowVolume
							}
						}
					}`,
			`
					query pet {
						pet {
							... on Dog {
								name @__defer_internal(id: 1)
								nickname @__defer_internal(id: 1)
								... {
									barkVolume @__defer_internal(id: 2, parentDeferId: 1)
								}
							}
							... on Dog {
								... {
									extra @__defer_internal(id: 3) {	
										noString @__defer_internal(id: 3)
									}
								}
								... {
									extra @__defer_internal(id: 4) {	
										string @__defer_internal(id: 4)
										noString @__defer_internal(id: 4)
									}
								}
							}
							... on Cat {
								name @__defer_internal(id: 5)
								meowVolume @__defer_internal(id: 5)
							}
						}
					}`, withIndent())
	})
}
