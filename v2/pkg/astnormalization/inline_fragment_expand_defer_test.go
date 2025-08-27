package astnormalization

import "testing"

func TestInlineFragmentExpandDefer(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(t, inlineFragmentExpandDefer, testDefinition, `
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
		runWithOptions(t, inlineFragmentExpandDefer, testDefinition, `
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
								name @defer_internal(id: "1")
								nickname @defer_internal(id: "1")
								... {
									barkVolume @defer_internal(id: "2", parentDeferId: "1")
								}
							}
							... on Dog {
								... {
									extra @defer_internal(id: "3") {	
										noString @defer_internal(id: "3")
									}
								}
								... {
									extra @defer_internal(id: "4") {	
										string @defer_internal(id: "4")
										noString @defer_internal(id: "4")
									}
								}
							}
							... on Cat {
								name @defer_internal(id: "5")
								meowVolume @defer_internal(id: "5")
							}
						}
					}`, runOptions{indent: true})
	})
}
