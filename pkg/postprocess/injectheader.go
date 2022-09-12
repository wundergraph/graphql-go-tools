package postprocess

import (
	"encoding/json"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
	"github.com/buger/jsonparser"
	"net/http"
)

type ProcessInjectHeader struct {
	header http.Header
}

func NewProcessInjectHeader(header http.Header) *ProcessInjectHeader {
	return &ProcessInjectHeader{header: header}
}

func (p *ProcessInjectHeader) Process(pre plan.Plan) plan.Plan {
	switch t := pre.(type) {
	case *plan.SynchronousResponsePlan:
		p.traverseNode(t.Response.Data)
	case *plan.StreamingResponsePlan:
		p.traverseNode(t.Response.InitialResponse.Data)
		for i := range t.Response.Patches {
			p.traverseFetch(t.Response.Patches[i].Fetch)
			p.traverseNode(t.Response.Patches[i].Value)
		}
	case *plan.SubscriptionResponsePlan:
		p.traverseTrigger(&t.Response.Trigger)
		p.traverseNode(t.Response.Response.Data)
	}
	return pre
}

func (p *ProcessInjectHeader) traverseNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		p.traverseFetch(n.Fetch)
		for i := range n.Fields {
			p.traverseNode(n.Fields[i].Value)
		}
	case *resolve.Array:
		p.traverseNode(n.Item)
	}
}

func (p *ProcessInjectHeader) traverseFetch(fetch resolve.Fetch) {
	if fetch == nil {
		return
	}
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		p.traverseSingleFetch(f)
	case *resolve.BatchFetch:
		p.traverseSingleFetch(f.Fetch)
	case *resolve.ParallelFetch:
		for i := range f.Fetches {
			p.traverseFetch(f.Fetches[i])
		}
	}
}

func (p *ProcessInjectHeader) traverseTrigger(trigger *resolve.GraphQLSubscriptionTrigger) {
	trigger.Input = []byte(p.injectHeader(trigger.Input))
}

func (p *ProcessInjectHeader) traverseSingleFetch(fetch *resolve.SingleFetch) {
	fetch.Input = p.injectHeader([]byte(fetch.Input))
}

func (p *ProcessInjectHeader) injectHeader(input []byte) string {
	var header http.Header
	val, valType, _, err := jsonparser.Get(input, "header")
	if err != nil && valType != jsonparser.NotExist {
		return string(input)
	}

	switch valType {
	case jsonparser.NotExist:
		header = p.header
	case jsonparser.Object:
		err := json.Unmarshal(val, &header)
		if err != nil {
			return string(input)
		}
		for key, val := range p.header {
			header[key] = val
		}
	default:
		return string(input)
	}

	m, err := json.Marshal(header)
	if err != nil {
		return string(input)
	}
	updated, err := jsonparser.Set(input, m, "header")
	if err != nil {
		return string(input)
	}
	return string(updated)
}
