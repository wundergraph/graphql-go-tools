package postprocess

import (
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// resolveInputTemplates is a postprocessor that resolves input template
type resolveInputTemplates struct {
}

func (r *resolveInputTemplates) ProcessFetchTree(root *resolve.FetchTreeNode) {
	r.traverseNode(root)
}

func (r *resolveInputTemplates) ProcessTrigger(trigger *resolve.GraphQLSubscriptionTrigger) {
	r.resolveInputTemplate(trigger.Variables, string(trigger.Input), &trigger.InputTemplate)
	trigger.Input = nil
	trigger.Variables = nil
}

func (r *resolveInputTemplates) traverseNode(node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		r.traverseSingleFetch(node.Item.Fetch.(*resolve.SingleFetch))
	case resolve.FetchTreeNodeKindParallel:
		for i := range node.ParallelNodes {
			r.traverseNode(node.ParallelNodes[i])
		}
	case resolve.FetchTreeNodeKindSequence:
		for i := range node.SerialNodes {
			r.traverseNode(node.SerialNodes[i])
		}
	}
}

func (r *resolveInputTemplates) traverseSingleFetch(fetch *resolve.SingleFetch) {
	r.resolveInputTemplate(fetch.Variables, fetch.Input, &fetch.InputTemplate)
	fetch.Input = ""
	fetch.Variables = nil
	fetch.InputTemplate.SetTemplateOutputToNullOnVariableNull = fetch.SetTemplateOutputToNullOnVariableNull
	fetch.SetTemplateOutputToNullOnVariableNull = false
}

func (r *resolveInputTemplates) resolveInputTemplate(variables resolve.Variables, input string, template *resolve.InputTemplate) {

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
