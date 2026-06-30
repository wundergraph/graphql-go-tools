package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestCachingPlannerAnnotateWithEmptyProvidersLeavesFetchCacheNil(t *testing.T) {
	fetch := &resolve.SingleFetch{}
	tree := resolve.Single(fetch)
	resp := &resolve.GraphQLResponse{}

	(&cachingPlanner{}).Annotate(resp, tree)

	assert.Equal(t, (*resolve.FetchCacheConfig)(nil), fetch.Cache)
}
