package engine_test

import (
	"testing"

	engine "github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache/cachetesting"
)

func TestCaching_StageL2RootFields_Golden(t *testing.T) {
	engine.Plan(t, cachetesting.StageL2RootFields, "{ topProducts { upc name } }", map[string]string{
		"*": `{"data":{"topProducts":[{"upc":"1","name":"Table"}]}}`,
	})
}
