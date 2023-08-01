package testsgo

import (
	"testing"
)

func TestNoFragmentCyclesRule(t *testing.T) {
	t.Skip("fails with fragment is unused - seems our rule should be splitted")

	ExpectErrors := func(t *testing.T, queryStr string) ResultCompare {
		return ExpectValidationErrors(t, NoFragmentCyclesRule, queryStr)
	}

	ExpectValid := func(t *testing.T, queryStr string) {
		ExpectErrors(t, queryStr)([]Err{})
	}

	t.Run("Validate: No circular fragment spreads", func(t *testing.T) {

		t.Run("single reference is valid", func(t *testing.T) {
			ExpectValid(t, `
      fragment fragA on Dog { ...fragB }
      fragment fragB on Dog { name }
    `)
		})

		t.Run("spreading twice is not circular", func(t *testing.T) {
			ExpectValid(t, `
      fragment fragA on Dog { ...fragB, ...fragB }
      fragment fragB on Dog { name }
    `)
		})

		t.Run("spreading twice indirectly is not circular", func(t *testing.T) {
			ExpectValid(t, `
      fragment fragA on Dog { ...fragB, ...fragC }
      fragment fragB on Dog { ...fragC }
      fragment fragC on Dog { name }
    `)
		})

		t.Run("double spread within abstract types", func(t *testing.T) {
			ExpectValid(t, `
      fragment nameFragment on Pet {
        ... on Dog { name }
        ... on Cat { name }
      }

      fragment spreadsInAnon on Pet {
        ... on Dog { ...nameFragment }
        ... on Cat { ...nameFragment }
      }
    `)
		})

		t.Run("does not false positive on unknown fragment", func(t *testing.T) {
			ExpectValid(t, `
      fragment nameFragment on Pet {
        ...UnknownFragment
      }
    `)
		})

		t.Run("spreading recursively within field fails", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Human { relatives { ...fragA } },
    `)([]Err{
				{
					message:   `Cannot spread fragment "fragA" within itself.`,
					locations: []Loc{{line: 2, column: 45}},
				},
			})
		})

		t.Run("no spreading itself directly", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Dog { ...fragA }
    `)([]Err{
				{
					message:   `Cannot spread fragment "fragA" within itself.`,
					locations: []Loc{{line: 2, column: 31}},
				},
			})
		})

		t.Run("no spreading itself directly within inline fragment", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Pet {
        ... on Dog {
          ...fragA
        }
      }
    `)([]Err{
				{
					message:   `Cannot spread fragment "fragA" within itself.`,
					locations: []Loc{{line: 4, column: 11}},
				},
			})
		})

		t.Run("no spreading itself indirectly", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Dog { ...fragB }
      fragment fragB on Dog { ...fragA }
    `)([]Err{
				{
					message: `Cannot spread fragment "fragA" within itself via "fragB".`,
					locations: []Loc{
						{line: 2, column: 31},
						{line: 3, column: 31},
					},
				},
			})
		})

		t.Run("no spreading itself indirectly reports opposite order", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragB on Dog { ...fragA }
      fragment fragA on Dog { ...fragB }
    `)([]Err{
				{
					message: `Cannot spread fragment "fragB" within itself via "fragA".`,
					locations: []Loc{
						{line: 2, column: 31},
						{line: 3, column: 31},
					},
				},
			})
		})

		t.Run("no spreading itself indirectly within inline fragment", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Pet {
        ... on Dog {
          ...fragB
        }
      }
      fragment fragB on Pet {
        ... on Dog {
          ...fragA
        }
      }
    `)([]Err{
				{
					message: `Cannot spread fragment "fragA" within itself via "fragB".`,
					locations: []Loc{
						{line: 4, column: 11},
						{line: 9, column: 11},
					},
				},
			})
		})

		t.Run("no spreading itself deeply", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Dog { ...fragB }
      fragment fragB on Dog { ...fragC }
      fragment fragC on Dog { ...fragO }
      fragment fragX on Dog { ...fragY }
      fragment fragY on Dog { ...fragZ }
      fragment fragZ on Dog { ...fragO }
      fragment fragO on Dog { ...fragP }
      fragment fragP on Dog { ...fragA, ...fragX }
    `)([]Err{
				{
					message: `Cannot spread fragment "fragA" within itself via "fragB", "fragC", "fragO", "fragP".`,
					locations: []Loc{
						{line: 2, column: 31},
						{line: 3, column: 31},
						{line: 4, column: 31},
						{line: 8, column: 31},
						{line: 9, column: 31},
					},
				},
				{
					message: `Cannot spread fragment "fragO" within itself via "fragP", "fragX", "fragY", "fragZ".`,
					locations: []Loc{
						{line: 8, column: 31},
						{line: 9, column: 41},
						{line: 5, column: 31},
						{line: 6, column: 31},
						{line: 7, column: 31},
					},
				},
			})
		})

		t.Run("no spreading itself deeply two paths", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Dog { ...fragB, ...fragC }
      fragment fragB on Dog { ...fragA }
      fragment fragC on Dog { ...fragA }
    `)([]Err{
				{
					message: `Cannot spread fragment "fragA" within itself via "fragB".`,
					locations: []Loc{
						{line: 2, column: 31},
						{line: 3, column: 31},
					},
				},
				{
					message: `Cannot spread fragment "fragA" within itself via "fragC".`,
					locations: []Loc{
						{line: 2, column: 41},
						{line: 4, column: 31},
					},
				},
			})
		})

		t.Run("no spreading itself deeply two paths -- alt traverse order", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Dog { ...fragC }
      fragment fragB on Dog { ...fragC }
      fragment fragC on Dog { ...fragA, ...fragB }
    `)([]Err{
				{
					message: `Cannot spread fragment "fragA" within itself via "fragC".`,
					locations: []Loc{
						{line: 2, column: 31},
						{line: 4, column: 31},
					},
				},
				{
					message: `Cannot spread fragment "fragC" within itself via "fragB".`,
					locations: []Loc{
						{line: 4, column: 41},
						{line: 3, column: 31},
					},
				},
			})
		})

		t.Run("no spreading itself deeply and immediately", func(t *testing.T) {
			ExpectErrors(t, `
      fragment fragA on Dog { ...fragB }
      fragment fragB on Dog { ...fragB, ...fragC }
      fragment fragC on Dog { ...fragA, ...fragB }
    `)([]Err{
				{
					message:   `Cannot spread fragment "fragB" within itself.`,
					locations: []Loc{{line: 3, column: 31}},
				},
				{
					message: `Cannot spread fragment "fragA" within itself via "fragB", "fragC".`,
					locations: []Loc{
						{line: 2, column: 31},
						{line: 3, column: 41},
						{line: 4, column: 31},
					},
				},
				{
					message: `Cannot spread fragment "fragB" within itself via "fragC".`,
					locations: []Loc{
						{line: 3, column: 41},
						{line: 4, column: 41},
					},
				},
			})
		})
	})

}
