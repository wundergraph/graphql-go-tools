package postprocess

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddMissingNestedDependencies_ProcessFetchTree(t *testing.T) {
	t.Run("add missing dependencies to nested fetches on same merge path", func(t *testing.T) {
		processor := &addMissingNestedDependencies{}
		input := seq(
			sf(0, provides("a")),
			sf(1, provides("b")),
			sf(2, at("a")),
			sf(3, at("b.c")),
			sf(4, at("a"), deps(0)),
			sf(5, provides("y")),
		)
		processor.ProcessFetchTree(input)
		expected := seq(
			sf(0, provides("a")),
			sf(1, provides("b")),
			sf(2, at("a"), deps(0)),
			sf(3, at("b.c"), deps(1)),
			sf(4, at("a"), deps(0)),
			sf(5, provides("y")),
		)
		require.Equal(t, expected, input)
	})
}
