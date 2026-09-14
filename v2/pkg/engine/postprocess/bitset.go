package postprocess

import "math/bits"

// bitset stores non-negative integers as bit positions. A nil bitset is empty.
// It is not compressed, so it suits dense sets whose copying as maps is expensive.
type bitset []uint64

func (b *bitset) set(i int) {
	w := i / 64
	b.grow(w + 1)
	(*b)[w] |= 1 << (i % 64)
}

func (b bitset) has(i int) bool {
	w := i / 64
	if w >= len(b) {
		return false
	}
	return b[w]&(1<<(i%64)) != 0
}

func (b *bitset) union(o bitset) {
	b.grow(len(o))
	for i, w := range o {
		(*b)[i] |= w
	}
}

// grow extends b to at least words in a single allocation.
func (b *bitset) grow(words int) {
	if n := words - len(*b); n > 0 {
		*b = append(*b, make([]uint64, n)...)
	}
}

func (b bitset) count() int {
	n := 0
	for _, w := range b {
		n += bits.OnesCount64(w)
	}
	return n
}
