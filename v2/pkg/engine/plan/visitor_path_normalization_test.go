package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNormalizePathRemovingFragments locks the invariant that the regex used by
// isEntityBoundaryField / isEntityRootField strips inline-fragment type markers
// from walker paths so that boundary comparisons are shape-independent.
//
// Regression guard: isEntityRootField previously compared a non-normalized
// current path against a normalized boundary path, so a query that wraps the
// boundary in `... on User { ... }` caused the prefix check to silently fail.
func TestNormalizePathRemovingFragments(t *testing.T) {
	v := &Visitor{}
	v.caching = newCachingPlannerState(v)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no fragment", "query.meInterface.reviews", "query.meInterface.reviews"},
		{"single inline fragment", "query.meInterface.$0User.reviews", "query.meInterface.reviews"},
		{"nested inline fragments", "query.meUnion.$0User.profile.$1Admin.role", "query.meUnion.profile.role"},
		{"trailing inline fragment", "query.meUnion.$0User", "query.meUnion"},
		{"fragment marker with digit", "query.root.$10Foo.child", "query.root.child"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := v.caching.normalizePathRemovingFragments(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestIsEntityRootPath is the focused A42 regression. Boundary paths stored by
// isEntityBoundaryField are already normalized (inline-fragment markers
// stripped). If the walker-side path check doesn't re-normalize before the
// prefix comparison, queries that wrap the boundary in an inline fragment
// silently fail entity-root detection — at runtime that shows up as missing
// entity L1/L2 population for subgraphs that return their entity boundary
// behind a fragment like `... on User { reviews }`.
//
// Before the fix this test's "fragment wraps the boundary directly" case
// returned false; after the fix it returns true.
func TestIsEntityRootPath(t *testing.T) {
	v := &Visitor{}
	v.caching = newCachingPlannerState(v)

	cases := []struct {
		name         string
		boundaryPath string
		fullPath     string
		want         bool
	}{
		{
			name:         "no fragment — direct child",
			boundaryPath: "query.meInterface.reviews",
			fullPath:     "query.meInterface.reviews.body",
			want:         true,
		},
		{
			name:         "fragment inside the path — direct child after normalization",
			boundaryPath: "query.meInterface.reviews",
			fullPath:     "query.meInterface.$0User.reviews.body",
			want:         true,
		},
		{
			name:         "fragment after the boundary — direct child after normalization",
			boundaryPath: "query.meInterface.reviews",
			fullPath:     "query.meInterface.reviews.$0Review.body",
			want:         true,
		},
		{
			name:         "deeper descendant is not a direct child",
			boundaryPath: "query.meInterface.reviews",
			fullPath:     "query.meInterface.reviews.author.name",
			want:         false,
		},
		{
			name:         "deeper descendant through fragment — still not a direct child",
			boundaryPath: "query.meInterface.reviews",
			fullPath:     "query.meInterface.$0User.reviews.author.name",
			want:         false,
		},
		{
			name:         "unrelated path",
			boundaryPath: "query.meInterface.reviews",
			fullPath:     "query.products.price",
			want:         false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := v.caching.isEntityRootPath(tc.boundaryPath, tc.fullPath)
			assert.Equal(t, tc.want, got)
		})
	}
}
