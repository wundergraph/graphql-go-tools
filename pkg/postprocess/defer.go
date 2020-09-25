package postprocess

import (
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

type ProcessDefer struct {
	objects []*resolve.Object
	out     *plan.StreamingResponsePlan
	updated bool
}

func (p *ProcessDefer) Process(pre plan.Plan) plan.Plan {

	p.out = nil
	p.updated = false
	p.objects = p.objects[:0]

	switch in := pre.(type) {
	case *plan.SynchronousResponsePlan:
		return p.synchronousResponse(in)
	case *plan.StreamingResponsePlan:
		return p.processStreamingResponsePlan(in)
	default:
		return pre
	}
}

func (p *ProcessDefer) processStreamingResponsePlan(in *plan.StreamingResponsePlan) plan.Plan {
	p.out = in
	for i := range p.out.Response.Patches {
		p.traverseNode(p.out.Response.Patches[i].Value)
	}
	p.traverseNode(p.out.Response.InitialResponse.Data)
	return p.out
}

func (p *ProcessDefer) synchronousResponse(pre *plan.SynchronousResponsePlan) plan.Plan {
	p.out = &plan.StreamingResponsePlan{
		FlushInterval: pre.FlushInterval,
		Response: resolve.GraphQLStreamingResponse{
			InitialResponse: &pre.Response,
		},
	}
	p.traverseNode(p.out.Response.InitialResponse.Data)
	if p.updated {
		return p.out
	}
	return pre
}

func (p *ProcessDefer) traverseNode(node resolve.Node) {

	switch n := node.(type) {
	case *resolve.Object:
		p.objects = append(p.objects, n)
		for i := range n.FieldSets {
			for j := range n.FieldSets[i].Fields {
				if n.FieldSets[i].Fields[j].Defer != nil {
					p.updated = true
					patchIndex, ok := p.createPatch(n, i, j)
					if !ok {
						continue
					}
					n.FieldSets[i].Fields[j].Defer = nil
					n.FieldSets[i].Fields[j].Value = &resolve.Null{
						Defer: resolve.Defer{
							Enabled:    true,
							PatchIndex: patchIndex,
						},
					}
					p.traverseNode(p.out.Response.Patches[patchIndex].Value)
				} else {
					p.traverseNode(n.FieldSets[i].Fields[j].Value)
				}
			}
		}
		p.objects = p.objects[:len(p.objects)-1]
	case *resolve.Array:
		p.traverseNode(n.Item)
	}
}

func (p *ProcessDefer) createPatch(object *resolve.Object, fieldSet, field int) (int, bool) {
	oldValue := object.FieldSets[fieldSet].Fields[field].Value
	var patch *resolve.GraphQLResponsePatch
	if object.FieldSets[fieldSet].HasBuffer && !p.bufferUsedOnNonDeferField(object, fieldSet, field, object.FieldSets[fieldSet].BufferID) {
		patchFetch, ok := p.processFieldSetBuffer(object, fieldSet)
		if !ok {
			return 0, false
		}
		patch = &resolve.GraphQLResponsePatch{
			Value:     oldValue,
			Fetch:     &patchFetch,
			Operation: literal.REPLACE,
		}
		object.FieldSets[fieldSet].HasBuffer = false
		object.FieldSets[fieldSet].BufferID = 0
	} else {
		patch = &resolve.GraphQLResponsePatch{
			Value:     oldValue,
			Operation: literal.REPLACE,
		}
	}
	p.out.Response.Patches = append(p.out.Response.Patches, patch)
	patchIndex := len(p.out.Response.Patches) - 1
	return patchIndex, true
}

func (p *ProcessDefer) bufferUsedOnNonDeferField(object *resolve.Object, fieldSet, field, bufferID int) bool {
	for i := range object.FieldSets {
		if object.FieldSets[i].BufferID != bufferID {
			continue
		}
		for j := range object.FieldSets[i].Fields {
			if i == fieldSet && j == field {
				continue // skip currently evaluated field
			}
			if object.FieldSets[i].Fields[j].Defer == nil {
				return true
			}
		}
	}
	return false
}

func (p *ProcessDefer) processFieldSetBuffer(object *resolve.Object, fieldSet int) (patchFetch resolve.SingleFetch, ok bool) {
	id := object.FieldSets[fieldSet].BufferID
	if p.objects[len(p.objects)-1].Fetch == nil {
		return patchFetch, false
	}
	switch fetch := p.objects[len(p.objects)-1].Fetch.(type) {
	case *resolve.SingleFetch:
		if fetch.BufferId != id {
			return patchFetch, false
		}
		patchFetch = *fetch
		patchFetch.BufferId = 0
		p.objects[len(p.objects)-1].Fetch = nil
		return patchFetch, true
	case *resolve.ParallelFetch:
		for k := range fetch.Fetches {
			if id == fetch.Fetches[k].BufferId {
				patchFetch = *fetch.Fetches[k]
				patchFetch.BufferId = 0
				fetch.Fetches = append(fetch.Fetches[:k], fetch.Fetches[k+1:]...)
				if len(fetch.Fetches) == 1 {
					p.objects[len(p.objects)-1].Fetch = fetch.Fetches[0]
				}
				return patchFetch, true
			}
		}
	}
	return patchFetch, false
}
