package plan

import "slices"

// refPair is a single provenance record of a field rewrite.
// In a copy log it is (original field ref -> new field ref).
// In a merge log it is (removed field ref -> surviving field ref).
type refPair struct {
	from int
	to   int
}

// buildRefMappings composes the copy and merge logs of a single rewrite
// into the two RewriteResult maps.
//
// The copy log holds one entry per field created during the rewrite, pointing at
// the pre-rewrite field ref it was created from. The merge log holds one entry per
// field merged away during the post-rewrite normalization, pointing at the field it
// was merged into, in chronological order - a survivor of an earlier merge can be
// removed by a later one.
func buildRefMappings(copyLog, mergeLog []refPair) (changedFieldRefs, fieldRefOrigins map[int][]int) {
	fieldRefOrigins = make(map[int][]int, len(copyLog))
	for _, c := range copyLog {
		fieldRefOrigins[c.to] = appendUniqueRef(fieldRefOrigins[c.to], c.from)
	}

	// a removed field transfers its origins to the survivor.
	// The merge log is acyclic: a removed field leaves the selection set and can never
	// participate in a later merge, so the redirect chain below always terminates.
	redirects := make(map[int]int, len(mergeLog))
	for _, m := range mergeLog {
		for _, originRef := range fieldRefOrigins[m.from] {
			fieldRefOrigins[m.to] = appendUniqueRef(fieldRefOrigins[m.to], originRef)
		}
		delete(fieldRefOrigins, m.from)
		redirects[m.from] = m.to
	}

	changedFieldRefs = make(map[int][]int, len(copyLog))
	for _, c := range copyLog {
		newRef := c.to
		for {
			survivorRef, ok := redirects[newRef]
			if !ok {
				break
			}
			newRef = survivorRef
		}
		changedFieldRefs[c.from] = appendUniqueRef(changedFieldRefs[c.from], newRef)
	}

	return changedFieldRefs, fieldRefOrigins
}

func appendUniqueRef(refs []int, ref int) []int {
	if slices.Contains(refs, ref) {
		return refs
	}
	return append(refs, ref)
}
