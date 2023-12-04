package postprocess

import (
	"slices"
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// ProcessDataSource is a postprocessor that processes input template and sort fetches of serial fetch
type ProcessDataSource struct{}

func (d *ProcessDataSource) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		d.traverseNode(t.Response.Data)
	case *plan.SubscriptionResponsePlan:
		d.traverseTrigger(&t.Response.Trigger)
		d.traverseNode(t.Response.Response.Data)
	}
	return pre
}

func (d *ProcessDataSource) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		d.traverseFetch(n.Fetch)
		for i := range n.Fields {
			d.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		d.traverseNode(n.Item)
	}
}

func (d *ProcessDataSource) traverseFetch(fetch resolve.Fetch) {
	if fetch == nil {
		return
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		d.traverseSingleFetch(f)
	case *resolve.ParallelFetch:
		for i := range f.Fetches {
			d.traverseFetch(f.Fetches[i])
		}
	case *resolve.SerialFetch:
		slices.SortFunc(f.Fetches, func(a, b resolve.Fetch) int {
			// serial fetch always has a single fetch as child
			// sort descending by serial id
			return b.(*resolve.SingleFetch).SerialID - a.(*resolve.SingleFetch).SerialID
		})
		for i := range f.Fetches {
			d.traverseFetch(f.Fetches[i])
		}
	default:
		// at this point, we should not have any other types of fetches
		// as from planner we could get only SingleFetch, ParallelFetch and SerialFetch
		// other types of fetches are created only during postprocessing via DataSourceFetch postprocessor
	}
}

func (d *ProcessDataSource) traverseTrigger(trigger *resolve.GraphQLSubscriptionTrigger) {
	d.resolveInputTemplate(trigger.Variables, string(trigger.Input), &trigger.InputTemplate)
	trigger.Input = nil
	trigger.Variables = nil
}

func (d *ProcessDataSource) traverseSingleFetch(fetch *resolve.SingleFetch) {
	d.resolveInputTemplate(fetch.Variables, fetch.Input, &fetch.InputTemplate)
	fetch.Input = ""
	fetch.Variables = nil
	fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull = fetch.SetTemplateOutputToNullOnVariableNull
	fetch.SetTemplateOutputToNullOnVariableNull = false
}

func (d *ProcessDataSource) resolveInputTemplate(variables resolve.Variables, input string, template *resolve.InputTemplate) {

	if input == "" {
		return
	}

	if !strings.Contains(input, "$$") {
		template.Segments = append(template.Segments, resolve.TemplateSegment{
			SegmentType: resolve.StaticSegmentType,
			Data:        []byte(input),
		})
		return
	}

	segments := strings.Split(input, "$$")

	isVariable := false
	for _, seg := range segments {
		switch {
		case isVariable:
			i, _ := strconv.Atoi(seg)
			variableTemplateSegment := (variables)[i].TemplateSegment()
			template.Segments = append(template.Segments, variableTemplateSegment)
			isVariable = false
		default:
			template.Segments = append(template.Segments, resolve.TemplateSegment{
				SegmentType: resolve.StaticSegmentType,
				Data:        []byte(seg),
			})
			isVariable = true
		}
	}
}
