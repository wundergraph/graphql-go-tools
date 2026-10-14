package caching

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
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

// Coarsest first, so a size limit cuts the finest purge surface.
const (
	subgraphSurrogateKeyTier = iota
	typeSurrogateKeyTier
	declaredSurrogateKeyTier
)

// ValidDeclaredSurrogateKey reports whether a declared tag can go in a header.
// One spelled like a derived key passes: the header is the CDN's contract and
// carries what the subgraph declared, the index files it as declared.
func ValidDeclaredSurrogateKey(tag string) bool {
	if tag == "" {
		return false
	}
	return !strings.ContainsFunc(tag, func(r rune) bool {
		return unicode.IsControl(r) || r == ' ' || r > unicode.MaxASCII
	})
}

func surrogateKeyTier(surrogateKey string) int {
	switch {
	case strings.HasPrefix(surrogateKey, subgraphSurrogateKeyPrefix):
		return subgraphSurrogateKeyTier
	case strings.HasPrefix(surrogateKey, typeSurrogateKeyPrefix):
		return typeSurrogateKeyTier
	default:
		return declaredSurrogateKeyTier
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
