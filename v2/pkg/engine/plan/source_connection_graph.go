package plan

// KeyJump represents possible jump from one data source to another
type KeyJump struct {
	From         DSHash
	To           DSHash
	SelectionSet string
	FieldPaths   []string
}

type SourceConnectionType int

const (
	SourceConnectionTypeDirect SourceConnectionType = iota
	SourceConnectionTypeIndirect
)

type SourceConnection struct {
	Jumps []KeyJump
	Type  SourceConnectionType
}

// DataSourceJumpsGraph represents a graph of possible jumps between each data sources
// We need a way to quickly find the shortest path from one data source to another
type DataSourceJumpsGraph struct {
	Jumps map[DSHash][]KeyJump
}

func (g *DataSourceJumpsGraph) GetPaths(source DSHash, target DSHash) ([]SourceConnection, bool) {
	// Initialize a map to store visited nodes to prevent cycles
	visited := make(map[DSHash]bool)

	// Helper function to perform DFS and find paths
	var dfs func(current DSHash, path []KeyJump) ([]SourceConnection, bool)
	dfs = func(current DSHash, path []KeyJump) ([]SourceConnection, bool) {
		if current == target {
			t := SourceConnectionTypeDirect
			if len(path) > 1 {
				t = SourceConnectionTypeIndirect
			}

			return []SourceConnection{{Jumps: path, Type: t}}, true
		}

		visited[current] = true
		defer func() { visited[current] = false }() // Unmark the node after visiting

		var connections []SourceConnection
		found := false

		for _, jump := range g.Jumps[current] {
			if !visited[jump.To] {
				newPath := append(path, jump)
				if conns, exists := dfs(jump.To, newPath); exists {
					connections = append(connections, conns...)
					found = true
				}
			}
		}

		return connections, found
	}

	// Start DFS from the source
	return dfs(source, nil)
}

func NewDataSourceJumpsGraph(keysPerPath map[DSHash][]KeyInfo) *DataSourceJumpsGraph {
	graph := &DataSourceJumpsGraph{
		Jumps: make(map[DSHash][]KeyJump),
	}

	for dsHash, keyInfos := range keysPerPath {
		for _, keyInfo := range keyInfos {
			if keyInfo.Source {
				for targetDSHash, targetKeyInfos := range keysPerPath {
					if targetDSHash != dsHash {
						for _, targetKeyInfo := range targetKeyInfos {
							if targetKeyInfo.Target && keyInfo.SelectionSet == targetKeyInfo.SelectionSet {
								jump := KeyJump{
									From:         dsHash,
									To:           targetDSHash,
									SelectionSet: keyInfo.SelectionSet,
									FieldPaths:   keyInfo.FieldPaths,
								}
								graph.Jumps[dsHash] = append(graph.Jumps[dsHash], jump)
							}
						}
					}
				}
			}
		}
	}

	return graph
}
