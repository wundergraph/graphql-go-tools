package cache

import (
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// The default tag vocabulary every L2 entry is written under. Keys are identity
// — derived from what a fetch sends, never externally addressable — while tags
// are the addressing layer an invalidation endpoint and a tag-purging CDN both
// speak:
//
//	subgraph:{subgraph}
//	type:{subgraph}:{TypeName}                 // Query for a root-field entry
//	entity:{subgraph}:{TypeName}:{key digest}  // entity entries only
//
// Tags are always emitted in that order — coarsest first — so the vocabulary
// reads the same in every op log and every purge request.
const (
	subgraphTagPrefix = "subgraph:"
	typeTagPrefix     = "type:"
	entityTagPrefix   = "entity:"
)

// entityTags renders one entity item's default tags. The entity tag's digest
// covers the item's `@key` fields ALONE — no `@requires` values, no argument
// digest, no partition segment — so every variant of one entity shares it and
// a single purge clears them all. The tags derive from the SAME pre-merge item
// as the entry's keys, so a tag can never describe a different entity than the
// key it is stored under; an item that renders no key subset still carries the
// two coarse tags, and nil means the item has no type to tag under at all.
func (t cacheKeyTemplate) entityTags(item *astjson.Value) []string {
	typeName := t.itemTypeName(item)
	if typeName == "" {
		return nil
	}
	tags := make([]string, 0, 3)
	tags = append(tags, subgraphTagPrefix+t.subgraph, typeTagPrefix+t.subgraph+":"+typeName)
	if keySubset, ok := appendRepresentationObject(nil, t.representation, item, keyFieldsOnly); ok {
		tags = append(tags, entityTagPrefix+t.subgraph+":"+typeName+":"+hashHex(keySubset))
	}
	return tags
}

// rootFieldTags renders a root-field entry's default tags. A root-field
// response is not one entity, so it carries no entity tag; its type is the
// coordinate's — Query — which makes "purge everything this subgraph answers at
// the root" a single tag.
func rootFieldTags(cfg *resolve.FetchCacheConfig) []string {
	return []string{
		subgraphTagPrefix + cfg.SubgraphName,
		typeTagPrefix + cfg.SubgraphName + ":" + cfg.KeySpec.TypeName,
	}
}
