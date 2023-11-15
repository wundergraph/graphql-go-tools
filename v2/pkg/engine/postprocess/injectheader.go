package postprocess

import (
	"encoding/json"
	"net/http"

	"github.com/buger/jsonparser"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
)

type ProcessInjectHeader struct {
	header              http.Header
	fetchInputProcessor *ProcessFetchInput
}

func NewProcessInjectHeader(header http.Header) *ProcessInjectHeader {
	p := &ProcessInjectHeader{header: header}
	p.fetchInputProcessor = NewProcessFetchInput(p.injectHeader)
	return p
}

func (p *ProcessInjectHeader) Process(pre plan.Plan) plan.Plan {
	return p.fetchInputProcessor.Process(pre)
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
