package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceConnectionGraph(t *testing.T) {

	t.Run("direct connection", func(t *testing.T) {
		t.Run("direct_connection_between_two_sources_with_matching_source/target_keys", func(t *testing.T) {
			keysPerPath := map[DSHash][]KeyInfo{
				1: {
					{
						DSHash:       1,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       true,
						Target:       false,
					},
				},
				2: {
					{
						DSHash:       2,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       false,
						Target:       true,
					},
				},
			}

			graph := NewDataSourceJumpsGraph(keysPerPath)
			path, exists := graph.GetPaths(1, 2)
			assert.True(t, exists, "Should have a connection")

			assert.Equal(t, []SourceConnection{
				{
					Jumps: []KeyJump{
						{
							From:         1,
							To:           2,
							SelectionSet: "id name",
							FieldPaths:   []string{"id", "name"},
						},
					},
					Type: SourceConnectionTypeDirect,
				},
			}, path)
		})

		t.Run("bidirectional_connection_when_both_source_and_target_keys_are_available", func(t *testing.T) {
			keysPerPath := map[DSHash][]KeyInfo{
				1: {
					{
						DSHash:       1,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       true,
						Target:       true,
					},
				},
				2: {
					{
						DSHash:       2,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       true,
						Target:       true,
					},
				},
			}

			graph := NewDataSourceJumpsGraph(keysPerPath)
			path1, exists := graph.GetPaths(1, 2)
			assert.True(t, exists, "Should have a connection")

			assert.Equal(t, []SourceConnection{
				{
					Jumps: []KeyJump{
						{
							From:         1,
							To:           2,
							SelectionSet: "id name",
							FieldPaths:   []string{"id", "name"},
						},
					},
					Type: SourceConnectionTypeDirect,
				},
			}, path1)

			path2, exists := graph.GetPaths(2, 1)
			assert.True(t, exists, "Should have a connection")

			assert.Equal(t, []SourceConnection{
				{
					Jumps: []KeyJump{
						{
							From:         2,
							To:           1,
							SelectionSet: "id name",
							FieldPaths:   []string{"id", "name"},
						},
					},
					Type: SourceConnectionTypeDirect,
				},
			}, path2)
		})

		t.Run("multiple direct paths between sources", func(t *testing.T) {
			keysPerPath := map[DSHash][]KeyInfo{
				1: {
					{
						DSHash:       1,
						TypeName:     "User",
						SelectionSet: "id",
						FieldPaths:   []string{"id"},
						Source:       true,
						Target:       true,
					},
					{
						DSHash:       1,
						TypeName:     "User",
						SelectionSet: "email",
						FieldPaths:   []string{"email"},
						Source:       true,
						Target:       true,
					},
				},
				2: {
					{
						DSHash:       2,
						TypeName:     "User",
						SelectionSet: "id",
						FieldPaths:   []string{"id"},
						Source:       true,
						Target:       true,
					},
					{
						DSHash:       2,
						TypeName:     "User",
						SelectionSet: "email",
						FieldPaths:   []string{"email"},
						Source:       true,
						Target:       true,
					},
				},
			}

			graph := NewDataSourceJumpsGraph(keysPerPath)
			path, exists := graph.GetPaths(1, 2)
			assert.True(t, exists, "Should have a connection")

			assert.Equal(t, []SourceConnection{
				{
					Jumps: []KeyJump{
						{
							From:         1,
							To:           2,
							SelectionSet: "id",
							FieldPaths:   []string{"id"},
						},
					},
					Type: SourceConnectionTypeDirect,
				},
				{
					Jumps: []KeyJump{
						{
							From:         1,
							To:           2,
							SelectionSet: "email",
							FieldPaths:   []string{"email"},
						},
					},
					Type: SourceConnectionTypeDirect,
				},
			}, path)
		})

	})

	t.Run("indirect_connection_through_key_chain_with_correct_source/target_keys", func(t *testing.T) {
		keysPerPath := map[DSHash][]KeyInfo{
			1: {
				{
					DSHash:       1,
					TypeName:     "User",
					SelectionSet: "id",
					FieldPaths:   []string{"id"},
					Source:       true,
					Target:       false,
				},
			},
			2: {
				{
					DSHash:       2,
					TypeName:     "User",
					SelectionSet: "id",
					FieldPaths:   []string{"id"},
					Source:       true,
					Target:       true,
				},
				{
					DSHash:       2,
					TypeName:     "User",
					SelectionSet: "email",
					FieldPaths:   []string{"email"},
					Source:       true,
					Target:       true,
				},
			},
			3: {
				{
					DSHash:       3,
					TypeName:     "User",
					SelectionSet: "email",
					FieldPaths:   []string{"email"},
					Source:       true,
					Target:       true,
				},
				{
					DSHash:       3,
					TypeName:     "User",
					SelectionSet: "role",
					FieldPaths:   []string{"role"},
					Source:       true,
					Target:       true,
				},
			},
			4: {
				{
					DSHash:       4,
					TypeName:     "User",
					SelectionSet: "role",
					FieldPaths:   []string{"role"},
					Source:       false,
					Target:       true,
				},
			},
		}

		graph := NewDataSourceJumpsGraph(keysPerPath)
		path, exists := graph.GetPaths(1, 4)
		assert.True(t, exists, "Should have a connection")

		assert.Equal(t, []SourceConnection{
			{
				Jumps: []KeyJump{
					{
						From:         1,
						To:           2,
						SelectionSet: "id",
						FieldPaths:   []string{"id"},
					},
					{
						From:         2,
						To:           3,
						SelectionSet: "email",
						FieldPaths:   []string{"email"},
					},
					{
						From:         3,
						To:           4,
						SelectionSet: "role",
						FieldPaths:   []string{"role"},
					},
				},
				Type: SourceConnectionTypeIndirect,
			},
		}, path)
	})

	t.Run("no connectiion", func(t *testing.T) {
		t.Run("no_connection_between_sources_with_different_keys", func(t *testing.T) {
			keysPerPath := map[DSHash][]KeyInfo{
				1: {
					{
						DSHash:       1,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       true,
						Target:       false,
					},
				},
				2: {
					{
						DSHash:       2,
						TypeName:     "User",
						SelectionSet: "email role",
						FieldPaths:   []string{"email", "role"},
						Source:       false,
						Target:       true,
					},
				},
			}

			graph := NewDataSourceJumpsGraph(keysPerPath)
			path, exists := graph.GetPaths(1, 2)
			assert.False(t, exists, "Should not have a connection")
			assert.Nil(t, path, "Path should be nil")
		})

		t.Run("no_connection_when_source_key_is_missing", func(t *testing.T) {
			keysPerPath := map[DSHash][]KeyInfo{
				1: {
					{
						DSHash:       1,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       false, // Not a source
						Target:       false,
					},
				},
				2: {
					{
						DSHash:       2,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       false,
						Target:       true,
					},
				},
			}

			graph := NewDataSourceJumpsGraph(keysPerPath)
			path, exists := graph.GetPaths(1, 2)
			assert.False(t, exists, "Should not have a connection")
			assert.Nil(t, path, "Path should be nil")
		})

		t.Run("no_connection_when_target_key_is_missing", func(t *testing.T) {
			keysPerPath := map[DSHash][]KeyInfo{
				1: {
					{
						DSHash:       1,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       true,
						Target:       false,
					},
				},
				2: {
					{
						DSHash:       2,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       false,
						Target:       false, // Not a target
					},
				},
			}

			graph := NewDataSourceJumpsGraph(keysPerPath)
			path, exists := graph.GetPaths(1, 2)
			assert.False(t, exists, "Should not have a connection")
			assert.Nil(t, path, "Path should be nil")
		})

		t.Run("no_connection_with_different_selection_sets", func(t *testing.T) {
			keysPerPath := map[DSHash][]KeyInfo{
				1: {
					{
						DSHash:       1,
						TypeName:     "User",
						SelectionSet: "id name",
						FieldPaths:   []string{"id", "name"},
						Source:       true,
						Target:       false,
					},
				},
				2: {
					{
						DSHash:       2,
						TypeName:     "User",
						SelectionSet: "id email",
						FieldPaths:   []string{"id", "email"},
						Source:       false,
						Target:       true,
					},
				},
			}

			graph := NewDataSourceJumpsGraph(keysPerPath)
			path, exists := graph.GetPaths(1, 2)
			assert.False(t, exists, "Should not have a connection")
			assert.Nil(t, path, "Path should be nil")
		})
	})
}

func TestSourceConnectionGraphCache(t *testing.T) {
	keysPerPath := map[DSHash][]KeyInfo{
		1: {
			{
				DSHash:       1,
				TypeName:     "User",
				SelectionSet: "id",
				FieldPaths:   []string{"id"},
				Source:       true,
				Target:       false,
			},
		},
		2: {
			{
				DSHash:       2,
				TypeName:     "User",
				SelectionSet: "id",
				FieldPaths:   []string{"id"},
				Source:       false,
				Target:       true,
			},
		},
	}

	graph := NewDataSourceJumpsGraph(keysPerPath)

	// First call to GetPaths should compute the path
	path, exists := graph.GetPaths(1, 2)
	assert.True(t, exists, "Should have a connection")
	assert.NotNil(t, path, "Path should not be nil")

	// Check that the cache is not empty and contains the expected path
	cacheKey := JumpCacheKey{Source: 1, Target: 2}
	cachedPath, cacheExists := graph.Cache[cacheKey]
	assert.True(t, cacheExists, "Cache should contain the path")
	assert.Equal(t, path, cachedPath, "Cached path should match the computed path")
}
