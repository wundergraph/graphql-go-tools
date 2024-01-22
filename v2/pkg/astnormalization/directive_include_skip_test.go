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
}
