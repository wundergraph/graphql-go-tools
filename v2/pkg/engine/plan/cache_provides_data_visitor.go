package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// cacheProvidesDataVisitor is the P1 caching walk (PLAN.md D9): a gated
// SECOND, filter-free walk over the operation, run on the planningWalker AFTER
// the main planning walk, building the per-fetch ProvidesData tree
// (alias → OriginalName, CacheArgs, entity boundaries). It never re-runs the
// planning visitor and never rebuilds the plan; it only produces the
// side-table attachTo hands to the plan's response.
//
// This is the registration SKELETON — the visitor body lands with task 05.
type cacheProvidesDataVisitor struct {
	walker *astvisitor.Walker
}

// reset clears all per-plan state so a Planner can be reused across Plan calls.
func (v *cacheProvidesDataVisitor) reset() {}

func (v *cacheProvidesDataVisitor) EnterField(ref int) {}

func (v *cacheProvidesDataVisitor) LeaveField(ref int) {}

// attachTo hands the collected ProvidesData side-table to the plan's response
// via resolve.GraphQLResponse.SetCacheProvidesData (task 05).
func (v *cacheProvidesDataVisitor) attachTo(pl Plan) {}
