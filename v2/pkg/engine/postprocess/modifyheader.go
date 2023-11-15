package postprocess

import (
	"encoding/json"
	"net/http"

	"github.com/buger/jsonparser"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
)

type HeaderModifier func(header http.Header)

type ProcessModifyHeader struct {
	headerModifier      HeaderModifier
	fetchInputProcessor *ProcessFetchInput
}

func NewProcessModifyHeader(headerModifier HeaderModifier) *ProcessModifyHeader {
	p := &ProcessModifyHeader{
		headerModifier: headerModifier,
	}
	p.fetchInputProcessor = NewProcessFetchInput(p.modifyHeader)
	return p
}

func (p *ProcessModifyHeader) Process(pre plan.Plan) plan.Plan {
	return p.fetchInputProcessor.Process(pre)
}

func (p *ProcessModifyHeader) modifyHeader(input []byte) string {
	if p.headerModifier == nil {
		return string(input)
	}

	var header http.Header
	val, valType, _, err := jsonparser.Get(input, "header")
	if err != nil && valType != jsonparser.NotExist {
		return string(input)
	}

	switch valType {
	case jsonparser.NotExist:
		header = make(http.Header)
	case jsonparser.Object:
		err := json.Unmarshal(val, &header)
		if err != nil {
			return string(input)
		}
	default:
		return string(input)
	}

	p.headerModifier(header)

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
