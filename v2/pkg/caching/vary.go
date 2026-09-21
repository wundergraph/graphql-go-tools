package caching

import (
	"net/http"
	"slices"
	"strings"
)

// MaxVaryHeaders bounds a vary record. A response naming more is not stored.
const MaxVaryHeaders = 100

func Vary(headers http.Header) (names []string, star bool) {
	for _, field := range headers.Values("Vary") {
		for name := range strings.SplitSeq(field, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			switch name {
			case "":
				continue
			case "*":
				return nil, true
			default:
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return slices.Compact(names), false
}

// VaryDigest digests the values sent carries for names, in order, an absent
// header counting as empty. sent is the request as the subgraph receives it,
// since that is what its answer could have varied on.
func VaryDigest(names []string, sent http.Header) Digest {
	parts := make([][]byte, 0, 2*len(names))
	for _, name := range names {
		parts = append(parts, []byte(name), []byte(strings.Join(sent.Values(name), ",")))
	}
	return DigestParts(parts...)
}

// MaxVarySets bounds the name sets a record keeps. Past it the oldest go.
const MaxVarySets = 8

// MergeVarySets is the record to write after a response varying on own: own
// first, then every set the record held before, so the variants stored under
// them stay reachable. Names come sorted and deduplicated from Vary, so equal
// sets are equal slices.
func MergeVarySets(own []string, seen [][]string) [][]string {
	sets := make([][]string, 0, 1+len(seen))
	sets = append(sets, own)
	for _, set := range seen {
		if len(sets) == MaxVarySets {
			break
		}
		if len(set) == 0 || slices.ContainsFunc(sets, func(kept []string) bool { return slices.Equal(kept, set) }) {
			continue
		}
		sets = append(sets, set)
	}
	return sets
}
