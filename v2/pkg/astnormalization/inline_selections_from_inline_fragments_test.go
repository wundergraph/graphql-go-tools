package astnormalization

import "testing"

func TestResolveInlineFragments(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(t, inlineSelectionsFromInlineFragments, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							... {
								name
							}
							... on Dog {
								nickname
							}
							... {
								... {
									doubleNested
									... on Dog {
										nestedDogName
									}
								}
							}
							extra { string }
							extra { string: noString }
						}
					}`,
			`
					query conflictingBecauseAlias {
						dog {
							name
							nickname
							doubleNested
							nestedDogName
							extra { string }
							extra { string: noString }
						}
					}`)
	})
	t.Run("with interface type", func(t *testing.T) {
		run(t, inlineSelectionsFromInlineFragments, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							... on Pet {
								name
							}
						}
					}`,
			`
					query conflictingBecauseAlias {
						dog {
							name
						}
					}`)
	})

	t.Run("with nested non compatible fragments", func(t *testing.T) {
		run(t, inlineSelectionsFromInlineFragments, testDefinition, `
					{
						dog {
							... on Pet {
								... on Cat {
									meowVolume
								}
							}
						}
					}`,
			`
					{
						dog {
							... on Pet {
								... on Cat {
									meowVolume
								}
							}
						}
					}`)
	})

	t.Run("with nested compatible fragments", func(t *testing.T) {
		run(t, inlineSelectionsFromInlineFragments, testDefinition, `
					{
						dog {
							... on Pet {
								... on Dog {
									name
								}
							}
						}
					}`,
			`
					{
						dog {
							name
						}
					}`)
	})

	t.Run("with internal defer", func(t *testing.T) {
		run(t, inlineSelectionsFromInlineFragments, testDefinition, `
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
					}`,
			`
					query pet {
						pet {
							... on Dog {
								name @defer_internal(id: "1")
								nickname @defer_internal(id: "1")
								barkVolume @defer_internal(id: "2", parentDeferId: "1")
							}
							... on Cat {
								name @defer_internal(id: "3")
								meowVolume @defer_internal(id: "3")
							}
						}
					}`)
	})
}
