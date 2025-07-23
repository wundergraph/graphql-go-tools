package astnormalization

import "testing"

func TestMergeInlineFragmentFieldSelections(t *testing.T) {
	t.Run("just fragments", func(t *testing.T) {
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
	})
	t.Run("just fields", func(t *testing.T) {
		t.Run("depth 1", func(t *testing.T) {
			run(t, mergeInlineFragmentSelections, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							extra { string }
							extra { noString: string }
						}
					}`, `
					query conflictingBecauseAlias {
						dog {
							extra { 
								string
								noString: string
							}
						}
					}`)
		})
		t.Run("depth 2", func(t *testing.T) {
			run(t, mergeInlineFragmentSelections, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							extra { string }
							extra { string: noString }
						}
						dog {
							extra { string }
							extra { string: noString }
						}
					}`, `
					query conflictingBecauseAlias {
						dog {
							extra { 
								string
								string: noString
								string
								string: noString
							}
						}
					}`)
		})
		t.Run("aliased", func(t *testing.T) {
			run(t, mergeInlineFragmentSelections, testDefinition, `
					query conflictingBecauseAlias {
						dog {
							x: extras { string }
							x: mustExtras { string }
						}
					}`, `
					query conflictingBecauseAlias {
						dog {
							x: extras { string }
							x: mustExtras { string }
						}
					}`)
		})
		t.Run("fields with directives", func(t *testing.T) {
			run(t, mergeInlineFragmentSelections, testDefinition, `
					{
						field @skip(if: $foo) {
							subfieldA
						}
						field @skip(if: $bar) {
							subfieldB
						}
					}`, `
					{
						field @skip(if: $foo) {
							subfieldA
						}
						field @skip(if: $bar) {
							subfieldB
						}
					}`)
		})

		t.Run("fields with the same directives", func(t *testing.T) {
			run(t, mergeInlineFragmentSelections, testDefinition, `
					{
						field @skip(if: $foo) {
							subfieldA
						}
						field @skip(if: $foo) {
							subfieldB
						}
					}`, `
					{
						field @skip(if: $foo) {
							subfieldA
							subfieldB
						}
					}`)
		})
	})
	t.Run("fragments and fields", func(t *testing.T) {
		t.Run("field field fragment", func(t *testing.T) {
			run(t, mergeInlineFragmentSelections, testDefinition, `
					query fragmentsWithFields {
						dog {
							extras { 
								... on DogExtra {
									string 
								}
							}
							extras { 
								... on DogExtra {
									string 
									string1
								}
							}
						}
					}`, `
					query fragmentsWithFields {
						dog {
							extras { 
								... on DogExtra {
									string 
									string
									string1
								}
							}
						}
					}`)
		})
		t.Run("mixed heavily fields with deep inline fragments", func(t *testing.T) {
			run(t, mergeInlineFragmentSelections, testDefinition, `
					query mixedFragmentTypes {
						pet {
							... on Dog {
								name
							}
						}
						pet {
							... on Dog {
								extras { 
									... on DogExtra {
										string 
									}
								}
							}
						}
						pet {
							... on Dog {
								name
								barkVolume
							}
						}
						pet {
							... on Dog {
								extras { 
									... on DogExtra {
										string1
									}
								}
							}
						}
						pet {
							... on Cat {
								name
							}
						}
						pet {
							... on Dog {
								name
								barkVolume
								owner {
									name
								}
							}
						}
						pet {
							... on Cat {
								meowVolume
							}
						}
						pet {
							... on Cat {
								meowVolume
							}
						}
					}`, `
					query mixedFragmentTypes {
						pet {
							... on Dog {
								name
								extras {
									... on DogExtra {
										string
										string1
									}
								}
								name
								barkVolume
								name
								barkVolume
								owner {
									name
								}
							}
							... on Cat {
								name
								meowVolume
								meowVolume
							}
						}
					}`)
		})

	})
}
