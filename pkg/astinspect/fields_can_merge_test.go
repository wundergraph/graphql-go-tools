package astinspect

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"testing"
)

func TestFieldsCanMerge(t *testing.T) {

	run := func(testOperation string, wantCanMerge bool) {

		operation := unsafeparser.ParseGraphqlDocumentString(testOperation)

		got := FieldsCanMerge(&operation, 0, 1)
		if wantCanMerge != got {
			panic(fmt.Errorf("want: %t, got: %t for: %s", wantCanMerge, got, testOperation))
		}
	}

	t.Run("different field", func(t *testing.T) {
		run(`{a b}`, true)
	})
	t.Run("same field", func(t *testing.T) {
		run(`{a a}`, true)
	})
	t.Run("aliased different field", func(t *testing.T) {
		run(`{a: b a}`, false)
	})
}
