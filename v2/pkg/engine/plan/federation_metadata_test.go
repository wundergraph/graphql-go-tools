package plan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSubscriptionEntityPopulationConfigurations(t *testing.T) {
	// These tests verify FindByTypeAndFieldName, which disambiguates
	// subscription entity population configs when multiple subscription fields
	// (e.g. itemCreated, itemUpdated) return the same entity type (e.g. Item)
	// but have different TTLs or cache settings.

	t.Run("FindByTypeAndFieldName returns field-specific config", func(t *testing.T) {
		// Two subscription fields produce configs for the same entity type "Item"
		// but with different field names and TTLs. FindByTypeAndFieldName must
		// return the config matching both the type AND the field name.
		configs := SubscriptionEntityPopulationConfigurations{
			{
				TypeName:  "Item",
				FieldName: "itemCreated",
				CacheName: "items",
				TTL:       60 * time.Second,
			},
			{
				TypeName:  "Item",
				FieldName: "itemUpdated",
				CacheName: "items",
				TTL:       120 * time.Second,
			},
		}

		// "itemCreated" should match the 60s config, not the 120s one
		result := configs.FindByTypeAndFieldName("Item", "itemCreated")
		assert.NotNil(t, result)
		assert.Equal(t, "itemCreated", result.FieldName)
		assert.Equal(t, 60*time.Second, result.TTL)

		// "itemUpdated" should match the 120s config
		result = configs.FindByTypeAndFieldName("Item", "itemUpdated")
		assert.NotNil(t, result)
		assert.Equal(t, "itemUpdated", result.FieldName)
		assert.Equal(t, 120*time.Second, result.TTL)
	})

	t.Run("FindByTypeAndFieldName returns nil when field not found", func(t *testing.T) {
		configs := SubscriptionEntityPopulationConfigurations{
			{
				TypeName:  "Item",
				FieldName: "itemCreated",
				CacheName: "items",
				TTL:       60 * time.Second,
			},
		}

		// Field name mismatch → nil
		result := configs.FindByTypeAndFieldName("Item", "nonExistent")
		assert.Nil(t, result)

		// Type name mismatch → nil
		result = configs.FindByTypeAndFieldName("Order", "itemCreated")
		assert.Nil(t, result)
	})
}
