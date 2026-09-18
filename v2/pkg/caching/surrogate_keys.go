package caching

import (
	"cmp"
	"slices"
	"strings"
)

// Surrogate key tiers. Stored with the entry, unlike Tags which only name an index.
const (
	subgraphSurrogateKeyPrefix = "subgraph-"
	typeSurrogateKeyPrefix     = "type-"
)

func SubgraphSurrogateKey(subgraph string) string {
	return subgraphSurrogateKeyPrefix + subgraph
}

func TypeSurrogateKey(subgraph, typeName string) string {
	return typeSurrogateKeyPrefix + subgraph + "-" + typeName
}

// surrogateKeyTier: coarsest first, so a size limit cuts the finest purge surface.
func surrogateKeyTier(surrogateKey string) int {
	switch {
	case strings.HasPrefix(surrogateKey, subgraphSurrogateKeyPrefix):
		return 0
	case strings.HasPrefix(surrogateKey, typeSurrogateKeyPrefix):
		return 1
	default:
		return 2
	}
}

// SortSurrogateKeys groups by tier, coarsest first, keeping the order within a tier.
func SortSurrogateKeys(surrogateKeys []string) {
	slices.SortStableFunc(surrogateKeys, func(a, b string) int {
		return cmp.Compare(surrogateKeyTier(a), surrogateKeyTier(b))
	})
}

// MergeSurrogateKeys appends every tag of src not already in dst, first seen order.
func MergeSurrogateKeys(dst []string, src ...[]string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, surrogateKey := range dst {
		seen[surrogateKey] = struct{}{}
	}
	for _, surrogateKeys := range src {
		for _, surrogateKey := range surrogateKeys {
			if _, ok := seen[surrogateKey]; ok {
				continue
			}
			seen[surrogateKey] = struct{}{}
			dst = append(dst, surrogateKey)
		}
	}
	return dst
}
