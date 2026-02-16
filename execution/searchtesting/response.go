package searchtesting

import (
	"bytes"
	"context"
	"testing"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type testPipeline struct {
	PlanConfig    plan.Configuration
	SupergraphDef string
}

func executeQuery(t *testing.T, pipeline *testPipeline, query string, variables string) string {
	t.Helper()

	def, parseReport := astparser.ParseGraphqlDocumentString(pipeline.SupergraphDef)
	if parseReport.HasErrors() {
		t.Fatalf("parse supergraph definition: %s", parseReport.Error())
	}
	op, parseReport := astparser.ParseGraphqlDocumentString(query)
	if parseReport.HasErrors() {
		t.Fatalf("parse query: %s", parseReport.Error())
	}

	// Set variables before normalization so that inline arguments (e.g. query: "shoes")
	// are extracted into the variables map alongside explicit variables.
	if variables != "" {
		op.Input.Variables = []byte(variables)
	}

	if err := asttransform.MergeDefinitionWithBaseSchema(&def); err != nil {
		t.Fatalf("MergeDefinitionWithBaseSchema: %v", err)
	}

	report := &operationreport.Report{}
	norm := astnormalization.NewNormalizer(true, true)
	norm.NormalizeOperation(&op, &def, report)
	if report.HasErrors() {
		t.Fatalf("normalize: %s", report.Error())
	}

	valid := astvalidation.DefaultOperationValidator()
	valid.Validate(&op, &def, report)
	if report.HasErrors() {
		t.Fatalf("validate: %s", report.Error())
	}

	p, err := plan.NewPlanner(pipeline.PlanConfig)
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	executionPlan := p.Plan(&op, &def, "", report)
	if report.HasErrors() {
		t.Fatalf("plan: %s", report.Error())
	}

	proc := postprocess.NewProcessor()
	proc.Process(executionPlan)

	syncPlan, ok := executionPlan.(*plan.SynchronousResponsePlan)
	if !ok {
		t.Fatalf("expected SynchronousResponsePlan, got %T", executionPlan)
	}

	if syncPlan.Response.Info == nil {
		syncPlan.Response.Info = &resolve.GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		}
	}

	resolver := resolve.New(context.Background(), resolve.ResolverOptions{
		MaxConcurrency:          32,
		PropagateSubgraphErrors: true,
	})

	ctx := resolve.NewContext(context.Background())
	// Use op.Input.Variables (post-normalization) which includes both explicit
	// variables and any inline arguments extracted during normalization.
	if len(op.Input.Variables) > 0 {
		ctx.Variables = astjson.MustParseBytes(op.Input.Variables)
	}

	buf := &bytes.Buffer{}
	_, err = resolver.ResolveGraphQLResponse(ctx, syncPlan.Response, nil, buf)
	if err != nil {
		t.Fatalf("ResolveGraphQLResponse: %v", err)
	}

	return buf.String()
}
