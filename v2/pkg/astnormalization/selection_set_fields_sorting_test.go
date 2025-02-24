package astnormalization

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

func getSortSelectionSetFieldsTestFunc(compareFn selectionSetFieldsCompare) registerNormalizeFunc {
	return func(walker *astvisitor.Walker) {
		sortSelectionSetFields(walker, compareFn)
	}
}

func TestSortSelectionSetFields(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(
			t,
			getSortSelectionSetFieldsTestFunc(CompareSelectionSetFieldsLexicographically),
			testDefinition,
			`
					query {
						simple
						dog {
							name
						}
						cat {
							name
						}
					}`,
			`
					query {
						cat {
							name
						}
						dog {
							name
						}
						simple
					}`,
		)
	})
	t.Run("nested", func(t *testing.T) {
		run(
			t,
			getSortSelectionSetFieldsTestFunc(CompareSelectionSetFieldsLexicographically),
			testDefinition,
			`
					query {
						simple
						dog {
							name
							barkVolume
						}
						cat {
							name
							meowVolume
						}
					}`,
			`
					query {
						cat {
							meowVolume
							name
						}
						dog {
							barkVolume
							name
						}
						simple
					}`,
		)
	})
	t.Run("aliases", func(t *testing.T) {
		runWithVariables(
			t,
			getSortSelectionSetFieldsTestFunc(CompareSelectionSetFieldsLexicographically),
			testDefinition,
			`
					query {
						simple
						dogB: findDog(complex: $dogInput) {
							name
							barkVolume
						}
						dogA: findDog(complex: $dogInput) {
							name
							barkVolume
						}
						cat {
							name
							meowVolume
						}
					}`,
			`
					query {
						cat {
							meowVolume
							name
						}
						dogA: findDog(complex: $dogInput) {
							barkVolume	
							name
						}
						dogB: findDog(complex: $dogInput) {
							barkVolume
							name
						}
						simple
					}`,
			`{"complex": {"name": "Sparky"}}`,
		)
	})
	t.Run("fragments", func(t *testing.T) {
		runWithVariables(
			t,
			getSortSelectionSetFieldsTestFunc(CompareSelectionSetFieldsLexicographically),
			testDefinition,
			`
					query {
						...CatFragment
						... on Query {
							dog {
								name
								barkVolume
							}
						}
						...AnotherCatFragment
						simple
						... on Query {
							anotherDog: dog {
								name
								barkVolume
							}
						}
					}
					fragment CatFragment on Query {	
						name
						meowVolume
					}
					fragment AnotherCatFragment on Query {	
						name
						meowVolume
					}
					`,
			`
					query {
						...AnotherCatFragment
						...CatFragment
						... on Query {
							dog {
								barkVolume
								name
							}
						}
						... on Query {
							anotherDog: dog {
								barkVolume
								name
							}
						}
						simple
					}
					fragment CatFragment on Query {
						meowVolume
						name
					}
					fragment AnotherCatFragment on Query {
						meowVolume
						name
					}
					`,
			`{"complex": {"name": "Sparky"}}`,
		)
	})
}
