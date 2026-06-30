package engine_test

import (
	"testing"

	engine "github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache/cachetesting"
)

func TestCaching_StageL2Entities_Golden(t *testing.T) {
	engine.Plan(t, cachetesting.StageL2Entities, "{ topProducts { upc name reviews { body } } }", map[string]string{
		"*": `{"data":{"topProducts":[{"upc":"1","name":"Table","reviews":[{"body":"Solid"}]}]}}`,
	})
}
