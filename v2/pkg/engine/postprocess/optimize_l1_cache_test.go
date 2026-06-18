package postprocess

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestOptimizeL1CacheProviderThenConsumer(t *testing.T) {
	root := resolve.Sequence(
		resolve.Single(entityFetch(0, "User", object(
			"User",
			field("id"),
			objectField("profile", object(
				"Profile",
				field("displayName"),
				field("email"),
			)),
		))),
		resolve.Single(entityFetch(1, "User", object(
			"User",
			objectField("profile", object(
				"Profile",
				field("displayName"),
			)),
		))),
	)

	processor := &optimizeL1Cache{}
	processor.ProcessFetchTree(root)

	assert.Equal(t, []bool{
		true,
		true,
	}, l1CacheStates(root))
}

func TestOptimizeL1CacheNoConsumer(t *testing.T) {
	root := resolve.Sequence(
		resolve.Single(entityFetch(0, "User", object(
			"User",
			field("id"),
			field("name"),
		))),
	)

	processor := &optimizeL1Cache{}
	processor.ProcessFetchTree(root)

	assert.Equal(t, []bool{
		false,
	}, l1CacheStates(root))
}

func TestOptimizeL1CacheAncestorUnionCoverage(t *testing.T) {
	root := resolve.Sequence(
		resolve.Single(entityFetch(0, "User", object(
			"User",
			field("id"),
		))),
		resolve.Single(entityFetch(1, "User", object(
			"User",
			field("name"),
		))),
		resolve.Single(entityFetch(2, "User", object(
			"User",
			field("id"),
			field("name"),
		))),
	)

	processor := &optimizeL1Cache{}
	processor.ProcessFetchTree(root)

	assert.Equal(t, []bool{
		true,
		true,
		true,
	}, l1CacheStates(root))
}

func TestOptimizeL1CacheDisableFlag(t *testing.T) {
	root := resolve.Sequence(
		resolve.Single(entityFetch(0, "User", object(
			"User",
			field("id"),
			field("name"),
		))),
		resolve.Single(entityFetch(1, "User", object(
			"User",
			field("id"),
		))),
	)

	processor := NewProcessor(DisableOptimizeL1Cache())
	for _, fetchTreeProcessor := range processor.processFetchTree {
		fetchTreeProcessor.ProcessFetchTree(root)
	}

	assert.Equal(t, []bool{
		false,
		false,
	}, l1CacheStates(root))
}

func TestOptimizeL1CacheRunsAfterConcreteFetchConversion(t *testing.T) {
	root := resolve.Sequence(
		resolve.Single(singleEntityFetch(0, "User", object(
			"User",
			field("id"),
			field("name"),
		))),
		resolve.Single(singleEntityFetch(1, "User", object(
			"User",
			field("id"),
		))),
	)

	processor := NewProcessor(
		DisableOrderSequenceByDependencies(),
		DisableCreateParallelNodes(),
	)
	for _, fetchTreeProcessor := range processor.processFetchTree {
		fetchTreeProcessor.ProcessFetchTree(root)
	}

	_, providerIsEntityFetch := root.ChildNodes[0].Item.Fetch.(*resolve.EntityFetch)
	_, consumerIsEntityFetch := root.ChildNodes[1].Item.Fetch.(*resolve.EntityFetch)

	assert.Equal(t, true, providerIsEntityFetch)
	assert.Equal(t, true, consumerIsEntityFetch)
	assert.Equal(t, []bool{
		true,
		true,
	}, l1CacheStates(root))
}

func entityFetch(fetchID int, typeName string, providesData *resolve.Object) *resolve.EntityFetch {
	return &resolve.EntityFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID: fetchID,
		},
		Cache: cacheConfig(typeName, providesData),
	}
}

func singleEntityFetch(fetchID int, typeName string, providesData *resolve.Object) *resolve.SingleFetch {
	return &resolve.SingleFetch{
		FetchConfiguration: resolve.FetchConfiguration{
			RequiresEntityFetch: true,
		},
		FetchDependencies: resolve.FetchDependencies{
			FetchID: fetchID,
		},
		InputTemplate: resolve.InputTemplate{
			Segments: []resolve.TemplateSegment{
				{
					SegmentType: resolve.StaticSegmentType,
					Data:        []byte(`{"representations":[`),
				},
				{
					SegmentType:  resolve.VariableSegmentType,
					VariableKind: resolve.ResolvableObjectVariableKind,
				},
				{
					SegmentType: resolve.StaticSegmentType,
					Data:        []byte(`]}`),
				},
			},
		},
		Cache: cacheConfig(typeName, providesData),
	}
}

func cacheConfig(typeName string, providesData *resolve.Object) *resolve.FetchCacheConfiguration {
	return &resolve.FetchCacheConfiguration{
		KeyTemplate: &resolve.EntityQueryCacheKeyTemplate{
			TypeName: typeName,
		},
		ProvidesData: providesData,
	}
}

func object(typeName string, fields ...*resolve.Field) *resolve.Object {
	return &resolve.Object{
		TypeName: typeName,
		Fields:   fields,
	}
}

func field(name string) *resolve.Field {
	return &resolve.Field{
		Name:  []byte(name),
		Value: &resolve.String{},
	}
}

func objectField(name string, value *resolve.Object) *resolve.Field {
	return &resolve.Field{
		Name:  []byte(name),
		Value: value,
	}
}

func l1CacheStates(root *resolve.FetchTreeNode) []bool {
	var states []bool
	collectL1CacheStates(root, &states)
	return states
}

func collectL1CacheStates(node *resolve.FetchTreeNode, states *[]bool) {
	if node == nil {
		return
	}
	if node.Item != nil {
		switch fetch := node.Item.Fetch.(type) {
		case *resolve.SingleFetch:
			if fetch.Cache != nil {
				*states = append(*states, fetch.Cache.UseL1Cache)
			}
		case *resolve.EntityFetch:
			if fetch.Cache != nil {
				*states = append(*states, fetch.Cache.UseL1Cache)
			}
		case *resolve.BatchEntityFetch:
			if fetch.Cache != nil {
				*states = append(*states, fetch.Cache.UseL1Cache)
			}
		}
	}
	for _, child := range node.ChildNodes {
		collectL1CacheStates(child, states)
	}
}
