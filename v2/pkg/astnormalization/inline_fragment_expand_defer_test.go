package astnormalization

import "testing"

func TestInlineFragmentExpandDefer(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(t, InlineFragmentExpandDefer, testDefinition, `
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
								name  @defer_internal(id: "1")
							}
						}
					}`)
	})
	t.Run("with interface type", func(t *testing.T) {
		run(t, InlineFragmentExpandDefer, testDefinition, `
					query pet {
						pet {
							... on Dog @defer {
								name
								nickname
								... @defer {
									barkVolume
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
								name @defer_internal(id: "1")
								nickname @defer_internal(id: "1")
								... {
									barkVolume @defer_internal(id: "2", parentDeferId: "1")
								}
							}
							... on Cat {
								name @defer_internal(id: "3")
								meowVolume @defer_internal(id: "3")
							}
						}
					}`)
	})
}
