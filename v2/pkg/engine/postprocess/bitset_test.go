package postprocess

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBitset(t *testing.T) {
	t.Run("nil bitset is empty", func(t *testing.T) {
		var b bitset
		require.Equal(t, 0, b.count())
		require.False(t, b.has(0))
		require.False(t, b.has(1000))
	})
	t.Run("set and test single bit", func(t *testing.T) {
		var b bitset
		b.set(5)
		require.True(t, b.has(5))
		require.False(t, b.has(4))
		require.False(t, b.has(6))
		require.Equal(t, 1, b.count())
	})
	t.Run("zero is a valid position", func(t *testing.T) {
		var b bitset
		require.False(t, b.has(0))
		b.set(0)
		require.True(t, b.has(0))
		require.Equal(t, 1, b.count())
	})
	t.Run("set is idempotent", func(t *testing.T) {
		var b bitset
		b.set(7)
		b.set(7)
		require.Equal(t, 1, b.count())
	})
	t.Run("word boundaries", func(t *testing.T) {
		var b bitset
		for _, i := range []int{63, 64, 127, 128} {
			b.set(i)
		}
		for _, i := range []int{63, 64, 127, 128} {
			require.True(t, b.has(i), "bit %d", i)
		}
		for _, i := range []int{0, 62, 65, 126, 129} {
			require.False(t, b.has(i), "bit %d", i)
		}
		require.Equal(t, 4, b.count())
	})
	t.Run("set grows to the needed word only", func(t *testing.T) {
		var b bitset
		b.set(200)
		require.True(t, b.has(200))
		require.Equal(t, 1, b.count())
		b.set(3)
		require.Equal(t, 2, b.count())
	})
	t.Run("test beyond length is false", func(t *testing.T) {
		var b bitset
		b.set(1)
		require.False(t, b.has(64))
		require.False(t, b.has(64*100))
	})
	t.Run("union into nil", func(t *testing.T) {
		var o bitset
		o.set(1)
		o.set(70)
		var b bitset
		b.union(o)
		require.True(t, b.has(1))
		require.True(t, b.has(70))
		require.Equal(t, 2, b.count())
	})
	t.Run("union with nil is a no-op", func(t *testing.T) {
		var b bitset
		b.set(9)
		b.union(nil)
		require.True(t, b.has(9))
		require.Equal(t, bitset{1 << 9}, b)
	})
	t.Run("union grows the receiver", func(t *testing.T) {
		var b bitset
		b.set(1)
		var o bitset
		o.set(130)
		b.union(o)
		require.True(t, b.has(1))
		require.True(t, b.has(130))
	})
	t.Run("union keeps receiver bits beyond the argument", func(t *testing.T) {
		var b bitset
		b.set(130)
		var o bitset
		o.set(1)
		b.union(o)
		require.True(t, b.has(1))
		require.True(t, b.has(130))
		require.Equal(t, 2, b.count())
	})
	t.Run("union does not alias the argument", func(t *testing.T) {
		var o bitset
		o.set(1)
		var b bitset
		b.union(o)
		b.set(2)
		require.False(t, o.has(2))
	})
	t.Run("union is idempotent and overlapping bits count once", func(t *testing.T) {
		var b bitset
		b.set(3)
		b.set(64)
		var o bitset
		o.set(3)
		o.set(65)
		b.union(o)
		b.union(o)
		require.Equal(t, 3, b.count())
	})
	t.Run("read-only methods do not allocate", func(t *testing.T) {
		var b bitset
		b.set(1)
		b.set(130)
		require.Zero(t, testing.AllocsPerRun(100, func() {
			b.has(1)
			b.has(500)
			b.count()
		}))
	})
	t.Run("set within the current length does not allocate", func(t *testing.T) {
		var b bitset
		b.set(130)
		i := 0
		require.Zero(t, testing.AllocsPerRun(100, func() {
			b.set(i % 192)
			i++
		}))
	})
	t.Run("union into a receiver that is long enough does not allocate", func(t *testing.T) {
		var b bitset
		b.set(130)
		var o bitset
		o.set(1)
		o.set(70)
		require.Zero(t, testing.AllocsPerRun(100, func() {
			b.union(o)
		}))
	})
	t.Run("matches a map set under random operations", func(t *testing.T) {
		rng := rand.New(rand.NewSource(1))
		const maxID = 300
		for round := range 200 {
			var b bitset
			ref := map[int]struct{}{}
			for range 50 {
				switch rng.Intn(3) {
				case 0, 1:
					id := rng.Intn(maxID)
					b.set(id)
					ref[id] = struct{}{}
				case 2:
					var o bitset
					for k := rng.Intn(5); k > 0; k-- {
						id := rng.Intn(maxID)
						o.set(id)
						ref[id] = struct{}{}
					}
					b.union(o)
				}
			}
			require.Equal(t, len(ref), b.count())
			for id := range maxID + 64 {
				_, inRef := ref[id]
				require.Equal(t, inRef, b.has(id), "round %d bit %d", round, id)
			}
		}
	})
}
