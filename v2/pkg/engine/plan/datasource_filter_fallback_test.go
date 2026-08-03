package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBestFallbackConnection(t *testing.T) {
	fallbackJump := func(missing int) KeyJump {
		fieldPaths := make([]KeyInfoFieldPath, missing+1)
		return KeyJump{
			Fallback:    true,
			FieldPaths:  fieldPaths,
			SourcePaths: fieldPaths[:1],
		}
	}

	t.Run("nil on empty input", func(t *testing.T) {
		assert.Nil(t, bestFallbackConnection(nil))
	})

	t.Run("prefers fewer missing key members", func(t *testing.T) {
		paths := []SourceConnection{
			{Source: 1, Jumps: []KeyJump{fallbackJump(2)}},
			{Source: 2, Jumps: []KeyJump{fallbackJump(1)}},
			{Source: 3, Jumps: []KeyJump{fallbackJump(3)}},
		}

		best := bestFallbackConnection(paths)
		assert.Equal(t, DSHash(2), best.Source)
	})

	t.Run("prefers fewer jumps over fewer missing members", func(t *testing.T) {
		paths := []SourceConnection{
			{Source: 1, Jumps: []KeyJump{fallbackJump(1), fallbackJump(1)}},
			{Source: 2, Jumps: []KeyJump{fallbackJump(3)}},
		}

		best := bestFallbackConnection(paths)
		assert.Equal(t, DSHash(2), best.Source)
	})

	t.Run("first wins ties", func(t *testing.T) {
		paths := []SourceConnection{
			{Source: 1, Jumps: []KeyJump{fallbackJump(1)}},
			{Source: 2, Jumps: []KeyJump{fallbackJump(1)}},
		}

		best := bestFallbackConnection(paths)
		assert.Equal(t, DSHash(1), best.Source)
	})
}
