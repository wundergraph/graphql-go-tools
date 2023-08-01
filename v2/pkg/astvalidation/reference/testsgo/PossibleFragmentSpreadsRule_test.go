package testsgo

import (
	"testing"
)

func TestPossibleFragmentSpreadsRule(t *testing.T) {
	t.Skip()

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, PossibleFragmentSpreadsRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: Possible fragment spreads", func(t *testing.T) {
		t.Run("of the same object", func(t *testing.T) {
			ExpectValid(t, `
      fragment objectWithinObject on Dog { ...dogFragment }
      fragment dogFragment on Dog { barkVolume }
    `)
		})

		t.Run("of the same object with inline fragment", func(t *testing.T) {
			ExpectValid(t, `
      fragment objectWithinObjectAnon on Dog { ... on Dog { barkVolume } }
    `)
		})

		t.Run("object into an implemented interface", func(t *testing.T) {
			ExpectValid(t, `
      fragment objectWithinInterface on Pet { ...dogFragment }
      fragment dogFragment on Dog { barkVolume }
    `)
		})

		t.Run("object into containing union", func(t *testing.T) {
			ExpectValid(t, `
      fragment objectWithinUnion on CatOrDog { ...dogFragment }
      fragment dogFragment on Dog { barkVolume }
    `)
		})

		t.Run("union into contained object", func(t *testing.T) {
			ExpectValid(t, `
      fragment unionWithinObject on Dog { ...catOrDogFragment }
      fragment catOrDogFragment on CatOrDog { __typename }
    `)
		})

		t.Run("union into overlapping interface", func(t *testing.T) {
			ExpectValid(t, `
      fragment unionWithinInterface on Pet { ...catOrDogFragment }
      fragment catOrDogFragment on CatOrDog { __typename }
    `)
		})

		t.Run("union into overlapping union", func(t *testing.T) {
			ExpectValid(t, `
      fragment unionWithinUnion on DogOrHuman { ...catOrDogFragment }
      fragment catOrDogFragment on CatOrDog { __typename }
    `)
		})

		t.Run("interface into implemented object", func(t *testing.T) {
			ExpectValid(t, `
      fragment interfaceWithinObject on Dog { ...petFragment }
      fragment petFragment on Pet { name }
    `)
		})

		t.Run("interface into overlapping interface", func(t *testing.T) {
			ExpectValid(t, `
      fragment interfaceWithinInterface on Pet { ...beingFragment }
      fragment beingFragment on Being { name }
    `)
		})

		t.Run("interface into overlapping interface in inline fragment", func(t *testing.T) {
			ExpectValid(t, `
      fragment interfaceWithinInterface on Pet { ... on Being { name } }
    `)
		})

		t.Run("interface into overlapping union", func(t *testing.T) {
			ExpectValid(t, `
      fragment interfaceWithinUnion on CatOrDog { ...petFragment }
      fragment petFragment on Pet { name }
    `)
		})

		t.Run("ignores incorrect type (caught by FragmentsOnCompositeTypesRule)", func(t *testing.T) {
			ExpectValid(t, `
      fragment petFragment on Pet { ...badInADifferentWay }
      fragment badInADifferentWay on String { name }
    `)
		})

		t.Run("ignores unknown fragments (caught by KnownFragmentNamesRule)", func(t *testing.T) {
			ExpectValid(t, `
      fragment petFragment on Pet { ...UnknownFragment }
    `)
		})

		t.Run("different object into object", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidObjectWithinObject on Cat { ...dogFragment }
      fragment dogFragment on Dog { barkVolume }
    `)([]Err{
				{
					message:   `Fragment "dogFragment" cannot be spread here as objects of type "Cat" can never be of type "Dog".`,
					locations: []Loc{{line: 2, column: 51}},
				},
			})
		})

		t.Run("different object into object in inline fragment", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidObjectWithinObjectAnon on Cat {
        ... on Dog { barkVolume }
      }
    `)([]Err{
				{
					message:   `Fragment cannot be spread here as objects of type "Cat" can never be of type "Dog".`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("object into not implementing interface", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidObjectWithinInterface on Pet { ...humanFragment }
      fragment humanFragment on Human { pets { name } }
    `)([]Err{
				{
					message:   `Fragment "humanFragment" cannot be spread here as objects of type "Pet" can never be of type "Human".`,
					locations: []Loc{{line: 2, column: 54}},
				},
			})
		})

		t.Run("object into not containing union", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidObjectWithinUnion on CatOrDog { ...humanFragment }
      fragment humanFragment on Human { pets { name } }
    `)([]Err{
				{
					message:   `Fragment "humanFragment" cannot be spread here as objects of type "CatOrDog" can never be of type "Human".`,
					locations: []Loc{{line: 2, column: 55}},
				},
			})
		})

		t.Run("union into not contained object", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidUnionWithinObject on Human { ...catOrDogFragment }
      fragment catOrDogFragment on CatOrDog { __typename }
    `)([]Err{
				{
					message:   `Fragment "catOrDogFragment" cannot be spread here as objects of type "Human" can never be of type "CatOrDog".`,
					locations: []Loc{{line: 2, column: 52}},
				},
			})
		})

		t.Run("union into non overlapping interface", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidUnionWithinInterface on Pet { ...humanOrAlienFragment }
      fragment humanOrAlienFragment on HumanOrAlien { __typename }
    `)([]Err{
				{
					message:   `Fragment "humanOrAlienFragment" cannot be spread here as objects of type "Pet" can never be of type "HumanOrAlien".`,
					locations: []Loc{{line: 2, column: 53}},
				},
			})
		})

		t.Run("union into non overlapping union", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidUnionWithinUnion on CatOrDog { ...humanOrAlienFragment }
      fragment humanOrAlienFragment on HumanOrAlien { __typename }
    `)([]Err{
				{
					message:   `Fragment "humanOrAlienFragment" cannot be spread here as objects of type "CatOrDog" can never be of type "HumanOrAlien".`,
					locations: []Loc{{line: 2, column: 54}},
				},
			})
		})

		t.Run("interface into non implementing object", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidInterfaceWithinObject on Cat { ...intelligentFragment }
      fragment intelligentFragment on Intelligent { iq }
    `)([]Err{
				{
					message:   `Fragment "intelligentFragment" cannot be spread here as objects of type "Cat" can never be of type "Intelligent".`,
					locations: []Loc{{line: 2, column: 54}},
				},
			})
		})

		t.Run("interface into non overlapping interface", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidInterfaceWithinInterface on Pet {
        ...intelligentFragment
      }
      fragment intelligentFragment on Intelligent { iq }
    `)([]Err{
				{
					message:   `Fragment "intelligentFragment" cannot be spread here as objects of type "Pet" can never be of type "Intelligent".`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("interface into non overlapping interface in inline fragment", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidInterfaceWithinInterfaceAnon on Pet {
        ...on Intelligent { iq }
      }
    `)([]Err{
				{
					message:   `Fragment cannot be spread here as objects of type "Pet" can never be of type "Intelligent".`,
					locations: []Loc{{line: 3, column: 9}},
				},
			})
		})

		t.Run("interface into non overlapping union", func(t *testing.T) {
			ExpectErrors(t, `
      fragment invalidInterfaceWithinUnion on HumanOrAlien { ...petFragment }
      fragment petFragment on Pet { name }
    `)([]Err{
				{
					message:   `Fragment "petFragment" cannot be spread here as objects of type "HumanOrAlien" can never be of type "Pet".`,
					locations: []Loc{{line: 2, column: 62}},
				},
			})
		})
	})

}
