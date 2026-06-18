package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCachingOptions(t *testing.T) {
	t.Run("zero value defaults to caching disabled", func(t *testing.T) {
		options := (ExecutionOptions{}).Caching

		assert.Equal(t, CachingOptions{}, options)
		assert.Nil(t, options.L2CacheKeyInterceptor)
	})

	t.Run("L2 cache key interceptor transforms keys", func(t *testing.T) {
		interceptor := L2CacheKeyInterceptor(func(info L2CacheKeyInterceptorInfo, key string) string {
			return info.CacheName + ":" + key
		})

		actual := interceptor(L2CacheKeyInterceptorInfo{
			SubgraphName: "products",
			CacheName:    "entities",
		}, `{"__typename":"Product","key":{"id":"1"}}`)

		assert.Equal(t, `entities:{"__typename":"Product","key":{"id":"1"}}`, actual)
	})
}
