package engine_test

import (
	"testing"

	engine "github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache/cachetesting"
)

func TestCaching_StageL2RootReusesEntity_Golden(t *testing.T) {
	engine.Plan(t, cachetesting.StageL2RootReusesEntity, `{ product(upc:"1") { name } }`, map[string]string{
		"*": `{"data":{"product":{"name":"Table"}}}`,
	})
}
