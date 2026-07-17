package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildRefMappings(t *testing.T) {
	t.Run("empty logs produce empty maps", func(t *testing.T) {
		changed, origins := buildRefMappings(nil, nil)
		assert.Empty(t, changed)
		assert.Empty(t, origins)
	})

	t.Run("copies without merges map one to one", func(t *testing.T) {
		copyLog := []refPair{{from: 0, to: 5}, {from: 0, to: 6}, {from: 1, to: 7}}

		changed, origins := buildRefMappings(copyLog, nil)

		assert.Equal(t, map[int][]int{0: {5, 6}, 1: {7}}, changed)
		assert.Equal(t, map[int][]int{5: {0}, 6: {0}, 7: {1}}, origins)
	})

	t.Run("merge transfers origins to the survivor", func(t *testing.T) {
		// user id copied to A (5) and B (6); planner id in B copied to 8; 8 merged into 6
		copyLog := []refPair{{from: 0, to: 5}, {from: 0, to: 6}, {from: 1, to: 7}, {from: 2, to: 8}}
		mergeLog := []refPair{{from: 8, to: 6}}

		changed, origins := buildRefMappings(copyLog, mergeLog)

		assert.Equal(t, map[int][]int{0: {5, 6}, 1: {7}, 2: {6}}, changed)
		assert.Equal(t, map[int][]int{5: {0}, 6: {0, 2}, 7: {1}}, origins)
	})

	t.Run("merge chain resolves to the final survivor", func(t *testing.T) {
		copyLog := []refPair{{from: 0, to: 5}, {from: 1, to: 6}, {from: 2, to: 7}}
		mergeLog := []refPair{{from: 6, to: 5}, {from: 5, to: 7}}

		changed, origins := buildRefMappings(copyLog, mergeLog)

		assert.Equal(t, map[int][]int{0: {7}, 1: {7}, 2: {7}}, changed)
		assert.Equal(t, map[int][]int{7: {2, 0, 1}}, origins)
	})

	t.Run("copies of one original merged together are deduplicated", func(t *testing.T) {
		copyLog := []refPair{{from: 0, to: 5}, {from: 0, to: 6}}
		mergeLog := []refPair{{from: 6, to: 5}}

		changed, origins := buildRefMappings(copyLog, mergeLog)

		assert.Equal(t, map[int][]int{0: {5}}, changed)
		assert.Equal(t, map[int][]int{5: {0}}, origins)
	})

	t.Run("merge of refs unknown to the copy log is ignored", func(t *testing.T) {
		copyLog := []refPair{{from: 0, to: 5}}
		mergeLog := []refPair{{from: 9, to: 8}}

		changed, origins := buildRefMappings(copyLog, mergeLog)

		assert.Equal(t, map[int][]int{0: {5}}, changed)
		assert.Equal(t, map[int][]int{5: {0}}, origins)
	})
}
