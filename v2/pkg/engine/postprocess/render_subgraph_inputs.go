package postprocess

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// renderSubgraphInputs renders the deferred entity-fetch input string from the
// planner's SubgraphOperation artifact and then clears the artifact so no AST
// survives postprocessing.
//
// It runs unconditionally (even when merging is disabled and even under
// DisableResolveInputTemplates): the datasource-testing mode needs a readable
// printed Input, and every SubgraphOperation must be cleared regardless of the
// MultiFetch flag. Artifact clearing lives here (moved out of createMultiFetch)
// so that clearing always happens after any merge has consumed the artifacts.
//
// The stage runs after createMultiFetch (so merged-away members are already
// removed from the tree and the survivors are known) and before
// resolveInputTemplates (which consumes the rendered Input string).
type renderSubgraphInputs struct {
	// disableRewriteOpNames mirrors fetchIDAppender's disable flag: the suffix
	// compensation below replicates the appender's string rewrite, so the two
	// must always be gated identically.
	disableRewriteOpNames bool
}

func (r *renderSubgraphInputs) ProcessFetchTree(root *resolve.FetchTreeNode) {
	r.traverseNode(root)
}

func (r *renderSubgraphInputs) traverseNode(node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if fetch, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
			r.processSingleFetch(fetch)
		}
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		for i := range node.ChildNodes {
			r.traverseNode(node.ChildNodes[i])
		}
	}
}

func (r *renderSubgraphInputs) processSingleFetch(fetch *resolve.SingleFetch) {
	op := fetch.SubgraphOperation
	if op == nil {
		return
	}
	// Render only when the planner deferred input assembly (empty Input). When
	// the planner still printed the input eagerly, keep its bytes verbatim and
	// only clear the artifact.
	if fetch.Input == "" {
		r.render(fetch, op)
	}
	fetch.SubgraphOperation = nil
}

// render builds the fetch input from the op,
// then applies the fetchIDAppender operation-name suffix compensation.
func (r *renderSubgraphInputs) render(fetch *resolve.SingleFetch, op *resolve.SubgraphOperation) {
	// Rebuild body.variables from the recorded fragments in write order.
	var variables []byte
	for i := range op.Variables {
		variables, _ = sjson.SetRawBytes(variables, op.Variables[i].Name, op.Variables[i].Value)
	}

	// Reuse the print-once cached minified operation (seeded by the planner);
	// PrintedQuery falls back to printing Document for artifacts that were not printed eagerly.
	query, err := op.PrintedQuery()
	if err != nil {
		return
	}

	input := httpclient.AssembleGraphQLRequestInput(variables, query, op.Envelope.Header, op.Envelope.URL, op.Envelope.Method)

	// The planner deferred the pretty query-plan print for mergeable entity
	// fetches; render it here for surviving (unmerged) fetches. DependsOnFields
	// was kept intact by the planner. QueryPlan is the same object as
	// fetch.Info.QueryPlan (visitor wires them to the same pointer).
	if fetch.QueryPlan != nil && fetch.QueryPlan.Query == "" && op.Document != nil {
		if pretty, prettyErr := astprinter.PrintStringIndent(op.Document, "    "); prettyErr == nil {
			fetch.QueryPlan.Query = pretty
		}
	}

	// fetchIDAppender compensation: fetch_id_appender.go mutates
	// only printed strings and no-ops on the empty deferred Input, so we apply
	// the identical whole-input first-occurrence replace after assembly. This
	// preserves today's semantics exactly, including the pre-existing quirk that
	// the replace could hit a URL/header substring.
	if !r.disableRewriteOpNames && fetch.OperationName != "" {
		expandedName := fmt.Sprintf("%s__%d", fetch.OperationName, fetch.FetchID)
		input = bytes.Replace(input, []byte(fetch.OperationName), []byte(expandedName), 1)
		if fetch.QueryPlan != nil {
			fetch.QueryPlan.Query = strings.Replace(fetch.QueryPlan.Query, fetch.OperationName, expandedName, 1)
		}
	}

	fetch.Input = string(input)
}
