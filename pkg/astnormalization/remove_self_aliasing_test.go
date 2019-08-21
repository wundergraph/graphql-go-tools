package astnormalization

import "testing"

func TestRemoveSelfAliasing(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(removeSelfAliasing, testDefinition, `
				{dog: dog}`,
			`
				{dog}`)
	})
}
