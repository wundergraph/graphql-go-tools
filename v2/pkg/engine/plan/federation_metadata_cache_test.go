package plan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEntityCacheConfigurationsFindByTypeName(t *testing.T) {
	configs := EntityCacheConfigurations{
		{
			TypeName:                    "Product",
			CacheName:                   "products",
			TTL:                         30 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnablePartialCacheLoad:      true,
			HashAnalyticsKeys:           true,
			ShadowMode:                  true,
			NegativeCacheTTL:            5 * time.Second,
		},
	}

	actual, exists := configs.FindByTypeName("Product")

	assert.Equal(t, EntityCacheConfiguration{
		TypeName:                    "Product",
		CacheName:                   "products",
		TTL:                         30 * time.Second,
		IncludeSubgraphHeaderPrefix: true,
		EnablePartialCacheLoad:      true,
		HashAnalyticsKeys:           true,
		ShadowMode:                  true,
		NegativeCacheTTL:            5 * time.Second,
	}, actual)
	assert.Equal(t, true, exists)

	actual, exists = configs.FindByTypeName("User")

	assert.Equal(t, EntityCacheConfiguration{}, actual)
	assert.Equal(t, false, exists)
}

func TestRootFieldCacheConfigurationsFindByTypeAndField(t *testing.T) {
	configs := RootFieldCacheConfigurations{
		{
			TypeName:                    "Query",
			FieldName:                   "product",
			CacheName:                   "products",
			TTL:                         time.Minute,
			IncludeSubgraphHeaderPrefix: true,
			EntityKeyMappings: []EntityKeyMapping{
				{
					EntityTypeName: "Product",
					FieldMappings: []FieldMapping{
						{
							EntityKeyField:      "upc",
							ArgumentPath:        []string{"input", "upc"},
							ArgumentIsEntityKey: true,
						},
					},
				},
			},
			ShadowMode:       true,
			PartialBatchLoad: true,
		},
	}

	actual, exists := configs.FindByTypeAndField("Query", "product")

	assert.Equal(t, RootFieldCacheConfiguration{
		TypeName:                    "Query",
		FieldName:                   "product",
		CacheName:                   "products",
		TTL:                         time.Minute,
		IncludeSubgraphHeaderPrefix: true,
		EntityKeyMappings: []EntityKeyMapping{
			{
				EntityTypeName: "Product",
				FieldMappings: []FieldMapping{
					{
						EntityKeyField:      "upc",
						ArgumentPath:        []string{"input", "upc"},
						ArgumentIsEntityKey: true,
					},
				},
			},
		},
		ShadowMode:       true,
		PartialBatchLoad: true,
	}, actual)
	assert.Equal(t, true, exists)

	actual, exists = configs.FindByTypeAndField("Query", "products")

	assert.Equal(t, RootFieldCacheConfiguration{}, actual)
	assert.Equal(t, false, exists)
}

func TestMutationFieldCacheConfigurationsFindByFieldName(t *testing.T) {
	configs := MutationFieldCacheConfigurations{
		{
			FieldName:                     "addReview",
			EnableEntityL2CachePopulation: true,
			TTL:                           45 * time.Second,
		},
	}

	actual, exists := configs.FindByFieldName("addReview")

	assert.Equal(t, MutationFieldCacheConfiguration{
		FieldName:                     "addReview",
		EnableEntityL2CachePopulation: true,
		TTL:                           45 * time.Second,
	}, actual)
	assert.Equal(t, true, exists)

	actual, exists = configs.FindByFieldName("updateReview")

	assert.Equal(t, MutationFieldCacheConfiguration{}, actual)
	assert.Equal(t, false, exists)
}

func TestMutationCacheInvalidationConfigurationsFindByFieldName(t *testing.T) {
	configs := MutationCacheInvalidationConfigurations{
		{
			FieldName:      "updateUser",
			EntityTypeName: "User",
		},
	}

	actual, exists := configs.FindByFieldName("updateUser")

	assert.Equal(t, MutationCacheInvalidationConfiguration{
		FieldName:      "updateUser",
		EntityTypeName: "User",
	}, actual)
	assert.Equal(t, true, exists)

	actual, exists = configs.FindByFieldName("deleteUser")

	assert.Equal(t, MutationCacheInvalidationConfiguration{}, actual)
	assert.Equal(t, false, exists)
}

func TestSubscriptionEntityPopulationConfigurationsFindByTypeAndFieldName(t *testing.T) {
	configs := SubscriptionEntityPopulationConfigurations{
		{
			TypeName:                    "Product",
			FieldName:                   "updateProductPrice",
			CacheName:                   "products",
			TTL:                         15 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnableInvalidationOnKeyOnly: true,
		},
		{
			TypeName:                    "Product",
			FieldName:                   "",
			CacheName:                   "invalid",
			TTL:                         30 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnableInvalidationOnKeyOnly: true,
		},
	}

	actual, exists := configs.FindByTypeAndFieldName("Product", "updateProductPrice")

	assert.Equal(t, SubscriptionEntityPopulationConfiguration{
		TypeName:                    "Product",
		FieldName:                   "updateProductPrice",
		CacheName:                   "products",
		TTL:                         15 * time.Second,
		IncludeSubgraphHeaderPrefix: true,
		EnableInvalidationOnKeyOnly: true,
	}, actual)
	assert.Equal(t, true, exists)

	actual, exists = configs.FindByTypeAndFieldName("Product", "updatedPrice")

	assert.Equal(t, SubscriptionEntityPopulationConfiguration{}, actual)
	assert.Equal(t, false, exists)

	actual, exists = SubscriptionEntityPopulationConfigurations{
		{
			TypeName:                    "Product",
			FieldName:                   "",
			CacheName:                   "invalid",
			TTL:                         30 * time.Second,
			IncludeSubgraphHeaderPrefix: true,
			EnableInvalidationOnKeyOnly: true,
		},
	}.FindByTypeAndFieldName("Product", "updateProductPrice")

	assert.Equal(t, SubscriptionEntityPopulationConfiguration{}, actual)
	assert.Equal(t, false, exists)

	actual, exists = configs.FindByTypeAndFieldName("Product", "")

	assert.Equal(t, SubscriptionEntityPopulationConfiguration{}, actual)
	assert.Equal(t, false, exists)

	actual, exists = configs.FindByTypeAndFieldName("", "updateProductPrice")

	assert.Equal(t, SubscriptionEntityPopulationConfiguration{}, actual)
	assert.Equal(t, false, exists)
}
