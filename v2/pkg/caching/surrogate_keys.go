package caching

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

// Prefixes of the derived tiers. Stored with the entry, unlike Tags which only name an index.
const (
	subgraphSurrogateKeyPrefix = "subgraph-"
	typeSurrogateKeyPrefix     = "type-"
)

// SurrogateKeyTier is where a surrogate key came from. Coarsest first, so a
// size limit cuts the finest purge surface.
type SurrogateKeyTier uint8

const (
	// SurrogateKeyTierSubgraph names the subgraph that answered.
	SurrogateKeyTierSubgraph SurrogateKeyTier = iota
	// SurrogateKeyTierType names an entity type of that subgraph.
	SurrogateKeyTierType
	// SurrogateKeyTierDeclared is a tag the subgraph declared, verbatim.
	SurrogateKeyTierDeclared
)

func (t SurrogateKeyTier) valid() bool {
	return t <= SurrogateKeyTierDeclared
}

// SurrogateKey is what a client response carries so a CDN can purge by it.
// The tier is kept beside the value: a declared tag may be spelled like a
// derived one.
type SurrogateKey struct {
	Value string
	Tier  SurrogateKeyTier
}

func SubgraphSurrogateKey(subgraph string) SurrogateKey {
	return SurrogateKey{Value: subgraphSurrogateKeyPrefix + subgraph, Tier: SurrogateKeyTierSubgraph}
}

func TypeSurrogateKey(subgraph, typeName string) SurrogateKey {
	return SurrogateKey{Value: typeSurrogateKeyPrefix + subgraph + "-" + typeName, Tier: SurrogateKeyTierType}
}

func DeclaredSurrogateKey(tag string) SurrogateKey {
	return SurrogateKey{Value: tag, Tier: SurrogateKeyTierDeclared}
}

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

// legacySurrogateKeyTier guesses by prefix, for entries stored without tiers.
func legacySurrogateKeyTier(value string) SurrogateKeyTier {
	switch {
	case strings.HasPrefix(value, subgraphSurrogateKeyPrefix):
		return SurrogateKeyTierSubgraph
	case strings.HasPrefix(value, typeSurrogateKeyPrefix):
		return SurrogateKeyTierType
	default:
		return SurrogateKeyTierDeclared
	}
}

// SortSurrogateKeys groups by tier, coarsest first, keeping the order within a tier.
func SortSurrogateKeys(surrogateKeys []SurrogateKey) {
	slices.SortStableFunc(surrogateKeys, func(a, b SurrogateKey) int {
		return cmp.Compare(a.Tier, b.Tier)
	})
}

// MergeSurrogateKeys appends every key of src not already in dst, first seen
// order. Equal values of different tiers are both kept.
func MergeSurrogateKeys(dst []SurrogateKey, src ...[]SurrogateKey) []SurrogateKey {
	seen := make(map[SurrogateKey]struct{}, len(dst))
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

// SurrogateKeyValues is the distinct values, first seen order, tiers dropped.
func SurrogateKeyValues(surrogateKeys []SurrogateKey) []string {
	if len(surrogateKeys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(surrogateKeys))
	values := make([]string, 0, len(surrogateKeys))
	for _, surrogateKey := range surrogateKeys {
		if _, ok := seen[surrogateKey.Value]; ok {
			continue
		}
		seen[surrogateKey.Value] = struct{}{}
		values = append(values, surrogateKey.Value)
	}
	return values
}
