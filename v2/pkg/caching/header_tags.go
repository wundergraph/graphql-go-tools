package caching

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

// Header tag tiers. Stored with the entry, unlike Tags which only name an index.
const (
	subgraphHeaderTagPrefix = "subgraph-"
	typeHeaderTagPrefix     = "type-"
)

func SubgraphHeaderTag(subgraph string) string {
	return subgraphHeaderTagPrefix + subgraph
}

func TypeHeaderTag(subgraph, typeName string) string {
	return typeHeaderTagPrefix + subgraph + "-" + typeName
}

// Coarsest first, so a size limit cuts the finest purge surface.
const (
	subgraphHeaderTagTier = iota
	typeHeaderTagTier
	declaredHeaderTagTier
)

// ValidDeclaredHeaderTag reports whether a declared tag can go in a header.
// One spelled like a derived tag passes: the header is the CDN's contract and
// carries what the subgraph declared, the index files it as declared.
func ValidDeclaredHeaderTag(tag string) bool {
	if tag == "" {
		return false
	}
	return !strings.ContainsFunc(tag, func(r rune) bool {
		return unicode.IsControl(r) || r == ' ' || r > unicode.MaxASCII
	})
}

func headerTagTier(headerTag string) int {
	switch {
	case strings.HasPrefix(headerTag, subgraphHeaderTagPrefix):
		return subgraphHeaderTagTier
	case strings.HasPrefix(headerTag, typeHeaderTagPrefix):
		return typeHeaderTagTier
	default:
		return declaredHeaderTagTier
	}
}

// SortHeaderTags groups by tier, coarsest first, keeping the order within a tier.
func SortHeaderTags(headerTags []string) {
	slices.SortStableFunc(headerTags, func(a, b string) int {
		return cmp.Compare(headerTagTier(a), headerTagTier(b))
	})
}

// MergeHeaderTags appends every tag of src not already in dst, first seen order.
func MergeHeaderTags(dst []string, src ...[]string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, headerTag := range dst {
		seen[headerTag] = struct{}{}
	}
	for _, headerTags := range src {
		for _, headerTag := range headerTags {
			if _, ok := seen[headerTag]; ok {
				continue
			}
			seen[headerTag] = struct{}{}
			dst = append(dst, headerTag)
		}
	}
	return dst
}
