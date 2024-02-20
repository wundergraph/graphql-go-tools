package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

type FetchExtractor struct {
}

func (e *FetchExtractor) Process(pre plan.Plan) plan.Plan {
	fieldsWithFetch := NewFetchFinder().Find(pre)

	createFetchesCopy := NewFetchesCopier(fieldsWithFetch)
	return createFetchesCopy.Process(pre)
}
