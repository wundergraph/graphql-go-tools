package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheFetchInfo_String(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var cfi *CacheFetchInfo
		assert.Equal(t, "", cfi.String())
	})
	t.Run("entity fetch", func(t *testing.T) {
		cfi := &CacheFetchInfo{
			DataSourceName: "accounts",
			FetchType:      "entity",
			TypeName:       "User",
		}
		assert.Equal(t, "accounts: entity(User)", cfi.String())
	})
	t.Run("rootField fetch", func(t *testing.T) {
		cfi := &CacheFetchInfo{
			DataSourceName: "products",
			FetchType:      "rootField",
			TypeName:       "Query",
			FieldName:      "topProducts",
		}
		assert.Equal(t, "products: rootField(Query.topProducts)", cfi.String())
	})
}

func TestWithCacheFetchInfo(t *testing.T) {
	t.Run("nil FetchInfo returns original context", func(t *testing.T) {
		ctx := context.Background()
		got := WithCacheFetchInfo(ctx, nil, FetchCacheConfiguration{})
		assert.Equal(t, ctx, got)
		assert.Nil(t, GetCacheFetchInfo(got))
	})
	t.Run("entity template", func(t *testing.T) {
		info := &FetchInfo{
			DataSourceName: "accounts",
			DataSourceID:   "ds-1",
			RootFields:     []GraphCoordinate{{TypeName: "User", FieldName: "name"}},
		}
		cfg := FetchCacheConfiguration{
			CacheKeyTemplate: &EntityQueryCacheKeyTemplate{},
		}
		ctx := WithCacheFetchInfo(context.Background(), info, cfg)
		cfi := GetCacheFetchInfo(ctx)
		assert.Equal(t, "accounts", cfi.DataSourceName)
		assert.Equal(t, "ds-1", cfi.DataSourceID)
		assert.Equal(t, "entity", cfi.FetchType)
		assert.Equal(t, "User", cfi.TypeName)
		assert.Equal(t, "", cfi.FieldName)
	})
	t.Run("root field template", func(t *testing.T) {
		info := &FetchInfo{
			DataSourceName: "products",
			DataSourceID:   "ds-2",
			RootFields:     []GraphCoordinate{{TypeName: "Query", FieldName: "topProducts"}},
		}
		cfg := FetchCacheConfiguration{
			CacheKeyTemplate: &RootQueryCacheKeyTemplate{},
		}
		ctx := WithCacheFetchInfo(context.Background(), info, cfg)
		cfi := GetCacheFetchInfo(ctx)
		assert.Equal(t, "products", cfi.DataSourceName)
		assert.Equal(t, "ds-2", cfi.DataSourceID)
		assert.Equal(t, "rootField", cfi.FetchType)
		assert.Equal(t, "Query", cfi.TypeName)
		assert.Equal(t, "topProducts", cfi.FieldName)
	})
	t.Run("empty RootFields", func(t *testing.T) {
		info := &FetchInfo{DataSourceName: "x"}
		cfg := FetchCacheConfiguration{
			CacheKeyTemplate: &EntityQueryCacheKeyTemplate{},
		}
		ctx := WithCacheFetchInfo(context.Background(), info, cfg)
		cfi := GetCacheFetchInfo(ctx)
		assert.Equal(t, "entity", cfi.FetchType)
		assert.Equal(t, "", cfi.TypeName)
	})
}

func TestGetCacheFetchInfo(t *testing.T) {
	t.Run("not set", func(t *testing.T) {
		assert.Nil(t, GetCacheFetchInfo(context.Background()))
	})
	t.Run("set and retrieved", func(t *testing.T) {
		info := &FetchInfo{DataSourceName: "test", RootFields: []GraphCoordinate{{TypeName: "T"}}}
		cfg := FetchCacheConfiguration{CacheKeyTemplate: &EntityQueryCacheKeyTemplate{}}
		ctx := WithCacheFetchInfo(context.Background(), info, cfg)
		cfi := GetCacheFetchInfo(ctx)
		assert.NotNil(t, cfi)
		assert.Equal(t, "test", cfi.DataSourceName)
	})
}
