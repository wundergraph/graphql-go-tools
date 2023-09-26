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
}
