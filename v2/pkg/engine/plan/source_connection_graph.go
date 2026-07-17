package plan

// KeyJump represents possible jump from one data source to another
type KeyJump struct {
	From DSHash
	To   DSHash
	// SelectionSet is the selection set of the TARGET datasource @key -
	// the key which the _entities representation has to satisfy to enter the To datasource.
	SelectionSet string
	// FieldPaths are the field paths of the TARGET datasource @key.
	FieldPaths []KeyInfoFieldPath
	TypeName   string
	// Fallback marks a jump where the source key is only a subset of the target compound @key.
	// Such a jump requires gathering the missing key members from other datasources first,
	// so it is used only when no plan could be built from exact key jumps
	// (see DataSourceFilter.allowFallbackKeyJumps).
	Fallback bool
	// SourcePaths is set only on fallback jumps: the field paths of the SOURCE datasource key,
	// i.e. the subset of FieldPaths which the From datasource can provide by itself.
	// The difference FieldPaths - SourcePaths is the missing key members to gather
	// (see nodeSelectionVisitor.addKeyRequirementsToOperation).
	SourcePaths []KeyInfoFieldPath
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
	Source          DSHash
	Target          DSHash
	IncludeFallback bool
}

// DataSourceJumpsGraph represents a graph of possible jumps between each data sources
// We need a way to quickly find the shortest path from one data source to another
type DataSourceJumpsGraph struct {
	Jumps map[DSHash][]KeyJump
	Cache map[JumpCacheKey][]SourceConnection
}

func (g *DataSourceJumpsGraph) GetPaths(source DSHash, target DSHash) ([]SourceConnection, bool) {
	return g.getPaths(source, target, false)
}

func (g *DataSourceJumpsGraph) GetPathsWithFallback(source DSHash, target DSHash) ([]SourceConnection, bool) {
	return g.getPaths(source, target, true)
}

func (g *DataSourceJumpsGraph) getPaths(source DSHash, target DSHash, includeFallback bool) ([]SourceConnection, bool) {
	// Create a cache key
	key := JumpCacheKey{Source: source, Target: target, IncludeFallback: includeFallback}

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
			if jump.Fallback && !includeFallback {
				continue
			}

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
					if !keyInfo.Source {
						continue
					}

					// An exact match of the key selection sets gives a regular jump.
					// When the source key is a strict subset of the target compound key,
					// we still record the jump, but mark it as a fallback:
					// it is usable only after the missing key members are gathered
					// from other datasources.
					fallback := false
					if keyInfo.SelectionSet != targetKeyInfo.SelectionSet {
						if !keyInfoFieldsCoverTargetKey(keyInfo, targetKeyInfo) {
							continue
						}
						fallback = true
					}

					jump := KeyJump{
						From:         sourceDsHash,
						To:           targetDSHash,
						SelectionSet: targetKeyInfo.SelectionSet,
						FieldPaths:   targetKeyInfo.FieldPaths,
						TypeName:     typeName,
						Fallback:     fallback,
					}
					if fallback {
						jump.SourcePaths = keyInfo.FieldPaths
					}
					graph.Jumps[sourceDsHash] = append(graph.Jumps[sourceDsHash], jump)
				}
			}
		}
	}

	return graph
}

// keyInfoFieldsCoverTargetKey reports whether every field path of the source key
// is a member of the target key, i.e. the source key is a subset of the target compound key.
// Such a source key alone is not enough to jump into the target datasource,
// but it could be complemented by gathering the missing members from other datasources.
func keyInfoFieldsCoverTargetKey(sourceKey, targetKey KeyInfo) bool {
	if sourceKey.SelectionSet == targetKey.SelectionSet {
		return true
	}

	if len(sourceKey.FieldPaths) == 0 || len(targetKey.FieldPaths) == 0 {
		return false
	}

	targetPaths := make(map[string]struct{}, len(targetKey.FieldPaths))
	for _, path := range targetKey.FieldPaths {
		targetPaths[path.Path] = struct{}{}
	}

	for _, path := range sourceKey.FieldPaths {
		if _, ok := targetPaths[path.Path]; !ok {
			return false
		}
	}

	return true
}
