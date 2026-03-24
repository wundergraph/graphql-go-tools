package plan

// KeyJump represents possible jump from one data source to another
type KeyJump struct {
	From         DSHash
	To           DSHash
	SelectionSet string
	FieldPaths   []KeyInfoFieldPath
	TypeName     string
}

type SourceConnectionType int

const (
	SourceConnectionTypeDirect SourceConnectionType = iota
	SourceConnectionTypeIndirect
)

type SourceConnection struct {
	Source DSHash
	Target DSHash
	Jumps  []KeyJump
	Type   SourceConnectionType
}

// JumpCacheKey represents a key for the cache map
type JumpCacheKey struct {
	Source DSHash
	Target DSHash
}

// DataSourceJumpsGraph represents a graph of possible jumps between each data sources
// We need a way to quickly find the shortest path from one data source to another
type DataSourceJumpsGraph struct {
	Jumps map[DSHash][]KeyJump
	Cache map[JumpCacheKey][]SourceConnection
}

func (g *DataSourceJumpsGraph) GetPaths(source DSHash, target DSHash) ([]SourceConnection, bool) {
	// Create a cache key
	key := JumpCacheKey{Source: source, Target: target}

	// Check if the path is already in the cache
	if path, found := g.Cache[key]; found {
		return path, len(path) > 0
	}

	// Initialize a map to store visited nodes to prevent cycles
	visited := make(map[DSHash]bool)

	// Helper function to perform DFS and find paths
	var dfs func(current DSHash, path []KeyJump, depth int) ([]SourceConnection, bool)
	dfs = func(current DSHash, path []KeyJump, depth int) ([]SourceConnection, bool) {
		if depth > 10 {
			return nil, false // Prevent deep recursion
		}

		if current == target {
			t := SourceConnectionTypeDirect
			if len(path) > 1 {
				t = SourceConnectionTypeIndirect
			}

			return []SourceConnection{{Jumps: path, Type: t, Source: source, Target: target}}, true
		}

		visited[current] = true
		defer func() { visited[current] = false }() // Unmark the node after visiting

		var connections []SourceConnection
		found := false

		for _, jump := range g.Jumps[current] {
			if depth > 0 && jump.SelectionSet == path[len(path)-1].SelectionSet {
				continue // Skip jumps with the same selection set
			}

			if !visited[jump.To] {
				newPath := append(path, jump)
				if conns, exists := dfs(jump.To, newPath, depth+1); exists {
					connections = append(connections, conns...)
					found = true
				}
			}
		}

		return connections, found
	}

	// Start DFS from the source
	paths, found := dfs(source, nil, 0)

	// Store the result in the cache
	if found {
		g.Cache[key] = paths
	} else {
		g.Cache[key] = nil
	}

	return paths, found
}

func NewDataSourceJumpsGraph(dataSources []DSHash, keysPerPath map[DSHash][]KeyInfo, typeName string) *DataSourceJumpsGraph {
	graph := &DataSourceJumpsGraph{
		Jumps: make(map[DSHash][]KeyJump),
		Cache: make(map[JumpCacheKey][]SourceConnection),
	}

	// NOTE: we have to record jumps in order of key appearance on the target data source
	// NOTE: we iterate over dataSource hashes instead of a map to preserve the order of data sources

	for _, targetDSHash := range dataSources {
		targetKeyInfos, exists := keysPerPath[targetDSHash]
		if !exists {
			continue
		}

		for _, targetKeyInfo := range targetKeyInfos {
			if !targetKeyInfo.Target {
				continue
			}

			for _, sourceDsHash := range dataSources {
				if targetDSHash == sourceDsHash {
					continue
				}

				sourceKeyInfos, exists := keysPerPath[sourceDsHash]
				if !exists {
					continue
				}

				for _, keyInfo := range sourceKeyInfos {
					if !keyInfo.Source || keyInfo.SelectionSet != targetKeyInfo.SelectionSet {
						continue
					}

					jump := KeyJump{
						From:         sourceDsHash,
						To:           targetDSHash,
						SelectionSet: keyInfo.SelectionSet,
						FieldPaths:   keyInfo.FieldPaths,
						TypeName:     typeName,
					}
					graph.Jumps[sourceDsHash] = append(graph.Jumps[sourceDsHash], jump)
				}
			}
		}
	}

	return graph
}
