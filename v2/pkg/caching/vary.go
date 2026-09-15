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
		for _, name := range strings.Split(field, ",") {
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
