package astnormalization

import "testing"

func TestDirectiveIncludeVisitor(t *testing.T) {
	t.Run("remove static include true on inline fragment", func(t *testing.T) {
		run(t, directiveIncludeSkip, testDefinition, `
				{
					dog {
						name: nickname
						... @include(if: true) {
							includeName: name @include(if: true)
							notIncludeName: name @include(if: false)
							notSkipName: name @skip(if: false)
							skipName: name @skip(if: true)
						}
					}
					notInclude: dog @include(if: false) {
						name
					}
					skip: dog @skip(if: true) {
						name
					}
				}`, `
				{
					dog {
						name: nickname
						... {
							includeName: name
							notSkipName: name
						}
					}
				}`)
	})

	t.Run("if node is last one replace selection with a typename", func(t *testing.T) {
		run(t, directiveIncludeSkip, testDefinition, `
				{
					dog {
						... @include(if: false) {
							includeName: name
						}
					}
					notInclude: dog {
						name @include(if: false)
					}
					skip: dog {
						name @skip(if: true)
					}
				}`, `
				{
					dog {__typename}
					notInclude: dog {__typename}
					skip: dog {__typename}
				}`)
	})
	t.Run("include variables true", func(t *testing.T) {
		runWithVariables(t, directiveIncludeSkip, testDefinition, `
				query($yes: Boolean!) {
					dog {
						... @include(if: $yes) {
							includeName: name
						}
					}
					withAlias: dog {
						name @include(if: $yes)
					}
				}`, `
				query($yes: Boolean!) {
					dog {
						... {
							includeName: name
						}
					}
					withAlias: dog {
						name
					}
				}`, `{"yes":true}`)
	})
	t.Run("include variables false", func(t *testing.T) {
		runWithVariables(t, directiveIncludeSkip, testDefinition, `
				query($no: Boolean!) {
					dog {
						... @include(if: $no) {
							includeName: name
						}
					}
					withAlias: dog {
						name @include(if: $no)
					}
				}`, `
				query($no: Boolean!){
					dog {
						__typename
					}
					withAlias: dog {
						__typename
					}
				}`, `{"no":false}`)
	})
	t.Run("include variables mixed", func(t *testing.T) {
		runWithVariables(t, directiveIncludeSkip, testDefinition, `
				query($yes: Boolean!, $no: Boolean!) {
					dog {
						... @include(if: $yes) {
							includeName: name
						}
					}
					withAlias: dog {
						name @include(if: $no)
					}
				}`, `
				query($yes: Boolean! $no: Boolean!){
					dog {
						__typename
					}
					withAlias: dog {
						name
					}
				}`, `{"yes":false,"no":true}`)
	})
	t.Run("skip variables true", func(t *testing.T) {
		runWithVariables(t, directiveIncludeSkip, testDefinition, `
				query($yes: Boolean!) {
					dog {
						... @skip(if: $yes) {
							includeName: name
						}
					}
					withAlias: dog {
						name @skip(if: $yes)
					}
				}`, `
				query($yes: Boolean!) {
					dog {
						__typename
					}
					withAlias: dog {
						__typename
					}
				}`, `{"yes":true}`)
	})
	t.Run("skip variables false", func(t *testing.T) {
		runWithVariables(t, directiveIncludeSkip, testDefinition, `
				query($no: Boolean!) {
					dog {
						... @skip(if: $no) {
							includeName: name
						}
					}
					withAlias: dog {
						name @skip(if: $no)
					}
				}`, `
				query($no: Boolean!){
					dog {
						... {
							includeName: name
						}
					}
					withAlias: dog {
						name
					}
				}`, `{"no":false}`)
	})
	t.Run("skip variables mixed", func(t *testing.T) {
		runWithVariables(t, directiveIncludeSkip, testDefinition, `
				query($yes: Boolean!, $no: Boolean!) {
					dog {
						... @skip(if: $yes) {
							includeName: name
						}
					}
					withAlias: dog {
						name @skip(if: $no)
					}
				}`, `
				query($yes: Boolean!, $no: Boolean!) {
					dog {
						__typename
					}
					withAlias: dog {
						name
					}
				}`, `{"yes":true,"no":false}`)
	})
	t.Run("skip include variables mixed", func(t *testing.T) {
		runWithVariables(t, directiveIncludeSkip, testDefinition, `
				query($yes: Boolean!, $no: Boolean!) {
					dog {
						... @skip(if: $yes) {
							includeName: name
						}
					}
					withAlias: dog {
						name @include(if: $no)
					}
				}`, `
				query($yes: Boolean!, $no: Boolean!) {
					dog {
						__typename
					}
					withAlias: dog {
						__typename
					}
				}`, `{"yes":true,"no":false}`)
	})
	t.Run("skip include variables mixed reverse", func(t *testing.T) {
		runWithVariables(t, directiveIncludeSkip, testDefinition, `
				query($yes: Boolean!, $no: Boolean!) {
					dog {
						... @include(if: $yes) {
							includeName: name
						}
					}
					withAlias: dog {
						name @skip(if: $no)
					}
				}`, `
				query($yes: Boolean!, $no: Boolean!) {
					dog {
						... {
							includeName: name
						}
					}
					withAlias: dog {
						name
					}
				}`, `{"yes":true,"no":false}`)
	})
}
