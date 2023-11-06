package astnormalization

import "testing"

func TestMergeInlineFragmentSelections(t *testing.T) {
	t.Run("fragments with the same field", func(t *testing.T) {
		run(t, mergeInlineFragmentSelections, testDefinition, `
					query a {
						pet {
							... on Dog {
								name
							}
							... on Dog {
								name
							}
							... on Dog {
								name
							}
						}
					}`, `
					query a {
						pet {
							... on Dog {
								name
								name
								name
							}
						}
					}`)
	})
	t.Run("fragments with different fields", func(t *testing.T) {
		run(t, mergeInlineFragmentSelections, testDefinition, `
					query a {
						pet {
							... on Dog {
								name
							}
							... on Dog {
								nickname
							}
							... on Dog {
								barkVolume
							}
						}
					}`, `
					query a {
						pet {
							... on Dog {
								name
								nickname
								barkVolume
							}
						}
					}`)
	})
	t.Run("fragments with different types", func(t *testing.T) {
		run(t, mergeInlineFragmentSelections, testDefinition, `
					query a {
						pet {
							... on Dog {
								name
							}
							... on Cat {
								name
							}
						}
					}`, `
					query a {
						pet {
							... on Dog {
								name
							}
							... on Cat {
								name
							}
						}
					}`)
	})
	t.Run("fragments with different directives", func(t *testing.T) {
		run(t, mergeInlineFragmentSelections, testDefinition, `
					query a {
						pet {
							... on Dog @skip(if: $foo) {
								name
							}
							... on Dog @skip(if: $bar) {
								nickname
							}
						}
					}`, `
					query a {
						pet {
							... on Dog @skip(if: $foo) {
								name
							}
							... on Dog @skip(if: $bar) {
								nickname
							}
						}
					}`)
	})

	t.Run("fragments with the same directives", func(t *testing.T) {
		run(t, mergeInlineFragmentSelections, testDefinition, `
					query a {
						pet {
							... on Dog @skip(if: $foo) {
								name
							}
							... on Dog @skip(if: $foo) {
								nickname
							}
						}
					}`, `
					query a {
						pet {
							... on Dog @skip(if: $foo) {
								name
								nickname
							}
						}
					}`)
	})
}
